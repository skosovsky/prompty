package prompty

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"maps"
	"slices"
	"sort"
	"strings"
	"sync"
)

// CaptureStage distinguishes canonical data, SDK projections and actual transport boundaries.
type CaptureStage string

const (
	CaptureExecution         CaptureStage = "canonical_execution"
	CaptureTranslatedRequest CaptureStage = "translated_request"
	CaptureTransportRequest  CaptureStage = "transport_request"
	CaptureTransportResponse CaptureStage = "transport_response"
	CaptureProviderResponse  CaptureStage = "provider_response"
	CaptureProviderChunk     CaptureStage = "provider_chunk"
	CaptureCanonicalParts    CaptureStage = "canonical_parts"
	CaptureTerminal          CaptureStage = "canonical_terminal"
)

// CaptureAccuracy describes the projection, not the quality of generated output.
type CaptureAccuracy string

const (
	captureRankUnknown = iota
	captureRankMetadata
	captureRankSemantic
	captureRankBytes
)

const (
	defaultCaptureMaxBytes = 65536
	captureJSONMediaType   = "application/json"
)

const (
	CaptureMetadata CaptureAccuracy = "metadata"
	CaptureSemantic CaptureAccuracy = "semantic"
	CaptureBytes    CaptureAccuracy = "bytes"
)

// CaptureRequirement is portable intent; runtime ports are rebound after persistence.
type CaptureRequirement struct {
	Stages           []CaptureStage  `json:"stages,omitempty"`
	Accuracy         CaptureAccuracy `json:"accuracy,omitempty"`
	Durable          bool            `json:"durable,omitempty"`
	PhysicalAttempts bool            `json:"physical_attempts,omitempty"`
}

// CaptureCoverage is implementation knowledge, never inferred from provider names.
type CaptureCoverage struct {
	Stages           map[CaptureStage]CaptureAccuracy
	PhysicalAttempts bool
	SDKBuild         string
}

// CaptureSinkCapabilities promises only guarantees implemented by the caller's sink.
type CaptureSinkCapabilities struct{ Durable bool }

// CaptureAck confirms acceptance; durable is not inferred from a successful write.
type CaptureAck struct{ Durable bool }

// CaptureSink must honor context cancellation and be safe across concurrent invocations.
type CaptureSink interface {
	Capabilities() CaptureSinkCapabilities
	WriteCapture(context.Context, CaptureRecord) (CaptureAck, error)
}

// CaptureProjection is the host-approved payload. Complete is false when source data was omitted.
type CaptureProjection struct {
	Metadata  CaptureRecordMetadata
	Payload   []byte
	MediaType string
	Codec     string
	Accuracy  CaptureAccuracy
	Redacted  bool
	Truncated bool
	Complete  bool
}

// CaptureProjector owns redaction/egress policy. Capability is an upper bound;
// actual projection accuracy is checked for every event. It must honor context.
type CaptureProjector interface {
	MaxAccuracy() CaptureAccuracy
	ProjectCapture(context.Context, CaptureRecord) (CaptureProjection, error)
}

// CaptureBinding contains non-persisted caller ports. No recorder is mandatory.
type CaptureBinding struct {
	Sink          CaptureSink
	Projector     CaptureProjector
	CorrelationID string
	MaxBytes      int // zero uses 64 KiB; this bounds delivered payload, not SDK response size
}

// CaptureRecord has stable invocation-local ordering. Empty PhysicalAttemptID
// means unobservable, not a synthetic attempt equal to the logical invocation.
type CaptureRecord struct {
	InvocationID      string
	CorrelationID     string
	PhysicalAttemptID string
	Sequence          uint64
	Stage             CaptureStage
	Profile           ProfileIdentity
	SDKBuild          string
	AfterDispatch     bool
	Projection        CaptureProjection
}

// CaptureFailure intentionally excludes raw error bodies and caller secrets.
type CaptureFailure struct {
	Stage         CaptureStage `json:"stage"`
	AfterDispatch bool         `json:"after_dispatch"`
	Kind          string       `json:"kind"`
}

// CaptureStageCoverage is a conservative implementation snapshot, not observed bytes.
type CaptureStageCoverage struct {
	Stage    CaptureStage    `json:"stage"`
	Accuracy CaptureAccuracy `json:"accuracy"`
}

// CaptureReport is a cloned invocation snapshot, not a persistence promise after a crash.
type CaptureReport struct {
	InvocationID string                 `json:"invocation_id"`
	SDKBuild     string                 `json:"sdk_build,omitempty"`
	Coverage     []CaptureStageCoverage `json:"coverage,omitempty"`
	Observed     []CaptureStage         `json:"observed,omitempty"`
	Failures     []CaptureFailure       `json:"failures,omitempty"`
	Closed       bool                   `json:"closed"`
	Complete     bool                   `json:"complete"`
}

var (
	ErrCapture            = errors.New("prompty: capture failed")
	ErrCaptureUnsupported = errors.New("prompty: required capture coverage unavailable")
)

// CaptureError distinguishes capture failure after possible execution from a
// pre-dispatch failure. Error strings never copy sink/projector secrets.
type CaptureError struct {
	Stage         CaptureStage
	AfterDispatch bool
	Cause         error
}

func (e *CaptureError) Error() string {
	return fmt.Sprintf("%v: stage=%s after_dispatch=%t", ErrCapture, e.Stage, e.AfterDispatch)
}

func (e *CaptureError) Unwrap() []error { return []error{ErrCapture, e.Cause} }

// CaptureSession serializes synchronous callbacks. This does not bound latency,
// concurrent callers or the retained advisory failure list. Do not re-enter Emit
// from its callbacks. Report may safely be inspected from a callback.
type CaptureSession struct {
	emitMu            sync.Mutex
	mu                sync.Mutex
	binding           CaptureBinding
	requirement       CaptureRequirement
	coverage          CaptureCoverage
	profile           ProfileIdentity
	durable           bool
	projectorAccuracy CaptureAccuracy
	sequence          uint64
	report            CaptureReport
}

// ValidateCaptureRequirement checks portable intent without requiring runtime ports.
func ValidateCaptureRequirement(req CaptureRequirement) error {
	if req.Accuracy != "" && captureAccuracyRank(req.Accuracy) == 0 {
		return ErrCaptureUnsupported
	}
	seen := make(map[CaptureStage]bool)
	for _, stage := range req.Stages {
		if !validCaptureStage(stage) || seen[stage] {
			return ErrCaptureUnsupported
		}
		seen[stage] = true
	}
	return nil
}

// NewCaptureSession validates all required coverage before any provider dispatch.
// Nil binding without requirements creates a disabled capture session.
func NewCaptureSession(
	req CaptureRequirement,
	binding *CaptureBinding,
	coverage CaptureCoverage,
	profile ProfileIdentity,
) (*CaptureSession, error) {
	if err := ValidateCaptureRequirement(req); err != nil {
		return nil, err
	}
	required := len(req.Stages) != 0 || req.Accuracy != "" || req.Durable || req.PhysicalAttempts
	if binding == nil || binding.Sink == nil {
		if required {
			return nil, ErrCaptureUnsupported
		}
		return new(CaptureSession), nil
	}
	projectorAccuracy := CaptureMetadata
	if binding.Projector != nil {
		projectorAccuracy = binding.Projector.MaxAccuracy()
	}
	if captureAccuracyRank(projectorAccuracy) == 0 || binding.MaxBytes < 0 {
		return nil, ErrCaptureUnsupported
	}
	capabilities := binding.Sink.Capabilities()
	if required && len(coverage.Stages) == 0 {
		return nil, ErrCaptureUnsupported
	}
	if (req.Durable && !capabilities.Durable) || (req.PhysicalAttempts && !coverage.PhysicalAttempts) {
		return nil, ErrCaptureUnsupported
	}
	if err := validateCaptureCoverage(req, coverage, projectorAccuracy); err != nil {
		return nil, err
	}
	var identity [16]byte
	if _, err := rand.Read(identity[:]); err != nil {
		return nil, &CaptureError{Stage: CaptureExecution, AfterDispatch: false, Cause: err}
	}
	s := new(CaptureSession)
	s.binding, s.requirement, s.coverage, s.profile = *binding, req, coverage, profile
	s.durable, s.projectorAccuracy = capabilities.Durable, projectorAccuracy
	s.report.InvocationID, s.report.SDKBuild = hex.EncodeToString(identity[:]), coverage.SDKBuild
	s.report.Complete = true
	s.requirement.Stages = slices.Clone(req.Stages)
	s.coverage.Stages = maps.Clone(coverage.Stages)
	for stage, accuracy := range s.coverage.Stages {
		s.report.Coverage = append(s.report.Coverage, CaptureStageCoverage{Stage: stage, Accuracy: accuracy})
	}
	sort.Slice(s.report.Coverage, func(i, j int) bool {
		return s.report.Coverage[i].Stage < s.report.Coverage[j].Stage
	})
	if s.binding.MaxBytes == 0 {
		s.binding.MaxBytes = defaultCaptureMaxBytes
	}
	return s, nil
}

func validateCaptureCoverage(
	req CaptureRequirement,
	coverage CaptureCoverage,
	projectorAccuracy CaptureAccuracy,
) error {
	minimum := captureAccuracyRank(req.Accuracy)
	if minimum > captureAccuracyRank(projectorAccuracy) {
		return ErrCaptureUnsupported
	}
	for stage, accuracy := range coverage.Stages {
		if !validCaptureStage(stage) || captureAccuracyRank(accuracy) == 0 ||
			(req.Accuracy != "" && len(req.Stages) == 0 && captureAccuracyRank(accuracy) < minimum) {
			return ErrCaptureUnsupported
		}
	}
	for _, stage := range req.Stages {
		available := captureAccuracyRank(coverage.Stages[stage])
		if available == 0 || available < minimum {
			return ErrCaptureUnsupported
		}
	}
	return nil
}

// Emit delivers an isolated host projection. Required errors stop the caller;
// advisory failures are retained in Report but do not change execution outcome.
func (s *CaptureSession) Emit(ctx context.Context, record CaptureRecord) error {
	if s == nil || s.binding.Sink == nil {
		return nil
	}
	if !validCaptureStage(record.Stage) {
		return &CaptureError{Stage: record.Stage, AfterDispatch: record.AfterDispatch, Cause: ErrCaptureUnsupported}
	}
	s.emitMu.Lock()
	defer s.emitMu.Unlock()
	s.mu.Lock()
	if s.report.Closed {
		s.mu.Unlock()
		return &CaptureError{Stage: record.Stage, AfterDispatch: record.AfterDispatch, Cause: ErrCaptureUnsupported}
	}
	s.sequence++
	record.Sequence = s.sequence
	record.InvocationID = s.report.InvocationID
	s.mu.Unlock()
	record.CorrelationID, record.Profile, record.SDKBuild = s.binding.CorrelationID, s.profile, s.coverage.SDKBuild
	if captureAccuracyRank(s.coverage.Stages[record.Stage]) == 0 {
		return s.failure(record, "coverage", ErrCaptureUnsupported)
	}
	if record.PhysicalAttemptID != "" && !s.coverage.PhysicalAttempts {
		return s.failure(record, "attempt_identity", ErrCaptureUnsupported)
	}
	if s.requirement.PhysicalAttempts && record.AfterDispatch && record.PhysicalAttemptID == "" {
		return s.failure(record, "attempt_identity", ErrCaptureUnsupported)
	}
	projection, err := s.project(ctx, record)
	if err != nil {
		return s.failure(record, "projector", err)
	}
	// Endpoint URLs can contain credentials. An explicit host projector may
	// include an approved scope reference in its payload; metadata always omits it.
	record.Profile.Endpoint = ""
	if validationErr := s.validateProjection(record.Stage, projection); validationErr != nil {
		return s.failure(record, "projection", validationErr)
	}
	if contextErr := ctx.Err(); contextErr != nil {
		return s.failure(record, "canceled", contextErr)
	}
	record.Projection = projection
	record.Projection.Payload = bytes.Clone(projection.Payload)
	record.Projection.Metadata = CloneCaptureRecordMetadata(projection.Metadata)
	ack, err := s.binding.Sink.WriteCapture(ctx, record)
	if err != nil {
		return s.failure(record, "sink", err)
	}
	if (ack.Durable && !s.durable) || (s.requirement.Durable && !ack.Durable) {
		return s.failure(record, "acknowledgement", ErrCaptureUnsupported)
	}
	return s.observe(record, projection.Complete)
}

func (s *CaptureSession) project(ctx context.Context, record CaptureRecord) (CaptureProjection, error) {
	if s.binding.Projector == nil {
		var projection CaptureProjection
		projection.Accuracy, projection.Redacted = CaptureMetadata, true
		projection.Metadata = CloneCaptureRecordMetadata(record.Projection.Metadata)
		projection.Metadata.RequestID, projection.Metadata.ProviderResponseID = "", ""
		return projection, nil
	}
	record.Projection.Payload = bytes.Clone(record.Projection.Payload)
	record.Projection.Metadata = CloneCaptureRecordMetadata(record.Projection.Metadata)
	return s.binding.Projector.ProjectCapture(ctx, record)
}

func (s *CaptureSession) observe(record CaptureRecord, complete bool) error {
	s.mu.Lock()
	if !slices.Contains(s.report.Observed, record.Stage) {
		s.report.Observed = append(s.report.Observed, record.Stage)
	}
	s.report.Complete = s.report.Complete && complete
	if record.Stage == CaptureTerminal {
		s.report.Closed = true
		for _, requiredStage := range s.requirement.Stages {
			if !slices.Contains(s.report.Observed, requiredStage) {
				// Publish incompleteness under the same lock as Closed; a concurrent
				// Report reader must not see a transient successful terminal.
				s.report.Complete = false
				s.mu.Unlock()
				return s.failure(record, "missing_stage", ErrCaptureUnsupported)
			}
		}
	}
	s.mu.Unlock()
	return nil
}

func (s *CaptureSession) validateProjection(stage CaptureStage, projection CaptureProjection) error {
	if err := projection.Metadata.Validate(); err != nil {
		return err
	}
	rank := captureAccuracyRank(projection.Accuracy)
	minimum := 0
	if len(s.requirement.Stages) == 0 || slices.Contains(s.requirement.Stages, stage) {
		minimum = captureAccuracyRank(s.requirement.Accuracy)
	}
	if rank == 0 || rank > captureAccuracyRank(s.coverage.Stages[stage]) ||
		rank > captureAccuracyRank(s.projectorAccuracy) || rank < minimum ||
		len(projection.Payload) > s.binding.MaxBytes {
		return ErrCaptureUnsupported
	}
	if projection.Accuracy == CaptureBytes && (projection.Redacted || projection.Truncated || !projection.Complete) {
		return ErrCaptureUnsupported
	}
	if projection.Complete && (projection.Redacted || projection.Truncated) {
		return ErrCaptureUnsupported
	}
	if projection.Accuracy == CaptureSemantic &&
		(projection.MediaType != captureJSONMediaType || !json.Valid(projection.Payload)) {
		return ErrCaptureUnsupported
	}
	return nil
}

func (s *CaptureSession) failure(record CaptureRecord, kind string, cause error) error {
	s.mu.Lock()
	s.report.Complete = false
	s.report.Failures = append(s.report.Failures, CaptureFailure{
		Stage: record.Stage, AfterDispatch: record.AfterDispatch, Kind: kind,
	})
	s.mu.Unlock()
	requiredStage := slices.Contains(s.requirement.Stages, record.Stage)
	if kind == "missing_stage" || requiredStage || s.requirement.Durable ||
		s.requirement.PhysicalAttempts || (s.requirement.Accuracy != "" && len(s.requirement.Stages) == 0) {
		return &CaptureError{Stage: record.Stage, AfterDispatch: record.AfterDispatch, Cause: cause}
	}
	return nil
}

// Report returns a snapshot isolated from sink/projector mutations.
func (s *CaptureSession) Report() CaptureReport {
	if s == nil {
		return CaptureReport{}
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	out := s.report
	out.Complete = out.Complete && out.Closed
	return CloneCaptureReport(out)
}

// Enabled distinguishes the ordinary no-sink execution path without serialization.
func (s *CaptureSession) Enabled() bool { return s != nil && s.binding.Sink != nil }

// RecordFailure records projection-encoding or hook failure with safe classification.
func (s *CaptureSession) RecordFailure(stage CaptureStage, after bool, kind string, cause error) error {
	if !s.Enabled() {
		return nil
	}
	if !validCaptureStage(stage) || strings.TrimSpace(kind) == "" {
		return &CaptureError{Stage: stage, AfterDispatch: after, Cause: ErrCaptureUnsupported}
	}
	var record CaptureRecord
	record.Stage, record.AfterDispatch = stage, after
	return s.failure(record, kind, cause)
}

// CloneCaptureReport isolates all mutable diagnostic collections.
func CloneCaptureReport(report CaptureReport) CaptureReport {
	out := report
	out.Observed = slices.Clone(report.Observed)
	out.Failures = slices.Clone(report.Failures)
	out.Coverage = slices.Clone(report.Coverage)
	return out
}

// CloneCaptureBinding snapshots configuration; shared runtime ports remain caller-owned.
func CloneCaptureBinding(binding *CaptureBinding) *CaptureBinding {
	if binding == nil {
		return nil
	}
	out := *binding
	return &out
}

func captureAccuracyRank(accuracy CaptureAccuracy) int {
	switch accuracy {
	case CaptureMetadata:
		return captureRankMetadata
	case CaptureSemantic:
		return captureRankSemantic
	case CaptureBytes:
		return captureRankBytes
	default:
		return captureRankUnknown
	}
}

func validCaptureStage(stage CaptureStage) bool {
	switch stage {
	case CaptureExecution, CaptureTranslatedRequest, CaptureTransportRequest, CaptureTransportResponse,
		CaptureProviderResponse, CaptureProviderChunk, CaptureCanonicalParts, CaptureTerminal:
		return true
	default:
		return false
	}
}
