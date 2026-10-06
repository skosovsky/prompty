package gen

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/skosovsky/prompty"
)

func TestGeneratedMethodNamesRejectCollisions(t *testing.T) {
	t.Parallel()
	for _, property := range []string{"validate_schema", "JSONSchema", "UnmarshalJSON"} {
		for _, part := range []string{"input", "late", "output"} {
			t.Run(property+"/"+part, func(t *testing.T) {
				t.Parallel()
				schema := &prompty.SchemaDefinition{
					Schema: prompty.JSONDocument(
						fmt.Sprintf(
							`{"type":"object","properties":{%q:{"type":"string"%s}}}`,
							property,
							map[string]string{"late": `,"late":true`}[part],
						),
					),
				}
				spec := &PromptSpec{ID: "method_collision"}
				if part == "output" {
					spec.ResponseFormat = schema
				} else {
					spec.InputSchema = schema
				}
				_, err := GenerateManifestTypes(spec, "consumer")
				require.Error(t, err)
				require.Contains(t, err.Error(), "conflicts with generated method")
			})
		}
	}
}

func TestGeneratedConsumerPreservesPresence(t *testing.T) {
	// Arrange: compile emitted Go code against the library, including nested nullable values.
	schema := &prompty.SchemaDefinition{
		Schema: prompty.JSONDocument(
			`{"type":"object","properties":{"items":{"type":"array","items":{"type":["integer","null"]}},"labels":{"type":"object","additionalProperties":{"type":["string","null"]}},"nullable":{"type":["string","null"]},"nullable_items":{"type":["array","null"],"items":{"type":"integer"}},"nested":{"type":"object","properties":{"value":{"type":["integer","null"]}}},"objects":{"type":"array","items":{"type":["object","null"],"properties":{"value":{"type":["integer","null"]}}}},"matrices":{"type":"array","items":{"type":["array","null"],"items":{"type":["boolean","null"]}}}},"allOf":[{"required":["items","labels"]}],"additionalProperties":false}`,
		),
	}
	spec := &PromptSpec{
		ID:          "presence",
		InputSchema: schema,
		Descriptor:  prompty.ManifestDescriptor{ID: "presence", Digest: "sha256:presence"},
	}
	generated, err := GenerateManifestTypes(spec, "consumer")
	require.NoError(t, err)
	shared, err := GenerateSharedTypes("consumer", []*PromptSpec{spec})
	require.NoError(t, err)
	directory, err := filepath.EvalSymlinks(t.TempDir())
	require.NoError(t, err)
	root, err := filepath.Abs(filepath.Join("..", "..", ".."))
	require.NoError(t, err)
	require.NoError(
		t,
		os.WriteFile(
			filepath.Join(directory, "go.mod"),
			[]byte(
				fmt.Sprintf(
					"module consumer\n\ngo 1.26.3\n\nrequire github.com/skosovsky/prompty v0.0.0\nreplace github.com/skosovsky/prompty => %s\n",
					fmt.Sprintf("%q", root),
				),
			),
			0600,
		),
	)
	workspace, err := os.ReadFile(filepath.Join(root, "go.work"))
	require.NoError(t, err)
	lines := strings.Split(string(workspace), "\n")
	for index, line := range lines {
		path := strings.TrimSpace(line)
		if path == "." || strings.HasPrefix(path, "./") {
			lines[index] = "\t" + fmt.Sprintf("%q", filepath.Join(root, path))
		}
		if path == ")" {
			lines[index] = "\t" + fmt.Sprintf("%q", directory) + "\n)"
		}
	}
	workspacePath := filepath.Join(directory, "go.work")
	require.NoError(t, os.WriteFile(workspacePath, []byte(strings.Join(lines, "\n")), 0600))
	sums, err := os.ReadFile(filepath.Join(root, "go.work.sum"))
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(workspacePath+".sum", sums, 0600))
	require.NoError(t, generated.Save(filepath.Join(directory, "presence_gen.go")))
	require.NoError(t, shared.Save(filepath.Join(directory, "shared_gen.go")))
	require.NoError(t, os.WriteFile(filepath.Join(directory, "presence_test.go"), []byte(`package consumer
import("encoding/json";"testing")
func TestPresence(t *testing.T){
 for _,document:=range []string{
  `+"`"+`{"items":[],"labels":{}}`+"`"+`,
  `+"`"+`{"items":[],"labels":{},"nullable":null,"nullable_items":null,"nested":{"value":null}}`+"`"+`,
  `+"`"+`{"items":[null,9007199254740993],"labels":{"key":"value","null":null},"nullable":"","nullable_items":[],"nested":{"value":0},"objects":[null,{"value":null}],"matrices":[null,[null,false]]}`+"`"+`,
 }{
  var input PresenceInput
  if err:=json.Unmarshal([]byte(document),&input);err!=nil{t.Fatal(err)}
  if err:=input.ValidateSchema();err!=nil{t.Fatal(err)}
  result,err:=json.Marshal(input);if err!=nil{t.Fatal(err)}
  var expected,actual map[string]json.RawMessage
  json.Unmarshal([]byte(document),&expected);json.Unmarshal(result,&actual)
  if len(actual)!=len(expected){t.Fatalf("presence lost: %s => %s",document,result)}
  for key,value:=range expected {if string(actual[key])!=string(value){t.Fatalf("value changed: %s => %s",document,result)}}
 }
 var input PresenceInput
 if err:=json.Unmarshal([]byte(`+"`"+`{}`+"`"+`),&input);err==nil{t.Fatal("missing required properties accepted")}
}
`), 0600))
	// Act: build and execute the generated consumer.
	command := exec.Command("go", "test", "./...")
	command.Dir = directory
	command.Env = append(os.Environ(), "GOWORK="+workspacePath)
	output, err := command.CombinedOutput()
	// Assert: source-valid presence survives decode, typed validation and remarshal.
	require.NoError(t, err, string(output)+"\n"+strings.Join(lines, "\n"))
}

func TestGeneratorRejectsUnrepresentableObjectContracts(t *testing.T) {
	t.Parallel()
	for _, document := range []string{
		`{"type":"object","properties":{"a":{"type":"string"}},"required":["unknown"]}`,
		`{"type":"object","properties":{"items":{"type":"array","items":{"type":"integer"}}},"minProperties":1}`,
		`{"type":"object","properties":{"a":{"type":"string"}},"if":{"required":["unknown"]},"else":{"required":["a"]}}`,
		`{"type":"object","properties":{"nested":{"type":"object","properties":{"a":{"type":"string"}},"minProperties":1}}}`,
		`{"type":["object","null"],"properties":{"a":{"type":"string"}}}`,
	} {
		for _, part := range []string{"input", "output"} {
			t.Run(part+document, func(t *testing.T) {
				t.Parallel()
				spec := &PromptSpec{ID: "projection"}
				schema := &prompty.SchemaDefinition{Schema: prompty.JSONDocument(document)}
				if part == "input" {
					spec.InputSchema = schema
				} else {
					spec.ResponseFormat = schema
				}
				_, err := GenerateManifestTypes(spec, "consumer")
				require.Error(t, err)
			})
		}
	}
}

func TestGeneratorRejectsInvalidJSONFieldNames(t *testing.T) {
	t.Parallel()
	for _, name := range []string{"a,b", `a\b`, "a\"b"} {
		for _, part := range []string{"input", "output", "nested"} {
			t.Run(part+name, func(t *testing.T) {
				t.Parallel()
				property := map[string]any{name: map[string]any{"type": "string"}}
				schemaMap := map[string]any{"type": "object", "properties": property}
				if part == "nested" {
					schemaMap = map[string]any{"type": "object", "properties": map[string]any{"nested": schemaMap}}
				}
				definition := &prompty.SchemaDefinition{Schema: prompty.MustJSONDocumentFromMap(schemaMap)}
				spec := &PromptSpec{ID: "names"}
				if part == "output" {
					spec.ResponseFormat = definition
				} else {
					spec.InputSchema = definition
				}
				_, err := GenerateManifestTypes(spec, "consumer")
				require.Error(t, err)
				require.Contains(t, err.Error(), "encoding/json field tag")
			})
		}
	}
}

func TestGeneratedProjectionRejectsRootReferenceSiblings(t *testing.T) {
	t.Parallel()
	for _, dialect := range []string{prompty.SchemaDialect7, prompty.SchemaDialect2020} {
		t.Run(dialect, func(t *testing.T) {
			t.Parallel()
			// Arrange: draft-07 ignores the apparent closed-object sibling of $ref.
			definition := &prompty.SchemaDefinition{
				Dialect: dialect,
				Schema: prompty.JSONDocument(
					fmt.Sprintf(
						`{"$schema":%q,"$ref":"#/definitions/open","type":"object","properties":{"n":{"type":"integer"}},"additionalProperties":false,"definitions":{"open":{"type":"object","minProperties":1}}}`,
						dialect,
					),
				),
			}
			if dialect == prompty.SchemaDialect7 {
				require.NoError(t, prompty.ValidateSchemaJSON(definition, prompty.JSONDocument(`{"extra":1}`)))
			}
			for _, output := range []bool{false, true} {
				spec := &PromptSpec{ID: "root_reference"}
				if output {
					spec.ResponseFormat = definition
				} else {
					spec.InputSchema = definition
				}
				// Act.
				_, err := GenerateManifestTypes(spec, "consumer")
				// Assert: DTO generation cannot infer object closure from ignored siblings.
				require.Error(t, err)
				require.Contains(t, err.Error(), "referenced object requires caller-owned type")
			}
		})
	}
}
