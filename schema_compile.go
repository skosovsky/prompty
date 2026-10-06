package prompty

import (
	"bytes"
	"cmp"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"reflect"
	"slices"
	"strings"
)

// SchemaMode selects fidelity, never provider availability or model routing.
type SchemaMode string

const (
	SchemaStrict   SchemaMode = "strict"
	SchemaLowering SchemaMode = "lowering"
)

// SchemaRole identifies the canonical schema's use.
type SchemaRole string

const (
	SchemaOutput    SchemaRole = "output"
	SchemaToolInput SchemaRole = "tool_input"
)

// SchemaPolicy permits only named transformations. Its zero value is strict.
type SchemaPolicy struct {
	Mode              SchemaMode `json:"mode,omitempty"`
	AllowedTransforms []string   `json:"allowed_transforms,omitempty"`
}

// SchemaTransformation describes a semantic change without schema values.
// Pointer is an RFC 6901 source location; empty means the root.
type SchemaTransformation struct {
	ID      string `json:"id"`
	Pointer string `json:"pointer"`
	Reason  string `json:"reason"`
}

// SchemaCompileRequest binds source-schema compilation to one destination and role.
// Validator is runtime-only and must not load implicit external references.
type SchemaCompileRequest struct {
	Source    JSONDocument
	Dialect   string
	Role      SchemaRole
	Name      string
	Profile   ProfileIdentity
	Policy    SchemaPolicy
	Validator SchemaValidator
}

// SchemaMappingRequest is isolated from caller-owned source and policy slices.
type SchemaMappingRequest struct {
	Source  JSONDocument
	Dialect string
	Role    SchemaRole
	Profile ProfileIdentity
	Policy  SchemaPolicy
}

// SchemaMapping is an adapter's canonical translated schema and change ledger.
// SDK-specific shape conversion follows compilation in the adapter module.
type SchemaMapping struct {
	Schema          JSONDocument
	Transformations []SchemaTransformation
}

// ProviderSchemaMapper is a trusted provider port, not a JSON Schema engine.
// It must report every semantic change and must not mutate or retain input aliases.
type ProviderSchemaMapper interface {
	MapSchema(request SchemaMappingRequest) (SchemaMapping, error)
}

// SchemaCompilationReport binds exact-byte SHA-256 digests to profile and role.
// Lowering never claims lossless provider constraints; original validation is required.
type SchemaCompilationReport struct {
	Profile          ProfileIdentity        `json:"profile"`
	Role             SchemaRole             `json:"role"`
	Name             string                 `json:"name,omitempty"`
	Dialect          string                 `json:"dialect"`
	Guarantee        SchemaMode             `json:"guarantee"`
	SourceDigest     string                 `json:"source_digest"`
	TranslatedDigest string                 `json:"translated_digest"`
	Transformations  []SchemaTransformation `json:"transformations,omitempty"`
}

// CompiledProviderSchema retains immutable documents and original validation.
// A successful compile is not evidence that a live provider supports the profile.
type CompiledProviderSchema struct {
	source     JSONDocument
	translated JSONDocument
	report     SchemaCompilationReport
	validator  ValueValidator
}

// Source returns the original exact bytes without exposing mutable state.
func (c *CompiledProviderSchema) Source() JSONDocument { return CloneJSONDocument(c.source) }

// Translated returns the canonical provider schema, not SDK or wire bytes.
func (c *CompiledProviderSchema) Translated() JSONDocument { return CloneJSONDocument(c.translated) }

// Report returns an isolated compilation report.
func (c *CompiledProviderSchema) Report() SchemaCompilationReport {
	out := c.report
	out.Transformations = slices.Clone(out.Transformations)
	return out
}

// ValidateOriginal validates the complete result against the source schema.
// Domain validation and outcome checks are separate responsibilities.
func (c *CompiledProviderSchema) ValidateOriginal(value JSONDocument) error {
	if c == nil || c.validator == nil {
		return ErrSchemaValidatorRequired
	}
	return c.validator.ValidateJSON(CloneJSONDocument(value))
}

// CompileProviderSchema validates before mapping and enforces explicit fidelity policy.
// No transport is owned by this function. Provider binding must call it before dispatch.
func CompileProviderSchema(request SchemaCompileRequest, mapper ProviderSchemaMapper) (*CompiledProviderSchema, error) {
	if err := ValidateEndpointIdentity(request.Profile.Endpoint); err != nil {
		return nil, err
	}
	if mapper == nil || request.Validator == nil {
		return nil, ErrSchemaValidatorRequired
	}
	if request.Role != SchemaOutput && request.Role != SchemaToolInput {
		return nil, fidelityCompileError("invalid schema role")
	}
	if request.Profile.Provider == "" || request.Profile.Endpoint == "" || request.Profile.Model == "" {
		return nil, fidelityCompileError("schema destination is incomplete")
	}
	policy, err := normalizeSchemaPolicy(request.Policy)
	if err != nil {
		return nil, err
	}
	dialect := request.Dialect
	if dialect == "" {
		dialect = SchemaDialect2020
	}
	source := CloneJSONDocument(request.Source)
	original, err := request.Validator.CompileSchema(CloneJSONDocument(source), dialect)
	if err != nil {
		return nil, err
	}
	if original == nil {
		return nil, ErrSchemaValidatorRequired
	}
	// Do not give the mapper an alias to policy used by the enforcement guard.
	mappingPolicy := policy
	mappingPolicy.AllowedTransforms = slices.Clone(policy.AllowedTransforms)
	mapping, err := mapper.MapSchema(SchemaMappingRequest{
		Source: CloneJSONDocument(source), Dialect: dialect, Role: request.Role,
		Profile: request.Profile, Policy: mappingPolicy,
	})
	if err != nil {
		return nil, err
	}
	translated := CloneJSONDocument(mapping.Schema)
	changes := slices.Clone(mapping.Transformations)
	slices.SortFunc(changes, func(a, b SchemaTransformation) int {
		if order := cmp.Compare(a.Pointer, b.Pointer); order != 0 {
			return order
		}
		if order := cmp.Compare(a.ID, b.ID); order != 0 {
			return order
		}
		return cmp.Compare(a.Reason, b.Reason)
	})
	if err := validateSchemaMapping(source, translated, changes, policy); err != nil {
		return nil, err
	}
	compiledTranslation, compileErr := request.Validator.CompileSchema(CloneJSONDocument(translated), dialect)
	if compileErr != nil {
		return nil, compileErr
	}
	if compiledTranslation == nil {
		return nil, ErrSchemaValidatorRequired
	}
	return &CompiledProviderSchema{
		source: source, translated: translated, validator: original,
		report: SchemaCompilationReport{
			Profile: request.Profile, Role: request.Role, Name: request.Name, Dialect: dialect, Guarantee: policy.Mode,
			SourceDigest: schemaDigest(source), TranslatedDigest: schemaDigest(translated), Transformations: changes,
		},
	}, nil
}

// ValidateSchemaPolicy checks intent and permissions, not adapter implementation.
// It does not prove that a provider can satisfy this policy.
func ValidateSchemaPolicy(policy SchemaPolicy) error {
	_, err := normalizeSchemaPolicy(policy)
	return err
}

func normalizeSchemaPolicy(policy SchemaPolicy) (SchemaPolicy, error) {
	policy.AllowedTransforms = slices.Clone(policy.AllowedTransforms)
	if policy.Mode == "" {
		policy.Mode = SchemaStrict
	}
	if policy.Mode != SchemaStrict && policy.Mode != SchemaLowering {
		return policy, fidelityCompileError("invalid schema mode")
	}
	if policy.Mode == SchemaStrict && len(policy.AllowedTransforms) != 0 {
		return policy, fidelityCompileError("strict mode cannot permit transformations")
	}
	seen := make(map[string]bool, len(policy.AllowedTransforms))
	for _, id := range policy.AllowedTransforms {
		if strings.TrimSpace(id) == "" || seen[id] {
			return policy, fidelityCompileError("invalid or duplicate transformation permission")
		}
		seen[id] = true
	}
	return policy, nil
}

func validateSchemaMapping(
	source, translated JSONDocument,
	changes []SchemaTransformation,
	policy SchemaPolicy,
) error {
	unchanged, err := equivalentSchemaJSON(source, translated)
	if err != nil {
		return err
	}
	if policy.Mode == SchemaStrict && (!unchanged || len(changes) != 0) {
		pointer := ""
		if len(changes) != 0 {
			pointer = changes[0].Pointer
		}
		return &SchemaFidelityError{Pointer: pointer, Reason: "strict mapping changed the source schema"}
	}
	if !unchanged && len(changes) == 0 {
		return fidelityCompileError("changed schema has no transformation report")
	}
	for _, change := range changes {
		if !slices.Contains(policy.AllowedTransforms, change.ID) ||
			strings.TrimSpace(change.Reason) == "" || !validSchemaPointer(change.Pointer) {
			return &SchemaFidelityError{Pointer: change.Pointer, Reason: "invalid or unpermitted transformation"}
		}
	}
	return nil
}

func fidelityCompileError(reason string) error {
	return &SchemaFidelityError{Pointer: "", Reason: reason}
}

func schemaDigest(document JSONDocument) string {
	digest := sha256.Sum256(document)
	return hex.EncodeToString(digest[:])
}

func equivalentSchemaJSON(source, translated JSONDocument) (bool, error) {
	decode := func(document JSONDocument) (any, error) {
		if !json.Valid(document) {
			return nil, fidelityCompileError("schema is not a complete JSON document")
		}
		decoder := json.NewDecoder(bytes.NewReader(document))
		decoder.UseNumber()
		var value any
		if err := decoder.Decode(&value); err != nil {
			return nil, fmt.Errorf("%w: %w", ErrSchemaUnsupported, err)
		}
		return value, nil
	}
	left, err := decode(source)
	if err != nil {
		return false, err
	}
	right, err := decode(translated)
	return reflect.DeepEqual(left, right), err
}

func validSchemaPointer(pointer string) bool {
	if pointer == "" {
		return true
	}
	if pointer[0] != '/' {
		return false
	}
	for i := 0; i < len(pointer); i++ {
		if pointer[i] != '~' {
			continue
		}
		i++
		if i >= len(pointer) || (pointer[i] != '0' && pointer[i] != '1') {
			return false
		}
	}
	return true
}
