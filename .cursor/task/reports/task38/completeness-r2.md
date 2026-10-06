# Task38 — независимая приёмка полноты, round 2

**Не принято. Полнота: 94,44% (17 / 18 обязательных пунктов).**

Проверен frozen snapshot `/tmp/prompty-task38-snapshot-02`, база `70fc301`. SHA-256 manifest: `66b0198f32730a1147dd84e5b30e8a15098c4b523aa3828e0371f720d7a582b6`. Проверены hash самого manifest и `shasum -a 256 -c` всех перечисленных файлов — все OK. Implementation/snapshot не изменялись. Для запуска проверок и probes snapshot скопирован в `/tmp/prompty-task38-audit-r2`; отдельный emitted consumer — `/tmp/task38-probe-r2`. Отчёт другого аудитора round 2 не читался. В реализации аудитор не участвовал.

Знаменатель тот же, что в round 1: восемь обязательных пронумерованных требований, девять acceptance criteria, обязательный closeout README. Partial не засчитывается. Предыдущие четыре findings устранены; новый C7 defect не позволяет выдать 100%.

## Матрица

| ID | Requirement | Code / проверка | Status |
| --- | --- | --- | --- |
| C1 | Dynamic JSON сохраняет integer/decimal/exponent, float только по caller-selected type | `JSONDocumentAsMap`/runtime/recipe/manifest/compiled metadata/structured decode с UseNumber; public `TestRuntimeDynamicNumbersRemainExact`, `TestStructuredDynamicOutputAndToolDecodePreserveNumbers`; YAML exact parser | complete |
| C2 | Точное structural equality, equivalent numeric spellings, malformed≠malformed | `internal/exactjson.Equal`, точные symbolic exponent/decimal coefficient, tool-contract и composition wrappers; `TestEqual` с key order, arrays, большие signed ints, decimals, malformed | complete |
| C3 | Explicit provider priority/error, fallback только без provider, ExtractSchema error signature | `schema.go:ExtractSchema/schemaFromProvider`, compile provider with dialect; обновлены tool/output callers; `TestExplicitMalformedSchemaNeverFallsBack` | complete |
| C4 | Reflection encoding/json supported subset, string option, omission/null/embedded, recursion/custom-codec reject | `schema_reflect.go`, quoted json.Number codec, invalid tag names rejected; `TestReflectionMatchesJSONPresenceAndQuotedScalars`, `TestReflectionQuotedJSONNumberMatchesEncoding`, `TestReflectionRejectsUnsupportedJSONTagName`, recursive/embedded existing tests | complete |
| C5 | Non-JSON YAML/JSON schema возвращает error, contract не исчезает | `parser/yaml/exact.go`, `rawToSchemaDefinition` error return, `DecodeInputs`; `TestSchemaNumbersExactAndNonJSONRejected` output/input/tool nan/inf; overflow numeric tests | complete |
| C6 | Composition constraints preserved/rejected, no override/solver, type/late consistent, docs | `inputSchemaMerge.add/validateNamespaces`, whole-property equality, required union, schema engine compilation; `manifest/schema_fidelity_test.go` nested/enum/maxLength/const/required; docs/data-contract and closeout | complete |
| C7 | Codegen uses source validation semantics; no extra restrictions; raw/typed presence explicit; values do not change along successful path | Source schema retained by `JSONSchema`/`ValidateSchema`/raw decoder, tags removed, json.Number exact, property outer pointers preserve empty/null. Однако nullable **array items** превращают null в zero, а source-valid open-object input теряет unknown presence и проваливает typed validation. Подтверждения R2-F1/R2-F2 ниже | partial |
| C8 | Schema validation и caller domain Validate — разные stages; no generated domain restrictions | Generated `ValidateSchema`, caller `Validate` отдельно в late/domain structured pipeline; no generated go-validator tags | complete |
| A1 | Runtime checkpoint/overlay/late большие signed numbers/nested/fractions через public API/template | `TestRuntimeDynamicNumbersRemainExact`: BindRuntime/Execute, checkpoint input, nested late, exponent overlay | complete |
| A2 | Различные большие const не совпадают, equivalent spelling совпадает | exactjson suite и composition public tests; tool-contract wrapper тот же точный comparator | complete |
| A3 | Malformed provider отвергается output helper/NewTypedTool | `TestExplicitMalformedSchemaNeverFallsBack`, explicit errors до IO | complete |
| A4 | `count,string` DTO проходит inferred schema; recursive reject before IO | Quoted scalar/Number tests, recursive extraction error; custom codec inference explicit unsupported | complete |
| A5 | YAML nan/inf manifest errors | YAML exact tests проверяют три контрактные позиции | complete |
| A6 | maxLength100/maxLength3, nested/enum/required не ослабляют composed input | Conflicting contracts rejected, exact equal contracts union required и реально validate merged schema | complete |
| A7 | Compiled consumer 3.4/zero/false/empty accepted, 3.6/missing rejected | `gen_out/schema_fidelity_test.go:TestGeneratedSchemaValidationPresenceAndFractionalBounds`: executed exit0, exact decimal/big integer/nested/late cases | complete |
| A8 | Golden fixtures проверяют behavior recipes/late/checkpoints | gen_out recipe construction, BindLate, CheckpointJSON, DecodeRecipeCheckpoint; generated presence subprocess compiles emitted consumer, not only text comparison | complete |
| A9 | Draft/local refs/original validation/lowering retained; no implicit resolver | Inferred provider dialect preserved in response helper/plan/sync/stream; `TestExplicitOutputDialectSurvivesPublicHelpers/Streaming` executed; existing schema reference/dialect/compiler/lowering suite executed exit0; isolated explicit loader unchanged | complete |
| G1 | Closeout: contracts, deleted paths, migration, checks, limitations | `.cursor/task/reports/task38/closeout.md` + `checks-r2.md` document breaking API/DTO/schema changes, removed legacy paths, migration, local/full checks, typed subset, BYOT/host ownership and no live conformance | complete |

## Перепроверка round 1

- Nullable caller `*int=nil` с required nullable schema теперь успешно materializes null. Public r1 probe returns `<no value>` вместо `0`; committed test explicitly renders `null` and compares JSON original vs bound input, including nil slice and nil nested slice elements. `WithLateInput` использует тот же исправленный `bindValue`; typed validation/domain отдельны.
- Generated optional collection + minProperties1: `{"items":[]}` decode/ValidateSchema/remarshal теперь successful, output сохраняет items. Outer-pointer представление и nested decoder initializers покрыты compiled generated consumer subprocess.
- Пустой caller-created json.Number соответствует выбранной encoding/json semantics: present zero. Docs/contracts/closeout исправлены; это допустимый исходным ТЗ explicit typed presence contract, не необходимость навязать token-presence тип.
- Draft-07 provider: ExtractSchema → WithResponseFormatFromStruct → Execute сохраняет Dialect; ValidateSchemaJSON проходит. Public inferred synchronous и native stream tests также проходят.
- Empty/null history сохраняется отдельно в history schema values и проходит required nullable schema даже с нулевой flattened render history. History-only empty payload валиден; targeted public history test проходит.

## R2-F1: nullable array items не сохраняются в generated DTO

**Места:** `cmd/prompty-gen/gen/schema.go:234` отбрасывает nullable при mapping; `239–254` возвращают underlying primitive/struct element без nullable pointer; pointer logic есть только в `emitStruct` для named property. Поэтому оно не применяется к `items`.

Public generator probe `/tmp/task38-probe-r2/generate_nullable_items.go` вызывает `GenerateManifestTypes` и `GenerateSharedTypes`, сохраняет emitted Go и компилирует consumer в `/tmp/task38-probe-r2/generated`. Schema:

```json
{"type":"object","properties":{"items":{"type":"array","items":{"type":["integer","null"]}}},"minProperties":1}
```

`json.Unmarshal({"items":[null]}, &ArrayProbeInput)` возвращает nil. `ValidateSchema()` тоже nil. `json.Marshal(input)` возвращает **{"items":[0]}**. Таким образом, успешный generated path заменяет null на число. Это не caller-selected DTO conversion: форму `[]json.Number` выбрал генератор. При nullable object items возможна аналогичная потеря null/presence; нужно исправить mapping или до эмиссии явно reject unsupported nullable child shape. Behavioral regression должен проверять arrays/nested arrays nullable primitives/objects и typed additionalProperties values, а не только named nullable properties.

Воспроизведение на отдельной копии:

```sh
GOWORK=/tmp/task38-probe-r2/go.work GOSUMDB=off GOCACHE=/tmp/prompty-go-cache go run /tmp/task38-probe-r2/generate_nullable_items.go
GOWORK=/tmp/task38-probe-r2/go.work GOSUMDB=off GOCACHE=/tmp/prompty-go-cache go test -v /tmp/task38-probe-r2/generated/*.go
```

`TestNullableArrayItems` падает: `null was changed: {"items":[0]}`; это semantic failure, emitted consumer компилируется.

## R2-F2: documented unknown-field projection всё ещё сужает source schema

На том же generated consumer source допускает extra properties (additionalProperties отсутствует) и требует minProperties1. Input **{"extra":1}** source-valid; generated UnmarshalJSON возвращает nil, затем typed marshal **{}**, ValidateSchema получает `ErrSchemaMismatch: minProperties got 0, want 1`. `TestOpenSchemaPresence` воспроизводимо падает. Recipe constructor использует ValidateSchema, поэтому source-valid input не проходит полный generated recipe path.

Места: `cmd/prompty-gen/gen/schema.go:emitStruct` хранит только declared properties; `emitJSONDecoder` декодирует в этот struct и теряет unknown fields; `ValidateSchema` снова проверяет marshal. Raw-before-decode validation не устраняет более узкую допустимую область typed DTO. Docs/closeout называют это encoding/json projection limitation, но C7 прямо запрещает создавать ограничения, отсутствующие в исходной schema; описание limitation не исполняет контракт. Не требуется универсальный application DTO: допустим заранее диагностируемый generation reject unsupported schema/constraint projection либо typed representation, сохраняющая необходимый контракт. Добавить regression на source-valid unknown presence и root constraints.

## Выполненные проверки и границы

- Integrity manifest hash + file hashes: OK.
- Targeted root fidelity/quoted/nullable/history/public dialect sync+stream tests, internal/exactjson, parser/yaml, manifest composition: exit0.
- Existing root schema compiler/reference/dialect/lowering tests: exit0.
- Generator method collision и compiled presence subprocess: exit0.
- Compiled gen_out fractional bounds/presence/recipe/late/checkpoints test: exit0.
- R1 probes на отдельной копии: исправленные semantics подтверждены.
- Новые emitted consumer probes: compilation successful, две semantic assertions failed по R2-F1/F2.
- Проверены `/tmp/task38-lint-r2.log` и `/tmp/task38-test-r2.log`: all modules/zero lint issues, race/testdata consumer проходят. Не запускала генерацию/проверки с возможными записями в frozen snapshot; запускала отдельную копию.

BYOT и границы библиотеки сохранены, existing schema engine используется, compatibility fallback/tag paths удалены. Root/parser/manifest/generator/docs/fixtures обновлены. Provider SDK fidelity оставлен task42, host-owned agent loop/policies/IO не добавлялись. Полнота здесь не является обещанием отсутствия любых ошибок; для этого отдельный независимый аудитор.

**Нужен новый раунд:** исправить R2-F1/R2-F2, добавить behavioral regressions, повторить required checks и freeze. Task38 commit этим отчётом не принят.
