# Task38 — независимая приёмка полноты, раунд 1

**Результат: не принято. Полнота 83,33% (15 / 18 обязательных пунктов).**

Проверен только frozen snapshot `/tmp/prompty-task38-snapshot-01`, база `70fc301`. SHA-256 файла manifest `/tmp/prompty-task38-snapshot-01.sha256`: `e75841b2084db5b5ec1dd0e6bc6d67cc35be71835585758a6cdfacbd77f1e884`; подтверждён командой `shasum -a 256`. Аудитор в реализации не участвовал. Рабочий код и snapshot не изменялись; probes созданны отдельно в `/tmp/task38-probe-r1`.

## Метод подсчёта

Считаются 8 пронумерованных требований контракта, 9 acceptance criteria и обязательный closeout из общего README. `partial` и `missing` дают 0. Общие архитектурные правила README проверены дополнительно ниже и не используются для искусственного увеличения знаменателя. Несколько дефектов одного пункта не увеличивают количество невыполненных пунктов. Контрактные требования и acceptance criteria перечислены отдельно, как в исходном ТЗ.

## Матрица требований

| ID | Обязательное требование | Реализация / доказательства | Статус |
| --- | --- | --- | --- |
| C1 | Dynamic JSON integer/decimal/exponent сохраняются точно; float только при caller-selected типе | `jsondoc.go:35`, `runtime.go:jsonDocumentObject`, `recipe.go:strictUnmarshalJSON`, `manifest/json.go`, `internal/compiled/wire.go`, structured output decoder с UseNumber; `schema_fidelity_test.go:TestRuntimeDynamicNumbersRemainExact`, `TestStructuredDynamicOutputAndToolDecodePreserveNumbers`, `parser/yaml/exact_test.go` | complete |
| C2 | Точное структурное равенство, эквивалентные записи числа, различение больших integers, malformed не равен | `internal/exactjson/equal.go`, `tool_contract.go:jsonDocumentsEqual`, `manifest/compose.go:equalSchemaValues`; `internal/exactjson/equal_test.go:TestEqual` покрывает знаки, exponent, порядок объектов/массивов и malformed | complete |
| C3 | Явная schema имеет приоритет; ошибка provider возвращается вместо reflection fallback; новый ExtractSchema error contract | `schema.go:ExtractSchema/schemaFromProvider`, callers в `tool_binding.go`, `render_plan.go`, structured helpers; `schema_fidelity_test.go:TestExplicitMalformedSchemaNeverFallsBack` | complete |
| C4 | Reflection соответствует encoding/json в поддерживаемом подмножестве: string, recursion reject, codecs reject, omission/null/embedded | `schema_reflect.go:schemaForType/collectStructFields/quotedJSONSchema`; `schema_fidelity_test.go:TestReflectionMatchesJSONPresenceAndQuotedScalars`, `schema_test.go:TestExtractSchema_RecursiveStructTerminates`, existing embedded tests. Inferred schema принимает marshal-документ с nullable полями; проблема дальнейшего binder отнесена к C7 | complete |
| C5 | YAML/JSON non-JSON schema values возвращают manifest error, контракт не исчезает | `parser/yaml/exact.go`, `parser/yaml/yaml.go:rawToSchemaDefinition` с error return; `manifest/inputs.go` нормализует через exact JSON; `parser/yaml/exact_test.go` проверяет output/input/tool `.nan` и `.inf` | complete |
| C6 | Composition сохраняет все ограничения или явно отвергает merge; нет local override/собственного solver; type/late согласованы; документировано | `manifest/compose.go:MergeInputSchemas/inputSchemaMerge.add/validateNamespaces`, whole-property exact equality, required union, engine compilation; `manifest/schema_fidelity_test.go` nested/enum/maxLength/const/required; `docs/schema-data-contract.md:9` | complete |
| C7 | Generated DTO/source schema имеют одинаковую validation semantics; no hidden narrowing; raw presence/typed presence явны; исходные значения сохраняются по пути | `cmd/prompty-gen/gen/schema.go:emitSchemaMethods` сохраняет schema и проверяет raw JSON; числовые типы json.Number и отсутствие validator tags исправляют основные кейсы. Но optional empty array теряет presence и делает валидный source JSON невалидным typed value; nullable input меняется binder; documented empty numeric token ошибочно принимается. Подтверждения F1–F3 ниже | partial |
| C8 | Schema validation отдельно от caller domain Validate; generator не добавляет domain restrictions | Generated метод `ValidateSchema`, а не `Validate`; `late_validation.go:validateLateInputStruct`, `structured_output.go:validateStructuredValue`, `schema_value.go`; прежний generated validate-tags path удалён | complete |
| A1 | Public runtime checkpoint/overlay/late сохраняет большие signed числа, nested JSON и дроби, проверено шаблоном | `schema_fidelity_test.go:TestRuntimeDynamicNumbersRemainExact` использует BindRuntime и Execute, checkpoint input + nested late + exponent overlay | complete |
| A2 | const 9007199254740992/9007199254740993 не совпадают; одна запись числа может совпасть | `internal/exactjson/equal_test.go`; `manifest/schema_fidelity_test.go:TestCompositionRejectsLossOfFieldConstraints/TestCompositionExactNumericEqualityAndRequiredUnion`; public paths используют тот же comparator | complete |
| A3 | Malformed SchemaProvider отвергается output helper и NewTypedTool | `TestExplicitMalformedSchemaNeverFallsBack`, ошибки до обращения к invoker | complete |
| A4 | Quoted scalar DTO проходит свою schema; recursive DTO reject до IO | `TestReflectionMatchesJSONPresenceAndQuotedScalars`, recursive rejection test; extraction вызывается до dispatch | complete |
| A5 | YAML .nan/.inf schema возвращает manifest error | `TestSchemaNumbersExactAndNonJSONRejected`: response_format/inputs/tools | complete |
| A6 | Parent/child различающиеся maxLength/nested/enum/required не ослабляют общий input | `TestCompositionRejectsLossOfFieldConstraints` отвергает differing constraints; `TestCompositionExactNumericEqualityAndRequiredUnion` проверяет union required | complete |
| A7 | Compiled generated consumer принимает 3.4/zero/false/empty, отвергает 3.6/missing | `cmd/prompty-gen/testdata/gen_out/schema_fidelity_test.go:TestGeneratedSchemaValidationPresenceAndFractionalBounds`; реально компилируется и запускается, включая exact decimals, большие ints, nested constraints и late validation | complete |
| A8 | Generated golden fixtures имеют behavioral tests recipes/late/checkpoints | `schema_fidelity_test.go` в gen_out строит recipe, BindLate, CheckpointJSON, DecodeRecipeCheckpoint; обновлены existing gen_out tests. Makefile явно включает ignored-by-Go testdata/gen_out | complete |
| A9 | Draft selection/local refs/original validation/lowering не ослаблены; no implicit resolver | Engine/local-ref/lowering пути сохранены. Но вывод schema из valid draft-07 SchemaProvider теряет dialect в output helpers, затем отвергается validator. F4 ниже; existing draft tests с вручную заданным SchemaDefinition.Dialect это не покрывают | partial |
| G1 | Closeout: изменённые контракты, удалённые paths, migration, проверки, ограничения | `docs/schema-data-contract.md` содержит часть migration/limitations, но closeout отсутствует; единственный файл reports/task38 в snapshot — contracts.md, он прямо заявляет незавершённую реализацию/приёмку. Нет сводки выполненных проверок/полного удаления legacy paths/границ проверок | missing |

## Подтверждённые незавершённые части

### F1. Nullable typed input меняется на zero до проверки original input schema

Места: `template_binder.go:164–178` (nil pointer → reflect.Zero), `template_binder.go:199–217` (nil slice → empty; nil struct elements → zero objects), `render_plan.go:validatePlanSchema` проверяет уже изменённые bindings. Генератор допускает nullable pointers: `cmd/prompty-gen/gen/schema.go:412–429`.

Public probe `/tmp/task38-probe-r1/main.go`: caller DTO `struct { Value *int json:"value" prompt:"value" }`, Value=nil, manifest schema requires `value` with type `["integer","null"]`; `NewRenderPlanFromStruct` и `Execute` успешны, шаблон `{{.Input.value}}` возвращает **"0"**. Исходный JSON payload имеет **null**. Это не caller-selected числовая конверсия и не нормализация записи одного числа. Нельзя закрыть C7 обещанием nullable schema, когда materialization подменяет значение. Нужна согласованная projection/presence/null семантика с regressions для early/late, slices/nested pointers и generated nullable DTO.

### F2. Generated omission добавляет ограничения, которых source schema нет

Места: `cmd/prompty-gen/gen/schema.go:420–421`, `435–444`, `477–498`; generated slices без pointer + `omitempty`; `ValidateSchema()` проверяет повторный marshal с потерянным property.

В отдельной копии generator использован публичный `GenerateManifestTypes` + `GenerateSharedTypes`, результат скомпилирован в `/tmp/task38-probe-r1/generated`. Source schema:

```json
{"type":"object","properties":{"items":{"type":"array","items":{"type":"integer"}}},"minProperties":1}
```

Input `{"items":[]}` валиден. Generated UnmarshalJSON возвращает nil. Далее json.Marshal(value) возвращает `{}`, и generated ValidateSchema возвращает `ErrSchemaMismatch`, `minProperties: got 0, want 1`. `New...Recipe` вызывает этот метод, поэтому consumer не принимает валидный input. Это запрещённое C7 сужение source-контракта. Документирование encoding/json projection не исполняет требование. Нужны representation/presence, сохраняющие explicit empty/null, либо явный generation reject неподдерживаемого контракта до появления consumer; tests на root presence constraints.

### F3. Documented typed numeric zero contract не реализован

`docs/schema-data-contract.md:7` и reports/contracts.md говорят, что пустой token caller-created required numeric field invalid. Но `schema_value.go:27` использует json.Marshal, который кодирует `json.Number("")` как `0`; generated ValidateSchema не проверяет token отдельно (`cmd/prompty-gen/gen/schema.go:477`).

Public probe `ValidateSchemaValue(&SchemaDefinition{Schema:{"type":"number"}}, json.Number(""))` возвращает **nil**. Таким образом, документированное различие unset token vs explicit "0" отсутствует. Нужно выбрать и реализовать явную typed presence семантику, согласованную с source schema, без универсального application DTO; тесты должны проверять documented contract, включая pointers/nested fields.

### F4. Draft-07 provider output теряет dialect downstream

Места: `render_plan.go:188` и `structured_output.go:117–119` создают SchemaDefinition только с Schema; `stream_structured_output.go:142–158` наследует пустой Dialect и вкладывает item schema внутрь anyOf. `schema.go:schemaFromProvider` уже успешно компилирует provider в объявленном dialect; `tool_binding.go:46/54` dialect сохраняет.

Public probe с provider `$schema:"http://json-schema.org/draft-07/schema#"`: ExtractSchema успешен; WithResponseFormatFromStruct успешен; Execute успешен, но execution.ResponseFormat.Dialect пуст. ValidateSchemaJSON того же valid draft-07 output возвращает `SchemaCompilationError` (declared schema dialect differs from requested dialect). Structured helper auto-inference аналогично компилирует с default2020 и откажет до invoker. Это противоречит docs/schema-data-contract.md:5 и A9. Нужны dialect propagation и public output/stream regressions с inferred provider, не только manually attached SchemaDefinition.

## Проверки и границы

Лично запускались targeted tests exactjson, parser/yaml, manifest и root fidelity tests — exit 0. Лично запускался compiled gen_out behavioral test — exit 0. Дополнительный generated presence probe воспроизводимо падает по semantic assertion, не compilation. Public null/dialect/numeric probes завершились exit 0 с указанными выше результатами. Для probes отдельный go.work включал только frozen modules; GOCACHE=/tmp/prompty-go-cache. GOSUMDB=off использован для локальной probe-сборки, поскольку sandbox ограничивает запись системного module cache; workspace dependency graph не менялся.

По сообщению реализатора до freeze `make lint` и `make test` (все модули с race) завершились exit 0. Аудитор не подменяет полноту зелёными проверками: эти probes отсутствуют в основном наборе.

BYOT/границы соблюдены: caller-owned DTO, отдельный domain Validate, существующий JSON Schema engine, без agent loop, retry/storage/model catalogs и implicit network/filesystem resolvers. Clear break реализован в ExtractSchema и generated validation; silent fallback provider и generated validator tags удалены. Docs/root/generator README и fixtures обновлены. Ограничения recursive/custom-codec inference диагностируются и соответствуют разрешённому ТЗ варианту. Но описания F1–F4 исправлениями не являются.

**До следующей приёмки:** устранить F1–F4 с behavioral regressions, подготовить обязательный closeout, повторить общие проверки и заморозить новый snapshot. Этот раунд не даёт разрешения на task38 commit.
