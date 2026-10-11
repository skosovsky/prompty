# Task44 — independent full errors audit r3

Decision: **ACCEPTED** по критерию ошибок. **0 открытых подтверждённых ошибок** в проверенном полном original scope. Отсутствие найденных дефектов не является абсолютной гарантией.

## Snapshot and independence

Immutable manifest `.cursor/task/reports/task44/snapshot-r3.json`, SHA256 `880409f876fd69daaf6b58381de6b653e6e44b4e886171c92c2e91913d8e5f6f`, 761 files, base59402ca. Identity/каждый hash проверены before/after: bad=[]; `/tmp/task44-errors-r3-integrity.log`, `/tmp/task44-errors-r3-integrity-final.log`.

Проверка применяет весь original task44 scope, общий README заданий, contracts и correction invariants, current frozen code/docs/matrix/examples. Не ограничена двумя r2 defects. Peer current report не читала. Implementation не делегировала и не меняла, commits не создавала. Clean checks `/tmp/prompty-task44-errors-checks-r3`, probes `/tmp/prompty-task44-errors-probes-r3` собраны из manifest-listed verified files. Own external tests находятся только в probes copy.

## All prior findings independently closed

**R1 model coercion.** Own public actual SDK HTTP tests Chat sync/native, Anthropic sync, Responses Create numeric/bool source модели повторены с race: explicit typed error вместо invented Model string, known content/usage/handle retained. Gemini/Ollama malformed numeric/bool explicit reject остаются, genuine positive strings сохраняются. Model guard paths непосредственно прочитаны: raw-free caller typed SDK string допустим; отсутствующий source model не подменяется request/profile model; source null следует provider-specific parse contract. Native Anthropic/Responses existing genuine-string guards сохраняются. All own5module probe runs exit0.

**R1 outcome + standalone transport error.** Retained refusal/incomplete + known net.Error больше не маскируют ошибку: exporter получает transport_failure и Error status, outcome остаётся отдельным metadata. Original result/error identity и default privacy сохранены. Own public middleware reproducer прошёл.

**R1 fetch metadata.** Get response+ErrCapture теперь экспортирует observed model, outcome и finish вместе с capture_failure/resource status. SuppressGenerationUsage удаляет только generation counters; DTO usage остаётся и не учитывается повторно. Own public WrapRemoteResponses.Get exporter probe прошёл, original result/error сохранены.

**R2 joined failures.** Original failing public exporter cases errors.Join(OutcomeError, unknown) и errors.Join(OutcomeError, ErrUsage) теперь operation_failure/Error при сохранённом provider outcome. Текущий classifier допускает обычный provider category только для error tree, каждый terminal cause которого — matching typed OutcomeError; generic sentinel и mismatch не дают такого знания. Известные capture/schema/protocol/transport/cancel errors имеют отдельный precedence.

Дополнительная независимая own public exporter matrix12cases с race: nil, typed, wrapped, pure join, nested pure join, unknown first/nested/multiwrap, mismatching typed/nested outcomes, bare sentinel, depth-bound chain. Все expected classifications/status прошли: `/tmp/task44-errors-r3-tree-matrix.log`. Full suite также выполнил author34cases. No raw error text egress, unknown cause не объявлена transport без знания.

**R2 continuation permission.** Chat/Responses modelErr теперь выставляет ContinuationUnavailable при сохранении independently verified facts. Исходный own actual checked HTTP replay probe: malformed model42 returns retained known content+typed error, unavailable=true; AssistantMessage->next checked Execute отвергается до HTTP, calls остаётся1. Дополнительный own wire encode/decode round-trip затем history replay также сохраняет flag/prefix и блокирует secondIO: `/tmp/task44-errors-r3-wire-continuation-probe.log`. Original Responses Create source probe тоже explicit error; current parser flag проверен непосредственно, full author checked history/batch/wire/source fixtures прошли.

Independent remote handle ID/status остаётся наблюдённым resource fact; explicit Previous-only port selection принадлежит caller. Это не разрешение replay malformed source response/history и не изменение core remote status/handle contract.

## Full original scope assessment

**Core data/model/wire.** Reviewed portable Response.Model и ResponseChunk.Model optional observed metadata versus selected request/profile; deep clone, buffered propagation, accumulated stream metadata и sole terminal, response wire/schema round-trip. Current code не выводит observed model из profile. Content/state/part associations остаются original contracts; knownzero/unknown usage presence и subsets preserved. Core остаётся BYOT без SDK/OTel dependency. Clear-break model/lifecycle contracts documented; additional optional wire model не меняет opaque-state projection policy.

**All provider paths and regressions.** Reviewed current Chat parse/native, Responses parse/native/handles, Anthropic sync/native/batch source guards, Gemini full/refusal/native exact source and Ollama complete/partial/native model mappings. Existing parsers reuse original schema/state/tool correlation/capture contracts. Independent actualSDK own probes delineate newly guarded affected endpoints и already rejecting adjacent endpoints. Full12module/race tests include sync/native/buffered/batch/cache, exact numeric/signature/call-result IDs/IsError/thinking/source preservation/partial/error/cancellation regressions tasks38–43. No confirmed adjacent source fidelity regression found.

**Sync OTel categories/privacy/ownership.** Caller tracer/provider configurable без mandatory global singleton; explicit fallback documented. Known response metadata recorded with post-dispatch errors, unknown counters omitted, knownzero emitted, total/cache/reasoning subsets not summed twice, no billing/catalog. Machine error categories preserve actual facts; no invented stage/attempt/retry. Projection receives owned execution/response/status/resource copies; mutation does not affect returned DTO. Error identity preserved under documented host non-mutation rule. Default telemetry excludes prompt/tool args/results/reasoning/opaque state/IDs/account/URL/credentials/raw error text/events; categories only. Full in-memory race/privacy fixtures and own tree matrix pass.

**Native/lazy lifecycle, cancellation and concurrency.** ObserveLifecycle attaches to original handle, does not add a producer or Events consumption layer. Span starts only on actual consumption and ends once with sync.Once. Unused and Close-before-start no fictitious generation duration. StreamStopReason separates consumer stop, explicit Close and cancellation, final status immutable. Known model/content/usage/finish/diagnostics survive lifecycle errors. Context binding retains cancellation channel; original finalizer/capture closeout and observer fallback preserve partial handle/state. Full race suite verifies original handle identity, unused/start/terminal/cancel/consumer/Close/concurrent isolation. No new goroutine leak or additional confirmed lifecycle defect found.

**Explicit operation ports and no double accounting.** Reviewed remote Create/Get/Cancel and separate native CreateStream wrapper, batch Submit/Get/Cancel/Results, cache Create/Get/UpdateExpiry/Delete. Wrappers return original results/errors, keep generation versus resource/job/item metadata distinct, use existing lifecycle/capture ports, expose no implicit optional capabilities/polls/retries/resume/stores. Get/Cancel keep safe non-usage response metadata; repeated resource retrieval omits generation usage. Resource TokenCount remains resource size. Resource clones protect handles, item bindings, source JSON, response state, diagnostics/capture and token pointers for projection. Runnable wrapper Examples and ownership/no-double-token tests pass.

**Conventions and docs/evidence.** Unchanged official pinned Development snapshot `4f85037ef86e92c510d2ef881a58f1076f6fc0e4` checked directly in r1: [client inference](https://raw.githubusercontent.com/open-telemetry/semantic-conventions-genai/4f85037ef86e92c510d2ef881a58f1076f6fc0e4/docs/gen-ai/client-inference.md), [operation spans](https://raw.githubusercontent.com/open-telemetry/semantic-conventions-genai/4f85037ef86e92c510d2ef881a58f1076f6fc0e4/docs/gen-ai/gen-ai-spans.md). Standard metadata/usage subset versus prompty extensions documented; no future-version blanket claim. Root/adapter README/godoc/migration and single capability matrix remain linked to actual ports/fixtures and caller-owned model knowledge. Checked/raw/native/buffered, local/hosted tools, reasoning/state, original schema, media/results, batch/cache and unknown/unsupported boundaries distinguished. Historical37 evidence unchanged; examples/generated consumers executed full run. No telemetry backend/dashboard/pricing/secret-scanner/eval/agentloop/storage scope added.

## Independent executed checks

- Clean full serial `make lint`, all12 modules: exit0 `/tmp/task44-errors-r3-lint.log`.
- Clean full `make test`, all12 modules/race/generated consumers/Examples: exit0 `/tmp/task44-errors-r3-test.log`.
- Original own r1/r2 probes repeated with race in5modules: all exit0; `/tmp/task44-errors-r3-{openai,anthropic,gemini,ollama,otelprompty}-probes.log`.
- Additional12case public error-tree matrix: race exit0 `/tmp/task44-errors-r3-tree-matrix.log`.
- Additional actualSDK wire/history continuation probe: race exit0 `/tmp/task44-errors-r3-wire-continuation-probe.log`.
- GOCACHE=/tmp/prompty-go-cache, GOLANGCI_LINT_CACHE=/tmp/prompty-lint-cache.
- All761 hashes/manifest identity unchanged after checks/probes.

Новых подтверждённых findings в полном проверенном scope не найдено. Live provider/account conformance не выполнялась; local SDK/exporter fixtures не доказывают live model support и не дают абсолютную гарантию отсутствия дефектов.
