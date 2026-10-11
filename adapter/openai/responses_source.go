package openai

import (
	"net/http"

	"github.com/openai/openai-go/v3/option"
)

// responseSource retains public SDK RawJSON and generic HTTP metadata only.
// SDK-MUTATION-ID: malformed body IDs are unavailable unless returned by SDK.
// It never reads, replaces or interprets the HTTP response body.
type responseSource struct {
	data     []byte
	status   int
	complete bool
}

func (s *responseSource) observe(request *http.Request, next option.MiddlewareNext) (*http.Response, error) {
	response, err := next(request)
	if response != nil {
		s.status = response.StatusCode
	}
	return response, err
}
