package prompty

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestDiagnosticObservationDoesNotCacheTemporaryEdges(t *testing.T) {
	for _, dialect := range []string{SchemaDialect7, SchemaDialect2020} {
		t.Run(dialect, func(t *testing.T) {
			// Arrange: observation must stop on this nested node, before root processing.
			const placeholder = "urn:prompty:diagnostic:value"
			node := map[string]any{"$ref": "#/missing"}
			root := map[string]any{"properties": map[string]any{"value": node}}
			compiler, err := diagnosticReferenceCompiler(
				map[string]any{schemaSourceURI: root},
				dialect,
				map[string]string{"missing": placeholder},
			)
			require.NoError(t, err)
			registerDiagnosticObservation(compiler, "urn:prompty:diagnostic:observation")
			for _, ref := range []string{"#/missing", placeholder, "#/missing-again"} {
				// Act: repeated mutation is private and must be read, never cached.
				node["$ref"] = ref
				compiled, err := compiler.Compile(schemaSourceURI + "#/properties/value")
				// Assert: only diagnostic observation, no usable successful validator.
				require.Nil(t, compiled)
				observation, ok := errors.AsType[*diagnosticSchemaObservationError](err)
				require.True(t, ok)
				require.Equal(t, schemaSourceURI+"#/properties/value", observation.schema.Location)
				target := diagnosticObservedTarget(observation.schema, "$ref")
				require.NotNil(t, target)
				want := ref + "#"
				if ref != placeholder {
					want = schemaSourceURI + ref
				}
				require.Equal(t, want, target.Location)
			}
		})
	}
}

func TestDiagnosticReferenceScaleKeepsAllOrigins(t *testing.T) {
	for _, dialect := range []string{SchemaDialect7, SchemaDialect2020} {
		for _, pattern := range []string{"local", "remote", "mixed", "annotations"} {
			t.Run(dialect+"/"+pattern, func(t *testing.T) {
				// Arrange: valid and irrelevant fields must not poison failed-edge probes.
				const count = 1000
				properties := make(map[string]any, count)
				examples := make([]any, 0, count)
				wanted := count
				for index := range count {
					ref, valid := diagnosticScaleReference(pattern, index)
					if valid {
						wanted--
					}
					properties[strconv.Itoa(index)] = map[string]any{"$ref": ref}
					if pattern == "annotations" {
						examples = append(examples, map[string]any{"$ref": fmt.Sprintf("#/irrelevant/%d", index)})
					}
				}
				doc := map[string]any{
					"properties":  properties,
					"definitions": map[string]any{"valid": true},
					"examples":    examples,
				}
				source, err := json.Marshal(doc)
				require.NoError(t, err)
				before := CloneJSONDocument(source)
				start := time.Now()
				// Act.
				compiled, err := NewJSONSchemaValidator(nil).CompileSchema(source, dialect)
				// Assert: no provider/network path and every active failed edge remains known.
				require.Nil(t, compiled)
				failure, ok := errors.AsType[*SchemaCompilationError](err)
				require.True(t, ok)
				require.Len(t, failure.Locations, wanted)
				require.Equal(t, before, JSONDocument(source))
				t.Logf("%d refs, %d origins, elapsed %s", count, wanted, time.Since(start))
			})
		}
	}
}

func diagnosticScaleReference(pattern string, index int) (string, bool) {
	if pattern == "remote" {
		return "https://fixture.invalid/absent#/missing", false
	}
	if pattern == "mixed" && index%2 == 0 {
		return "#/definitions/valid", true
	}
	return "#/missing", false
}

func TestInconclusiveReferenceProbeDoesNotValidateInvalidExtension(t *testing.T) {
	for _, dialect := range []string{SchemaDialect7, SchemaDialect2020} {
		t.Run(dialect, func(t *testing.T) {
			// Arrange: custom data becomes an actual schema only through its ref.
			source := JSONDocument(
				`{"allOf":[{"$ref":"#/missing"},{"$ref":"#/custom"}],"custom":{"$ref":"first","type":42}}`,
			)
			before := CloneJSONDocument(source)
			// Act.
			compiled, err := NewJSONSchemaValidator(nil).CompileSchema(source, dialect)
			// Assert: an inconclusive probe is never a successful validator.
			require.Nil(t, compiled)
			require.ErrorIs(t, err, ErrSchemaUnsupported)
			failure, ok := errors.AsType[*SchemaCompilationError](err)
			require.True(t, ok)
			require.Error(t, failure.Cause)
			require.NotContains(t, failure.Locations, SchemaSourceLocation{Resource: "", Pointer: "/custom/$ref"})
			require.Equal(t, before, source)
		})
	}
}

func TestDiagnosticManyExplicitResourcesAndBooleanTargets(t *testing.T) {
	for _, dialect := range []string{SchemaDialect7, SchemaDialect2020} {
		t.Run(dialect, func(t *testing.T) {
			// Arrange: independently scoped resources with existing true/false targets
			// and a failing transitive edge. Resource documents are caller-owned.
			const count = 100
			resources := make(map[string]JSONDocument, count)
			properties := make(map[string]any, count)
			for index := range count {
				uri := fmt.Sprintf("https://fixture.invalid/resource/%d", index)
				resources[uri] = JSONDocument(
					`{"definitions":{"yes":true,"no":false},"allOf":[{"$ref":"#/definitions/yes"},{"$ref":"#/definitions/no"},{"$ref":"#/absent"}]}`,
				)
				properties[strconv.Itoa(index)] = map[string]any{"$ref": uri}
			}
			source, err := json.Marshal(map[string]any{"properties": properties})
			require.NoError(t, err)
			validator := NewJSONSchemaValidator(resources)
			// Act.
			compiled, err := validator.CompileSchema(source, dialect)
			// Assert: only actual transitive failed edges, no Boolean-target false positives.
			require.Nil(t, compiled)
			failure, ok := errors.AsType[*SchemaCompilationError](err)
			require.True(t, ok)
			require.Len(t, failure.Locations, count)
			for _, location := range failure.Locations {
				require.Contains(t, resources, location.Resource)
				require.Equal(t, "/allOf/2/$ref", location.Pointer)
			}
			require.Equal(t, resources, validator.resources)
		})
	}
}

func BenchmarkDiagnosticReferenceOrigins(b *testing.B) {
	for _, count := range []int{100, 1000, 10000} {
		for _, pattern := range []string{"local", "mixed", "annotations", "unique-booleans", "valid-baseline", "valid-unique-booleans"} {
			b.Run(fmt.Sprintf("%s/%d", pattern, count), func(b *testing.B) {
				source, err := diagnosticBenchmarkSource(count, pattern)
				require.NoError(b, err)
				validator := NewJSONSchemaValidator(nil)
				b.ResetTimer()
				b.ReportAllocs()
				for b.Loop() {
					compiled, err := validator.CompileSchema(source, SchemaDialect2020)
					if pattern == "valid-baseline" || pattern == "valid-unique-booleans" {
						require.NoError(b, err)
						require.NotNil(b, compiled)
					} else {
						require.Nil(b, compiled)
						require.ErrorIs(b, err, ErrSchemaUnsupported)
					}
				}
			})
		}
	}
}

func diagnosticBenchmarkSource(count int, pattern string) ([]byte, error) {
	properties := make(map[string]any, count)
	definitions := make(map[string]any, count)
	examples := make([]any, 0, count)
	for index := range count {
		ref, _ := diagnosticScaleReference(pattern, index)
		if pattern == "valid-baseline" {
			ref = "#/definitions/valid"
		}
		if pattern == "unique-booleans" || pattern == "valid-unique-booleans" {
			definitions[strconv.Itoa(index)] = index%2 == 0
			if index > 0 || pattern == "valid-unique-booleans" {
				ref = "#/definitions/" + strconv.Itoa(index)
			}
		}
		properties[strconv.Itoa(index)] = map[string]any{"$ref": ref}
		if pattern == "annotations" {
			examples = append(examples, map[string]any{"$ref": ref, "type": index})
		}
	}
	definitions["valid"] = true
	return json.Marshal(map[string]any{"properties": properties, "definitions": definitions, "examples": examples})
}

func TestDiagnosticUniqueBenchmarkOnlyChangesOneReference(t *testing.T) {
	// Arrange: valid engine baseline has the identical graph, not a shared target.
	invalid, err := diagnosticBenchmarkSource(100, "unique-booleans")
	require.NoError(t, err)
	valid, err := diagnosticBenchmarkSource(100, "valid-unique-booleans")
	require.NoError(t, err)
	// Act: repair exactly the sole missing reference; preserve every definition.
	repaired := bytes.Replace(invalid, []byte(`"#/missing"`), []byte(`"#/definitions/0"`), 1)
	// Assert: the benchmark ratio compares the same source except that edge.
	require.Equal(t, 1, bytes.Count(invalid, []byte(`"#/missing"`)))
	require.Equal(t, valid, repaired)
}
