package remoteregistry

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/skosovsky/prompty"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/goleak"
)

func TestMain(m *testing.M) {
	goleak.VerifyTestMain(m)
}

type mockFetcher struct {
	mu     sync.Mutex
	data   map[string][]byte
	fetch  func(ctx context.Context, name, env string) ([]byte, error)
	called int
}

func (m *mockFetcher) Fetch(ctx context.Context, name, env string) ([]byte, error) {
	m.mu.Lock()
	m.called++
	m.mu.Unlock()
	if m.fetch != nil {
		data, err := m.fetch(ctx, name, env)
		if err != nil {
			return nil, fmt.Errorf("%w: %w", ErrFetchFailed, err)
		}
		return data, nil
	}
	key := name + ":" + env
	if d, ok := m.data[key]; ok {
		return d, nil
	}
	if d, ok := m.data[name+":"]; ok {
		return d, nil
	}
	return nil, fmt.Errorf("%w: not found", ErrFetchFailed)
}

func TestRegistry_GetTemplate_Success(t *testing.T) {
	t.Parallel()
	manifestYAML := `
id: support_agent
version: "1"
messages:
  - role: system
    content: "Hello {{ .user_name }}"
`
	m := &mockFetcher{data: map[string][]byte{"support_agent:": []byte(manifestYAML)}}
	reg := New(m, WithTTL(time.Minute))
	ctx := context.Background()
	tpl, err := reg.GetTemplate(ctx, "support_agent", "")
	require.NoError(t, err)
	require.NotNil(t, tpl)
	assert.Equal(t, "support_agent", tpl.Metadata.ID)
	assert.Empty(t, tpl.Metadata.Environment)
	assert.Equal(t, 1, m.called)
	// Second call hits cache
	tpl2, err := reg.GetTemplate(ctx, "support_agent", "")
	require.NoError(t, err)
	assert.Equal(t, "support_agent", tpl2.Metadata.ID)
	assert.Equal(t, 1, m.called)
}

func TestRegistry_GetTemplate_EnvSpecific(t *testing.T) {
	t.Parallel()
	prodYAML := `
id: p
version: "1"
messages:
  - role: system
    content: "Production"
`
	m := &mockFetcher{data: map[string][]byte{"p:production": []byte(prodYAML)}}
	reg := New(m, WithTTL(time.Minute))
	ctx := context.Background()
	tpl, err := reg.GetTemplate(ctx, "p", "production")
	require.NoError(t, err)
	require.NotNil(t, tpl)
	assert.Equal(t, "Production", tpl.Messages[0].Content)
	assert.Equal(t, "production", tpl.Metadata.Environment)
}

func TestRegistry_GetTemplate_SetsEnvironment(t *testing.T) {
	t.Parallel()
	manifestYAML := `
id: env_test
version: "1"
messages:
  - role: system
    content: "Env"
`
	m := &mockFetcher{data: map[string][]byte{
		"env_test:":        []byte(manifestYAML),
		"env_test:staging": []byte(manifestYAML),
	}}
	reg := New(m, WithTTL(time.Minute))
	ctx := context.Background()
	tplEmpty, err := reg.GetTemplate(ctx, "env_test", "")
	require.NoError(t, err)
	assert.Empty(t, tplEmpty.Metadata.Environment)
	tplStaging, err := reg.GetTemplate(ctx, "env_test", "staging")
	require.NoError(t, err)
	assert.Equal(t, "staging", tplStaging.Metadata.Environment)
}

func TestRegistry_GetTemplate_FetchError(t *testing.T) {
	t.Parallel()
	m := &mockFetcher{
		fetch: func(context.Context, string, string) ([]byte, error) {
			return nil, errors.New("network error")
		},
	}
	reg := New(m)
	ctx := context.Background()
	_, err := reg.GetTemplate(ctx, "x", "")
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrFetchFailed)
}

func TestRegistry_GetTemplate_NotFoundWrapsErrTemplateNotFound(t *testing.T) {
	t.Parallel()
	m := &mockFetcher{
		fetch: func(context.Context, string, string) ([]byte, error) {
			return nil, fmt.Errorf("%w: %q", ErrNotFound, "missing")
		},
	}
	reg := New(m)
	ctx := context.Background()
	_, err := reg.GetTemplate(ctx, "missing", "")
	require.Error(t, err)
	assert.ErrorIs(t, err, prompty.ErrTemplateNotFound)
}

func TestRegistry_GetTemplate_InvalidManifest(t *testing.T) {
	t.Parallel()
	m := &mockFetcher{data: map[string][]byte{"bad:": []byte("id: bad\nmessages: [unclosed")}}
	reg := New(m)
	ctx := context.Background()
	_, err := reg.GetTemplate(ctx, "bad", "")
	require.Error(t, err)
	assert.ErrorIs(t, err, prompty.ErrInvalidManifest)
}

func TestRegistry_GetTemplate_TTLExpiry(t *testing.T) {
	t.Parallel()
	manifestYAML := `
id: ttl_test
version: "1"
messages:
  - role: system
    content: "v1"
`
	called := 0
	m := &mockFetcher{
		fetch: func(context.Context, string, string) ([]byte, error) {
			called++
			return []byte(manifestYAML), nil
		},
	}
	reg := New(m, WithTTL(50*time.Millisecond))
	ctx := context.Background()

	tpl, err := reg.GetTemplate(ctx, "ttl_test", "")
	require.NoError(t, err)
	assert.Equal(t, "v1", tpl.Messages[0].Content)
	assert.Equal(t, 1, called)

	time.Sleep(60 * time.Millisecond)
	tpl2, err := reg.GetTemplate(ctx, "ttl_test", "")
	require.NoError(t, err)
	assert.Equal(t, "v1", tpl2.Messages[0].Content)
	assert.Equal(t, 2, called)
}

func TestRegistry_GetTemplate_InfiniteTTL(t *testing.T) {
	t.Parallel()
	manifestYAML := `
id: infinite
version: "1"
messages:
  - role: system
    content: "cached"
`
	called := 0
	m := &mockFetcher{
		fetch: func(context.Context, string, string) ([]byte, error) {
			called++
			return []byte(manifestYAML), nil
		},
	}
	reg := New(m, WithTTL(0))
	ctx := context.Background()

	tpl, err := reg.GetTemplate(ctx, "infinite", "")
	require.NoError(t, err)
	assert.Equal(t, "cached", tpl.Messages[0].Content)
	assert.Equal(t, 1, called)

	time.Sleep(20 * time.Millisecond)
	tpl2, err := reg.GetTemplate(ctx, "infinite", "")
	require.NoError(t, err)
	assert.Equal(t, "cached", tpl2.Messages[0].Content)
	assert.Equal(t, 1, called, "TTL<=0: cache never expires, fetcher not called again")
}

func TestRegistry_GetTemplate_NegativeTTLNeverExpires(t *testing.T) {
	t.Parallel()
	manifestYAML := `
id: neg_ttl
version: "1"
messages:
  - role: system
    content: "v1"
`
	called := 0
	m := &mockFetcher{
		fetch: func(context.Context, string, string) ([]byte, error) {
			called++
			return []byte(manifestYAML), nil
		},
	}
	reg := New(m, WithTTL(-time.Hour))
	ctx := context.Background()
	tpl, err := reg.GetTemplate(ctx, "neg_ttl", "")
	require.NoError(t, err)
	assert.Equal(t, "v1", tpl.Messages[0].Content)
	time.Sleep(30 * time.Millisecond)
	tpl2, err := reg.GetTemplate(ctx, "neg_ttl", "")
	require.NoError(t, err)
	assert.Equal(t, "v1", tpl2.Messages[0].Content)
	assert.Equal(t, 1, called, "TTL<0: cache never expires")
}

func TestRegistry_GetTemplate_CacheSafety(t *testing.T) {
	t.Parallel()
	manifestYAML := `
id: safe
version: "1"
messages:
  - role: system
    content: "Original"
tools:
  - name: only_tool
    description: "Only"
`
	m := &mockFetcher{data: map[string][]byte{"safe:": []byte(manifestYAML)}}
	reg := New(m)
	ctx := context.Background()
	tpl1, err := reg.GetTemplate(ctx, "safe", "")
	require.NoError(t, err)
	require.NotNil(t, tpl1)
	tpl1.Messages[0].Content = "Mutated"
	tpl1.Tools = append(tpl1.Tools, prompty.ToolDefinition{Name: "extra", Description: "Extra"})
	tpl2, err := reg.GetTemplate(ctx, "safe", "")
	require.NoError(t, err)
	require.NotNil(t, tpl2)
	assert.Equal(t, "Original", tpl2.Messages[0].Content)
	assert.Len(t, tpl2.Tools, 1)
	assert.Equal(t, "only_tool", tpl2.Tools[0].Name)
}

func TestRegistry_GetTemplate_ContextCancellation(t *testing.T) {
	t.Parallel()
	m := &mockFetcher{
		fetch: func(ctx context.Context, _, _ string) ([]byte, error) {
			<-ctx.Done()
			return nil, ctx.Err()
		},
	}
	reg := New(m)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := reg.GetTemplate(ctx, "x", "")
	require.Error(t, err)
	assert.ErrorIs(t, err, context.Canceled)
}

func TestRegistry_GetTemplate_Concurrent(t *testing.T) {
	t.Parallel()
	manifestYAML := `
id: conc
version: "1"
messages:
  - role: system
    content: "x"
`
	m := &mockFetcher{data: map[string][]byte{"conc:": []byte(manifestYAML)}}
	reg := New(m)
	ctx := context.Background()
	type result struct {
		tpl *prompty.ChatPromptTemplate
		err error
	}
	results := make(chan result, 50)
	for range 50 {
		go func() {
			tpl, err := reg.GetTemplate(ctx, "conc", "")
			results <- result{tpl: tpl, err: err}
		}()
	}
	for range 50 {
		r := <-results
		require.NoError(t, r.err)
		require.NotNil(t, r.tpl)
		assert.Equal(t, "conc", r.tpl.Metadata.ID)
	}
}

func TestRegistry_GetTemplate_InvalidName(t *testing.T) {
	t.Parallel()
	m := &mockFetcher{data: map[string][]byte{}}
	reg := New(m)
	ctx := context.Background()
	_, err := reg.GetTemplate(ctx, "invalid:name", "")
	require.Error(t, err)
	require.ErrorIs(t, err, prompty.ErrInvalidName)
	assert.Contains(t, err.Error(), ":")
}

func TestRegistry_Close(t *testing.T) {
	t.Parallel()
	m := &mockFetcher{data: map[string][]byte{}}
	reg := New(m)
	err := reg.Close()
	require.NoError(t, err)
}

func TestRegistry_New_NilFetcherPanics(t *testing.T) {
	t.Parallel()
	require.Panics(t, func() { New(nil) })
}

func TestRegistry_Evict(t *testing.T) {
	t.Parallel()
	manifestYAML := `
id: evict_me
version: "1"
messages:
  - role: system
    content: "x"
`
	m := &mockFetcher{data: map[string][]byte{"evict_me:": []byte(manifestYAML)}}
	reg := New(m, WithTTL(time.Minute))
	ctx := context.Background()
	_, err := reg.GetTemplate(ctx, "evict_me", "")
	require.NoError(t, err)
	assert.Equal(t, 1, m.called)
	reg.Evict("evict_me", "")
	_, err = reg.GetTemplate(ctx, "evict_me", "")
	require.NoError(t, err)
	assert.Equal(t, 2, m.called, "after Evict, next GetTemplate should fetch again")
}

func TestRegistry_EvictAll(t *testing.T) {
	t.Parallel()
	manifestYAML := `
id: all
version: "1"
messages:
  - role: system
    content: "x"
`
	m := &mockFetcher{data: map[string][]byte{"all:": []byte(manifestYAML)}}
	reg := New(m, WithTTL(time.Minute))
	ctx := context.Background()
	_, err := reg.GetTemplate(ctx, "all", "")
	require.NoError(t, err)
	reg.EvictAll()
	_, err = reg.GetTemplate(ctx, "all", "")
	require.NoError(t, err)
	assert.Equal(t, 2, m.called)
}
