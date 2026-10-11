package prompty

import "context"

// Invoker defines the canonical model invocation port (sync + stream).
// Implementations must check required guarantees against their effective
// destination before dispatch and fail closed when they cannot establish them.
// Helpers enforce local schema/outcome/tool contracts but cannot independently
// inspect an arbitrary host implementation's transport, profile or SDK ports.
// Use a checked adapter client, or delegate unchanged executions to one, when
// provider preflight is required. Wrappers must not discard requirements or
// replace requested native streaming with buffered execution.
type Invoker interface {
	Execute(ctx context.Context, exec *PromptExecution) (*Response, error)
	ExecuteStream(ctx context.Context, exec *PromptExecution, mode StreamMode) *Stream
}
