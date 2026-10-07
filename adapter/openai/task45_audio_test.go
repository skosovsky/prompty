package openai

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/skosovsky/prompty"
	"github.com/skosovsky/prompty/adapter"
)

func TestAudioFormatAdmission(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		mime string
		want string
	}{
		{"audio/mp3", "mp3"}, {"audio/mpeg", "mp3"},
		{"audio/wav", "wav"}, {"audio/wave", "wav"}, {"audio/x-wav", "wav"},
		{"AUDIO/WAV; rate=44100", "wav"},
		{"audio/ogg", ""}, {"audio/", ""}, {"", ""}, {"audio/mp3; broken", ""},
	} {
		t.Run(tc.mime, func(t *testing.T) {
			t.Parallel()
			// Arrange.
			exec := &prompty.PromptExecution{Messages: []prompty.ChatMessage{{
				Role: prompty.RoleUser,
				Content: []prompty.ContentPart{prompty.MediaPart{
					MediaType: "audio", MIMEType: tc.mime, Data: []byte{1, 2, 3},
				}},
			}}}
			// Act.
			params, err := New().Translate(exec)
			// Assert: unsupported formats do not produce a native audio request.
			if tc.want == "" {
				require.ErrorIs(t, err, adapter.ErrUnsupportedContentType)
				return
			}
			require.NoError(t, err)
			parts := params.Messages[0].OfUser.Content.OfArrayOfContentParts
			require.Len(t, parts, 1)
			require.Equal(t, tc.want, parts[0].GetInputAudio().Format)
			require.Equal(t, "AQID", parts[0].GetInputAudio().Data)
		})
	}
}
