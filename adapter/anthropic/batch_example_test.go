package anthropic

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
)

func ExampleNewMessageBatches() {
	// Local SDK fixture; no live account/model support is inferred.
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
		if strings.HasSuffix(request.URL.Path, "/results") {
			_, _ = fmt.Fprintln(w, batchExpiredLine)
			_, _ = fmt.Fprintln(w, batchErrorLine)
			_, _ = fmt.Fprintln(w, batchSuccessLine)
			return
		}
		writeBatchFixture(w, "ended")
	}))
	defer server.Close()
	manager, input := batchFixtureManager(server), batchFixtureInput()
	submitted, err := manager.Submit(context.Background(), input)
	if err != nil {
		panic(err)
	}
	// Host stores handle/input, chooses when to retrieve and retries explicitly.
	result, err := manager.Results(context.Background(), *submitted.Handle, batchFixtureBindings(input))
	if err != nil {
		panic(err)
	}
	fmt.Println(result.Handle.Status, result.Completeness.CorrelationComplete)
	for _, item := range result.Items {
		fmt.Println(item.ID, item.Outcome)
	}
	// Output:
	// ended true
	// three expired
	// two error
	// one success
}
