package prompty

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"iter"
)

const streamBufferPreviewLimit = 1024

var streamMaxObjectSizeBytes = 10 * 1024 * 1024

type streamJSONMode int

const (
	streamJSONModeUnknown streamJSONMode = iota
	streamJSONModeObject
	streamJSONModeArray
)

// StreamStructuredOutput streams structured JSON objects from ExecuteStream.
func StreamStructuredOutput[T any](ctx context.Context, invoker Invoker, exec *PromptExecution) iter.Seq2[T, error] {
	return func(yield func(T, error) bool) {
		var zero T

		if invoker == nil {
			yield(zero, errors.New("stream structured output: invoker is nil"))
			return
		}

		workExec, err := prepareStructuredExecution[T](exec)
		if err != nil {
			yield(zero, err)
			return
		}

		streamCtx, cancel := context.WithCancel(ctx)
		defer cancel()

		parser := newStructuredStreamParser[T]()
		for chunk, err := range invoker.ExecuteStream(streamCtx, workExec) {
			if err != nil {
				cancel()
				yield(zero, err)
				return
			}
			if chunk == nil {
				continue
			}

			for _, text := range textPartsFromContent(chunk.Content) {
				items, parseErr := parser.feed(text)
				if parseErr != nil {
					cancel()
					yield(zero, parseErr)
					return
				}
				for _, item := range items {
					if !yield(item, nil) {
						cancel()
						return
					}
				}
			}
		}

		if err := parser.finish(); err != nil {
			cancel()
			yield(zero, err)
		}
	}
}

func textPartsFromContent(parts []ContentPart) []string {
	out := make([]string, 0, len(parts))
	for _, part := range parts {
		switch x := part.(type) {
		case TextPart:
			if x.Text != "" {
				out = append(out, x.Text)
			}
		case *TextPart:
			if x != nil && x.Text != "" {
				out = append(out, x.Text)
			}
		}
	}
	return out
}

type structuredStreamParser[T any] struct {
	mode          streamJSONMode
	started       bool
	completed     bool
	skippingFence bool
	capturing     bool
	depth         int
	inString      bool
	escape        bool
	current       bytes.Buffer
	preview       bytes.Buffer
}

func newStructuredStreamParser[T any]() *structuredStreamParser[T] {
	return &structuredStreamParser[T]{}
}

func (p *structuredStreamParser[T]) feed(text string) ([]T, error) {
	var out []T
	for i := range len(text) {
		ch := text[i]

		if p.completed {
			if p.consumeFenceOrWhitespace(ch) {
				continue
			}
			return nil, p.errorf("unexpected trailing data after structured output")
		}

		if !p.started {
			if p.consumeFenceOrWhitespace(ch) {
				continue
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
	if p.capturing || p.depth != 0 {
		return p.errorf("incomplete JSON in stream")
	}
	if p.started && !p.completed {
		return p.errorf("incomplete JSON in stream")
	}
	return nil
}

func (p *structuredStreamParser[T]) consumeFenceOrWhitespace(ch byte) bool {
	if p.skippingFence {
		if ch == '\n' {
			p.skippingFence = false
		}
		return true
	}
	switch ch {
	case ' ', '\n', '\r', '\t':
		return true
	case '`':
		p.skippingFence = true
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
		switch ch {
		case ' ', '\n', '\r', '\t', ',':
			return nil, nil
		case ']':
			p.appendPreviewByte(ch)
			p.completed = true
			return nil, nil
		case '{':
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
	if err := json.Unmarshal([]byte(raw), &item); err != nil {
		return item, p.errorf("failed to decode streamed JSON object: %v", err)
	}
	if isNilStructuredValue(item) {
		return item, p.errorf("decoded nil streamed result")
	}
	if err := validateStructuredValue(&item); err != nil {
		return item, p.errorf("semantic validation failed: %v", err)
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
	return fmt.Errorf("stream structured output: "+format+"; partial buffer tail: %q", append(args, preview)...)
}
