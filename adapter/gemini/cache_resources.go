package gemini

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"regexp"
	"strings"
	"time"

	"google.golang.org/genai"

	"github.com/skosovsky/prompty"
	"github.com/skosovsky/prompty/adapter"
)

// CacheResources performs explicit Gemini cached-content lifecycle operations.
// Account binding is host metadata, not an IAM mechanism. No store or worker is created.
type CacheResources struct {
	base           *Adapter
	accountBinding string
}

// NewCacheResources injects ordinary Gemini transport/profile options plus a
// secret-free host account reference. Calls do not renew/recreate resources.
func NewCacheResources(accountBinding string, opts ...Option) *CacheResources {
	return &CacheResources{base: New(opts...), accountBinding: accountBinding}
}

var cacheResourceName = regexp.MustCompile(
	`^(cachedContents/[A-Za-z0-9_-]+|projects/[A-Za-z0-9_-]+/locations/[A-Za-z0-9_-]+/cachedContents/[A-Za-z0-9_-]+)$`,
)

func (m *CacheResources) preflight(
	exec *prompty.PromptExecution,
	capability string,
) (prompty.ExecutionDiagnostics, error) {
	var zero prompty.ExecutionDiagnostics
	if strings.TrimSpace(m.accountBinding) == "" {
		return zero, prompty.ErrCacheResourceScope
	}
	if m.base.client == nil {
		return zero, adapter.ErrNoClient
	}
	if m.base.responseLimit <= 0 {
		return zero, prompty.ErrCacheResource
	}
	desc := m.base.RuntimeDescriptor()
	if strings.TrimSpace(desc.Endpoint) == "" {
		return zero, adapter.ErrNoRuntimeDescriptor
	}
	working := exec.Clone()
	working.Requirements.Capabilities = append(working.Requirements.Capabilities, capability)
	if capability != prompty.CapabilityCacheCreate {
		model := ""
		if working.ModelOptions != nil {
			model = working.ModelOptions.Model
		}
		return adapter.PreflightOperation(desc, model, working.Requirements.Capabilities, working.Requirements.Override)
	}
	return adapter.PreflightInputOperation(desc, working)
}

func (m *CacheResources) forHandle(
	handle prompty.CacheResourceHandle,
	capability string,
) (prompty.ExecutionDiagnostics, error) {
	var zero prompty.ExecutionDiagnostics
	if err := prompty.ValidateCacheResourceHandle(handle); err != nil {
		return zero, err
	}
	if !cacheResourceName.MatchString(handle.ProviderID) {
		return zero, prompty.ErrCacheResource
	}
	if handle.Identity.Provider != wireGemini || handle.Identity.Endpoint != m.base.RuntimeDescriptor().Endpoint ||
		handle.AccountBinding != m.accountBinding {
		return zero, prompty.ErrCacheResourceScope
	}
	exec := prompty.SimplePrompt("")
	exec.ModelOptions = &prompty.ModelOptions{
		Temperature:      nil,
		MaxTokens:        nil,
		TopP:             nil,
		Stop:             nil,
		ProviderSettings: nil,
		Model:            handle.Identity.Model,
	}
	report, err := m.preflight(exec, capability)
	if err == nil && report.Profile != handle.Identity {
		return report, prompty.ErrCacheResourceScope
	}
	return report, err
}

// Create maps immutable canonical input and performs one cache creation request.
// Generation-only options are rejected; lifecycle capture is caller-owned.
//
//nolint:cyclop,funlen,gocognit,gocyclo // Ordered provider protocol checks preserve capture, partial-result and terminal-state semantics.
func (m *CacheResources) Create(
	ctx context.Context,
	input prompty.CacheResourceCreateRequest,
) (*prompty.CacheResourceResult, error) {
	if err := adapter.ValidateExecution(input.Execution); err != nil {
		return nil, err
	}
	if err := prompty.ValidateCacheResourceExpiry(input.Expiry, time.Now()); err != nil {
		return nil, err
	}
	exec := input.Execution.Clone()
	original := exec.Clone()
	if err := validateCacheCreateExecution(exec); err != nil {
		return nil, err
	}
	report, err := m.preflight(exec, prompty.CapabilityCacheCreate)
	if err != nil {
		return nil, err
	}
	validator := exec.Validator
	if validator == nil {
		validator = prompty.NewJSONSchemaValidator(nil)
	}
	toolNames := make(map[string]bool)
	for i := range exec.Tools {
		tool := &exec.Tools[i]
		if tool.Name == "" || toolNames[tool.Name] {
			return nil, prompty.ErrSchemaUnsupported
		}
		toolNames[tool.Name] = true
		schema := tool.Parameters
		if len(schema) == 0 {
			schema = prompty.JSONDocument(`{"type":"object"}`)
		}
		compiled, compileErr := prompty.CompileProviderSchema(prompty.SchemaCompileRequest{
			Source: schema, Dialect: tool.Dialect, Role: prompty.SchemaToolInput, Name: tool.Name,
			Profile: report.Profile, Policy: exec.Requirements.Schema, Validator: validator,
		}, m.base)
		if compileErr != nil {
			return nil, compileErr
		}
		tool.Parameters = compiled.Translated()
		report.Schemas = append(report.Schemas, compiled.Report())
	}
	translated, err := m.base.Translate(exec)
	if err != nil {
		return nil, err
	}
	ordinaryContent := false
	for _, message := range exec.Messages {
		if message.Role != prompty.RoleSystem && message.Role != prompty.RoleDeveloper {
			ordinaryContent = true
		}
	}
	if !ordinaryContent {
		translated.Contents = nil
	} // Cache creation needs no synthetic generation turn.
	config := &genai.CreateCachedContentConfig{ //nolint:exhaustruct_v5 // SDK DTO: omitted optional fields and other union arms retain their zero values.
		HTTPOptions:       noRetryHTTPOptions(nil),
		Contents:          translated.Contents,
		SystemInstruction: translated.Config.SystemInstruction,
		Tools:             translated.Config.Tools,
		ToolConfig:        translated.Config.ToolConfig,
		TTL:               input.Expiry.TTL,
		ExpireTime:        input.Expiry.ExpiresAt,
	}

	immutable := *config
	immutable.TTL, immutable.ExpireTime = 0, time.Time{}
	immutable.HTTPOptions = nil // Dispatch encoding is not immutable cache content.
	encoded, err := json.Marshal(immutable)
	if err != nil {
		return nil, prompty.ErrCacheResource
	}
	digest := sha256.Sum256(encoded)
	handle := prompty.CacheResourceHandle{ProviderID: "", ExpiresAt: time.Time{},
		Format: prompty.CacheResourceWireFormat, Identity: report.Profile, AccountBinding: m.accountBinding,
		ContentDigest: hex.EncodeToString(digest[:]), State: prompty.CacheResourceUnknown,
	}
	result := &prompty.CacheResourceResult{Handle: nil, TokenCount: nil, Diagnostics: report}
	capture, captureErr := newCacheOperationCapture("cache.create", report, prompty.CacheResourceOperationOptions{
		Capture: original.Requirements.Capture, Binding: original.Capture,
	})
	if captureErr != nil {
		return result, captureErr
	}
	var wire prompty.PromptExecutionWire
	if capture.session.Enabled() {
		wire, err = prompty.MarshalExecution(original, prompty.WirePolicy{State: prompty.StatePreserve})
		if err != nil {
			return capture.finish(ctx, result, err)
		}
	}
	if captureErr = capture.before(ctx,
		map[string]any{wireOperation: "cache.create", "execution": wire, "expiry": input.Expiry},
		map[string]any{wireOperation: "cache.create", wireModel: translated.Model, "config": config},
	); captureErr != nil {
		return capture.finish(ctx, result, captureErr)
	}
	source := &responseSource{
		data: bytes.Buffer{},
		job:  nil, cache: nil,
		overflow: false,
		complete: false,
		status:   0,
		limit:    m.base.responseLimit,
	}
	if contextErr := ctx.Err(); contextErr != nil {
		return capture.finish(ctx, result, contextErr)
	}
	if expiryErr := prompty.ValidateCacheResourceExpiry(input.Expiry, time.Now()); expiryErr != nil {
		return capture.finish(ctx, result, expiryErr)
	}
	if stateErr := m.base.ValidateProviderState(original, report.Profile); stateErr != nil {
		return capture.finish(ctx, result, stateErr)
	}
	capture.source, capture.dispatched = source, true
	raw, callErr := m.base.client.Caches.Create(
		ctx,
		translated.Model,
		config,
	)

	err = callErr
	source.status = sdkOperationHTTPStatus(err)
	source.cache = raw
	if raw != nil {
		encoded, encodingErr := json.Marshal(raw)
		if encodingErr == nil && len(encoded) <= source.limit {
			_, _ = source.data.Write(encoded)
			source.complete = true
		}
	}
	if err == nil {
		if raw == nil {
			source.complete = true
		}
	}
	if err != nil {
		if source.cache != nil {
			_, _ = cacheResourceResult(source, handle, result)
			if result.Handle != nil {
				result.Handle.State = prompty.CacheResourceUnknown
			}
		}
		return capture.finish(ctx, result, cacheOperationError(ctx, err))
	}
	return capture.parsed(ctx, source, handle, result)
}

func validateCacheCreateExecution(exec *prompty.PromptExecution) error {
	if exec.RequestCache != nil {
		return prompty.ErrCacheRequirement
	}
	if exec.CacheResource != nil {
		return prompty.ErrCacheRequirement
	}
	if exec.ResponseFormat != nil || exec.Requirements.OutputMode != "" {
		return prompty.ErrCacheRequirement
	}
	if opts := exec.ModelOptions; opts != nil &&
		(opts.Temperature != nil || opts.MaxTokens != nil || opts.TopP != nil ||
			len(opts.Stop) != 0 || len(opts.ProviderSettings) != 0) {
		return prompty.ErrCacheRequirement
	}
	if len(exec.RequiredTools) != 0 {
		return prompty.ErrCacheRequirement
	}
	return prompty.RewriteCachePolicies(exec.Messages, func(_ string, policy *prompty.CachePolicy) (bool, error) {
		if policy != nil {
			return false, prompty.ErrCacheRequirement
		}
		return true, nil
	})
}

// Get inspects a scoped handle even after its observed expiry; no recache occurs.
func (m *CacheResources) Get(
	ctx context.Context,
	handle prompty.CacheResourceHandle,
	options ...prompty.CacheResourceOperationOptions,
) (*prompty.CacheResourceResult, error) {
	report, err := m.forHandle(handle, prompty.CapabilityCacheInspect)
	if err != nil {
		return nil, err
	}
	result := &prompty.CacheResourceResult{TokenCount: nil, Handle: &handle, Diagnostics: report}
	capture, captureErr := newCacheOperationCapture("cache.inspect", report, options...)
	if captureErr != nil {
		return result, captureErr
	}
	if captureErr = capture.before(ctx,
		map[string]any{wireOperation: "cache.inspect", wireHandle: handle},
		map[string]any{wireOperation: "cache.inspect", wireName: handle.ProviderID},
	); captureErr != nil {
		return capture.finish(ctx, result, captureErr)
	}
	if contextErr := ctx.Err(); contextErr != nil {
		return capture.finish(ctx, result, contextErr)
	}
	source := &responseSource{
		data: bytes.Buffer{},
		job:  nil, cache: nil,
		overflow: false,
		complete: false,
		status:   0,
		limit:    m.base.responseLimit,
	}
	capture.source, capture.dispatched = source, true
	raw, callErr := m.base.client.Caches.Get(
		ctx,
		handle.ProviderID,
		&genai.GetCachedContentConfig{HTTPOptions: noRetryHTTPOptions(nil)},
	)

	err = callErr
	source.status = sdkOperationHTTPStatus(err)
	source.cache = raw
	if raw != nil {
		encoded, encodingErr := json.Marshal(raw)
		if encodingErr == nil && len(encoded) <= source.limit {
			_, _ = source.data.Write(encoded)
			source.complete = true
		}
	}
	if err == nil {
		if raw == nil {
			source.complete = true
		}
	}
	if err != nil {
		return capture.finish(ctx, result, cacheOperationError(ctx, err))
	}
	return capture.parsed(ctx, source, handle, result)
}

// UpdateExpiry changes only lifetime; content/config identity remains unchanged.
func (m *CacheResources) UpdateExpiry(
	ctx context.Context,
	handle prompty.CacheResourceHandle,
	expiry prompty.CacheResourceExpiry,
	options ...prompty.CacheResourceOperationOptions,
) (*prompty.CacheResourceResult, error) {
	if err := prompty.ValidateCacheResourceExpiry(expiry, time.Now()); err != nil {
		return nil, err
	}
	report, err := m.forHandle(handle, prompty.CapabilityCacheUpdate)
	if err != nil {
		return nil, err
	}
	if handle.State == prompty.CacheResourceDeleted {
		return nil, prompty.ErrCacheResourceExpired
	}
	result := &prompty.CacheResourceResult{TokenCount: nil, Handle: &handle, Diagnostics: report}
	config := &genai.UpdateCachedContentConfig{
		HTTPOptions: noRetryHTTPOptions(nil),
		TTL:         expiry.TTL,
		ExpireTime:  expiry.ExpiresAt,
	}
	capture, captureErr := newCacheOperationCapture("cache.update_expiry", report, options...)
	if captureErr != nil {
		return result, captureErr
	}
	if captureErr = capture.before(ctx,
		map[string]any{wireOperation: "cache.update_expiry", wireHandle: handle, "expiry": expiry},
		map[string]any{wireOperation: "cache.update_expiry", wireName: handle.ProviderID, "config": config},
	); captureErr != nil {
		return capture.finish(ctx, result, captureErr)
	}
	if contextErr := ctx.Err(); contextErr != nil {
		return capture.finish(ctx, result, contextErr)
	}
	if expiryErr := prompty.ValidateCacheResourceExpiry(expiry, time.Now()); expiryErr != nil {
		return capture.finish(ctx, result, expiryErr)
	}
	source := &responseSource{
		data: bytes.Buffer{},
		job:  nil, cache: nil,
		overflow: false,
		complete: false,
		status:   0,
		limit:    m.base.responseLimit,
	}
	capture.source, capture.dispatched = source, true
	handle.State = prompty.CacheResourceUnknown
	raw, callErr := m.base.client.Caches.Update(
		ctx,
		handle.ProviderID,
		config,
	)

	err = callErr
	source.status = sdkOperationHTTPStatus(err)
	source.cache = raw
	if raw != nil {
		encoded, encodingErr := json.Marshal(raw)
		if encodingErr == nil && len(encoded) <= source.limit {
			_, _ = source.data.Write(encoded)
			source.complete = true
		}
	}
	if err == nil {
		if raw == nil {
			source.complete = true
		}
	}
	if err != nil {
		handle.State = prompty.CacheResourceUnknown
		return capture.finish(ctx, result, cacheOperationError(ctx, err))
	}
	return capture.parsed(ctx, source, handle, result)
}

// Delete performs one explicit deletion. An uncertain result keeps identity for inspection.
func (m *CacheResources) Delete(
	ctx context.Context,
	handle prompty.CacheResourceHandle,
	options ...prompty.CacheResourceOperationOptions,
) (*prompty.CacheResourceResult, error) {
	report, err := m.forHandle(handle, prompty.CapabilityCacheDelete)
	if err != nil {
		return nil, err
	}
	result := &prompty.CacheResourceResult{TokenCount: nil, Handle: &handle, Diagnostics: report}
	capture, captureErr := newCacheOperationCapture("cache.delete", report, options...)
	if captureErr != nil {
		return result, captureErr
	}
	if captureErr = capture.before(ctx,
		map[string]any{wireOperation: "cache.delete", wireHandle: handle},
		map[string]any{wireOperation: "cache.delete", wireName: handle.ProviderID},
	); captureErr != nil {
		return capture.finish(ctx, result, captureErr)
	}
	if contextErr := ctx.Err(); contextErr != nil {
		return capture.finish(ctx, result, contextErr)
	}
	source := &responseSource{
		data: bytes.Buffer{},
		job:  nil, cache: nil,
		overflow: false,
		complete: false,
		status:   0,
		limit:    m.base.responseLimit,
	}
	capture.source, capture.dispatched = source, true
	handle.State = prompty.CacheResourceUnknown
	raw, callErr := m.base.client.Caches.Delete(
		ctx,
		handle.ProviderID,
		&genai.DeleteCachedContentConfig{HTTPOptions: noRetryHTTPOptions(nil)},
	)

	err = callErr
	source.status = sdkOperationHTTPStatus(err)
	if raw != nil {
		encoded, encodingErr := json.Marshal(raw)
		if encodingErr == nil && len(encoded) <= source.limit {
			_, _ = source.data.Write(encoded)
			source.complete = true
		}
	}
	if err == nil {
		if raw == nil {
			source.complete = true
		}
	}
	if err != nil || source.overflow || !source.complete {
		handle.State = prompty.CacheResourceUnknown
		return capture.finish(ctx, result, cacheOperationError(ctx, err))
	}
	handle.State = prompty.CacheResourceDeleted
	return capture.finish(ctx, result, nil)
}

func cacheOperationError(ctx context.Context, _ error) error {
	// SDK/transport error messages can contain URLs, headers or provider payload.
	// Unknown acceptance must not authorize a second mutation.
	return errors.Join(prompty.ErrCacheResourceUnknown, ctx.Err())
}

func cacheResourceResult(
	source *responseSource,
	handle prompty.CacheResourceHandle,
	result *prompty.CacheResourceResult,
) (*prompty.CacheResourceResult, error) {
	observed := source.cache
	if observed == nil || !cacheResourceName.MatchString(observed.Name) ||
		handle.ProviderID != "" && observed.Name != handle.ProviderID {
		return result, prompty.ErrCacheResourceUnknown
	}
	// Retain SDK-returned identity even if unrelated projection/validation fails.
	// This is reconciliation evidence only, never usable continuation or retry permission.
	handle.ProviderID = observed.Name
	handle.State = prompty.CacheResourceUnknown
	result.Handle = &handle
	if !observed.ExpireTime.IsZero() {
		handle.ExpiresAt = observed.ExpireTime
	}
	// SDK-GENAI-SOURCE: zero scalar has no presence evidence; keep count unknown.
	if observed.UsageMetadata != nil && observed.UsageMetadata.TotalTokenCount != 0 {
		if observed.UsageMetadata.TotalTokenCount < 0 {
			return result, prompty.ErrCacheResourceUnknown
		}
		count := int64(observed.UsageMetadata.TotalTokenCount)
		result.TokenCount = &count
	}
	if source.overflow || !source.complete || observed.ExpireTime.IsZero() ||
		strings.TrimPrefix(observed.Model, "models/") != strings.TrimPrefix(handle.Identity.Model, "models/") {
		return result, prompty.ErrCacheResourceUnknown
	}
	handle.State = prompty.CacheResourceActive
	if !handle.ExpiresAt.After(time.Now()) {
		handle.State = prompty.CacheResourceExpired
	}
	return result, nil
}

var _ prompty.CacheResourceManager = (*CacheResources)(nil)

// WithCacheAccountBinding sets a secret-free host account reference for reuse.
// It does not configure credentials or authorize access to a remote resource.
func WithCacheAccountBinding(binding string) Option {
	return func(a *Adapter) { a.cacheAccountBinding = binding }
}

// ValidateCacheResource checks current availability and complete destination binding.
func (a *Adapter) ValidateCacheResource(handle prompty.CacheResourceHandle, destination prompty.ProfileIdentity) error {
	if strings.TrimSpace(a.cacheAccountBinding) == "" {
		return prompty.ErrCacheResourceScope
	}
	if !cacheResourceName.MatchString(handle.ProviderID) {
		return prompty.ErrCacheResource
	}
	return prompty.ValidateCacheResourceReuse(handle, destination, a.cacheAccountBinding, time.Now())
}
