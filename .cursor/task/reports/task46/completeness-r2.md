# Task46 — независимая приёмка полноты, round 2

Проверен latest working tree 2026-10-07 после исправлений round 1/2. Каждый AC засчитывается только целиком (10%).

**Итог AC1–AC10: 90%.** AC1–AC8 и AC10 выполнены. AC9 ожидает postrelease published-mode semantic PASS. **Локальный implementation scope готов к публикации:** подтверждённых недостающих требований реализации в проверенной части не осталось. Это не 100% полного acceptance и не завершение task46.

## Независимые актуальные проверки

1. `env GOPATH=/private/tmp/prompty-task46-go GOCACHE=/private/tmp/prompty-task46-build GOMODCACHE=/private/tmp/prompty-task46-mod GOLANGCI_LINT_CACHE=/private/tmp/prompty-task46-lint python3 scripts/host_dispatch_checks.py local --source toolsy=../toolsy --source contexty=../contexty --source routery=../routery --run --report .cursor/task/reports/task46/completeness-local-r2.json` — PASS latest tree: real semantic `go test -race -count=1 ./...`, optional lint (0 issues), offline demo. JSON и log сохранены.
2. `env GOPATH=/private/tmp/prompty-task46-go GOCACHE=/private/tmp/prompty-task46-build GOMODCACHE=/private/tmp/prompty-task46-mod GOWORK=off go test -race -count=1 -run 'Test(AdmitToolCalls|OneShotOrdinaryError|TypedToolCancellation|CustomToolInvokerCancellation)' .` — PASS.
3. Проверены обязательные execution logs: `make-test-r1.log`, `make-lint-r2.log`, `make-release-check-r1.log` показывают успешные suite/lint/release contract checks. Последующие изменения semantic UTF-8 проверены `targeted-r4.log` и новым независимым optional full suite/lint выше. Новые API/модульный inventory после последних fixes не изменялись.
4. `ci-source-r1.json`: PASS того же runner на чистых sources, точно совпадающих с refs CI. toolsy `6ed0a8a6a4e664c25f32397fa63dd6f0a46cba09`, contexty `e7e315a08dcbfb72b658fd41431e2848205e2e56`, routery `8530347e4cf19c715c1e28a1e4712c83990da201`; dirty пуст у всех. Предшествующий clean-head failure не скрыт, source fallback не введён.

## AC1 — 10%, выполнен

ToolInvoker/ExecuteWithTypedTools остаются JSON-only one-shot без native ID inside handler и caller lifecycle recognition. `AdmitToolCalls` — минимальный универсальный pure-validation port, snapshots original execution/response; повторно использует существующие schema/scope/batch checks. Core не импортирует tool/context/lifetime consumers; host-specific Batch, outcomes, approval gates и DTO живут внутри optional module. Контракты, godoc, README и docs navigation обновлены. Архитектурного runtime расширения нет.

## AC2 — 10%, выполнен

`TestFullSDKDispatchAndContinuation` проверяет checked Invoker → actual OpenAI SDK/local HTTP → canonical Response → actual context checkpoint codec → whole-batch admission → реальный Session → tool result projection → следующий checked SDK request. Ассерты исследуют фактически отправленный continuation HTTP body. cmd/demo проходит без credentials/payments.

## AC3 — 10%, выполнен

Полный SDK test доказывает два одинаковых name/args с разными native IDs внутри ToolInput.CallID и downstream result correlation. Duplicate/ambiguous ID-less calls отвергаются до effects в whole-batch test; неподтверждённый одиночный ID-less call отвергается core fail-closed test. Owned call snapshots сохраняют ID/name/raw JSON.

## AC4 — 10%, выполнен

Invalid last call, scope и forced mismatch дают ноль effects. Original-schema-after-lowering test реально компилирует nullable transport schema, но admission использует source schema и отвергает null. Exact `9007199254740993` подтверждён на реальном execution input/raw args/provider result пути. Schema/execution snapshot и чистые validators не запускают effects; core admission regression подтверждает это.

## AC5 — 10%, выполнен

Реальный PendingApprovalError обеспечивает stop-on-control, errors.Is/errors.As/type identity, pending ID и completed evidence. Execution недоступен до завершения batch; ResumeApproval после explicit host decision выполняет тот же pending ID и дальнейшие calls без replay завершённого before effect. Unknown/infrastructure failures retain pending and repeated Run не повторяет их. Scope ограничен локальным in-memory recipe, не IAM/durable engine.

## AC6 — 10%, выполнен

Добавленный `TestOneShotOrdinaryErrorContinuesAdmittedBatch` проверяет fs.PathError, ErrorIs/ErrorAs/identity, partial result и второй handler/result без cancellation; independent targeted race PASS. Cancellation regressions также PASS. Real business failure nil Go error становится safe IsError result; OpenAI Chat без error mapping явно отвергает его до HTTP dispatch. Audience/private/control/binary/MIME/delivery/malformed JSON/UTF-8 cases не получают model content; effects/progress остаются host evidence.

## AC7 — 10%, выполнен

Rich opaque checkpoint exact equality сохраняет state/media/error/part and message annotations/scopes/expiry/associations, bytes owned. Negative tests покрывают destination/expiry/codec/binding/omitted state/invalid associations. Semantic mapping round-trip теперь реально проходит CompileSnapshot, MarshalMessages/UnmarshalMessages с зарегистрированным ContextCodec и сохраняет exact raw argument string через caller sidecar. Sidecar проверяет index/ID/name/JSON number semantics; stale numeric changes и unknown codec fail closed. Добавлен ранее отсутствовавший URL+Data negative test. Таблица unsupported cases сопоставлена tests: chunks, explicit argument MIME, non-text/nested result, image detail, URL MIME/bytes, non-image media, audience/control/unknown MIME/delivery и malformed JSON/UTF-8. Nested text/raw args UTF-8 checks работают в обе стороны и имеют regression tests. Logging response с omitted required state не восстанавливается для continuation.

## AC8 — 10%, выполнен

Channel-synchronized callback/concurrent/repeated Close test доказывает nonblocking cancel и joining cleanup только после iterator unwind; no sleep/polling proof. Close-before-consumption даёт ноль provider source calls. Caller Consume один; done signal caller-owned, hidden producer goroutine нет. Actual Lifetime импортируется внутри optional module.

## AC9 — 0%, ещё не выполнен целиком

Local real integration PASS latest tree, clean CI pinned source suite PASS. Portable runner записывает фактические refs/commands; published mode uses GOWORK=off temporary consumer, forbids source overrides/replaces, doesn't alter manifests silently. Core/production dependency graphs независимы, example optional.

**Оставшееся требование:** successful published-mode tests после публикации root с новым AdmitToolCalls. `published-baseline.json` честно FAIL на старом опубликованном root, где port отсутствует; он не доказывает поддержку нового контрактного API. Нужен новый runner published report с `go test -race -count=1` + lint/demo и проверкой actual module refs без replace. Этот release-dependent gate не может быть засчитан до публикации и реального запуска. CI фактического hosted job status этот отчёт также не выдаёт за доказанный.

## AC10 — 10%, выполнен

AAA regressions и targeted integration checks проходят; обязательные make lint/test/release-check имеют успешные logs. Последние mapping-only guards дополнительно прошли full optional race/lint независимо. Docs/examples/godoc/migration объясняют helper vs full Response, AssistantMessage/StatePreserve, caller cleanup joining и pure admission; live/provider signatures/auth/durability/rollback ограничения указаны. Новых обязательных production deps нет.

## Что требуется для завершения

Повторить AC9 после release и published semantic PASS; добавить новый round отчёт до 100%. Завершить независимый errors audit последних UTF-8 fixes. AC12 требует actual released refs, migration comment URL для автора и closed issue evidence. Этот отчёт не разрешает считать task полностью завершённой при отсутствии этих доказательств; внешние действия reviewer не выполнял.
