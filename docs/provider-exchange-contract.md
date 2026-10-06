# Existing provider exchange fidelity

## Gemini arguments and continuation

The checked generation paths use google.golang.org/genai v1.48.0. Its `deepMarshal` converts interface JSON numbers to float64. The adapter observes the original HTTP response and native SSE event source; batch inline/JSONL parsing likewise uses original JSON with `UseNumber`. Signed and unsigned complete calls share the exact source restoration and SDK-shape checks. Source ordinals, part count, signature bytes, scope, partial-state completion and canonical binding remain enforced. Unknown or incomplete precision fails explicitly; incremental args remain provisional until completion.

Before the final SDK JSON encoder, existing sync, buffered/native streaming, inline/file batch submission and cache creation restore checked native contents through the SDK `ExtrasRequestProvider` hook. This preserves nested integer/decimal/exponent values and explicit empty argument objects. These encoding guarantees apply to adapter-owned dispatch. A host calling the raw SDK directly with translated DTOs owns its subsequent serialization pipeline. The native SDK must still be able to perform its intermediate conversion: values outside its finite numeric range (for example `1e1000`) are rejected rather than rounded, repaired or retried. Exact content is derived from the translated request; an arbitrary host callback cannot replace it later in the hook chain.

For direct `ParseResponse` on a caller-supplied SDK DTO without original source, string/bool/null, Go integer values and `json.Number` retain their declared JSON representation. Float32/float64 anywhere in maps, arrays/slices, exported struct fields or pointers cannot attest original provider precision and are rejected. Unserializable/cyclic JSON arguments fail explicitly; recursive numeric inspection tracks visited containers without an arbitrary nesting policy. This limitation does not replace the exact observed IO path. Runtime Go callbacks and supplied DTO provenance remain host-owned. With an original source, SDK rounding is accepted only at corresponding float leaves; every exact integer and all other native fields must agree independently. Custom JSON projections whose float leaves cannot be bound are rejected when the exact projection differs.

When a later Gemini/Ollama part fails decoding, already accepted canonical parts remain available together with the error. The partial response has unknown outcome/finish and unavailable continuation; it cannot be treated as successful completion. No automatic retry occurs. Failed or canceled checked streams conservatively mark a retained exchange unavailable for continuation, including malformed whole frames, transport errors, missing finish, conflicting call assembly and consumer stop. Observed finish metadata, when already known, remains evidence rather than a success guarantee. Source/SDK errors before any accepted canonical part produce no invented partial response. The Ollama HTTP decoder preserves ordinary reasoning/text and preceding valid calls on an argument failure; sync and buffered/native parsing publish this prefix without a successful finish. Duplicate argument keys, including nested or escaped equivalents, are rejected before decoding into maps.

Gemini `FunctionResponse.ID` receives the canonical `ToolCallID`. Parallel repeated function names are correlated by distinct native IDs. Duplicate calls/results, changed names, unknown results within supplied call history and ambiguous ID-less/mixed correlation fail instead of guessing. A result-only low-level request can carry explicit native IDs without a preceding call in that request; its earlier provider history is host-owned. No ID is generated from a function name.

Checked execution derives `tool_name_correlation` from ID-less call/result history, even without an explicit requirement. The profile must confirm it and the adapter must implement the native mapping port. Low-level Translate/Execute expose native protocol mechanics and do not provide checked profile admission; hosts choosing them own capability confirmation. Gemini and Ollama retain their native name-preserving mapping ports.

## Reasoning and result semantics

Ollama v0.17.4 continuation maps a contiguous canonical reasoning prefix to `Message.Thinking`. Multiple adjacent reasoning parts concatenate in order. Ordinary text remains in `Content`; subsequent reasoning after another part is explicitly unsupported because one native field cannot preserve that interleaving. The ordinary sync/buffered/native response → wire → next HTTP request paths preserve thinking without automatically logging it. Explicit host capture policies remain independent.

| Existing result path | IsError | Content shapes |
|---|---|---|
| Gemini | Native `error` or `output` response key; native call ID retained | Existing text results; unsupported multimodal forms rejected |
| Anthropic | Existing native `is_error` retained | Existing text/image mappings; other shapes rejected |
| OpenAI Chat | Explicit unsupported error before dispatch | Existing text-only tool result mapping |
| OpenAI Responses | Existing explicit unsupported error | Existing checked result shapes |
| Ollama | Explicit unsupported error before dispatch | Existing text-only results; unsupported multimodal forms rejected |

An error result is never silently encoded as success. New multimodal or provider features belong to task43. Ollama has no batch path in this library; Gemini's existing inline/file batch parsing uses the same argument contract. No new provider number/state types enter core, and existing incompatible state/wire formats remain rejected.

Protocol references checked against the pinned local SDKs: [Gemini function calling](https://ai.google.dev/gemini-api/docs/function-calling), [Ollama tool calling and follow-up fields](https://docs.ollama.com/capabilities/tool-calling), [Ollama thinking](https://docs.ollama.com/capabilities/thinking). Actual local HTTP/SSE/JSONL fixtures establish serialization conformance; they are not live model capability evidence.

Migration: declare name-correlation support in host profiles for ID-less history. Regenerate provider tests expecting silent IsError loss or SDK number normalization. Use canonical exact args and checked invocation for provider admission; remove legacy repair/fabricated-ID assumptions.
