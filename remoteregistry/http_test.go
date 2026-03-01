package remoteregistry

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/skosovsky/prompty"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestHTTPFetcher_Fetch_Success(t *testing.T) {
	t.Parallel()
	manifestYAML := `
id: support_agent
version: "1"
messages:
  - role: system
    content: "Hello {{ .user_name }}"
`
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/support_agent.yaml", r.URL.Path)
		w.Header().Set("Content-Type", "application/yaml")
		_, _ = w.Write([]byte(manifestYAML))
	}))
	defer srv.Close()

	h, err := NewHTTPFetcher(srv.URL)
	require.NoError(t, err)
	reg := New(h)
	ctx := context.Background()
	tpl, err := reg.GetTemplate(ctx, "support_agent")
	require.NoError(t, err)
	require.NotNil(t, tpl)
	assert.Equal(t, "support_agent", tpl.Metadata.ID)
}

func TestHTTPFetcher_Fetch_EnvSpecific(t *testing.T) {
	t.Parallel()
	prodYAML := `
id: p
version: "1"
messages:
  - role: system
    content: "Production"
`
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/p.production.yaml", r.URL.Path)
		_, _ = w.Write([]byte(prodYAML))
	}))
	defer srv.Close()

	h, err := NewHTTPFetcher(srv.URL)
	require.NoError(t, err)
	data, err := h.Fetch(context.Background(), "p.production")
	require.NoError(t, err)
	assert.Contains(t, string(data), "Production")
}

func TestHTTPFetcher_Fetch_IdResolution(t *testing.T) {
	t.Parallel()
	baseYAML := `
id: fallback
version: "1"
messages:
  - role: system
    content: "Base"
`
	var mu sync.Mutex
	var paths []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		paths = append(paths, r.URL.Path)
		mu.Unlock()
		_, _ = w.Write([]byte(baseYAML))
	}))
	defer srv.Close()

	h, err := NewHTTPFetcher(srv.URL)
	require.NoError(t, err)
	data, err := h.Fetch(context.Background(), "fallback")
	require.NoError(t, err)
	assert.Contains(t, string(data), "Base")
	mu.Lock()
	pathsCopy := append([]string(nil), paths...)
	mu.Unlock()
	assert.Equal(t, []string{"/fallback.yaml"}, pathsCopy)
}

func TestHTTPFetcher_Fetch_BearerAuth(t *testing.T) {
	t.Parallel()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		auth := r.Header.Get("Authorization")
		if auth != "Bearer secret-token" {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		_, _ = w.Write([]byte(`
id: auth
version: "1"
messages:
  - role: system
    content: "OK"
`))
	}))
	defer srv.Close()

	h, err := NewHTTPFetcher(srv.URL, WithAuthToken("secret-token"))
	require.NoError(t, err)
	data, err := h.Fetch(context.Background(), "auth")
	require.NoError(t, err)
	assert.Contains(t, string(data), "OK")
}

func TestHTTPFetcher_Fetch_NotFound(t *testing.T) {
	t.Parallel()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.NotFound(w, r)
	}))
	defer srv.Close()

	h, err := NewHTTPFetcher(srv.URL)
	require.NoError(t, err)
	_, err = h.Fetch(context.Background(), "nonexistent")
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrNotFound)
}

func TestHTTPFetcher_Fetch_HTTPError(t *testing.T) {
	t.Parallel()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "server error", http.StatusInternalServerError)
	}))
	defer srv.Close()

	h, err := NewHTTPFetcher(srv.URL)
	require.NoError(t, err)
	_, err = h.Fetch(context.Background(), "x")
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrHTTPStatus)
}

func TestHTTPFetcher_NewInvalidURL(t *testing.T) {
	t.Parallel()
	_, err := NewHTTPFetcher("")
	require.Error(t, err)
	_, err = NewHTTPFetcher("://invalid")
	require.Error(t, err)
	_, err = NewHTTPFetcher("no-scheme")
	require.Error(t, err)
}

func TestHTTPFetcher_Fetch_BodyTooLarge(t *testing.T) {
	t.Parallel()
	// Server sends more than maxBodySize; fetcher detects truncation and returns ErrFetchFailed.
	bigBody := make([]byte, maxBodySize+1)
	for i := range bigBody {
		bigBody[i] = 'x'
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write(bigBody)
	}))
	defer srv.Close()

	h, err := NewHTTPFetcher(srv.URL)
	require.NoError(t, err)
	_, err = h.Fetch(context.Background(), "large")
	require.Error(t, err)
	require.ErrorIs(t, err, ErrFetchFailed)
	assert.Contains(t, err.Error(), "exceeds")

	reg := New(h)
	_, err = reg.GetTemplate(context.Background(), "large")
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrFetchFailed)
}

func TestHTTPFetcher_IntegrationWithRegistry(t *testing.T) {
	t.Parallel()
	manifestYAML := `
id: integ
version: "1"
messages:
  - role: system
    content: "Integrated"
`
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(manifestYAML))
	}))
	defer srv.Close()

	h, err := NewHTTPFetcher(srv.URL)
	require.NoError(t, err)
	reg := New(h)
	ctx := context.Background()
	tpl, err := reg.GetTemplate(ctx, "integ")
	require.NoError(t, err)
	require.NotNil(t, tpl)
	assert.Equal(t, "integ", tpl.Metadata.ID)
	assert.Equal(t, prompty.RoleSystem, tpl.Messages[0].Role)
}

func TestHTTPFetcher_WithHTTPClient(t *testing.T) {
	t.Parallel()
	manifestYAML := `
id: client_test
version: "1"
messages:
  - role: system
    content: "OK"
`
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(manifestYAML))
	}))
	defer srv.Close()

	customClient := &http.Client{Timeout: 5 * time.Second}
	h, err := NewHTTPFetcher(srv.URL, WithHTTPClient(customClient))
	require.NoError(t, err)
	data, err := h.Fetch(context.Background(), "client_test")
	require.NoError(t, err)
	assert.Contains(t, string(data), "client_test")

	// WithHTTPClient(nil) must not overwrite default client; Fetch must not panic.
	h2, err := NewHTTPFetcher(srv.URL, WithHTTPClient(nil))
	require.NoError(t, err)
	data2, err := h2.Fetch(context.Background(), "client_test")
	require.NoError(t, err)
	assert.Contains(t, string(data2), "client_test")
}

func TestHTTPFetcher_Fetch_UserAgentSet(t *testing.T) {
	t.Parallel()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ua := r.Header.Get("User-Agent")
		assert.Contains(t, ua, "prompty-remote-registry")
		_, _ = w.Write([]byte(`
id: ua
version: "1"
messages:
  - role: system
    content: "OK"
`))
	}))
	defer srv.Close()
	h, err := NewHTTPFetcher(srv.URL)
	require.NoError(t, err)
	_, err = h.Fetch(context.Background(), "ua")
	require.NoError(t, err)
}

func TestHTTPFetcher_Fetch_ContextCancellation(t *testing.T) {
	t.Parallel()
	block := make(chan struct{})
	srv := httptest.NewServer(http.HandlerFunc(func(_ http.ResponseWriter, _ *http.Request) {
		<-block
	}))
	defer srv.Close()
	defer close(block)
	h, err := NewHTTPFetcher(srv.URL)
	require.NoError(t, err)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err = h.Fetch(ctx, "x")
	require.Error(t, err)
	assert.ErrorIs(t, err, context.Canceled)
}

func TestHTTPFetcher_Fetch_DotInName(t *testing.T) {
	t.Parallel()
	manifestYAML := `
id: support.agent
version: "1"
messages:
  - role: system
    content: "Dot name"
`
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/support.agent.yaml", r.URL.Path)
		_, _ = w.Write([]byte(manifestYAML))
	}))
	defer srv.Close()
	h, err := NewHTTPFetcher(srv.URL)
	require.NoError(t, err)
	data, err := h.Fetch(context.Background(), "support.agent")
	require.NoError(t, err)
	assert.Contains(t, string(data), "support.agent")
}
