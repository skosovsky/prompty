# Task44 — independent full errors audit r2

Decision: **REJECTED**. **2 открытых подтверждённых P2 класса ошибок** в adjacent correction boundaries. Прежние конкретные r1 reproductions исправлены, но исправления не закрывают весь контракт.

## Snapshot and independence

Immutable manifest `.cursor/task/reports/task44/snapshot-r2.json`, SHA256 `d5bfab48e77c1d41d23e486091141e823b29b3c7da8dffe94774cbe6a999aeb6`, 761 files, base59402ca. Все hashes/identity проверены до/после, bad=[]: `/tmp/task44-errors-r2-integrity.log`, `/tmp/task44-errors-r2-integrity-final.log`.

Применён весь original task44 scope и общий task README, contracts с R1 correction invariants, updated docs/matrix/examples и текущий frozen code. Не ограничивалась изменениями r1. Peer current report не читала, implementation не меняла/не делегировала, commits не создавала. Clean checks copy `/tmp/prompty-task44-errors-checks-r2`, own probes `/tmp/prompty-task44-errors-probes-r2`; обе получены из manifest-listed verified files. Additional external test files только в probes copy.

## R1 reproductions rechecked

Все прежние own actual SDK HTTP probes + in-memory exporter reproductions повторены без изменений исходных assertions с race, exit0:

- OpenAI Chat sync/native и Responses Create numeric/bool raw model больше не рекламируют SDK-coerced string; возвращают typed error, известный content/usage/remote handle сохраняются.
- Anthropic sync numeric/bool raw model explicit error, known content/usage сохранены, unavailable=true.
- Gemini/Ollama malformed numeric/bool model по-прежнему explicit reject; positive genuine string проходит.
- Retained refusal/incomplete + standalone known net.Error теперь transport_failure и Error status.
- Responses Get response+ErrCapture теперь сохраняет observed model/outcome/finish, capture category и original DTO/error.

Logs `/tmp/task44-errors-r2-{openai,anthropic,gemini,ollama,otelprompty}-probes.log`, все exit0. New author actual SDK source type/duplicate/absent/raw-free SDK/batch fixtures также executed independent full suite. Это закрывает конкретные reproductions r1; adjacent defects ниже остаются.

## F1-r2 — P2: joined независимая ошибка всё ещё маскируется OutcomeError sentinel

Location `ext/otelprompty/telemetry.go:275–290` (guard `err != nil && !errors.Is(err, prompty.ErrResponseOutcome)`); `operationSpan.finish` status exclusions for provider_refusal/provider_incomplete.

Executed own public WithTracing + in-memory exporter race reproduction:

- BYOT Invoker returns Response{Outcome:Refusal, Model:"known"} вместе с errors.Join(&OutcomeError{Outcome:Refusal}, errors.New("SECRET-independent"));
- второй case — errors.Join(OutcomeError, prompty.ErrUsage).

Original result/error/independent cause сохраняются, но exported category=provider_refusal, error.type=provider_refusal, Status=Unset. Независимая failure пропадает из machine classification. `/tmp/task44-errors-r2-joined-probe.log`, test `TestAuditTask44JoinedOutcomeAndIndependentFailure`, 2 failing cases.

errors.Is подтверждает наличие outcome sentinel в дереве, но не отсутствие других причин. Только дерево с чистой соответствующей outcome cause может классифицироваться как обычный provider control outcome; joined usage/unknown failure должна оставаться ошибкой операции при отдельно сохранённом outcome. Не нужно придумывать transport stage для неизвестной cause или экспортировать её текст. Source error identity/privacy правильные; ошибка именно классификации/status. Это существенное продолжение F2-r1, а не новая требуемая telemetry subsystem.

## F2-r2 — P2: malformed model partial Chat response допускается к continuation

Location `adapter/openai/openai.go:535–549`: modelErr удаляет model и меняет OutcomeIncomplete, но возвращаемый Response не получает ContinuationUnavailable=true. `adapter/client.go:finishParseFailure` и Response.AssistantMessage сохраняют flagfalse; checked next request допускает эту history.

R1 correction invariant в task44/contracts прямо требует malformed source error с independently verified partial facts без continuation permission. Anthropic новый source-model path выставляет flag, Chat sync этого не делает.

Executed actual SDK public checked HTTP reproduction (`/tmp/task44-errors-r2-continuation-probe.log`, `TestAuditTask44MalformedModelDoesNotGrantReplay`):

1. valid text completion="known", raw model numeric42. Checked Execute returns retained known response + ErrInvalidResponse, Model empty, ContinuationUnavailable=false.
2. Response.AssistantMessage succeeds. Append projected message to new PromptExecution and invoke the same checked client.
3. Second HTTP request действительно отправляется; valid second completion succeeds. Observed calls=2,nextErr=nil.

Failure evidence должна сохраняться; malformed source response не получает разрешение на новую continuation автоматически. History flag должен донести запрет через wire/projection и checked admission. Уже подтверждён actual sync path; buffered/batch и Responses modelErr parser — adjacent paths для проверки исправления (Responses parser тоже не выставляет flag по code inspection, но отдельное replay reproduction для него не заявляется).

## Full scope assessed again

**Core model/wire/stream.** Response/ResponseChunk observed model отдельно от request/profile; propagation through clone/wire/buffered/final terminal inspected. Optional schema field correct; core не получил SDK/OTel dependency. Knownzero/unknown, cache/reasoning subsets и original content/state associations retained. StopReason consumer/Close/cancel runs under existing lifecycle locking; immutable terminal/status clone preserved. No new producer goroutine, retry/resume or replacement Events handle. Full independent race suite covers cancellation/Close/consumer/unused/original handle/metadata retention. F2-r2 exposes sync continuation policy gap despite successful native closeout guards.

**Every provider boundary.** Current guards/source models reviewed directly for Chat sync/native, Responses parsing/native, Anthropic sync/native/batch, Gemini sync/refusal/native exact source, Ollama full/partial/native. OpenAI model parser checks top-level genuine unique model string against SDK source; raw-free typed caller values accepted, absent model not inferred. Anthropic validates unique original source and genuine string, allows provider-specific null/absent as unknown; native start stricter original contract preserved. Responses native source types already guarded. Checked HTTP probes above validate affected and correctly rejecting ports. Original task38–43 numeric/schema/source/state/IsError/tool correlation/hosted/pause/capture regressions pass full suite.

**Sync telemetry and ownership.** Caller-owned tracer/provider injected, documented global fallback optional. One logical span, no physical-attempt claim or fabricated retry count. Available request/observed models, outcome/finish and known token counters recorded with response+error. Unknown counters omitted, knownzero retained, subsets not double-added; no price catalogue. Execution/response/status/resource snapshots deep-owned for host projection, original DTO/errors returned; error objects preserved under documented non-mutation expectation. Default excludes prompts/tools/reasoning/state/URL/account/IDs/raw error text/events; safe status categories remain. F1-r2 shows error-tree purity hole, not payload egress.

**Lazy tracing/concurrency/capture.** ObserveLifecycle binds to original lazy handle; spans begin at consumption, finish once, Close-before/unused no artificial provider duration. Span context retains cancellation binding/source context, callbacks retain original finalizer/capture contract and known metadata. Observer-registration error fallback preserves known remote handle and rejects before new dispatch. Full race suite and Examples cover isolated concurrent streams, body close, parent cancellation, consumer stop and partial/error capture. No additional confirmed lifecycle/data race defect found.

**Explicit operation ports.** Reviewed Responses Create/Get/Cancel/native stream, batch Submit/Get/Cancel/Results, cache Create/Get/UpdateExpiry/Delete wrappers. Current SuppressGenerationUsage separates Get/Cancel response metadata from billing counters, preserves both response and resource facts, so fixed F3-r1 no longer discards all response metadata. Repeated retrieval/resource operations never emit generation token attrs. Job/resource state remains separate from item outcome; cache token count distinct from generation cost. Deep cloning resource handles/bindings/JSON/diagnostics/capture/token pointers preserves projection isolation. No polls/retries/stores/automatic resume or synthetic item generations added.

**Conventions/docs/matrix/migration/examples.** Same pinned official Development snapshot `4f85037ef86e92c510d2ef881a58f1076f6fc0e4` was checked directly in r1 and remains unchanged: [client inference](https://raw.githubusercontent.com/open-telemetry/semantic-conventions-genai/4f85037ef86e92c510d2ef881a58f1076f6fc0e4/docs/gen-ai/client-inference.md), [operation spans](https://raw.githubusercontent.com/open-telemetry/semantic-conventions-genai/4f85037ef86e92c510d2ef881a58f1076f6fc0e4/docs/gen-ai/gen-ai-spans.md). Standard observed attributes versus prompty total/categories/logical scope documented; implemented subset not future-version guarantee. Default privacy explicitly restricts optional content/high-cardinality egress. Updated root/adapter README/godoc and capability matrix remain code/fixture-linked, separate checked/raw/native/buffered/local/hosted/state/media/schema/batch/cache/profile unknown/unsupported. Migration clear-break38–43 + extension legacy attrs/lifecycle/model documented; historical37 evidence untouched. Runnable Examples/generated consumers executed. No live provider model catalogue, monetary cost, scanner guarantee, backend, storage/eval/agent loop claims added.

## Checks and limits

- Independent clean full serial `make lint` all12: exit0 `/tmp/task44-errors-r2-lint.log`.
- Independent clean full `make test` all12/race/generated/Examples: exit0 `/tmp/task44-errors-r2-test.log`.
- All5module original own probes: race exit0, logs above.
- Additional error-tree and continuation probes: intentional failing assertions reproduce F1/F2; logs above.
- GOCACHE=/tmp/prompty-go-cache, GOLANGCI_LINT_CACHE=/tmp/prompty-lint-cache.
- After full checks/probes all761 snapshot hashes and manifest SHA unchanged.

Новых подтверждённых ошибок сверх F1-r2/F2-r2 в проверенном полном scope не найдено. Local SDK fixtures не доказывают live provider/account conformance и не дают абсолютную гарантию отсутствия дефектов. Acceptance требует исправления двух классов и новой полной paired frozen приёмки.
