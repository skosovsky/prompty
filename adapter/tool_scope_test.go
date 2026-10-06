package adapter

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/skosovsky/prompty"
)

func TestCheckedAdapterRejectsChangedBoundToolDefinition(t *testing.T) {
	t.Parallel()
	// Arrange: policy survives cloning inside the checked adapter client.
	fixture := &checkedFixture{descriptor: fixtureDescriptor()}
	execution := prompty.SimplePrompt("work")
	execution.Tools = []prompty.ToolDefinition{{Name: "read"}}
	execution, err := execution.WithToolScope(prompty.ToolScope{Allowed: []prompty.ToolManifest{{Name: "read"}}})
	require.NoError(t, err)
	execution.Tools[0].Name = "delete_all"
	// Act.
	_, err = NewClient(nativeCheckedFixture{fixture}).Execute(context.Background(), execution)
	// Assert: neither translation nor SDK dispatch happens.
	require.ErrorIs(t, err, prompty.ErrToolScope)
	require.Zero(t, fixture.translations)
	require.Zero(t, fixture.calls)
}
