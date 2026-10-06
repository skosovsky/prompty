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
	if m.base.responseClient == nil {
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
	if handle.Identity.Provider != "gemini" || handle.Identity.Endpoint != m.base.RuntimeDescriptor().Endpoint ||
		handle.AccountBinding != m.accountBinding {
		return zero, prompty.ErrCacheResourceScope
	}
	exec := prompty.SimplePrompt("")
	exec.ModelOptions = &prompty.ModelOptions{Model: handle.Identity.Model}
	report, err := m.preflight(exec, capability)
	if err == nil && report.Profile != handle.Identity {
		return report, prompty.ErrCacheResourceScope
	}
	return report, err
}

// Create maps immutable canonical input and performs one cache creation request.
// Generation-only options are rejected; lifecycle capture is caller-owned.
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
	config := &genai.CreateCachedContentConfig{
		Contents: translated.Contents, SystemInstruction: translated.Config.SystemInstruction,
		Tools: translated.Config.Tools, ToolConfig: translated.Config.ToolConfig,
		TTL: input.Expiry.TTL, ExpireTime: input.Expiry.ExpiresAt,
	}
	config.HTTPOptions, err = exactContentOptions(config.HTTPOptions, config.Contents, "contents")
	if err != nil {
		return nil, err
	}
	immutable := *config
	immutable.TTL, immutable.ExpireTime = 0, time.Time{}
	immutable.HTTPOptions = nil // Dispatch encoding is not immutable cache content.
	encoded, err := json.Marshal(immutable)
	if err != nil {
		return nil, prompty.ErrCacheResource
	}
	digest := sha256.Sum256(encoded)
	handle := prompty.CacheResourceHandle{
		Format: prompty.CacheResourceWireFormat, Identity: report.Profile, AccountBinding: m.accountBinding,
		ContentDigest: hex.EncodeToString(digest[:]), State: prompty.CacheResourceUnknown,
	}
	result := &prompty.CacheResourceResult{Diagnostics: report}
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
		map[string]any{"operation": "cache.create", "execution": wire, "expiry": input.Expiry},
		map[string]any{"operation": "cache.create", "model": translated.Model, "config": config},
	); captureErr != nil {
		return capture.finish(ctx, result, captureErr)
	}
	source := &responseSource{limit: m.base.responseLimit}
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
	_, err = m.base.responseClient.Caches.Create(
		context.WithValue(ctx, responseSourceKey{}, source),
		translated.Model,
		config,
	)
	if err != nil {
		if source.status >= 200 && source.status < 300 {
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
	result := &prompty.CacheResourceResult{Handle: &handle, Diagnostics: report}
	capture, captureErr := newCacheOperationCapture("cache.inspect", report, options...)
	if captureErr != nil {
		return result, captureErr
	}
	if captureErr = capture.before(ctx,
		map[string]any{"operation": "cache.inspect", "handle": handle},
		map[string]any{"operation": "cache.inspect", "name": handle.ProviderID},
	); captureErr != nil {
		return capture.finish(ctx, result, captureErr)
	}
	if contextErr := ctx.Err(); contextErr != nil {
		return capture.finish(ctx, result, contextErr)
	}
	source := &responseSource{limit: m.base.responseLimit}
	capture.source, capture.dispatched = source, true
	_, err = m.base.responseClient.Caches.Get(
		context.WithValue(ctx, responseSourceKey{}, source),
		handle.ProviderID,
		nil,
	)
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
	result := &prompty.CacheResourceResult{Handle: &handle, Diagnostics: report}
	config := &genai.UpdateCachedContentConfig{TTL: expiry.TTL, ExpireTime: expiry.ExpiresAt}
	capture, captureErr := newCacheOperationCapture("cache.update_expiry", report, options...)
	if captureErr != nil {
		return result, captureErr
	}
	if captureErr = capture.before(ctx,
		map[string]any{"operation": "cache.update_expiry", "handle": handle, "expiry": expiry},
		map[string]any{"operation": "cache.update_expiry", "name": handle.ProviderID, "config": config},
	); captureErr != nil {
		return capture.finish(ctx, result, captureErr)
	}
	if contextErr := ctx.Err(); contextErr != nil {
		return capture.finish(ctx, result, contextErr)
	}
	if expiryErr := prompty.ValidateCacheResourceExpiry(expiry, time.Now()); expiryErr != nil {
		return capture.finish(ctx, result, expiryErr)
	}
	source := &responseSource{limit: m.base.responseLimit}
	capture.source, capture.dispatched = source, true
	handle.State = prompty.CacheResourceUnknown
	_, err = m.base.responseClient.Caches.Update(
		context.WithValue(ctx, responseSourceKey{}, source),
		handle.ProviderID,
		config,
	)
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
	result := &prompty.CacheResourceResult{Handle: &handle, Diagnostics: report}
	capture, captureErr := newCacheOperationCapture("cache.delete", report, options...)
	if captureErr != nil {
		return result, captureErr
	}
	if captureErr = capture.before(ctx,
		map[string]any{"operation": "cache.delete", "handle": handle},
		map[string]any{"operation": "cache.delete", "name": handle.ProviderID},
	); captureErr != nil {
		return capture.finish(ctx, result, captureErr)
	}
	if contextErr := ctx.Err(); contextErr != nil {
		return capture.finish(ctx, result, contextErr)
	}
	source := &responseSource{limit: m.base.responseLimit}
	capture.source, capture.dispatched = source, true
	handle.State = prompty.CacheResourceUnknown
	_, err = m.base.responseClient.Caches.Delete(
		context.WithValue(ctx, responseSourceKey{}, source),
		handle.ProviderID,
		nil,
	)
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
	if source.overflow || !source.complete {
		return result, prompty.ErrCacheResourceUnknown
	}
	payload := source.data.Bytes()
	if !json.Valid(payload) || uniqueGeminiJSONValue(json.NewDecoder(bytes.NewReader(payload))) != nil {
		return result, prompty.ErrCacheResourceUnknown
	}
	var observed struct {
		Name          string          `json:"name"`
		Model         string          `json:"model"`
		ExpireTime    time.Time       `json:"expireTime"`
		UsageMetadata json.RawMessage `json:"usageMetadata"`
	}
	if !exactCacheResourceFields(payload, []string{"name", "model", "expireTime", "usageMetadata"}) {
		return result, prompty.ErrCacheResourceUnknown
	}
	if err := json.Unmarshal(payload, &observed); err != nil || !cacheResourceName.MatchString(observed.Name) ||
		observed.ExpireTime.IsZero() ||
		strings.TrimPrefix(observed.Model, "models/") != strings.TrimPrefix(handle.Identity.Model, "models/") ||
		handle.ProviderID != "" && observed.Name != handle.ProviderID {
		return result, prompty.ErrCacheResourceUnknown
	}
	handle.ProviderID = observed.Name
	handle.ExpiresAt = observed.ExpireTime
	handle.State = prompty.CacheResourceUnknown
	result.Handle = &handle
	if len(observed.UsageMetadata) > 0 && !bytes.Equal(observed.UsageMetadata, []byte("null")) {
		if !exactCacheResourceFields(observed.UsageMetadata, []string{"totalTokenCount"}) {
			return result, prompty.ErrCacheResourceUnknown
		}
		var usage struct {
			TotalTokenCount *int64 `json:"totalTokenCount"`
		}
		if decodeErr := json.Unmarshal(observed.UsageMetadata, &usage); decodeErr != nil ||
			usage.TotalTokenCount != nil && *usage.TotalTokenCount < 0 {
			return result, prompty.ErrCacheResourceUnknown
		}
		result.TokenCount = usage.TotalTokenCount
	}
	handle.State = prompty.CacheResourceActive
	if !handle.ExpiresAt.After(time.Now()) {
		handle.State = prompty.CacheResourceExpired
	}
	return result, nil
}

func exactCacheResourceFields(payload []byte, fields []string) bool {
	var object map[string]json.RawMessage
	if json.Unmarshal(payload, &object) != nil || object == nil {
		return false
	}
	for key := range object {
		for _, field := range fields {
			if strings.EqualFold(key, field) && key != field {
				return false
			}
		}
	}
	return true
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
