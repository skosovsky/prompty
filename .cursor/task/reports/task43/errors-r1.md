# Task43 — independent errors audit r1

Decision: **REJECTED**. Открыты **2 подтверждённых P1 класса ошибок**. Full green checks не отменяют executed adversarial failures.

## Snapshot and independence

Frozen workspace manifest `.cursor/task/reports/task43/snapshot-r1.json`, SHA256 `5627f70f37aca76f3b1e46791c26cd0a94e99a49b6f5d5aa6f8d9bbd47cc74c6`, 800 файлов, base `132afc221c7db0f69088ce0b341f4f779b4be8ce`. Все manifest hashes проверены before/after; bad=[]: `/tmp/task43-errors-r1-integrity.log`, `/tmp/task43-errors-r1-integrity-final.log`. Manifest identity совпал.

Прочитаны полный original task43 scope, общий README заданий, contracts, provider capability matrix, implementation новых портов/core contracts и применимые SDK fixtures/tests. Peer current report не читала; implementation не делегировала и не меняла. Frozen/working implementation неизменны. Clean checks copy `/tmp/prompty-task43-errors-checks-r1`, probes copy `/tmp/prompty-task43-errors-probes-r1`. Файлы обеих копий получены только из hash-verified manifest.

## F1 — P1: Responses native stream panic при отрицательном summary_index

Места: `adapter/openai/responses_sse_machine.go:295–302` и `366–390`. Новый reasoning item имеет summaryIndex=-1 и пустые summary arrays. Equality against summaryIndex разрешает incoming -1, затем код индексирует slices[-1]. Presence.Valid проверяет наличие SDK value, не неотрицательный диапазон.

Executed actual SDK HTTP/SSE reproduction через public Responses.CreateStream:

1. response.created с in_progress и известным response ID.
2. response.output_item.added с reasoning ID rs_fixture и summary:[].
3. response.reasoning_summary_text.delta, item_id=rs_fixture, output_index=0, summary_index=-1, delta="bad".

Consumption panics `runtime error: index out of range [-1]` вместо protocol error/partial lifecycle. Independently reproduced также reasoning_summary_text.done и reasoning_summary_part.done до part.added с тем же -1. Все 3 recovered panic assertions failing с race: `/tmp/task43-errors-r1-summary-probe.log`. Source probe `adapter/openai/audit_task43_summary_test.go` только в отдельной temp copy.

Внешний provider event не должен уронить harness. Требуется range/lifecycle validation до любого summary slice indexing; recover в library не заменяет корректную state machine. Task43 mandatory native error/cancellation/partial contract нарушен.

## F2 — P1: Anthropic malformed обязательные delta fields становятся successful output

Места: `adapter/anthropic/stream.go:218–227` и accept/whole-event validation. Delta path использует SDK-coerced `d.Text`, не удостоверяя raw JSON type/presence/uniqueness. Content block start/source validators не покрывают event delta envelope. Final block source собирается из уже изменённого SDK value, поэтому поздняя source validation не обнаруживает loss/coercion.

Executed actual SDK HTTP/SSE через public checked native invoker:

- message_start valid;
- text content_block_start с text="known";
- content_block_delta delta={type:"text_delta", text:null};
- content_block_stop; message_delta end_turn + known usage; message_stop.

Фактически error=nil, StreamCompleted, OutcomeCompleted, Content text="known", ContinuationUnavailable=false. Те же independent fixtures:

| malformed source delta | canonical completed text |
| --- | --- |
| text:null | known |
| missing text | known |
| text:42 | known42 |
| duplicate text keys lost/kept | SDK-selected knownlost |

Все cases должны explicit fail-closed с known preceding prefix; source null/missing/number не являются текстовым fragment, duplicate JSON fields не дают удостоверяемого source. Сейчас получается тихая потеря/подмена данных и продолжаемый success. `/tmp/task43-errors-r1-anthropic-delta-probe.log`, own public actual SDK race probe `adapter/anthropic/audit_task43_delta_test.go` в temp copy. 4 failing assertions.

Confirmed только перечисленные executed text variants. Аналогичное использование SDK values в input_json/thinking/signature branches — место для системной проверки того же source boundary, но отдельные неисполненные variants не объявляются confirmed findings.

## Full mandatory scope reviewed

**Responses controls/strict.** Reviewed endpoint-specific reasoning whitelist/schema profile gate, summary отдельно от encrypted state, strict opt-in/required capability, original strict subset without hidden optional/nullability lowering, original argument validation и known schema digests. Existing actual SDK requests/invalid profile zero-IO fixtures проходят. Strict tool schemas remain identity-mapped before strict checks. Get/Cancel/background independent operations и schema rebinding сохраняются без автоматического native-stream/strict requirement.

**Responses native stream.** Reviewed lazy CreateStream only, no sync/Get fallback/retry, SDK body closure, per-invocation source/state, known handle retention/immutability, increasing event sequences/correlation IDs/output indices, deltas/done/terminal exact output comparison, reasoning summaries/encrypted state, hosted search/citations, usage presence/late counters, finalization/unknown mandatory event отказ. Full actual SDK stream conformance/cancellation/Close/concurrent isolation/capture fixtures green. F1 показывает непокрытый hostile index lifecycle. Partial failures retain already available handle/result and general core closeout revokes continuation.

**Ollama Format/Think.** Reviewed direct + checked profile admission, strict allowlisted top-level Think mapping with bool/null/low/medium/high, effective ModelOptionsSchema и forbidden combinations, no model-name discovery. Original schema object copied to Format without lowering/fallback. Completed output validates original schema; native nonterminal JSON provisional and reasoning separate. Actual SDK fixtures and original-schema negative cases green.

**Gemini media.** Reviewed image PNG/JPEG/WebP subset, InputMIME+media-result profile gate, ID/name/order preservation, ordered native text/ref output and image Parts displayName, cloned caller bytes, backend-specific inline/Vertex gs URI forms, explicit MIME/URI/bytes+URI refusal, no download/upload/recache. Existing sync/native/batch/cache exact encoding and task42 fidelity remain in full green suite. Actual SDK request/wire/media examples pass.

**Anthropic hosted/pause.** Reviewed closed versioned web_search_20250305/web_fetch_20250910 declarations/settings, local-name collisions, forbidden operation combinations, validated server input/result/error shapes, native call/result linkage/source order, citations/document/PDF shapes and state source manifest replay. Hosted remains scoped MessageAnnotations, never ToolCallPart/local handler authorization. OutcomePaused distinguishes transport completion from successful model/structured output; explicit caller resume and unchanged wire/history fixtures green, handlers stay zero. Native publishes source-proven completed hosted prefix, private signed reasoning state preserved, batch mapping/rebinding and source identity rejection covered by actual SDK fixtures. F2 identifies raw event validation gap despite valid start/final reconstruction contracts.

**Core universal contracts/boundaries.** Reviewed HostedTools cloning/wire/schema portable shape and per-adapter mapping gates; foreign/required scoped annotation checks/profile requirements, StateOmit removes scoped metadata+revokes required continuation, deep owned Scope/Value, optional unscoped host metadata, conservative truncate dependencies. Required annotations do not force new encrypted output. StreamPartFinalization validates exact fragments/tool args/type/policy, works on clone candidate atomically, adjusts associations, rejects repeat/unknown/sealed content. OutcomePaused accepted as control outcome, typed output/helpers do not execute local handlers. Existing schema/scope/capture binding preflight remains; vendor DTOs/loop/storage/transport/credentials/policies stay host-owned.

**Capture, cancellation and migration.** Reviewed semantic versus HTTP-byte coverage, projector/sink failure separate from provider rejection, accepted handle retention and terminal diagnostics, original scoped metadata in callback snapshot. Full race suite verifies body-close/parent cancel/Close/consumer stop/concurrent stream isolation. Public docs/matrix identify actual implemented port versus caller live capability and explicit unsupported subsets; examples are offline runnable fixture Examples in modules and executed by full tests. Legacy Responses-only hosted field replaced by canonical execution intent; no resume loops/reconnect/media IO/sandbox/browser/session manager.

No additional confirmed defects found in the reviewed mandatory scope. Это не сокращает следующую приёмку: r2 должен повторно проверять весь scope и новые source-validation adversarial variants.

## Independent checks

- Clean-copy full serial `make lint` all12modules: exit0, `/tmp/task43-errors-r1-lint.log`.
- Clean-copy full `make test`, all modules/race/generated consumers/Examples: exit0, `/tmp/task43-errors-r1-test.log`.
- Adversarial actual SDK own probes race: intentional failing F1/F2 logs above, implementation copy source unchanged except external test files.
- GOCACHE=/tmp/prompty-go-cache, GOLANGCI_LINT_CACHE=/tmp/prompty-lint-cache.

Live model/account conformance не выполнялась и не приписывается локальным fixtures. Acceptance требует исправления обоих confirmed P1 classes и нового полного frozen audit. Отсутствие других найденных дефектов не означает абсолютной гарантии.
