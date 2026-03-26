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

type mockRegistryWithExtras struct {
	tpl         *prompty.ChatPromptTemplate
	ids         []string
	info        prompty.TemplateInfo
	closeCalled bool
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

func (m *mockRegistryWithExtras) GetTemplate(_ context.Context, _ string) (*prompty.ChatPromptTemplate, error) {
	return prompty.CloneTemplate(m.tpl), nil
}

func (m *mockRegistryWithExtras) List(_ context.Context) ([]string, error) {
	return append([]string(nil), m.ids...), nil
}

func (m *mockRegistryWithExtras) Stat(_ context.Context, _ string) (prompty.TemplateInfo, error) {
	return m.info, nil
}

func (m *mockRegistryWithExtras) Close() error {
	m.closeCalled = true
	return nil
}

func TestRegistry_GetTemplate_Success(t *testing.T) {
	t.Parallel()
	manifestJSON := `{"id":"support_agent","version":"1","messages":[{"role":"system","content":[{"type":"text","text":"Hello {{ .user_name }}"}]}]}`
	m := &mockFetcher{data: map[string][]byte{"support_agent": []byte(manifestJSON)}}
	base, err := New(m, WithParser(manifest.NewJSONParser()))
	require.NoError(t, err)
	reg := WithCache(base, time.Minute)
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
	base, err := New(m, WithParser(manifest.NewJSONParser()), WithEnvironment("production"))
	require.NoError(t, err)
	reg := WithCache(base, time.Minute)
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
	base, err := New(m, WithParser(manifest.NewJSONParser()), WithEnvironment("staging"))
	require.NoError(t, err)
	reg := WithCache(base, time.Minute)
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
	base, err := New(m, WithParser(manifest.NewJSONParser()), WithEnvironment("prod"))
	require.NoError(t, err)
	reg := WithCache(base, time.Minute)
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
	base, err := New(m, WithParser(manifest.NewJSONParser()))
	require.NoError(t, err)
	reg := WithCache(base, 50*time.Millisecond)
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
	base, err := New(m, WithParser(manifest.NewJSONParser()))
	require.NoError(t, err)
	reg := WithCache(base, 0)
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
	base, err := New(m, WithParser(manifest.NewJSONParser()))
	require.NoError(t, err)
	reg := WithCache(base, -time.Hour)
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

func TestCachedRegistry_GetTemplate_ConcurrentDedupe(t *testing.T) {
	t.Parallel()
	manifestJSON := `{"id":"conc_cached","version":"1","messages":[{"role":"system","content":[{"type":"text","text":"x"}]}]}`
	m := &mockFetcher{
		fetch: func(context.Context, string) ([]byte, error) {
			time.Sleep(20 * time.Millisecond)
			return []byte(manifestJSON), nil
		},
	}
	base, err := New(m, WithParser(manifest.NewJSONParser()))
	require.NoError(t, err)
	reg := WithCache(base, time.Minute)
	ctx := context.Background()
	const workers = 40
	errs := make(chan error, workers)
	for range workers {
		go func() {
			tpl, getErr := reg.GetTemplate(ctx, "conc_cached")
			if getErr == nil && (tpl == nil || tpl.Metadata.ID != "conc_cached") {
				getErr = errors.New("unexpected template returned from cache")
			}
			errs <- getErr
		}()
	}
	for range workers {
		require.NoError(t, <-errs)
	}
	assert.Equal(t, 1, m.called, "dedupe: only one underlying fetch expected")
}

func TestCachedRegistry_GetTemplate_CallerCancellationIsolation(t *testing.T) {
	t.Parallel()
	manifestJSON := `{"id":"ctx_isolation","version":"1","messages":[{"role":"system","content":[{"type":"text","text":"x"}]}]}`
	started := make(chan struct{})
	m := &mockFetcher{
		fetch: func(ctx context.Context, _ string) ([]byte, error) {
			select {
			case <-started:
			default:
				close(started)
			}
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			case <-time.After(40 * time.Millisecond):
				return []byte(manifestJSON), nil
			}
		},
	}
	base, err := New(m, WithParser(manifest.NewJSONParser()))
	require.NoError(t, err)
	reg := WithCache(base, time.Minute)

	firstCtx, cancel := context.WithTimeout(context.Background(), 10*time.Millisecond)
	defer cancel()
	firstErr := make(chan error, 1)
	go func() {
		_, getErr := reg.GetTemplate(firstCtx, "ctx_isolation")
		firstErr <- getErr
	}()

	<-started
	tpl, err := reg.GetTemplate(context.Background(), "ctx_isolation")
	require.NoError(t, err)
	require.NotNil(t, tpl)
	assert.Equal(t, "ctx_isolation", tpl.Metadata.ID)

	err = <-firstErr
	require.Error(t, err)
	require.ErrorIs(t, err, context.DeadlineExceeded)
	assert.Equal(t, 1, m.called, "dedupe should still use one shared fetch")
}

func TestCachedRegistry_GetTemplate_CancelsSharedFetchWhenAllWaitersCancel(t *testing.T) {
	t.Parallel()
	started := make(chan struct{})
	canceled := make(chan struct{})
	m := &mockFetcher{
		fetch: func(ctx context.Context, _ string) ([]byte, error) {
			select {
			case <-started:
			default:
				close(started)
			}
			<-ctx.Done()
			select {
			case <-canceled:
			default:
				close(canceled)
			}
			return nil, ctx.Err()
		},
	}
	base, err := New(m, WithParser(manifest.NewJSONParser()))
	require.NoError(t, err)
	reg := WithCache(base, time.Minute)

	ctx1, cancel1 := context.WithTimeout(context.Background(), 15*time.Millisecond)
	defer cancel1()
	ctx2, cancel2 := context.WithTimeout(context.Background(), 30*time.Millisecond)
	defer cancel2()

	errs := make(chan error, 2)
	go func() {
		_, getErr := reg.GetTemplate(ctx1, "ctx_cancel_all")
		errs <- getErr
	}()
	go func() {
		_, getErr := reg.GetTemplate(ctx2, "ctx_cancel_all")
		errs <- getErr
	}()

	<-started

	err1 := <-errs
	err2 := <-errs
	require.Error(t, err1)
	require.Error(t, err2)
	require.ErrorIs(t, err1, context.DeadlineExceeded)
	require.ErrorIs(t, err2, context.DeadlineExceeded)

	select {
	case <-canceled:
	case <-time.After(250 * time.Millisecond):
		t.Fatal("shared fetch was not canceled after last waiter left")
	}
	assert.Equal(t, 1, m.called, "first wave should dedupe to one shared fetch")

	ctx3, cancel3 := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel3()
	_, err = reg.GetTemplate(ctx3, "ctx_cancel_all")
	require.Error(t, err)
	require.ErrorIs(t, err, context.DeadlineExceeded)
	assert.Equal(t, 2, m.called, "failed/canceled shared fetch must not populate cache")
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
	base, err := New(m, WithParser(manifest.NewJSONParser()))
	require.NoError(t, err)
	reg := WithCache(base, time.Minute)
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
	base, err := New(m, WithParser(manifest.NewJSONParser()))
	require.NoError(t, err)
	reg := WithCache(base, time.Minute)
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

func TestCachedRegistry_List_DelegatesToBase(t *testing.T) {
	t.Parallel()
	base := &mockRegistryWithExtras{
		tpl: &prompty.ChatPromptTemplate{},
		ids: []string{"a", "b"},
	}
	reg := WithCache(base, time.Minute)
	ids, err := reg.List(context.Background())
	require.NoError(t, err)
	assert.Equal(t, []string{"a", "b"}, ids)
}

func TestCachedRegistry_Stat_DelegatesToBase(t *testing.T) {
	t.Parallel()
	now := time.Now()
	base := &mockRegistryWithExtras{
		tpl: &prompty.ChatPromptTemplate{},
		info: prompty.TemplateInfo{
			ID:        "a",
			Version:   "1",
			UpdatedAt: now,
		},
	}
	reg := WithCache(base, time.Minute)
	info, err := reg.Stat(context.Background(), "a")
	require.NoError(t, err)
	assert.Equal(t, "a", info.ID)
	assert.Equal(t, "1", info.Version)
	assert.True(t, info.UpdatedAt.Equal(now))
}

func TestCachedRegistry_Close_DelegatesToBase(t *testing.T) {
	t.Parallel()
	base := &mockRegistryWithExtras{tpl: &prompty.ChatPromptTemplate{}}
	reg := WithCache(base, time.Minute)
	err := reg.Close()
	require.NoError(t, err)
	assert.True(t, base.closeCalled)
}
