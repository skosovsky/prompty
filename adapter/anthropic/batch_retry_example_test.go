package anthropic

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"

	"github.com/skosovsky/prompty"
)

func ExampleNewMessageBatches_failedOnlyRetry() {
	// Host policy, not a retry coordinator inside prompty. Local SDK fixture only.
	var submissions atomic.Int32
	requests := make(chan []string, 2)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
		if strings.HasSuffix(request.URL.Path, "/results") {
			_, _ = fmt.Fprintln(w, batchExpiredLine)
			_, _ = fmt.Fprintln(w, batchErrorLine)
			_, _ = fmt.Fprintln(w, batchSuccessLine)
			return
		}
		if request.Method == http.MethodGet {
			w.Header().Set("Content-Type", "application/json")
			_, _ = fmt.Fprint(
				w,
				`{"id":"msgbatch_1","type":"message_batch","processing_status":"ended","request_counts":{"processing":0,"succeeded":1,"errored":1,"expired":1,"canceled":0}}`,
			)
			return
		}
		var input struct {
			Requests []struct {
				ID string `json:"custom_id"`
			} `json:"requests"`
		}
		if err := json.NewDecoder(request.Body).Decode(&input); err != nil {
			http.Error(w, "invalid fixture request", http.StatusBadRequest)
			return
		}
		var ids []string
		for _, item := range input.Requests {
			ids = append(ids, item.ID)
		}
		requests <- ids
		id := submissions.Add(1)
		w.Header().Set("Content-Type", "application/json")
		_, _ = fmt.Fprintf(
			w,
			`{"id":"msgbatch_%d","type":"message_batch","processing_status":"in_progress","request_counts":{"processing":%d,"succeeded":0,"errored":0,"expired":0,"canceled":0}}`,
			id,
			len(ids),
		)
	}))
	defer server.Close()
	manager, input := batchFixtureManager(server), batchFixtureInput()
	ctx := context.Background()
	first, err := manager.Submit(ctx, input)
	if err != nil {
		// Unknown submit must be reconciled, not blindly retried.
		panic(err)
	}
	observed, err := manager.Get(ctx, *first.Handle)
	if err != nil {
		panic(err)
	}
	result, err := manager.Results(ctx, *observed.Handle, batchFixtureBindings(input))
	if err != nil || !result.Completeness.CorrelationComplete {
		panic("reconcile incomplete retrieval before choosing retries")
	}
	originals := make(map[string]*prompty.PromptExecution)
	for _, item := range input.Items {
		originals[item.ID] = item.Execution
	}
	retry := prompty.BatchSubmitRequest{}
	for _, item := range result.Items {
		// This host explicitly elects to retry error/expired, never success/unknown.
		if item.Outcome != prompty.BatchItemError && item.Outcome != prompty.BatchItemExpired {
			continue
		}
		retry.Items = append(retry.Items, prompty.BatchItem{
			ID: "retry-" + item.ID, OriginalID: item.Binding.OriginalID,
			AttemptID: "second", ParentAttemptID: item.Binding.AttemptID,
			Execution: originals[item.ID].Clone(),
		})
	}
	second, err := manager.Submit(ctx, retry)
	if err != nil {
		panic(err)
	}
	<-requests              // Original payload contained all three item IDs.
	fmt.Println(<-requests) // Retry HTTP payload excludes the successful item.
	fmt.Println(first.Handle.ProviderID, second.Handle.ProviderID)
	for _, item := range second.Handle.Items {
		fmt.Println(item.ID, item.OriginalID, item.AttemptID, item.ParentAttemptID)
	}
	// Output:
	// [retry-three retry-two]
	// msgbatch_1 msgbatch_2
	// retry-three three second first
	// retry-two two second first
}
