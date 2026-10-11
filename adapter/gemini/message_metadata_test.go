package gemini

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"
	"google.golang.org/genai"

	"github.com/skosovsky/prompty"
)

func TestNativeMessageMetadataLateOrderedExact(t *testing.T) {
	t.Parallel()
	// Arrange: real SDK source mapping, metadata after finish, global chunk refs.
	citations := `{"citationSources":[{"startIndex":0,"endIndex":2,"uri":"https://fixture.invalid","license":""}]}`
	first := `{"groundingChunks":[{"web":{"uri":"https://a.invalid","title":"a"}}],"webSearchQueries":["q"]}`
	second := `{"groundingChunks":[{"web":{"uri":"https://b.invalid"}}],"groundingSupports":[{"groundingChunkIndices":[0,1],"confidenceScores":[0.1234567890123456789,1],"segment":{"partIndex":0,"startIndex":0,"endIndex":2,"text":"é"}}]}`
	client, calls := geminiNativeFixture(t,
		`{"candidates":[{"content":{"parts":[{"text":"é"}]}}]}`,
		`{"candidates":[{"finishReason":"STOP"}]}`,
		`{"candidates":[{"citationMetadata":`+citations+`,"groundingMetadata":`+first+`}]}`,
		`{"candidates":[{"groundingMetadata":`+second+`}]}`,
		`{"usageMetadata":{"totalTokenCount":0}}`,
	)
	// Act.
	stream := client.ExecuteStream(t.Context(), prompty.SimplePrompt("fixture"), prompty.StreamNative)
	terminals, metadataFrames := 0, 0
	for event, err := range stream.Events() {
		require.NoError(t, err)
		if event.Result != nil {
			terminals++
		}
		if len(event.MessageAnnotations) != 0 {
			metadataFrames++
			require.Empty(t, event.Content)
			require.Empty(t, event.Identity.PartIDs)
			require.Empty(t, event.Annotations)
		}
	}
	status := stream.Status()
	// Assert: no fake content/binding or SDK float/zero omission in annotations.
	require.Equal(t, prompty.StreamCompleted, status.State)
	require.Equal(t, int32(1), calls.Load())
	require.Equal(t, 1, terminals)
	require.Equal(t, 2, metadataFrames)
	require.Equal(t, []prompty.ContentPart{prompty.TextPart{Text: "é"}}, status.Result.Content)
	require.Equal(t, []prompty.MessageAnnotation{
		{Kind: "candidate_citation_metadata", Value: prompty.JSONDocument(citations)},
		{Kind: "candidate_grounding_metadata", Value: prompty.JSONDocument(first)},
		{Kind: "candidate_grounding_metadata", Value: prompty.JSONDocument(second)},
	}, status.Result.MessageAnnotations)
	require.Equal(t, []prompty.UsageCounter{prompty.UsageTotal}, status.Result.Usage.Known)
	wire, err := prompty.MarshalResponse(status.Result, prompty.WirePolicy{State: prompty.StatePreserve})
	require.NoError(t, err)
	data, err := json.Marshal(wire)
	require.NoError(t, err)
	restored, err := prompty.DecodeResponseJSON(data)
	require.NoError(t, err)
	require.Equal(t, status.Result.MessageAnnotations, restored.MessageAnnotations)
}

func TestNativeMalformedMetadataRetainsEarlierUpdates(t *testing.T) {
	t.Parallel()
	for _, metadata := range []string{
		`"CitationMetadata":{"citationSources":[{"uri":"https://a.invalid"}]}`,
		`"GroundingMetadata":{"groundingChunks":[]}`,
		`"citationMetadata":{"citationSources":[null]}`,
		`"citationMetadata":{"citationSources":[{"startIndex":3,"endIndex":1}]}`,
		`"citationMetadata":{"citationSources":[],"citations":[]}`,
		`"groundingMetadata":{"groundingChunks":[null]}`,
		`"groundingMetadata":{"groundingSupports":[null]}`,
		`"groundingMetadata":{"groundingSupports":[{"groundingChunkIndices":[1]}]}`,
		`"groundingMetadata":{"groundingSupports":[{"groundingChunkIndices":[-1]}]}`,
		`"groundingMetadata":{"groundingSupports":[{"groundingChunkIndices":[0],"confidenceScores":[1,1]}]}`,
		`"groundingMetadata":{"groundingSupports":[{"groundingChunkIndices":[0],"confidenceScores":[2]}]}`,
		`"groundingMetadata":{"groundingSupports":[{"segment":{"partIndex":-1}}]}`,
		`"groundingMetadata":{"groundingSupports":[{"segment":{"startIndex":3,"endIndex":1}}]}`,
	} {
		t.Run(metadata, func(t *testing.T) {
			t.Parallel()
			// Arrange: previously accepted grounding/text cannot be lost on fault.
			client, calls := geminiNativeFixture(
				t,
				`{"candidates":[{"content":{"parts":[{"text":"partial"}]},"groundingMetadata":{"groundingChunks":[{"web":{"uri":"https://a.invalid"}}]}}]}`,
				`{"candidates":[{`+metadata+`}]}`,
				`{"candidates":[{"finishReason":"STOP"}]}`,
			)
			// Act.
			stream := client.ExecuteStream(t.Context(), prompty.SimplePrompt("fixture"), prompty.StreamNative)
			terminals := 0
			for event, err := range stream.Events() {
				if event.Result != nil {
					terminals++
					require.Error(t, err)
				}
			}
			// Assert.
			status := stream.Status()
			require.Equal(t, prompty.StreamFailed, status.State)
			require.Equal(t, int32(1), calls.Load())
			require.Equal(t, 1, terminals)
			require.Len(t, status.Result.Content, 1)
			require.Len(t, status.Result.MessageAnnotations, 1)
		})
	}
}

func TestSyncMetadataNeverBindsFirstPart(t *testing.T) {
	t.Parallel()
	// Arrange: two text parts; SDK metadata has no authoritative part index.
	a := New()
	metadata := &genai.CitationMetadata{Citations: []*genai.Citation{{URI: "https://fixture.invalid"}}}
	raw := &genai.GenerateContentResponse{Candidates: []*genai.Candidate{{
		Content:          &genai.Content{Parts: []*genai.Part{{Text: "first"}, {Text: "second"}}},
		CitationMetadata: metadata, FinishReason: genai.FinishReasonStop,
	}}}
	// Act.
	response, err := a.ParseResponse(raw)
	// Assert.
	require.NoError(t, err)
	require.Len(t, response.Content, 2)
	require.Empty(t, response.Annotations)
	require.Len(t, response.MessageAnnotations, 1)
	require.Equal(t, "candidate_citation_metadata", response.MessageAnnotations[0].Kind)
}
