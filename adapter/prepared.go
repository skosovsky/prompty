package adapter

import (
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/skosovsky/prompty"
)

// PreparedRequest binds a translated SDK request to its immutable checked profile.
// Its payload is opaque and used only by the high-level executor.
type PreparedRequest[Req any] struct {
	request        Req
	diagnostics    prompty.ExecutionDiagnostics
	schemas        preparedSchemas
	capture        *prompty.CaptureSession
	execution      *prompty.PromptExecution
	stateExecution *prompty.PromptExecution
	cacheResource  *prompty.CacheResourceHandle
	dispatched     bool
}

// Diagnostics returns a copy of the pre-dispatch decision.
func (p *PreparedRequest[Req]) Diagnostics() prompty.ExecutionDiagnostics {
	report := prompty.CloneExecutionDiagnostics(p.diagnostics)
	if p.capture.Enabled() {
		capture := p.capture.Report()
		report.Capture = &capture
	}
	return report
}

// Prepare validates a cloned execution before translation or transport dispatch.
// SDK Execute is a low-level transport primitive and does not run this gate.
func Prepare[Req, Resp any](
	adp ProviderAdapter[Req, Resp],
	exec *prompty.PromptExecution,
	mode prompty.StreamMode,
) (*PreparedRequest[Req], error) {
	if err := ValidateExecution(exec); err != nil {
		return nil, err
	}
	if mode != "" && mode != prompty.StreamNative && mode != prompty.StreamBuffered {
		return nil, prompty.ErrStreamMode
	}
	describer, ok := any(adp).(RuntimeDescriber)
	if !ok {
		return nil, ErrNoRuntimeDescriptor
	}
	desc := CloneRuntimeDescriptor(describer.RuntimeDescriptor())
	if strings.TrimSpace(desc.Name) == "" || strings.TrimSpace(desc.Endpoint) == "" ||
		strings.TrimSpace(desc.DefaultModel) == "" {
		return nil, fmt.Errorf("%w: provider, endpoint and default model required", ErrNoRuntimeDescriptor)
	}
	working := exec.Clone()
	_, hasNative := any(adp).(StreamerAdapter[Req])
	report, err := Preflight(desc, working, mode, hasNative)
	if err != nil {
		return nil, err
	}
	if mappingErr := validateToolMappingPorts(adp, working, mode, report); mappingErr != nil {
		return nil, mappingErr
	}
	if cacheErr := validateAdapterCacheResource(adp, working, report.Profile); cacheErr != nil {
		return nil, cacheErr
	}
	if stateErr := prompty.ValidateContinuation(working.Messages, report.Profile, time.Now()); stateErr != nil {
		return nil, stateErr
	}
	if stateErr := validateAdapterState(adp, working, report.Profile); stateErr != nil {
		return nil, stateErr
	}
	capture, err := prepareCapture(adp, working, mode, report.Profile)
	if err != nil {
		return nil, err
	}
	var original *prompty.PromptExecution
	if capture.Enabled() {
		original = working.Clone()
	}
	_, cacheProfile, err := resolveProfile(desc, working)
	if err != nil {
		return nil, err
	}
	cacheProfile.Identity = report.Profile
	report.Cache, err = prepareCache(adp, working, cacheProfile, cacheCombinationFeatures(desc, working, mode))
	if err != nil {
		return nil, err
	}
	schemas, err := compilePreparedSchemas(adp, working, report.Profile)
	if err != nil {
		return nil, err
	}
	report.Schemas = schemas.reports()
	stateExecution := snapshotDispatchState(working)
	req, err := adp.Translate(working)
	if err != nil {
		return nil, err
	}
	req, err = prepareNativeRequest(adp, req, mode)
	if err != nil {
		return nil, err
	}
	return &PreparedRequest[Req]{
		request: req, diagnostics: report, schemas: schemas, capture: capture, execution: original,
		stateExecution: stateExecution,
		cacheResource:  clonePreparedCacheResource(working.CacheResource), dispatched: false,
	}, nil
}

func snapshotDispatchState(exec *prompty.PromptExecution) *prompty.PromptExecution {
	for _, message := range exec.Messages {
		if len(message.ProviderState) != 0 || message.ContinuationUnavailable {
			return exec.Clone()
		}
	}
	return nil
}

func validatePreparedDispatch[Req any](adp any, prepared *PreparedRequest[Req]) error {
	if err := validateCacheResourceBinding(adp, prepared.cacheResource, prepared.diagnostics.Profile); err != nil {
		return err
	}
	return ValidateDispatchState(adp, prepared.stateExecution, prepared.diagnostics.Profile)
}

// ValidateDispatchState rechecks an already-prepared immutable input immediately
// before generation/job dispatch, after host callbacks or upload. It performs no
// generation preparation, network operation, renewal or retry. Results-only
// reconciliation must not call this generation-lease gate.
func ValidateDispatchState(adp any, exec *prompty.PromptExecution, destination prompty.ProfileIdentity) error {
	if exec == nil {
		return nil
	}
	isolated := exec.Clone()
	if err := prompty.ValidateContinuation(isolated.Messages, destination, time.Now()); err != nil {
		return err
	}
	return validateAdapterState(adp, isolated, destination)
}

func prepareNativeRequest[Req any](adp any, req Req, mode prompty.StreamMode) (Req, error) {
	if mode == prompty.StreamNative {
		if preparer, ok := adp.(NativeRequestPreparer[Req]); ok {
			return preparer.PrepareNativeRequest(req)
		}
	}
	return req, nil
}

func validateAdapterState(adp any, exec *prompty.PromptExecution, destination prompty.ProfileIdentity) error {
	for _, message := range exec.Messages {
		if len(message.ProviderState) == 0 {
			continue
		}
		stateful, ok := adp.(ProviderStateAdapter)
		if !ok {
			return fmt.Errorf("%w: adapter has no state translator", prompty.ErrProviderState)
		}
		return stateful.ValidateProviderState(exec, destination)
	}
	return nil
}

func validateAdapterCacheResource(adp any, exec *prompty.PromptExecution, destination prompty.ProfileIdentity) error {
	return validateCacheResourceBinding(adp, exec.CacheResource, destination)
}

func clonePreparedCacheResource(handle *prompty.CacheResourceHandle) *prompty.CacheResourceHandle {
	if handle == nil {
		return nil
	}
	cloned := *handle
	return &cloned
}

func validateCacheResourceBinding(
	adp any,
	handle *prompty.CacheResourceHandle,
	destination prompty.ProfileIdentity,
) error {
	if handle == nil {
		return nil
	}
	mapper, exists := adp.(CacheResourceAdapter)
	if !exists {
		return fmt.Errorf("%w: adapter has no cache resource mapping", ErrUnsupportedCapability)
	}
	return mapper.ValidateCacheResource(*handle, destination)
}

func validateForcedToolAdapter(adp any, report prompty.ExecutionDiagnostics, required []string) error {
	if !slices.Contains(required, prompty.CapabilityForcedToolSelection) {
		return nil
	}
	selection, exists := adp.(ForcedToolAdapter)
	if exists && selection.SupportsForcedTool() {
		return nil
	}
	report = prompty.CloneExecutionDiagnostics(report)
	for index := range report.Checks {
		if report.Checks[index].Requirement == prompty.CapabilityForcedToolSelection {
			report.Checks[index].Status = prompty.CapabilityUnsupported
			report.Checks[index].Reason = "adapter has no native named-tool selection mapping"
		}
	}
	return &CapabilityError{Diagnostics: report, Cause: ErrUnsupportedCapability}
}

func validateToolMappingPorts(
	adp any,
	exec *prompty.PromptExecution,
	mode prompty.StreamMode,
	report prompty.ExecutionDiagnostics,
) error {
	if exec.Requirements.OutputMode == prompty.OutputSyntheticTool {
		mapper, exists := adp.(SyntheticOutputAdapter)
		if !exists || !mapper.SupportsSyntheticOutput() {
			return fmt.Errorf("%w: adapter has no synthetic output mapping", ErrUnsupportedCapability)
		}
	}
	if err := validateForcedToolAdapter(adp, report, requiredCapabilities(exec, mode)); err != nil {
		return err
	}
	if slices.Contains(exec.Requirements.Capabilities, prompty.CapabilityToolNameCorrelation) {
		correlator, exists := adp.(ToolNameCorrelationAdapter)
		if !exists || !correlator.SupportsToolNameCorrelation() {
			return fmt.Errorf("%w: adapter has no tool-name correlation mapping", ErrUnsupportedCapability)
		}
	}
	return nil
}
