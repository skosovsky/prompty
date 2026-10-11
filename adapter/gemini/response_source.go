package gemini

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"

	"google.golang.org/genai"

	"github.com/skosovsky/prompty"
	"github.com/skosovsky/prompty/adapter"
)

const defaultResponsePayloadLimit = 4 << 20

// SDK representation capture; never original body observation.
type responseSource struct {
	data     bytes.Buffer
	limit    int
	overflow bool
	complete bool
	status   int
	job      *genai.BatchJob
	cache    *genai.CachedContent
}

func (a *Adapter) executeObservedResponse(
	ctx context.Context,
	request *Request,
) (*genai.GenerateContentResponse, error) {
	if a.client == nil {
		return nil, adapter.ErrNoClient
	}
	if a.responseLimit <= 0 {
		return nil, prompty.ErrProviderState
	}
	copied := copyNativeRequest(request)
	response, err := a.client.Models.GenerateContent(ctx, copied.Model, copied.Contents, copied.Config)
	if err != nil {
		return response, safeObservedResponseError(ctx, err)
	}
	return response, nil
}

func safeObservedResponseError(ctx context.Context, sdkError error) error {
	cause := ctx.Err()
	if cause == nil {
		switch {
		case errors.Is(sdkError, context.Canceled):
			cause = context.Canceled
		case errors.Is(sdkError, context.DeadlineExceeded):
			cause = context.DeadlineExceeded
		}
	}
	if cause != nil {
		return errors.Join(prompty.ErrProviderState, cause)
	}
	return prompty.ErrProviderState
}

// SDK-GENAI-NUM: public DTO arguments cannot attest original numeric fidelity.
// SDK-GENAI-NUM in docs/official-sdk-migration.md also defines signed state as
// SDK representation, never independently verified original provider source.
func originalGeminiPart(_ *genai.GenerateContentResponse, _ int, part *genai.Part) (*genai.Part, error) {
	if part != nil && part.FunctionCall != nil {
		if _, err := json.Marshal(part.FunctionCall.Args); err != nil || containsSDKFloat(part.FunctionCall.Args) {
			return nil, errors.Join(prompty.ErrProviderState, adapter.ErrMalformedArgs)
		}
	}
	return part, nil
}
func containsSDKFloat(value any) bool { return adapter.HasUnverifiedJSONNumbers(value) }

// sdkUsageFailure retains SDK-known billing evidence without publishing content,
// state or a success outcome after downstream representation failure.
func sdkUsageFailure(raw *genai.GenerateContentResponse) *prompty.Response {
	if raw == nil {
		return nil
	}
	usage := geminiUsage(raw)
	if len(usage.Known) == 0 {
		return nil
	}
	response := prompty.NewResponse(nil)
	response.Model, response.Usage = raw.ModelVersion, usage
	response.Outcome, response.FinishReason = "", ""
	response.ContinuationUnavailable = true
	return response
}

func sdkUsageFailureChunk(raw *genai.GenerateContentResponse) *prompty.ResponseChunk {
	response := sdkUsageFailure(raw)
	if response == nil {
		return nil
	}
	frame := new(prompty.ResponseChunk)
	frame.Kind, frame.Model, frame.Usage = prompty.StreamUsage, response.Model, response.Usage
	frame.ContinuationUnavailable = true
	return frame
}

// The public SDK error code is available rejection metadata. Successful SDK
// job/cache DTOs expose no exact HTTP status; leave that evidence unknown.
func sdkOperationHTTPStatus(err error) int {
	code := 0
	if value, ok := errors.AsType[genai.APIError](err); ok {
		code = value.Code
	} else if pointer, ok := errors.AsType[*genai.APIError](err); ok && pointer != nil {
		code = pointer.Code
	}
	if code < 100 || code > 599 {
		return 0
	}
	return code
}
