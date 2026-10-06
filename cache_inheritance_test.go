package prompty

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestMessageCacheInheritanceUsesEnclosingBlocks(t *testing.T) {
	t.Parallel()
	omit := &CachePolicy{Mechanism: CacheOmit, Mode: "", Provider: "", Controls: nil}
	for _, tc := range []struct {
		name      string
		parts     []ContentPart
		inherited bool
	}{
		{"empty", nil, false},
		{"default text", []ContentPart{TextPart{Text: "fixture", CachePolicy: nil}}, true},
		{"explicit text", []ContentPart{TextPart{Text: "fixture", CachePolicy: omit}}, false},
		{"pointer text", []ContentPart{&TextPart{Text: "fixture", CachePolicy: omit}}, false},
		{"nested override cannot suppress enclosing block", []ContentPart{ToolResultPart{
			ToolCallID: "call", Name: "lookup", IsError: false, CachePolicy: nil,
			Content: []ContentPart{TextPart{Text: "child", CachePolicy: omit}},
		}}, true},
		{"enclosing override suppresses message default", []ContentPart{ToolResultPart{
			ToolCallID: "call", Name: "lookup", IsError: false, CachePolicy: omit,
			Content: []ContentPart{TextPart{Text: "child", CachePolicy: nil}},
		}}, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			// Arrange.
			message := ChatMessage{Role: RoleTool, Content: tc.parts, CachePolicy: &CachePolicy{
				Mechanism: CacheBlockMarker, Mode: CacheAdvisory, Provider: "", Controls: nil,
			}}
			// Act.
			inherited, err := MessageInheritsCachePolicy(message)
			// Assert.
			require.NoError(t, err)
			require.Equal(t, tc.inherited, inherited)
			require.Equal(t, CacheOmit, omit.Mechanism)
		})
	}
}

func TestMessageWithoutCacheDefaultDoesNotInheritPolicy(t *testing.T) {
	t.Parallel()
	// Arrange.
	message := ChatMessage{Role: RoleUser, Content: []ContentPart{TextPart{Text: "fixture"}}, CachePolicy: nil}
	// Act.
	inherited, err := MessageInheritsCachePolicy(message)
	// Assert.
	require.NoError(t, err)
	require.False(t, inherited)
}
