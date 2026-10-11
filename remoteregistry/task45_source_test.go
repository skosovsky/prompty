package remoteregistry

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/skosovsky/prompty"
	"github.com/skosovsky/prompty/manifest"
)

func TestHTTPLogicalPaths(t *testing.T) {
	t.Parallel()
	for _, id := range []string{"a%2fb", "a/b", "%2e%2e/admin", "q?x=1", "frag#x", "кофе"} {
		t.Run(id, func(t *testing.T) {
			t.Parallel()
			// Arrange: HTTP path and configured query are independent of logical ID syntax.
			requests := make(chan *url.URL, 1)
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				u := *r.URL
				requests <- &u
				_, _ = w.Write([]byte("fixture"))
			}))
			defer server.Close()
			fetcher, err := NewHTTPFetcher(server.URL + "/prompts?configured=yes")
			require.NoError(t, err)
			// Act.
			data, err := fetcher.Fetch(t.Context(), id)
			// Assert.
			require.NoError(t, err)
			require.Equal(t, "fixture", string(data))
			u := <-requests
			require.Equal(t, "/prompts/"+id+".yaml", u.Path)
			require.Equal(t, "configured=yes", u.RawQuery)
			require.Empty(t, u.Fragment)
			require.Equal(t, (&url.URL{Path: u.Path}).EscapedPath(), u.EscapedPath())
		})
	}
}

type boundedFixtureBody struct {
	prefix *bytes.Reader
	end    error
	zero   bool
	extra  bool
	closed int
}

func (b *boundedFixtureBody) Read(p []byte) (int, error) {
	if b.prefix.Len() > 0 {
		return b.prefix.Read(p)
	}
	if b.zero {
		b.zero = false
		return 0, nil
	}
	if b.extra {
		b.extra = false
		p[0] = 'x'
		if !errors.Is(b.end, io.EOF) {
			return 1, b.end
		}
		return 1, nil
	}
	return 0, b.end
}
func (b *boundedFixtureBody) Close() error { b.closed++; return nil }

type sourceRoundTripper func(*http.Request) (*http.Response, error)

func (f sourceRoundTripper) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }
func TestHTTPBoundedReadPreservesTerminalEvidence(t *testing.T) {
	t.Parallel()
	for _, test := range []struct {
		name        string
		end         error
		zero, extra bool
		want        error
		success     bool
	}{
		{name: "exact EOF", end: io.EOF, success: true},
		{name: "extra", end: io.EOF, extra: true},
		{name: "byte with cancellation", end: context.Canceled, extra: true, want: context.Canceled},
		{name: "no progress", end: nil, want: io.ErrNoProgress},
		{name: "zero then extra", end: io.EOF, zero: true, extra: true},
		{name: "unexpected EOF", end: io.ErrUnexpectedEOF, want: io.ErrUnexpectedEOF},
		{name: "canceled", end: context.Canceled, want: context.Canceled},
		{name: "deadline", end: context.DeadlineExceeded, want: context.DeadlineExceeded},
	} {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			// Arrange.
			body := &boundedFixtureBody{
				prefix: bytes.NewReader(bytes.Repeat([]byte("a"), maxBodySize)),
				end:    test.end,
				zero:   test.zero,
				extra:  test.extra,
			}
			fetcher, err := NewHTTPFetcher(
				"https://fixture.invalid",
				WithHTTPClient(&http.Client{Transport: sourceRoundTripper(func(*http.Request) (*http.Response, error) {
					return &http.Response{StatusCode: http.StatusOK, Body: body}, nil
				})}),
			)
			require.NoError(t, err)
			// Act.
			data, err := fetcher.Fetch(t.Context(), "agent")
			// Assert.
			if test.success {
				require.NoError(t, err)
				require.Len(t, data, maxBodySize)
			} else {
				require.Error(t, err)
				require.Nil(t, data)
				if test.want != nil {
					require.ErrorIs(t, err, test.want)
				}
			}
			require.Equal(t, 1, body.closed)
		})
	}
}

func TestCacheEvictionDetachesInflightGeneration(t *testing.T) {
	t.Parallel()
	for _, all := range []bool{false, true} {
		for _, oldFirst := range []bool{false, true} {
			t.Run(fmt.Sprintf("all=%v/oldFirst=%v", all, oldFirst), func(t *testing.T) {
				checkCacheEviction(t, all, oldFirst)
			})
		}
	}
}

func TestHTTPImportedLogicalID(t *testing.T) {
	t.Parallel()
	// Arrange: imports use logical bytes through the same fetcher as main plans.
	var paths []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		paths = append(paths, r.URL.Path)
		if strings.HasSuffix(r.URL.Path, "main.yaml") {
			_, _ = w.Write(
				[]byte(
					`{"id":"main","version":"1","imports":[{"id":"child%2fpart"}],"messages":[{"role":"user","content":[{"type":"text","text":"main"}]}]}`,
				),
			)
			return
		}
		_, _ = w.Write(
			[]byte(
				`{"id":"child%2fpart","version":"1","messages":[{"role":"system","content":[{"type":"text","text":"child"}]}]}`,
			),
		)
	}))
	defer server.Close()
	fetcher, err := NewHTTPFetcher(server.URL + "/prompts")
	require.NoError(t, err)
	registry, err := New(fetcher, WithParser(manifest.NewJSONParser()))
	require.NoError(t, err)
	// Act.
	plan, err := registry.Plan(t.Context(), "main", prompty.RegistryPlanInput{})
	// Assert.
	require.NoError(t, err)
	require.NotNil(t, plan)
	require.Contains(t, paths, "/prompts/child%2fpart.yaml")
	require.NotContains(t, paths, "/prompts/child/part.yaml")
}

func checkCacheEviction(t *testing.T, all, oldFirst bool) {
	t.Helper()
	t.Parallel()
	// Arrange: each generation is held independently, without timer scheduling.
	started := make(chan int, 2)
	gates := []chan struct{}{make(chan struct{}), make(chan struct{})}
	var calls atomic.Int32
	fetcher := &mockFetcher{fetch: func(ctx context.Context, _ string) ([]byte, error) {
		n := int(calls.Add(1))
		if n > 2 {
			return nil, errors.New("unexpected third generation")
		}
		started <- n
		select {
		case <-gates[n-1]:
		case <-ctx.Done():
			return nil, ctx.Err()
		}
		return []byte(
			fmt.Sprintf(
				`{"id":"agent","version":"%d","messages":[{"role":"user","content":[{"type":"text","text":"generation%d"}]}]}`,
				n,
				n,
			),
		), nil
	}}
	base, err := New(fetcher, WithParser(manifest.NewJSONParser()))
	require.NoError(t, err)
	cache := WithCache(base, 0)
	type result struct {
		snapshot *prompty.ManifestSnapshot
		err      error
	}
	old := make(chan result, 1)
	fresh := make(chan result, 1)
	joined := make(chan result, 1)
	capture := func(ctx context.Context, out chan<- result) {
		s, e := cache.CaptureManifest(ctx, "agent")
		out <- result{s, e}
	}
	go capture(t.Context(), old)
	require.Equal(t, 1, <-started)
	// Act: post-eviction callers must select only the fresh inflight entry.
	if all {
		cache.EvictAll()
	} else {
		cache.Evict("agent")
	}
	go capture(t.Context(), fresh)
	require.Equal(t, 2, <-started)
	admitted := make(chan struct{})
	joinCtx := &admittedWaiterContext{Context: t.Context(), admitted: admitted}
	go capture(joinCtx, joined)
	<-admitted
	if oldFirst {
		close(gates[0])
		o := <-old
		require.NoError(t, o.err)
		close(gates[1])
		f := <-fresh
		require.NoError(t, f.err)
		require.NotEqual(t, o.snapshot.Descriptor(), f.snapshot.Descriptor())
		j := <-joined
		require.NoError(t, j.err)
		require.Equal(t, f.snapshot.Descriptor(), j.snapshot.Descriptor())
	} else {
		close(gates[1])
		f := <-fresh
		require.NoError(t, f.err)
		j := <-joined
		require.NoError(t, j.err)
		require.Equal(t, f.snapshot.Descriptor(), j.snapshot.Descriptor())
		close(gates[0])
		o := <-old
		require.NoError(t, o.err)
		require.NotEqual(t, o.snapshot.Descriptor(), f.snapshot.Descriptor())
	}
	// Assert: old completion cannot publish over generation2.
	snapshot, err := cache.CaptureManifest(t.Context(), "agent")
	require.NoError(t, err)
	plan, err := snapshot.Plan(t.Context(), prompty.RegistryPlanInput{})
	require.NoError(t, err)
	execution, err := plan.Execute(t.Context())
	require.NoError(t, err)
	require.Equal(t, "generation2", execution.Messages[0].Content[0].(prompty.TextPart).Text)
	require.EqualValues(t, 2, calls.Load())
}

func TestHTTPPreservesConfiguredEscapedBase(t *testing.T) {
	t.Parallel()
	// Arrange: an intentionally escaped base separator remains configured URL syntax.
	request := make(chan string, 1)
	server := httptest.NewServer(
		http.HandlerFunc(
			func(w http.ResponseWriter, r *http.Request) { request <- r.RequestURI; _, _ = w.Write([]byte("ok")) },
		),
	)
	defer server.Close()
	fetcher, err := NewHTTPFetcher(server.URL + "/base%2F?fixed=yes/")
	require.NoError(t, err)
	// Act.
	_, err = fetcher.Fetch(t.Context(), "a%2fb")
	// Assert.
	require.NoError(t, err)
	require.Equal(t, "/base%2F/a%252fb.yaml?fixed=yes/", <-request)
}
