# Task38 — независимый аудит ошибок, round 4

**Решение: принято по error acceptance. Открытых подтверждённых ошибок не найдено.** Все ранее подтверждённые classes r1–r3 закрыты в проверенных воспроизведениях. Это результат аудита конкретного snapshot, не утверждение об абсолютной безошибочности библиотеки.

## Идентичность и независимость

- Frozen: `/tmp/prompty-task38-snapshot-04`, baseline `70fc301`.
- SHA256 manifest `/tmp/prompty-task38-snapshot-04.sha256`: `a1337cdb30d32aa3e25b4d2f8d859736814c38bc5a476bfb92cabc9cd23c09f7`.
- Проверены все 672 entries через `shasum -a 256 -c` до аудита и повторно после исполнения frozen suite: оба exit 0. Логи `/tmp/task38-errors-r4-integrity.log`, `/tmp/task38-errors-r4-integrity-final.log`.
- Аудитор не участвовал в implementation. Текущий completeness-r4 другого аудитора не читался. Source/frozen не менялись. Дополнительные probes выполнялись только в `/tmp/prompty-task38-errors-probes-r4`.

## Повторная проверка findings

| Class | Результат проверки |
| --- | --- |
| r1: output helper терял draft7 dialect | Helpers, inferred execution и streaming сохраняют dialect; regression suite passes |
| r1: json.Number,string self-schema mismatch | Numeric quoted codec соответствует actual encoding/json; value/pointer/null checks pass |
| r1: generated method/field collision | Input/LateInput/Output collisions диагностируются до emission |
| r1: optional empty collection presence | Outer pointer сохраняет empty vs absent, nullable initializer сохраняет explicit null; compiled consumers pass |
| r1: unsupported JSON tag names | Inference/codegen возвращают явную ошибку вместо несовпадающего успешного контракта |
| r1: empty json.Number documentation | Документация теперь соответствует encoding/json zero-as-0 semantics |
| r2: recursive named slice/pointer/array fatal/hang | Общая depth guard и cycle-safe normalization возвращают ErrSchemaUnsupported |
| r2: field provider + string стирал constraints | Unsupported combination явно отвергается; enclosing provider остаётся escape hatch |
| r2: nullable array primitive становился zero | Recursive nullable mapping сохраняет null, включая nested arrays/objects/typed map values; emitted consumer passes |
| r2: required вне properties удалялся | Непредставимый контракт явно rejected; no-late schema не фильтрует исходный required |
| r3: empty/default item ID ломал stream refs | Independent original reproductions теперь проходят для empty/default/relative/absolute IDs |
| r3: draft7 root $ref sibling closure обходил projection guard | Independent generator probe теперь rejects input/output referenced projections в обоих dialects до closed shortcut |

Поддержанные C4/C7 варианты задачи допускают diagnostic unsupported вместо synthesis рекурсивной schema или универсального unknown-field DTO. В проверенных отказах нет successful schema weakening; caller-owned types и explicit contracts остаются доступными.

## Независимые дополнительные probes

1. **Public inferred stream item IDs:** повторён original r3 `TestAuditR3StreamIDs` с standalone source validation перед stream. Для empty `$id`, canonical default source URI, relative `item.json` и distinct absolute URI invoker вызывается, valid array yields `{N:1}`, error nil. Frozen regressions дополнительно проверяют object/array valid и invalid values в draft7/2020.
2. **Supplied-resource ownership:** `TestAuditR4SuppliedResourceOwner` передаёт source с absolute root ID и local `$defs.n` minimum 10, одновременно регистрируя другой caller resource по тому же canonical ID с противоположным maximum 1. Compiled original source принимает 10 и отвергает 1; source-local constraints не подменены зарегистрированным resource, ресурсы не затёрты переходом на canonical retrieval URI.
3. **Absolute-root reference diagnostics:** `TestAuditR4AbsoluteReferenceDiagnostics` с unsupplied relative missing.json возвращает SchemaCompilationError с `Resource:""`, `Pointer:"/properties/n/$ref"`. Explicit loader по-прежнему не делает network/filesystem IO.
4. **Draft7 fragment item identity:** valid original schema с `$id:"#item"` и local definitions refs сохраняет validation после inferred stream wrapping; probe passes.
5. **Referenced object generation subset:** `TestAuditR4ReferenceProjectionRejects` независимо повторяет original draft7 sibling-closure schema для input/output и дополнительно draft2020. Во всех четырёх случаях explicit `referenced object requires caller-owned type`, successful lossy emission отсутствует.
6. **Dynamic numbers:** independent large integer/nested array/long decimal JSON decoding остаётся json.Number и сохраняет исходное numeric value.

Probe code: root `audit_r4_test.go`, `audit_resources_r4_test.go` и generator `cmd/prompty-gen/gen/audit_r4_test.go` в отдельной temp-copy. Ни один probe не вносился в implementation.

## Полный task38 scope

Заново рассмотрены task38, общий README, contracts/closeout/checks и код всех затронутых paths: dynamic runtime/checkpoint/overlay/late JSON; exact structural comparator; explicit provider errors/dialects; reflection string/omission/null/embedding/custom codecs/recursion; YAML exact numeric tokens и non-JSON rejection; composition repeated-property/root namespace preservation; generated source-schema validation, raw presence, defaults, early/late splitting и nullable child representation; combined runtime input validation/history presence; original output/tool/stream validation и separate domain checks; isolated schema compiler/resources, reference origins/diagnostics и adapter-facing schema contracts.

Зафиксированные границы сохранены: typed subset может явно отказывать; библиотека не добавляет agent loop, storage, retries, model catalog, universal application DTO или implicit resolver. SDK mapping fidelity task42 не приписывается task38. Непроверенные speculative combinations не выдаются за confirmed findings.

## Evidence и ограничения проверки

- Независимый frozen execution: `GOCACHE=/tmp/prompty-go-cache go test ./... ./cmd/prompty-gen/gen ./cmd/prompty-gen/testdata/gen_out` — exit 0, `/tmp/task38-errors-r4-baseline.log`.
- Все дополнительные probes, описанные выше, — exit 0 после исправления expected generator rejection assertion.
- Проверены авторские full logs `/tmp/task38-lint-r4-final.log` и `/tmp/task38-test-r4-final.log`: all workspace modules, race и generated consumers pass; lint zero issues. Full make executions приняты как evidence автора, отдельно от собственного execution; они не были повторно запущены аудитором.
- Live provider requests не выполнялись. Аудит не обещает исчерпывающего доказательства для всех возможных schemas/Go types, но открытых подтверждённых ошибок в проверенном snapshot не осталось. Error acceptance этого frozen snapshot разрешено.
