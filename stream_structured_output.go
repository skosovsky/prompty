package prompty

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"iter"
	"net/url"
	"slices"
)

const streamBufferPreviewLimit = 1024

var streamMaxObjectSizeBytes = 10 * 1024 * 1024

// ErrStructuredStream identifies invalid incremental JSON syntax/shape, not
// provider refusal or voluntary consumer cancellation.
var ErrStructuredStream = errors.New("prompty: invalid structured stream")

type streamJSONMode int

const (
	streamJSONModeUnknown streamJSONMode = iota
	streamJSONModeObject
	streamJSONModeArray
)

// StreamStructuredOutput yields provisional JSON objects. They are final only
// after successful iterator exhaustion and whole-document validation.
func StreamStructuredOutput[T any](
	ctx context.Context, invoker Invoker, exec *PromptExecution, mode StreamMode,
) iter.Seq2[T, error] {
	return func(yield func(T, error) bool) {
		var zero T

		if invoker == nil {
			yield(zero, errors.New("stream structured output: invoker is nil"))
			return
		}

		workExec, err := prepareStructuredStreamExecution[T](exec)
		if err != nil {
			yield(zero, err)
			return
		}
		validator, err := compileSchemaSnapshot(
			workExec.Validator,
			workExec.ResponseFormat.Schema,
			workExec.ResponseFormat.Dialect,
		)
		if err != nil {
			yield(zero, err)
			return
		}
		if validator == nil {
			yield(zero, ErrSchemaValidatorRequired)
			return
		}

		streamCtx, cancel := context.WithCancel(ctx)
		defer cancel()

		parser := newStructuredStreamParser[T]()
		if boundaryErr := validateDispatchBoundary(streamCtx, workExec); boundaryErr != nil {
			yield(zero, boundaryErr)
			return
		}
		stream := invoker.ExecuteStream(streamCtx, workExec.Clone(), mode)
		if stream == nil {
			yield(zero, ErrStreamProtocol)
			return
		}
		defer func() { _ = stream.Close() }()
		var pending []T
		skipContent := false
		if bindErr := stream.attachValidation(func(chunk *ResponseChunk) error {
			pending = nil
			if chunk.Outcome != "" && chunk.Outcome != OutcomeCompleted {
				skipContent = true
			}
			if skipContent {
				return nil // retain subsequent usage; failure is classified at finalization
			}
			if len(chunk.Content) == 0 {
				return nil
			}
			texts, textErr := strictTextPartsFromContent(chunk.Content)
			if textErr != nil {
				return textErr
			}
			for _, text := range texts {
				items, parseErr := parser.feed(text)
				if parseErr != nil {
					return parseErr
				}
				pending = append(pending, items...)
			}
			return nil
		}, func(_ context.Context, response *Response) error {
			if response.Outcome != OutcomeCompleted {
				return &OutcomeError{Outcome: response.Outcome, FinishReason: response.FinishReason}
			}
			if finishErr := parser.finish(); finishErr != nil {
				return finishErr
			}
			document, textErr := response.StrictText()
			if textErr != nil {
				return textErr
			}
			return validator.ValidateJSON(JSONDocument(document))
		}); bindErr != nil {
			yield(zero, bindErr)
			return
		}
		for chunk, err := range stream.Events() {
			if err != nil {
				cancel()
				yield(zero, err)
				return
			}
			if chunk == nil {
				cancel()
				yield(zero, errors.New("stream structured output: nil chunk"))
				return
			}
			if chunk.Kind == StreamTerminal {
				continue
			}
			// Role/start/finish and usage-only chunks carry no content. They are
			// protocol events, not evidence of an empty model response.
			if len(chunk.Content) == 0 {
				continue
			}
			for _, item := range pending {
				if !yield(item, nil) {
					cancel()
					return
				}
			}
		}
	}
}

// Without an explicit document schema the stream accepts one T object or an
// array of T objects. Explicit schemas describe the whole document, not an item.
func prepareStructuredStreamExecution[T any](exec *PromptExecution) (*PromptExecution, error) {
	working, err := prepareStructuredExecution[T](exec)
	if err != nil || exec.ResponseFormat != nil {
		return working, err
	}
	itemSchema, err := schemaForStructuredType[T]()
	if err != nil {
		return nil, err
	}
	documentSchema := map[string]any{
		schemaKeywordAnyOf: []any{
			itemSchema,
			map[string]any{schemaKeywordType: schemaTypeArray, schemaKeywordItems: itemSchema},
		},
	}
	if schemaHasReferenceScope(itemSchema) {
		// Give the item its own resource boundary so its local refs still address
		// the original item root, and include that resource only once.
		originalDocument, marshalErr := MapToJSONDocument(itemSchema)
		if marshalErr != nil {
			return nil, marshalErr
		}
		wrapperURI := fmt.Sprintf("%s?structured-stream=%x", schemaSourceURI, sha256.Sum256(originalDocument))
		sourceBase, _ := url.Parse(schemaSourceURI)
		declaredID, _ := itemSchema["$id"].(string)
		itemID, parseErr := url.Parse(declaredID)
		if parseErr != nil {
			return nil, fmt.Errorf("structured stream item id: %w", parseErr)
		}
		itemSchema["$id"] = sourceBase.ResolveReference(itemID).String()
		definitionsKeyword := "$defs"
		if working.ResponseFormat.Dialect == SchemaDialect7 {
			definitionsKeyword = "definitions"
		}
		reference := map[string]any{schemaKeywordRef: "#/" + definitionsKeyword + "/item"}
		documentSchema = map[string]any{
			"$id":              wrapperURI,
			definitionsKeyword: map[string]any{"item": itemSchema},
			schemaKeywordAnyOf: []any{
				reference,
				map[string]any{schemaKeywordType: schemaTypeArray, schemaKeywordItems: reference},
			},
		}
	}
	document, err := MapToJSONDocument(documentSchema)
	if err != nil {
		return nil, err
	}
	working.ResponseFormat.Schema = document
	return working, nil
}

func strictTextPartsFromContent(parts []ContentPart) ([]string, error) {
	if len(parts) == 0 {
		return nil, ErrEmptyTextResponse
	}
	out := make([]string, 0, len(parts))
	for _, part := range parts {
		switch x := part.(type) {
		case TextPart:
			if x.Text != "" {
				out = append(out, x.Text)
			}
		case *TextPart:
			if x == nil {
				return nil, ErrEmptyTextResponse
			}
			if x.Text != "" {
				out = append(out, x.Text)
			}
		default:
			return nil, &NonTextResponseError{PartKinds: []string{contentPartKind(part)}}
		}
	}
	if len(out) == 0 {
		return nil, ErrEmptyTextResponse
	}
	return out, nil
}

type structuredStreamParser[T any] struct {
	mode            streamJSONMode
	started         bool
	completed       bool
	capturing       bool
	depth           int
	inString        bool
	escape          bool
	current         bytes.Buffer
	preview         bytes.Buffer
	arrayHasItem    bool
	arrayNeedsValue bool
}

func newStructuredStreamParser[T any]() *structuredStreamParser[T] {
	return &structuredStreamParser[T]{}
}

func (p *structuredStreamParser[T]) feed(text string) ([]T, error) {
	var out []T
	for i := range len(text) {
		ch := text[i]

		if p.completed {
			if isJSONWhitespace(ch) {
				continue
			}
			return nil, p.errorf("unexpected trailing data after structured output")
		}

		if !p.started {
			if isJSONWhitespace(ch) {
				continue
			}
			if ch == '`' {
				return nil, p.errorf("markdown code fences are not supported (JSON-only)")
			}
			switch ch {
			case '{':
				p.started = true
				p.mode = streamJSONModeObject
				p.startCapture(ch)
			case '[':
				p.started = true
				p.mode = streamJSONModeArray
				p.appendPreviewByte(ch)
			default:
				return nil, p.errorf("unexpected prefix byte %q before JSON payload", ch)
			}
			continue
		}

		switch p.mode {
		case streamJSONModeObject:
			item, completed, err := p.consumeObjectByte(ch)
			if err != nil {
				return nil, err
			}
			if completed {
				out = append(out, item)
			}
		case streamJSONModeArray:
			items, err := p.consumeArrayByte(ch)
			if err != nil {
				return nil, err
			}
			out = append(out, items...)
		default:
			return nil, p.errorf("unknown JSON stream mode")
		}
	}
	return out, nil
}

func (p *structuredStreamParser[T]) finish() error {
	if !p.started {
		return ErrEmptyTextResponse
	}
	if p.capturing || p.depth != 0 {
		return p.errorf("incomplete JSON in stream")
	}
	if p.started && !p.completed {
		return p.errorf("incomplete JSON in stream")
	}
	return nil
}

func isJSONWhitespace(ch byte) bool {
	switch ch {
	case ' ', '\n', '\r', '\t':
		return true
	default:
		return false
	}
}

func (p *structuredStreamParser[T]) consumeObjectByte(ch byte) (T, bool, error) {
	var zero T

	if !p.capturing {
		return zero, false, p.errorf("object stream lost capture state")
	}

	if err := p.writeCapturedByte(ch); err != nil {
		return zero, false, err
	}
	if p.depth != 0 {
		return zero, false, nil
	}

	item, err := p.decodeCurrent()
	if err != nil {
		return zero, false, err
	}
	p.capturing = false
	p.completed = true
	return item, true, nil
}

func (p *structuredStreamParser[T]) consumeArrayByte(ch byte) ([]T, error) {
	if !p.capturing {
		if isJSONWhitespace(ch) {
			return nil, nil
		}
		switch ch {
		case ',':
			if !p.arrayHasItem || p.arrayNeedsValue {
				return nil, p.errorf("unexpected comma in JSON array stream")
			}
			p.arrayNeedsValue = true
			return nil, nil
		case ']':
			if p.arrayNeedsValue {
				return nil, p.errorf("trailing comma in JSON array stream")
			}
			p.appendPreviewByte(ch)
			p.completed = true
			return nil, nil
		case '{':
			if p.arrayHasItem && !p.arrayNeedsValue {
				return nil, p.errorf("missing comma in JSON array stream")
			}
			p.startCapture(ch)
			return nil, nil
		default:
			return nil, p.errorf("unsupported non-object item %q in JSON array stream", ch)
		}
	}

	if err := p.writeCapturedByte(ch); err != nil {
		return nil, err
	}
	if p.depth != 0 {
		return nil, nil
	}

	item, err := p.decodeCurrent()
	if err != nil {
		return nil, err
	}
	p.capturing = false
	p.arrayHasItem = true
	p.arrayNeedsValue = false
	return []T{item}, nil
}

func (p *structuredStreamParser[T]) startCapture(ch byte) {
	p.capturing = true
	p.depth = 1
	p.inString = false
	p.escape = false
	p.current.Reset()
	p.current.WriteByte(ch)
	p.appendPreviewByte(ch)
}

func (p *structuredStreamParser[T]) writeCapturedByte(ch byte) error {
	if streamMaxObjectSizeBytes > 0 && p.current.Len()+1 > streamMaxObjectSizeBytes {
		p.appendPreviewByte(ch)
		return p.errorf("stream object too large (limit: %d bytes)", streamMaxObjectSizeBytes)
	}
	p.current.WriteByte(ch)
	p.appendPreviewByte(ch)

	if p.inString {
		if p.escape {
			p.escape = false
			return nil
		}
		switch ch {
		case '\\':
			p.escape = true
		case '"':
			p.inString = false
		}
		return nil
	}

	switch ch {
	case '"':
		p.inString = true
	case '{', '[':
		p.depth++
	case '}', ']':
		p.depth--
		if p.depth < 0 {
			return p.errorf("invalid JSON nesting while streaming structured output")
		}
	}
	return nil
}

func (p *structuredStreamParser[T]) decodeCurrent() (T, error) {
	var item T

	raw := p.current.String()
	dec := json.NewDecoder(bytes.NewReader([]byte(raw)))
	dec.UseNumber()
	dec.DisallowUnknownFields()
	if err := dec.Decode(&item); err != nil {
		return item, p.errorf("failed to decode streamed JSON object: %w", err)
	}
	if err := dec.Decode(&struct{}{}); err != io.EOF {
		if err == nil {
			return item, p.errorf("trailing JSON after streamed object")
		}
		return item, p.errorf("failed to decode streamed JSON object: %w", err)
	}
	if isNilStructuredValue(item) {
		return item, p.errorf("decoded nil streamed result")
	}
	if err := validateStructuredValue(&item); err != nil {
		return item, &ValidationError{
			RawAssistantMessage: nil,
			FeedbackPrompt:      semanticFeedbackText(err),
			Err:                 fmt.Errorf("stream structured output: semantic validation failed: %w", err),
		}
	}
	return item, nil
}

func (p *structuredStreamParser[T]) appendPreviewByte(ch byte) {
	p.preview.WriteByte(ch)
	if p.preview.Len() <= streamBufferPreviewLimit {
		return
	}

	tail := append([]byte(nil), p.preview.Bytes()[p.preview.Len()-streamBufferPreviewLimit:]...)
	p.preview.Reset()
	p.preview.Write(tail)
}

func (p *structuredStreamParser[T]) previewText() string {
	if p.preview.Len() <= streamBufferPreviewLimit {
		return p.preview.String()
	}
	return p.preview.String()[p.preview.Len()-streamBufferPreviewLimit:]
}

func (p *structuredStreamParser[T]) errorf(format string, args ...any) error {
	preview := p.previewText()
	if preview == "" && p.current.Len() > 0 {
		preview = p.current.String()
		if len(preview) > streamBufferPreviewLimit {
			preview = preview[len(preview)-streamBufferPreviewLimit:]
		}
	}
	values := append([]any{ErrStructuredStream}, args...)
	values = append(values, preview)
	return fmt.Errorf("%w: stream structured output: "+format+"; partial buffer tail: %q", values...)
}

func schemaHasReferenceScope(value any) bool {
	switch node := value.(type) {
	case map[string]any:
		for key, child := range node {
			switch key {
			case schemaKeywordRef, schemaKeywordDynamicRef, "$recursiveRef", "$id", "$anchor", "$dynamicAnchor":
				return true
			}
			if schemaHasReferenceScope(child) {
				return true
			}
		}
	case []any:
		if slices.ContainsFunc(node, schemaHasReferenceScope) {
			return true
		}
	}
	return false
}
