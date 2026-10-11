package openai

import (
	"context"
	"errors"
	"net/http"
	"strings"

	sdk "github.com/openai/openai-go/v3"
	"github.com/openai/openai-go/v3/option"

	"github.com/skosovsky/prompty"
	"github.com/skosovsky/prompty/adapter"
)

// BatchFiles is an explicit SDK-backed upload/download transport, not a store.
// SDK endpoint and account binding are supplied by host; no client internals are read.
type BatchFiles struct {
	client  *sdk.Client
	binding prompty.BatchFileBinding
}

func NewBatchFiles(accountBinding, endpoint string, client *sdk.Client) *BatchFiles {
	return &BatchFiles{
		client:  client,
		binding: prompty.BatchFileBinding{Provider: wireOpenai, Endpoint: endpoint, AccountBinding: accountBinding},
	}
}

func (f *BatchFiles) Binding() prompty.BatchFileBinding {
	if f == nil {
		return prompty.BatchFileBinding{}
	}
	return f.binding
}

func (f *BatchFiles) CaptureAccuracy() prompty.CaptureAccuracy {
	if f == nil || f.client == nil {
		return ""
	}
	return prompty.CaptureSemantic
}

func (f *BatchFiles) validate() error {
	if f == nil || f.client == nil {
		return adapter.ErrNoClient
	}
	if f.binding.Provider != wireOpenai || strings.TrimSpace(f.binding.Endpoint) == "" ||
		strings.TrimSpace(f.binding.AccountBinding) == "" {
		return prompty.ErrBatchScope
	}
	return prompty.ValidateEndpointIdentity(f.binding.Endpoint)
}

func (f *BatchFiles) Upload(
	ctx context.Context,
	input prompty.BatchFileUpload,
) (*prompty.BatchFileUploadResult, error) {
	if err := f.validate(); err != nil {
		return nil, err
	}
	if input.Content == nil || input.Purpose != wireBatch || input.Filename != wireBatchJsonl ||
		input.MediaType != wireApplicationJsonl {
		return nil, prompty.ErrBatch
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	source := new(responseSource)
	raw, callErr := f.client.Files.New(
		ctx,
		sdk.FileNewParams{ //nolint:exhaustruct_v5 // SDK DTO: omitted optional fields and other union arms retain their zero values.
			File:    sdk.File(input.Content, input.Filename, input.MediaType),
			Purpose: sdk.FilePurposeBatch,
		},
		option.WithMaxRetries(0),
		option.WithMiddleware(source.observe),
	)
	if raw != nil {
		source.data, source.complete = []byte(raw.RawJSON()), true
	}
	result := &prompty.BatchFileUploadResult{Resource: prompty.BatchResourceRef{Kind: "", ProviderID: ""},
		SourceJSON: prompty.CloneJSONDocument(source.data),
		HTTPStatus: source.status,
	}

	if callErr != nil {
		cause := prompty.ErrBatchFileUnknown
		if batchMutationRejected(source.status) {
			cause = prompty.ErrBatchRejected
		}
		return result, errors.Join(cause, ctx.Err())
	}
	if raw == nil || !uniqueResponseJSON([]byte(raw.RawJSON())) || !raw.JSON.ID.Valid() ||
		!batchPathID.MatchString(raw.ID) ||
		raw.Object != "file" ||
		raw.Purpose != wireBatch {
		return result, prompty.ErrBatchFileUnknown
	}
	result.Resource = prompty.BatchResourceRef{Kind: wireInput, ProviderID: raw.ID}
	return result, nil
}

func (f *BatchFiles) Download(
	ctx context.Context,
	resource prompty.BatchResourceRef,
) (*prompty.BatchFileDownload, error) {
	if err := f.validate(); err != nil {
		return nil, err
	}
	if (resource.Kind != wireOutput && resource.Kind != wireError) || !batchPathID.MatchString(resource.ProviderID) {
		return nil, prompty.ErrBatch
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	response, err := f.client.Files.Content(ctx, resource.ProviderID, option.WithMaxRetries(0))
	if err != nil || response == nil || response.Body == nil {
		if response != nil && response.Body != nil {
			_ = response.Body.Close()
		}
		return nil, errors.Join(prompty.ErrBatchUnavailable, ctx.Err())
	}
	if response.StatusCode < http.StatusOK || response.StatusCode >= http.StatusMultipleChoices {
		_ = response.Body.Close()
		return nil, prompty.ErrBatchUnavailable
	}
	return &prompty.BatchFileDownload{Body: response.Body, HTTPStatus: response.StatusCode}, nil
}

var _ prompty.BatchFileTransport = (*BatchFiles)(nil)
