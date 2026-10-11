# Task40 — независимая приёмка ошибок, round 3

**Решение: принято по проверке ошибок. Открытых подтверждённых ошибок: 0.** Это результат выполненного полного обзора и probes, не абсолютная гарантия отсутствия любых дефектов. В implementation не участвовала; текущий peer completeness report не читала.

## Identity и independent evidence

Frozen `/tmp/prompty-task40-snapshot-03`, база `c8141c5`. Manifest SHA256 **c78e268c2e185fc856abc3f6395349d6e0a17ca83a7f6deaaac1202e553ad01e** совпал. Все **704 файла** verified до/после аудита: `/tmp/task40-errors-r3-integrity.log`, `/tmp/task40-errors-r3-integrity-final.log`. Frozen/source не изменены. Additional probes находятся только в `/tmp/prompty-task40-errors-probes-r3`.

Прочитаны весь task40, общий README задач, contracts, closeout/checks-r3, публичный source-snapshot contract. Обзор охватывает весь scope, не только исправленные parser sites.

Независимо выполнены:

- На frozen `GOCACHE=/tmp/prompty-go-cache GOLANGCI_LINT_CACHE=/tmp/prompty-lint-cache make lint`: **exit 0**, все 12 modules, zero issues. `/tmp/task40-errors-r3-lint.log`.
- На frozen `GOCACHE=/tmp/prompty-go-cache make test`: **exit 0**, все modules/race/generated consumers. `/tmp/task40-errors-r3-test.log`.
- В separate copy все четыре r1 и remote r2 reproduction assertions без изменения: `go test -race -run TestAudit -v . ./cmd/prompty-gen`: **exit 0**. `/tmp/task40-errors-r3-probes.log`.
- Расширенная самостоятельная suite добавляет standalone peek mutation, cached remote capture mutation, canceled materializer/post-materializer cancellation: **exit 0** с race. `/tmp/task40-errors-r3-probes-expanded.log`.

Probe sources: root `audit_task40_r1_test.go` и module `cmd/prompty-gen/audit_task40_r1_test.go` в temp-copy. Live remote/provider conformance не заявляется; Git fixtures локальные.

## Closure прежних подтверждённых findings

**R1 F1/parser input mutation и R2 remote pre-peek bypass: закрыты.** File и remote captures с parser, выполняющим json decode затем OLD→NEW in-place, отвергаются до получения snapshot. Remote ReadManifestBytes больше не запускает parser до sealing. Standalone PeekComposeFieldsE также работает через private input и mutation rejection: caller bytes остаются OLD, returned error содержит ErrInvalidManifest. Cached remote wrapper не создаёт успешный snapshot из rejected mutable source. Shared captured parser helper остаётся единым guard, а не дополнительным hash после построения готового plan.

**R1 F2/retained parsed alias: закрыт.** Parser-retained RawManifest изменён после Capture; independent probe получает captured OLD и rendered OLD. Parsed output deep-cloned до cache и import traversal; поздний parser reuse не меняет source generation.

**R1 F3/nested ComposeValues alias: закрыт.** Public nested materializer probe сохраняет caller input=true и original token=true. Clone map и Lookup возвращают owned deep copies; nested runtime values остаются отдельно от source identity.

**R1 F4/codegen ID/path alias collision: закрыт.** Independent two-file reproduction теперь отвергается index construction. Existing collision tests проверяют оба порядка. Source replacement fixture и generated consumers проходят; schema и descriptor берутся из captured file generation, а ambiguous logical-ID/path ownership не разрешается silent overwrite.

## Полный scope проверки

**Capture и integrity.** Main/transitive/inactive conditional imports, layers и configured partials захватываются один раз в именованный closure; missing imports и cycles приводят к ошибке. Canonical identity учитывает logical ID, named source bytes и значимую declarative config; map traversal order не влияет на digest. Imports/partials-only changes меняют identity; runtime condition selection не подменяет closure. Environment, version, partial pattern/resolution mode и revision binding отражены в config. Source digest не аттестует parser/funcmap Go implementation, внешние services, random/time или будущий model response.

**Built-in registries и cache pairing.** File cache stores snapshot identity и parsed generation; old-cache/new-descriptor mismatch воспроизводится без sleep, Reload получает новую generation, retained old snapshot исполним под собственной identity. Concurrent Reload/Plan/Execute прошли race; Template/Sources/getter clones защищают owned source/input data. Embed копирует source bytes при construction, conservative validation не требует runtime condition values. Partials имеют deterministic names/parse order; исчезновение main/import/partial storage после capture не инициирует reread готового snapshot.

Remote uncached делает новый capture по операции; TTL cache удерживает complete snapshot/compose closure и identity без fresh probes на cache hit. Shared inflight capture сохраняет отдельную cancellation каждого waiter; all waiter cancellation не превращается в successful generation. Cache instances остаются host-scoped источником/trust domain. Mutable multi-read file/remote bundle документирован как наблюдавшиеся bytes, не atomic filesystem/remote revision. Remote r2 mutation происходила до sealing и теперь устранена.

**Git immutable source.** WithRevision требует RevisionFetcher; unsupported binding/unavailable commit/moving ref не дают mutable fallback. Git full commit hash выбирает commit tree regular blobs независимо от checkout/pull. Shallow commit availability остаётся явной host prerequisite. Local Git regression и full race suite прошли. FS/Git registries не объявлены sandbox или version-control/storage service.

**Recipes/materialization и cancellation.** VerifiedManifestPlan выполняет capture→identity compare→Plan одного snapshot, а не независимые Verify/Plan storage reads. Typed recipes и raw BindRuntime используют этот протокол, minimal custom capture port достаточен; incomplete ordinary Registry не получает stronger verified guarantee. Источник может исчезнуть после Capture, если host удерживает snapshot. Custom materializer получает copied bundle и opaque isolated runtime input; detected source mutation reject сохраняется.

Самостоятельный cancellation probe подтверждает: materializer завершился после cancel — plan не возвращён, context cause сохранена; повторный Plan с уже canceled context не запускает callback. Built-in capture/Plan проверяют context; detached kill goroutines, retries/rollback и agent loop не добавлены. Concurrent invocation/source isolation подтверждены full race suite и собственными последовательными ownership probes.

**Plan/compiled/codegen/wire.** SourceDescriptor сохраняется clone/late binding/response-format/tool-scope overrides; materialization не подменяет его post-hoc descriptor. Internal compiled.FromRenderPlan берёт identity только из captured plan, не читает registry, uncaptured plan reject. Raw/canonical constructors digest assignment удалены. Breaking versioned descriptor и compiled wire v3 отвергают старые incomplete identities; executable JSON Schema и wire conformance fixtures проходят. Core execution wire сохраняет tool scope/provider continuation state. Generated schema/constructors/consumer suite согласованы с captured index.

Source integrity token не является подписью сохранённого rendered execution. Host сохраняет контроль над trust/auth/storage/retention, revision history, runtime Go dependencies и restore policy. Библиотека не добавляет новый агентный checkpoint, IAM backend или обязательный application container.

## Итог

Все пять прежних reproduction paths и новые дополнительные probes проходят; full independent lint/test/race/generated checks прошли. В выполненном полном review этой frozen версии новых подтверждённых ошибок/регрессий не обнаружено. Ошибок, блокирующих принятие task40 r3 по заявленному source contract, нет.
