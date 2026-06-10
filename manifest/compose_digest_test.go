package manifest

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestComposeClosureDigest_CyclicImportsFails(t *testing.T) {
	t.Parallel()
	main := []byte(`{"id":"a","imports":[{"id":"b"}]}`)
	b := []byte(`{"id":"b","imports":[{"id":"a"}]}`)
	read := func(id string) ([]byte, error) {
		switch id {
		case "a":
			return main, nil
		case "b":
			return b, nil
		default:
			return nil, assert.AnError
		}
	}
	_, err := ComposeClosureDigestSHA256("a", read, NewJSONParser())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "cyclic import")
}

func TestComposeClosureDigest_EmptyImportIDFails(t *testing.T) {
	t.Parallel()
	main := []byte(`{"id":"main","imports":[{"id":""}]}`)
	read := func(id string) ([]byte, error) {
		if id == "main" {
			return main, nil
		}
		return nil, assert.AnError
	}
	_, err := ComposeClosureDigestSHA256("main", read, NewJSONParser())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "import id is required")
}
