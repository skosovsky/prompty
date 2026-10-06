package openai

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"

	"github.com/openai/openai-go/v3/option"

	"github.com/skosovsky/prompty"
)

const remoteResponseMaxBytes = 16 * 1024 * 1024

type responseSource struct {
	data     []byte
	status   int
	complete bool
}

// The operation-local observer bounds the body before SDK decoding; it does not
// alter the injected client's auth/redirect policy or capture auth headers.
func (s *responseSource) observe(request *http.Request, next option.MiddlewareNext) (*http.Response, error) {
	response, err := next(request)
	if err != nil || response == nil {
		return response, err
	}
	s.status = response.StatusCode
	if response.Body == nil {
		return response, nil
	}
	data, readErr := io.ReadAll(io.LimitReader(response.Body, remoteResponseMaxBytes+1))
	closeErr := response.Body.Close()
	s.data = data
	s.complete = readErr == nil && len(data) <= remoteResponseMaxBytes
	if !s.complete || closeErr != nil {
		return nil, prompty.ErrRemoteResponseUnknown
	}
	response.Body = io.NopCloser(bytes.NewReader(data))
	return response, nil
}

// A single safe identity can remain known even when another duplicate/malformed
// provider field prevents SDK decoding. Multiple identity keys are never guessed.
func (s *responseSource) acceptedIdentity() string {
	if !s.complete || s.status < http.StatusOK || s.status >= http.StatusMultipleChoices || !json.Valid(s.data) {
		return ""
	}
	decoder := json.NewDecoder(bytes.NewReader(s.data))
	token, err := decoder.Token()
	if err != nil || token != json.Delim('{') {
		return ""
	}
	count, identity := 0, ""
	for decoder.More() {
		key, keyErr := decoder.Token()
		var value json.RawMessage
		if keyErr != nil || decoder.Decode(&value) != nil {
			return ""
		}
		if key == "id" {
			count++
			if json.Unmarshal(value, &identity) != nil {
				return ""
			}
		}
	}
	if count != 1 || !responseResourceID.MatchString(identity) {
		return ""
	}
	return identity
}

func retainUnknownResponseIdentity(
	source *responseSource,
	handle prompty.RemoteResponseHandle,
	result *prompty.RemoteResponseResult,
) {
	identity := source.acceptedIdentity()
	if identity == "" || (handle.ProviderID != "" && handle.ProviderID != identity) {
		return
	}
	handle.ProviderID, handle.Status = identity, prompty.RemoteResponseUnknown
	result.Handle = &handle
}
