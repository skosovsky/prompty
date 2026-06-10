package manifest

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/skosovsky/prompty"
)

type stubComposeReader struct {
	byID map[string][]byte
}

func (s *stubComposeReader) ReadManifestBytes(_ context.Context, id string) ([]byte, error) {
	return s.byID[id], nil
}

func (s *stubComposeReader) LoadByID(_ context.Context, id string) (*RawManifest, error) {
	data := s.byID[id]
	var raw RawManifest
	if err := NewJSONParser().Unmarshal(data, &raw); err != nil {
		return nil, err
	}
	return &raw, nil
}

func Test_newComposeManifestDescriptor_TransitiveDigest(t *testing.T) {
	t.Parallel()
	main := []byte(
		`{"id":"composed_main","imports":[{"id":"composed_child"}],` +
			`"layers":[{"id":"base_system","role":"system","content":[{"type":"text","text":"base"}]},` +
			`{"id":"child_rules","import_ref":"composed_child"},` +
			`{"id":"user_turn","role":"user","content":[{"type":"text","text":"hi"}]}]}`,
	)
	child := []byte(
		`{"id":"composed_child","layers":[{"id":"child_rules","role":"system",` +
			`"content":[{"type":"text","text":"child"}]}]}`,
	)
	reader := &stubComposeReader{byID: map[string][]byte{
		"composed_main":  main,
		"composed_child": child,
	}}
	parser := NewJSONParser()

	desc, err := newComposeManifestDescriptor(
		context.Background(), "composed_main", reader, parser,
	)
	require.NoError(t, err)
	assert.NotEqual(t, prompty.ManifestDigestSHA256(main), desc.Digest)

	err = verifyComposeManifestDescriptor(context.Background(), desc, reader, parser)
	require.NoError(t, err)

	tampered := []byte(
		`{"id":"composed_child","layers":[{"id":"child_rules","role":"system",` +
			`"content":[{"type":"text","text":"tampered"}]}]}`,
	)
	reader.byID["composed_child"] = tampered
	err = verifyComposeManifestDescriptor(context.Background(), desc, reader, parser)
	require.Error(t, err)
	assert.ErrorIs(t, err, prompty.ErrManifestDigestMismatch)
}

func TestRecommendManifestDescriptor_SelectsCompose(t *testing.T) {
	t.Parallel()
	main := []byte(
		`{"id":"composed_main","imports":[{"id":"composed_child"}],` +
			`"layers":[{"id":"base_system","role":"system","content":[{"type":"text","text":"base"}]},` +
			`{"id":"child_rules","import_ref":"composed_child"},` +
			`{"id":"user_turn","role":"user","content":[{"type":"text","text":"hi"}]}]}`,
	)
	child := []byte(
		`{"id":"composed_child","layers":[{"id":"child_rules","role":"system",` +
			`"content":[{"type":"text","text":"child"}]}]}`,
	)
	reader := &stubComposeReader{byID: map[string][]byte{
		"composed_main":  main,
		"composed_child": child,
	}}
	parser := NewJSONParser()

	desc, err := recommendManifestDescriptor(
		context.Background(), "composed_main", reader, parser,
	)
	require.NoError(t, err)
	assert.NotEqual(t, prompty.ManifestDigestSHA256(main), desc.Digest)

	reader.byID["composed_child"] = []byte(
		`{"id":"composed_child","layers":[{"id":"child_rules","role":"system",` +
			`"content":[{"type":"text","text":"tampered"}]}]}`,
	)
	err = verifyManifestDescriptor(context.Background(), desc, reader, parser)
	require.Error(t, err)
	assert.ErrorIs(t, err, prompty.ErrManifestDigestMismatch)
}

func TestRecommendManifestDescriptor_LayersOnlyUsesComposeDigest(t *testing.T) {
	t.Parallel()
	layersOnly := []byte(
		`{"id":"layers_only","layers":[{"id":"sys","role":"system",` +
			`"content":[{"type":"text","text":"hi"}]}]}`,
	)
	reader := &stubComposeReader{byID: map[string][]byte{"layers_only": layersOnly}}
	parser := NewJSONParser()

	desc, err := recommendManifestDescriptor(context.Background(), "layers_only", reader, parser)
	require.NoError(t, err)
	assert.NotEqual(t, prompty.ManifestDigestSHA256(layersOnly), desc.Digest)
	require.NoError(t, verifyManifestDescriptor(context.Background(), desc, reader, parser))
}

func TestComposeClosureDigestSHA256_CorruptChildUnmarshalFails(t *testing.T) {
	t.Parallel()
	main := []byte(`{"id":"composed_main","imports":[{"id":"composed_child"}]}`)
	corruptChild := []byte(`{not-json`)
	read := func(id string) ([]byte, error) {
		switch id {
		case "composed_main":
			return main, nil
		case "composed_child":
			return corruptChild, nil
		default:
			return nil, prompty.ErrTemplateNotFound
		}
	}
	_, err := ComposeClosureDigestSHA256("composed_main", read, NewJSONParser())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "unmarshal import")
}

func TestVerifyManifestDescriptor_ComposeTamperFails(t *testing.T) {
	t.Parallel()
	main := []byte(
		`{"id":"composed_main","imports":[{"id":"composed_child"}],` +
			`"messages":[{"role":"user","content":[{"type":"text","text":"hi"}]}]}`,
	)
	child := []byte(`{"id":"composed_child","messages":[{"role":"system","content":[{"type":"text","text":"child"}]}]}`)
	reader := &stubComposeReader{byID: map[string][]byte{
		"composed_main":  main,
		"composed_child": child,
	}}
	parser := NewJSONParser()

	desc, err := recommendManifestDescriptor(context.Background(), "composed_main", reader, parser)
	require.NoError(t, err)
	require.NoError(t, verifyManifestDescriptor(context.Background(), desc, reader, parser))

	reader.byID["composed_child"] = []byte(
		`{"id":"composed_child","messages":[{"role":"system","content":[{"type":"text","text":"tampered"}]}]}`,
	)
	err = verifyManifestDescriptor(context.Background(), desc, reader, parser)
	require.Error(t, err)
	assert.ErrorIs(t, err, prompty.ErrManifestDigestMismatch)
}

func TestComposeClosureDigest_InactiveImportStillInClosure(t *testing.T) {
	t.Parallel()
	main := []byte(
		`{"id":"composed_conditional_main","imports":[{"id":"composed_child",` +
			`"condition":{"match":{"capabilities.workspace_enabled":true}}}],` +
			`"messages":[{"role":"user","content":[{"type":"text","text":"hi"}]}]}`,
	)
	child := []byte(`{"id":"composed_child","messages":[{"role":"system","content":[{"type":"text","text":"child"}]}]}`)
	reader := &stubComposeReader{byID: map[string][]byte{
		"composed_conditional_main": main,
		"composed_child":            child,
	}}
	parser := NewJSONParser()

	desc, err := newComposeManifestDescriptor(context.Background(), "composed_conditional_main", reader, parser)
	require.NoError(t, err)

	tampered := []byte(`{"id":"composed_child","messages":[{"role":"system","content":[{"type":"text","text":"x"}]}]}`)
	reader.byID["composed_child"] = tampered
	err = verifyComposeManifestDescriptor(context.Background(), desc, reader, parser)
	require.Error(t, err)
	assert.ErrorIs(t, err, prompty.ErrManifestDigestMismatch)
}

func TestRecommendManifestDescriptor_EmptyBytesFails(t *testing.T) {
	t.Parallel()
	reader := &stubComposeReader{byID: map[string][]byte{"empty": {}}}
	parser := NewJSONParser()
	_, err := recommendManifestDescriptor(context.Background(), "empty", reader, parser)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "empty")
}

func TestCheckpointRecommend_CorruptManifest(t *testing.T) {
	t.Parallel()
	reader := &stubComposeReader{byID: map[string][]byte{
		"bad": []byte(`{not-json`),
	}}
	_, err := CheckpointRecommend(context.Background(), "bad", reader, NewJSONParser())
	require.Error(t, err)
	assert.ErrorIs(t, err, prompty.ErrInvalidManifest)
}

func TestCheckpointVerify_CorruptManifest(t *testing.T) {
	t.Parallel()
	reader := &stubComposeReader{byID: map[string][]byte{
		"bad": []byte(`{not-json`),
	}}
	err := CheckpointVerify(
		context.Background(),
		prompty.ManifestDescriptor{ID: "bad", Digest: "deadbeef"},
		reader,
		NewJSONParser(),
	)
	require.Error(t, err)
	assert.ErrorIs(t, err, prompty.ErrInvalidManifest)
}

func TestCheckpointRecommend_ComposeCorruptChild(t *testing.T) {
	t.Parallel()
	main := []byte(`{"id":"composed_main","imports":[{"id":"composed_child"}]}`)
	corruptChild := []byte(`{not-json`)
	read := func(id string) ([]byte, error) {
		switch id {
		case "composed_main":
			return main, nil
		case "composed_child":
			return corruptChild, nil
		default:
			return nil, prompty.ErrTemplateNotFound
		}
	}
	reader := &digestFuncReader{read: read}
	_, err := CheckpointRecommend(context.Background(), "composed_main", reader, NewJSONParser())
	require.Error(t, err)
	require.ErrorIs(t, err, prompty.ErrInvalidManifest)
	require.Contains(t, err.Error(), "unmarshal import")
}

func TestCheckpointRecommend_ComposeMissingImport(t *testing.T) {
	t.Parallel()
	main := []byte(`{"id":"composed_main","imports":[{"id":"missing_child"}]}`)
	read := func(id string) ([]byte, error) {
		if id == "composed_main" {
			return main, nil
		}
		return nil, prompty.ErrTemplateNotFound
	}
	reader := &digestFuncReader{read: read}
	_, err := CheckpointRecommend(context.Background(), "composed_main", reader, NewJSONParser())
	require.Error(t, err)
	require.ErrorIs(t, err, prompty.ErrTemplateNotFound)
	require.Contains(t, err.Error(), "read import")
}

type digestFuncReader struct {
	read func(id string) ([]byte, error)
}

func (r *digestFuncReader) ReadManifestBytes(_ context.Context, id string) ([]byte, error) {
	return r.read(id)
}
