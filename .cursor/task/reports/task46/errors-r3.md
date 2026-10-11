# Task46 — независимый аудит ошибок, round 3

Итог: все подтверждённые замечания R1/R2 исправлены. В текущем проверенном scope открытых подтверждённых ошибок, regressions или нарушений архитектурных границ не обнаружено. Error acceptance реализации: PASS. Published dependency gate, release/publication и issue closeout пока не засчитываются этим отчётом и должны быть завершены отдельно.

## Повторная проверка исправлений

- Argument MIME явно отвергается в fromContextCall; оба explicit MIME отрицательных случая есть в TestConsumerSemanticUnsupported.
- Project отвергает invalid UTF-8 plain text и JSON до ContentPart создания; json.Valid не используется как замена Unicode validation.
- Semantic mapper guards standalone/nested text, raw args и native identities в обе стороны. contextRawArguments guards original sidecar Args/ID/Name. R2 воспроизведения теперь fail closed до codec; отрицательные forward/reverse tests есть в TestSemanticMappingRejectsInvalidUTF8.
- ContextCodec sidecar сохраняет raw Args как string через настоящий MarshalMessages/UnmarshalMessages. Exact whitespace + large integer + distinct same-name IDs выдержали дополнительный independent probe. Tampered ID/index/raw semantic numeric value/invalid UTF-8 sidecar отвергаются без partial output. Unknown codec rejected, original sidecar clone owns Calls slice.
- URL + Data explicit unsupported negative test присутствует; business errors/private outcomes/controls остаются host-owned, unsupported provider error mapping отвергается до HTTP.
- Run сохраняет pending error, Batch.Execution не выдаёт continuation при pending. Explicit ResumeApproval применяется только к pre-effect PendingApprovalError, повторно проверяет original batch и destination. Completed calls не replay.
- Helper ordinary handler error regression показывает второй effect/result и оригинальный errors.Is/As. Cancellation regressions продолжают отсекать последующие handlers.
- Admission remains pure BYOT core validation; no consumer lifecycle/policy or application runtime inserted into core. Dependencies stay optional.

## CI/runner changes

CI source checkouts pin immutable commits вместо remote main. Independent `GOWORK=off go list -m -json` confirmed those exact hashes match declared external published refs:

- toolsy: `6ed0a8a6a4e664c25f32397fa63dd6f0a46cba09`;
- contexty: `e7e315a08dcbfb72b658fd41431e2848205e2e56`;
- routery: `8530347e4cf19c715c1e28a1e4712c83990da201`.

Inspected `.cursor/task/reports/task46/ci-source-r1.json`: success=true, all three pinned worktrees dirty='', semantic `go test -race -count=1 ./...`, optional lint and demo exit 0. Local-r7 records actual different local source HEAD/dirty honestly; it is not confused with immutable CI source evidence. Published runner enforces GOWORK=off/no source override/no replacement and records actual module refs; no fake dependency substitution introduced.

## Executed verification

Separate fresh optional snapshot `/private/tmp/prompty-task46-errors-r3/consumer`, temporary workspace root + adapter/openai + actual local consumer modules. Env GOPATH=/private/tmp/prompty-task46-go GOCACHE=/private/tmp/prompty-task46-build GOMODCACHE=/private/tmp/prompty-task46-mod.

1. Optional `go test -race -count=1 ./...`: PASS (package + demo build).
2. Additional independent `go test -race -count=1 -run TestAuditR3 -v .`: PASS. Probes retained in `adversarial-r3_test.go.txt`.
3. Core `GOWORK=off go test -race -count=1 -run 'TestOneShotOrdinaryErrorContinuesAdmittedBatch|TestCustomToolInvokerCancellationMarksRetainedResult|TestTypedToolCancellationKeepsCompletedResultAndErrors|TestAdmitToolCalls' .`: PASS.
4. Declared external module metadata vs CI commit pins: exact Origin.Hash match, no source ref ambiguity.

Implementation этим агентом не редактировалась. No external messages/releases executed. This report is independent error acceptance; it does not replace requirement-by-requirement completeness acceptance or final release/closeout evidence.
