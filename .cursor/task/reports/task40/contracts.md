# Task40 — контракт source snapshot

Статус: принято r3 (100%, 0 open confirmed errors). База: c8141c5.

- `ManifestSnapshotRegistry.CaptureManifest(ctx, id)` возвращает immutable snapshot. `ManifestCheckpointRegistry` включает этот порт; Registry без него остаётся пригоден для обычного Plan, но не для verified materialization. Execute/BindRuntime требуют только capture-порт; raw bytes остаются отдельной optional capability.
- Snapshot захватывает именованный bundle bytes и parsed manifest closure той же generation и декларативную конфигурацию разрешения. Identity — versioned SHA-256 canonical JSON (сортировка map keys), включающий logical ID, конфигурацию, main/import closure и подключённые partials. Все conditional imports захватываются независимо от runtime values; cycles отклоняются.
- Материализатор получает только копию captured bundle и input; встроенные материализаторы не читают источник повторно. RenderPlan хранит identity своего snapshot. Clone/late binding сохраняют её. Compiled prompt берёт identity из плана; чтение registry после построения плана исключается.
- Verified restore: захват snapshot через текущую cache policy, сравнение descriptor, построение плана из этого же snapshot. При несовпадении — ErrManifestDigestMismatch. Старый snapshot можно явно удержать host и передать через snapshot registry; библиотека не хранит историю ревизий.
- File cache хранит immutable snapshot до Reload. Embed захватывает исходники при создании. Remote без cache захватывает bundle при каждой операции; TTL cache хранит snapshot, включая compose sources, и не делает fresh probes на cache hit.
- Mutable multi-read capture — bundle наблюдавшихся bytes, а не обещание atomic source revision. Immutable revision binding должен предоставляться source port; невозможность binding — ошибка. Git commit binding читает blobs указанного commit, независимо от checkout/pull.
- Environment, partial pattern, resolution mode и явно заданная version входят в конфигурацию. Runtime compose values выбирают ветви внутри captured closure и не меняют source identity.
- Identity не удостоверяет код parser/Go funcmap, внешний I/O, clock/random/model response. Host отвечает за доверие, storage и retention; source snapshot не является checkpoint agent run.
- Canonical/raw constructors готового compiled execution удалены; compiled принимает только captured RenderPlan. Прямой caller-owned execution сохраняется публичным execution wire.
- Breaking formats: descriptor digest имеет обязательный version prefix; recipe/checkpoint и compiled wire отвергают старые неполные source identities. Удаляются post-hoc raw-byte digest constructors для rendered execution.
