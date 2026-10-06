package prompty

import (
	"context"
	"errors"
	"io"
)

var ErrBatchFileUnknown = errors.New("prompty: batch file upload outcome unknown")

// BatchFileBinding declares host-owned transport destination, never IAM.
type BatchFileBinding struct {
	Provider       string
	Endpoint       string
	AccountBinding string
}

type BatchResourceRef struct {
	Kind       string `json:"kind"`
	ProviderID string `json:"provider_id"`
}

type BatchFileUpload struct {
	Content   io.Reader
	Filename  string
	MediaType string
	Purpose   string
}

type BatchFileUploadResult struct {
	Resource   BatchResourceRef
	SourceJSON JSONDocument
	HTTPStatus int
}

type BatchFileDownload struct {
	Body       io.ReadCloser
	HTTPStatus int
}

// BatchFileTransport supplies transport/auth/egress; no file storage is implied.
// Upload must not retry unknown acceptance. Download body ownership transfers
// to the operation, which closes it exactly once even on parsing/capture failure.
type BatchFileTransport interface {
	Binding() BatchFileBinding
	Upload(context.Context, BatchFileUpload) (*BatchFileUploadResult, error)
	Download(context.Context, BatchResourceRef) (*BatchFileDownload, error)
}

// BatchFileCaptureDescriber declares observable original source coverage. Host
// transport implementations without it do not inherit SDK capture capabilities.
type BatchFileCaptureDescriber interface {
	CaptureAccuracy() CaptureAccuracy
}
