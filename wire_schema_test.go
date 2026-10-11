package prompty

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestWireSchemasMatchEmittedStateContracts(t *testing.T) {
	t.Parallel()
	// Arrange.
	response := stateFixture()
	wire, err := MarshalResponse(response, WirePolicy{State: StatePreserve})
	require.NoError(t, err)
	message, err := response.AssistantMessage()
	require.NoError(t, err)
	execution := SimplePrompt("fixture").AddMessage(message)
	executionWire, err := MarshalExecution(execution, WirePolicy{State: StatePreserve})
	require.NoError(t, err)
	responseJSON, err := json.Marshal(wire)
	require.NoError(t, err)
	executionJSON, err := json.Marshal(executionWire)
	require.NoError(t, err)
	// Act.
	decodedResponse, responseErr := DecodeResponseJSON(responseJSON)
	decodedExecution, executionErr := DecodeExecutionJSON(executionJSON)
	// Assert.
	require.NoError(t, responseErr)
	require.NoError(t, executionErr)
	require.Equal(t, response.ProviderState, decodedResponse.ProviderState)
	require.Equal(t, message.ProviderState, decodedExecution.Messages[1].ProviderState)
}

func TestWireSchemaRejectsForeignFieldsVariantsAndFormats(t *testing.T) {
	t.Parallel()
	for _, mutation := range []string{"unknown root", "mixed part", "old format", "negative usage"} {
		t.Run(mutation, func(t *testing.T) {
			t.Parallel()
			// Arrange.
			wire, err := MarshalResponse(stateFixture(), WirePolicy{State: StatePreserve})
			require.NoError(t, err)
			serialized, err := json.Marshal(wire)
			require.NoError(t, err)
			var value map[string]any
			require.NoError(t, json.Unmarshal(serialized, &value))
			switch mutation {
			case "unknown root":
				value["legacy"] = true
			case "mixed part":
				value["content"].([]any)[0].(map[string]any)["name"] = "not a text field"
			case "old format":
				delete(value, "format")
			case "negative usage":
				value["usage"].(map[string]any)["total_tokens"] = -1
			}
			invalid, err := json.Marshal(value)
			require.NoError(t, err)
			// Act.
			_, decodeErr := DecodeResponseJSON(invalid)
			// Assert.
			require.ErrorIs(t, decodeErr, ErrSchemaMismatch)
		})
	}
}

func TestWireSchemaResourcesAreIsolated(t *testing.T) {
	t.Parallel()
	// Arrange.
	resources := WireSchemaResources()
	// Act.
	resources["https://prompty.invalid/response-wire"][0] = 'X'
	delete(resources, "https://prompty.invalid/wire-common")
	// Assert.
	fresh := WireSchemaResources()
	require.JSONEq(
		t,
		string(fresh["https://prompty.invalid/response-wire"]),
		string(WireSchemaResources()["https://prompty.invalid/response-wire"]),
	)
	require.Contains(t, fresh, "https://prompty.invalid/wire-common")
}
