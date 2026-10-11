package anthropic

import (
	"encoding/json"
	"strconv"
	"strings"
	"testing"

	sdk "github.com/anthropics/anthropic-sdk-go"
)

func BenchmarkTask45TextReconstruction(b *testing.B) {
	for _, segments := range []int{100, 1000} {
		b.Run(strconv.Itoa(segments), func(b *testing.B) {
			var event sdk.MessageStreamEventUnion
			raw := `{"type":"content_block_delta","index":0,"delta":{"type":"text_delta","text":"` + strings.Repeat(
				"x",
				1024,
			) + `"}}`
			if err := json.Unmarshal([]byte(raw), &event); err != nil {
				b.Fatal(err)
			}
			b.ReportAllocs()
			b.SetBytes(int64(segments * 1024))
			for b.Loop() {
				state := &messageStreamState{}
				block := &messageStreamBlock{
					kind:   "text",
					source: map[string]json.RawMessage{"text": json.RawMessage(`""`)},
				}
				for range segments {
					if _, err := state.delta(event, block); err != nil {
						b.Fatal(err)
					}
				}
				block.source["text"], _ = json.Marshal(block.text.String())
			}
		})
	}
}
