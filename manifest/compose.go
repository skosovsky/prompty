package manifest

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"maps"
	"sort"

	"github.com/skosovsky/prompty"
	"github.com/skosovsky/prompty/internal/exactjson"
)

// RawCondition is a structured matcher for conditional imports (MVP: match map only).
type RawCondition struct {
	Match map[string]any `json:"match,omitempty" yaml:"match,omitempty"`
}

// RawImport references another manifest with an optional condition.
type RawImport struct {
	ID        string        `json:"id"                  yaml:"id"`
	Condition *RawCondition `json:"condition,omitempty" yaml:"condition,omitempty"`
}

// RawLayer is a declarative prompt layer or import reference.
//

type RawLayer struct {
	ID          string               `json:"id"                     yaml:"id"`
	Role        string               `json:"role,omitempty"         yaml:"role,omitempty"`
	LayerKind   prompty.LayerKind    `json:"layer_kind,omitempty"   yaml:"layer_kind,omitempty"`
	ImportRef   string               `json:"import_ref,omitempty"   yaml:"import_ref,omitempty"`
	Content     []RawContentPart     `json:"content,omitempty"      yaml:"content,omitempty"`
	Optional    bool                 `json:"optional,omitempty"     yaml:"optional,omitempty"`
	CachePolicy *prompty.CachePolicy `json:"cache_policy,omitempty" yaml:"cache_policy,omitempty"`
	Metadata    map[string]any       `json:"metadata,omitempty"     yaml:"metadata,omitempty"`
}

// ManifestLoader resolves manifests by id for composition and schema merge.
//
//nolint:revive // ManifestLoader is the explicit cross-package contract name
type ManifestLoader interface {
	LoadByID(ctx context.Context, id string) (*RawManifest, error)
}

// MemoryLoader is an in-memory ManifestLoader for tests and prompty-gen indexing.
type MemoryLoader struct {
	ByID map[string]*RawManifest
}

const (
	schemaObjectType    = "object"
	schemaTypeKey       = "type"
	schemaPropertiesKey = "properties"
)

func (l *MemoryLoader) LoadByID(_ context.Context, id string) (*RawManifest, error) {
	if l == nil || l.ByID == nil {
		return nil, fmt.Errorf("compose: unknown manifest id %q", id)
	}
	raw, ok := l.ByID[id]
	if !ok || raw == nil {
		return nil, fmt.Errorf("compose: unknown manifest id %q", id)
	}
	return raw, nil
}

// ComposeContext carries typed runtime values for conditional imports.
type ComposeContext struct {
	Ctx                         context.Context
	Values                      prompty.ComposeValues
	Loader                      ManifestLoader
	AllowMissingConditionValues bool
}

func composeLoaderCtx(ctx ComposeContext) context.Context {
	if ctx.Ctx != nil {
		return ctx.Ctx
	}
	return context.Background()
}

// matchCondition evaluates structured condition.match against typed compose values.
// Missing dot-path keys yield false; comparison is strict (type-sensitive).
func matchCondition(match map[string]any, values prompty.ComposeValues) (bool, error) {
	if len(match) == 0 {
		return true, nil
	}
	for key, want := range match {
		got, ok := values.Lookup(key)
		if !ok {
			return false, nil
		}
		if !strictEqual(got, want) {
			return false, nil
		}
	}
	return true, nil
}

func strictEqual(a, b any) bool { return equalSchemaValues(a, b) }

func PeekComposeOrError(data []byte, u Unmarshaler) (bool, error) {
	return PeekComposeFieldsE(data, u)
}

// PeekComposeFieldsE reports whether manifest bytes declare imports or layers.
// Unmarshal errors return (false, err) — corrupt bytes are not treated as compose.
func PeekComposeFieldsE(data []byte, u Unmarshaler) (bool, error) {
	if len(data) == 0 || u == nil {
		return false, nil
	}
	raw, err := parseCapturedManifest(data, u)
	if err != nil {
		return false, fmt.Errorf("peek compose fields: %w", err)
	}
	return len(raw.Imports) > 0 || len(raw.Layers) > 0, nil
}

func importActive(imp RawImport, values prompty.ComposeValues, allowMissingValues bool) (bool, error) {
	if imp.Condition == nil || len(imp.Condition.Match) == 0 {
		return true, nil
	}
	if !values.IsSet() {
		if allowMissingValues {
			return true, nil
		}
		return false, fmt.Errorf("compose: import %q condition.match requires compose values", imp.ID)
	}
	return matchCondition(imp.Condition.Match, values)
}

// MergeInputSchemas combines the input namespace without overriding field contracts.
// Repeated properties and root constraints must match exactly; required names are unioned.
func MergeInputSchemas(local *prompty.SchemaDefinition, imports ...*RawManifest) (*prompty.SchemaDefinition, error) {
	definitions := []*prompty.SchemaDefinition{local}
	for _, imp := range imports {
		if imp != nil {
			definitions = append(definitions, imp.InputSchema)
		}
	}
	merge := inputSchemaMerge{root: make(map[string]any), properties: make(map[string]any), dialect: ""}
	for _, definition := range definitions {
		if definition == nil {
			continue
		}
		if err := merge.add(definition); err != nil {
			return nil, err
		}
	}
	if merge.dialect == "" {
		return nil, nil //nolint:nilnil // absent input schemas are valid.
	}
	required, err := mergeRequired(local, imports...)
	if err != nil {
		return nil, err
	}
	if namespaceErr := merge.validateNamespaces(definitions); namespaceErr != nil {
		return nil, namespaceErr
	}
	merged := merge.root
	properties := merge.properties
	dialect := merge.dialect
	merged["properties"] = properties
	if len(required) > 0 {
		merged["required"] = required
	}
	document, err := prompty.MapToJSONDocument(merged)
	if err != nil {
		return nil, err
	}
	return &prompty.SchemaDefinition{Name: "", Description: "", Schema: document, Dialect: dialect}, nil
}

func equalSchemaValues(a, b any) bool {
	left, err := json.Marshal(a)
	if err != nil {
		return false
	}
	right, err := json.Marshal(b)
	return err == nil && exactjson.Equal(left, right)
}

func inputProperties(schema *prompty.SchemaDefinition) (map[string]any, error) {
	if schema == nil || len(schema.Schema) == 0 {
		return map[string]any{}, nil
	}
	doc, err := prompty.JSONDocumentAsMap(schema.Schema)
	if err != nil {
		return nil, fmt.Errorf("merge inputs: decode schema properties: %w", err)
	}
	props, _ := doc[schemaPropertiesKey].(map[string]any)
	if props == nil {
		return map[string]any{}, nil
	}
	return maps.Clone(props), nil
}

func mergeRequired(local *prompty.SchemaDefinition, imports ...*RawManifest) ([]string, error) {
	req, err := requiredSet(local)
	if err != nil {
		return nil, err
	}
	for _, imp := range imports {
		if imp == nil {
			continue
		}
		impReq, impErr := requiredSet(imp.InputSchema)
		if impErr != nil {
			return nil, impErr
		}
		for name := range impReq {
			req[name] = true
		}
	}
	if len(req) == 0 {
		return nil, nil
	}
	out := make([]string, 0, len(req))
	for name := range req {
		out = append(out, name)
	}
	sort.Strings(out)
	return out, nil
}

func requiredSet(schema *prompty.SchemaDefinition) (map[string]bool, error) {
	out := make(map[string]bool)
	if schema == nil {
		return out, nil
	}
	doc, err := prompty.JSONDocumentAsMap(schema.Schema)
	if err != nil {
		return nil, fmt.Errorf("merge inputs: decode schema required: %w", err)
	}
	switch r := doc["required"].(type) {
	case []string:
		for _, s := range r {
			out[s] = true
		}
	case []any:
		for _, e := range r {
			if s, ok := e.(string); ok {
				out[s] = true
			}
		}
	}
	return out, nil
}

func cloneRawManifest(src *RawManifest) (*RawManifest, error) {
	if src == nil {
		return nil, errors.New("compose: manifest is nil")
	}
	data, err := json.Marshal(src)
	if err != nil {
		return nil, fmt.Errorf("compose: clone manifest: %w", err)
	}
	var dst RawManifest
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.UseNumber()
	if err := decoder.Decode(&dst); err != nil {
		return nil, fmt.Errorf("compose: clone manifest: %w", err)
	}
	return &dst, nil
}

// checkImportCycles detects cycles among imports. With runtime values set, only active imports are traversed.
func checkImportCycles(
	ctx context.Context,
	imports []RawImport,
	loader ManifestLoader,
	values prompty.ComposeValues,
	allowMissingValues bool,
) error {
	if loader == nil || len(imports) == 0 {
		return nil
	}
	visiting := make(map[string]bool)
	stack := make(map[string]bool)
	for _, imp := range imports {
		if imp.ID == "" {
			continue
		}
		active, err := importActive(imp, values, allowMissingValues)
		if err != nil {
			return err
		}
		if !active {
			continue
		}
		if err := visitImportGraph(ctx, imp.ID, loader, values, allowMissingValues, visiting, stack); err != nil {
			return err
		}
	}
	return nil
}

func importDeclared(imports []RawImport, ref string) bool {
	for _, imp := range imports {
		if imp.ID == ref {
			return true
		}
	}
	return false
}

func visitImportGraph(
	ctx context.Context,
	id string,
	loader ManifestLoader,
	values prompty.ComposeValues,
	allowMissingValues bool,
	visiting, stack map[string]bool,
) error {
	if stack[id] {
		return fmt.Errorf("compose: cyclic import %q", id)
	}
	if visiting[id] {
		return nil
	}
	visiting[id] = true
	stack[id] = true
	raw, err := loader.LoadByID(ctx, id)
	if err != nil {
		return err
	}
	for _, imp := range raw.Imports {
		if imp.ID == "" {
			return errors.New("compose: import id is required")
		}
		active, activeErr := importActive(imp, values, allowMissingValues)
		if activeErr != nil {
			return activeErr
		}
		if !active {
			continue
		}
		if err := visitImportGraph(ctx, imp.ID, loader, values, allowMissingValues, visiting, stack); err != nil {
			return err
		}
	}
	stack[id] = false
	return nil
}

//nolint:gocognit // transitive import walk mirrors compose graph resolution
func collectTransitiveImports(
	ctx context.Context,
	raw *RawManifest,
	loader ManifestLoader,
	values prompty.ComposeValues,
	allowMissingValues bool,
) ([]*RawManifest, error) {
	if raw == nil || loader == nil || len(raw.Imports) == 0 {
		return nil, nil
	}
	visited := make(map[string]bool)
	var out []*RawManifest
	var walk func(m *RawManifest) error
	walk = func(m *RawManifest) error {
		for _, imp := range m.Imports {
			if imp.ID == "" {
				return errors.New("compose: import id is required")
			}
			active, activeErr := importActive(imp, values, allowMissingValues)
			if activeErr != nil {
				return activeErr
			}
			if !active {
				continue
			}
			if visited[imp.ID] {
				continue
			}
			visited[imp.ID] = true
			child, err := loader.LoadByID(ctx, imp.ID)
			if err != nil {
				return fmt.Errorf("compose: load import %q: %w", imp.ID, err)
			}
			out = append(out, child)
			if len(child.Imports) > 0 {
				if err := walk(child); err != nil {
					return err
				}
			}
		}
		return nil
	}
	if err := walk(raw); err != nil {
		return nil, err
	}
	return out, nil
}

// ResolveEffectiveInputSchema merges local inputs with all referenced imports (codegen union).
func ResolveEffectiveInputSchema(
	ctx context.Context,
	raw *RawManifest,
	loader ManifestLoader,
) (*prompty.SchemaDefinition, error) {
	if raw == nil {
		return nil, errors.New("compose: manifest is nil")
	}
	if len(raw.Imports) == 0 {
		return raw.InputSchema, nil
	}
	if loader == nil {
		return nil, errors.New("compose: manifest loader is required for imports")
	}
	if err := checkImportCycles(ctx, raw.Imports, loader, prompty.ComposeValues{}, true); err != nil {
		return nil, err
	}
	imports, err := collectTransitiveImports(ctx, raw, loader, prompty.ComposeValues{}, true)
	if err != nil {
		return nil, err
	}
	return MergeInputSchemas(raw.InputSchema, imports...)
}

func rejectLayersAndMessagesTogether(raw *RawManifest) error {
	if len(raw.Layers) > 0 && len(raw.Messages) > 0 {
		return errors.New("compose: manifest must not declare both layers and messages; move all turns into layers")
	}
	return nil
}

func validateInlineLayerIDs(layers []RawLayer) error {
	for _, layer := range layers {
		if layer.ImportRef != "" {
			continue
		}
		if layer.ID == "" {
			return errors.New("compose: inline layer requires id")
		}
	}
	return nil
}

// ExpandRawManifest flattens layers and merges inputs in-place.
func ExpandRawManifest(raw *RawManifest, ctx ComposeContext) error {
	if raw == nil {
		return errors.New("compose: manifest is nil")
	}
	if len(raw.Layers) == 0 && len(raw.Imports) == 0 {
		return nil
	}
	if ctx.Loader == nil {
		return errors.New("compose: manifest loader is required for imports/layers")
	}
	working, err := cloneRawManifest(raw)
	if err != nil {
		return err
	}
	lctx := composeLoaderCtx(ctx)
	if cycleErr := checkImportCycles(
		lctx,
		working.Imports,
		ctx.Loader,
		ctx.Values,
		ctx.AllowMissingConditionValues,
	); cycleErr != nil {
		return cycleErr
	}
	activeImports, err := resolveActiveImports(
		lctx,
		working.Imports,
		ctx.Values,
		ctx.AllowMissingConditionValues,
		ctx.Loader,
	)
	if err != nil {
		return err
	}
	if err := rejectLayersAndMessagesTogether(working); err != nil {
		return err
	}
	if len(working.Layers) > 0 {
		msgs, flatErr := flattenLayers(working, activeImports, ctx, make(map[string]bool))
		if flatErr != nil {
			return flatErr
		}
		working.Messages = msgs
	}
	if len(working.Imports) > 0 {
		importManifests := make([]*RawManifest, 0, len(working.Imports))
		for _, imp := range working.Imports {
			child, ok := activeImports[imp.ID]
			if !ok || child == nil {
				continue
			}
			importManifests = append(importManifests, child)
		}
		merged, mergeErr := MergeInputSchemas(working.InputSchema, importManifests...)
		if mergeErr != nil {
			return mergeErr
		}
		working.InputSchema = merged
	}
	working.Layers = nil
	working.Imports = nil
	*raw = *working
	return nil
}

func resolveActiveImports(
	lctx context.Context,
	imports []RawImport,
	values prompty.ComposeValues,
	allowMissingValues bool,
	loader ManifestLoader,
) (map[string]*RawManifest, error) {
	out := make(map[string]*RawManifest, len(imports))
	if loader == nil {
		return out, nil
	}
	composeCtx := ComposeContext{
		Ctx:                         lctx,
		Values:                      values,
		Loader:                      loader,
		AllowMissingConditionValues: allowMissingValues,
	}
	for _, imp := range imports {
		if imp.ID == "" {
			return nil, errors.New("compose: import id is required")
		}
		active, err := importActive(imp, values, allowMissingValues)
		if err != nil {
			return nil, err
		}
		if !active {
			continue
		}
		loaded, err := loader.LoadByID(lctx, imp.ID)
		if err != nil {
			return nil, fmt.Errorf("compose: load import %q: %w", imp.ID, err)
		}
		child, err := cloneRawManifest(loaded)
		if err != nil {
			return nil, err
		}
		if len(child.Layers) > 0 || len(child.Imports) > 0 {
			if expandErr := ExpandRawManifest(child, composeCtx); expandErr != nil {
				return nil, fmt.Errorf("compose: expand import %q: %w", imp.ID, expandErr)
			}
		}
		out[imp.ID] = child
	}
	return out, nil
}

//nolint:gocognit,nestif // import_ref expansion walks nested manifest graphs
func flattenLayers(
	raw *RawManifest,
	activeImports map[string]*RawManifest,
	ctx ComposeContext,
	visiting map[string]bool,
) ([]RawMessage, error) {
	if err := validateInlineLayerIDs(raw.Layers); err != nil {
		return nil, err
	}
	out := make([]RawMessage, 0, len(raw.Layers))
	for _, layer := range raw.Layers {
		if layer.ImportRef != "" {
			child, ok := activeImports[layer.ImportRef]
			if !ok {
				if importDeclared(raw.Imports, layer.ImportRef) {
					continue
				}
				return nil, fmt.Errorf("compose: unknown import_ref %q in layer %q", layer.ImportRef, layer.ID)
			}
			if visiting[layer.ImportRef] {
				return nil, fmt.Errorf("compose: cyclic import %q", layer.ImportRef)
			}
			visiting[layer.ImportRef] = true
			childMsgs, err := manifestMessages(child, activeImports, ctx, visiting)
			visiting[layer.ImportRef] = false
			if err != nil {
				return nil, err
			}
			for i := range childMsgs {
				if childMsgs[i].LayerID == "" {
					childMsgs[i].LayerID = layer.ID
				}
				if childMsgs[i].LayerKind == "" {
					childMsgs[i].LayerKind = layer.LayerKind
				}
			}
			out = append(out, childMsgs...)
			continue
		}
		if layer.Role == "" {
			return nil, fmt.Errorf("compose: layer %q requires role or import_ref", layer.ID)
		}
		if len(layer.Content) == 0 {
			return nil, fmt.Errorf("compose: layer %q requires content", layer.ID)
		}
		out = append(out, RawMessage{LegacySourceID: "",
			Role:        layer.Role,
			LayerID:     layer.ID,
			LayerKind:   layer.LayerKind,
			Content:     layer.Content,
			Optional:    layer.Optional,
			CachePolicy: layer.CachePolicy,
			Metadata:    layer.Metadata,
		})
	}
	return out, nil
}

func manifestMessages(
	raw *RawManifest,
	activeImports map[string]*RawManifest,
	ctx ComposeContext,
	visiting map[string]bool,
) ([]RawMessage, error) {
	if len(raw.Layers) > 0 {
		return flattenLayers(raw, activeImports, ctx, visiting)
	}
	return raw.Messages, nil
}

type inputSchemaMerge struct {
	root       map[string]any
	properties map[string]any
	dialect    string
}

func (m *inputSchemaMerge) add(definition *prompty.SchemaDefinition) error {
	effective := definition.Dialect
	if effective == "" {
		effective = prompty.SchemaDialect2020
	}
	if m.dialect != "" && effective != m.dialect {
		return errors.New("merge inputs: conflicting dialects")
	}
	m.dialect = effective
	document, err := prompty.SchemaDocumentAsMap(definition.Schema)
	if err != nil {
		return fmt.Errorf("merge inputs: %w", err)
	}
	if _, compileErr := prompty.NewJSONSchemaValidator(nil).
		CompileSchema(definition.Schema, effective); compileErr != nil {
		return fmt.Errorf("merge inputs: %w", compileErr)
	}
	for key, value := range document {
		if key == "required" || key == "properties" {
			continue
		}
		if previous, exists := m.root[key]; exists && !equalSchemaValues(previous, value) {
			return fmt.Errorf("merge inputs: conflicting root constraint %q", key)
		}
		m.root[key] = value
	}
	fields, err := inputProperties(definition)
	if err != nil {
		return err
	}
	for name, property := range fields {
		if previous, exists := m.properties[name]; exists && !equalSchemaValues(previous, property) {
			return fmt.Errorf("merge inputs: property %q has conflicting contract between manifests", name)
		}
		m.properties[name] = property
	}
	return nil
}
func (m *inputSchemaMerge) validateNamespaces(definitions []*prompty.SchemaDefinition) error {
	for _, definition := range definitions {
		if definition == nil {
			continue
		}
		document, err := prompty.SchemaDocumentAsMap(definition.Schema)
		if err != nil {
			return err
		}
		fields, err := inputProperties(definition)
		if err != nil {
			return err
		}
		if len(fields) == len(m.properties) {
			continue
		}
		if additional, exists := document["additionalProperties"]; exists && additional != true {
			return errors.New("merge inputs: additionalProperties cannot be preserved while adding fields")
		}
		if _, exists := document["unevaluatedProperties"]; exists {
			return errors.New("merge inputs: unevaluatedProperties cannot be preserved while adding fields")
		}
	}
	return nil
}
