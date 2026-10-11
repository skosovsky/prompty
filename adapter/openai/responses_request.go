package openai

import (
	"encoding/json"
	"strings"

	sdk "github.com/openai/openai-go/v3"
	"github.com/openai/openai-go/v3/responses"

	"github.com/skosovsky/prompty"
	"github.com/skosovsky/prompty/adapter"
)

type preparedRemoteResponse struct {
	params responses.ResponseNewParams
	schema *prompty.CompiledProviderSchema
	tools  responseToolContract
}

// MapSchema shares OpenAI schema semantics without sharing endpoint capabilities.
func (r *Responses) MapSchema(input prompty.SchemaMappingRequest) (prompty.SchemaMapping, error) {
	if input.Profile.Provider != "openai.responses" {
		return prompty.SchemaMapping{}, prompty.ErrSchemaUnsupported
	}
	input.Profile.Provider = wireOpenai
	return r.base.MapSchema(input)
}

func (r *Responses) compileOutput(
	exec *prompty.PromptExecution,
	report *prompty.ExecutionDiagnostics,
	mapper prompty.ProviderSchemaMapper,
) (*prompty.CompiledProviderSchema, error) {
	if exec.ResponseFormat == nil {
		return nil, nil //nolint:nilnil // No mapped value or emitted frame is a successful absence, not an operation failure.
	}
	validator := exec.Validator
	if validator == nil {
		validator = prompty.NewJSONSchemaValidator(nil)
	}
	definition := exec.ResponseFormat
	schema, err := prompty.CompileProviderSchema(prompty.SchemaCompileRequest{
		Source: definition.Schema, Dialect: definition.Dialect, Role: prompty.SchemaOutput, Name: definition.Name,
		Profile: report.Profile, Policy: exec.Requirements.Schema, Validator: validator,
	}, mapper)
	if err == nil {
		report.Schemas = append(report.Schemas, schema.Report())
	}
	return schema, err
}

func (r *Responses) restoreOutputSchema(
	exec *prompty.PromptExecution,
	handle prompty.RemoteResponseHandle,
	report *prompty.ExecutionDiagnostics,
) (*prompty.CompiledProviderSchema, error) {
	if (exec.ResponseFormat == nil) != (handle.OutputSchemaDigest == "") {
		return nil, prompty.ErrSchemaValidatorRequired
	}
	schema, err := r.compileOutput(exec, report, responseOriginalSchemaMapper{})
	if err == nil && schema != nil && responseSchemaBindingDigest(schema) != handle.OutputSchemaDigest {
		return nil, prompty.ErrRemoteResponseScope
	}
	return schema, err
}

//nolint:funlen,gocognit,nestif // Ordered provider protocol checks preserve capture, partial-result and terminal-state semantics.
func (r *Responses) prepareRequest(
	exec *prompty.PromptExecution,
	input prompty.RemoteResponseRequest,
	report *prompty.ExecutionDiagnostics,
) (preparedRemoteResponse, error) {
	var prepared preparedRemoteResponse
	if exec.CacheResource != nil {
		return prepared, adapter.ErrUnsupportedCapability
	}
	if err := adapter.RejectSyntheticOutput(exec); err != nil {
		return prepared, err
	}
	//nolint:exhaustruct_v5 // SDK DTO: optional fields and unselected union arms stay zero.
	params := responses.ResponseNewParams{
		Model:        report.Profile.Model,
		Store:        sdk.Bool(*input.Store),
		Background:   sdk.Bool(input.Background),
		Instructions: sdk.String(""),
	}
	if err := applyResponsesRequestCache(&params, exec.RequestCache); err != nil {
		return prepared, err
	}
	if err := prepareHostedTools(exec.HostedTools, &params); err != nil {
		return prepared, err
	}
	tools, toolsErr := r.compileTools(exec, report, false)
	if toolsErr != nil {
		return prepared, toolsErr
	}
	if toolsErr = prepareResponseTools(exec, tools, &params, input.StrictTools); toolsErr != nil {
		return prepared, toolsErr
	}
	if responseRequiresOutputState(exec) {
		params.Include = append(params.Include, responses.ResponseIncludableReasoningEncryptedContent)
	}
	if input.Previous != nil {
		params.PreviousResponseID = sdk.String(input.Previous.ProviderID)
	}
	var instructions []string
	history := newResponseToolHistory(input.Previous)
	for _, message := range exec.Messages {
		if message.Role == prompty.RoleSystem || message.Role == prompty.RoleDeveloper {
			text, err := responseInputText(message)
			if err != nil {
				return prepared, err
			}
			instructions = append(instructions, text)
			continue
		}
		if message.Role == prompty.RoleTool {
			items, resultErr := history.resultInput(message)
			if resultErr != nil {
				return prepared, resultErr
			}
			params.Input.OfInputItemList = append(params.Input.OfInputItemList, items...)
			continue
		}
		if message.Role != prompty.RoleUser && message.Role != prompty.RoleAssistant {
			return prepared, adapter.ErrUnsupportedCapability
		}
		items, err := responseMessageInput(message, report.Profile)
		if err != nil {
			return prepared, err
		}
		if historyErr := history.register(message); historyErr != nil {
			return prepared, historyErr
		}
		params.Input.OfInputItemList = append(params.Input.OfInputItemList, items...)
	}
	params.Instructions = sdk.String(strings.Join(instructions, "\n\n"))
	if options := exec.ModelOptions; options != nil {
		if len(options.Stop) != 0 {
			return prepared, adapter.ErrUnsupportedCapability
		}
		if err := applyResponsesReasoning(&params, options); err != nil {
			return prepared, err
		}
		if options.Temperature != nil {
			params.Temperature = sdk.Float(*options.Temperature)
		}
		if options.TopP != nil {
			params.TopP = sdk.Float(*options.TopP)
		}
		if options.MaxTokens != nil {
			params.MaxOutputTokens = sdk.Int(*options.MaxTokens)
		}
	}
	schema, err := r.compileOutput(exec, report, r)
	if err != nil {
		return prepared, err
	}
	if schema != nil {
		mapped, mapErr := prompty.SchemaDocumentAsMap(schema.Translated())
		if mapErr != nil {
			return prepared, mapErr
		}
		params.Text.Format.OfJSONSchema = &responses.ResponseFormatTextJSONSchemaConfigParam{ //nolint:exhaustruct_v5 // SDK DTO: omitted optional fields and other union arms retain their zero values.
			Name:        exec.ResponseFormat.Name,
			Description: sdk.String(exec.ResponseFormat.Description),
			Schema:      schemaSDKMap(mapped),
			Strict:      sdk.Bool(true),
		}
	}
	return preparedRemoteResponse{params: params, schema: schema, tools: tools}, nil
}

// Retrieval validates the original contract without remapping/resending a schema.
// Request lowering does not need to be reauthorized to inspect accepted output.
type responseOriginalSchemaMapper struct{}

func (responseOriginalSchemaMapper) MapSchema(input prompty.SchemaMappingRequest) (prompty.SchemaMapping, error) {
	return prompty.SchemaMapping{Transformations: nil, Schema: prompty.CloneJSONDocument(input.Source)}, nil
}

func responseInputText(message prompty.ChatMessage) (string, error) {
	if !responseCacheOmitted(message.CachePolicy) {
		return "", adapter.ErrUnsupportedCapability
	}
	if len(message.ProviderState) != 0 || message.ContinuationUnavailable ||
		len(message.Annotations) != 0 || len(message.MessageAnnotations) != 0 || len(message.Metadata) != 0 {
		return "", adapter.ErrUnsupportedCapability
	}
	var text strings.Builder
	for _, part := range message.Content {
		switch value := part.(type) {
		case prompty.TextPart:
			if !responseCacheOmitted(value.CachePolicy) {
				return "", adapter.ErrUnsupportedCapability
			}
			text.WriteString(value.Text)
		case *prompty.TextPart:
			if value == nil || !responseCacheOmitted(value.CachePolicy) {
				return "", adapter.ErrUnsupportedCapability
			}
			text.WriteString(value.Text)
		default:
			return "", adapter.ErrUnsupportedCapability
		}
	}
	return text.String(), nil
}

func validateRemoteOutput(schema *prompty.CompiledProviderSchema, response *prompty.Response) error {
	if schema == nil {
		return nil
	}
	if response.Outcome != prompty.OutcomeCompleted {
		return &prompty.OutcomeError{Outcome: response.Outcome, FinishReason: response.FinishReason}
	}
	var document strings.Builder
	for _, part := range response.Content {
		if _, ok := part.(prompty.ReasoningPart); ok {
			continue
		}
		text, ok := part.(prompty.TextPart)
		if !ok {
			return prompty.ErrSchemaJSONDecode
		}
		document.WriteString(text.Text)
	}
	if !json.Valid([]byte(document.String())) {
		return prompty.ErrSchemaJSONDecode
	}
	return schema.ValidateOriginal([]byte(document.String()))
}
