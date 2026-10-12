package prompty

import (
	"errors"
	"fmt"
	"slices"
	"testing"
)

type validationUnknownPart struct{ TextPart }

// rewriteValidationOracle is the previous validation implementation. Comparing
// first errors catches ordering changes across structural and policy failures.
func rewriteValidationOracle(messages []ChatMessage) error {
	return RewriteCachePolicies(
		slices.Clone(messages),
		func(_ string, _ *CachePolicy) (bool, error) { return true, nil },
	)
}

func TestValidateCachePoliciesReadOnlyCompatibility(t *testing.T) {
	// Arrange.
	bad := &CachePolicy{Mechanism: "unknown"}
	valid := &CachePolicy{Mechanism: CacheBlockMarker, Provider: "example", Controls: JSONDocument(`{"ttl":"1h"}`)}
	parts := []ContentPart{
		TextPart{Text: "text", CachePolicy: valid}, &TextPart{Text: "pointer", CachePolicy: valid},
		MediaPart{Data: []byte{1, 2, 3}, CachePolicy: valid}, &MediaPart{Data: []byte{4, 5, 6}, CachePolicy: valid},
		ReasoningPart{Text: "reason", CachePolicy: valid}, &ReasoningPart{Text: "pointer", CachePolicy: valid},
		ToolCallPart{ID: "id", CachePolicy: valid}, &ToolCallPart{ID: "pointer", CachePolicy: valid},
		ToolResultPart{CachePolicy: valid, Content: []ContentPart{TextPart{CachePolicy: valid}}},
		&ToolResultPart{CachePolicy: valid, Content: []ContentPart{&MediaPart{Data: []byte{7}, CachePolicy: valid}}},
		nil, (*TextPart)(nil), (*MediaPart)(nil), (*ReasoningPart)(nil), (*ToolCallPart)(nil), (*ToolResultPart)(nil),
		validationUnknownPart{}, TextPart{CachePolicy: bad},
		ToolResultPart{Content: []ContentPart{&ToolResultPart{Content: []ContentPart{TextPart{CachePolicy: bad}}}}},
	}
	policies := []*CachePolicy{
		nil,
		valid,
		bad,
		{Mechanism: CacheBlockMarker, Controls: JSONDocument(`{"ttl":"1h"}`)},
		{Mechanism: CacheBlockMarker, Provider: "example", Controls: JSONDocument(`null`)},
		{Mechanism: CacheBlockMarker, Provider: "example", Controls: JSONDocument(`[]`)},
		{Mechanism: CacheBlockMarker, Provider: "example", Controls: JSONDocument(`{`)},
		{Mechanism: CacheOmit},
		{Mechanism: CacheOmit, Mode: CacheRequired},
	}
	for i, part := range parts {
		for j, messagePolicy := range policies {
			for k, later := range parts {
				t.Run(fmt.Sprintf("part%d/policy%d/later%d", i, j, k), func(t *testing.T) {
					// Arrange: nested content also tests wrappers and invalid-policy ordering.
					messages := []ChatMessage{
						{
							CachePolicy: messagePolicy,
							Content:     []ContentPart{part, ToolResultPart{Content: []ContentPart{later}}},
						},
					}
					want := rewriteValidationOracle(messages)
					// Act.
					got := ValidateCachePolicies(messages)
					// Assert.
					if fmt.Sprint(got) != fmt.Sprint(want) ||
						errors.Is(got, ErrCachePolicy) != errors.Is(want, ErrCachePolicy) {
						t.Fatalf("got %v; want %v", got, want)
					}
					var gotCache, wantCache *CacheError
					if errors.As(got, &gotCache) != errors.As(want, &wantCache) ||
						!sameValidationCacheError(gotCache, wantCache) {
						t.Fatalf("cache errors differ: got %#v; want %#v", gotCache, wantCache)
					}
				})
			}
		}
	}
}

func TestValidateCachePoliciesDoesNotMutateInput(t *testing.T) {
	// Arrange: retain every mutable reference and pointer representation.
	controls := JSONDocument(`{"ttl":"1h"}`)
	policy := &CachePolicy{Mechanism: CacheBlockMarker, Provider: "example", Controls: controls}
	media := &MediaPart{Data: []byte{1, 2, 3}, CachePolicy: policy}
	nested := &ToolResultPart{CachePolicy: policy, Content: []ContentPart{media}}
	messages := []ChatMessage{{CachePolicy: policy, Content: []ContentPart{nested}}}
	originalControls := slices.Clone(controls)
	originalData := slices.Clone(media.Data)
	// Act.
	err := ValidateCachePolicies(messages)
	// Assert.
	if err != nil {
		t.Fatal(err)
	}
	if messages[0].Content[0] != nested || nested.Content[0] != media || messages[0].CachePolicy != policy ||
		media.CachePolicy != policy {
		t.Fatal("validation replaced caller-owned content or policies")
	}
	if !slices.Equal(media.Data, originalData) || !slices.Equal(controls, originalControls) {
		t.Fatal("validation mutated payload or controls")
	}
}

func TestValidateCachePoliciesNestedControlsAndPath(t *testing.T) {
	for _, controls := range []JSONDocument{JSONDocument(`null`), JSONDocument(`[]`), JSONDocument(`{`), JSONDocument(`{"ttl":"1h"}`)} {
		t.Run(string(controls), func(t *testing.T) {
			// Arrange: a nil message policy must still validate nested controls.
			policy := &CachePolicy{Mechanism: CacheBlockMarker, Provider: "example", Controls: controls}
			messages := []ChatMessage{
				{
					Content: []ContentPart{
						&ToolResultPart{
							Content: []ContentPart{
								ToolResultPart{
									Content: []ContentPart{&MediaPart{Data: []byte{1}, CachePolicy: policy}},
								},
							},
						},
					},
				},
			}
			want := rewriteValidationOracle(messages)
			// Act.
			got := ValidateCachePolicies(messages)
			// Assert.
			if fmt.Sprint(got) != fmt.Sprint(want) {
				t.Fatalf("got %v; want %v", got, want)
			}
			if string(controls) == `{"ttl":"1h"}` {
				if got != nil {
					t.Fatal(got)
				}
				return
			}
			var cacheErr *CacheError
			if !errors.As(got, &cacheErr) ||
				cacheErr.Decision.Path != "/messages/0/content/0/content/0/content/0/cache_policy" {
				t.Fatalf("wrong nested cache error: %v", got)
			}
		})
	}
}

func BenchmarkValidateCachePoliciesMedia(b *testing.B) {
	for _, size := range []int{1 << 20, 8 << 20} {
		for _, kind := range []string{"no_policy", "mixed", "nested"} {
			b.Run(fmt.Sprintf("%s/%dMiB", kind, size>>20), func(b *testing.B) {
				// Arrange: construct fixture before measurement, never move invocation work here.
				media := MediaPart{Data: make([]byte, size)}
				content := []ContentPart{media}
				policy := &CachePolicy{Mechanism: CacheBlockMarker}
				switch kind {
				case "mixed":
					content = []ContentPart{TextPart{CachePolicy: policy}, &media, ReasoningPart{}, ToolCallPart{}}
				case "nested":
					content = []ContentPart{
						&ToolResultPart{
							CachePolicy: policy,
							Content: []ContentPart{
								ToolResultPart{Content: []ContentPart{media, TextPart{CachePolicy: policy}}},
							},
						},
					}
				}
				messages := []ChatMessage{{Content: content}}
				b.ReportAllocs()
				b.ResetTimer()
				// Act and assert.
				for b.Loop() {
					if err := ValidateCachePolicies(messages); err != nil {
						b.Fatal(err)
					}
				}
			})
		}
	}
}

func sameValidationCacheError(a, b *CacheError) bool {
	if a == nil || b == nil {
		return a == b
	}
	return a.Decision == b.Decision && errors.Is(a.Cause, b.Cause)
}
