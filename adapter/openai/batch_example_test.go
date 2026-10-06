package openai

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
)

func ExampleNewBatches() {
	// Deterministic SDK/multipart/JSONL fixture; no live provider call.
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/v1/files":
			writeBatchUploadFixture(w)
		case "/v1/files/file-errors/content":
			_, _ = fmt.Fprintln(w, batchExpiredLine)
			_, _ = fmt.Fprintln(w, batchErrorLine)
		case "/v1/files/file-output/content":
			_, _ = fmt.Fprintln(w, batchSuccessLine)
		default:
			writeBatchFixture(w, "completed")
		}
	}))
	defer server.Close()
	// Host supplies bound SDK-backed files transport and batch-capable profiles.
	manager, input := batchFixtureManager(server), batchFixtureInput()
	submitted, err := manager.Submit(context.Background(), input)
	if err != nil {
		panic(err)
	}
	result, err := manager.Results(context.Background(), *submitted.Handle, batchFixtureBindings(input))
	if err != nil {
		panic(err)
	}
	fmt.Println(result.Handle.ProviderStatus, result.Completeness.CorrelationComplete)
	for _, item := range result.Items {
		fmt.Println(item.ID, item.Outcome)
	}
	// Output:
	// completed true
	// three expired
	// two error
	// one success
}
