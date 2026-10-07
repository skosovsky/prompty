package openai

import (
	"net/http"

	"github.com/openai/openai-go/v3/option"
)

// responseSSESource observes HTTP metadata without consuming or replacing the
// response body. The SDK owns SSE decoding and body closure. Capturing individual
// decoded events must happen in the consuming loop, never in this middleware.
type responseSSESource struct {
	status   int
	events   int
	complete bool
}

func (s *responseSSESource) observe(
	request *http.Request, next option.MiddlewareNext,
) (*http.Response, error) {
	response, err := next(request)
	if response != nil {
		s.status = response.StatusCode
	}
	return response, err
}
