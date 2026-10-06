package adapter

import (
	"errors"

	"github.com/skosovsky/prompty"
)

// PreparedBatchResultItem binds local original-schema validation to a checked
// results profile. It contains no SDK request and cannot dispatch generation.
type PreparedBatchResultItem struct {
	Diagnostics prompty.ExecutionDiagnostics
	schemas     preparedSchemas
	forcedTool  string
}

// PrepareBatchResultItem checks retrieval capability and compiles original
// contracts without replaying inputs or requiring generation-only leases.
func PrepareBatchResultItem[Req, Resp any](
	adp ProviderAdapter[Req, Resp], exec *prompty.PromptExecution,
) (*PreparedBatchResultItem, error) {
	if err := ValidateExecution(exec); err != nil {
		return nil, err
	}
	if exec.Capture != nil || len(exec.Requirements.Capture.Stages) != 0 || exec.Requirements.Capture.Accuracy != "" ||
		exec.Requirements.Capture.Durable || exec.Requirements.Capture.PhysicalAttempts {
		return nil, prompty.ErrCaptureUnsupported
	}
	describer, ok := any(adp).(RuntimeDescriber)
	if !ok {
		return nil, ErrNoRuntimeDescriptor
	}
	original := exec.Clone()
	model := ""
	if original.ModelOptions != nil {
		model = original.ModelOptions.Model
	}
	report, err := PreflightOperation(CloneRuntimeDescriptor(describer.RuntimeDescriptor()), model,
		[]string{prompty.CapabilityBatchResults}, original.Requirements.Override)
	if err != nil {
		return nil, err
	}
	schemas, err := compilePreparedSchemas(batchOriginalSchemaMapper{}, original, report.Profile)
	if err != nil {
		return nil, err
	}
	report.Schemas = schemas.reports()
	return &PreparedBatchResultItem{Diagnostics: report, schemas: schemas, forcedTool: original.ForcedTool}, nil
}

type batchOriginalSchemaMapper struct{}

func (batchOriginalSchemaMapper) MapSchema(input prompty.SchemaMappingRequest) (prompty.SchemaMapping, error) {
	return prompty.SchemaMapping{Schema: prompty.CloneJSONDocument(input.Source), Transformations: nil}, nil
}

func ParseBatchResultItem[Req, Resp any](
	adp ProviderAdapter[Req, Resp], prepared *PreparedBatchResultItem, raw Resp,
) (*prompty.Response, error) {
	if prepared == nil {
		return nil, prompty.ErrBatchScope
	}
	var response *prompty.Response
	var err error
	if parser, ok := any(adp).(ProfileResponseParser[Resp]); ok {
		response, err = parser.ParseResponseForProfile(raw, prepared.Diagnostics.Profile)
	} else {
		response, err = adp.ParseResponse(raw)
	}
	if response == nil {
		return nil, errors.Join(err, ErrInvalidResponse)
	}
	response.Diagnostics = prompty.CloneExecutionDiagnostics(prepared.Diagnostics)
	if err != nil {
		return response, err
	}
	if err = prompty.ValidateResponseMetadata(response); err != nil {
		return response, err
	}
	if prepared.forcedTool != "" && response.Outcome != prompty.OutcomeRefusal &&
		response.Outcome != prompty.OutcomeIncomplete {
		calls := 0
		for _, part := range response.Content {
			if call, ok := part.(prompty.ToolCallPart); ok {
				if call.Name != prepared.forcedTool {
					return response, prompty.ErrSchemaMismatch
				}
				calls++
			}
		}
		if calls == 0 {
			return response, prompty.ErrSchemaMismatch
		}
	}
	return response, prepared.schemas.validate(response)
}
