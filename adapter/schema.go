package adapter

import (
	"fmt"
	"strings"

	"github.com/skosovsky/prompty"
)

type preparedSchemas struct {
	output *prompty.CompiledProviderSchema
	tools  map[string]*prompty.CompiledProviderSchema
	order  []string
}

func compilePreparedSchemas(
	adp any, exec *prompty.PromptExecution, profile prompty.ProfileIdentity,
) (preparedSchemas, error) {
	prepared := preparedSchemas{output: nil, tools: make(map[string]*prompty.CompiledProviderSchema), order: nil}
	if exec.ResponseFormat == nil && len(exec.Tools) == 0 {
		return prepared, nil
	}
	mapper, ok := adp.(prompty.ProviderSchemaMapper)
	if !ok {
		return prepared, fmt.Errorf("%w: adapter has no schema mapper", prompty.ErrSchemaUnsupported)
	}
	validator := exec.Validator
	if validator == nil {
		validator = prompty.NewJSONSchemaValidator(nil)
	}
	compile := func(source prompty.JSONDocument, dialect string, role prompty.SchemaRole, name string) (*prompty.CompiledProviderSchema, error) {
		return prompty.CompileProviderSchema(prompty.SchemaCompileRequest{
			Source: source, Dialect: dialect, Role: role, Name: name, Profile: profile,
			Policy: exec.Requirements.Schema, Validator: validator,
		}, mapper)
	}
	if exec.ResponseFormat != nil {
		compiled, err := compile(
			exec.ResponseFormat.Schema,
			exec.ResponseFormat.Dialect,
			prompty.SchemaOutput,
			exec.ResponseFormat.Name,
		)
		if err != nil {
			return prepared, err
		}
		prepared.output = compiled
		exec.ResponseFormat.Schema = compiled.Translated()
	}
	for i := range exec.Tools {
		tool := &exec.Tools[i]
		if tool.Name == "" || prepared.tools[tool.Name] != nil {
			return prepared, fmt.Errorf("%w: empty or duplicate tool schema name", prompty.ErrSchemaUnsupported)
		}
		source := tool.Parameters
		if len(source) == 0 {
			source = prompty.JSONDocument(`{"type":"object"}`)
		}
		compiled, err := compile(source, tool.Dialect, prompty.SchemaToolInput, tool.Name)
		if err != nil {
			return prepared, err
		}
		prepared.tools[tool.Name] = compiled
		prepared.order = append(prepared.order, tool.Name)
		tool.Parameters = compiled.Translated()
	}
	return prepared, nil
}

func (p preparedSchemas) reports() []prompty.SchemaCompilationReport {
	var reports []prompty.SchemaCompilationReport
	if p.output != nil {
		reports = append(reports, p.output.Report())
	}
	for _, name := range p.order {
		reports = append(reports, p.tools[name].Report())
	}
	return reports
}

func (p preparedSchemas) validate(response *prompty.Response) error {
	if p.output == nil && len(p.tools) == 0 {
		return nil
	}
	if response.Outcome == prompty.OutcomeRefusal || response.Outcome == prompty.OutcomeIncomplete {
		return &prompty.OutcomeError{Outcome: response.Outcome, FinishReason: response.FinishReason}
	}
	if response.Outcome == prompty.OutcomePaused {
		// Validate any local call data, but do not claim completed output schema
		// success or turn a valid provider pause into a failed exchange.
		_, err := p.validateToolParts(response.Content)
		return err
	}
	calls, err := p.validateToolParts(response.Content)
	if err != nil {
		return err
	}
	if response.Outcome == prompty.OutcomeToolCalls {
		if calls == 0 {
			return ErrInvalidResponse
		}
		return nil
	}
	if calls != 0 {
		return ErrInvalidResponse
	}
	if p.output == nil {
		return nil
	}
	if response.Outcome != prompty.OutcomeCompleted {
		return &prompty.OutcomeError{Outcome: response.Outcome, FinishReason: response.FinishReason}
	}
	return validateCompiledOutput(p.output, response.Content)
}

func validateCompiledOutput(schema *prompty.CompiledProviderSchema, parts []prompty.ContentPart) error {
	var text strings.Builder
	for _, part := range parts {
		if _, reasoning := part.(prompty.ReasoningPart); reasoning {
			continue
		}
		t, ok := part.(prompty.TextPart)
		if !ok {
			return fmt.Errorf("%w: non-text structured output", prompty.ErrSchemaMismatch)
		}
		text.WriteString(t.Text)
	}
	return schema.ValidateOriginal(prompty.JSONDocument(text.String()))
}

func (p preparedSchemas) validateToolParts(parts []prompty.ContentPart) (int, error) {
	count := 0
	for _, part := range parts {
		call, ok := part.(prompty.ToolCallPart)
		if !ok {
			continue
		}
		count++
		schema := p.tools[call.Name]
		if schema == nil {
			return count, fmt.Errorf("%w: response tool has no compiled execution schema", prompty.ErrSchemaMismatch)
		}
		if err := schema.ValidateOriginal(prompty.JSONDocument(call.Args)); err != nil {
			return count, err
		}
	}
	return count, nil
}
