# prompty-gen

prompty-gen — генератор Go-кода для prompty-манифестов в стиле SQLC. Генерирует контракты (типы Input/Output) и методы Render для рендеринга промптов. **Выполнение (Execute) — ответственность runtime: вы компонуете Render + `prompty.Execute` по своему усмотрению.**

## Философия

- **SRP:** генератор = контракт + Render; runtime = композиция + Execute.
- Генератор не внедряет LLM-клиент и не генерирует Execute — только типы и Render.
- Исполнение промптов: `exec, _ := prompts.RenderXxx(ctx, input)` → `prompty.Execute(ctx, client, exec)`.

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

| Параметр | Описание |
|----------|----------|
| `name` | Имя пакета (используется если не задан `package`) |
| `path` | Директория для сгенерированных `*_gen.go` файлов |
| `queries` | Glob-паттерны или пути к директориям с манифестами (`.yaml`, `.yml`, `.json`) |
| `package` | Имя Go-пакета в сгенерированном коде (по умолчанию = `name`) |
| `mode` | `consts` или `types` (по умолчанию `types`). См. ниже. |

### Режимы

| Режим | Описание |
|-------|----------|
| **consts** | Только `PromptID` const и `AllPromptIDs()`. Без зависимости от input_schema. Вывод: `<package>_consts_gen.go`. |
| **types** | Полная модель: shared-файл (Prompts, NewPrompts, validate) + per-manifest файлы (Input/Output, Render\<Name\>). |

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

- **Shared** `<package>_shared_gen.go`: `type PromptID`, `var validate`, `type Prompts`, `func NewPrompts(r prompty.Registry) *Prompts`, `func AllPromptIDs() []PromptID`.
- **Per-manifest** `<id>_gen.go`: `const Xxx PromptID`, типы Input/Output, `func (p *Prompts) RenderXxx(ctx, input) (*prompty.PromptExecution, error)`.

Render выполняет: validate input → GetTemplate → vars map → `tmpl.Format(ctx, vars)`. Без Execute и LLMClient.

## Mapping JSON Schema → Go

- `object` с `properties` → именованный struct.
- `object` без `properties` / `additionalProperties` → `map[string]any`.
- `additionalProperties: { type: string }` → `map[string]string`.
- Optional → `*T` для скаляров и object-with-properties; `T` (nil-check) для array/object-without-properties.
- Массивы структур: validate-тег `dive` для вложенной валидации.
- `minItems` / `maxItems` → validate `min` / `max` для длины среза.

### Семантика полей

- **required bool:** генерируется как `*bool` + `validate:"required"`. Presence проверяется через non-nil: `nil` = отсутствует (ошибка), `false` и `true` = валидны.
- **default:** применяется только к optional полям. Для required игнорируется.
- `minItems` / `maxItems` → validate-теги `min` / `max` для длины массива.

## Tutorial: композиция Render + Execute

```go
reg, _ := fileregistry.New("./prompts", fileregistry.WithParser(yaml.New()))
prompts := NewPrompts(reg)

// 1. Render промпта
exec, err := prompts.RenderSupportAgent(ctx, SupportAgentInput{
    UserQuery: "Where is my order?",
    BotName:   ptr("SupportBot"),
})
if err != nil {
    return err
}

// 2. Выполнение — на ваше усмотрение (prompty.Execute, streaming, etc.)
resp, err := prompty.Execute(ctx, client, exec)
```

Для prewarm кэша registry по всем ID:

```go
for _, id := range AllPromptIDs() {
    _, _ = reg.GetTemplate(ctx, string(id))
}
```

### Композиция нескольких Render* перед Execute

Склейте сообщения из нескольких промптов и отправьте в `prompty.Execute`:

```go
exec1, _ := prompts.RenderSalesPersona(ctx, SalesPersonaInput{Tone: "formal"})
exec2, _ := prompts.RenderClinicRules(ctx, ClinicRulesInput{})
combined := &prompty.PromptExecution{
    Model:    exec1.Model,
    Messages: append(exec1.Messages, exec2.Messages...),
}
resp, err := prompty.Execute(ctx, client, combined)
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
3. **DoD check:** Generated Input structs (e.g. `PromptsInternalRouterInput`) must contain expected fields (`CurrentDoctorTime`, `Timezone`, `ChatHistory`, etc.), not be empty — this validates YAML `input_schema.properties` are correctly normalized from `map[any]any` to `map[string]any`.
