package adapter

import (
	"context"
	"iter"

	"github.com/skosovsky/prompty"
)

// clientImpl implements prompty.Invoker via ProviderAdapter.
type clientImpl[Req any, Resp any] struct {
	adapter ProviderAdapter[Req, Resp]
}

// Generate performs Translate → Execute → ParseResponse.
func (c *clientImpl[Req, Resp]) Generate(ctx context.Context, exec *prompty.PromptExecution) (*prompty.Response, error) {
	req, err := c.adapter.Translate(ctx, exec)
	if err != nil {
		return nil, err
	}
	resp, err := c.adapter.Execute(ctx, req)
	if err != nil {
		return nil, err
	}
	return c.adapter.ParseResponse(ctx, resp)
}

// GenerateStream uses StreamerAdapter if implemented; otherwise polyfill via Generate.
func (c *clientImpl[Req, Resp]) GenerateStream(ctx context.Context, exec *prompty.PromptExecution) iter.Seq2[*prompty.ResponseChunk, error] {
	streamer, ok := any(c.adapter).(StreamerAdapter[Req])
	if ok {
		req, err := c.adapter.Translate(ctx, exec)
		if err != nil {
			return func(yield func(*prompty.ResponseChunk, error) bool) { yield(nil, err) }
		}
		return streamer.ExecuteStream(ctx, req)
	}
	// Polyfill: single chunk via Generate
	resp, err := c.Generate(ctx, exec)
	if err != nil {
		return func(yield func(*prompty.ResponseChunk, error) bool) { yield(nil, err) }
	}
	if resp == nil {
		return func(yield func(*prompty.ResponseChunk, error) bool) {
			yield(&prompty.ResponseChunk{Content: nil, IsFinished: true}, nil)
		}
	}
	return func(yield func(*prompty.ResponseChunk, error) bool) {
		chunk := &prompty.ResponseChunk{
			Content:    resp.Content,
			Usage:      resp.Usage,
			IsFinished: true,
		}
		yield(chunk, nil)
	}
}

// NewClient creates prompty.LLMClient from ProviderAdapter. Middlewares wrap the base Invoker.
func NewClient[Req any, Resp any](adp ProviderAdapter[Req, Resp], mws ...prompty.Middleware) prompty.LLMClient {
	base := &clientImpl[Req, Resp]{adapter: adp}
	if len(mws) == 0 {
		return base
	}
	return prompty.Chain(base, mws...)
}
