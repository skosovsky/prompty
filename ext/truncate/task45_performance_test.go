package truncate

import (
	"context"
	"strconv"
	"strings"
	"testing"

	"github.com/skosovsky/prompty"
)

func BenchmarkTask45FullBudgetEstimation(b *testing.B) {
	for _, turns := range []int{16, 128} {
		b.Run(strconv.Itoa(turns), func(b *testing.B) {
			messages := []prompty.ChatMessage{prompty.NewSystemMessage("known policy")}
			for range turns {
				messages = append(
					messages,
					prompty.NewUserMessage(strings.Repeat("x", 1024)),
					prompty.NewAssistantMessage("known answer"),
				)
			}
			messages = append(messages, prompty.NewUserMessage("current"))
			execution := prompty.NewExecution(messages)
			options := budget(2)
			b.ReportAllocs()
			for b.Loop() {
				result, err := Apply(context.Background(), execution, options)
				if err != nil || len(result.Execution.Messages) != 2 {
					b.Fatalf("invalid reduction: %v", err)
				}
			}
		})
	}
}
