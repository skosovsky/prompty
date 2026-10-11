package hostdispatch

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"time"

	"github.com/skosovsky/contexty"

	"github.com/skosovsky/prompty"
)

const checkpointType = "host.prompty-response"

// responsePayload is a registered opaque host codec, not context text.
type responsePayload struct {
	Wire json.RawMessage `json:"wire"`
}

func (responsePayload) ExtensionType() string { return checkpointType }
func (p responsePayload) CloneExtension() contexty.Extension {
	return responsePayload{Wire: bytes.Clone(p.Wire)}
}

func checkpointProfile(destination prompty.ProfileIdentity) contexty.Descriptor {
	raw, _ := json.Marshal(destination)
	digest := sha256.Sum256(raw)
	return contexty.Descriptor{ID: hex.EncodeToString(digest[:]), Revision: "1"}
}

func checkpointCodec() contexty.MessageCodec {
	registry := contexty.NewExtensionRegistry()
	registry.RegisterOpaquePayload(checkpointType, contexty.Descriptor{ID: checkpointType, Revision: "1"},
		func(raw []byte) (contexty.Extension, error) {
			var payload responsePayload
			decoder := json.NewDecoder(bytes.NewReader(raw))
			decoder.DisallowUnknownFields()
			if err := decoder.Decode(&payload); err != nil {
				return nil, fmt.Errorf("checkpoint decode: %w", err)
			}
			if _, err := prompty.DecodeResponseJSON(payload.Wire); err != nil {
				return nil, fmt.Errorf("checkpoint response: %w", err)
			}
			return payload, nil
		})
	return contexty.MessageCodec{Provenance: contexty.DefaultProvenanceRegistry(), Extensions: registry}
}

// SaveResponse stores a canonical response in a registered context opaque codec.
// Host retains preceding history separately; this is an immutable storage carrier,
// not a context-budgeting semantic projection or a full agent checkpoint.
func SaveResponse(response *prompty.Response, destination prompty.ProfileIdentity) ([]byte, error) {
	message, err := response.AssistantMessage()
	if err != nil {
		return nil, fmt.Errorf("checkpoint assistant: %w", err)
	}
	if err = prompty.ValidateContinuation([]prompty.ChatMessage{message}, destination, time.Now()); err != nil {
		return nil, fmt.Errorf("checkpoint continuation: %w", err)
	}
	wire, err := prompty.MarshalResponse(response, prompty.WirePolicy{State: prompty.StatePreserve})
	if err != nil {
		return nil, fmt.Errorf("checkpoint wire: %w", err)
	}
	raw, err := json.Marshal(wire)
	if err != nil {
		return nil, fmt.Errorf("checkpoint marshal: %w", err)
	}
	carrier := contexty.TextMessage(contexty.RoleAssistant, "opaque response checkpoint")
	carrier.ID = "response"
	carrier.Extensions = []contexty.Extension{contexty.OpaqueState{
		ID: "response-state", Codec: contexty.Descriptor{ID: checkpointType, Revision: "1"},
		Payload: responsePayload{Wire: raw}, Placement: contexty.OpaquePlacement{AfterPart: 0},
		Binding: contexty.OpaqueBinding{Profile: checkpointProfile(destination)},
	}}
	encoded, err := contexty.MarshalMessageJSON(carrier, checkpointCodec())
	if err != nil {
		return nil, fmt.Errorf("checkpoint context encode: %w", err)
	}
	return encoded, nil
}

// RestoreResponse validates both the context binding and provider scope/expiry.
func RestoreResponse(raw []byte, destination prompty.ProfileIdentity) (*prompty.Response, error) {
	codec := checkpointCodec()
	carrier, err := contexty.UnmarshalMessageJSON(raw, codec)
	if err != nil {
		return nil, fmt.Errorf("checkpoint context decode: %w", err)
	}
	if err = contexty.ValidateOpaqueState(
		[]contexty.Message{carrier},
		contexty.JSONSerializer(codec),
		checkpointProfile(destination),
	); err != nil {
		return nil, fmt.Errorf("checkpoint binding: %w", err)
	}
	if carrier.ID != "response" || carrier.Role != contexty.RoleAssistant || len(carrier.Extensions) != 1 {
		return nil, ErrProjection
	}
	state, ok := carrier.Extensions[0].(contexty.OpaqueState)
	if !ok {
		return nil, ErrProjection
	}
	payload, ok := state.Payload.(responsePayload)
	if !ok {
		return nil, ErrProjection
	}
	response, err := prompty.DecodeResponseJSON(payload.Wire)
	if err != nil {
		return nil, fmt.Errorf("checkpoint response: %w", err)
	}
	message, err := response.AssistantMessage()
	if err != nil {
		return nil, fmt.Errorf("checkpoint assistant: %w", err)
	}
	if err = prompty.ValidateContinuation([]prompty.ChatMessage{message}, destination, time.Now()); err != nil {
		return nil, fmt.Errorf("checkpoint continuation: %w", err)
	}
	return response, nil
}
