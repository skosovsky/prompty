package adapter

import "github.com/skosovsky/prompty"

// PreparedBatchItem reuses checked request/schema preparation without dispatch.
// Request belongs to the submission transport. Results use separate local
// original-schema preparation instead of replaying this request.
type PreparedBatchItem[Req any] struct {
	Request     Req
	Diagnostics prompty.ExecutionDiagnostics
}

func PrepareBatchItem[Req, Resp any](
	adp ProviderAdapter[Req, Resp],
	exec *prompty.PromptExecution,
) (*PreparedBatchItem[Req], error) {
	if err := ValidateExecution(exec); err != nil {
		return nil, err
	}
	if exec.Capture != nil || len(exec.Requirements.Capture.Stages) != 0 || exec.Requirements.Capture.Accuracy != "" ||
		exec.Requirements.Capture.Durable || exec.Requirements.Capture.PhysicalAttempts {
		return nil, prompty.ErrCaptureUnsupported
	}
	prepared, err := Prepare(adp, exec, "")
	if err != nil {
		return nil, err
	}
	return &PreparedBatchItem[Req]{
		Request:     prepared.request,
		Diagnostics: prepared.Diagnostics(),
	}, nil
}
