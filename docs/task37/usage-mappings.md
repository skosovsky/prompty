> Historical task37 evidence; superseded by [current contracts](../README.md) and [migration](../migration.md). Original audit content below is retained; old API snippets and test counts are not current acceptance.

# Canonical usage mapping inventory

This inventories actual module mappings, not provider/model support guarantees. Counts are SDK-observed values; core does not infer billable tokens, cache misses or hidden reasoning from them. `Known` distinguishes explicit observed zero from absence. All mapped responses use cumulative mode. A custom source may explicitly use delta; core adds only its known counter fields with checked arithmetic, never adds breakdowns to total, and commits accepted stream updates atomically.

| Module | Presence | Primary counters and total | Breakdowns |
| --- | --- | --- | --- |
| OpenAI | SDK JSON field validity or nonzero Go fixture value; absent/null zero stays unknown | Copy prompt, completion and provider total independently. Check each int64 against nonnegative platform int range before conversion. Do not reconstruct or double total. | Copy observed cached prompt and completion reasoning independently; never add them to primary counters. Cache write is unavailable. |
| Anthropic | SDK JSON field validity or nonzero Go fixture value; absent/null zero stays unknown | Prompt = uncached input + cache read + cache creation only when all three are known. Total = those three + output only when all four are known. Checked sums reject negative or overflowing inputs before addition/conversion. Output remains independently observed. | Read/write copied individually with checked conversion. They are already included in derived prompt/total and are not added again. No token count is invented for reasoning. |
| Gemini | Nonzero SDK int32 value, or explicit matching numeric zero in SDKHTTPResponse usageMetadata. Missing/null/non-numeric/mismatching zero cannot establish presence. No metadata object means unknown usage. | Copy prompt, candidates and authoritative provider total independently. All nonnegative int32 values fit supported Go int widths. Negative counts fail checked response metadata validation. No total arithmetic. | Copy cached content and thoughts independently; never add either to provider total. Cache write is unavailable. |
| Ollama | SDK int nonzero value only: this DTO cannot prove explicit zero presence | Copy prompt evaluation/completion evaluation counters. Derive total with checked addition only when both are observed nonzero. Negative counts fail checked metadata/arithmetic validation. | No observed cache or reasoning token breakdown is exposed by this mapper; thinking text is not a token counter. |

On derived arithmetic or OpenAI conversion failure, an invalid counter is not advertised as known. Individually representable counters and known provider content/outcome survive with ErrUsage. The checked executor retains parser partial results without replay. Native OpenAI accepts the representable current frame then fails/releases transport; unread later counters do not replace the accepted snapshot. Provider capture metadata omits invalid OpenAI counts; original SDK raw JSON remains separate, and canonical terminal records parse failure.

Evidence:

- Core `usage_test.go`: partial cumulative/delta/breakdown merge, no total double count, known shape/mode/negative validation, checked overflow and atomic rejected stream updates, no tool dispatch/structured retry on invalid metadata.
- Shared `adapter/usage_sum_test.go`: nonnegative/platform bounds including exact maximum and positive-wrap sum; `adapter/usage_test.go`: sync/buffered known parser result, one dispatch, one failed capture terminal and fresh capture diagnostics.
- OpenAI `stream_contract_test.go` and `usage_range_test.go`: explicit zero vs absent; every primary/breakdown conversion, representable partial response/capture metadata, actual SDK native frame preservation and body close on both declared fixture model bindings.
- Anthropic `metadata_test.go` and `anthropic_test.go`: absent/null/partial/known-zero cache presence, mapped breakdown composition, positive-wrap prompt and total overflow with retained response.
- Gemini `usage_test.go`: numeric zero presence trust cases and all mapped counters with provider total retained without adding breakdowns.
- Ollama `metadata_test.go`: observed primary counters, absent zero, derived total, incomplete metadata and overflow with known response retained.

The range test adapts to the runtime platform width. The current verification host is arm64; this document does not claim an executed 32-bit test run. SDK decoding that loses or rejects fields before the adapter receives them is outside these canonical mappings, not evidence that the mapper observed those fields.
