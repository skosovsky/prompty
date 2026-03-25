package remoteregistry

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/skosovsky/prompty"
	"github.com/skosovsky/prompty/manifest"

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
	fetch  func(ctx context.Context, id string) ([]byte, error)
	called int
}

func (m *mockFetcher) Fetch(ctx context.Context, id string) ([]byte, error) {
	m.mu.Lock()
	m.called++
	m.mu.Unlock()
	if m.fetch != nil {
		data, err := m.fetch(ctx, id)
		if err != nil {
			return nil, fmt.Errorf("%w: %w", ErrFetchFailed, err)
		}
		return data, nil
	}
	if d, ok := m.data[id]; ok {
		return d, nil
	}
	return nil, fmt.Errorf("%w: %q", ErrNotFound, id)
}

func TestRegistry_GetTemplate_Success(t *testing.T) {
	t.Parallel()
	manifestJSON := `{"id":"support_agent","version":"1","messages":[{"role":"system","content":[{"type":"text","text":"Hello {{ .user_name }}"}]}]}`
	m := &mockFetcher{data: map[string][]byte{"support_agent": []byte(manifestJSON)}}
	reg, err := New(m, WithParser(manifest.NewJSONParser()), WithTTL(time.Minute))
	require.NoError(t, err)
	ctx := context.Background()
	tpl, err := reg.GetTemplate(ctx, "support_agent")
	require.NoError(t, err)
	require.NotNil(t, tpl)
	assert.Equal(t, "support_agent", tpl.Metadata.ID)
	assert.Equal(t, 1, m.called)
	tpl2, err := reg.GetTemplate(ctx, "support_agent")
	require.NoError(t, err)
	assert.Equal(t, "support_agent", tpl2.Metadata.ID)
	assert.Equal(t, 1, m.called)
}

func TestRegistry_GetTemplate_EnvSpecific(t *testing.T) {
	t.Parallel()
	prodJSON := `{"id":"p","version":"1","messages":[{"role":"system","content":[{"type":"text","text":"Production"}]}]}`
	m := &mockFetcher{data: map[string][]byte{"p.production": []byte(prodJSON)}}
	reg, err := New(m, WithParser(manifest.NewJSONParser()), WithTTL(time.Minute), WithEnvironment("production"))
	require.NoError(t, err)
	ctx := context.Background()
	tpl, err := reg.GetTemplate(ctx, "p")
	require.NoError(t, err)
	require.NotNil(t, tpl)
	require.Len(t, tpl.Messages[0].Content, 1)
	assert.Equal(t, "Production", tpl.Messages[0].Content[0].Text, "env variant p.production should be preferred")
}

func TestRegistry_GetTemplate_EnvFallbackBaseAndStaging(t *testing.T) {
	t.Parallel()
	baseJSON := `{"id":"env_test","version":"1","messages":[{"role":"system","content":[{"type":"text","text":"Base"}]}]}`
	stagingJSON := `{"id":"env_test","version":"1","messages":[{"role":"system","content":[{"type":"text","text":"Staging"}]}]}`
	m := &mockFetcher{data: map[string][]byte{
		"env_test":         []byte(baseJSON),
		"env_test.staging": []byte(stagingJSON),
	}}
	reg, err := New(m, WithParser(manifest.NewJSONParser()), WithTTL(time.Minute), WithEnvironment("staging"))
	require.NoError(t, err)
	ctx := context.Background()
	tpl, err := reg.GetTemplate(ctx, "env_test")
	require.NoError(t, err)
	require.NotNil(t, tpl)
	require.Len(t, tpl.Messages[0].Content, 1)
	assert.Equal(
		t,
		"Staging",
		tpl.Messages[0].Content[0].Text,
		"env variant env_test.staging should be preferred over base",
	)
}

func TestRegistry_GetTemplate_EnvFallbackToBase(t *testing.T) {
	t.Parallel()
	baseJSON := `{"id":"p","version":"1","messages":[{"role":"system","content":[{"type":"text","text":"BaseOnly"}]}]}`
	m := &mockFetcher{data: map[string][]byte{"p": []byte(baseJSON)}}
	reg, err := New(m, WithParser(manifest.NewJSONParser()), WithTTL(time.Minute), WithEnvironment("prod"))
	require.NoError(t, err)
	ctx := context.Background()
	tpl, err := reg.GetTemplate(ctx, "p")
	require.NoError(t, err)
	require.NotNil(t, tpl)
	require.Len(t, tpl.Messages[0].Content, 1)
	assert.Equal(t, "BaseOnly", tpl.Messages[0].Content[0].Text, "should fallback to base when env variant missing")
}

func TestRegistry_GetTemplate_FetchError(t *testing.T) {
	t.Parallel()
	m := &mockFetcher{
		fetch: func(context.Context, string) ([]byte, error) {
			return nil, errors.New("network error")
		},
	}
	reg, err := New(m, WithParser(manifest.NewJSONParser()))
	require.NoError(t, err)
	ctx := context.Background()
	_, err = reg.GetTemplate(ctx, "x")
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrFetchFailed)
}

func TestRegistry_GetTemplate_NotFoundWrapsErrTemplateNotFound(t *testing.T) {
	t.Parallel()
	m := &mockFetcher{
		fetch: func(context.Context, string) ([]byte, error) {
			return nil, fmt.Errorf("%w: %q", ErrNotFound, "missing")
		},
	}
	reg, err := New(m, WithParser(manifest.NewJSONParser()))
	require.NoError(t, err)
	ctx := context.Background()
	_, err = reg.GetTemplate(ctx, "missing")
	require.Error(t, err)
	assert.ErrorIs(t, err, prompty.ErrTemplateNotFound)
}

func TestRegistry_GetTemplate_InvalidManifest(t *testing.T) {
	t.Parallel()
	m := &mockFetcher{data: map[string][]byte{"bad": []byte("id: bad\nmessages: [unclosed")}}
	reg, err := New(m, WithParser(manifest.NewJSONParser()))
	require.NoError(t, err)
	ctx := context.Background()
	_, err = reg.GetTemplate(ctx, "bad")
	require.Error(t, err)
	assert.ErrorIs(t, err, prompty.ErrInvalidManifest)
}

func TestRegistry_GetTemplate_TTLExpiry(t *testing.T) {
	t.Parallel()
	manifestJSON := `{"id":"ttl_test","version":"1","messages":[{"role":"system","content":[{"type":"text","text":"v1"}]}]}`
	called := 0
	m := &mockFetcher{
		fetch: func(context.Context, string) ([]byte, error) {
			called++
			return []byte(manifestJSON), nil
		},
	}
	reg, err := New(m, WithParser(manifest.NewJSONParser()), WithTTL(50*time.Millisecond))
	require.NoError(t, err)
	ctx := context.Background()
	tpl, err := reg.GetTemplate(ctx, "ttl_test")
	require.NoError(t, err)
	require.Len(t, tpl.Messages[0].Content, 1)
	assert.Equal(t, "v1", tpl.Messages[0].Content[0].Text)
	assert.Equal(t, 1, called)

	time.Sleep(60 * time.Millisecond)
	tpl2, err := reg.GetTemplate(ctx, "ttl_test")
	require.NoError(t, err)
	require.Len(t, tpl2.Messages[0].Content, 1)
	assert.Equal(t, "v1", tpl2.Messages[0].Content[0].Text)
	assert.Equal(t, 2, called)
}

func TestRegistry_GetTemplate_InfiniteTTL(t *testing.T) {
	t.Parallel()
	manifestJSON := `{"id":"infinite","version":"1","messages":[{"role":"system","content":[{"type":"text","text":"cached"}]}]}`
	called := 0
	m := &mockFetcher{
		fetch: func(context.Context, string) ([]byte, error) {
			called++
			return []byte(manifestJSON), nil
		},
	}
	reg, err := New(m, WithParser(manifest.NewJSONParser()), WithTTL(0))
	require.NoError(t, err)
	ctx := context.Background()
	tpl, err := reg.GetTemplate(ctx, "infinite")
	require.NoError(t, err)
	require.Len(t, tpl.Messages[0].Content, 1)
	assert.Equal(t, "cached", tpl.Messages[0].Content[0].Text)
	assert.Equal(t, 1, called)

	time.Sleep(20 * time.Millisecond)
	tpl2, err := reg.GetTemplate(ctx, "infinite")
	require.NoError(t, err)
	require.Len(t, tpl2.Messages[0].Content, 1)
	assert.Equal(t, "cached", tpl2.Messages[0].Content[0].Text)
	assert.Equal(t, 1, called, "TTL<=0: cache never expires, fetcher not called again")
}

func TestRegistry_GetTemplate_NegativeTTLNeverExpires(t *testing.T) {
	t.Parallel()
	manifestJSON := `{"id":"neg_ttl","version":"1","messages":[{"role":"system","content":[{"type":"text","text":"v1"}]}]}`
	called := 0
	m := &mockFetcher{
		fetch: func(context.Context, string) ([]byte, error) {
			called++
			return []byte(manifestJSON), nil
		},
	}
	reg, err := New(m, WithParser(manifest.NewJSONParser()), WithTTL(-time.Hour))
	require.NoError(t, err)
	ctx := context.Background()
	tpl, err := reg.GetTemplate(ctx, "neg_ttl")
	require.NoError(t, err)
	require.Len(t, tpl.Messages[0].Content, 1)
	assert.Equal(t, "v1", tpl.Messages[0].Content[0].Text)
	time.Sleep(30 * time.Millisecond)
	tpl2, err := reg.GetTemplate(ctx, "neg_ttl")
	require.NoError(t, err)
	require.Len(t, tpl2.Messages[0].Content, 1)
	assert.Equal(t, "v1", tpl2.Messages[0].Content[0].Text)
	assert.Equal(t, 1, called, "TTL<0: cache never expires")
}

func TestRegistry_GetTemplate_CacheSafety(t *testing.T) {
	t.Parallel()
	manifestJSON := `{"id":"safe","version":"1","messages":[{"role":"system","content":[{"type":"text","text":"Original"}]}],"tools":[{"name":"only_tool","description":"Only","parameters":{}}]}`
	m := &mockFetcher{data: map[string][]byte{"safe": []byte(manifestJSON)}}
	reg, err := New(m, WithParser(manifest.NewJSONParser()))
	require.NoError(t, err)
	ctx := context.Background()
	tpl1, err := reg.GetTemplate(ctx, "safe")
	require.NoError(t, err)
	require.NotNil(t, tpl1)
	tpl1.Messages[0].Content = []prompty.TemplatePart{{Type: "text", Text: "Mutated"}}
	tpl1.Tools = append(tpl1.Tools, prompty.ToolDefinition{Name: "extra", Description: "Extra"})
	tpl2, err := reg.GetTemplate(ctx, "safe")
	require.NoError(t, err)
	require.NotNil(t, tpl2)
	require.Len(t, tpl2.Messages[0].Content, 1)
	assert.Equal(t, "Original", tpl2.Messages[0].Content[0].Text)
	assert.Len(t, tpl2.Tools, 1)
	assert.Equal(t, "only_tool", tpl2.Tools[0].Name)
}

func TestRegistry_GetTemplate_ContextCancellation(t *testing.T) {
	t.Parallel()
	m := &mockFetcher{
		fetch: func(ctx context.Context, _ string) ([]byte, error) {
			<-ctx.Done()
			return nil, ctx.Err()
		},
	}
	reg, err := New(m, WithParser(manifest.NewJSONParser()))
	require.NoError(t, err)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err = reg.GetTemplate(ctx, "x")
	require.Error(t, err)
	assert.ErrorIs(t, err, context.Canceled)
}

func TestRegistry_GetTemplate_Concurrent(t *testing.T) {
	t.Parallel()
	manifestJSON := `{"id":"conc","version":"1","messages":[{"role":"system","content":[{"type":"text","text":"x"}]}]}`
	m := &mockFetcher{data: map[string][]byte{"conc": []byte(manifestJSON)}}
	reg, err := New(m, WithParser(manifest.NewJSONParser()))
	require.NoError(t, err)
	ctx := context.Background()
	type result struct {
		tpl *prompty.ChatPromptTemplate
		err error
	}
	results := make(chan result, 50)
	for range 50 {
		go func() {
			tpl, err := reg.GetTemplate(ctx, "conc")
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

func TestRegistry_GetTemplate_InvalidID(t *testing.T) {
	t.Parallel()
	m := &mockFetcher{data: map[string][]byte{}}
	reg, err := New(m, WithParser(manifest.NewJSONParser()))
	require.NoError(t, err)
	ctx := context.Background()
	_, err = reg.GetTemplate(ctx, "invalid:name")
	require.Error(t, err)
	require.ErrorIs(t, err, prompty.ErrInvalidName)
	assert.Contains(t, err.Error(), ":")
}

func TestRegistry_Close(t *testing.T) {
	t.Parallel()
	m := &mockFetcher{data: map[string][]byte{}}
	reg, err := New(m, WithParser(manifest.NewJSONParser()))
	require.NoError(t, err)
	err = reg.Close()
	require.NoError(t, err)
}

func TestRegistry_New_NilFetcherPanics(t *testing.T) {
	t.Parallel()
	require.Panics(t, func() { _, _ = New(nil) })
}

func TestRegistry_Evict(t *testing.T) {
	t.Parallel()
	manifestJSON := `{"id":"evict_me","version":"1","messages":[{"role":"system","content":[{"type":"text","text":"x"}]}]}`
	m := &mockFetcher{data: map[string][]byte{"evict_me": []byte(manifestJSON)}}
	reg, err := New(m, WithParser(manifest.NewJSONParser()), WithTTL(time.Minute))
	require.NoError(t, err)
	ctx := context.Background()
	_, err = reg.GetTemplate(ctx, "evict_me")
	require.NoError(t, err)
	assert.Equal(t, 1, m.called)
	reg.Evict("evict_me")
	_, err = reg.GetTemplate(ctx, "evict_me")
	require.NoError(t, err)
	assert.Equal(t, 2, m.called, "after Evict, next GetTemplate should fetch again")
}

func TestRegistry_EvictAll(t *testing.T) {
	t.Parallel()
	manifestJSON := `{"id":"all","version":"1","messages":[{"role":"system","content":[{"type":"text","text":"x"}]}]}`
	m := &mockFetcher{data: map[string][]byte{"all": []byte(manifestJSON)}}
	reg, err := New(m, WithParser(manifest.NewJSONParser()), WithTTL(time.Minute))
	require.NoError(t, err)
	ctx := context.Background()
	_, err = reg.GetTemplate(ctx, "all")
	require.NoError(t, err)
	reg.EvictAll()
	_, err = reg.GetTemplate(ctx, "all")
	require.NoError(t, err)
	assert.Equal(t, 2, m.called)
}

func TestRegistry_List_ReturnsNilWhenNoLister(t *testing.T) {
	t.Parallel()
	m := &mockFetcher{data: map[string][]byte{}}
	reg, err := New(m, WithParser(manifest.NewJSONParser()))
	require.NoError(t, err)
	ctx := context.Background()
	ids, err := reg.List(ctx)
	require.NoError(t, err)
	assert.Nil(t, ids)
}

func TestRegistry_Stat_ReturnsErrWhenNoStatter(t *testing.T) {
	t.Parallel()
	m := &mockFetcher{data: map[string][]byte{}}
	reg, err := New(m, WithParser(manifest.NewJSONParser()))
	require.NoError(t, err)
	ctx := context.Background()
	_, err = reg.Stat(ctx, "any")
	require.Error(t, err)
	assert.ErrorIs(t, err, prompty.ErrTemplateNotFound)
}
