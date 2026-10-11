package anthropic

import (
	"encoding/json"

	sdk "github.com/anthropics/anthropic-sdk-go"

	"github.com/skosovsky/prompty"
	"github.com/skosovsky/prompty/adapter"
)

// Caller-constructed SDK values have no source attestation. Observed block values
// must be checked before SDK coercions can become canonical text/tool identities.
func validateConsumedBlockSource(block sdk.ContentBlockUnion) error {
	raw := block.RawJSON()
	if raw == "" || block.Type != wireText && block.Type != wireToolUse {
		return nil
	}
	if !uniqueSyntheticJSON([]byte(raw)) {
		return adapter.ErrInvalidResponse
	}
	fields, err := nativeSourceObject([]byte(raw))
	if err != nil {
		return err
	}
	kind, ok := sourceString(fields, wireType, true)
	if !ok || kind != block.Type {
		return adapter.ErrInvalidResponse
	}
	switch kind {
	case wireText:
		if _, ok := sourceString(fields, wireText, false); !ok {
			return adapter.ErrInvalidResponse
		}
	case wireToolUse:
		for _, key := range []string{"id", "name"} {
			if _, ok := sourceString(fields, key, true); !ok {
				return adapter.ErrInvalidResponse
			}
		}
		var object map[string]json.RawMessage
		if json.Unmarshal(fields["input"], &object) != nil || object == nil {
			return adapter.ErrMalformedArgs
		}
	}
	return nil
}

func partialAnthropicSourceFailure(
	parts []prompty.ContentPart,
	states []prompty.PartState,
	annotations []prompty.PartAnnotation,
	err error,
) (*prompty.Response, error) {
	if len(parts) == 0 {
		return nil, err
	}
	return &prompty.Response{
		Model:              "",
		MessageAnnotations: nil,
		Usage: prompty.Usage{
			PromptTokens:              0,
			CompletionTokens:          0,
			TotalTokens:               0,
			PromptTokensCached:        0,
			PromptTokensCacheCreation: 0,
			CompletionTokensReasoning: 0,
			Known:                     nil,
			Mode:                      "",
		},
		FinishReason: "",
		Outcome:      "",
		Diagnostics: prompty.ExecutionDiagnostics{
			Profile:    prompty.ProfileIdentity{Provider: "", Endpoint: "", Model: "", ModelFamily: ""},
			Snapshot:   "",
			Provenance: "",
			Override:   false,
			Checks:     nil,
			Schemas:    nil,
			Capture:    nil,
			Cache:      nil,
		},
		Content:                 parts,
		ProviderState:           states,
		Annotations:             annotations,
		ContinuationUnavailable: true,
	}, err
}

// Invalid observed counters cannot use the SDK's coerced number. Preserve the
// independently valid counters, while preventing derived totals from using it.
func sanitizeObservedUsage(usage *sdk.Usage) (map[string]bool, error) {
	invalid := make(map[string]bool)
	if usage.RawJSON() == "" {
		return invalid, nil
	}
	fields, err := nativeSourceObject([]byte(usage.RawJSON()))
	if err != nil {
		return nil, prompty.ErrUsage
	}
	for key, counter := range map[string]*int64{
		"input_tokens":                &usage.InputTokens,
		"output_tokens":               &usage.OutputTokens,
		"cache_read_input_tokens":     &usage.CacheReadInputTokens,
		"cache_creation_input_tokens": &usage.CacheCreationInputTokens,
	} {
		if _, present := fields[key]; !present {
			continue
		}
		if _, valid := sourceIndex(fields, key); !valid {
			invalid[key] = true
			*counter = 0
		}
	}
	if len(invalid) > 0 {
		return invalid, prompty.ErrUsage
	}
	return invalid, nil
}
