# Task40 — независимая приёмка ошибок, round 2

**Решение: не принято. Открытых подтверждённых ошибок: 1 (P1).** Четыре исходных r1 probes проходят, однако parser source isolation остаётся неполной в remote read path. Implementation/source не менялись; текущий peer completeness report не читала.

## Identity и выполненные проверки

Frozen `/tmp/prompty-task40-snapshot-02`, база `c8141c5`, SHA256 manifest `58250bf7ef16b94095539574706d1a87f322c629a533fbe52d035e3386711c53`. Identity совпала; все **701 файлов** verified до/после аудита. Логи `/tmp/task40-errors-r2-integrity.log`, `/tmp/task40-errors-r2-integrity-final.log`.

Повторно прочитаны task40, общий README, contracts, closeout/checks-r2 и публичный source-snapshot contract; весь scope проверялся независимо от r1 findings. Frozen и рабочая implementation оставались read-only. Probe copy `/tmp/prompty-task40-errors-probes-r2`.

Независимые full runs на frozen:

- `GOCACHE=/tmp/prompty-go-cache GOLANGCI_LINT_CACHE=/tmp/prompty-lint-cache make lint`: **exit 0**, все 12 modules, zero issues; `/tmp/task40-errors-r2-lint.log`.
- `GOCACHE=/tmp/prompty-go-cache make test`: **exit 0**, все modules/race/generated consumers; `/tmp/task40-errors-r2-test.log`.
- Исходные четыре r1 probes, перенесённые без изменения assertions: `go test -race -run TestAudit -v . ./cmd/prompty-gen`: **exit 0**; `/tmp/task40-errors-r2-probes.log`.
- Дополнительный remote parser adversarial probe с race: **exit 1** на подтверждённом нарушении; `/tmp/task40-errors-r2-remote-probe.log`.

Live remote/provider conformance не утверждается. Git fixtures локальные. Green full checks не покрывают новый remote edge case.

## F1 — P1: remote compose peek меняет observed bytes до parser capture guard

**Места:** `remoteregistry/registry.go:66–72`, особенно вызов `manifest.PeekComposeFieldsE(data,r.parser)` в строке 69; `manifest/compose.go:105–113`, вызов `u.Unmarshal(data,&raw)` в строке 110. Guard в `manifest/snapshot.go:193–203` применяется только после этого незащищённого чтения.

Remote registry fetch получает source bytes, затем делает metadata peek тем же caller-supplied parser на этом slice и только после peek возвращает bytes в `CaptureSources`. Если parser успешно разбирает OLD и нормализует/mutates свой input в NEW, CaptureSources получает уже изменённый NEW. Второй guarded parse не видит нарушения, если mutation idempotent. Identity и execution описывают bytes, которые source не возвращал. Обещание private read-only parser input и captured observed bytes обойдёно. TTL cached registry вызывает этот же base CaptureManifest, поэтому freeze cache не устраняет исходную corruption.

**Executed reproduction:** `/tmp/prompty-task40-errors-probes-r2/audit_task40_r1_test.go`, `TestAuditRemoteParserCaptureIsolation`.

1. Fetcher всегда возвращает fresh bytes `oldSource` с text OLD.
2. Parser выполняет обычный `json.Unmarshal(data,out)`, после чего in-place заменяет OLD→NEW (равная длина).
3. `remoteregistry.New(...WithParser(parser))`, `CaptureManifest(ctx,"agent")`, snapshot.Plan и Execute проходят без ошибок.
4. Лог: **fetch returned OLD**, captured manifest содержит **NEW**, rendered text **NEW**.

Команда: `GOCACHE=/tmp/prompty-go-cache go test -race -run TestAuditRemoteParserCaptureIsolation -v .` в отдельной копии. Exit 1 на assertion «remote compose peek mutates observed source before capture guard». Действия последовательны; race detector не обнаруживает гонку. Это устранимая library ownership ошибка, а не требование доказать корректность произвольного parser code или аттестовать Go implementation.

**Необходимая коррекция:** устранить parser access к original fetch bytes до sealing: убрать ненужный compose peek из verified read path либо дать ему изолированный input с тем же mutation rejection. Не проглатывать detected source mutation в optional metadata probe. Проверить cached/uncached remote capture и остальные preliminary parser calls. Captured digest должен удостоверять полученные source bytes, а successful materialization не должна принимать source mutation.

## Повторная проверка r1 fixes

- **R1 parser file input mutation:** исходный file probe теперь отклонён capture; old/new inconsistent snapshot не возникает. Однако class остаётся открыт в remote path выше.
- **R1 retained parsed alias:** deep clone после parse закрывает последовательное изменение parser-retained RawManifest: captured OLD, rendered OLD; прежний probe pass. Parsed imports/conditional closure также sealed до обхода.
- **R1 nested compose alias:** deep map cloning и copied Lookup result закрывают public nested probe: caller input=true, original token=true после materializer mutation. Existing positive condition tests проходят; source identity не зависит от runtime selection.
- **R1 codegen alias collision:** explicit logical-ID/path aliases tracked с owner и ambiguous collision rejected; исходный probe pass. Проверены author tests для обоих порядков, обычный captured-index/source replacement и generated consumers. Schema/digest source больше не silently заменяется чужим path alias в воспроизведении r1.

## Полный scope и границы

Повторно проверены file cache old/new generation mismatch, Reload и concurrent Plan/Execute isolation; main/import/partial disappearance после capture, partial-only identity change и canonical map ordering; embed construction sealing и conservative validation. Mutable file/remote capture по-прежнему правильно документирован как bundle наблюдавшихся reads, не atomic source transaction. Remote TTL retains captured closure и source identity, не делает fresh compose probes на cache hit; concurrent inflight waiter cancellation и race suites проходят. Новый F1 находится до создания remote bundle, а не в TTL generation pairing.

Conservative capture включает main/transitive/inactive imports, layers и configured partials, обнаруживает cycles/missing imports. Runtime conditions выбирают внутри captured closure, environment/config/version/partial resolution/revision входят в identity. File и embed partial resolution modes явно различены. Context gates перед capture/Plan и после materializer не приписывают identity отменённому successful plan. Materializer получает copied sources, source mutation reject остаётся; внешние Go functions/services/clock/random/model response не включаются в digest.

Typed recipes и raw BindRuntime используют один Capture→compare→snapshot.Plan, без independent Verify/Plan TOCTOU. BYOT minimal capture port поддержан, ordinary Registry не обязан заявлять stronger guarantee. RenderPlan SourceDescriptor сохраняется через clone/late/output/scope bindings, Template getter возвращает clone. Internal compiled FromRenderPlan читает identity из plan без storage reread; post-hoc raw/canonical constructors удалены. Compiled v3/core execution wire сохраняет scope/state; legacy incomplete descriptor/compiled wire явно отвергаются. Digest остаётся integrity token sources, не подпись доверенного stored execution; host отвечает за auth/storage/retention.

Git revision binding использует полный commit hash и commit blobs, moving refs/unavailable/unsupported binding reject без mutable fallback. Local revision fixture, all module race suite, adapter/example/codegen consumers passed. Host trust/BYOT boundaries сохраняются, источник не объявлен filesystem sandbox; агентный loop/version-control/storage backend не введены.

Дополнительных подтверждённых ошибок сверх remote F1 не найдено в выполненных проверках. Для принятия нужен новый frozen snapshot с закрытым F1 и повторной независимой проверкой. Этот вывод не абсолютная гарантия отсутствия любых дефектов.
