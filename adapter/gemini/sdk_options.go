package gemini

import "google.golang.org/genai"

// Public per-operation SDK retry control. The borrowed host client, endpoint,
// auth and options are unchanged; accepted mutations are never replayed by us.
func noRetryHTTPOptions(original *genai.HTTPOptions) *genai.HTTPOptions {
	var options genai.HTTPOptions
	if original != nil {
		options = *original
		options.Headers = original.Headers.Clone()
	}
	attempts := int32(1)
	options.RetryOptions = &genai.HTTPRetryOptions{
		Attempts:        &attempts,
		InitialDelay:    nil,
		MaxDelay:        nil,
		ExpBase:         nil,
		Jitter:          nil,
		HTTPStatusCodes: nil,
	}
	return &options
}
