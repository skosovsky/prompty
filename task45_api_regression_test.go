package prompty

import (
	"strconv"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestRenderPlanConstructorNilContract(t *testing.T) {
	t.Parallel()
	// Arrange / Act / Assert: convenience misuse fails immediately, checked input returns an error.
	require.PanicsWithValue(t, "render plan: template must not be nil", func() { NewRenderPlan(nil) })
	plan, err := NewRenderPlanFromPlanInput(nil, RegistryPlanInput{})
	require.Error(t, err)
	require.Nil(t, plan)
	template, err := NewChatPromptTemplate([]MessageTemplate{{Role: RoleUser, Content: TextContent("ok")}})
	require.NoError(t, err)
	require.NotNil(t, NewRenderPlan(template))
}

func TestStructuredDiagnosticSuffixAcrossFragments(t *testing.T) {
	t.Parallel()
	// Arrange: a long unterminated object spans multiple preview wraps and feeds.
	document := `{"text":"BEGIN_SECRET` + strings.Repeat("abcd", 2048) + "end marker"
	parser := newStructuredStreamParser[struct {
		Text string `json:"text"`
	}]()
	for start := 0; start < len(document); start += 137 {
		end := min(start+137, len(document))
		_, err := parser.feed(document[start:end])
		require.NoError(t, err)
	}
	// Act.
	err := parser.finish()
	// Assert: preserve the same bounded suffix, without retaining the old prefix in diagnostics.
	require.Error(t, err)
	require.ErrorIs(t, err, ErrStructuredStream)
	require.Contains(t, err.Error(), strconv.Quote(document[len(document)-streamBufferPreviewLimit:]))
	require.NotContains(t, err.Error(), "BEGIN_SECRET")
}
