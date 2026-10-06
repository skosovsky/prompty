package prompty

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"strings"
)

const (
	BatchWireFormat        = "prompty.batch"
	CapabilityBatchSubmit  = "batch_submit"
	CapabilityBatchInspect = "batch_inspect"
	CapabilityBatchCancel  = "batch_cancel"
	CapabilityBatchResults = "batch_results"
)

var (
	ErrBatch            = errors.New("prompty: invalid batch contract")
	ErrBatchScope       = errors.New("prompty: incompatible batch binding")
	ErrBatchUnknown     = errors.New("prompty: batch mutation outcome unknown")
	ErrBatchRejected    = errors.New("prompty: batch mutation rejected")
	ErrBatchUnavailable = errors.New("prompty: batch results unavailable")
	ErrBatchResults     = errors.New("prompty: incomplete batch result retrieval")
)

type BatchStatus string

const (
	BatchQueued    BatchStatus = "queued"
	BatchRunning   BatchStatus = "in_progress"
	BatchCanceling BatchStatus = "canceling"
	BatchEnded     BatchStatus = "ended"
	BatchFailed    BatchStatus = "failed"
	BatchExpired   BatchStatus = "expired"
	BatchCanceled  BatchStatus = "canceled"
	BatchUnknown   BatchStatus = "unknown"
)

type BatchItemBinding struct {
	ID              string          `json:"id"`
	OriginalID      string          `json:"original_id"`
	AttemptID       string          `json:"attempt_id"`
	ParentAttemptID string          `json:"parent_attempt_id,omitempty"`
	Identity        ProfileIdentity `json:"identity"`
	ExecutionDigest string          `json:"execution_digest"`
}

type BatchHandle struct {
	Format         string             `json:"format"`
	Provider       string             `json:"provider"`
	Endpoint       string             `json:"endpoint"`
	AccountBinding string             `json:"account_binding"`
	ProviderID     string             `json:"provider_id"`
	Status         BatchStatus        `json:"status"`
	ProviderStatus string             `json:"provider_status,omitempty"`
	Items          []BatchItemBinding `json:"items"`
	Resources      []BatchResourceRef `json:"resources,omitempty"`
}

type BatchItem struct {
	ID              string
	OriginalID      string
	AttemptID       string
	ParentAttemptID string
	Execution       *PromptExecution
}

type BatchOperationOptions struct {
	Capture CaptureRequirement `json:"capture,omitzero" yaml:"capture,omitempty"`
	Binding *CaptureBinding    `json:"-"                yaml:"-"`
}

type BatchSubmitRequest struct {
	Items   []BatchItem
	Options BatchOperationOptions
}

type BatchResultBinding struct {
	ID        string
	Execution *PromptExecution
}

type BatchResultsOptions struct {
	Options      BatchOperationOptions
	Bindings     []BatchResultBinding
	MaxLineBytes int
	MaxBytes     int64
}

type BatchItemOutcome string

const (
	BatchItemSuccess    BatchItemOutcome = "success"
	BatchItemError      BatchItemOutcome = "error"
	BatchItemRefusal    BatchItemOutcome = "refusal"
	BatchItemIncomplete BatchItemOutcome = "incomplete"
	BatchItemExpired    BatchItemOutcome = "expired"
	BatchItemCanceled   BatchItemOutcome = "canceled"
	BatchItemUnknown    BatchItemOutcome = "unknown"
)

type BatchItemResult struct {
	ID         string
	Binding    *BatchItemBinding
	Outcome    BatchItemOutcome
	Response   *Response
	SourceJSON JSONDocument
	// ErrorKind is a safe category; provider error payload stays in SourceJSON.
	ErrorKind string
}

type BatchCompleteness struct {
	DownloadComplete    bool
	CorrelationComplete bool
	MissingIDs          []string
	DuplicateIDs        []string
	UnexpectedIDs       []string
}

type BatchResult struct {
	Handle              *BatchHandle
	Items               []BatchItemResult
	Completeness        BatchCompleteness
	Diagnostics         []ExecutionDiagnostics
	Capture             *CaptureReport
	Resources           []BatchResourceRef
	SubmissionAttempted bool
}

type BatchManager interface {
	Submit(context.Context, BatchSubmitRequest) (*BatchResult, error)
	Get(context.Context, BatchHandle, ...BatchOperationOptions) (*BatchResult, error)
	Cancel(context.Context, BatchHandle, ...BatchOperationOptions) (*BatchResult, error)
	Results(context.Context, BatchHandle, BatchResultsOptions) (*BatchResult, error)
}

func CloneBatchHandle(handle BatchHandle) BatchHandle {
	handle.Items = append([]BatchItemBinding(nil), handle.Items...)
	handle.Resources = append([]BatchResourceRef(nil), handle.Resources...)
	return handle
}

func ValidateBatchHandle(handle BatchHandle) error {
	if handle.Format != BatchWireFormat || strings.TrimSpace(handle.Provider) == "" ||
		strings.TrimSpace(handle.Endpoint) == "" || strings.TrimSpace(handle.AccountBinding) == "" ||
		strings.TrimSpace(handle.ProviderID) == "" || len(handle.Items) == 0 {
		return ErrBatch
	}
	if err := ValidateEndpointIdentity(handle.Endpoint); err != nil {
		return err
	}
	if handle.ProviderStatus != "" && strings.TrimSpace(handle.ProviderStatus) == "" {
		return ErrBatch
	}
	switch handle.Status {
	case BatchQueued, BatchRunning, BatchCanceling, BatchEnded, BatchFailed, BatchExpired, BatchCanceled, BatchUnknown:
	default:
		return ErrBatch
	}
	seen := make(map[string]bool)
	resourceKinds := make(map[string]bool)
	for _, resource := range handle.Resources {
		if strings.TrimSpace(resource.Kind) == "" || resourceKinds[resource.Kind] ||
			strings.TrimSpace(resource.ProviderID) == "" {
			return ErrBatch
		}
		resourceKinds[resource.Kind] = true
	}
	for _, item := range handle.Items {
		if strings.TrimSpace(item.ID) == "" || seen[item.ID] || strings.TrimSpace(item.OriginalID) == "" ||
			strings.TrimSpace(item.AttemptID) == "" || item.ParentAttemptID == item.AttemptID ||
			(item.ParentAttemptID != "" && strings.TrimSpace(item.ParentAttemptID) == "") ||
			item.Identity.Provider != handle.Provider || item.Identity.Endpoint != handle.Endpoint ||
			strings.TrimSpace(item.Identity.Model) == "" {
			return ErrBatch
		}
		digest, err := hex.DecodeString(item.ExecutionDigest)
		if err != nil || len(digest) != sha256.Size || strings.ToLower(item.ExecutionDigest) != item.ExecutionDigest {
			return ErrBatch
		}
		seen[item.ID] = true
	}
	return nil
}

func EncodeBatchHandle(handle BatchHandle) (JSONDocument, error) {
	if err := ValidateBatchHandle(handle); err != nil {
		return nil, err
	}
	return json.Marshal(handle)
}

func DecodeBatchHandle(document JSONDocument) (BatchHandle, error) {
	var handle BatchHandle
	if err := ValidateWireJSON(document, BatchWireFormat); err != nil {
		return handle, err
	}
	if err := json.Unmarshal(document, &handle); err != nil {
		return handle, err
	}
	return handle, ValidateBatchHandle(handle)
}

// BatchExecutionDigest binds portable input; validators/capture ports are rebound.
// Capture belongs to the operation, so capture intent does not change item identity.
func BatchExecutionDigest(exec *PromptExecution) (string, error) {
	if exec == nil {
		return "", ErrBatch
	}
	working := exec.Clone()
	working.Capture = nil
	var noCapture CaptureRequirement
	working.Requirements.Capture = noCapture
	wire, err := MarshalExecution(working, WirePolicy{State: StatePreserve})
	if err != nil {
		return "", err
	}
	document, err := json.Marshal(wire)
	if err != nil {
		return "", err
	}
	digest := sha256.Sum256(document)
	return hex.EncodeToString(digest[:]), nil
}

// ReconcileBatchResults retains all observations while reporting correlation gaps.
func ReconcileBatchResults(handle BatchHandle, items []BatchItemResult, downloaded bool) BatchCompleteness {
	var completeness BatchCompleteness
	completeness.DownloadComplete = downloaded
	expected, seen := make(map[string]bool), make(map[string]int)
	for _, binding := range handle.Items {
		expected[binding.ID] = true
	}
	for _, item := range items {
		seen[item.ID]++
		if !expected[item.ID] && seen[item.ID] == 1 {
			completeness.UnexpectedIDs = append(completeness.UnexpectedIDs, item.ID)
		}
		if seen[item.ID] == 2 {
			completeness.DuplicateIDs = append(completeness.DuplicateIDs, item.ID)
		}
	}
	for _, binding := range handle.Items {
		if seen[binding.ID] == 0 {
			completeness.MissingIDs = append(completeness.MissingIDs, binding.ID)
		}
	}
	completeness.CorrelationComplete = downloaded && len(completeness.MissingIDs) == 0 &&
		len(completeness.DuplicateIDs) == 0 && len(completeness.UnexpectedIDs) == 0
	return completeness
}
