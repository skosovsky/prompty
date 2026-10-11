package openai

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"

	"github.com/skosovsky/prompty"
)

func ExampleHostedSearchItems() {
	// Deterministic SDK transport fixture; not a live model capability claim.
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, hostedOutputFixture)
	}))
	defer server.Close()
	// Bind a caller-owned profile declaring create/web-search/file-search.
	manager := hostedFixtureAdapter(server)
	result, err := manager.Create(context.Background(), prompty.RemoteResponseRequest{
		Execution: withResponseHostedTools(prompty.SimplePrompt("Find reference material"),
			[]prompty.HostedToolRequest{
				{Kind: HostedWebSearch},
				{Kind: HostedFileSearch, Settings: []byte(`{"vector_store_ids":["vs_fixture"]}`)},
			}), Store: new(false),
	})
	if err != nil {
		panic(err)
	}
	items, err := HostedSearchItems(result.Response)
	if err != nil {
		panic(err)
	}
	for _, item := range items {
		fmt.Println(item.Kind, item.OutputIndex, item.Coverage)
	}
	// Output:
	// web_search 0 available
	// file_search 2 available
}
