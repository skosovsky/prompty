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

// AllowedImagePrefixes are Content-Type prefixes accepted for image media (e.g. "image/png"). Do not modify.
var AllowedImagePrefixes = []string{"image/"}

// DefaultClient is the HTTP client used for fetching. Override in tests to use a custom client (e.g. TLS with InsecureSkipVerify).
var DefaultClient = http.DefaultClient

// DefaultFetcher implements URL-to-bytes fetching using standard HTTP (HTTPS only, size limit, image MIME check).
// It satisfies the Fetcher interface used by prompty.ResolveMedia; no import of prompty is required.
type DefaultFetcher struct {
	MaxBodySize int64
}

// Fetch downloads the URL and returns body and Content-Type. Only https is allowed; response is limited to MaxBodySize (or DefaultMaxBodySize if 0).
func (f DefaultFetcher) Fetch(ctx context.Context, rawURL string) (data []byte, mimeType string, err error) {
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
	resp, err := DefaultClient.Do(req)
	if err != nil {
		return nil, "", fmt.Errorf("mediafetch: do request: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return nil, "", fmt.Errorf("mediafetch: status %s", resp.Status)
	}
	mimeType = resp.Header.Get("Content-Type")
	if idx := strings.Index(mimeType, ";"); idx >= 0 {
		mimeType = strings.TrimSpace(mimeType[:idx])
	}
	allowed := false
	for _, prefix := range AllowedImagePrefixes {
		if strings.HasPrefix(mimeType, prefix) {
			allowed = true
			break
		}
	}
	if mimeType != "" && !allowed {
		return nil, "", fmt.Errorf("%w: %s", ErrUnsupportedType, mimeType)
	}
	data, err = io.ReadAll(io.LimitReader(resp.Body, maxBytes+1))
	if err != nil {
		return nil, "", fmt.Errorf("mediafetch: read body: %w", err)
	}
	if int64(len(data)) > maxBytes {
		return nil, "", ErrBodyTooLarge
	}
	return data, mimeType, nil
}

// FetchImage downloads a URL with ctx, size limit, and optional MIME check. Only https is allowed.
func FetchImage(ctx context.Context, rawURL string, maxBytes int64) (data []byte, contentType string, err error) {
	return DefaultFetcher{MaxBodySize: maxBytes}.Fetch(ctx, rawURL)
}
