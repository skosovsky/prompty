package prompty

import (
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestSchemaReferencesNeverFetchHTTP(t *testing.T) {
	t.Parallel()
	// Arrange: an accidental implicit loader could successfully retrieve this schema.
	var requests atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		requests.Add(1)
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"type":"integer","minimum":10}`)
	}))
	t.Cleanup(server.Close)
	uri := server.URL + "/schema"
	source := JSONDocument(`{"$ref":"` + uri + `"}`)
	for _, test := range []struct {
		name      string
		resources map[string]JSONDocument
		valid     bool
	}{
		{"missing resource", nil, false},
		{"supplied resource", map[string]JSONDocument{uri: JSONDocument(`{"type":"integer","minimum":10}`)}, true},
		{"missing transitive resource", map[string]JSONDocument{uri: JSONDocument(`{"$ref":"` + server.URL + `/hidden"}`)}, false},
		{"missing transitive fragment", map[string]JSONDocument{uri: JSONDocument(`{"$ref":"#/missing"}`)}, false},
		{"missing transitive anchor", map[string]JSONDocument{uri: JSONDocument(`{"$ref":"#missing"}`)}, false},
	} {
		for _, role := range []SchemaRole{SchemaOutput, SchemaToolInput} {
			t.Run(test.name+"/"+string(role), func(t *testing.T) {
				t.Parallel()
				// Act: mapper invocation happens only after complete original reference resolution.
				mapperCalls := 0
				compiled, err := CompileProviderSchema(SchemaCompileRequest{
					Source: source, Role: role,
					Profile:   ProfileIdentity{Provider: "fixture", Endpoint: "fixture", Model: "fixture"},
					Validator: NewJSONSchemaValidator(test.resources),
				}, schemaMapperFunc(func(request SchemaMappingRequest) (SchemaMapping, error) {
					mapperCalls++
					return SchemaMapping{Schema: request.Source}, nil
				}))
				// Assert: supplied schemas validate locally, missing references never fetch/replay.
				if test.valid {
					require.NoError(t, err)
					require.Equal(t, 1, mapperCalls)
					require.ErrorIs(t, compiled.ValidateOriginal(JSONDocument(`1`)), ErrSchemaMismatch)
					require.NoError(t, compiled.ValidateOriginal(JSONDocument(`10`)))
				} else {
					require.ErrorIs(t, err, ErrSchemaUnsupported)
					require.Nil(t, compiled)
					require.Zero(t, mapperCalls)
					failure, ok := errors.AsType[*SchemaCompilationError](err)
					require.True(t, ok)
					origin := SchemaSourceLocation{Resource: "", Pointer: "/$ref"}
					if test.name == "missing transitive resource" || test.name == "missing transitive fragment" ||
						test.name == "missing transitive anchor" {
						origin.Resource = uri
					}
					require.Equal(t, []SchemaSourceLocation{origin}, failure.Locations)
				}
				require.Zero(t, requests.Load())
			})
		}
	}
}
