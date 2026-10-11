package prompty

import (
	"context"
	"encoding/json"
	"fmt"
	"iter"
	"maps"
	"strconv"
	"strings"
	"testing"
)

func BenchmarkTask45PlanCloneChain(b *testing.B) {
	for _, keys := range []int{16, 1024} {
		defaults := make(map[string]any, keys)
		for i := range keys {
			defaults[strconv.Itoa(i)] = map[string]any{"values": []any{1, "known", true}}
		}
		doc, err := json.Marshal(defaults)
		if err != nil {
			b.Fatal(err)
		}
		option, err := WithPartialVariablesJSON(doc)
		if err != nil {
			b.Fatal(err)
		}
		template, err := NewChatPromptTemplate(
			[]MessageTemplate{{Role: RoleUser, Content: TextContent("known")}},
			option,
		)
		if err != nil {
			b.Fatal(err)
		}
		plan := NewRenderPlan(template)
		for _, count := range []int{1, 8} {
			b.Run(fmt.Sprintf("keys=%d/with=%d", keys, count), func(b *testing.B) {
				benchmarkPlanCloneChain(b, plan, count)
			})
		}
	}
}

func BenchmarkTask45FrameDeliveryCopies(b *testing.B) {
	for _, observed := range []bool{false, true} {
		b.Run(strconv.FormatBool(observed), func(b *testing.B) {
			frame := &ResponseChunk{
				Kind:    StreamFinish,
				Outcome: OutcomeCompleted,
				Content: []ContentPart{
					MediaPart{MediaType: "image", MIMEType: "image/png", Data: make([]byte, 1<<20)},
				},
				Identity: StreamIdentity{PartIDs: []string{"known"}},
			}
			var options []StreamOption
			if observed {
				options = append(
					options,
					WithStreamFrameObserver(func(context.Context, *ResponseChunk) error { return nil }),
				)
			}
			b.ReportAllocs()
			for b.Loop() {
				stream := NewStream(
					context.Background(),
					StreamNative,
					func(context.Context) iter.Seq2[*ResponseChunk, error] {
						return func(yield func(*ResponseChunk, error) bool) { yield(frame, nil) }
					},
					options...)
				for _, err := range stream.Events() {
					if err != nil {
						b.Fatal(err)
					}
				}
				if stream.Status().State != StreamCompleted {
					b.Fatal("incomplete stream")
				}
			}
		})
	}
}

func BenchmarkTask45StructuredPreview(b *testing.B) {
	for _, size := range []int{1 << 20, 10 << 20} {
		b.Run(fmt.Sprintf("%dMiB", size>>20), func(b *testing.B) {
			document := `{"text":"` + strings.Repeat("x", size-64) + `"}`
			b.ReportAllocs()
			b.SetBytes(int64(len(document)))
			for b.Loop() {
				parser := newStructuredStreamParser[struct {
					Text string `json:"text"`
				}]()
				items, err := parser.feed(document)
				if err != nil {
					b.Fatal(err)
				}
				if err = parser.finish(); err != nil {
					b.Fatal(err)
				}
				if len(items) != 1 {
					b.Fatal("missing item")
				}
			}
		})
	}
}

func BenchmarkTask45BoundProjection(b *testing.B) {
	for _, size := range []int{16, 1024, 8192} {
		vars := make(map[string]any, size)
		for i := range size {
			vars[strconv.Itoa(i)] = i
		}
		for _, pooled := range []bool{false, true} {
			b.Run(fmt.Sprintf("keys=%d/pool=%v", size, pooled), func(b *testing.B) {
				benchmarkBoundProjection(b, vars, pooled)
			})
		}
	}
}

func BenchmarkTask45FrameFinalizations(b *testing.B) {
	for _, size := range []int{128, 1024} {
		b.Run(strconv.Itoa(size), func(b *testing.B) {
			parts := make([]ContentPart, size)
			ids := make([]string, size)
			for i := range size {
				parts[i] = TextPart{Text: "fixture"}
				ids[i] = strconv.Itoa(i)
			}
			b.ReportAllocs()
			for b.Loop() {
				response := NewResponse(parts)
				identity := StreamIdentity{PartIDs: append([]string(nil), ids...)}
				sealed := make(map[string]bool, size)
				for _, id := range ids {
					err := finalizeStreamParts(
						response,
						&identity,
						[]StreamPartFinalization{{PartID: id, Content: TextPart{Text: "fixture"}}},
						sealed,
					)
					if err != nil {
						b.Fatal(err)
					}
					sealed[id] = true
				}
			}
		})
	}
}

func benchmarkBoundProjection(b *testing.B, vars map[string]any, pooled bool) {
	b.Helper()
	b.ReportAllocs()
	for b.Loop() {
		if pooled {
			m := mergeBoundVarsWithPartials(nil, vars)
			if len(m) != len(vars) {
				b.Fatal("missing keys")
			}
			releaseBoundInputMap(m)
		} else {
			m := make(map[string]any, len(vars))
			maps.Copy(m, vars)
			if len(m) != len(vars) {
				b.Fatal("missing keys")
			}
		}
	}
}

func benchmarkPlanCloneChain(b *testing.B, plan *RenderPlan, count int) {
	b.Helper()
	b.ReportAllocs()
	for b.Loop() {
		candidate := plan
		for range count {
			next, err := candidate.WithLateInputJSON(JSONDocument(`{}`))
			if err != nil {
				b.Fatal(err)
			}
			candidate = next
		}
		if candidate == plan {
			b.Fatal("missing copy")
		}
	}
}
