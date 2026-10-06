package openai

import (
	"github.com/openai/openai-go/v3"

	"github.com/skosovsky/prompty"
	"github.com/skosovsky/prompty/adapter"
)

// CaptureResponseMetadata projects only SDK-observed identity and counters.
// A ChatCompletion ID is not an HTTP request ID or HTTP status.
func (*Adapter) CaptureResponseMetadata(response *openai.ChatCompletion) prompty.CaptureRecordMetadata {
	if response == nil {
		var empty prompty.CaptureRecordMetadata
		return empty
	}
	// Invalid counters are omitted here; checked parsing reports ErrUsage separately.
	usage, _ := usageFromOpenAI(response.Usage)
	var metadata prompty.CaptureRecordMetadata
	metadata.ProviderResponseID, metadata.Usage = response.ID, &usage
	return metadata
}

func chunkCaptureMetadata(chunk openai.ChatCompletionChunk) prompty.CaptureRecordMetadata {
	usage, _ := usageFromOpenAI(chunk.Usage)
	var metadata prompty.CaptureRecordMetadata
	metadata.ProviderResponseID, metadata.Usage = chunk.ID, &usage
	return metadata
}

// PrepareNativeRequest includes the usage option in the captured translated DTO.
func (*Adapter) PrepareNativeRequest(req *openai.ChatCompletionNewParams) (*openai.ChatCompletionNewParams, error) {
	if req == nil {
		return nil, adapter.ErrNilExecution
	}
	copyParams := *req
	copyParams.StreamOptions.IncludeUsage = openai.Bool(true)
	return &copyParams, nil
}

// CaptureCoverage declares SDK identity. Generic sync capture is semantic JSON;
// this does not promise HTTP/SSE byte capture or physical attempt observability.
func (*Adapter) CaptureCoverage(mode prompty.StreamMode) prompty.CaptureCoverage {
	var coverage prompty.CaptureCoverage
	coverage.SDKBuild = adapter.DependencyBuildIdentity("github.com/openai/openai-go/v3")
	if mode == prompty.StreamNative {
		coverage.Stages = make(map[prompty.CaptureStage]prompty.CaptureAccuracy)
		coverage.Stages[prompty.CaptureProviderChunk] = prompty.CaptureSemantic
	}
	return coverage
}
