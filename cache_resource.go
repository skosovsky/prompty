package prompty

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"errors"
	"strings"
	"time"
)

const (
	CacheResourceWireFormat = "prompty.cache_resource"
	CapabilityCacheCreate   = "cache_resource_create"
	CapabilityCacheInspect  = "cache_resource_inspect"
	CapabilityCacheUpdate   = "cache_resource_update_expiry"
	CapabilityCacheDelete   = "cache_resource_delete"
	CapabilityCacheReuse    = "cache_resource_reuse"
)

// CacheResourceState is an observation, not a durable resource guarantee.
type CacheResourceState string

const (
	CacheResourceActive  CacheResourceState = "active"
	CacheResourceExpired CacheResourceState = "expired"
	CacheResourceDeleted CacheResourceState = "deleted"
	CacheResourceUnknown CacheResourceState = "unknown"
)

var (
	ErrCacheResource        = errors.New("prompty: invalid cache resource handle")
	ErrCacheResourceScope   = errors.New("prompty: incompatible cache resource binding")
	ErrCacheResourceExpired = errors.New("prompty: cache resource expired or unavailable")
	ErrCacheResourceExpiry  = errors.New("prompty: invalid cache resource expiry")
	ErrCacheResourceUnknown = errors.New("prompty: cache resource operation outcome unknown")
)

// CacheResourceHandle persists secret-free binding data, not credentials or content.
// AccountBinding is a host stable reference; comparison is not authorization.
type CacheResourceHandle struct {
	Format         string             `json:"format"`
	Identity       ProfileIdentity    `json:"identity"`
	AccountBinding string             `json:"account_binding"`
	ProviderID     string             `json:"provider_id"`
	ContentDigest  string             `json:"content_digest"`
	ExpiresAt      time.Time          `json:"expires_at"`
	State          CacheResourceState `json:"state"`
}

// CacheResourceExpiry selects exactly one positive TTL or future absolute expiry.
type CacheResourceExpiry struct {
	TTL       time.Duration
	ExpiresAt time.Time
}

// ValidateCacheResourceExpiry validates explicit request lifetime at one instant.
func ValidateCacheResourceExpiry(expiry CacheResourceExpiry, now time.Time) error {
	if expiry.TTL < 0 || (expiry.TTL == 0) == expiry.ExpiresAt.IsZero() {
		return ErrCacheResourceExpiry
	}
	if !expiry.ExpiresAt.IsZero() && !expiry.ExpiresAt.After(now) {
		return ErrCacheResourceExpiry
	}
	return nil
}

// CacheResourceCreateRequest reuses the canonical prompt for immutable cached inputs.
type CacheResourceCreateRequest struct {
	Execution *PromptExecution
	Expiry    CacheResourceExpiry
}

// CacheResourceResult retains an observed handle even when a mutation is uncertain.
// TokenCount is an optional resource-size observation, not invocation billing usage.
type CacheResourceResult struct {
	Handle      *CacheResourceHandle `json:"handle,omitempty"`
	Diagnostics ExecutionDiagnostics `json:"diagnostics"`
	TokenCount  *int64               `json:"token_count,omitempty"`
}

// CacheResourceOperationOptions is runtime capture intent for inspect/update/delete.
// The host owns redaction, egress, persistence and concurrency of its ports.
type CacheResourceOperationOptions struct {
	Capture CaptureRequirement `json:"capture,omitzero" yaml:"capture,omitempty"`
	Binding *CaptureBinding    `json:"-"                yaml:"-"`
}

// CacheResourceManager is an optional caller-driven remote lifecycle port.
// Implementations do not provide storage, renewal loops or automatic mutation retries.
type CacheResourceManager interface {
	Create(context.Context, CacheResourceCreateRequest) (*CacheResourceResult, error)
	Get(context.Context, CacheResourceHandle, ...CacheResourceOperationOptions) (*CacheResourceResult, error)
	UpdateExpiry(
		context.Context,
		CacheResourceHandle,
		CacheResourceExpiry,
		...CacheResourceOperationOptions,
	) (*CacheResourceResult, error)
	Delete(context.Context, CacheResourceHandle, ...CacheResourceOperationOptions) (*CacheResourceResult, error)
}

// ValidateCacheResourceHandle checks persisted shape without asserting remote existence.
func ValidateCacheResourceHandle(handle CacheResourceHandle) error {
	if handle.Format != CacheResourceWireFormat || (handle.ExpiresAt.IsZero() && handle.State != CacheResourceUnknown) {
		return ErrCacheResource
	}
	if ValidateEndpointIdentity(handle.Identity.Endpoint) != nil {
		return ErrCacheResource
	}
	for _, field := range []string{handle.Identity.Provider, handle.Identity.Endpoint, handle.Identity.Model, handle.AccountBinding, handle.ProviderID} {
		if strings.TrimSpace(field) == "" {
			return ErrCacheResource
		}
	}
	digest, err := hex.DecodeString(handle.ContentDigest)
	if err != nil || len(digest) != 32 || strings.ToLower(handle.ContentDigest) != handle.ContentDigest {
		return ErrCacheResource
	}
	switch handle.State {
	case CacheResourceActive, CacheResourceExpired, CacheResourceDeleted, CacheResourceUnknown:
	default:
		return ErrCacheResource
	}
	return nil
}

// ValidateCacheResourceReuse checks binding and current availability, not IAM.
func ValidateCacheResourceReuse(
	handle CacheResourceHandle,
	destination ProfileIdentity,
	accountBinding string,
	now time.Time,
) error {
	if err := ValidateCacheResourceHandle(handle); err != nil {
		return err
	}
	if handle.Identity != destination || handle.AccountBinding != accountBinding {
		return ErrCacheResourceScope
	}
	if handle.State != CacheResourceActive || !handle.ExpiresAt.After(now) {
		return ErrCacheResourceExpired
	}
	return nil
}

// EncodeCacheResourceHandle validates and serializes the portable resource binding.
func EncodeCacheResourceHandle(handle CacheResourceHandle) (JSONDocument, error) {
	if err := ValidateCacheResourceHandle(handle); err != nil {
		return nil, err
	}
	return json.Marshal(handle)
}

// DecodeCacheResourceHandle validates strict wire shape before restoring binding.
// The host must rebind transport/account and separately authorize access.
func DecodeCacheResourceHandle(document JSONDocument) (CacheResourceHandle, error) {
	var handle CacheResourceHandle
	if err := ValidateWireJSON(document, CacheResourceWireFormat); err != nil {
		return handle, err
	}
	if err := json.Unmarshal(document, &handle); err != nil {
		return handle, err
	}
	return handle, ValidateCacheResourceHandle(handle)
}

func cloneCacheResource(handle *CacheResourceHandle) *CacheResourceHandle {
	if handle == nil {
		return nil
	}
	out := *handle
	return &out
}
