package prompty

import (
	"errors"
	"strings"
)

// ErrCaptureReport identifies an inconsistent diagnostic snapshot, not a
// provider execution failure or an instruction to retry a dispatched request.
var ErrCaptureReport = errors.New("prompty: invalid capture report")

// Validate checks semantic consistency without inventing required stages,
// physical attempts or durability evidence absent from the report.
func (r CaptureReport) Validate() error {
	if strings.TrimSpace(r.InvocationID) == "" || (r.Complete && (!r.Closed || len(r.Failures) != 0)) {
		return ErrCaptureReport
	}
	covered := make(map[CaptureStage]bool)
	for _, coverage := range r.Coverage {
		if !validCaptureStage(coverage.Stage) || captureAccuracyRank(coverage.Accuracy) == 0 ||
			covered[coverage.Stage] {
			return ErrCaptureReport
		}
		covered[coverage.Stage] = true
	}
	observed := make(map[CaptureStage]bool)
	for _, stage := range r.Observed {
		if !covered[stage] || observed[stage] {
			return ErrCaptureReport
		}
		observed[stage] = true
	}
	if r.Closed != observed[CaptureTerminal] {
		return ErrCaptureReport
	}
	for _, failure := range r.Failures {
		if !validCaptureStage(failure.Stage) || strings.TrimSpace(failure.Kind) == "" {
			return ErrCaptureReport
		}
	}
	return nil
}
