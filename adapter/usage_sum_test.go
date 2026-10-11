package adapter

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/skosovsky/prompty"
)

func TestSumUsageCountersCheckedRange(t *testing.T) {
	t.Parallel()
	maximum := int64(int(^uint(0) >> 1))
	for _, test := range []struct {
		name    string
		values  []int64
		want    int
		invalid bool
	}{
		{"empty", nil, 0, false},
		{"known zero", []int64{0, 0}, 0, false},
		{"exact limit", []int64{maximum - 1, 1}, int(maximum), false},
		{"overflow", []int64{maximum, 1}, 0, true},
		{"positive wrap", []int64{maximum, maximum, 3}, 0, true},
		{"negative", []int64{1, -1}, 0, true},
	} {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			// Arrange: provider counts are int64; the canonical range is platform int.
			values := append([]int64(nil), test.values...)
			// Act.
			got, err := SumUsageCounters(values...)
			// Assert.
			if test.invalid {
				require.ErrorIs(t, err, prompty.ErrUsage)
			} else {
				require.NoError(t, err)
			}
			require.Equal(t, test.want, got)
			require.Equal(t, test.values, values)
		})
	}
}
