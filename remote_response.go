package prompty

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"errors"
	"maps"
	"strings"
)

const (
	RemoteResponseWireFormat       = "prompty.remote_response"
	CapabilityResponseCreate       = "response_create"
	CapabilityResponseRetrieve     = "response_retrieve"
	CapabilityResponseCancel       = "response_cancel"
	CapabilityResponseContinuation = "response_continuation"
	CapabilityResponseBackground   = "response_background"
	CapabilityHostedWebSearch      = "hosted_web_search"
	CapabilityHostedFileSearch     = "hosted_file_search"
)

type RemoteResponseStatus string

const (
	RemoteResponseQueued     RemoteResponseStatus = "queued"
	RemoteResponseRunning    RemoteResponseStatus = "in_progress"
	RemoteResponseCompleted  RemoteResponseStatus = "completed"
	RemoteResponseFailed     RemoteResponseStatus = "failed"
	RemoteResponseIncomplete RemoteResponseStatus = "incomplete"
	RemoteResponseCanceled   RemoteResponseStatus = "canceled"
	RemoteResponseUnknown    RemoteResponseStatus = "unknown"
)

var (
	ErrRemoteResponse            = errors.New("prompty: invalid remote response contract")
	ErrRemoteResponseScope       = errors.New("prompty: incompatible remote response binding")
	ErrRemoteResponseUnknown     = errors.New("prompty: remote response operation outcome unknown")
	ErrRemoteResponseUnavailable = errors.New("prompty: remote response unavailable")
)

// RemoteResponseHandle carries observed remote identity, not credentials or a lease.
// AccountBinding is a host-owned reference, not authorization.
type RemoteResponseHandle struct {
	Format         string               `json:"format"`
	Identity       ProfileIdentity      `json:"identity"`
	AccountBinding string               `json:"account_binding"`
	ProviderID     string               `json:"provider_id"`
	Status         RemoteResponseStatus `json:"status"`
	Background     bool                 `json:"background"`
	Store          bool                 `json:"store"`
	// ProviderStateRequired persists the retrieval guarantee, not provider data.
	// It does not implicitly become a new Create's execution requirement.
	ProviderStateRequired bool              `json:"provider_state_required"`
	OutputSchemaDigest    string            `json:"output_schema_digest,omitempty"`
	HostedTools           []string          `json:"hosted_tools,omitempty"`
	ToolSchemaDigests     map[string]string `json:"tool_schema_digests,omitempty"`
	ForcedTool            string            `json:"forced_tool,omitempty"`
	ToolCalls             map[string]string `json:"tool_calls,omitempty"`
}

// HostedToolRequest is an explicit provider tool, never a local handler binding.
// Settings are adapter-validated JSON; arbitrary extensions are not silently sent.
type HostedToolRequest struct {
	Kind     string       `json:"kind"`
	Settings JSONDocument `json:"settings,omitempty"`
}

type RemoteResponseRequest struct {
	Execution   *PromptExecution
	Previous    *RemoteResponseHandle
	Background  bool
	Store       *bool
	HostedTools []HostedToolRequest
}

// RemoteResponseOperationOptions contains runtime ports and original output schema
// for checked retrieval. Host must rebind these after restoring a handle.
type RemoteResponseOperationOptions struct {
	Capture      CaptureRequirement
	Binding      *CaptureBinding `json:"-" yaml:"-"`
	OutputSchema *SchemaDefinition
	Validator    SchemaValidator `json:"-" yaml:"-"`
	Tools        []ToolDefinition
}

// RemoteResponseResult separates remote lifecycle from canonical model outcome.
// Response can contain partial output on failed/incomplete operations.
type RemoteResponseResult struct {
	Handle      *RemoteResponseHandle `json:"handle,omitempty"`
	Response    *Response             `json:"-"`
	Diagnostics ExecutionDiagnostics  `json:"diagnostics"`
}

type RemoteResponseManager interface {
	Create(context.Context, RemoteResponseRequest) (*RemoteResponseResult, error)
	Get(context.Context, RemoteResponseHandle, ...RemoteResponseOperationOptions) (*RemoteResponseResult, error)
	Cancel(context.Context, RemoteResponseHandle, ...RemoteResponseOperationOptions) (*RemoteResponseResult, error)
}

func ValidateRemoteResponseHandle(handle RemoteResponseHandle) error {
	if handle.Format != RemoteResponseWireFormat || ValidateEndpointIdentity(handle.Identity.Endpoint) != nil {
		return ErrRemoteResponse
	}
	for _, value := range []string{handle.Identity.Provider, handle.Identity.Model, handle.AccountBinding, handle.ProviderID} {
		if strings.TrimSpace(value) == "" {
			return ErrRemoteResponse
		}
	}
	switch handle.Status {
	case RemoteResponseQueued, RemoteResponseRunning, RemoteResponseCompleted, RemoteResponseFailed,
		RemoteResponseIncomplete, RemoteResponseCanceled, RemoteResponseUnknown:
	default:
		return ErrRemoteResponse
	}
	if handle.OutputSchemaDigest != "" {
		decoded, err := hex.DecodeString(handle.OutputSchemaDigest)
		if err != nil || len(decoded) != 32 || strings.ToLower(handle.OutputSchemaDigest) != handle.OutputSchemaDigest {
			return ErrRemoteResponse
		}
	}
	if err := validateRemoteResponseTools(handle); err != nil {
		return err
	}
	seen := make(map[string]bool)
	for _, kind := range handle.HostedTools {
		if strings.TrimSpace(kind) == "" || seen[kind] {
			return ErrRemoteResponse
		}
		seen[kind] = true
	}
	return nil
}

func validateRemoteResponseTools(handle RemoteResponseHandle) error {
	for name, digest := range handle.ToolSchemaDigests {
		decoded, err := hex.DecodeString(digest)
		if strings.TrimSpace(name) == "" || err != nil || len(decoded) != 32 || strings.ToLower(digest) != digest {
			return ErrRemoteResponse
		}
	}
	if handle.ForcedTool != "" && handle.ToolSchemaDigests[handle.ForcedTool] == "" {
		return ErrRemoteResponse
	}
	for id, name := range handle.ToolCalls {
		if strings.TrimSpace(id) == "" || handle.ToolSchemaDigests[name] == "" {
			return ErrRemoteResponse
		}
	}
	return nil
}

// CloneRemoteResponseHandle isolates caller-owned extension collections.
func CloneRemoteResponseHandle(handle RemoteResponseHandle) RemoteResponseHandle {
	handle.HostedTools = append([]string(nil), handle.HostedTools...)
	handle.ToolSchemaDigests = maps.Clone(handle.ToolSchemaDigests)
	handle.ToolCalls = maps.Clone(handle.ToolCalls)
	return handle
}

func EncodeRemoteResponseHandle(handle RemoteResponseHandle) (JSONDocument, error) {
	if err := ValidateRemoteResponseHandle(handle); err != nil {
		return nil, err
	}
	return json.Marshal(handle)
}

func DecodeRemoteResponseHandle(document JSONDocument) (RemoteResponseHandle, error) {
	var handle RemoteResponseHandle
	if err := ValidateWireJSON(document, RemoteResponseWireFormat); err != nil {
		return handle, err
	}
	if err := json.Unmarshal(document, &handle); err != nil {
		return handle, err
	}
	return handle, ValidateRemoteResponseHandle(handle)
}
