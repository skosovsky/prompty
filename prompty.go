package prompty

import (
	"context"
	"fmt"
	"reflect"
	"time"
)

// Role is the message role in a chat (system, developer, user, assistant, tool).
type Role string

// LayerKind identifies a prompt layer category for composition/replacement.
type LayerKind string

// Chat message roles.
const (
	RoleSystem    Role = "system"
	RoleDeveloper Role = "developer" // Replaces system for OpenAI o1/o3-style models
	RoleUser      Role = "user"
	RoleAssistant Role = "assistant"
	RoleTool      Role = "tool"
)

// ContentPart is a sealed interface for message parts. Only package types implement it via isContentPart().
//
// Contract: All ProviderAdapter implementations of ParseResponse MUST return
// a []ContentPart slice containing only value types (e.g. TextPart, not *TextPart).
// Consumers can rely on this and need no defensive checks for pointer vs value.
type ContentPart interface {
	isContentPart()
}

// CachePolicy declares cache behavior for a message or content part.
// Type examples: "ephemeral".
type CachePolicy struct {
	Type string `json:"type,omitempty" yaml:"type,omitempty"`
}

// MessageProvenance tracks manifest/layer origin for a rendered message.
type MessageProvenance struct {
	ManifestID string `json:"manifest_id,omitempty" yaml:"manifest_id,omitempty"`
	LayerID    string `json:"layer_id,omitempty"    yaml:"layer_id,omitempty"`
}

// TextPart holds plain text content.
type TextPart struct {
	Text        string
	CachePolicy *CachePolicy `json:"cache_policy,omitempty" yaml:"cache_policy,omitempty"`
}

func (TextPart) isContentPart() {}

// MediaPart holds universal media (image, audio, video, document). URL or Data may be set.
// Adapters that do not accept URL natively may require callers to resolve URLs into inline data first.
type MediaPart struct {
	MediaType   string       // "image", "audio", "video", "document"
	MIMEType    string       // e.g. "application/pdf", "image/jpeg"
	URL         string       // Optional: link (adapters may fetch and convert to inline)
	Data        []byte       // Optional: raw bytes (base64 is decoded by adapters as needed)
	CachePolicy *CachePolicy `json:"cache_policy,omitempty" yaml:"cache_policy,omitempty"`
}

func (MediaPart) isContentPart() {}

// ReasoningPart is the hidden reasoning chain returned by some models (e.g. DeepSeek R1, OpenAI o-series).
type ReasoningPart struct {
	Text        string
	CachePolicy *CachePolicy `json:"cache_policy,omitempty" yaml:"cache_policy,omitempty"`
}

func (ReasoningPart) isContentPart() {}

// ToolCallPart represents an AI request to call a function (in assistant message).
// In streaming: ArgsChunk holds incremental JSON; Args is set in non-stream ParseResponse.
type ToolCallPart struct {
	ID          string // Empty for models that do not support ID (e.g. base Gemini)
	Name        string
	Args        string       // Full JSON string of arguments (non-stream response)
	ArgsChunk   string       // Chunk of JSON arguments (streaming); client glues chunks
	CachePolicy *CachePolicy `json:"cache_policy,omitempty" yaml:"cache_policy,omitempty"`
}

func (ToolCallPart) isContentPart() {}

// ToolResultPart is the result of a tool call (in message with Role "tool").
// Content is a slice of multimodal parts (text, images, etc.).
type ToolResultPart struct {
	ToolCallID  string
	Name        string
	Content     []ContentPart
	IsError     bool
	CachePolicy *CachePolicy `json:"cache_policy,omitempty" yaml:"cache_policy,omitempty"`
}

func (ToolResultPart) isContentPart() {}

// ChatMessage is a single message with role and content parts (supports multimodal).
// CachePolicy hints providers to cache this message (e.g. ephemeral prompt caching).
// Provenance records manifest/layer origin; Metadata is for provider-specific extras only.
//
//nolint:golines // ChatMessage tag block kept readable alongside CachePolicy/Provenance
type ChatMessage struct {
	Role        Role
	Content     []ContentPart
	CachePolicy *CachePolicy       `json:"cache_policy,omitempty" yaml:"cache_policy,omitempty"`
	Provenance  *MessageProvenance `json:"provenance,omitempty"   yaml:"provenance,omitempty"`
	Metadata    JSONDocument       // Provider-specific message-scoped extras (JSON object)
	LayerKind   LayerKind          `json:"layer_kind,omitempty" yaml:"layer_kind,omitempty"`
}

// ToolDefinition is the universal tool schema.
// JSON tags are required for template functions (e.g. render_tools_as_json) that marshal tools.
//
//nolint:tagalign,golines // tag columns are kept close to existing public DTO style.
type ToolDefinition struct {
	Name         string       `json:"name"                 yaml:"name"`
	Description  string       `json:"description"          yaml:"description"`
	Parameters   JSONDocument `json:"parameters,omitempty" yaml:"parameters,omitempty"` // JSON Schema for parameters
	Capabilities []string     `json:"capabilities,omitempty" yaml:"capabilities,omitempty"`
}

// SchemaDefinition describes a structured output (JSON Schema) for response format.
type SchemaDefinition struct {
	Name        string       `json:"name,omitempty"        yaml:"name,omitempty"`
	Description string       `json:"description,omitempty" yaml:"description,omitempty"`
	Schema      JSONDocument `json:"schema"                yaml:"schema"` // JSON Schema
}

// PromptMetadata holds observability metadata.
// Known fields: ID, Version, Description, Tags, Environment.
// Extras holds arbitrary keys from manifest metadata block for tracing/custom middleware.
type PromptMetadata struct {
	ID           string       `json:"id"`
	Version      string       `json:"version,omitempty"`
	Description  string       `json:"description,omitempty"`
	Tags         []string     `json:"tags,omitempty"`
	Capabilities []string     `json:"capabilities,omitempty"`
	Environment  string       `json:"environment,omitempty"`
	Extras       JSONDocument `json:"extras,omitempty"`
}

// ModelOptions holds typed, cross-provider model settings for one template/execution.
// ProviderSettings preserves provider-specific manifest keys without requiring generic SDK mapping.
type ModelOptions struct {
	Model            string       `json:"model,omitempty"             yaml:"model,omitempty"`
	Temperature      *float64     `json:"temperature,omitempty"       yaml:"temperature,omitempty"`
	MaxTokens        *int64       `json:"max_tokens,omitempty"        yaml:"max_tokens,omitempty"`
	TopP             *float64     `json:"top_p,omitempty"             yaml:"top_p,omitempty"`
	Stop             []string     `json:"stop,omitempty"              yaml:"stop,omitempty"`
	ProviderSettings JSONDocument `json:"provider_settings,omitempty" yaml:"provider_settings,omitempty"`
}

// PromptExecution is the result of formatting a template; immutable after creation.
type PromptExecution struct {
	Messages       []ChatMessage
	Tools          []ToolDefinition
	RequiredTools  []string
	ForcedTool     string
	ModelOptions   *ModelOptions
	Metadata       PromptMetadata
	ResponseFormat *SchemaDefinition `json:"response_format,omitempty" yaml:"response_format,omitempty"`
}

// NewExecution creates a new prompt execution from a set of messages.
func NewExecution(messages []ChatMessage) *PromptExecution {
	return &PromptExecution{
		Messages:      cloneMessages(messages),
		RequiredTools: []string{},
	}
}

// WithHistory returns a new execution with cloned history messages appended.
func (e *PromptExecution) WithHistory(history []ChatMessage) *PromptExecution {
	if e == nil {
		return nil
	}
	messages := append(cloneMessages(e.Messages), cloneMessages(history)...)
	return cloneExecutionWithMessages(e, messages)
}

// AddMessage returns a new execution with one cloned message appended.
func (e *PromptExecution) AddMessage(msg ChatMessage) *PromptExecution {
	if e == nil {
		return nil
	}
	messages := append(cloneMessages(e.Messages), cloneChatMessage(msg))
	return cloneExecutionWithMessages(e, messages)
}

// WithMessages returns a new execution with provided messages replacing current history.
func (e *PromptExecution) WithMessages(messages []ChatMessage) *PromptExecution {
	if e == nil {
		return nil
	}
	return cloneExecutionWithMessages(e, cloneMessages(messages))
}

// Normalize returns a new PromptExecution with consecutive system/developer messages merged into one.
// Content is merged: TextPart texts are concatenated with "\n\n"; other parts (e.g. MediaPart) are preserved in order.
// Provenance, Metadata, and LayerKind use first-wins semantics from the leading message; trailing values are dropped.
// Adjacent messages are not merged when LayerID differs (including empty vs non-empty).
// Call explicitly when history may have produced adjacent system messages (e.g. to avoid provider 400).
func (e *PromptExecution) Normalize() *PromptExecution {
	if e == nil || len(e.Messages) == 0 {
		return e
	}
	var out []ChatMessage
	for i := 0; i < len(e.Messages); i++ {
		cur := e.Messages[i]
		if cur.Role != RoleSystem && cur.Role != RoleDeveloper {
			out = append(out, cloneChatMessage(cur))
			continue
		}
		// Merge consecutive system/developer messages unless layer provenance differs.
		merged := cloneChatMessage(cur)
		for j := i + 1; j < len(e.Messages) && (e.Messages[j].Role == RoleSystem || e.Messages[j].Role == RoleDeveloper); j++ {
			if provenanceLayerIDConflicts(merged, e.Messages[j]) {
				break
			}
			merged = mergeSystemMessages(merged, e.Messages[j])
			i = j
		}
		out = append(out, merged)
	}
	return &PromptExecution{
		Messages:       out,
		Tools:          cloneToolDefinitions(e.Tools),
		RequiredTools:  cloneStringSlice(e.RequiredTools),
		ForcedTool:     e.ForcedTool,
		ModelOptions:   cloneModelOptions(e.ModelOptions),
		Metadata:       clonePromptMetadata(e.Metadata),
		ResponseFormat: cloneSchemaDefinition(e.ResponseFormat),
	}
}

// mergeSystemMessages merges two system/developer messages while preserving content-part boundaries.
// Provenance, Metadata, and LayerKind are taken from a (first wins).
// If either source has nil CachePolicy or cache types mismatch, merged message CachePolicy is nil.
func mergeSystemMessages(a, b ChatMessage) ChatMessage {
	content := make([]ContentPart, 0, len(a.Content)+len(b.Content)+1)
	content = append(content, cloneContentParts(a.Content)...)
	if hasTextContent(a.Content) && hasTextContent(b.Content) {
		content = append(content, TextPart{Text: "\n\n"})
	}
	content = append(content, cloneContentParts(b.Content)...)
	return ChatMessage{
		Role:        a.Role,
		Content:     content,
		CachePolicy: mergeMessageCachePolicy(a.CachePolicy, b.CachePolicy),
		Provenance:  cloneMessageProvenance(a.Provenance),
		Metadata:    CloneJSONDocument(a.Metadata),
		LayerKind:   a.LayerKind,
	}
}

func hasTextContent(parts []ContentPart) bool {
	for _, p := range parts {
		switch p.(type) {
		case TextPart, *TextPart:
			return true
		}
	}
	return false
}

func provenanceLayerID(msg ChatMessage) string {
	if msg.Provenance == nil {
		return ""
	}
	return msg.Provenance.LayerID
}

func provenanceLayerIDConflicts(a, b ChatMessage) bool {
	aID := provenanceLayerID(a)
	bID := provenanceLayerID(b)
	return aID != bID
}

func mergeMessageCachePolicy(a, b *CachePolicy) *CachePolicy {
	if a == nil || b == nil {
		return nil
	}
	if a.Type != b.Type {
		return nil
	}
	return cloneCachePolicy(a)
}

// Fetcher defines how media URLs are resolved into raw bytes. Callers can use mediafetch.DefaultFetcher or provide a custom implementation (e.g. S3, local files).
type Fetcher interface {
	Fetch(ctx context.Context, url string) (data []byte, mimeType string, err error)
}

// ResolvedMedia returns a cloned execution where MediaParts with URL and empty Data are fetched via Fetcher.
// MIME type is populated from fetcher response; callers can use this for image/audio/video/document URLs.
func (e *PromptExecution) ResolvedMedia(
	ctx context.Context,
	fetcher Fetcher,
) (*PromptExecution, error) {
	if e == nil {
		return nil, nil
	}
	out := e.Clone()
	for i, msg := range out.Messages {
		for j, part := range msg.Content {
			mp, ok := part.(MediaPart)
			if !ok {
				continue
			}
			if mp.URL == "" || len(mp.Data) > 0 {
				continue
			}
			if isNilFetcher(fetcher) {
				return nil, fmt.Errorf("resolve media %s: %w", mp.URL, ErrNoFetcher)
			}
			data, contentType, err := fetcher.Fetch(ctx, mp.URL)
			if err != nil {
				return nil, fmt.Errorf("resolve media %s: %w", mp.URL, err)
			}
			mp.Data = data
			mp.MIMEType = contentType
			out.Messages[i].Content[j] = mp
		}
	}
	return out, nil
}

func isNilFetcher(fetcher Fetcher) bool {
	if fetcher == nil {
		return true
	}
	value := reflect.ValueOf(fetcher)
	switch value.Kind() {
	case reflect.Pointer, reflect.Interface, reflect.Map, reflect.Slice, reflect.Func:
		return value.IsNil()
	default:
		return false
	}
}

// NewSystemMessage creates a single system message with text content.
func NewSystemMessage(text string) ChatMessage {
	return ChatMessage{
		Role:    RoleSystem,
		Content: []ContentPart{TextPart{Text: text}},
	}
}

// NewUserMessage creates a single user message with text content.
func NewUserMessage(text string) ChatMessage {
	return ChatMessage{
		Role:    RoleUser,
		Content: []ContentPart{TextPart{Text: text}},
	}
}

// NewAssistantMessage creates a single assistant message with text content.
func NewAssistantMessage(text string) ChatMessage {
	return ChatMessage{
		Role:    RoleAssistant,
		Content: []ContentPart{TextPart{Text: text}},
	}
}

func newAssistantMessageWithContent(content []ContentPart) ChatMessage {
	return ChatMessage{
		Role:    RoleAssistant,
		Content: cloneContentParts(content),
	}
}

func newToolMessageWithContent(content []ContentPart) ChatMessage {
	return ChatMessage{
		Role:    RoleTool,
		Content: cloneContentParts(content),
	}
}

func newToolResultPart(toolCallID, name, text string, isError bool) ToolResultPart {
	return ToolResultPart{
		ToolCallID: toolCallID,
		Name:       name,
		Content:    []ContentPart{TextPart{Text: text}},
		IsError:    isError,
	}
}

// TemplatePart is one part of a message template (text or media). Type determines which field set is the template source.
type TemplatePart struct {
	Type        string       // "text" or "media"
	Text        string       // Go text/template for type "text"
	MediaType   string       // Go text/template for type "media" (for example: image, audio, video, document)
	MIMEType    string       // Optional Go text/template for type "media" (for example: image/png)
	URL         string       // Optional Go text/template for type "media"
	CachePolicy *CachePolicy `json:"cache_policy,omitempty" yaml:"cache_policy,omitempty"`
}

// TextContent returns a single text TemplatePart slice for convenience.
func TextContent(text string) []TemplatePart {
	return []TemplatePart{{Type: "text", Text: text}}
}

// MessageTemplate is the raw template for one message before rendering.
// After RenderPlan.Execute it becomes a ChatMessage with substituted values.
// Optional: true skips the message if all referenced variables are zero-value.
// CachePolicy applies message-level cache hint; parts may override with their own cache_policy.
type MessageTemplate struct {
	Role        Role           // RoleSystem, RoleUser, RoleAssistant (and others; see Role* constants)
	Content     []TemplatePart // Parts to render (text and/or media); each part is a Go text/template
	Optional    bool           // true → skip if all referenced variables are zero-value
	CachePolicy *CachePolicy   `json:"cache_policy,omitempty" yaml:"cache_policy,omitempty"`
	Metadata    JSONDocument   `json:"metadata,omitempty"     yaml:"metadata,omitempty"`
	LayerID     string         `json:"layer_id,omitempty"     yaml:"layer_id,omitempty"`
	LayerKind   LayerKind      `json:"layer_kind,omitempty"   yaml:"layer_kind,omitempty"`
}

// TemplateInfo holds metadata about a template without parsing its body.
type TemplateInfo struct {
	ID        string
	Version   string
	UpdatedAt time.Time
}

// TemplateDescriptor is manifest metadata without template compilation or input binding.
type TemplateDescriptor struct {
	Metadata          PromptMetadata
	ModelOptions      *ModelOptions
	Tools             []ToolDefinition
	RequiredTools     []string
	RequiredInputVars []string
	InputSchema       *SchemaDefinition
	ResponseFormat    *SchemaDefinition
	LayerIDs          []string
	Capabilities      []string
	Tags              []string
}

// ResolveManifestOpts configures manifest metadata resolution (e.g. runtime compose context).
type ResolveManifestOpts struct {
	composeValues ComposeValues
}

// ResolveManifestOption configures ResolveManifest. Without options, composed manifests use a
// conservative view (union of all imports) suitable for codegen.
type ResolveManifestOption func(*ResolveManifestOpts)

// WithResolveComposeValues sets typed runtime values for condition.match during descriptor expansion.
// Passing an intentionally empty ComposeValues enables strict runtime compose evaluation.
func WithResolveComposeValues(values ComposeValues) ResolveManifestOption {
	return func(o *ResolveManifestOpts) {
		if !values.IsSet() {
			return
		}
		o.composeValues = values
	}
}

// WithResolveComposeContext sets typed runtime compose context during descriptor expansion.
func WithResolveComposeContext(ctx ComposeContext) ResolveManifestOption {
	return func(o *ResolveManifestOpts) {
		if ctx == nil {
			return
		}
		values := ctx.ComposeValues()
		if values.IsSet() {
			o.composeValues = values
		}
	}
}

// ComposeValues returns typed runtime compose values and whether they were set.
func (o ResolveManifestOpts) ComposeValues() (ComposeValues, bool) {
	if !o.composeValues.IsSet() {
		return ComposeValues{}, false
	}
	return o.composeValues, true
}

// ApplyResolveManifestOptions merges resolve options.
func ApplyResolveManifestOptions(opts []ResolveManifestOption) ResolveManifestOpts {
	var out ResolveManifestOpts
	for _, opt := range opts {
		if opt != nil {
			opt(&out)
		}
	}
	return out
}

// ManifestResolver resolves manifest metadata without rendering.
type ManifestResolver interface {
	ResolveManifest(ctx context.Context, id string, opts ...ResolveManifestOption) (TemplateDescriptor, error)
}

// PromptDescriber exposes declarative prompt introspection for host routing.
type PromptDescriber interface {
	DescribePrompt(ctx context.Context, id string) (TemplateDescriptor, error)
}

// ManifestBytesReader supplies raw manifest bytes for digest computation.
type ManifestBytesReader interface {
	ReadManifestBytes(ctx context.Context, id string) ([]byte, error)
}

// ManifestComposeChecker reports whether a manifest uses imports/layers (compose-aware caching).
// Corrupt manifest bytes must return a non-nil error (never silently report false).
type ManifestComposeChecker interface {
	ManifestUsesComposeE(ctx context.Context, id string) (bool, error)
}

// DescribingRegistry is required by prompty-gen NewPromptCatalog (plan + DescribePrompt).
type DescribingRegistry interface {
	Registry
	PromptDescriber
}

// PromptCatalogRegistry is required by prompty-gen recipe/index APIs.
type PromptCatalogRegistry interface {
	DescribingRegistry
	ManifestCheckpointRegistry
}

// Registry returns a chat prompt template by id.
// id is a single identifier (e.g. "doctor", "doctor.prod"); environments are expressed via file layout.
type Registry interface {
	Plan(ctx context.Context, id string, input RegistryPlanInput) (*RenderPlan, error)
}

// ManifestCheckpointRegistry supplies registry Plan, raw bytes, and manifest checkpoint recommend/verify.
type ManifestCheckpointRegistry interface {
	Registry
	ManifestBytesReader
	RecommendManifestDescriptor(ctx context.Context, id string) (ManifestDescriptor, error)
	VerifyManifestDescriptor(ctx context.Context, desc ManifestDescriptor) error
}

// Lister is optional. When implemented by a registry, List returns available template ids.
type Lister interface {
	List(ctx context.Context) ([]string, error)
}

// Statter is optional. When implemented by a registry, Stat returns template metadata without parsing the body.
type Statter interface {
	Stat(ctx context.Context, id string) (TemplateInfo, error)
}
