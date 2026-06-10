package manifest

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestIndexFileManifests_PathIDFallback(t *testing.T) {
	t.Parallel()
	files := []string{"/cfg/prompts/child/no_id.yaml"}
	read := func(fpath string) (*RawManifest, error) {
		assert.Equal(t, files[0], fpath)
		return &RawManifest{Messages: []RawMessage{{Role: "user"}}}, nil
	}
	loader, err := IndexFileManifests(files, read, IndexFileOptions{
		IDFromPath: func(_ string) string {
			return "child/no_id"
		},
	})
	require.NoError(t, err)
	raw, err := loader.LoadByID(context.Background(), "child/no_id")
	require.NoError(t, err)
	assert.Equal(t, "child/no_id", raw.ID)
}
