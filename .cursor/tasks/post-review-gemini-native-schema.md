# Перенести native JSON Schema transport в Gemini adapter

Статус: IN PROGRESS — исправление и регрессии выполнены; immutable review/release verification ниже.

**Основание:** PROMPTY-S3 — конкретный пробел adapter API, не доказанный production bug. Проверено на v0.16.0.

[Профильный отчёт](</Users/skosovsky/Library/Mobile Documents/com~apple~CloudDocs/Загрузки/Projects/ai/ai-libs/reviews/kosmify-post-upgrade-2026-10-11/prompty.md>) · [Общие результаты](</Users/skosovsky/Library/Mobile Documents/com~apple~CloudDocs/Загрузки/Projects/ai/ai-libs/reviews/kosmify-post-upgrade-2026-10-11/README.md>). Пути к коду ниже относительны репозиторию, указанному в контексте.

## Зависимости

Нет. Сначала сверить замечание с актуальным исходным кодом и опубликованными модулями.

## Конкретное изменение

Добавить узкий явно выбираемый native JSON Schema mode/profile для сценария, уже реализованного в Kosmify. Зафиксировать поддерживаемые endpoint/model semantics и subset схемы до реализации. Сверить с официальным контрактом используемой версии provider SDK; не обещать всю JSON Schema.

Переносимый механизм: internal/infrastructure/ai/gemini/json_schema_mapper.go:14–136, checked_json.go:27–44 и checked_tools.go:62–73 в Kosmify. В опубликованном adapter schema.go и gemini.go используют ограниченную typed ResponseSchema. Business schema policy, endpoint selection, budgets и telemetry остаются у приложения.

Не добавлять новый provider framework. Режим должен сохранить original-schema validation; недопустимо молча отбрасывать неизвестные semantics ради успешного dispatch.

## Детализация реализации по коду

### Baseline и границы adapter

Повторно прочитан текущий локальный prompty `a36a34382a24213f2db0c01a36ebd5d8d9694a67` (2026-10-11). Восстановление истории/API v0.16 уже выполнено `14bb258`; старое предупреждение о permissive checkout `4fe0d027` более неактуально. `Adapter.MapSchema`, `SchemaMappingRequest`, `SchemaFidelityError` и checked preparation присутствуют. Native JSON schema mode **всё ещё отсутствует**: `MapSchema` вызывает ограниченный `mapToGenaiSchema`, output получает `ResponseSchema`, а не `ResponseJsonSchema`.

Точки изменения теперь: `adapter/gemini/gemini.go` — `Adapter`, `Option`, `Adapter.Translate` (:119) и **существующая** свободная функция `applyToolsAndResponseFormat` (:548), которую Translate вызывает на :164. В опубликованной v0.16 функция была inline, поэтому старое утверждение о её отсутствии относилось только к тому snapshot. Также `schema.go:Adapter.MapSchema/mapGenaiSchemaAt`, `schema_test.go:schemaDispatchFixture/TestStrictSchemaRejectsLossBeforeDispatch`.

Начать с существующих consumer tests `TestCheckedJSONNativeSchemaTransportPreservesExactConstraints` и `TestCheckedJSONNativeSchemaRejectsUnmappedSemanticsBeforeDispatch` в Kosmify. Он по-прежнему закрепляет v0.16.0, remote публикация новой ветки не проверялась. Focused `go test -count=1 -run 'Schema' .` в отдельной копии актуального adapter/gemini — PASS; это подтверждение нынешнего strict typed mode, не готовности native mode.

### Предлагаемый минимальный контракт

**Новое, предлагаемое API:** один opt-in `WithNativeJSONSchema()` с private boolean в Adapter. Никакой реестр стратегий/новый provider framework не нужен. Конкретные capabilities проверяются уже существующим profile/endpoint механизмом; наличие option само по себе не доказывает поддержку endpoint. Старый constructor без option сохраняет typed-schema поведение.

1. В `MapSchema` для выбранного native mode проверить `request.Profile` через существующую привязку provider/endpoint/model. Проверки только `Provider == "gemini"` недостаточно для произвольного endpoint.
2. `prompty.SchemaDocumentAsMap(request.Source)` → admission supported subset → `prompty.SchemaMapping{Schema: prompty.CloneJSONDocument(request.Source)}`. Не конвертировать numeric constraints через `float64`; `json.Number` должен дойти до JSON marshaling без изменения лексемы/значения.
3. Для output schema в `Translate` использовать проверенную schema из execution: `ResponseMIMEType="application/json"`, `ResponseJsonSchema=schema`, `ResponseSchema=nil`. В typed mode сохранить обратное поведение. Не допускать одновременную отправку двух schema полей.
4. Для tool schemas использовать существующий `ParametersJsonSchema`, admission тем же mode/profile контрактом; не использовать typed `Parameters` conversion как fallback. Не менять names, tool choice и checks captured tool names.
5. Checked core продолжает проверять **ответ по original schema**, а не ослабленной transport schema. Admission failure — `SchemaFidelityError` с детерминированным JSON pointer и нулём dispatch. Не логировать целиком пользовательскую схему в ошибке.

Wire assignment (эскиз, `native` — новое private поле, не существующий API):

```go
if a.nativeJSONSchema && exec.ResponseFormat != nil {
    schema, err := prompty.SchemaDocumentAsMap(exec.ResponseFormat.Schema)
    if err != nil { return nil, err }
    req.Config.ResponseMIMEType = "application/json"
    req.Config.ResponseJsonSchema = schema
    req.Config.ResponseSchema = nil
}
```

В текущем коде wire config собирается внутри свободной `applyToolsAndResponseFormat(config, exec, wantGoogleSearch)`: у неё нет receiver `a` и переменной `req`. При реализации превратить её в private метод Adapter (минимально: добавить receiver и изменить один вызов в Translate), затем использовать `config.ResponseJsonSchema`/`config.ResponseSchema` непосредственно. Фрагмент выше показывает присваивания в request, а не готовый patch к этой функции. Ветвить native/typed **до** вызова `mapToGenaiSchema`; не вызывать typed mapping перед native-supported keyword. Direct Translate не должен молча пропускать keyword admission; переиспользовать тот же private mapper/validator. Проверка фактического profile остаётся у checked preparation, где известна identity.

### Ограниченный subset, без обещания всей JSON Schema

Исходный host механизм: `json_schema_mapper.go:mapNativeJSONSchema/validateNativeJSONSchema`, companion `rejectNativeJSONSchemaCycles`. Перенести алгоритм, но прежде сверить его subset с официальным provider SDK/endpoint контрактом и зафиксировать ссылку/версию в adapter docs. Аудит проверял `google.golang.org/genai v1.72.0`; текущую SDK версию взять из `adapter/gemini/go.mod`. Письменное наличие поля в Go struct не означает поддержку каждого keyword сервером.

Минимум для parity: object/properties/required, array/items, string/number/integer, nested minimum/maximum, additionalProperties bool/schema, поддерживаемые enum. Прочие keyword из host (`$defs`, локальный `$ref`, prefixItems, anyOf, propertyOrdering, format и т.д.) поддержать только после проверки endpoint profile; если его контракт не установлен — явный preflight reject, не удаление keyword. Не вводить скрытое ослабление схемы ради успешного вызова. Если narrower subset не покрывает consumer fixture, очистка host shim остаётся заблокирована, это не «готово».

Обход схемы — рекурсивный по schema positions, с sorted keys и JSON Pointer escaping `~→~0`, `/→~1`; не трактовать произвольный пользовательский object/enum value как вложенную schema. Проверять типы значений keyword, а не только их названия. References: внешние запретить; локальные валидировать resolve/cycle; unresolved, reference siblings с непереносимой semantics и cycles — reject до сети. Существующие host helpers не копировать слепо вместе с business budgets/telemetry.

### AAA и команды

- Arrange native option + допустимый effective profile + exact schema `minimum:9007199254740993`, decimal maximum и `additionalProperties:false`; Act checked client с fake transport; Assert реально сериализованное request body содержит точные числа, один schema field и исходные restrictions.
- Output и tool schema fixtures раздельные: `ResponseJsonSchema` и `ParametersJsonSchema`; вложенные objects/arrays тоже сохраняют constraints. Typed default fixtures не меняются.
- Arrange unsupported keyword/malformed keyword/local missing ref/cycle/external ref/foreign profile; Act checked dispatch; Assert `SchemaFidelityError`, ожидаемый pointer, calls=0. Ошибка не превращается в retry/typed fallback.
- Arrange schema с minimum, fake provider возвращает ниже minimum или extra property; Act checked structured call; Assert original-schema validation rejects, хотя transport был native. Пробный fake, не live Gemini.
- Existing consumer parity fixture работает через публичный SDK option без `MapSchema`/`Translate` override; разрешается оставить host Execute override для budget/telemetry, это иная ответственность.

```sh
(cd adapter/gemini && GOWORK=off go test -count=1 ./...)
(cd adapter/gemini && GOWORK=off go vet ./...)
make lint
make test
make test-integration
make test-e2e
```

Отдельно пройти consumer fixture на опубликованном релизе перед удалением shim; результаты команд и provider-contract verification фиксировать фактически, не объявлять live compatibility по одному fake transport.

## Условия приёмки

- [ ] Wire fixtures output и tool schemas сохраняют minimum/maximum/additionalProperties и точные числовые ограничения; проверяется реально отправляемый request.
- [ ] Неподдержанные keyword/reference/cycle отклоняются до I/O, число provider calls равно нулю. Допустимый subset и ограничения описаны явно.
- [ ] Сохраняется проверка ответа по исходной схеме; существующий режим adapter не меняет поведение без явного выбора нового режима.
- [ ] Consumer fixtures TestCheckedJSONNativeSchemaTransportPreservesExactConstraints и TestCheckedJSONNativeSchemaRejectsUnmappedSemanticsBeforeDispatch (найти актуальные имена) проходят через публичный SDK API без host MapSchema/Translate overrides.

## Порядок работы и приёмка двумя субагентами

Работа начинается с текущего git status, инструкций репозитория и проверки исходного finding: отчёт относится к Kosmify `2de8c9204f6108faaf06e2f329e0035e088c4003` и его pinned SDK. Локальный HEAD, remote основная ветка и опубликованный модуль могут различаться. Если уже исправлено, доказать это focused regression и версией, не переписывать работающий код. Чужие изменения не затрагивать.

Использовать contract-first и AAA. Только перечисленный scope; не создавать новые framework, Python-инфраструктуру, feature registry, harness или обязательные зависимости между независимыми библиотеками. Статус проверки: PASS / FAIL / NOT RUN с командой и причиной. Наличие теста не считается успешным прогоном.

Два независимых reviewer-субагента читают финальный diff, исходники и тесты сами:

- **A — корректность:** воспроизведение, контракты, ошибки, cancellation/concurrency, регрессии. Для каждого критерия — verdict и evidence.
- **B — интеграция и границы:** соответствие consumer-сценарию, совместимость, отсутствие обхода защит, лишних fallback/абстракций, достаточность документации и scope. Для каждого критерия — verdict и evidence.

Приёмка только при APPROVE обоих на одном и том же финальном code SHA и выполненных обязательных проверках. Замечания с нарушением критериев исправить, спорные — разрешить по коду/тесту; после изменений повторить затронутые проверки и review обоими. NOT RUN обязательной проверки или необоснованное отклонение замечания не превращать в Done. Не менять критерии, чтобы получить APPROVE. Краткие результаты, оставшиеся неблокирующие замечания и фактические команды записать в этот документ; отдельная система отчётности не нужна.

## Результат для потребителя

После двух APPROVE выполнить существующий release workflow из Makefile репозитория. Для обратно совместимого исправления — `make release-patch`, если эта цель поддерживается текущей инфраструктурой; для несовместимого изменения использовать предусмотренный репозиторием release path и явно описать break. Не обходить release checks ручной публикацией тегов. Несколько принятых задач одной библиотеки допустимо выпустить одним релизом.

Проверить remote основную ветку на наличие fix, удалённые теги всех затронутых модулей, соответствие tag исходному коду, успешность необходимых CI jobs на правильном SHA и доступность модулей через `GOWORK=off go mod download -json` без local replace. Проверить module sums/origin и consumer fixture на опубликованных версиях. Релизный commit может отличаться только ожидаемыми version metadata; иной code diff требует повторной приёмки. Записать фактические module/version/source SHA/tag/CI evidence. До этой проверки статус «код принят, релиз не подтверждён», а зависимая очистка Kosmify заблокирована.

## Goal для запуска в корне репозитория

```text
Выполни задачу .cursor/tasks/post-review-gemini-native-schema.md полностью в её границах.
Сначала прочитай задачу и локальные инструкции, проверь актуальный код и зависимости.
Реализуй минимальное исправление, AAA-регрессии и необходимую документацию.
Не меняй чужие изменения, prompt content или бизнес-политику вне scope.
Запусти предусмотренные задачей проверки и существующие проверки репозитория.
Создай двух независимых reviewer-субагентов A и B с ролями из задачи.
Каждый должен самостоятельно прочитать код и проверить условия приёмки.
Исправь подтверждённые замечания; добейся APPROVE обоих на одном final code SHA.
Обязательный NOT RUN или неподтверждённый dependency release означает, что задача не завершена.
Выпусти через существующий make release path и проверь remote source, tags, CI и скачиваемые модули по разделу «Результат для потребителя».
Не объявляй опубликованным код по одному наличию локального тега.
Запиши в задачу фактические SHA, команды/результаты, verdict обоих reviewers и итоговый статус.
Если есть блокер, опиши его точно; не закрывай задачу формальным PASS.
```

## Фактическое выполнение 2026-10-11

Baseline: чистый рабочий каталог; local HEAD и remote main
`9dc2516ec5afe2624c442791293baefbef017cf6`. Consumer Kosmify HEAD
`2de8c9204f6108faaf06e2f329e0035e088c4003`, pinned prompty v0.16.0.
Опубликованный prompty/adapter/gemini v0.16.2: source/tag
`b7de0a032ed1519f94daa3ce1766c81b4b0faea6`; native option отсутствовал.
SDK genai v1.72.0 скачан через `GOWORK=off go mod download -json`:
origin `65c31e349c142e2542222d7b024dc50bbb173aec`, tag v1.72.0,
sum `h1:sn3V1cHkHQhMcjtUrO2y1r54jprbFjmeokBU1IKfpZk=`.
Проверены локальные general/Go инструкции, существующие preflight/schema core,
release.sh и официальные GenerateContent/structured-output контракты (ссылки и
точный subset в adapter/gemini/README.md).

Реализация: публичный opt-in WithNativeJSONSchema, сохранён default typed mode;
узкий admission без изменения схемы; output/tool JSON schema transport; точные
числа восстанавливаются после float64 normalization SDK в request-local hook.
Caller client/request hook сохраняется; original-schema validation остаётся у
checked core. Все references отклоняются целиком (более узкий subset), потому
resolve/cycle transport здесь не обещан. Core компилирует original schema раньше
mapper: malformed/missing/external refs могут дать SchemaCompilationError вместо
SchemaFidelityError. Это существующий ранний guard, не fallback/dispatch; negative
fixtures проверяют calls=0 также для этих ошибок. Прямой native admission даёт
SchemaFidelityError с точным pointer. Core и его порядок проверок не изменены.

Проверки (GOWORK=off; GOCACHE=/tmp/prompty-native-cache,
GOLANGCI_LINT_CACHE=/tmp/prompty-native-lint — default cache вне sandbox):

- PASS: `(cd adapter/gemini && go test -count=1 ./...)`.
- PASS: `(cd adapter/gemini && go vet ./...)`.
- PASS: `(cd adapter/gemini && go test -race -count=1 ./...)`.
- PASS: `make lint`, `make test`, `make test-integration`, `make test-e2e`;
  после final hook regression повторены (полные результаты уточняются ниже).
- PASS: актуальные consumer fixtures
  `GOWORK=off go test -count=1 -run '^TestCheckedJSONNativeSchema' ./internal/infrastructure/ai/gemini`
  в `/tmp/prompty-native-kosmify`, архив Kosmify указанного HEAD: только в этой
  временной копии удалены checkedJSONAdapter.MapSchema/Translate, включён публичный
  WithNativeJSONSchema; Execute с host budget/telemetry оставлен. Для локального
  прогона root и adapter/gemini replaced на текущий source. Consumer repository
  не менялся. Перед завершением требуется тот же прогон на release без replace.
- PASS: A независимо выполнил adapter tests/vet/focused race/lint, checked negative
  probes и реальные consumer fixtures; B независимо adapter tests/vet и изучение
  contracts/scope. Final SHA/verdict фиксируются ниже после immutable review.

Подтверждённое замечание review B: сохранить client-level ExtrasRequestProvider,
который SDK не наследует при установке request hook. Исправлено через публичный
ClientConfig; фактический HTTP fixture подтверждает один вызов hook и marker.
Отдельных provider framework, dependencies, business policies, prompt content,
branches и изменений consumer repository не добавлено.

На момент source commit: код проверен; финальные SHA/verdict/release/CI/module
verification будут записаны ниже по фактическим результатам. До подтверждения
публикации статус «код принят, релиз не подтверждён», очистка host shim заблокирована.
Пользователь разрешил один commit задачи и отдельный metadata commit, создаваемый
штатным make release workflow. Итог публикации дописывается в этот документ без
дополнительного ручного commit.
