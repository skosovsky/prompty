//go:build integration

package remoteregistry

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/skosovsky/prompty"
	"github.com/skosovsky/prompty/manifest"
)

func TestIntegrationHTTPFetcher_IntegrationWithRegistry(t *testing.T) {
	t.Parallel()
	manifestJSON := `{"id":"integ","version":"1","messages":[{"role":"system","content":[{"type":"text","text":"Integrated"}]}]}`
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(manifestJSON))
	}))
	defer srv.Close()

	h, err := NewHTTPFetcher(srv.URL)
	require.NoError(t, err)
	reg, err := New(h, WithParser(manifest.NewJSONParser()))
	require.NoError(t, err)
	ctx := context.Background()
	tpl, err := templateFromPlan(ctx, reg, "integ")
	require.NoError(t, err)
	require.NotNil(t, tpl)
	assert.Equal(t, "integ", tpl.Metadata.ID)
	assert.Equal(t, prompty.RoleSystem, tpl.Messages[0].Role)
}
