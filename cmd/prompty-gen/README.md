# prompty-gen

prompty-gen — генератор Go-кода для prompty-манифестов. Генерирует typed contracts, recipe/checkpoint API и index по prompt id. **Вызов модели остается ответственностью runtime: generated recipe материализует `prompty.PromptExecution`, а дальше вы передаете его в invoker, middleware или streaming path.**

## Философия

- **SRP:** генератор = typed prompt contract + recipe/checkpoint/index; runtime = registry + invoker.
- Генератор не внедряет LLM-клиент и не генерирует provider call — только typed API для создания и восстановления prompt execution.
- Исполнение промптов: `recipe, _ := catalog.NewXxxRecipe(ctx, input)` → `exec, _ := recipe.ExecuteWithContract(ctx, registry, contract)` → `invoker.Execute(ctx, exec)`.

## Установка

```bash
go install github.com/skosovsky/prompty/cmd/prompty-gen@latest
```

Или сборка из репозитория:

```bash
cd cmd/prompty-gen && go build -o prompty-gen .
```

## Конфигурация

Создайте файл `prompty.yaml` в корне проекта:

```yaml
version: "1"

packages:
  - name: prompts
    path: ./internal/prompts
    queries:
      - "prompts/*.yaml"
      - "prompts/*.json"
    package: prompts
    mode: types  # consts | types (по умолчанию types)
```

### Параметры пакета

| Параметр  | Описание                                                                      |
| --------- | ----------------------------------------------------------------------------- |
| `name`    | Имя пакета (используется если не задан `package`)                             |
| `path`    | Директория для сгенерированных `*_gen.go` файлов                              |
| `queries` | Glob-паттерны или пути к директориям с манифестами (`.yaml`, `.yml`, `.json`) |
| `package` | Имя Go-пакета в сгенерированном коде (по умолчанию = `name`)                  |
| `mode`    | `consts` или `types` (по умолчанию `types`). См. ниже.                        |

### Режимы

| Режим      | Описание                                                                                                                                                     |
| ---------- | ------------------------------------------------------------------------------------------------------------------------------------------------------------ |
| **consts** | Только `PromptID` const и `AllPromptIDs()`. Манифесты должны использовать формат `messages` + `inputs`. Вывод: `<package>_consts_gen.go`.                     |
| **types**  | Полная модель: shared-файл (`PromptCatalog`, `NewPromptCatalog`, `validate`, `PromptIndex`) + per-manifest файлы (Input/Output, typed compose context при `condition.match`, recipe/checkpoint API, RequiredTools/ID). |

**Примечание:** Режимы `lite` и `full` удалены.

## Использование

```bash
# Генерация (конфиг prompty.yaml в текущей директории)
prompty-gen generate

# Указать путь к конфигу
prompty-gen -config path/to/prompty.yaml generate

# Список найденных манифестов
prompty-gen list
```

## Что генерируется

### consts mode

Один файл `<package>_consts_gen.go`:

- `type PromptID string`
- `const ( SupportAgent PromptID = "support_agent" ... )`
- `func AllPromptIDs() []PromptID`

### types mode

- **Shared** `<package>_shared_gen.go`: `type PromptID`, `var validate`, `type Recipe` с `CheckpointJSON()`, `type PromptCatalog` (интерфейс), `PromptIndex` с static `Lookup(id)` entry metadata/required tools/descriptor, `NewRecipeFromJSON`, `DecodeRecipeCheckpoint`, `func NewPromptCatalog(r prompty.PromptCatalogRegistry) PromptCatalog`, `func AllPromptIDs() []PromptID`.
- **Per-manifest** `<id>_gen.go`: `const Xxx PromptID`, типы Input/Output, `type XxxPrompt struct`, typed `XxxRecipe`, `NewXxxRecipeFromCheckpoint`, `RequiredTools()`, `ID()`. Для prompt без compose conditions генерируется `NewRecipe`; для prompt с `condition.match` генерируются `XxxComposeContext`, `XxxRecipePayload` и `NewRecipeWithComposeContext`.

`NewRecipe` / `NewRecipeWithComposeContext` выполняют: defaults → validate input → fail-fast bind check → `registry.RecommendManifestDescriptor(ctx, id)` → typed recipe. `Checkpoint()` возвращает JSON-safe DTO, `CheckpointJSON()` сериализует DTO для generic `Recipe`, `NewXxxRecipeFromCheckpoint` повторно применяет defaults/validation/bind checks и восстанавливает typed recipe без ручного switch. `RequiredTools()` и `PromptIndex.Lookup(id)` возвращают generated literals без registry IO.

## Mapping JSON Schema → Go

- `object` с `properties` → именованный struct.
- `object` без `properties` требует `additionalProperties: { type: T }` (примитив или array) → `map[string]T`.
- `object` без `properties` и без typed `additionalProperties`, свойства без `type`, `oneOf`/`anyOf` union → ошибка codegen (strict, fail-closed).
- `additionalProperties: { type: string }` → `map[string]string`.
- Optional → `*T` для скаляров и object-with-properties; `T` (nil-check) для array/object-without-properties.
- Массивы структур: validate-тег `dive` для вложенной валидации.
- `minItems` / `maxItems` → validate `min` / `max` для длины среза.

### Семантика полей

- **required bool:** генерируется как `*bool` + `validate:"required"`. Presence проверяется через non-nil: `nil` = отсутствует (ошибка), `false` и `true` = валидны.
- **default:** применяется для optional pointer-полей в сгенерированном `NewRecipe(...)` до валидации и checkpoint creation.
- Supported defaults: `string`, `integer`, `number`, `boolean` (optional поля).
- Defaults для неподдерживаемых типов (например `array`, вложенный `object`) приводят к fail-fast ошибке генерации.
- `minItems` / `maxItems` → validate-теги `min` / `max` для длины массива.

### Template binding (`prompt` vs `json`)

- Генератор всегда эмитит `prompt:"<field>"` для полей, участвующих в шаблонизации (ключи `.Input` в манифесте).
- Теги `json` (включая `omitempty`) нужны только для transport/API; на рендеринг шаблона они **не влияют**.
- `NewRecipe*` с непустым input schema: `prompty.PlanInputFrom(input)` используется как fail-fast bind check; nil-указатели и пустые слайсы сохраняют ключи в `.Input` при execution.
- Пустой manifest input (без `properties` / без payload): generated recipe не вызывает `PlanInputFrom`.

## Tutorial: recipe + checkpoint + execute

```go
reg, _ := fileregistry.New("./prompts", fileregistry.WithParser(yaml.New()))
catalog := NewPromptCatalog(reg)
var invoker prompty.Invoker // e.g. adapter.NewClient(openaiAdapter)

// 1. Typed recipe промпта
recipe, err := catalog.NewSupportAgentRecipe(ctx, SupportAgentInput{
	UserQuery: "Where is my order?",
})
if err != nil {
	return err
}

checkpoint, err := recipe.Checkpoint()
if err != nil {
	return err
}
restored, err := NewSupportAgentRecipeFromCheckpoint(checkpoint)
if err != nil {
	return err
}
contract := prompty.ToolContractFunc(func(name string) bool {
	return runtimeHasTool(name)
})
exec, err := restored.ExecuteWithContract(ctx, reg, contract)
if err != nil {
	return err
}

// 2. Выполнение — на ваше усмотрение (adapter.NewClient(...), middleware, streaming и т.д.)
resp, err := invoker.Execute(ctx, exec)
```

### Full flow checklist

1. Генерация: `prompty-gen generate` -> `PromptCatalog` + typed recipes + `PromptIndex`.
2. Runtime handle: `catalog.NewXxxRecipe(...)`, `catalog.NewXxxRecipeWithComposeContext(...)` или `PromptIndex.NewRecipeFromJSON(...)` -> generated `Recipe`.
3. JSON decode: обычные prompts принимают JSON input; prompts с `condition.match` принимают generated `XxxRecipePayload{Input, Compose}` и требуют непустой `Compose`; unknown JSON fields отклоняются.
4. Checkpoint: `recipe.Checkpoint()` -> JSON DTO + error; restore через `NewXxxRecipeFromCheckpoint(...)` или `PromptIndex.DecodeRecipeCheckpoint(...)`.
5. Композиция: декларативные `imports`/`layers` в манифесте; runtime `condition.match` — через generated `XxxComposeContext`.
6. Late input: `recipe.BindLate(XxxLateInput{...})` для манифестов с late-полями.
7. Required tools: `recipe.ExecuteWithContract(ctx, reg, contract)` для preflight, когда runtime должен проверить доступные tools.
8. Выполнение без required-tool preflight: `recipe.Execute(ctx, reg)` -> `*prompty.PromptExecution`; основной runtime path должен использовать `ExecuteWithContract`.
9. Вызов модели: `invoker.Execute(ctx, exec)`.

Для static metadata/descriptor lookup по всем ID:

```go
index := NewPromptIndex(nil)
for _, id := range AllPromptIDs() {
	entry, ok := index.Lookup(id)
	if !ok {
		continue
	}
	_ = entry.Metadata()
	_ = entry.RequiredTools()
	_ = entry.Descriptor()
}
```

### Static metadata and composition

Метаданные без registry IO:

```go
entry, ok := catalog.Index().Lookup(prompts.SupportAgent)
if !ok {
	return errUnknownPrompt
}
metadata := entry.Metadata()
tools := entry.RequiredTools()
descriptor := entry.Descriptor()
```

Композиция слоёв — в YAML/JSON манифесте (`imports`, `layers`, `import_ref`). Runtime values для `condition.match` передавайте через generated typed compose context:

```go
recipe, _ := catalog.NewMainAgentRecipeWithComposeContext(
	ctx,
	MainAgentInput{Query: "hi"},
	NewMainAgentComposeContext(true),
)
exec, _ := recipe.ExecuteWithContract(ctx, reg, contract)
resp, err := invoker.Execute(ctx, exec)
```

## Интеграция в Makefile / Git Sync

```makefile
.PHONY: generate
generate:
	go run ./cmd/prompty-gen -config prompty.yaml generate

.PHONY: gen-check
gen-check: generate
	git diff --exit-code  # fail CI если сгенерированный код не закоммичен
```

В CI вызывайте `make gen-check` перед сборкой.

## Интеграция в CI

```yaml
# .github/workflows/ci.yml
- run: go install github.com/skosovsky/prompty/cmd/prompty-gen@latest
- run: prompty-gen generate
- run: git diff --exit-code  # проверка, что сгенерированный код закоммичен
```

## Обновление golden-файлов

При изменении генератора обновите эталонные файлы в `testdata/`:

```bash
cd cmd/prompty-gen/gen && go test -run TestGenerate_GoldenCompare -args -- -golden=../testdata
```

Golden files in `cmd/prompty-gen/testdata/`:

- `shared_gen.go.golden`
- `support_agent_gen.go.golden`
- `consts_gen.go.golden`
- `no_vars_gen.go.golden`
- `late_binding_agent_gen.go.golden`
- `late_required_agent_gen.go.golden`
- `composed_main_gen.go.golden`
- `composed_child_gen.go.golden`
- `composed_conditional_main_gen.go.golden`

Without `-golden`, `TestGenerate_GoldenCompare` verifies generated output against these files.
