package prompty

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
)

// stripMarkdownJSON removes markdown code block wrappers (e.g. ```json ... ```) before JSON parsing.
func stripMarkdownJSON(s string) string {
	s = strings.TrimSpace(s)
	s = strings.TrimPrefix(s, "```json")
	s = strings.TrimPrefix(s, "```")
	s = strings.TrimSuffix(s, "```")
	return strings.TrimSpace(s)
}

// ExecuteWithStructuredOutput performs a request to the LLM and attempts to parse the response as JSON into type T.
// On JSON validation error, it adds a pair of messages to PromptExecution (assistant with the "bad" output
// and user with the error text) and retries up to maxRetries times.
//
// maxRetries is the number of retry attempts on JSON validation error. Total API calls = maxRetries + 1
// (e.g. maxRetries=3 means up to 4 calls). LLM responses wrapped in markdown (```json ... ```)
// are automatically stripped before parsing.
func ExecuteWithStructuredOutput[T any](
	ctx context.Context,
	client LLMClient,
	exec *PromptExecution,
	maxRetries int,
) (*T, error) {
	if client == nil {
		return nil, fmt.Errorf("structured output: client is nil")
	}
	if exec == nil {
		return nil, fmt.Errorf("structured output: execution is nil")
	}

	var lastErr error

	for attempt := 0; attempt <= maxRetries; attempt++ {
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}

		resp, err := client.Generate(ctx, exec)
		if err != nil {
			return nil, err
		}
		if resp == nil {
			return nil, fmt.Errorf("structured output: nil response")
		}

		var result T
		rawText := stripMarkdownJSON(resp.Text())
		if err := json.Unmarshal([]byte(rawText), &result); err != nil {
			lastErr = err
			exec = exec.AppendValidationRetry(resp.Text(), err)
			continue
		}
		return &result, nil
	}

	return nil, fmt.Errorf("structured output: validation failed after %d retries: %w", maxRetries+1, lastErr)
}
