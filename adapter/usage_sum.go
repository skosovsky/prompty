package adapter

import (
	"fmt"

	"github.com/skosovsky/prompty"
)

// SumUsageCounters adds provider counters without overflowing canonical int usage.
// Presence and provider-specific composition remain the caller's responsibility.
func SumUsageCounters(values ...int64) (int, error) {
	maximum := int64(int(^uint(0) >> 1))
	var total int64
	for _, value := range values {
		if value < 0 || value > maximum-total {
			return 0, fmt.Errorf("%w: counter sum outside canonical range", prompty.ErrUsage)
		}
		total += value
	}
	return int(total), nil
}
