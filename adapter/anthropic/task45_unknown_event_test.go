package anthropic

import (
	"encoding/json"
	"testing"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/stretchr/testify/require"

	"github.com/skosovsky/prompty/adapter"
)

func TestPinnedStreamRejectsUnknownSemanticVariants(t *testing.T) {
	t.Parallel()
	for _, fixture := range []struct {
		raw   string
		cause error
	}{
		{`{"type":"future_semantic_event"}`, adapter.ErrUnsupportedContentType},
		{`{"type":"content_block_delta","index":0,"delta":{"type":"future_delta","text":"known"}}`, adapter.ErrUnsupportedContentType},
		{`{"type":"content_block_start","index":0,"content_block":{"type":"future_content","text":"known"}}`, adapter.ErrUnsupportedContentType},
	} {
		// Arrange: a message is active, but this SDK union has no supported mapping.
		var event anthropic.MessageStreamEventUnion
		require.NoError(t, json.Unmarshal([]byte(fixture.raw), &event))
		state := new(messageStreamState)
		state.started = true
		state.blocks = make(map[int64]*messageStreamBlock)
		if event.Type == "content_block_delta" {
			state.blocks[0] = &messageStreamBlock{kind: "text"}
		}
		// Act.
		chunk, err := state.accept(event)
		// Assert: unknown semantics never become a ping/empty successful response.
		require.ErrorIs(t, err, fixture.cause)
		require.Nil(t, chunk)
	}
	// Valid control: only the known ping is ignored.
	var ping anthropic.MessageStreamEventUnion
	require.NoError(t, json.Unmarshal([]byte(`{"type":"ping"}`), &ping))
	state := new(messageStreamState)
	chunk, err := state.accept(ping)
	require.NoError(t, err)
	require.Nil(t, chunk)
}
