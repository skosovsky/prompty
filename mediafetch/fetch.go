// Package mediafetch provides safe URL download for adapters that require inline media (e.g. base64).
// Used when MediaPart has URL but the provider API does not accept URLs (Anthropic, Ollama).
package mediafetch

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
)

const (
	// DefaultMaxBodySize is the default limit for media download (10 MiB).
	DefaultMaxBodySize = 10 << 20
)

var (
	// ErrUnsafeScheme is returned when the URL scheme is not https.
	ErrUnsafeScheme = errors.New("mediafetch: only https scheme is allowed")
	// ErrBodyTooLarge is returned when the response exceeds the size limit.
	ErrBodyTooLarge = errors.New("mediafetch: response body exceeds size limit")
	// ErrUnsupportedType is returned when Content-Type is not allowed (e.g. not image/*).
	ErrUnsupportedType = errors.New("mediafetch: unsupported content type")
)

// DefaultFetcher implements URL-to-bytes fetching using standard HTTP (HTTPS only, size limit, image MIME check).
// It satisfies the Fetcher interface used by prompty.ResolveMedia; no import of prompty is required.
type DefaultFetcher struct {
	MaxBodySize int64
	// Client supplies transport, egress and redirect restrictions. It is never mutated.
	// A nil client uses http.DefaultClient. HTTPS remains mandatory on every redirect.
	Client *http.Client
}

// Fetch downloads the URL and returns body and Content-Type. Only https is allowed; response is limited to MaxBodySize (or DefaultMaxBodySize if 0).
func (f DefaultFetcher) Fetch(ctx context.Context, rawURL string) ([]byte, string, error) {
	maxBytes := f.MaxBodySize
	if maxBytes <= 0 {
		maxBytes = DefaultMaxBodySize
	}
	u, err := url.Parse(rawURL)
	if err != nil {
		return nil, "", fmt.Errorf("mediafetch: parse URL: %w", err)
	}
	if u.Scheme != "https" {
		return nil, "", ErrUnsafeScheme
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return nil, "", fmt.Errorf("mediafetch: new request: %w", err)
	}
	client := f.Client
	if client == nil {
		client = http.DefaultClient
	}
	// Copy the client, retaining its transport and timeout without modifying the
	// caller's redirect policy or sharing per-fetch state.
	checkedClient := *client
	checkedClient.CheckRedirect = func(next *http.Request, via []*http.Request) error {
		if next.URL.Scheme != "https" {
			return ErrUnsafeScheme
		}
		if client.CheckRedirect != nil {
			return client.CheckRedirect(next, via)
		}
		const maxRedirects = 10
		if len(via) >= maxRedirects {
			return errors.New("mediafetch: stopped after 10 redirects")
		}
		return nil
	}
	resp, err := checkedClient.Do(req)
	if err != nil {
		return nil, "", fmt.Errorf("mediafetch: do request: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return nil, "", fmt.Errorf("mediafetch: status %s", resp.Status)
	}
	mimeType := resp.Header.Get("Content-Type")
	if idx := strings.Index(mimeType, ";"); idx >= 0 {
		mimeType = strings.TrimSpace(mimeType[:idx])
	}
	allowed := strings.HasPrefix(mimeType, "image/")
	if mimeType != "" && !allowed {
		return nil, "", fmt.Errorf("%w: %s", ErrUnsupportedType, mimeType)
	}
	data, err := readBoundedBody(resp.Body, maxBytes)
	if err != nil {
		return nil, "", fmt.Errorf("mediafetch: read body: %w", err)
	}
	return data, mimeType, nil
}

// Read at most the limit plus a one-byte probe without adding to the limit;
// maxBytes may be MaxInt64, for which maxBytes+1 would wrap to a negative bound.
func readBoundedBody(body io.Reader, maxBytes int64) ([]byte, error) {
	data, err := io.ReadAll(io.LimitReader(body, maxBytes))
	if err != nil {
		return nil, err
	}
	if int64(len(data)) < maxBytes {
		return data, nil
	}
	var probe [1]byte
	read, probeErr := io.ReadFull(body, probe[:])
	if read != 0 {
		return nil, ErrBodyTooLarge
	}
	if !errors.Is(probeErr, io.EOF) {
		return nil, probeErr
	}
	return data, nil
}

// FetchImage downloads a URL with ctx, size limit, and optional MIME check. Only https is allowed.
func FetchImage(ctx context.Context, rawURL string, maxBytes int64) ([]byte, string, error) {
	return DefaultFetcher{MaxBodySize: maxBytes, Client: nil}.Fetch(ctx, rawURL)
}
