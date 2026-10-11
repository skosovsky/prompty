package gemini

import (
	"encoding/json"
	"strconv"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/skosovsky/prompty"
)

func partialFixture(values ...string) []json.RawMessage {
	fragments := make([]json.RawMessage, len(values))
	for index, value := range values {
		fragments[index] = json.RawMessage(value)
	}
	return fragments
}

func TestPartialArgumentsNestedValuesAndStringContinuation(t *testing.T) {
	t.Parallel()
	// Arrange: arrays arrive out of order; number lexemes must remain exact.
	var arguments partialArguments
	first := partialFixture(
		`{"jsonPath":"$.title","stringValue":"warm ","willContinue":true}`,
		`{"jsonPath":"$.items[1].n","numberValue":9007199254740993}`,
		`{"jsonPath":"$.items[0].n","numberValue":1.234567890123456789}`,
		`{"jsonPath":"$.nil","nullValue":null}`,
		`{"jsonPath":"$.flag","boolValue":false}`,
		`{"jsonPath":"$['dot.key']['']","stringValue":""}`,
	)
	// Act: the open string is provisional; a valueless marker closes it.
	require.NoError(t, arguments.accept(first))
	_, err := arguments.complete()
	require.ErrorIs(t, err, prompty.ErrIncompleteToolCallArgs)
	require.NoError(t, arguments.accept(partialFixture(
		`{"jsonPath":"$.title","stringValue":"weather"}`,
	)))
	completed, err := arguments.complete()
	// Assert: false/null/empty string are present values, not dropped defaults.
	require.NoError(t, err)
	value := geminiJSONValue([]byte(completed)).(map[string]any)
	require.Equal(t, "warm weather", value["title"])
	require.Equal(t, false, value["flag"])
	_, nullPresent := value["nil"]
	require.True(t, nullPresent)
	require.Nil(t, value["nil"])
	require.Empty(t, value["dot.key"].(map[string]any)[""])
}

func TestPartialArgumentsExactCompletedDocument(t *testing.T) {
	t.Parallel()
	// Arrange: path aliases identify one open string, never separate arguments.
	var arguments partialArguments
	require.NoError(t, arguments.accept(partialFixture(
		`{"jsonPath":"$.title","stringValue":"warm ","willContinue":true}`,
		`{"jsonPath":"$.items[1].n","numberValue":9007199254740993}`,
		`{"jsonPath":"$.items[0].n","numberValue":1.234567890123456789}`,
		`{"jsonPath":"$.nil","nullValue":null}`,
		`{"jsonPath":"$.flag","boolValue":false}`,
	)))
	// Act.
	require.NoError(t, arguments.accept(partialFixture(
		`{"jsonPath":"$[\"title\"]","stringValue":"weather","willContinue":true}`,
		`{"jsonPath":"$.title"}`,
	)))
	completed, err := arguments.complete()
	// Assert: JSONEq would round these numbers; compare Number-decoded values.
	require.NoError(t, err)
	require.Equal(
		t,
		geminiJSONValue(
			[]byte(
				`{"title":"warm weather","items":[{"n":1.234567890123456789},{"n":9007199254740993}],"nil":null,"flag":false}`,
			),
		),
		geminiJSONValue([]byte(completed)),
	)
}

func TestPartialArgumentsInvalidBatchIsTransactional(t *testing.T) {
	t.Parallel()
	for _, invalid := range []string{
		`{"jsonPath":"$.old","numberValue":2}`,
		`{"jsonPath":"$.old.child","stringValue":"bad"}`,
		`{"jsonPath":"$.new","stringValue":"text","numberValue":1}`,
		`{"jsonPath":"$.new","boolValue":false,"willContinue":true}`,
		`{"jsonPath":"$.new","stringValue":null}`,
		`{"jsonPath":"$.new","numberValue":"1"}`,
		`{"jsonPath":"$.new","nullValue":"UNKNOWN"}`,
		`{"jsonPath":"$.new","willContinue":null}`,
		`{"jsonPath":"$.new"}`,
		`{"jsonPath":"$.new","private-extension":true}`,
		`{"JsonPath":"$.new","numberValue":1}`,
		`{"jsonPath":"$.new","NumberValue":1}`,
		`{"jsonPath":"$.new","stringValue":"a","stringValue":"b"}`,
		`{"jsonPath":"$..new","numberValue":1}`,
		`{"jsonPath":"$.list[*]","numberValue":1}`,
	} {
		t.Run(invalid, func(t *testing.T) {
			t.Parallel()
			// Arrange: an invalid second field must roll back the whole update batch.
			var arguments partialArguments
			require.NoError(t, arguments.accept(partialFixture(`{"jsonPath":"$.old","numberValue":1}`)))
			before, err := arguments.complete()
			require.NoError(t, err)
			// Act.
			err = arguments.accept(partialFixture(`{"jsonPath":"$.acceptedOnlyIfAtomic","boolValue":true}`, invalid))
			// Assert: no partial mutation or payload-bearing error.
			require.ErrorIs(t, err, prompty.ErrStreamProtocol)
			require.NotContains(t, err.Error(), "private")
			after, completeErr := arguments.complete()
			require.NoError(t, completeErr)
			require.Equal(t, before, after)
		})
	}
}

func TestPartialArgumentsSparseIndexCannotAllocateFromUntrustedMagnitude(t *testing.T) {
	t.Parallel()
	// Arrange: one sparse entry, enormous index; no array allocation by index.
	var arguments partialArguments
	index := strconv.Itoa(int(^uint(0) >> 1))
	require.NoError(t, arguments.accept(partialFixture(`{"jsonPath":"$.items[`+index+`]","numberValue":1}`)))
	// Act.
	_, err := arguments.complete()
	// Assert: missing slots are not fabricated nulls or an enormous allocation.
	require.ErrorIs(t, err, prompty.ErrIncompleteToolCallArgs)
	require.Len(t, arguments.root.members["items"].elements, 1)
}

func TestPartialPathRejectsDepthBeyondJSONValidator(t *testing.T) {
	t.Parallel()
	// Arrange: a single string must not create an unbounded recursive tree.
	path := "$" + strings.Repeat(".x", 10001)
	// Act.
	_, err := parseArgumentPath(path)
	// Assert: reject before cloning/materializing that tree.
	require.ErrorIs(t, err, prompty.ErrStreamProtocol)
}

func TestPartialPathQuotedKeysAndInvalidQueries(t *testing.T) {
	t.Parallel()
	// Arrange/Act/Assert: singular paths only, correct escaping and exact keys.
	for _, path := range []string{`$.root[0]["a.b"]`, `$['a\'b']["quote\"key"]`, `$.ключ['']`} {
		steps, err := parseArgumentPath(path)
		require.NoError(t, err)
		require.NotEmpty(t, steps)
	}
	for _, path := range []string{"$", "$[0]", "$.1bad", "$..key", "$['bad]", "$[+1]", "$[01]", "$[-1]", "$.array[0:2]", "$[?(@.x)]"} {
		_, err := parseArgumentPath(path)
		require.ErrorIs(t, err, prompty.ErrStreamProtocol)
	}
}
