package prompty

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
)

func BenchmarkTask45ToolAdmission(b *testing.B) {
	tool, err := NewTypedTool(
		"lookup",
		func(_ context.Context, args exampleLookupArgs) (int, error) { return len(args.Name), nil },
	)
	if err != nil {
		b.Fatal(err)
	}
	raw, err := json.Marshal(exampleLookupArgs{Name: strings.Repeat("x", 1024)})
	if err != nil {
		b.Fatal(err)
	}
	args := string(raw)
	for _, preflight := range []bool{false, true} {
		name := "invoke"
		if preflight {
			name = "batch-preflight-and-invoke"
		}
		b.Run(name, func(b *testing.B) {
			benchmarkToolAdmission(b, tool, args, preflight)
		})
	}
}

func benchmarkToolAdmission(b *testing.B, tool *TypedTool[exampleLookupArgs, int], args string, preflight bool) {
	b.Helper()
	b.ReportAllocs()
	for b.Loop() {
		if preflight {
			if err := tool.ValidateArgs(args); err != nil {
				b.Fatal(err)
			}
		}
		result, err := tool.Invoke(context.Background(), args)
		if err != nil || result != 1024 {
			b.Fatalf("invalid invocation: %v", err)
		}
	}
}
