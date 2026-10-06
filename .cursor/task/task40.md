# task40 — Единый snapshot для manifest digest, plan и execution

**Статус:** принято r3: полнота 100% (17/17), открытых подтверждённых ошибок нет; lint/test/race зелёные.
**Приоритет:** P1. **База аудита:** `70fc301`; **база реализации:** `c8141c5`, 6 октября 2026.
**Правила:** [README](README.md). Согласовать composition semantics с [task38](task38.md).

## Цель

Verified descriptor должен удостоверять именно исходники, из которых материализуется execution. Кеш, изменение файлов, повторные remote reads или partial templates не должны позволять приписать старому execution новый digest.

## Сверка кода

- [fileregistry/registry.go](../../fileregistry/registry.go), `loadTemplate`: cache по ID возвращает ранее загруженный template.
- [manifest/checkpoint.go](../../manifest/checkpoint.go), `verifyManifestDescriptor`: digest сравнивается со свежими bytes.
- [recipe.go](../../recipe.go) и [runtime.go](../../runtime.go): verification и registry Plan выполняются отдельно.
- [internal/compiled/prompt.go](../../internal/compiled/prompt.go), `FromRenderPlanRegistry`: raw bytes / descriptor читаются независимо от уже построенного plan.
- [manifest/compose_digest.go](../../manifest/compose_digest.go): digest охватывает manifest/import closure, но не подключённые `.tmpl` partials.
- [fileregistry/registry.go](../../fileregistry/registry.go) и [embedregistry/registry.go](../../embedregistry/registry.go) подключают partials отдельно. [remoteregistry/cache.go](../../remoteregistry/cache.go) имеет собственные reads/cache semantics.

Воспроизведённый дефект: прогреть Plan на `OLD` → заменить файл на `NEW` → получить descriptor `NEW` → создать recipe с ним → Execute возвращает `OLD` без ошибки.

## Требования

1. Ввести единый договор verified materialization через существующие registry/descriptor/plan abstractions. Snapshot содержит идентичность и захваченные исходники, необходимые для построения конкретного plan. Не добавлять параллельный независимый каталог manifests.
2. Verification и materialization используют одни захваченные bytes. Простая перестановка вызовов, unconditional Reload или дополнительный hash после чтения не являются устранением TOCTOU.
3. Digest включает deterministic closure исходников: main manifest, transitive imports/layers, подключённые partials и значимую декларативную конфигурацию их разрешения. Сохранить защиту conditional imports и cycle detection. Порядок обхода не влияет на digest; изменение источника, способного изменить template, влияет.
4. Определить область гарантии: digest исходников не удостоверяет код Go funcmap, внешние сервисы, время/randomHex или будущий model response. Такие runtime dependencies принадлежат host и не превращаются в сериализуемый код. Документация не обещает одинаковый текст для недетерминированных функций.
5. Cache хранит snapshot identity вместе с parsed/compiled material. Возвращённый execution не может получить descriptor от другого cache generation. Старый корректный immutable snapshot можно исполнить только под его собственной identity и явно определённой политикой restore; иначе mismatch.
6. Применить контракт ко всем встроенным registries: file/embed/remote/cache/git, typed recipes, raw BindRuntime и compiled prompt. Direct custom registries остаются возможными через небольшой универсальный порт. Неполная custom implementation не должна заявлять verified materialization.
7. Для mutable remote/Git source не обещать atomic revision из нескольких несвязанных чтений. Зафиксировать захваченный bundle; если требуется целостная source revision, использовать явный immutable revision binding и ошибку при невозможности выполнить его. Не строить storage/version-control service внутри prompty.
8. Изменить descriptor/checkpoint формат, если нужно; старые неполные форматы явно отвергать. Обновить codegen constructors и wire contracts, удалить прежние paths, приписывающие digest готовому plan постфактум.

## Acceptance

- [x] OLD-cache / NEW-descriptor сценарий даёт NEW из соответствующего snapshot либо явный mismatch; OLD с digest NEW невозможен.
- [x] Детерминированный fixture меняет source между verify/plan/read; результат никогда не помечен чужой identity. Не использовать sleep-based race reproduction.
- [x] Изменение только imported manifest или partial меняет identity; reorder обхода при неизменном bundle не меняет её.
- [x] Проверены file, embed, remote cached/uncached, Git и custom registry contract; источник может исчезнуть после capture без подмены уже захваченного snapshot.
- [x] Concurrent Reload/Plan/Execute сохраняют isolation и проходят race tests; execution/plan не мутируют caller-owned inputs.
- [x] Recipe checkpoint, raw BindRuntime и internal compiled используют один смысл digest.
- [x] Environment, conditional composition, runtime compose values и partial resolution покрыты fixtures.
- [x] Новая документация чётко отделяет source integrity от deterministic rendering и durable agent state.

Прямой filesystem/Git registry не объявлять sandbox для недоверенных файлов. Host остаётся владельцем доверия к источнику, доступа, сохранения snapshot и retention. Source hardening не должен превращать эту задачу в общий filesystem security framework. Проверки/миграция — по README.

## Приёмка r3

Snapshot `c78e268c2e185fc856abc3f6395349d6e0a17ca83a7f6deaaac1202e553ad01e`, 704 файла. Независимые отчёты: [полнота](reports/task40/completeness-r3.md), [ошибки](reports/task40/errors-r3.md), [проверки](reports/task40/checks-r3.md). Предыдущие два отклонённых раунда сохранены.
