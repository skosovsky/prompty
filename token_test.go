package prompty

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCharFallbackCounter_Count(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name     string
		cpt      int
		text     string
		want     int
		wantErr  bool
	}{
		{"empty default", 0, "", 0, false},
		{"empty cpt4", 4, "", 0, false},
		{"ASCII short default", 0, "hello", 2, false},           // 5 runes / 4 = 2
		{"ASCII short cpt4", 4, "hello", 2, false},
		{"ASCII exact", 4, "abcd", 1, false},
		{"ASCII longer", 4, "abcdefgh", 2, false},
		{"Cyrillic", 4, "привет", 2, false},                   // 6 runes
		{"Cyrillic cpt2", 2, "привет", 3, false},
		{"limit over len", 4, "hi", 1, false},
		{"unicode mixed", 4, "Hello 世界", 2, false},          // 8 runes, 8/4=2 tokens
		{"zero cpt uses 4", 0, "12345678", 2, false},
		{"negative cpt uses 4", -1, "1234", 1, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			c := &CharFallbackCounter{CharsPerToken: tt.cpt}
			got, err := c.Count(tt.text)
			if tt.wantErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestCharFallbackCounter_ZeroValue(t *testing.T) {
	t.Parallel()
	var c CharFallbackCounter
	n, err := c.Count("12345678")
	require.NoError(t, err)
	assert.Equal(t, 2, n)
}
