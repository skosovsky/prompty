package openai

import (
	"bytes"
	"encoding/json"
	"maps"
	"strings"

	sdk "github.com/openai/openai-go/v3"
	"github.com/openai/openai-go/v3/responses"

	"github.com/skosovsky/prompty"
	"github.com/skosovsky/prompty/adapter"
)

const responsesFunctionCodec = "openai.responses.function_call.json"

type responseToolContract struct {
	schemas map[string]*prompty.CompiledProviderSchema
	digests map[string]string
	forced  string
}

func (r *Responses) compileTools(
	exec *prompty.PromptExecution,
	report *prompty.ExecutionDiagnostics,
	original bool,
) (responseToolContract, error) {
	contract := responseToolContract{
		schemas: make(map[string]*prompty.CompiledProviderSchema),
		digests: make(map[string]string),
		forced:  exec.ForcedTool,
	}
	validator := exec.Validator
	if validator == nil {
		validator = prompty.NewJSONSchemaValidator(nil)
	}
	var mapper prompty.ProviderSchemaMapper = r
	if original {
		mapper = responseOriginalSchemaMapper{}
	}
	for _, tool := range exec.Tools {
		if strings.TrimSpace(tool.Name) == "" || contract.schemas[tool.Name] != nil {
			return contract, prompty.ErrSchemaUnsupported
		}
		source := tool.Parameters
		if len(source) == 0 {
			source = []byte(`{"type":"object"}`)
		}
		compiled, err := prompty.CompileProviderSchema(prompty.SchemaCompileRequest{
			Source: source, Dialect: tool.Dialect, Role: prompty.SchemaToolInput, Name: tool.Name,
			Profile: report.Profile, Policy: exec.Requirements.Schema, Validator: validator,
		}, mapper)
		if err != nil {
			return contract, err
		}
		contract.schemas[tool.Name] = compiled
		contract.digests[tool.Name] = responseSchemaBindingDigest(compiled)
		report.Schemas = append(report.Schemas, compiled.Report())
	}
	for _, name := range exec.RequiredTools {
		if contract.schemas[name] == nil {
			return contract, prompty.ErrSchemaUnsupported
		}
	}
	if contract.forced != "" && contract.schemas[contract.forced] == nil {
		return contract, adapter.ErrForcedToolNotFound
	}
	return contract, nil
}

func (r *Responses) restoreTools(
	exec *prompty.PromptExecution,
	handle prompty.RemoteResponseHandle,
	report *prompty.ExecutionDiagnostics,
) (responseToolContract, error) {
	exec.ForcedTool = handle.ForcedTool
	contract, err := r.compileTools(exec, report, true)
	if err != nil {
		return contract, err
	}
	if !maps.Equal(contract.digests, handle.ToolSchemaDigests) {
		return contract, prompty.ErrRemoteResponseScope
	}
	return contract, nil
}

func prepareResponseTools(
	exec *prompty.PromptExecution,
	contract responseToolContract,
	params *responses.ResponseNewParams,
	strict bool,
) error {
	for _, tool := range exec.Tools {
		parameters, err := prompty.SchemaDocumentAsMap(contract.schemas[tool.Name].Translated())
		if err != nil {
			return err
		}
		if strict {
			if err := validateStrictFunctionSchema(parameters); err != nil {
				return err
			}
		}
		params.Tools = append(
			params.Tools,
			responses.ToolUnionParam{OfFunction: &responses.FunctionToolParam{
				Name:        tool.Name,
				Description: sdk.String(tool.Description),
				Parameters:  schemaSDKMap(parameters),
				Strict:      sdk.Bool(strict),
			}},
		)
	}
	// Replace hosted preparation's explicit empty tools extra field with the final
	// combined list, so local functions cannot be shadowed during SDK encoding.
	if len(params.Tools) != 0 {
		params.SetExtraFields(map[string]any{"tools": params.Tools})
	}
	if contract.forced != "" {
		params.ToolChoice.OfFunctionTool = &responses.ToolChoiceFunctionParam{Name: contract.forced}
	}
	return nil
}

func parseResponseFunction(
	source []byte,
	response *prompty.Response,
	profile prompty.ProfileIdentity,
	contract responseToolContract,
	seen map[string]bool,
) error {
	var call responses.ResponseFunctionToolCall
	var fields map[string]json.RawMessage
	if json.Unmarshal(source, &fields) != nil || json.Unmarshal(source, &call) != nil ||
		!call.JSON.CallID.Valid() || !call.JSON.Name.Valid() || !call.JSON.Arguments.Valid() ||
		strings.TrimSpace(call.CallID) == "" ||
		seen[call.CallID] ||
		call.Type != "function_call" {
		return prompty.ErrRemoteResponse
	}
	seen[call.CallID] = true
	if id, present := fields["id"]; present {
		var value string
		if json.Unmarshal(id, &value) != nil || strings.TrimSpace(value) == "" {
			return prompty.ErrRemoteResponse
		}
	}
	if status, present := fields["status"]; present {
		var value string
		if json.Unmarshal(status, &value) != nil ||
			(value != "completed" && value != "in_progress" && value != "incomplete") {
			return prompty.ErrRemoteResponse
		}
	}
	if call.Status != "" && call.Status != "completed" && call.Status != "in_progress" &&
		call.Status != "incomplete" {
		return prompty.ErrRemoteResponse
	}
	index := len(response.Content)
	response.Content = append(
		response.Content,
		prompty.ToolCallPart{ID: call.CallID, Name: call.Name, Args: call.Arguments},
	)
	response.ProviderState = append(
		response.ProviderState,
		prompty.PartState{PartIndex: index, Envelope: prompty.ProviderEnvelope{
			Format:   prompty.ProviderStateFormat,
			Scope:    profile,
			Codec:    responsesFunctionCodec,
			Required: true,
			Payload:  bytes.Clone(source),
		}},
	)
	if call.Status != "" && call.Status != "completed" {
		if response.Outcome != prompty.OutcomeRefusal {
			response.Outcome = prompty.OutcomeIncomplete
		}
		return nil
	}
	schema := contract.schemas[call.Name]
	if schema == nil || (contract.forced != "" && contract.forced != call.Name) {
		return prompty.ErrSchemaMismatch
	}
	if !uniqueResponseJSON([]byte(call.Arguments)) {
		return adapter.ErrMalformedArgs
	}
	if err := schema.ValidateOriginal([]byte(call.Arguments)); err != nil {
		return err
	}
	if response.Outcome == prompty.OutcomeCompleted {
		response.Outcome = prompty.OutcomeToolCalls
	}
	return nil
}

func validateResponseToolSelection(
	response *prompty.Response,
	contract responseToolContract,
) error {
	if contract.forced == "" || response.Outcome == prompty.OutcomeRefusal ||
		response.Outcome == prompty.OutcomeIncomplete {
		return nil
	}
	if response.Outcome != prompty.OutcomeToolCalls {
		return prompty.ErrSchemaMismatch
	}
	return nil
}
