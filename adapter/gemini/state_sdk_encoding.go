package gemini

import (
	"encoding/json"
	"fmt"
	"math/big"
	"reflect"

	"google.golang.org/genai"

	"github.com/skosovsky/prompty"
)

// SDK-GENAI-NUM: reject signed state before dispatch when SDK normalization
// would change its numeric value. No request rewrite repairs the converter.
func validateGeminiSDKStateEncoding(part *genai.Part) error {
	original, err := json.Marshal(part)
	if err != nil {
		return incompatibleGeminiSDKState()
	}
	var normalized map[string]any
	if decodeErr := json.Unmarshal(original, &normalized); decodeErr != nil {
		return incompatibleGeminiSDKState()
	}
	encoded, err := json.Marshal(normalized)
	if err != nil || !json.Valid(encoded) || !exactGeminiSDKJSONEqual(original, encoded) {
		return incompatibleGeminiSDKState()
	}
	return nil
}

// Numeric spelling is not a signed value, but rounding is. Keep this comparison
// separate from the codec's exact reconstruction check; never normalize saved state.
func exactGeminiSDKJSONEqual(original, encoded []byte) bool {
	return reflect.DeepEqual(
		geminiExactJSONNumbers(geminiJSONValue(original)),
		geminiExactJSONNumbers(geminiJSONValue(encoded)),
	)
}

type geminiExactNumber string

func geminiExactJSONNumbers(value any) any {
	switch v := value.(type) {
	case json.Number:
		if number, ok := new(big.Rat).SetString(v.String()); ok {
			return geminiExactNumber(number.RatString())
		}
		return v
	case map[string]any:
		for key, item := range v {
			v[key] = geminiExactJSONNumbers(item)
		}
	case []any:
		for index, item := range v {
			v[index] = geminiExactJSONNumbers(item)
		}
	}
	return value
}

func incompatibleGeminiSDKState() error {
	return fmt.Errorf(
		"%w: saved representation is incompatible with SDK encoding; obtain fresh provider state using SDK-representable domain values",
		prompty.ErrProviderState,
	)
}
