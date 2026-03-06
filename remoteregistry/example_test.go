package remoteregistry

import (
	"context"
	"fmt"
	"time"
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

func ExampleRegistry_GetTemplate() {
	manifest := `
id: demo
version: "1"
messages:
  - role: system
    content: "Hello {{ .name }}"
`
	fetcher := &staticFetcher{data: map[string][]byte{"demo": []byte(manifest)}}
	reg := New(fetcher, WithTTL(time.Minute))
	ctx := context.Background()
	tpl, err := reg.GetTemplate(ctx, "demo")
	if err != nil {
		panic(err)
	}
	fmt.Println(tpl.Metadata.ID)
	fmt.Println(len(tpl.Messages))
	// Output:
	// demo
	// 1
}

func ExampleNew() {
	manifest := `
id: demo
version: "1"
messages:
  - role: user
    content: "Hi"
`
	fetcher := &staticFetcher{data: map[string][]byte{"demo": []byte(manifest)}}
	reg := New(fetcher, WithTTL(5*time.Minute))
	ctx := context.Background()
	tpl, err := reg.GetTemplate(ctx, "demo")
	if err != nil {
		panic(err)
	}
	if len(tpl.Messages[0].Content) > 0 && tpl.Messages[0].Content[0].Type == "text" {
		fmt.Println(tpl.Messages[0].Content[0].Text)
	}
	// Output:
	// Hi
}
