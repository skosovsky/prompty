package prompty

import (
	"errors"
	"math"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestCheckedComposePathsAreAtomic(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name  string
		pairs []ComposeValue
	}{
		{"empty", []ComposeValue{ComposeBool("", true)}},
		{"leading", []ComposeValue{ComposeBool(".a", true)}},
		{"trailing", []ComposeValue{ComposeBool("a.", true)}},
		{"middle", []ComposeValue{ComposeBool("a..b", true)}},
		{"blank", []ComposeValue{ComposeBool("a. \t.b", true)}},
		{"duplicate", []ComposeValue{ComposeInt("a", 1), ComposeInt("a", 2)}},
		{"parent-first", []ComposeValue{ComposeInt("a", 1), ComposeInt("a.b", 2)}},
		{"child-first", []ComposeValue{ComposeInt("a.b", 2), ComposeInt("a", 1)}},
		{"nan", []ComposeValue{ComposeFloat("a", math.NaN())}},
		{"infinity", []ComposeValue{ComposeFloat("a", math.Inf(1))}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			// Arrange: valid prefix must not survive a rejected path/value.
			pairs := append([]ComposeValue{ComposeString("known.value", "kept")}, tc.pairs...)
			// Act.
			values, err := CheckedComposeValuesFromPairs(pairs...)
			// Assert.
			require.ErrorIs(t, err, ErrComposeValues)
			require.False(t, values.IsSet())
			_, exists := values.Lookup("known.value")
			require.False(t, exists)
			require.Panics(t, func() { NewComposeValuesFromPairs(pairs...) })
		})
	}
	// Valid controls: siblings are order-independent and empty values mean strict set.
	values, err := CheckedComposeValuesFromPairs(ComposeBool("a.b", true), ComposeInt("a.c", 2))
	require.NoError(t, err)
	value, ok := values.Lookup("a.c")
	require.True(t, ok)
	require.Equal(t, int64(2), value)
	empty, err := CheckedComposeValuesFromPairs()
	require.NoError(t, err)
	require.True(t, empty.IsSet())
}

type composeAdmissionFixture struct {
	err    error
	called bool
}

func (f *composeAdmissionFixture) ValidateComposeContext() error { return f.err }
func (f *composeAdmissionFixture) ComposeValues() ComposeValues {
	f.called = true
	return NewComposeValuesFromPairs(ComposeBool("feature.enabled", true))
}

func TestComposeContextAdmissionParity(t *testing.T) {
	t.Parallel()
	// Arrange: invalid typed context must fail before values are read in either API.
	sentinel := errors.New("invalid fixture context")
	for _, invalid := range []bool{false, true} {
		fixture := &composeAdmissionFixture{}
		if invalid {
			fixture.err = sentinel
		}
		// Act.
		input, planErr := PlanInputWithComposeContext(RegistryPlanInput{}, fixture)
		options, optionErr := PromptRuntimeOptionsFromComposeContext(fixture)
		// Assert.
		if invalid {
			require.ErrorIs(t, planErr, sentinel)
			require.ErrorIs(t, optionErr, sentinel)
			require.False(t, fixture.called)
			require.False(t, input.ComposeValues().IsSet())
			require.Empty(t, options.ComposeValues)
		} else {
			require.NoError(t, planErr)
			require.NoError(t, optionErr)
			require.True(t, fixture.called)
			require.True(t, input.ComposeValues().IsSet())
			require.JSONEq(t, `{"feature":{"enabled":true}}`, string(options.ComposeValues))
		}
	}
	input, err := PlanInputWithComposeContext(RegistryPlanInput{}, nil)
	require.NoError(t, err)
	require.False(t, input.ComposeValues().IsSet())
}
