# SDK capability contract

Canonical provider results retain what the pinned official SDK exposes through
public APIs. They do not universally attest original JSON bytes or numeric
precision. Core schema/JSONDocument and typed domain decoding retain their own
Number-preserving contracts; they cannot recover data lost before mapping.

| Provider | Arguments and continuation | Usage presence |
|---|---|---|
| Gemini genai v1.72.0 | FunctionCall.Args and numeric PartialArg use SDK float64 values. Finite zero/one/fractions/nested values pass. Canonical signed state persists that representation and preserves signature bytes, part association, order and ID/name. Public SDK converter owns next request encoding. | Scalar zero and absent counters are indistinguishable: zero stays unknown, nonzero known. A present UsageMetadata object alone is insufficient. |
| Ollama v0.40.0 | ToolCallFunctionArguments.ToMap uses SDK values, including rounded float64. Canonical ordinary history persists and replays via Chat; caller json.Number outbound remains separate. Opaque provider-state translation is unavailable. | Scalar zero stays unknown; nonzero prompt/eval counters known. Available total derivation requires known operands; no invented zero or cost. |
| OpenAI v3.73.0 Chat/Responses | Public argument strings/RawJSON retain available raw numbers; native delta assembly does not use float64. | Public JSON field metadata retains explicit zero; missing stays unknown. |
| Anthropic v1.78.0 Messages | Public RawMessage/RawJSON retain available raw numbers; native input_json_delta retains JSON text. | Public JSON field metadata retains explicit zero; missing stays unknown. |

Checked validation still enforces the original admitted schema against the
available argument value: required, type, range and additionalProperties where
the provider schema port supports them. Unsupported schema mapping fails before
IO; this change does not widen provider schema support. NaN/Inf, cycles and
unsupported Go values fail JSON serialization. Malformed/incomplete arguments
stay rejected. Adapters never invoke handlers; provisional progress is not a
completed tool call.

`9007199254740993` may become `9007199254740992` and
`1.234567890123456789` may become `1.2345678901234567` in Gemini/Ollama DTOs.
Validation sees that resulting value, so an exact minimum can reject a rounded
value. Post-decode conversion to json.Number cannot recover precision or source
spelling. Use string domain schemas for exact IDs/decimals; do not automatically
change types. No strict mode or alternate wire path is introduced.

Signed state is scoped opaque provider material, not a signature verified by the
adapter. Ordinary SDK representation survives response and execution persistence
and the next SDK request. Scope/destination/model, expiry, required-state and
content/part association checks still reject incompatible reuse. Fixtures prove
transfer only; live service acceptance of fixture signatures is untested.

Exact caller/legacy persisted state is a different input: if the public SDK would
change its protected numeric value, admission fails with ErrProviderState and
static migration guidance before IO. Obtain a fresh exchange using representable
domain values. Do not edit, normalize, strip or recreate protected state; do not
retry an accepted mutation automatically.

Usage is cumulative where declared. Trailing usage-only records remain observable;
repeated snapshots do not add twice. Known partial counters survive later errors;
stop/cancel retains only observed evidence. Unknown counters never establish zero
cost or budget safety. SDK numeric, key-normalization and scalar-presence limits
are passing characterizations, not upstream fixes.

Offline examples: `scripts/verify-sdk-consumer.sh` runs a fresh GOWORK=off consumer
using all four official SDKs. It exercises consumer-shaped P1/S1/A1 references,
small message_ref, zero/fraction/nested values, persistence, state gates and
native/trailing/partial usage. Isolated real-consumer ordinary probes and unchanged
inclusive-reasoning accounting pass. Original Kosmify exact-number/known-zero
expectations require migration; full Kosmify/Flowy suites and production behavior
remain unverified. Consumer schemas needing exact IDs/decimals require a domain
contract decision. Review logs and private consumer probes remain local-only in
the ignored `.cursor/` directory. See the published
[limitation register](official-sdk-migration.md).
