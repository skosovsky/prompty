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

func Test_CheckpointRecommend_TransitiveDigest(t *testing.T) {
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

	desc, err := CheckpointRecommend(
		context.Background(), "composed_main", reader,
	)
	require.NoError(t, err)
	assert.NotEqual(t, prompty.ManifestDigestSHA256(main), desc.Digest)

	err = CheckpointVerify(context.Background(), desc, reader)
	require.NoError(t, err)

	tampered := []byte(
		`{"id":"composed_child","layers":[{"id":"child_rules","role":"system",` +
			`"content":[{"type":"text","text":"tampered"}]}]}`,
	)
	reader.byID["composed_child"] = tampered
	err = CheckpointVerify(context.Background(), desc, reader)
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

	desc, err := CheckpointRecommend(
		context.Background(), "composed_main", reader,
	)
	require.NoError(t, err)
	assert.NotEqual(t, prompty.ManifestDigestSHA256(main), desc.Digest)

	reader.byID["composed_child"] = []byte(
		`{"id":"composed_child","layers":[{"id":"child_rules","role":"system",` +
			`"content":[{"type":"text","text":"tampered"}]}]}`,
	)
	err = CheckpointVerify(context.Background(), desc, reader)
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

	desc, err := CheckpointRecommend(context.Background(), "layers_only", reader)
	require.NoError(t, err)
	assert.NotEqual(t, prompty.ManifestDigestSHA256(layersOnly), desc.Digest)
	require.NoError(t, CheckpointVerify(context.Background(), desc, reader))
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
	_, err := CaptureSources(
		context.Background(),
		"composed_main",
		&digestFuncReader{read: read},
		NewJSONParser(),
		nil,
		nil,
	)
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

	desc, err := CheckpointRecommend(context.Background(), "composed_main", reader)
	require.NoError(t, err)
	require.NoError(t, CheckpointVerify(context.Background(), desc, reader))

	reader.byID["composed_child"] = []byte(
		`{"id":"composed_child","messages":[{"role":"system","content":[{"type":"text","text":"tampered"}]}]}`,
	)
	err = CheckpointVerify(context.Background(), desc, reader)
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

	desc, err := CheckpointRecommend(context.Background(), "composed_conditional_main", reader)
	require.NoError(t, err)

	tampered := []byte(`{"id":"composed_child","messages":[{"role":"system","content":[{"type":"text","text":"x"}]}]}`)
	reader.byID["composed_child"] = tampered
	err = CheckpointVerify(context.Background(), desc, reader)
	require.Error(t, err)
	assert.ErrorIs(t, err, prompty.ErrManifestDigestMismatch)
}

func TestRecommendManifestDescriptor_EmptyBytesFails(t *testing.T) {
	t.Parallel()
	reader := &stubComposeReader{byID: map[string][]byte{"empty": {}}}
	_, err := CheckpointRecommend(context.Background(), "empty", reader)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "empty")
}

func TestCheckpointRecommend_CorruptManifest(t *testing.T) {
	t.Parallel()
	reader := &stubComposeReader{byID: map[string][]byte{
		"bad": []byte(`{not-json`),
	}}
	_, err := CheckpointRecommend(context.Background(), "bad", reader)
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
		prompty.ManifestDescriptor{
			ID:     "bad",
			Digest: "source-bundle.v1:sha256:0000000000000000000000000000000000000000000000000000000000000000",
		},
		reader,
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
	_, err := CheckpointRecommend(context.Background(), "composed_main", reader)
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
	_, err := CheckpointRecommend(context.Background(), "composed_main", reader)
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

func (s *stubComposeReader) CaptureManifest(ctx context.Context, id string) (*prompty.ManifestSnapshot, error) {
	return CaptureSources(ctx, id, s, NewJSONParser(), nil, nil)
}
func (r *digestFuncReader) CaptureManifest(ctx context.Context, id string) (*prompty.ManifestSnapshot, error) {
	return CaptureSources(ctx, id, r, NewJSONParser(), nil, nil)
}
