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

// FetchImage downloads a URL with ctx, size limit, and optional MIME check. Only https is allowed.
func FetchImage(ctx context.Context, rawURL string, maxBytes int64) (data []byte, contentType string, err error) {
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
	contentType = resp.Header.Get("Content-Type")
	if idx := strings.Index(contentType, ";"); idx >= 0 {
		contentType = strings.TrimSpace(contentType[:idx])
	}
	allowed := false
	for _, prefix := range AllowedImagePrefixes {
		if strings.HasPrefix(contentType, prefix) {
			allowed = true
			break
		}
	}
	if contentType != "" && !allowed {
		return nil, "", fmt.Errorf("%w: %s", ErrUnsupportedType, contentType)
	}
	data, err = io.ReadAll(io.LimitReader(resp.Body, maxBytes+1))
	if err != nil {
		return nil, "", fmt.Errorf("mediafetch: read body: %w", err)
	}
	if int64(len(data)) > maxBytes {
		return nil, "", ErrBodyTooLarge
	}
	return data, contentType, nil
}
