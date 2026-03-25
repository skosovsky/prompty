package remoteregistry

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// HTTPFetcher fetches YAML manifests over HTTP. URL resolution: {baseURL}/{name}.{env}.yaml (or .yml),
// then fallback to {baseURL}/{name}.yaml (or .yml). 404 tries next candidate; other non-2xx returns ErrHTTPStatus.
var _ Fetcher = (*HTTPFetcher)(nil)

// maxBodySize limits HTTP response body size (1 MB); YAML manifests are small.
const maxBodySize = 1 << 20

// defaultUserAgent is the User-Agent header value for HTTP requests.
const defaultUserAgent = "prompty-remote-registry/1.0"

// defaultHTTPClientTimeout is the default timeout for manifest fetches when no custom client is set.
const defaultHTTPClientTimeout = 30 * time.Second

// HTTPFetcher holds base URL, client, and optional Bearer token.
type HTTPFetcher struct {
	baseURL    string
	httpClient *http.Client
	authToken  string
}

// HTTPOption configures HTTPFetcher.
type HTTPOption func(*HTTPFetcher)

// WithHTTPClient sets the HTTP client. Default has 30s timeout. If c is nil, the default client is left unchanged.
func WithHTTPClient(c *http.Client) HTTPOption {
	return func(h *HTTPFetcher) {
		if c != nil {
			h.httpClient = c
		}
	}
}

// WithAuthToken sets the Bearer token for Authorization header.
func WithAuthToken(token string) HTTPOption {
	return func(h *HTTPFetcher) {
		h.authToken = token
	}
}

// NewHTTPFetcher creates an HTTPFetcher. baseURL must be a valid URL (e.g. https://api.example.com/prompts).
func NewHTTPFetcher(baseURL string, opts ...HTTPOption) (*HTTPFetcher, error) {
	baseURL = strings.TrimSuffix(baseURL, "/")
	if baseURL == "" {
		return nil, errors.New("remoteregistry: base URL must not be empty")
	}
	parsed, err := url.Parse(baseURL)
	if err != nil || parsed.Scheme == "" {
		return nil, fmt.Errorf("remoteregistry: invalid base URL %q", baseURL)
	}
	h := &HTTPFetcher{
		baseURL:    baseURL,
		httpClient: &http.Client{Timeout: defaultHTTPClientTimeout},
	}
	for _, opt := range opts {
		opt(h)
	}
	return h, nil
}

// Fetch tries URLs in order: {base}/{id}.yaml, {base}/{id}.yml.
// On 404 proceeds to next; on other non-2xx returns ErrHTTPStatus.
func (h *HTTPFetcher) Fetch(ctx context.Context, id string) ([]byte, error) {
	if err := ValidatePathForFetch(id); err != nil {
		return nil, err
	}
	for _, path := range CandidatePaths(id) {
		data, err := h.fetchOne(ctx, path)
		if err != nil {
			if errors.Is(err, errNotFound) {
				continue
			}
			return nil, err
		}
		return data, nil
	}
	return nil, fmt.Errorf("%w: %q", ErrNotFound, id)
}

var errNotFound = errors.New("not found")

func (h *HTTPFetcher) fetchOne(ctx context.Context, pathSeg string) ([]byte, error) {
	u, err := url.JoinPath(h.baseURL, pathSeg)
	if err != nil {
		return nil, fmt.Errorf("%w: join path: %w", ErrFetchFailed, err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return nil, fmt.Errorf("%w: %w", ErrFetchFailed, err)
	}
	req.Header.Set("User-Agent", defaultUserAgent)
	if h.authToken != "" {
		req.Header.Set("Authorization", "Bearer "+h.authToken)
	}
	resp, err := h.httpClient.Do(req) // #nosec G704 -- URL is from config and path-escaped name
	if err != nil {
		return nil, fmt.Errorf("%w: %w", ErrFetchFailed, err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode == http.StatusNotFound {
		return nil, errNotFound
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("%w: %w: %s %s", ErrFetchFailed, ErrHTTPStatus, resp.Status, u)
	}
	data, err := io.ReadAll(io.LimitReader(resp.Body, maxBodySize))
	if err != nil {
		return nil, fmt.Errorf("%w: read body: %w", ErrFetchFailed, err)
	}
	// Detect truncation: if more data is available, body exceeded maxBodySize.
	probe := make([]byte, 1)
	if n, _ := resp.Body.Read(probe); n > 0 {
		return nil, fmt.Errorf("%w: response body exceeds %d bytes", ErrFetchFailed, maxBodySize)
	}
	return data, nil
}
