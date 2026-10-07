# Task46 — независимая финальная приёмка полноты AC1–AC10, round 3

Проверена актуальная чистая рабочая реализация после release commit `4d028f87265f687580ab6614dfcf36bb6d125b45` и публикации. Каждый критерий засчитывается целиком по 10%.

**Verdict: 100% AC1–AC10.** Gaps предыдущих rounds закрыты; требования реализации и dependency acceptance подтверждены. Этот verdict не заявляет завершение closeout AC12: комментарий автору и закрытое issue требуют отдельного evidence.

## Независимые действия reviewer

- Прочитаны актуальные docs/host-tool-dispatch.md, runner, optional manifest, текущий CI profile и предыдущие реализационные tests/evidence. `git status --short` чистый.
- После release независимо повторён published runner:
  `env GOPATH=/private/tmp/prompty-task46-go GOCACHE=/private/tmp/prompty-task46-build GOMODCACHE=/private/tmp/prompty-task46-mod GOLANGCI_LINT_CACHE=/private/tmp/prompty-task46-lint python3 scripts/host_dispatch_checks.py published --ref v0.16.0 --run --report .cursor/task/reports/task46/completeness-published-r3.json`
  **PASS:** `go test -race -count=1 ./...`, lint (0 issues), offline demo. Command outputs и реальный dependency graph сохранены в completeness-published-r3.json/.log.
- Published-mode env фиксирован runner как `GOWORK=off`, `--source` запрещён, temporary consumer manifest без Replace. Source files dependencies не подменяются. Root/adapter ref требуется явно и записан.
- Проверены ci-source-r2.json и local-r8.json: success true, semantic race suite/lint/demo exit 0. Чистые source refs CI: toolsy `6ed0a8a6a4e664c25f32397fa63dd6f0a46cba09`; contexty `3ce901169b09b7136b126d9baa98e1875b33fa82`; routery `fe61308e030e163195ccdb4d548109a5a4ebdde1`; dirty у всех пуст.
- Прочитан actual published contexty docs/supported-mapping.md из module cache, совпадающий с docs permalink. Recipe явно объявляет более узкий supported semantic subset, rich metadata/media/annotations идут opaque либо explicit unsupported.

## Полнота по критериям

| AC | Результат | Evidence |
| --- | --- | --- |
| AC1 | 10%, выполнен | helper JSON-only one-shot, core pure AdmitToolCalls использует исходные schemas/scope/batch validators; approval/errors/outcome policy живут в optional Batch. Godoc и docs фиксируют ownership. Root/production manifests не приобрели consumer runtime deps. |
| AC2 | 10%, выполнен | TestFullSDKDispatchAndContinuation: actual SDK/local HTTP, checked Invoker, canonical checkpoint, real Session calls, следующий checked HTTP request; assertions inspect sent continuation body. Independent published suite и demo PASS. |
| AC3 | 10%, выполнен | Два одинаковых name/args native-one/native-two проверены внутри ToolInput.CallID и model tool_call_id. Whole-batch tests отвергают duplicate/ambiguous/unconfirmed ID-less до effects. |
| AC4 | 10%, выполнен | Scope/forced/invalid-last отказ даёт ноль effects; real CompileProviderSchema lowering nullable не заменяет original schema admission. Exact 9007199254740993 сохраняется в raw invocation/model result/actual semantic codec; snapshots не мутируются. |
| AC5 | 10%, выполнен | Реальный PendingApprovalError сохраняет errors.Is/errors.As/identity, completed outcomes и pending ID, stop-on-control; explicit ResumeApproval с тем же ID не повторяет before effect. Infrastructure pending запрещает replay. |
| AC6 | 10%, выполнен | TestOneShotOrdinaryErrorContinuesAdmittedBatch fs.PathError + partial result + второй handler; cancellation regressions останавливают следующие calls. Real business failure nil Go error формирует safe IsError; unsupported provider rejects до HTTP. Audience/controls/private/binary/unknown MIME/UTF-8 не получают model content. |
| AC7 | 10%, выполнен | Rich checkpoint exact equality state/media/error/annotations/scope/expiry/associations. Negative tests destination/expiry/codec/binding/omitted state/association; semantic negatives image detail/URL MIME and bytes/non-image/non-text/chunks/argument MIME. Real ContextCodec raw-args sidecar round-trip preserves bytes; stale semantics/unknown codec rejected. Nested/raw UTF-8 regressions PASS. |
| AC8 | 10%, выполнен | Channel-based callback/concurrent/repeated Close и separate completion/lifetime join; Close-before-consumption zero provider source calls. No sleep polling proof, hidden producer or blocking join in core Close. |
| AC9 | 10%, выполнен | Clean CI source runner PASS; current local source runner PASS; published postrelease runner independently PASS race count1/lint/demo without workspace/replacements. Optional consumers pinned published toolsy0.18.0/contexty0.13.1/routery0.7.0; root+adapter0.16.0 реально загружены. Core независим, sources paths explicit, CI не требует audit files. |
| AC10 | 10%, выполнен | AAA unit and targeted regressions, mandatory make lint/test/release-check logs PASS; release-break-r2.log повторяет release gates и публикацию. Independent published suite проверяет последние semantic fixes. Docs/godoc/examples/migration обновлены; live signature/auth/durability/rollback ограничения честные. |

## Release evidence, не closeout

release-transaction.json имеет status published, source `c2253dc71ccc531707cf3b2fbbb89bc859d5d751`, release commit `4d028f87265f687580ab6614dfcf36bb6d125b45`. release-break-r2.log подтверждает gates и завершённую публикацию. remote-refs.json содержит все 13 ожидаемых remote refs с exact commit, missing_or_wrong пуст. Новый universal public admission contract и strengthened host behavior обосновывают release-break.

Published root/adapter подтверждены не только refs: реальные fetched modules использованы в независимом race semantic runner. Optional recipe source использует runner для явного выбора published root/adapter ref; исходный manifest recipe не редактируется незаметно, временные изменения dependency set полностью отражены в report.

## Оставшиеся completion gates за пределами AC1–AC10

AC11 требует итогового независимого errors report без открытых подтверждённых ошибок. AC12 требует явный migration comment автору (URL) и closed issue evidence; reviewer не выполнял внешних сообщений и не закрывал issue. 100% implementation acceptance не подменяет closeout. Hosted GitHub CI job completion также не заявлен: проверены именно чистые источники с CI refs тем же runner.
