package prompty

import (
	"bytes"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"sync"
	"testing"
)

func TestParsedMetadataNormalization(t *testing.T) {
	for _, tc := range []struct {
		name      string
		input     JSONDocument
		want      string
		nilOutput bool
	}{
		{name: "absent", nilOutput: true},
		{name: "zero length", input: JSONDocument{}, nilOutput: true},
		{name: "empty object", input: JSONDocument(` { } `), want: `{}`},
		{name: "canonical exact numbers", input: JSONDocument(` {"z":9007199254740993,"decimal":1.2300,"exponent":1e+100,"nested":{"items":[-0,18446744073709551615]},"a":"<tag>"} `), want: `{"a":"\u003ctag\u003e","decimal":1.2300,"exponent":1e+100,"nested":{"items":[-0,18446744073709551615]},"z":9007199254740993}`},
		{name: "duplicate keys", input: JSONDocument(`{"z":1,"z":2}`), want: `{"z":2}`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			// Arrange.
			tpl, err := NewChatPromptTemplate(
				[]MessageTemplate{{Role: RoleUser, Content: TextContent("constant"), Metadata: tc.input}},
			)
			if err != nil {
				t.Fatal(err)
			}
			// Act.
			result, err := tpl.renderTemplates(nil)
			// Assert.
			if err != nil {
				t.Fatal(err)
			}
			got := result.Messages[0].Metadata
			if string(got) != tc.want || (got == nil) != tc.nilOutput {
				t.Fatalf("metadata = %q (nil %v), want %q (nil %v)", got, got == nil, tc.want, tc.nilOutput)
			}
		})
	}
}

func TestParsedMetadataConstructorErrors(t *testing.T) {
	for _, tc := range []struct{ name, input, message string }{
		{"null", "null", "json document: expected an object, got null"},
		{"invalid", "{", "json document: invalid complete JSON document"},
		{"trailing", "{} {}", "json document: invalid complete JSON document"},
		{"array", "[]", "json document: decode map:"},
		{"scalar", "3", "json document: decode map:"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			// Arrange.
			messages := []MessageTemplate{
				{Role: RoleSystem, Content: TextContent("ok")},
				{Role: RoleUser, Content: TextContent("constant"), Metadata: JSONDocument(tc.input)},
			}
			// Act.
			tpl, err := NewChatPromptTemplate(messages)
			// Assert.
			if tpl != nil || !errors.Is(err, ErrTemplateParse) ||
				!strings.Contains(err.Error(), "message 1 metadata: "+tc.message) {
				t.Fatalf("unexpected result: %v, %v", tpl, err)
			}
		})
	}
	t.Run("content error precedes metadata", func(t *testing.T) {
		// Arrange.
		messages := []MessageTemplate{{Role: RoleUser, Content: TextContent("{{"), Metadata: JSONDocument("null")}}
		// Act.
		_, err := NewChatPromptTemplate(messages)
		// Assert.
		if !errors.Is(err, ErrTemplateParse) || !strings.Contains(err.Error(), "message 0 part 0:") {
			t.Fatalf("unexpected error: %v", err)
		}
	})
}

func TestParsedMetadataOwnership(t *testing.T) {
	// Arrange.
	source := JSONDocument(`{"nested":{"items":["original"]},"n":9007199254740993}`)
	want := []byte(`{"n":9007199254740993,"nested":{"items":["original"]}}`)
	tpl, err := NewChatPromptTemplate(
		[]MessageTemplate{{Role: RoleUser, Content: TextContent("constant"), Metadata: source}},
	)
	if err != nil {
		t.Fatal(err)
	}
	clone := CloneTemplate(tpl)
	render := func(template *ChatPromptTemplate) JSONDocument {
		t.Helper()
		result, err := template.renderTemplates(nil)
		if err != nil {
			t.Fatal(err)
		}
		return result.Messages[0].Metadata
	}
	first := render(tpl)
	second := render(clone)
	// Act: overwrite actual byte backing arrays, including public clone metadata.
	for _, doc := range []JSONDocument{source, tpl.Messages[0].Metadata, clone.Messages[0].Metadata} {
		for i := range doc {
			doc[i] = 'x'
		}
	}
	// Assert: both existing and subsequent output snapshots remain canonical.
	for _, doc := range []JSONDocument{first, second, render(tpl), render(clone)} {
		if !bytes.Equal(doc, want) {
			t.Fatalf("metadata changed: %q", doc)
		}
	}
	// Act: rendered documents are independently caller-owned.
	for i := range first {
		first[i] = 'a'
	}
	if !bytes.Equal(second, want) {
		t.Fatalf("first output mutation changed second output: %q", second)
	}
	for i := range second {
		second[i] = 'b'
	}
	// Assert.
	for _, template := range []*ChatPromptTemplate{tpl, clone} {
		if got := render(template); !bytes.Equal(got, want) {
			t.Fatalf("rendered mutation escaped: %q", got)
		}
	}
}

func TestParsedMetadataConcurrentRenderOwnership(t *testing.T) {
	// Arrange.
	tpl, err := NewChatPromptTemplate(
		[]MessageTemplate{
			{Role: RoleUser, Content: TextContent("constant"), Metadata: JSONDocument(`{"n":9007199254740993}`)},
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	templates := []*ChatPromptTemplate{tpl, CloneTemplate(tpl)}
	var workers sync.WaitGroup
	failures := make(chan error, 16)
	// Act.
	for range 16 {
		workers.Go(func() {
			for j := range 50 {
				result, err := templates[j%len(templates)].renderTemplates(nil)
				if err != nil {
					failures <- err
					return
				}
				doc := result.Messages[0].Metadata
				if string(doc) != `{"n":9007199254740993}` {
					failures <- fmt.Errorf("metadata changed: %q", doc)
					return
				}
				for k := range doc {
					doc[k] = 'x'
				}
			}
		})
	}
	workers.Wait()
	close(failures)
	// Assert.
	for err := range failures {
		t.Error(err)
	}
}

func benchmarkMetadataFixture(b *testing.B, keys int) []MessageTemplate {
	b.Helper()
	metadata := map[string]any{}
	for i := range keys {
		metadata[strconv.Itoa(i)] = "value"
	}
	doc, err := MapToJSONDocument(metadata)
	if err != nil {
		b.Fatal(err)
	}
	return []MessageTemplate{{Role: RoleUser, Content: TextContent("constant"), Metadata: doc}}
}

func BenchmarkWarmRenderMetadata(b *testing.B) {
	for _, keys := range []int{0, 16, 1024} {
		b.Run(strconv.Itoa(keys), func(b *testing.B) {
			// Arrange: fixture construction and encoding are outside the measured loop.
			tpl, err := NewChatPromptTemplate(benchmarkMetadataFixture(b, keys))
			if err != nil {
				b.Fatal(err)
			}
			b.ReportAllocs()
			for b.Loop() {
				// Act.
				result, err := tpl.renderTemplates(nil)
				// Assert.
				if err != nil || len(result.Messages) != 1 {
					b.Fatal(err)
				}
			}
		})
	}
}

func BenchmarkColdConstructionMetadata(b *testing.B) {
	for _, keys := range []int{0, 16, 1024} {
		b.Run(strconv.Itoa(keys), func(b *testing.B) {
			// Arrange: input JSON fixture only; canonical parsing belongs to construction.
			messages := benchmarkMetadataFixture(b, keys)
			b.ReportAllocs()
			for b.Loop() {
				// Act.
				tpl, err := NewChatPromptTemplate(messages)
				// Assert.
				if err != nil || len(tpl.Messages) != 1 {
					b.Fatal(err)
				}
			}
		})
	}
}
