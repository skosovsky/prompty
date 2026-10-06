# Task42 — independent completeness audit, round 2

**Decision: REJECT — 94.44% (17/18 fully complete).** One mandatory requirement is partial; partial is not counted. This is an audit conclusion, not a mathematical guarantee.

## Identity and scope

Frozen `/tmp/prompty-task42-snapshot-02`, baseline `d829ab2`. Manifest SHA-256 identity `07d402cb92cbbda50ab55413de42f4235c31647427be992d6e84cc227771ca8a`; all **726/726** hashes verified before and after, logs `/tmp/task42-integrity-r2-before.log`, `/tmp/task42-integrity-r2-after.log`.

Full unchanged denominator: **8 requirements + 9 acceptance bullets + mandatory closeout = 18**. Entire specification, README constraints, current contracts/documentation, code, closeout and fixtures were reviewed; no scope waived. No current peer report read, no implementation participation or source edits. Independent probes used separate copy `/tmp/prompty-task42-audit-r2`, with `/tmp/prompty-go-cache`.

## Full requirement matrix

| Requirement | Code / test evidence | Status |
| --- | --- | --- |
| C1. Exact numeric arguments for signed and unsigned IO, source before irreversible SDK conversion | Gemini HTTP/SSE source observation, originalGeminiPart and exact JSON batch parsing; UseNumber canonical args plus exactContentOptions/part encoding before final SDK serializer; actual unsigned sync/buffered/native and batch/cache fixtures cover large/nested/fraction/exponent/empty args. | Complete |
| C2. Source-less direct DTO precision is not invented; acceptable representations and explicit rejection | Shared reflective numeric guard rejects float containers/pointers/exported fields; exact integers/json.Number pass, cycles/unserializable objects reject without recursive panic/depth policy. Gemini/Ollama direct precision fixtures preserve known parts with unknown outcome and errors. Actual observed IO remains supported. | Complete |
| C3. Gemini native call/result ID and repeated-name correlation; explicit confirmed name-only capability; no fake IDs | FunctionResponse.ID transfer/error output mapping; correlation.go validates duplicate, unknown result, changed name and mixed/ambiguous ID-less cases. Checked preflight derives name-correlation capability from history, profile/mapping fixture; repeated-name wire/actual request IDs preserved in order. | Complete |
| C4. Ollama reasoning continues in Thinking, explicit granularity, no ordinary text merge | Contiguous reasoning-prefix mapping, ordered concatenation, unsupported later reasoning rejection. Actual sync/buffered/native response→wire→next request Thinking/Content fixture passes; no automatic logging. | Complete |
| C5. IsError and multimodal result mappings preserve native semantics or explicitly reject | Gemini native error/output, Anthropic native is_error and existing media mappings; Chat/Responses/Ollama unsupported IsError reject; unsupported content shapes fail before dispatch. Existing result tests pass; no task43 mappings added. | Complete |
| C6. Signed source/ordinal/signature/scope/partial completion/original validation preserved, increments provisional | Per-leaf Gemini argument binding now rejects mixed exact integer/source conflict and independently compares non-argument fields. Original r1 signed/unsigned probe passes. Existing required-state/scope/ordinal/signature/media/partial Vertex/cancellation fixtures retained; incremental calls remain provisional. | Complete |
| C7. Same guarantees across existing sync/native/batch/history paths; unsupported paths explicit; actual IO fixtures | All successful exchange paths and same-frame partial failures are covered. **Native Ollama later malformed frame without its own accepted parts fails to mark the already accumulated prefix unavailable; unlike same-frame failures, the incomplete exchange remains replayable (F1).** Existing native→history guarantees are therefore incomplete. | **Partial** |
| C8. Provider-neutral core and host-owned DTO/handlers/endpoint/transport/policies; incompatible codecs/wire reject | Reflection/precision logic stays in adapter packages, provider state codecs remain vendor modules. Existing incompatible format/codec/scope rejection and host policy ownership retained; no loops/storage/retries/model catalogue or provider core types introduced. | Complete |
| A1. Unsigned HTTP 9007199254740993 exact canonical args and typed int64 value | Actual unsigned SDK exchange fixture, typed NewTypedTool handler receives exact int64, native IDs retained. | Complete |
| A2. Next actual SDK request preserves nested/negative/fraction/exponent and empty object | Final contents hook/empty object encoder with actual HTTP/SSE request body assertions, signed exact SDK numbers and cache continuation fixtures. | Complete |
| A3. Sync/native complete args equal; applicable batch cases pass | Equivalent unsigned sync/buffered/native fixture and batch inline/JSONL numbers/IDs fixture; native partial completed projection and provisional state tests. | Complete |
| A4. Wire→AssistantMessage→next request retains IDs and repeated-name result association | Two-ID same-name actual exchange verifies first/second calls and reversed second/first FunctionResponse IDs after StatePreserve wire restoration. | Complete |
| A5. Explicit supported name-only path; malformed/duplicate/ambiguous correlation reject | Checked unknown/unsupported/supported name-correlation profile fixture; mapping port and Gemini malformed/mixed/repeated/unknown-name cases; no guessing/fabricated IDs. | Complete |
| A6. Ollama thinking round-trip remains separate and not automatically logged | Actual sync/buffered/native Thinking wire/history request fixture; prefix-only translator and explicit host capture boundary. | Complete |
| A7. Signed/partial order and signatures retained; altered/missing required state reject | Existing signed ordered wire round-trip, required-state alteration/optional downgrade/foreign destination/media/signature tests; actual Vertex partial sequence/interleaving/cancel fixtures. Mixed exact/float parse association now rejects. | Complete |
| A8. Unknown precision/unrepresentable result semantics return explicit errors with available partial result; no retry | Direct source-less prefix fixtures and new actual Ollama malformed/duplicate args same-frame fixtures retain thinking/text, set unknown finish/outcome, report error. Native later-frame failure also retains prefix/error (F1 is availability-to-history inconsistency, counted under C7). No automatic retry. | Complete |
| A9. Actual SDK fixtures prove original defects fixed, race/cancellation conformance retained | HTTP/SSE/JSONL precision/ID/thinking/empty-object fixtures, safe source overflow/context/isolation/state cancellation tests; independent full root/adapters/generated race pass and author final all-module checks green. | Complete |
| M1. Mandatory breaking closeout/migration/checks/limits | Closeout/current provider exchange document describe new contracts, removed silent error/precision assumptions, migration, host raw-SDK pipeline and finite intermediate SDK bound, prefix granularity/unsupported paths, and checks. These explicit limits do not waive F1. | Complete |

## R1 correction independently verified

Copied original r1 probe unchanged into separate r2 copy. `go test -race ./adapter/gemini -run TestAuditMixedExactSourceBinding -v`, exit **0**; `/tmp/task42-completeness-probe-r2.log`:

```text
signed=false err=prompty: invalid provider state; response=nil
signed=true  err=prompty: invalid provider state; response=nil
PASS
```

`argument_binding.go` compares per numeric leaf and only allows SDK normalization at its corresponding float leaf. Maps/slices/arrays/pointers are considered independently; exact integers/json.Number and all non-argument native fields must agree. Existing actual SDK all-float source-restoration paths still pass. No broad all-number rejection substituted for exact IO.

Ollama HTTP malformed argument retention and duplicate-key policy were also reviewed: read ordinary fields separately from raw args, decode exact args with duplicate detection before map construction, preserve earlier accepted parts and inject an internal rejected-call sentinel solely for parser failure projection. Buffered unknown-outcome response now uses Delta, native same-frame partial sets unavailable. Those fixes work in the included actual HTTP fixtures, but the empty-prefix error branch still misses F1.

## F1 — native later-frame argument failure leaves prefix continuable

Location: `adapter/ollama/stream.go:95–97`. `nativeStreamState.frame` returns `nil, err` when a failing frame has no locally accepted parts. Consequently it never reaches the new `frame.ContinuationUnavailable = true` branch. Earlier frames already accumulated by the core stream keep `ContinuationUnavailable=false`. The terminal error/status alone does not set that metadata on the retained Response.

Independent actual HTTP/NDJSON probe `/tmp/prompty-task42-audit-r2/adapter/ollama/task42_audit_multiframe_test.go`; `/tmp/task42-completeness-multiframe-r2.log`.

1. First frame: known reasoning/text plus valid `lookup` call, `done=false`.
2. Second frame: only another tool call with `arguments: []`, no thinking/text, `done=true`.
3. Checked native stream returns explicit malformed-args error and retained three-part prefix, as required.
4. However prefix has `ContinuationUnavailable=false`, unknown outcome/empty finish, and `Result.AssistantMessage()` succeeds with no unavailable marker. The failed exchange can therefore enter history despite the declared parse-failure continuation boundary.

```text
error=ollama: malformed tool arguments / adapter: ... malformed
Content=[ReasoningPart, TextPart, valid ToolCallPart]
ContinuationUnavailable=false; Outcome=""; FinishReason=""
AssistantMessage error=<nil>
```

Command: `GOCACHE=/tmp/prompty-go-cache go test -race ./adapter/ollama -run TestAuditLaterMalformedFrameDisablesContinuation -v`, **exit 1**. This is deterministic local actual HTTP, no sleeps, DTO mocks or source edits.

The public provider-exchange contract states that a parse-failure prefix has unavailable continuation, and the new r2 same-frame branch implements exactly that promise. Loss of the marker when the malformed call moves into a later empty-prefix frame is not a documented SDK limitation. It is a difference between native existing paths/history restore. Keep accepted content/error, but propagate unavailability for the accumulated failed exchange even when the current frame has no accepted content. Verify no successful finish is published and no invalid call enters the prefix. Test prior valid calls as well as thinking/text-only prefixes.

## Checks, scope and practical limits

- Snapshot hashes 726/726 pass before and after.
- Independent `GOCACHE=/tmp/prompty-go-cache go test -race ./... ./adapter/gemini/... ./adapter/ollama/... ./adapter/openai/... ./adapter/anthropic/... ./cmd/prompty-gen/testdata/gen_out`: **exit 0**; `/tmp/task42-completeness-independent-r2.log`.
- Independent r1 source-binding probe: **exit 0**; new native multi-frame probe: **exit 1**.
- Final author `/tmp/task42-lint-final-r2b.log`, `/tmp/task42-test-final-r2.log` inspected: all 12 modules, zero lint issues, passing race and explicit generated consumers. `/tmp/task42-adapters-final-r2.log` final adapter/root boundary run passes. These suites cover same-frame prefix retention, not F1's later frame.

Checked admission versus low-level protocol behavior remains explicit. Confirmed name correlation is required by checked preflight, raw SDK transport/DTO provenance is host-owned, exact finite SDK intermediate range is an explicit error boundary, unrepresentable result/interleaving shapes reject before dispatch. These are allowed boundaries, not substitutes for mandatory lossless existing-path behavior.

BYOT and core boundaries remain intact: vendor types stay in adapters, callbacks/storage/handlers/endpoint/policies/agent loop remain host-owned; no implicit retries, probes or workflow service added. No additional incomplete mandatory requirement was confirmed in this audit, but **C7 partial prevents 100% acceptance: 17/18 = 94.44%**. A corrected frozen snapshot and paired full audit are required.
