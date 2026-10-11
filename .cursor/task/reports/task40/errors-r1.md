# Task40 — независимая приёмка ошибок, round 1

**Решение: не принято. Открытых подтверждённых findings: 4 (три P1, один P2).** Frozen/source не изменялись; в implementation не участвовала и текущий peer completeness report не читала.

## Версия и executed evidence

Frozen `/tmp/prompty-task40-snapshot-01`, база `c8141c5`. Manifest SHA256 `3726add65ce44f68b23b4e86d60309285bae3d7c48aa78272c6ebd2688549cfe` совпал; все **697 файлов** verified до/после аудита. Логи `/tmp/task40-errors-r1-integrity.log`, `/tmp/task40-errors-r1-integrity-final.log`.

Прочитаны task40, общий README, contracts, closeout, checks-r1 и публичный source-snapshot contract; проверен весь scope built-in/custom capture, source closure/config/conditions, cache поколения, recipe/raw runtime binding, compiled/source descriptor/wire и codegen. Независимо выполнены на frozen:

- `GOCACHE=/tmp/prompty-go-cache GOLANGCI_LINT_CACHE=/tmp/prompty-lint-cache make lint`: exit 0, все modules, zero issues. `/tmp/task40-errors-r1-lint.log`.
- `GOCACHE=/tmp/prompty-go-cache make test`: exit 0, все modules/race/generated consumers. `/tmp/task40-errors-r1-test.log`.

Adversarial probes только в separate copy `/tmp/prompty-task40-errors-probes-r1`:

- Root `audit_task40_r1_test.go`; `go test -race -run TestAudit -v .`: exit 1, все три assertions воспроизводят нарушения; `/tmp/task40-errors-r1-probes-expanded.log`.
- Module `cmd/prompty-gen/audit_task40_r1_test.go`; `go test -run TestAuditCapturedIndexAliasCollision -v .`: exit 1, assertion воспроизводит различие source/schema; `/tmp/task40-errors-r1-codegen-probe.log`.

Все четыре findings подтверждены executed repro; гипотезы не включены в blockers. Live remote/provider calls не выполнялись. Git suite использует локальные fixtures.

## F1 — P1: parser может заменить bytes, которые получают identity после разбора

**Место:** `manifest/snapshot.go:80–90`, особенно `Unmarshal(data, &raw)` в строке 85 и сохранение того же data в строке 89.

Capture копирует bytes источника, но передаёт эту рабочую копию parser до sealing. Parser может успешно разобрать исходные bytes и затем изменить переданный mutable slice. Captured bundle/digest сохраняет изменённые bytes, а parsed closure — прежний результат. Обещание «verification и materialization используют одни захваченные bytes» нарушается.

**Repro:** `TestAuditParserSourceIdentity`. File registry использует parser, который выполняет обычный `json.Unmarshal(data,out)`, затем in-place заменяет `OLD` на `NEW` без изменения длины. Capture и Plan проходят без ошибок. Наблюдение: `Sources()["manifest:agent"]` содержит **NEW**, execution рендерит **OLD**. Ни Go funcmap, ни повторные storage reads не участвуют.

**Коррекция:** сохранить observed source bytes отдельно от parser argument; дать parser изолированный input и reject detected source mutation либо гарантировать, что accepted parsed material относится именно к неизменённому source. Защита должна применяться ко всему capture closure и дополнительным parser calls на source paths.

## F2 — P1: sealed parsed closure сохраняет alias parser-owned output

**Место:** `manifest/snapshot.go:84–90`, storing `s.parsed[id] = &raw`; closure в строках 35–49 использует этот объект позднее. Clone при построении шаблона происходит после возможного внешнего изменения и не восстанавливает исходное состояние.

**Repro:** `TestAuditParsedSnapshotIsolation`. Parser выполняет только нормальный json decode и сохраняет переданный `*manifest.RawManifest`. После успешного Capture caller через этот сохранённый объект меняет text OLD→NEW (никакой concurrency или async worker нет). Snapshot Sources остаётся **OLD**, тот же snapshot.Plan/Execute рендерит **NEW** без ошибки. Captured immutable snapshot имеет mutable output alias наружу.

**Коррекция:** seal parsed material defensive deep clone на границе получения от parser, до сохранения/обхода imports. Не полагаться на поздний clone уже изменённого parser-owned объекта.

Доверие к произвольному parser/materializer code и исключение его implementation из digest остаются правильной library boundary: невозможно доказать корректность arbitrary host code. Эти два findings касаются устранимого ownership/aliasing в библиотеке, а не требования доказать семантику parser или сериализовать Go code. Public Unmarshaler не объявляет ownership запрета на reuse или mutation предоставленных buffers; strong immutable snapshot должен изолировать собственные retained sources/material.

## F3 — P2: nested ComposeValues alias позволяет materializer менять caller input

**Места:** `plan_input.go:45–57`, cloneComposeMap копирует только один nested уровень через maps.Clone; `manifest_snapshot.go:103–104`, Plan передаёт input materializer; public ComposeValues.Lookup возвращает mutable subtree.

**Repro:** `TestAuditMaterializerInputIsolation`. Caller создаёт `NewComposeValuesFromPairs(ComposeBool("feature.deep.on",true))` и PlanInputWithComposeContext. Custom materializer вызывает `input.ComposeValues().Lookup("feature.deep")`, получает map и устанавливает `on=false`. После успешного Plan **caller input=false и original token=false**. Sources не меняются, source-mutation guard молчит. Все действия последовательны; race detector не находит гонку, но ownership нарушен.

**Коррекция:** deep clone всех nested compose values и не возвращать mutable aliases через публичный Lookup; обеспечить изоляцию token/input на materializer границе. Это acceptance о caller-owned inputs и runtime composition, а не расширение task в общий state engine.

## F4 — P1: codegen captured index может связать schema и descriptor разных manifests

**Место:** `cmd/prompty-gen/main.go:173–180`, unconditional записи explicit ID и path-derived alias в `index.byID`. Raw loader при этом индексируется другой политикой — уникальными explicit IDs. `loadSpec` берёт byPath, а descriptor capture — byID.

**Repro:** `TestAuditCapturedIndexAliasCollision`:

- `a.json`: explicit `id:b`, inputs.first:string, text FIRST.
- `b.json`: explicit `id:c`, inputs.second:integer, text SECOND.
- `indexManifests([a,b], dir,["."])` успешно: explicit IDs b,c не повторяются.
- `loadSpec(a,...)` возвращает ID b и schema **first:string**.
- `CaptureManifest(spec.ID=b)` возвращает bytes **b.json / id:c / second:integer**: его path alias b перезаписал explicit b первого файла.
- Materialized captured plan рендерит **SECOND**.

В результате штатный codegen flow может присвоить spec из a identity источника b. Это происходит без изменения файлов, custom parser и гонки. Изменение порядка files меняет победителя alias; политика зависит от обхода. Green one-file source-replacement fixture не покрывает collision.

**Коррекция:** reject неоднозначные logical-ID/path-alias collisions либо применять единую явную resolution policy для schema loader и source index; schema/digest constructors обязаны выбирать одно captured source. Добавить оба порядка обхода и import-alias cases.

## Остальной scope и предел проверки

File old-cache/new-descriptor fixture корректно даёт mismatch, Reload выбирает новую generation, старые удержанные snapshot/plan сохраняют identity. Partials включены в bundle вместе с deterministic names/resolution settings; partial-only change меняет digest, исчезновение sources после capture не требует reread. Embed seals caller FS bytes и conservatively validates construction. Remote uncached получает новый bundle по операции; TTL cache хранит полный snapshot, composition на cache hit не делает fresh probes. Shared inflight cancellation отделяет waiters; all module race suites прошли. Mutable multi-read bundle документирован как наблюдавшиеся bytes, без обещания atomic revision.

Conservative import capture включает inactive/transitive imports и обнаруживает cycles/missing imports. Runtime compose values выбирают captured ветки; environment, partial resolution, version/revision включены в config. Map key traversal canonical; codegen alias F4 отдельно нарушает mapping источника, а не canonical JSON hashing.

Typed recipe и raw BindRuntime используют Capture→compare→Plan одного snapshot и не вызывают independent Verify/Plan. Минимальный BYOT capture port поддержан, неполный ordinary Registry не заявляет verified materialization. RenderPlan descriptor сохраняется clone/late/output/toolscope binding. Compiled FromRenderPlan берёт identity из plan, не делает storage reads; legacy raw/canonical post-hoc constructors удалены. Descriptor/source digest versions и compiled v3 reject legacy wire; host storage trust/retention/auth остаются внешними, digest не подпись rendered execution.

Git immutable binding принимает полный commit hash, читает commit tree regular blobs, отвергает moving refs/unavailable commit/unsupported binding и не переходит на mutable checkout fallback. Local Git regression и все adapter/example/generated consumers прошли независимый full run. Context gates capture/plan и source mutation rejection проверены; detached handler cancellation/agent loop/storage service не вводятся.

Новых подтверждённых findings сверх F1–F4 в выполненном полном обзоре не обнаружено. Приёмка блокирована этими четырьмя воспроизведёнными нарушениями; требуется новая frozen версия и повторные независимые проверки. Вывод не является абсолютной гарантией отсутствия других дефектов.
