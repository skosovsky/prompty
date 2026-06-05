package remoteregistry

import (
	"context"
	"fmt"
	"time"

	"github.com/skosovsky/prompty"
	"github.com/skosovsky/prompty/manifest"
)

// staticFetcher returns a fixed manifest for use in examples.
type staticFetcher struct {
	data map[string][]byte
}

func (s *staticFetcher) Fetch(_ context.Context, id string) ([]byte, error) {
	if d, ok := s.data[id]; ok {
		return d, nil
	}
	return nil, fmt.Errorf("%w: %s", ErrFetchFailed, "not found")
}

func ExampleRegistry_Plan() {
	manifestJSON := `{"id":"demo","version":"1","messages":[{"role":"system","content":[{"type":"text","text":"Hello {{ .Input.name }}"}]}]}`
	fetcher := &staticFetcher{data: map[string][]byte{"demo": []byte(manifestJSON)}}
	base, _ := New(fetcher, WithParser(manifest.NewJSONParser()))
	reg := WithCache(base, time.Minute)
	ctx := context.Background()
	input, err := prompty.PlanInputFrom(struct {
		Name string `prompt:"name"`
	}{Name: "Ada"})
	if err != nil {
		panic(err)
	}
	plan, err := reg.Plan(ctx, "demo", input)
	if err != nil {
		panic(err)
	}
	exec, err := plan.Execute(ctx)
	if err != nil {
		panic(err)
	}
	text := exec.Messages[0].Content[0].(prompty.TextPart).Text
	fmt.Println(text)
	// Output:
	// Hello Ada
}

func ExampleNew() {
	manifestJSON := `{"id":"demo","version":"1","messages":[{"role":"user","content":[{"type":"text","text":"Hi"}]}]}`
	fetcher := &staticFetcher{data: map[string][]byte{"demo": []byte(manifestJSON)}}
	base, _ := New(fetcher, WithParser(manifest.NewJSONParser()))
	reg := WithCache(base, 5*time.Minute)
	ctx := context.Background()
	tpl, err := templateFromPlan(ctx, reg, "demo")
	if err != nil {
		panic(err)
	}
	if len(tpl.Messages[0].Content) > 0 && tpl.Messages[0].Content[0].Type == "text" {
		fmt.Println(tpl.Messages[0].Content[0].Text)
	}
	// Output:
	// Hi
}
