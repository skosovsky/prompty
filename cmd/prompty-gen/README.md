# prompty-gen

prompty-gen — генератор Go-кода для prompty-манифестов в стиле SQLC. Генерирует контракты (типы Input/Output) и методы Render для рендеринга промптов. **Выполнение (Execute) — ответственность runtime: вы компонуете Render + `prompty.Invoker.Execute` по своему усмотрению.**

## Философия

- **SRP:** генератор = контракт + Render; runtime = композиция + Execute.
- Генератор не внедряет LLM-клиент и не генерирует Execute — только типы и Render.
- Исполнение промптов: `plan, _ := prompts.Xxx.Render(ctx, input)` → `exec, _ := plan.Execute(ctx)` → `invoker.Execute(ctx, exec)`.

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
| **consts** | Только `PromptID` const и `AllPromptIDs()`. Манифесты должны быть v2.0 (`messages` + `inputs`). Вывод: `<package>_consts_gen.go`.                            |
| **types**  | Полная модель: shared-файл (`PromptCatalog`, `NewPromptCatalog`, `validate`) + per-manifest файлы (Input/Output, per-prompt type с Render/RequiredTools/ID). |

**Примечание:** Режимы `lite` и `full` удалены (breaking change).

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

- **Shared** `<package>_shared_gen.go`: `type PromptID`, `var validate`, `type PromptCatalog` (интерфейс), `func NewPromptCatalog(r prompty.DescribingRegistry) PromptCatalog`, `func AllPromptIDs() []PromptID`.
- **Per-manifest** `<id>_gen.go`: `const Xxx PromptID`, типы Input/Output, `type XxxPrompt struct`, методы `Render`, `RequiredTools()`, `ID()`.

Render выполняет: validate input → bind plan input → `registry.Plan(ctx, id, planInput)` и возвращает `*prompty.RenderPlan`. Для манифестов с полями в `inputs` bind — `PlanInputFrom(input)`; для пустого input (без `properties`) — `prompty.RegistryPlanInput{}`. `RequiredTools()` возвращает литерал из `required_tools` манифеста.

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
- **default:** применяется для optional pointer-полей в сгенерированном `Render(...)` до валидации и вызова `registry.Plan(...)`.
- Supported defaults: `string`, `integer`, `number`, `boolean` (optional поля).
- Defaults для неподдерживаемых типов (например `array`, вложенный `object`) приводят к fail-fast ошибке генерации.
- `minItems` / `maxItems` → validate-теги `min` / `max` для длины массива.

### Template binding (`prompt` vs `json`)

- Генератор всегда эмитит `prompt:"<field>"` для полей, участвующих в шаблонизации (ключи `.Input` в манифесте).
- Теги `json` (включая `omitempty`) нужны только для transport/API; на рендеринг шаблона они **не влияют**.
- `Render*` с непустым input schema: `prompty.PlanInputFrom(input)` → `registry.Plan(ctx, id, planInput)`; nil-указатели и пустые слайсы сохраняют ключи в `.Input`.
- Пустой manifest input (без `properties` / без payload): `prompty.RegistryPlanInput{}`, не `PlanInputFrom` и не `nil`.

## Tutorial: композиция Render + Execute

```go
reg, _ := fileregistry.New("./prompts", fileregistry.WithParser(yaml.New()))
catalog := NewPromptCatalog(reg)
var invoker prompty.Invoker // e.g. adapter.NewClient(openaiAdapter)

// 1. RenderPlan промпта
plan, err := catalog.RenderSupportAgent(ctx, SupportAgentInput{
	UserQuery: "Where is my order?",
})
if err != nil {
	return err
}

plan, err = plan.WithLate(struct {
	AllowedTools []string `prompt:"allowed_tools"`
}{AllowedTools: []string{"get_order_status"}})
if err != nil {
	return err
}
exec, err := plan.Execute(ctx)
if err != nil {
	return err
}

// 2. Выполнение — на ваше усмотрение (adapter.NewClient(...), middleware, streaming и т.д.)
resp, err := invoker.Execute(ctx, exec)
```

### Full v2 flow checklist

1. Генерация: `prompty-gen generate` -> `PromptCatalog` + typed `RenderXxx`.
2. Runtime рендер: `catalog.RenderXxx(...)` -> `*prompty.RenderPlan`.
3. Композиция (опционально): декларативные `imports`/`layers` в манифесте; late — `WithLate` / `WithLateInput`; `WithResponseFormatDefinition`.
4. Выполнение плана: `plan.Execute(ctx)` -> `*prompty.PromptExecution`.
5. Вызов модели: `invoker.Execute(ctx, exec)`.

Для prewarm кэша registry по всем ID:

```go
for _, id := range AllPromptIDs() {
    _, _ = reg.Plan(ctx, string(id), prompty.RegistryPlanInput{})
}
```

### Композиция нескольких планов перед Execute

Метаданные без рендера (если registry реализует `prompty.PromptDescriber`):

```go
desc, err := catalog.Descriptor(ctx, prompts.SupportAgent)
```

Композиция слоёв — в YAML/JSON манифесте (`imports`, `layers`, `import_ref`). Runtime capabilities для `condition.match` передавайте через `prompty.PlanInputWithCapabilities`:

```go
input, _ := prompty.PlanInputFrom(MainAgentInput{Query: "hi"})
input = prompty.PlanInputWithCapabilities(input, map[string]any{
	"capabilities": map[string]any{"workspace_enabled": true},
})
plan, _ := reg.Plan(ctx, "main_agent", input)
exec, _ := plan.Execute(ctx)
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

## Зависимости целевого проекта

- `github.com/skosovsky/prompty`
- `github.com/go-playground/validator/v10` (для types mode)

```bash
go get github.com/skosovsky/prompty
go get github.com/go-playground/validator/v10
```

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
cd cmd/prompty-gen/gen && go test -run TestGenerate_GoldenCompare -- -golden=../testdata
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

## External DoD validation (kosmify-prompts)

**Manual validation step** — cannot be run in this repo; perform in the consuming project after local tests pass.

After changing prompty or prompty-gen (e.g. YAML normalization in task17-1), run external validation in a consuming project (e.g. kosmify-prompts):

1. `go install ./cmd/prompty-gen` (from prompty repo)
2. `make generate` (in kosmify-prompts)
3. **DoD check:** Generated Input structs (e.g. `PromptsInternalRouterInput`) must contain expected fields (`CurrentDoctorTime`, `Timezone`, `ChatHistory`, etc.), not be empty — это проверяет корректный разбор contract-style `inputs.<field>` в типы.
