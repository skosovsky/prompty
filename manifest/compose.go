package manifest

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"maps"
	"math"
	"math/big"
	"reflect"
	"sort"
	"strconv"

	"github.com/skosovsky/prompty"
	"github.com/skosovsky/prompty/internal/late"
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
//nolint:golines,tagalign // layer wire struct uses wide aligned field columns
type RawLayer struct {
	ID          string               `json:"id" yaml:"id"`
	Role        string               `json:"role,omitempty" yaml:"role,omitempty"`
	LayerKind   prompty.LayerKind    `json:"layer_kind,omitempty" yaml:"layer_kind,omitempty"`
	ImportRef   string               `json:"import_ref,omitempty" yaml:"import_ref,omitempty"`
	Content     []RawContentPart     `json:"content,omitempty" yaml:"content,omitempty"`
	Optional    bool                 `json:"optional,omitempty" yaml:"optional,omitempty"`
	CachePolicy *prompty.CachePolicy `json:"cache_policy,omitempty" yaml:"cache_policy,omitempty"`
	Metadata    map[string]any       `json:"metadata,omitempty" yaml:"metadata,omitempty"`
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

func strictEqual(a, b any) bool {
	if ar, ok := numericRat(a); ok {
		br, bok := numericRat(b)
		return bok && ar.Cmp(br) == 0
	}
	return reflect.DeepEqual(a, b)
}

func numericRat(v any) (*big.Rat, bool) {
	switch n := v.(type) {
	case int:
		return new(big.Rat).SetInt64(int64(n)), true
	case int8:
		return new(big.Rat).SetInt64(int64(n)), true
	case int16:
		return new(big.Rat).SetInt64(int64(n)), true
	case int32:
		return new(big.Rat).SetInt64(int64(n)), true
	case int64:
		return new(big.Rat).SetInt64(n), true
	case uint:
		return new(big.Rat).SetUint64(uint64(n)), true
	case uint8:
		return new(big.Rat).SetUint64(uint64(n)), true
	case uint16:
		return new(big.Rat).SetUint64(uint64(n)), true
	case uint32:
		return new(big.Rat).SetUint64(uint64(n)), true
	case uint64:
		return new(big.Rat).SetUint64(n), true
	case float32:
		return finiteFloatRat(float64(n))
	case float64:
		return finiteFloatRat(n)
	case json.Number:
		r, ok := new(big.Rat).SetString(n.String())
		return r, ok
	default:
		return nil, false
	}
}

func finiteFloatRat(n float64) (*big.Rat, bool) {
	if math.IsNaN(n) || math.IsInf(n, 0) {
		return nil, false
	}
	r, ok := new(big.Rat).SetString(strconv.FormatFloat(n, 'g', -1, 64))
	return r, ok
}

// PeekComposeOrError reports whether manifest bytes declare imports or layers.
// Unmarshal errors return (false, err) — corrupt bytes are not treated as compose.
func PeekComposeOrError(data []byte, u Unmarshaler) (bool, error) {
	return PeekComposeFieldsE(data, u)
}

// PeekComposeFieldsE reports whether manifest bytes declare imports or layers.
// Unmarshal errors return (false, err) — corrupt bytes are not treated as compose.
func PeekComposeFieldsE(data []byte, u Unmarshaler) (bool, error) {
	if len(data) == 0 || u == nil {
		return false, nil
	}
	var raw RawManifest
	if err := u.Unmarshal(data, &raw); err != nil {
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

// MergeInputSchemas unions local and import input schemas (codegen uses conservative union).
//
//nolint:gocognit // import merge walks properties and late-flag propagation rules
func MergeInputSchemas(local *prompty.SchemaDefinition, imports ...*RawManifest) (*prompty.SchemaDefinition, error) {
	localProps, err := inputProperties(local)
	if err != nil {
		return nil, err
	}
	merged := maps.Clone(localProps)
	for _, imp := range imports {
		if imp == nil {
			continue
		}
		impProps, propErr := inputProperties(imp.InputSchema)
		if propErr != nil {
			return nil, propErr
		}
		for name, prop := range impProps {
			if existing, ok := merged[name]; ok {
				if !samePropertyType(existing, prop) {
					return nil, fmt.Errorf(
						"merge inputs: property %q has conflicting type between manifests",
						name,
					)
				}
				exM, _ := existing.(map[string]any)
				impM, _ := prop.(map[string]any)
				if late.PropertyIsLate(impM) && !late.PropertyIsLate(exM) {
					merged[name] = mergeLateFlagIntoProperty(exM, impM)
				}
				continue
			}
			merged[name] = prop
		}
	}
	if len(merged) == 0 {
		return local, nil
	}
	required, reqErr := mergeRequired(local, imports...)
	if reqErr != nil {
		return nil, reqErr
	}
	schema := map[string]any{
		"type":       "object",
		"properties": merged,
	}
	if len(required) > 0 {
		schema["required"] = required
	}
	doc, err := prompty.MapToJSONDocument(schema)
	if err != nil {
		return nil, err
	}
	return &prompty.SchemaDefinition{Schema: doc}, nil
}

func inputProperties(schema *prompty.SchemaDefinition) (map[string]any, error) {
	if schema == nil || len(schema.Schema) == 0 {
		return map[string]any{}, nil
	}
	doc, err := prompty.JSONDocumentAsMap(schema.Schema)
	if err != nil {
		return nil, fmt.Errorf("merge inputs: decode schema properties: %w", err)
	}
	props, _ := doc["properties"].(map[string]any)
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

func samePropertyType(a, b any) bool {
	am, aOK := a.(map[string]any)
	bm, bOK := b.(map[string]any)
	if !aOK || !bOK {
		return reflect.DeepEqual(a, b)
	}
	at, _ := am["type"].(string)
	bt, _ := bm["type"].(string)
	if at != bt {
		return false
	}
	af, _ := am["format"].(string)
	bf, _ := bm["format"].(string)
	if af != bf {
		return false
	}
	return late.PropertyIsLate(am) == late.PropertyIsLate(bm)
}

func mergeLateFlagIntoProperty(existing, importProp map[string]any) map[string]any {
	out := maps.Clone(existing)
	if out == nil {
		out = make(map[string]any)
	}
	if v, ok := importProp["late"].(bool); ok && v {
		out["late"] = true
	}
	if v, ok := importProp["x-prompty-late"].(bool); ok && v {
		out["x-prompty-late"] = true
	}
	return out
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
	if err := json.Unmarshal(data, &dst); err != nil {
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
		out = append(out, RawMessage{
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
