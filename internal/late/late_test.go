package late

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestFilterEarlyRequired_DropsLateFields(t *testing.T) {
	t.Parallel()
	props := map[string]any{
		"early": map[string]any{"type": "string"},
		"late":  map[string]any{"type": "string", "late": true},
	}
	got := FilterEarlyRequired([]string{"early", "late"}, props)
	assert.Equal(t, []string{"early"}, got)
}
