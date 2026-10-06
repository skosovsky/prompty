# Task40 — implementation closeout

Статус: принято r3: 17/17=100%, 0 open confirmed errors, full lint/test/race green. База c8141c5.

Изменены source materialization contracts: immutable ManifestSnapshot/SourceBundle, универсальный capture-порт, source-only materializer с защитой от мутации источников, versioned descriptor, capture-verify-plan в одной generation. Typed Execute/Plan и BindRuntime принимают маленький capture-порт. Обычный Registry остаётся самостоятельным BYOT путём без verified claims.

File cache и TTL cache хранят bundle generation и захваченный parsed manifest closure вместе с её identity; composition больше не требует fresh cache probes. Embed копирует исходники на construction и проверяет шаблоны консервативно. RenderPlan сохраняет SourceDescriptor через clone/runtime binding. Compiled snapshot читает identity из плана, использует core execution wire/StatePreserve и wire version 3.

Codegen читает schema и digest из захваченного file index; generated constructors/consumers и goldens обновлены. JSON Schema descriptor исполняется в conformance fixture и соответствует wire decoding. Host может удержать snapshot после исчезновения источника; history/storage/retention не добавлены.

Удалены старые Verify→отдельный registry Plan paths, повторные reads для digest готового compiled execution, raw-byte и canonical compiled constructors, старый compose-only digest helper и cached compose probes. Не создаются compatibility aliases. Legacy source digests и compiled wire отвергаются. File template metadata больше не подставляет версию из отдельного Stat read.

Immutable remote binding — явный RevisionFetcher/WithRevision; Git читает blobs полного commit hash независимо от checkout. Moving references, unavailable commits и unsupported binding не подменяются mutable fallback. Mutable multi-read capture гарантирует captured bundle, а не atomic remote revision. Код parser/materializer/funcmap, clock/random/services/model responses не удостоверяются source digest. FS/Git не являются sandbox; host владеет trust и durable agent state.

Миграция: регенерировать constructors и checkpoints; реализовать capture-порт для custom verified registry; использовать CaptureManifest/VerifiedManifestPlan, удерживать snapshot при host-owned restore policy. Для compiled использовать FromRenderPlan(ctx, capturedPlan), не читать registry после materialization.

Round2: parser получает отдельную копию bytes, mutation отклоняется, RawManifest deep-cloned до cache. Compose map/Lookup deep-copy. Codegen reject logical-ID/path alias collisions в обоих порядках индексации. Добавлены adversarial regression tests для всех четырёх замечаний.

Round3: remote ReadManifestBytes больше не вызывает parser до capture. PeekComposeFieldsE использует тот же guarded ownership helper; readonly input и mutation rejection действуют и для metadata probes. Remote/standalone peek regression fixtures сохраняют fetcher-owned bytes и отвергают mutation.

Принятый snapshot: c78e268c2e185fc856abc3f6395349d6e0a17ca83a7f6deaaac1202e553ad01e (704 файла). Source-хеши рабочего дерева проверены до обновления только acceptance metadata.
