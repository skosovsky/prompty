package truncate

import (
	"context"
	"math"
	"reflect"

	"github.com/skosovsky/prompty"
)

// MessageEstimator estimates only canonical messages. Tools/schema/native
// framing/output reserve are not guaranteed by this coverage.
type MessageEstimator struct{ Counter prompty.TokenCounter }

func (m MessageEstimator) Estimate(ctx context.Context, execution *prompty.PromptExecution) (Estimate, error) {
	result := Estimate{Scope: ScopeMessages, Tokens: 0, Known: false}
	if execution == nil || nilPort(m.Counter) {
		return result, ErrInvalidEstimate
	}
	for _, message := range execution.Messages {
		if err := ctx.Err(); err != nil {
			return result, err
		}
		count, err := m.Counter.CountMessage(message)
		if err != nil {
			return result, err
		}
		if count < 0 || count > math.MaxInt-result.Tokens {
			return result, ErrInvalidEstimate
		}
		result.Tokens += count
	}
	result.Known = true
	return result, nil
}

func nilPort(port any) bool {
	if port == nil {
		return true
	}
	value := reflect.ValueOf(port)
	switch value.Kind() {
	case reflect.Chan, reflect.Func, reflect.Interface, reflect.Map, reflect.Pointer, reflect.Slice:
		return value.IsNil()
	case reflect.Invalid,
		reflect.Bool,
		reflect.Int,
		reflect.Int8,
		reflect.Int16,
		reflect.Int32,
		reflect.Int64,
		reflect.Uint,
		reflect.Uint8,
		reflect.Uint16,
		reflect.Uint32,
		reflect.Uint64,
		reflect.Uintptr,
		reflect.Float32,
		reflect.Float64,
		reflect.Complex64,
		reflect.Complex128,
		reflect.Array,
		reflect.String,
		reflect.Struct,
		reflect.UnsafePointer:
		fallthrough
	default:
		return false
	}
}

func estimate(ctx context.Context, estimator Estimator, execution *prompty.PromptExecution) (Estimate, error) {
	if err := ctx.Err(); err != nil {
		return Estimate{}, err
	}
	value, err := estimator.Estimate(ctx, execution.Clone())
	if err != nil {
		return value, err
	}
	if !value.Known || value.Tokens < 0 || (value.Scope != ScopeMessages && value.Scope != ScopeExecution) {
		return value, ErrInvalidEstimate
	}
	return value, ctx.Err()
}
