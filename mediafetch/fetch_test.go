package mediafetch

import (
	"context"
	"errors"
	"fmt"
	"io"
	"math"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

type bodyFailureReader struct{ cause error }

func (r bodyFailureReader) Read([]byte) (int, error) { return 0, r.cause }

func TestReadBoundedBodyRetainsReadAndProbeErrors(t *testing.T) {
	t.Parallel()
	sentinel := errors.New("fixture body failure")
	for _, test := range []struct {
		name   string
		prefix string
		cause  error
	}{
		{"read failure", "im", sentinel},
		{"probe failure", "image", sentinel},
		{"read cancellation", "im", context.Canceled},
		{"probe cancellation", "image", context.Canceled},
	} {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			// Arrange: fail either before the limit or during the overflow probe.
			body := io.MultiReader(strings.NewReader(test.prefix), bodyFailureReader{cause: test.cause})
			// Act.
			data, err := readBoundedBody(body, 5)
			// Assert: failure is not an empty success or a fabricated size violation.
			require.ErrorIs(t, err, test.cause)
			require.NotErrorIs(t, err, ErrBodyTooLarge)
			require.Nil(t, data)
		})
	}
}

func TestFetchRejectsRedirectDowngradeBeforeDispatch(t *testing.T) {
	t.Parallel()
	// Arrange.
	var calls atomic.Int32
	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		calls.Add(1)
		w.Header().Set("Content-Type", "image/png")
		_, _ = fmt.Fprint(w, "fixture-secret")
	}))
	defer target.Close()
	redirect := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, target.URL, http.StatusFound)
	}))
	defer redirect.Close()
	fetcher := DefaultFetcher{Client: redirect.Client()}
	// Act.
	_, _, err := fetcher.Fetch(context.Background(), redirect.URL)
	// Assert.
	if !errors.Is(err, ErrUnsafeScheme) || calls.Load() != 0 {
		t.Fatalf("downgrade: calls=%d error=%v", calls.Load(), err)
	}
}

func TestFetchChecksEveryHopBeforeDowngrade(t *testing.T) {
	t.Parallel()
	// Arrange: two valid HTTPS redirects precede the attempted downgrade.
	var unsafeCalls, secureCalls, policyChecks atomic.Int32
	unsafe := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		unsafeCalls.Add(1)
	}))
	defer unsafe.Close()
	secure := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		secureCalls.Add(1)
		switch r.URL.Path {
		case "/start":
			http.Redirect(w, r, "/middle", http.StatusFound)
		case "/middle":
			http.Redirect(w, r, "/last", http.StatusFound)
		default:
			http.Redirect(w, r, unsafe.URL, http.StatusFound)
		}
	}))
	defer secure.Close()
	client := secure.Client()
	client.CheckRedirect = func(*http.Request, []*http.Request) error {
		policyChecks.Add(1)
		return nil
	}
	// Act.
	data, _, err := (DefaultFetcher{Client: client}).Fetch(context.Background(), secure.URL+"/start")
	// Assert: the host's permissive policy cannot weaken the scheme invariant.
	require.ErrorIs(t, err, ErrUnsafeScheme)
	require.Empty(t, data)
	require.EqualValues(t, 3, secureCalls.Load())
	require.EqualValues(t, 2, policyChecks.Load())
	require.Zero(t, unsafeCalls.Load())
}

func TestFetchRedirectLimitsRemainCallerOwned(t *testing.T) {
	t.Parallel()
	for _, test := range []struct {
		name        string
		callerLimit int
		requests    int32
	}{
		{"default ten", 0, 10},
		{"caller three", 3, 3},
		{"caller twelve", 12, 12},
	} {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			// Arrange.
			var requests atomic.Int32
			server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				requests.Add(1)
				http.Redirect(w, r, "/loop", http.StatusFound)
			}))
			defer server.Close()
			client := server.Client()
			denied := errors.New("fixture caller hop limit")
			if test.callerLimit != 0 {
				client.CheckRedirect = func(_ *http.Request, via []*http.Request) error {
					if len(via) >= test.callerLimit {
						return denied
					}
					return nil
				}
			}
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			// Act.
			_, _, err := (DefaultFetcher{Client: client}).Fetch(ctx, server.URL)
			// Assert.
			require.Error(t, err)
			require.Equal(t, test.requests, requests.Load())
			if test.callerLimit != 0 {
				require.ErrorIs(t, err, denied)
				require.ErrorIs(t, client.CheckRedirect(nil, make([]*http.Request, test.callerLimit)), denied)
			} else {
				require.Contains(t, err.Error(), "stopped after 10 redirects")
				require.Nil(t, client.CheckRedirect)
			}
		})
	}
}

func TestFetchCancellationBetweenRedirectsPreventsNextRequest(t *testing.T) {
	t.Parallel()
	// Arrange: cancellation occurs after the initial response, before the next hop.
	var targetCalls atomic.Int32
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/" {
			http.Redirect(w, r, "/target", http.StatusFound)
			return
		}
		targetCalls.Add(1)
	}))
	defer server.Close()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	client := server.Client()
	client.CheckRedirect = func(*http.Request, []*http.Request) error {
		cancel()
		return nil
	}
	// Act.
	data, _, err := (DefaultFetcher{Client: client}).Fetch(ctx, server.URL)
	// Assert.
	require.ErrorIs(t, err, context.Canceled)
	require.Empty(t, data)
	require.Zero(t, targetCalls.Load())
}

func TestFetchIndependentConcurrentRedirectPolicies(t *testing.T) {
	t.Parallel()
	// Arrange: policies differ despite sharing a TLS server/transport configuration.
	var targetCalls atomic.Int32
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/" {
			http.Redirect(w, r, "/image", http.StatusFound)
			return
		}
		targetCalls.Add(1)
		w.Header().Set("Content-Type", "image/png")
		_, _ = fmt.Fprint(w, "image")
	}))
	defer server.Close()
	allowedClient, deniedClient := *server.Client(), *server.Client()
	denied := errors.New("fixture deny policy")
	deniedClient.CheckRedirect = func(*http.Request, []*http.Request) error { return denied }
	type fetchResult struct {
		allowed bool
		data    []byte
		err     error
	}
	results := make(chan fetchResult, 2)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	// Act.
	for _, test := range []struct {
		allowed bool
		client  *http.Client
	}{{true, &allowedClient}, {false, &deniedClient}} {
		go func() {
			data, _, err := (DefaultFetcher{Client: test.client}).Fetch(ctx, server.URL)
			results <- fetchResult{allowed: test.allowed, data: data, err: err}
		}()
	}
	// Assert: no goroutine calls testing assertions and neither policy is overwritten.
	for range 2 {
		result := <-results
		if result.allowed {
			require.NoError(t, result.err)
			require.Equal(t, "image", string(result.data))
		} else {
			require.ErrorIs(t, result.err, denied)
			require.Empty(t, result.data)
		}
	}
	require.EqualValues(t, 1, targetCalls.Load())
	require.Nil(t, allowedClient.CheckRedirect)
	require.ErrorIs(t, deniedClient.CheckRedirect(nil, nil), denied)
}

func TestFetchBodyLimitExactOverflowAndDefaults(t *testing.T) {
	t.Parallel()
	for _, test := range []struct {
		name    string
		limit   int64
		content string
		err     error
	}{
		{"exact", 5, "image", nil},
		{"over", 4, "image", ErrBodyTooLarge},
		{"empty", 1, "", nil},
		{"int64 boundary", math.MaxInt64, "image", nil},
		{"default exact", 0, strings.Repeat("i", DefaultMaxBodySize), nil},
		{"default over", 0, strings.Repeat("i", DefaultMaxBodySize+1), ErrBodyTooLarge},
	} {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			// Arrange.
			server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.Header().Set("Content-Type", "image/png")
				_, _ = fmt.Fprint(w, test.content)
			}))
			defer server.Close()
			// Act.
			data, _, err := (DefaultFetcher{Client: server.Client(), MaxBodySize: test.limit}).Fetch(
				context.Background(), server.URL)
			// Assert.
			if test.err != nil {
				require.ErrorIs(t, err, test.err)
				require.Empty(t, data)
			} else {
				require.NoError(t, err)
				require.Equal(t, test.content, string(data))
			}
		})
	}
}

func TestFetchPreservesCallerRedirectPolicy(t *testing.T) {
	t.Parallel()
	// Arrange.
	var calls atomic.Int32
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/" {
			http.Redirect(w, r, "/image", http.StatusFound)
			return
		}
		calls.Add(1)
		w.Header().Set("Content-Type", "image/png")
		_, _ = fmt.Fprint(w, "image")
	}))
	defer server.Close()
	client := server.Client()
	denied := errors.New("caller redirect denied")
	client.CheckRedirect = func(*http.Request, []*http.Request) error { return denied }
	// Act.
	_, _, err := (DefaultFetcher{Client: client}).Fetch(context.Background(), server.URL)
	// Assert.
	if !errors.Is(err, denied) || calls.Load() != 0 {
		t.Fatalf("caller policy: calls=%d error=%v", calls.Load(), err)
	}
	if !errors.Is(client.CheckRedirect(nil, nil), denied) {
		t.Fatal("caller client mutated")
	}
}

func TestFetchSecureRedirectAndLimits(t *testing.T) {
	t.Parallel()
	// Arrange.
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/" {
			http.Redirect(w, r, "/image", http.StatusFound)
			return
		}
		w.Header().Set("Content-Type", "image/png")
		_, _ = fmt.Fprint(w, "image")
	}))
	defer server.Close()
	// Act.
	data, mime, err := (DefaultFetcher{Client: server.Client()}).Fetch(context.Background(), server.URL)
	_, _, largeErr := (DefaultFetcher{Client: server.Client(), MaxBodySize: 2}).Fetch(
		context.Background(), server.URL)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, _, cancelErr := (DefaultFetcher{Client: server.Client()}).Fetch(ctx, server.URL)
	// Assert.
	if err != nil || string(data) != "image" || mime != "image/png" {
		t.Fatalf("secure redirect: %q %q %v", data, mime, err)
	}
	if !errors.Is(largeErr, ErrBodyTooLarge) || !errors.Is(cancelErr, context.Canceled) {
		t.Fatalf("limits: size=%v canceled=%v", largeErr, cancelErr)
	}
}
