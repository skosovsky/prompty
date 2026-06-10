package manifest

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestPeekComposeFieldsE(t *testing.T) {
	t.Parallel()
	parser := NewJSONParser()

	t.Run("flat", func(t *testing.T) {
		t.Parallel()
		data := []byte(`{"id":"flat","messages":[{"role":"user","content":[{"type":"text","text":"hi"}]}]}`)
		ok, err := PeekComposeFieldsE(data, parser)
		require.NoError(t, err)
		assert.False(t, ok)
	})

	t.Run("compose_imports", func(t *testing.T) {
		t.Parallel()
		data := []byte(`{"id":"main","imports":[{"id":"child"}]}`)
		ok, err := PeekComposeFieldsE(data, parser)
		require.NoError(t, err)
		assert.True(t, ok)
	})

	t.Run("compose_layers", func(t *testing.T) {
		t.Parallel()
		data := []byte(`{"id":"main","layers":[{"id":"base","role":"system","content":[{"type":"text","text":"x"}]}]}`)
		ok, err := PeekComposeFieldsE(data, parser)
		require.NoError(t, err)
		assert.True(t, ok)
	})

	t.Run("corrupt", func(t *testing.T) {
		t.Parallel()
		ok, err := PeekComposeFieldsE([]byte(`{not-json`), parser)
		require.Error(t, err)
		assert.False(t, ok)
	})

	t.Run("empty", func(t *testing.T) {
		t.Parallel()
		ok, err := PeekComposeFieldsE(nil, parser)
		require.NoError(t, err)
		assert.False(t, ok)
	})
}

func TestPeekComposeOrError_MatchesPeekComposeFieldsE(t *testing.T) {
	t.Parallel()
	parser := NewJSONParser()
	data := []byte(`{"id":"main","imports":[{"id":"child"}]}`)
	want, wantErr := PeekComposeFieldsE(data, parser)
	got, gotErr := PeekComposeOrError(data, parser)
	assert.Equal(t, want, got)
	assert.Equal(t, wantErr, gotErr)
}
