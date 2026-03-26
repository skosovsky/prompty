package prompty

// Middleware wraps Invoker, allowing cross-cutting behavior
// for Execute and ExecuteStream (logging, metrics, tracing, etc.).
type Middleware func(next Invoker) Invoker

// Chain combines multiple middlewares around the base Invoker.
// The leftmost middleware in the chain executes first.
func Chain(base Invoker, middlewares ...Middleware) Invoker {
	h := base
	for i := len(middlewares) - 1; i >= 0; i-- {
		h = middlewares[i](h)
	}
	return h
}
