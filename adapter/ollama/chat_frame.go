package ollama

import (
	"bufio"
	"errors"
	"io"

	"github.com/skosovsky/prompty/adapter"
)

var errChatEOF = errors.New("ollama: chat EOF")

// ReadSlice bounds scratch space; accumulated payload never exceeds limit.
// A newline is framing, not part of the payload's limit. No limit+1 overflow.
func readChatFrame(reader *bufio.Reader, limit int) ([]byte, error) {
	var frame []byte
	for {
		part, err := reader.ReadSlice('\n')
		if len(part) > 0 && part[len(part)-1] == '\n' {
			part = part[:len(part)-1]
		}
		if len(part) > limit-len(frame) {
			return nil, adapter.ErrInvalidResponse
		}
		frame = append(frame, part...)
		switch {
		case err == nil:
			return frame, nil
		case errors.Is(err, bufio.ErrBufferFull):
			continue
		case errors.Is(err, io.EOF):
			return frame, errChatEOF
		default:
			return nil, err
		}
	}
}
