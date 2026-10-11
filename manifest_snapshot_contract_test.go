package prompty

import (
	"context"
	"encoding/json"
	"os"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func snapshotFixtureMaterializer(
	_ context.Context,
	bundle SourceBundle,
	_ RegistryPlanInput,
) (*ChatPromptTemplate, error) {
	return NewChatPromptTemplate(
		[]MessageTemplate{{Role: RoleUser, Content: TextContent(string(bundle.Sources["main"]))}},
	)
}

func TestManifestSnapshot_DeterministicIdentityAndDefensiveCopies(t *testing.T) {
	t.Parallel()
	original := SourceBundle{
		Sources: map[string][]byte{"main": []byte("OLD"), "child": []byte("rules")},
		Config:  map[string]string{"environment": "prod", "partials": "partials/*.tmpl"},
	}
	snapshot, err := NewManifestSnapshot("agent", original, snapshotFixtureMaterializer)
	require.NoError(t, err)
	reordered := SourceBundle{
		Sources: map[string][]byte{"child": []byte("rules"), "main": []byte("OLD")},
		Config:  map[string]string{"partials": "partials/*.tmpl", "environment": "prod"},
	}
	same, err := NewManifestSnapshot("agent", reordered, snapshotFixtureMaterializer)
	require.NoError(t, err)
	require.Equal(t, snapshot.Descriptor(), same.Descriptor())
	original.Sources["main"][0] = 'X'
	original.Config["environment"] = "test"
	exported := snapshot.Sources()
	exported.Sources["main"][0] = 'Y'
	exported.Config["environment"] = "stage"
	plan, err := snapshot.Plan(context.Background(), RegistryPlanInput{})
	require.NoError(t, err)
	execution, err := plan.Execute(context.Background())
	require.NoError(t, err)
	require.Equal(t, "OLD", execution.Messages[0].Content[0].(TextPart).Text)
	reordered.Sources["child"] = []byte("changed import")
	changed, err := NewManifestSnapshot("agent", reordered, snapshotFixtureMaterializer)
	require.NoError(t, err)
	require.NotEqual(t, snapshot.Descriptor(), changed.Descriptor())
	reordered.Sources["child"] = []byte("rules")
	reordered.Config["partials"] = "other/*.tmpl"
	changed, err = NewManifestSnapshot("agent", reordered, snapshotFixtureMaterializer)
	require.NoError(t, err)
	require.NotEqual(t, snapshot.Descriptor(), changed.Descriptor())
}

func TestManifestSnapshot_RejectsMutatingCustomMaterializer(t *testing.T) {
	t.Parallel()
	snapshot, err := NewManifestSnapshot("agent", SourceBundle{Sources: map[string][]byte{"main": []byte("OLD")}},
		func(ctx context.Context, bundle SourceBundle, input RegistryPlanInput) (*ChatPromptTemplate, error) {
			bundle.Sources["main"] = []byte("NEW")
			return snapshotFixtureMaterializer(ctx, bundle, input)
		})
	require.NoError(t, err)
	_, err = snapshot.Plan(context.Background(), RegistryPlanInput{})
	require.ErrorIs(t, err, ErrManifestDigestMismatch)
	require.Equal(t, "OLD", string(snapshot.Sources().Sources["main"]))
}

func TestManifestDescriptor_RejectsLegacyWire(t *testing.T) {
	t.Parallel()
	var descriptor ManifestDescriptor
	err := json.Unmarshal([]byte(`{"id":"agent","digest":"deadbeef"}`), &descriptor)
	require.ErrorIs(t, err, ErrWireFormat)
}

func TestManifestDescriptor_ExecutableSchemaMatchesWire(t *testing.T) {
	t.Parallel()
	schema, err := os.ReadFile("schemas/manifest-descriptor.json")
	require.NoError(t, err)
	definition := &SchemaDefinition{Schema: JSONDocument(schema)}
	validDigest := "source-bundle.v1:sha256:" + strings.Repeat("a", 64)
	for _, fixture := range []struct {
		id, digest string
		valid      bool
	}{
		{"agent", validDigest, true}, {".hidden/agent", validDigest, true}, {"nested/agent", validDigest, true},
		{".", validDigest, false}, {"../agent", validDigest, false}, {"a//agent", validDigest, false},
		{"agent.json", validDigest, false}, {"agent", strings.Repeat("a", 64), false},
	} {
		data, err := json.Marshal(map[string]string{"id": fixture.id, "digest": fixture.digest})
		require.NoError(t, err)
		schemaErr := ValidateSchemaJSON(definition, JSONDocument(data))
		var descriptor ManifestDescriptor
		wireErr := json.Unmarshal(data, &descriptor)
		require.Equal(t, fixture.valid, schemaErr == nil, fixture.id)
		require.Equal(t, fixture.valid, wireErr == nil, fixture.id)
	}
}

func TestComposeValuesNestedOwnership(t *testing.T) {
	t.Parallel()
	// Arrange.
	values := NewComposeValuesFromPairs(ComposeBool("feature.deep.on", true))
	input := planInputWithComposeValues(RegistryPlanInput{}, values)
	// Act: both exported nested values and private plan copies can be changed independently.
	exported, ok := values.Lookup("feature.deep")
	require.True(t, ok)
	exported.(map[string]any)["on"] = false
	private := input.ComposeValues().mapValue()
	private["feature"].(map[string]any)["deep"].(map[string]any)["on"] = false
	// Assert.
	original, ok := values.Lookup("feature.deep.on")
	require.True(t, ok)
	require.Equal(t, true, original)
	bound, ok := input.ComposeValues().Lookup("feature.deep.on")
	require.True(t, ok)
	require.Equal(t, true, bound)
}
