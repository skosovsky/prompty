package prompty

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestBatchHandleWireAndLineage(t *testing.T) {
	t.Parallel()
	// Arrange.
	digest, err := BatchExecutionDigest(SimplePrompt("document"))
	require.NoError(t, err)
	handle := BatchHandle{Format: BatchWireFormat, Provider: "fixture", Endpoint: "https://fixture.invalid",
		AccountBinding: "host", ProviderID: "remote", Status: BatchEnded, ProviderStatus: "completed",
		Resources: []BatchResourceRef{{Kind: "input", ProviderID: "file-input"}},
		Items: []BatchItemBinding{
			{ID: "retry", OriginalID: "original", AttemptID: "attempt-2", ParentAttemptID: "attempt-1",
				Identity: ProfileIdentity{
					Provider: "fixture",
					Endpoint: "https://fixture.invalid",
					Model:    "model",
				}, ExecutionDigest: digest},
		}}
	// Act.
	document, err := EncodeBatchHandle(handle)
	require.NoError(t, err)
	decoded, err := DecodeBatchHandle(document)
	// Assert.
	require.NoError(t, err)
	require.Equal(t, handle, decoded)
	cloned := CloneBatchHandle(handle)
	cloned.Items[0].ID = "changed"
	cloned.Resources[0].ProviderID = "changed"
	require.Equal(t, "retry", handle.Items[0].ID)
	require.Equal(t, "file-input", handle.Resources[0].ProviderID)
	for _, corrupt := range []string{
		strings.Replace(string(document), `"format":"prompty.batch"`, `"format":"old.batch"`, 1),
		strings.Replace(string(document), `"attempt_id":"attempt-2"`, `"attempt_id":"attempt-1"`, 1),
		strings.Replace(string(document), `"account_binding":"host"`, `"account_binding":" "`, 1),
		strings.Replace(string(document), `"provider_id":"file-input"`, `"provider_id":" "`, 1),
		strings.Replace(string(document), `"status":"ended"`, `"status":"ended","credentials":"secret"`, 1),
	} {
		_, err = DecodeBatchHandle([]byte(corrupt))
		require.Error(t, err)
	}
}

func TestBatchCompletenessRetainsCorrelationProblems(t *testing.T) {
	t.Parallel()
	// Arrange.
	handle := BatchHandle{Items: []BatchItemBinding{{ID: "one"}, {ID: "two"}, {ID: "three"}}}
	items := []BatchItemResult{
		{ID: "three", Outcome: BatchItemExpired},
		{ID: "one", Outcome: BatchItemSuccess},
		{ID: "one"},
		{ID: "other"},
	}
	// Act.
	report := ReconcileBatchResults(handle, items, true)
	// Assert.
	require.True(t, report.DownloadComplete)
	require.False(t, report.CorrelationComplete)
	require.Equal(t, []string{"two"}, report.MissingIDs)
	require.Equal(t, []string{"one"}, report.DuplicateIDs)
	require.Equal(t, []string{"other"}, report.UnexpectedIDs)
	require.Len(t, items, 4)
}
