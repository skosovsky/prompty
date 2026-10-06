package prompty

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestProviderSettingsPreserveIntegerPrecision(t *testing.T) {
	t.Parallel()
	// Arrange.
	options := &ModelOptions{ProviderSettings: JSONDocument(`{"budget":9007199254740993,"fraction":1024.5}`)}
	// Act.
	settings, err := ProviderSettingsMap(options)
	// Assert.
	require.NoError(t, err)
	require.Equal(t, json.Number("9007199254740993"), settings["budget"])
	require.Equal(t, json.Number("1024.5"), settings["fraction"])
}
