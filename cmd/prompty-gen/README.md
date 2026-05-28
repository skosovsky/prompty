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

- **Shared** `<package>_shared_gen.go`: `type PromptID`, `var validate`, `type PromptCatalog` (интерфейс), `func NewPromptCatalog(r prompty.Registry) PromptCatalog`, `func AllPromptIDs() []PromptID`.
- **Per-manifest** `<id>_gen.go`: `const Xxx PromptID`, типы Input/Output, `type XxxPrompt struct`, методы `Render`, `RequiredTools()`, `ID()`.

Render выполняет: validate input → `registry.Plan(ctx, id, input)` и возвращает `*prompty.RenderPlan`. `RequiredTools()` возвращает литерал из `required_tools` манифеста.

## Mapping JSON Schema → Go

- `object` с `properties` → именованный struct.
- `object` без `properties` / `additionalProperties` → `map[string]any`.
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

exec, err := plan.WithLateVariables(map[string]any{
	"allowed_tools": []string{"get_order_status"},
}).Execute(ctx)
if err != nil {
	return err
}

// 2. Выполнение — на ваше усмотрение (adapter.NewClient(...), middleware, streaming и т.д.)
resp, err := invoker.Execute(ctx, exec)
```

### Full v2 flow checklist

1. Генерация: `prompty-gen generate` -> `PromptCatalog` + typed `RenderXxx`.
2. Runtime рендер: `catalog.RenderXxx(...)` -> `*prompty.RenderPlan`.
3. Композиция (опционально): `WithLateVariables`, `ReplaceLayer`, `AppendToLayer`, `WithResponseFormat`.
4. Выполнение плана: `plan.Execute(ctx)` -> `*prompty.PromptExecution`.
5. Вызов модели: `invoker.Execute(ctx, exec)`.

Для prewarm кэша registry по всем ID:

```go
for _, id := range AllPromptIDs() {
    _, _ = reg.Plan(ctx, string(id), nil)
}
```

### Композиция нескольких планов перед Execute

Метаданные без рендера (если registry реализует `prompty.ManifestResolver`):

```go
desc, err := catalog.Descriptor(ctx, prompts.SupportAgent)
```

Скомпонуйте слои через `ReplaceLayer` или дополните хвост через `AppendToLayer`, затем выполните единый план:

```go
basePlan, _ := catalog.RenderSalesPersona(ctx, SalesPersonaInput{Tone: "formal"})
rulesPlan, _ := catalog.RenderClinicRules(ctx, ClinicRulesInput{})

composed, _ := basePlan.ReplaceLayer("rules", rulesPlan)
exec, _ := composed.Execute(ctx)
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
go test ./cmd/prompty-gen/gen -run TestGenerate_Golden -args -golden=./cmd/prompty-gen/testdata
```

Файлы `shared_gen.go.golden`, `support_agent_gen.go.golden`, `consts_gen.go.golden` будут перезаписаны. Без `-golden` тест `TestGenerate_Golden` пропускается; `TestGenerate_GoldenCompare` проверяет соответствие сгенерированного кода golden-файлам.

## External DoD validation (kosmify-prompts)

**Manual validation step** — cannot be run in this repo; perform in the consuming project after local tests pass.

After changing prompty or prompty-gen (e.g. YAML normalization in task17-1), run external validation in a consuming project (e.g. kosmify-prompts):

1. `go install ./cmd/prompty-gen` (from prompty repo)
2. `make generate` (in kosmify-prompts)
3. **DoD check:** Generated Input structs (e.g. `PromptsInternalRouterInput`) must contain expected fields (`CurrentDoctorTime`, `Timezone`, `ChatHistory`, etc.), not be empty — это проверяет корректный разбор contract-style `inputs.<field>` в типы.
