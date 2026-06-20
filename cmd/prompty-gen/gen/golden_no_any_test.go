package gen

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

// Generated Go must not contain the any type.
func TestGoldenGeneratedOutput_NoAnyType(t *testing.T) {
	t.Parallel()
	anyWord := regexp.MustCompile(`\bany\b`)
	root := filepath.Join("..", "testdata")
	err := filepath.WalkDir(root, func(path string, d os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if d.IsDir() {
			return nil
		}
		if !strings.HasSuffix(path, "_gen.go") && !strings.HasSuffix(path, ".golden") {
			return nil
		}
		data, readErr := os.ReadFile(path)
		if readErr != nil {
			return readErr
		}
		if anyWord.Match(data) {
			t.Errorf("generated artifact must not contain any type: %s", path)
		}
		return nil
	})
	require.NoError(t, err)
}
