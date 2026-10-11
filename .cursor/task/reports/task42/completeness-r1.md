# Task42 — independent completeness audit, round 1

**Decision: REJECT — 94.44% (17/18 fully complete).** One mandatory requirement is partial and is not counted. This is an independent audit conclusion, not a mathematical guarantee of defect absence.

## Snapshot, independence and scope

Only frozen `/tmp/prompty-task42-snapshot-01` was audited, baseline `d829ab2`. Manifest SHA-256 identity: `6149d18e1a33a1064924a6b112ee27b2423d10c9d68b4ae9e2332fe078a75021`. Every **719/719** hash verified before and after: `/tmp/task42-integrity-r1-before.log`, `/tmp/task42-integrity-r1-after.log`.

Full task42 requirements/acceptance, README rules, current contracts, public provider-exchange documentation and closeout were considered. Denominator: **8 numbered requirements + 9 acceptance bullets + mandatory closeout = 18**. No scope was reduced to a checkbox list or to the originally reported regressions. Frozen/working implementation was not edited. Probe runs used separate copy `/tmp/prompty-task42-audit-r1`; current peer audit report was not read.

## Full requirement matrix

| Requirement | Code / test evidence | Status |
| --- | --- | --- |
| C1. Exact source numeric contract for all arguments, including unsigned calls, before irreversible SDK conversion | `gemini/response_source.go` captures raw HTTP JSON; unsigned and signed FunctionCall parts use originalGeminiPart; native source observation and batch inline/JSONL source parsing use exact JSON; `assistantContent` UseNumber plus final exactContentOptions hook. Actual SDK unsigned sync/buffered/native HTTP fixtures cover big integer, nested negative/fraction/exponent and empty object. | Complete |
| C2. Direct source-less DTO does not invent precision; acceptable representations and explicit ambiguous rejection, without rejecting correct IO | Shared `adapter.HasUnverifiedJSONNumbers` recursively inspects maps/slices/arrays/pointers/exported fields with cycle detection. Gemini/Ollama direct floats reject, integers/json.Number pass; unserializable/cyclic args error; direct precision fixtures retain accepted prefix with unknown outcome/unavailable continuation. Actual IO source path remains exact. | Complete |
| C3. Gemini native FunctionResponse.ID, repeated-name parallel order/correlation, explicit name-only capability, no synthetic IDs | `gemini.toolResultContent` transfers ToolCallID and uses native response ID; correlation.go validates duplicate/unknown/changed-name/mixed-ID/name ambiguity; checked preflight automatically derives name correlation for ID-less history and requires profile plus mapping port. Actual repeated-name two-ID HTTP/wire/next-request fixture, unknown/unsupported/supported name-only profile fixture and malformed history tests pass. | Complete |
| C4. Ollama reasoning continuation via Thinking; explicit supported granularity and no text merge/loss | Ollama assistant translation calls ollamaThinking, accepts contiguous reasoning prefix in order and rejects reasoning after ordinary content; preserves Thinking separately from Content. Actual sync/buffered/native response→wire→next HTTP fixture and unsupported interleaving tests pass. No automatic logging introduced. | Complete |
| C5. IsError and existing multimodal result semantics preserved or explicitly rejected | Gemini native error/output plus FunctionResponse ID; Anthropic native is_error and existing image/text/document mappings; OpenAI Chat explicit IsError rejection added, Responses existing rejection; Ollama explicit error rejection. Unsupported result shapes use explicit errors rather than text conversion; existing adapter result tests and full boundary suites pass. No task43 mappings added. | Complete |
| C6. Do not weaken signed source/ordinal/signature/scope binding, partial/unknown outcome or original validation; increments provisional | Existing scope/signature/ordinal/required-state and partial-sequence validators remain, with actual signed/partial/cancel fixtures. **Mixed exact integer plus unrelated float permits source/DTO mismatch and creates required signed state (F1), weakening the declared source association check.** | **Partial** |
| C7. Sync/native stream/batch/history paths same guarantees; unsupported paths explicit; actual IO fixtures | Gemini sync/buffered/native, inline/JSONL batch, cache and history use exact contents/source paths; Ollama sync/buffered/native and history exact HTTP/thinking. Existing OpenAI/Anthropic HTTP/SSE and applicable batch fixtures retained. Ollama batch absent and explicitly unsupported. Independent full adapter/race and root/generated runs pass. | Complete |
| C8. Core provider-neutral, host types/handlers/endpoint/transport/policies; incompatible versions reject, no legacy repair | Numeric inspection helper lives in adapter package, not provider-specific core DTO/state types. Existing provider-state format/codec/scope validation retained; old/unsupported states fail rather than guessed repair. No framework, loops, automatic retries/storage or provider model catalogue introduced. | Complete |
| A1. Unsigned Gemini 9007199254740993 yields exact canonical args and typed int64 value | TestUnsignedExchangeActualSDKPreservesExactArgumentsAndIDs uses actual HTTP/SSE and checked invocation, typed NewTypedTool handler gets exact integer; native call IDs remain first/second. | Complete |
| A2. Next actual SDK request preserves nested arrays/objects, fractions/exponents/negative large numbers and empty object | Same exchange fixture checks final request body contents; exactContents/partEncoding correct SDK deepMarshal/omitempty; signed SDK number/empty object fixtures and cache content fixture cover continuation serialization. | Complete |
| A3. Equivalent sync/native stream complete arguments and applicable batch cases | Unsigned exchange fixture across sync/buffered/native; batch inline and JSONL source fixture checks identical numeric cases and IDs; native partial completion tests maintain provisional increments until completed. | Complete |
| A4. Wire→AssistantMessage→next request preserves call IDs and repeated-name result IDs | Actual two-call exchange uses StatePreserve wire restoration and reversed-order tool results; FunctionResponse IDs are second/first, exact original calls first/second retained. | Complete |
| A5. Explicit supported name-only path; malformed/duplicate/ambiguous correlation rejected | Checked ID-less history admission supports only confirmed profile capability; generic mapping-port fixture and Gemini malformed/mixed/duplicate/unknown-name history tests. No fake ID/name substitution. | Complete |
| A6. Ollama thinking→canonical→wire→request preserved separately, no automatic logging | TestThinkingWireContinuationActualHTTP across sync/buffered/native verifies Thinking and ordinary Content independently after wire restore; translation only, explicit host capture remains separate. | Complete |
| A7. Signed/partial sequences retain order/signatures; altered or missing required state rejected | Existing signed ordered response/execution wire round-trip, foreign destination/content alteration/optional required state rejection and actual Vertex partial sequence/cancellation/interleaving fixtures pass. F1 is a new parse source/DTO association failure, recorded under C6 rather than duplicating the percentage penalty for already-tested saved-state validation. | Complete |
| A8. Unknown precision/unrepresentable result semantics explicitly error with available partial result, no retry | Direct unknown/cyclic numeric fixtures retain accepted Gemini/Ollama thinking/text prefix, clear finish/outcome and disable continuation. Native Ollama callback partial fixture emits no false Finish. IsError/result rejection happens before dispatch. Safe source failures preserve context classification without replay. | Complete |
| A9. Actual SDK fixtures reproduce/correct original defects; race/cancellation conformance preserved | Real HTTP/SSE/JSONL precision/ID/thinking/empty-object fixtures, signature/source bounds/state and cancellation/isolation tests; independent adapter race and root/generated suites pass; author final all-module logs green. | Complete |
| M1. Mandatory closeout: changed contracts, removed legacy assumptions, breaking migration/checks/limits | Closeout and public provider-exchange document list source/direct/SDK serialization guarantees, IsError/name-correlation changes, migration, exact finite SDK boundary, host raw SDK ownership, unsupported granularity/paths and final checks. No unsupported limitation is used to waive F1. | Complete |

## F1 — unrelated float enables exact integer source mismatch

Location: `adapter/gemini/response_source.go:234–250`, especially the whole-object `containsSDKFloat` condition at `:239` and the whole-object float normalization at `:243–247`.

The exact comparison first rejects differing integer values. The fallback is then enabled if **any** argument contains a float. It normalizes the entire source to float64 and compares serialized trees, thereby permitting mismatch in another field whose caller representation is exact. The code comment at :238 says exact caller representations must agree directly, but that invariant is not enforced per value.

Independent deterministic probe `/tmp/prompty-task42-audit-r1/adapter/gemini/task42_audit_probe_test.go`, `/tmp/task42-completeness-probe-r1.log`:

1. SDK DTO FunctionCall args: `number = int64(9007199254740992)`, `ordinary = float64(1)`.
2. SDKHTTPResponse.Body source: `number = 9007199254740993`, `ordinary = 1`, same call ID/name/part ordinal.
3. ParseResponse succeeds with no error and replaces the exact caller integer with the conflicting source value.
4. Repeat with identical valid thoughtSignature on source/DTO: parse again succeeds and constructs Required `gemini.part.json` continuation state for the conflicting source.

```text
signed=false err=<nil>; Args={"number":9007199254740993,"ordinary":1}
signed=true err=<nil>; Required state created; same conflicting Args
```

Command in separate copy: `GOCACHE=/tmp/prompty-go-cache go test -race ./adapter/gemini -run TestAuditMixedExactSourceBinding -v`, **exit 1**. No snapshot/source file was changed.

This is the mixed-representation version of the existing TestCallerExactSDKDTOAndSourceMustAgree regression, which tests an exact integer conflict only when there is no other float. Direct source provenance is host-owned, but the public parse contract explicitly checks source/DTO part association and allows exact declared Go integer representations. A float in a different field is not evidence that this exact integer legitimately underwent SDK conversion. The signed variant demonstrates why this belongs to C6's source binding invariant; ordinary actual SDK DTOs with genuinely float-converted numeric leaves must continue working.

Expected acceptance evidence: compare exact declared integer/json.Number leaves against source exactly, while allowing verified SDK normalization only at corresponding ambiguous float leaves. Test mixed nested containers, signed and unsigned parts, retain known prefix on error, and retain actual SDK HTTP/SSE/JSONL exact ingress/continuation regressions. No provider number type needs to enter core.

## Checks and protocol boundaries

- Snapshot hashes 719/719 pass before and after.
- Independent adapter/package/provider race: `go test -race ./adapter/... ./adapter/gemini/... ./adapter/ollama/... ./adapter/openai/... ./adapter/anthropic/...`, **exit 0**, `/tmp/task42-completeness-adapters-r1.log`.
- Independent root and generated consumers race: `go test -race ./... ./cmd/prompty-gen/testdata/gen_out`, **exit 0**, `/tmp/task42-completeness-root-r1.log`.
- Independent mixed source binding probe: **exit 1**, as detailed above.
- Author final logs inspected: `/tmp/task42-lint-final-r1d.log`, `/tmp/task42-test-final-r1c.log` contain all 12 modules and explicit generated consumers, zero lint issues and passing race suites. Formatting-only final adapter boundary rerun `/tmp/task42-adapters-final-r1b.log` passes. Those checks do not cover mixed exact/float source binding.

Pinned SDK/API boundaries were checked against local code and official [Gemini function protocol](https://ai.google.dev/gemini-api/docs/function-calling), [Ollama tool continuation](https://docs.ollama.com/capabilities/tool-calling) and [Thinking fields](https://docs.ollama.com/capabilities/thinking). Ollama's protocol uses separate thinking/content fields and accumulated follow-up; current prefix-only mapping explicitly rejects unrepresentable interleaving. Local fixtures are serialization evidence, not live model capability proof.

Checked admission automatically requires confirmed name-correlation support for ID-less history; low-level native translation is separately documented and does not invent profile proof. Adapter-owned Gemini dispatch restores exact contents after SDK normalization; host raw SDK calls own their later serialization pipeline. Finite intermediate SDK range rejects unrepresentable numbers explicitly, without rounded success. These boundaries match the task's permitted explicit unsupported behavior and do not excuse F1.

BYOT and library boundaries otherwise preserved: host handlers/DTO/endpoint/transport/policies remain host-owned; vendor numbers/state do not leak into core; no retries, agent-loop or storage service added. Full acceptance needs a corrected immutable snapshot and paired audit. **17/18 = 94.44%; C6 remains partial.**
