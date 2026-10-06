package gemini

import (
	"bufio"
	"bytes"
	"errors"
	"io"
)

// streamSource is invocation-local. The SDK scanner receives at most one line
// per body read, so read-ahead cannot associate a later payload with this event.
// This observes source JSON only; it does not claim original transport bytes.
type streamSource struct {
	limit    int
	payload  []byte
	status   int
	failure  error
	validate func([]byte) error
}

type streamSourceKey struct{}

type streamSourceBody struct {
	io.ReadCloser

	reader   *bufio.Reader
	source   *streamSource
	pending  []byte
	terminal error
}

func (b *streamSourceBody) Read(dst []byte) (int, error) {
	if len(dst) == 0 {
		return 0, nil
	}
	if len(b.pending) == 0 {
		if b.terminal != nil {
			return 0, b.terminal
		}
		line, err := readStreamSourceLine(b.reader, b.source.limit)
		if err != nil && !errors.Is(err, io.EOF) {
			b.terminal = err
			b.source.failure = err
			return 0, err
		}
		b.pending, b.terminal = line, err
		if len(line) == 0 {
			return 0, err
		}
		prefix, data, ok := bytes.Cut(bytes.TrimRight(line, "\r\n"), []byte(":"))
		if ok && bytes.Equal(prefix, []byte("data")) {
			b.source.payload = bytes.Clone(bytes.TrimPrefix(data, []byte(" ")))
			if b.source.validate != nil {
				if validationErr := b.source.validate(b.source.payload); validationErr != nil {
					b.source.failure, b.terminal = validationErr, validationErr
					b.pending = nil
					return 0, validationErr
				}
			}
		}
	}
	n := copy(dst, b.pending)
	b.pending = b.pending[n:]
	return n, nil
}

// The bound includes SSE framing. Never expose a truncated line to the SDK.
// Fixed reader scratch plus bounded retained data avoids limit+1 overflow.
func readStreamSourceLine(reader *bufio.Reader, limit int) ([]byte, error) {
	if limit <= 0 {
		return nil, io.ErrShortBuffer
	}
	var line []byte
	for {
		fragment, err := reader.ReadSlice('\n')
		if len(fragment) > limit-len(line) {
			return nil, io.ErrShortBuffer
		}
		line = append(line, fragment...)
		if errors.Is(err, bufio.ErrBufferFull) {
			continue
		}
		return line, err
	}
}
