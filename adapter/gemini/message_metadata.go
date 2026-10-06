package gemini

import (
	"bytes"
	"encoding/json"
	"math"
	"strings"

	"google.golang.org/genai"

	"github.com/skosovsky/prompty"
	"github.com/skosovsky/prompty/adapter"
)

// REST Gemini uses citationSources; Vertex and the SDK DTO use citations.
// Normalize only the decode copy. Annotations retain the original source value.
func normalizeCitationSource(payload []byte) ([]byte, error) {
	var root map[string]json.RawMessage
	if json.Unmarshal(payload, &root) != nil {
		return nil, prompty.ErrProviderState
	}
	if len(root["candidates"]) == 0 {
		return payload, nil
	}
	var candidates []map[string]json.RawMessage
	if json.Unmarshal(root["candidates"], &candidates) != nil {
		return nil, prompty.ErrProviderState
	}
	changed := false
	for _, candidate := range candidates {
		value, ok := candidate["citationMetadata"]
		if !ok || bytes.Equal(bytes.TrimSpace(value), []byte("null")) {
			continue
		}
		var metadata map[string]json.RawMessage
		if json.Unmarshal(value, &metadata) != nil {
			return nil, prompty.ErrProviderState
		}
		if sources, found := metadata["citationSources"]; found {
			if _, collision := metadata["citations"]; collision {
				return nil, prompty.ErrProviderState
			}
			delete(metadata, "citationSources")
			metadata["citations"] = sources
			candidate["citationMetadata"], _ = json.Marshal(metadata)
			changed = true
		}
	}
	if !changed {
		return payload, nil
	}
	root["candidates"], _ = json.Marshal(candidates)
	return json.Marshal(root)
}

func sourceCandidateAnnotations(payload []byte) ([]prompty.MessageAnnotation, error) {
	var source struct {
		Candidates []map[string]json.RawMessage `json:"candidates"`
	}
	if !json.Valid(payload) || uniqueGeminiJSONValue(json.NewDecoder(bytes.NewReader(payload))) != nil ||
		json.Unmarshal(payload, &source) != nil || len(source.Candidates) != 1 || source.Candidates[0] == nil {
		return nil, prompty.ErrProviderState
	}
	var annotations []prompty.MessageAnnotation
	for _, field := range []struct{ key, kind string }{
		{"citationMetadata", "candidate_citation_metadata"},
		{"groundingMetadata", "candidate_grounding_metadata"},
	} {
		for key := range source.Candidates[0] {
			if key != field.key && strings.EqualFold(key, field.key) {
				return nil, prompty.ErrProviderState
			}
		}
		value, found := source.Candidates[0][field.key]
		if !found || bytes.Equal(bytes.TrimSpace(value), []byte("null")) {
			continue
		}
		var object map[string]json.RawMessage
		if json.Unmarshal(value, &object) != nil || object == nil {
			return nil, prompty.ErrProviderState
		}
		annotations = append(annotations, prompty.MessageAnnotation{
			Kind: field.kind, Value: prompty.CloneJSONDocument(value),
		})
	}
	return annotations, nil
}

// Chunk indices refer to accumulated provider grounding chunks, never canonical
// Content. Byte segments are checked for shape, not bound to guessed parts.
func validateCandidateMetadata(candidate *genai.Candidate, previousChunks int) (int, error) {
	if err := validateCandidateMetadataShape(candidate); err != nil {
		return previousChunks, err
	}
	metadata := candidate.GroundingMetadata
	if metadata == nil {
		return previousChunks, nil
	}
	if previousChunks < 0 || len(metadata.GroundingChunks) > math.MaxInt-previousChunks {
		return previousChunks, adapter.ErrInvalidResponse
	}
	count := previousChunks + len(metadata.GroundingChunks)
	for _, support := range metadata.GroundingSupports {
		for _, index := range support.GroundingChunkIndices {
			if int(index) >= count {
				return previousChunks, adapter.ErrInvalidResponse
			}
		}
	}
	return count, nil
}

func validateCandidateMetadataShape(candidate *genai.Candidate) error {
	if candidate.CitationMetadata != nil {
		for _, citation := range candidate.CitationMetadata.Citations {
			if citation == nil || citation.StartIndex < 0 || citation.EndIndex < citation.StartIndex {
				return adapter.ErrInvalidResponse
			}
		}
	}
	metadata := candidate.GroundingMetadata
	if metadata == nil {
		return nil
	}
	for _, chunk := range metadata.GroundingChunks {
		if chunk == nil {
			return adapter.ErrInvalidResponse
		}
	}
	for _, support := range metadata.GroundingSupports {
		if support == nil ||
			len(support.ConfidenceScores) != 0 && len(support.ConfidenceScores) != len(support.GroundingChunkIndices) {
			return adapter.ErrInvalidResponse
		}
		for _, index := range support.GroundingChunkIndices {
			if index < 0 {
				return adapter.ErrInvalidResponse
			}
		}
		for _, score := range support.ConfidenceScores {
			if math.IsNaN(float64(score)) || math.IsInf(float64(score), 0) || score < 0 || score > 1 {
				return adapter.ErrInvalidResponse
			}
		}
		if segment := support.Segment; segment != nil &&
			(segment.PartIndex < 0 || segment.StartIndex < 0 || segment.EndIndex < segment.StartIndex) {
			return adapter.ErrInvalidResponse
		}
	}
	return nil
}
