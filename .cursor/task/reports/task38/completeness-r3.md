# Task38 — независимая приёмка полноты, round 3

**Полнота: 100% (18 / 18). Task38 принята по полноте.**

Аудитор не участвовал в реализации. Проверен только frozen snapshot `/tmp/prompty-task38-snapshot-03`, baseline `70fc301`. SHA-256 файла manifest `/tmp/prompty-task38-snapshot-03.sha256`: `59fb5176fd4f172c72039e1ac9e83b6eec8a0de804abdabc9058da24c5355881`. Hash самого manifest совпадает; `shasum -a 256 -c` подтверждает все перечисленные файлы. Integrity log `/tmp/task38-integrity-r3.log`.

Snapshot и рабочий implementation не изменялись. Проверки запускались на отдельной копии `/tmp/prompty-task38-audit-r3`, emitted consumer probes находятся в `/tmp/task38-probe-r3`. Текущий отчёт независимого аудитора ошибок не читался. Решение ниже касается полноты относительно исходного task38 и общего README, а не является заменой отдельной приёмки ошибок.

## Метод

Знаменатель сохранён из r1/r2: 8 обязательных пронумерованных требований, 9 acceptance criteria, обязательный closeout README. Partial/missing не засчитываются. ТЗ и границы не сокращены. Explicit reject неподдерживаемого inference/generation/merge принят только там, где ТЗ допускает диагностируемое unsupported вместо успешного ослабленного/усечённого контракта. Для этого не требуется универсальный application DTO или unknown-field storage.

## Матрица requirement → evidence → status

| ID | Обязательный пункт | Реализация и проверка | Status |
| --- | --- | --- | --- |
| C1 | Dynamic JSON exact integer/decimal/exponent; float только по caller-selected DTO type | `jsondoc.go:JSONDocumentAsMap`, runtime `jsonDocumentObject`, recipe `strictUnmarshalJSON`, manifest JSON/YAML, compiled metadata и structured/tool decoders используют UseNumber; `TestRuntimeDynamicNumbersRemainExact`, `TestStructuredDynamicOutputAndToolDecodePreserveNumbers`, YAML exact tests | complete |
| C2 | Exact structural equality, numeric spelling equivalence, large integer distinction, malformed never equal | `internal/exactjson.Equal` symbolic decimal/exponent normalization; `tool_contract.go:jsonDocumentsEqual` и composition используют его; `TestEqual` covers reordered keys, nested arrays, signed big ints, decimals, huge exponents, malformed/trailing documents | complete |
| C3 | Explicit schema priority/error; fallback only absent provider; ExtractSchema error contract/callers updated | `schema.go:ExtractSchema/schemaFromProvider`; provider compile in declared dialect; NewTypedTool/output callers обрабатывают errors; `TestExplicitMalformedSchemaNeverFallsBack`; field provider + quoted wire mismatch explicitly diagnosed | complete |
| C4 | Reflection encoding/json supported subset: string, omission/null/embedded, recursion/custom-codec errors | `schema_reflect.go` + shared `internal/jsonfield`; quoted scalar/Number, pointers/slices/null, embedding dominance; composite depth/cycle guards; `TestReflectionMatchesJSONPresenceAndQuotedScalars`, `TestReflectionQuotedJSONNumberMatchesEncoding`, `TestReflectionRejectsRecursiveCompositeTypes`, `TestReflectionRejectsFieldProviderStringProjection`, tag/embedded tests | complete |
| C5 | YAML/JSON inability to represent schema returns manifest error; contract never silently disappears | exact YAML numeric parser + errors from rawToSchemaDefinition and DecodeInputs; `TestSchemaNumbersExactAndNonJSONRejected` exercises output/input/tool nan/inf; plain numeric overflow preserved, explicit strings remain strings | complete |
| C6 | Composition retains/rejects full constraints; no local override/solver; metadata/dialect consistent; documented clear break | `MergeInputSchemas/inputSchemaMerge.add/validateNamespaces`: engine compile, whole-property/root exact equality, required union, restricted namespace reject; manifest fidelity tests for maxLength/nested/enum/const/required; docs/data-contract and closeout | complete |
| C7 | Generated original schema validation semantics; no generated nonzero/fraction rounding/new restrictions; raw vs typed presence explicit | Generated `JSONSchema`, `ValidateSchema`, validating raw decoder; no validator tags; exact numeric types; optional outer pointers; recursive nullable item/map representation; unsupported object projections rejected before emission; compiled `TestGeneratedConsumerPreservesPresence`, `TestGeneratorRejectsUnrepresentableObjectContracts`; independent r1/r2 probes rechecked below | complete |
| C8 | Schema vs caller domain Validate separate; no generator-owned domain restrictions | Generated method named ValidateSchema; caller Validate and tags remain separate in late/domain/structured paths; source engine remains validator; generated tags removed | complete |
| A1 | Public runtime checkpoint/overlay/late large signed numbers/nested/decimal round-trip through template | `TestRuntimeDynamicNumbersRemainExact`: BindRuntime + Execute, checkpoint input, nested late objects/arrays, exact exponent overlay | complete |
| A2 | Different large const rejected; equivalent numeric spelling agrees | exactjson tests + `manifest/schema_fidelity_test.go` public composition tests; public tool comparator uses same exact implementation | complete |
| A3 | Malformed provider rejected in output helper and NewTypedTool | `TestExplicitMalformedSchemaNeverFallsBack`, no reflection fallback and no dispatch | complete |
| A4 | Quoted DTO validates own marshal schema; recursive DTO rejected before IO | quoted scalar/Number marshal conformance tests; recursion guards including slice/array/pointer and existing struct recursion tests | complete |
| A5 | YAML minimum nan/inf manifest error, no successful disappearing ResponseFormat | exact parser tests cover response_format, inputs and tools | complete |
| A6 | maxLength100/maxLength3, nested/enum/required preserved or merge rejected | `TestCompositionRejectsLossOfFieldConstraints`, `TestCompositionExactNumericEqualityAndRequiredUnion`: conflict reject and merged-schema validation | complete |
| A7 | Actual compiled consumer accepts3.4/0/false/empty and rejects3.6/missing | `cmd/prompty-gen/testdata/gen_out/schema_fidelity_test.go`: real generated consumer test executes fractional/exact/big integer/nested/late input validation | complete |
| A8 | Golden behavior, generated recipes/late/checkpoint beyond textual code | gen_out builds recipe, BindLate, CheckpointJSON, DecodeRecipeCheckpoint; emitted consumer subprocess tests decode→typed validation→marshal with null/empty/nested arrays/maps | complete |
| A9 | Draft/local ref/original validation/lowering guarantees intact; no implicit resolver | Output helpers preserve dialect; inferred streaming item reference scope becomes a single embedded resource; public draft2020/draft7 ref stream success/failure regressions; schema compiler/reference/lowering targeted suite passes; explicit isolated loader unchanged | complete |
| G1 | Closeout: contracts, deleted legacy paths, migration, checks, limitations | reports/task38/closeout.md and checks-r3.md cover breaking APIs/DTO/presence/dialects, removed fallback/tags/float paths, caller migration, local/full checks and host/library/live-conformance boundaries | complete |

## Прежние blockers и adversarial probes

1. **R1 nullable binding:** independent public probe `main.go` на новой копии возвращает null (`<no value>` в простом template), не число0; explicit nullable-render test сравнивает original JSON и bound map, включая nil slices и nil nested object items. Early/late используют общий исправленный binder. Empty/null history presence сохраняется отдельно от flattened render history, public history-only scenarios проходят.
2. **R1/R2 optional collections:** compiled consumer теперь сохраняет optional `[]`/`{}` и explicit null через outer pointer; nested nullable fields тоже сохраняют presence. Empty Go json.Number semantics документированы согласованно с encoding/json (present zero), raw missing required отдельно отвергается.
3. **R1 dialect:** public probe ExtractSchema→WithResponseFormatFromStruct→Execute→ValidateSchemaJSON сохраняет draft7 и проходит; public inferred sync/native stream tests проходят.
4. **R2 nullable array items:** independent public generator probe создаёт **closed** object schema с optional integer-or-null array и minProperties1; emitted code компилируется. `{"items":[null]}` успешно decode/ValidateSchema/marshal и остаётся **{"items":[null]}**, вместо прежнего0. `TestNullableArrayItems` PASS. Общая recursive nullable mapping также применяется к typed map values; repo subprocess fixture покрывает null string map value, nullable object items, nested nullable array/boolean values и exact large integers.
5. **R2 source-valid unknown presence:** прежняя open schema minProperties1 диагностируется `GenerateManifestTypes` до emission: `open object keyword minProperties requires lossless unknown-field representation; use caller-owned types or closed object schema`. Это разрешённый explicit unsupported вариант, без успешного сужения source contract. Probe harness имеет panic при error, но сама library возвращает error. Repo reject tests проверяют open/conditional/nested projections, required вне properties, nullable root objects. No-late split больше не фильтрует required. Closed supported variants действительно проходят compiled behavior.
6. **Local refs stream:** public `TestStructuredStreamingPreservesItemReferenceScope` проверяет object/array для draft2020 и draft7, с local refs, включая provider $id; constraint violation отвергается `ErrSchemaMismatch`. Schema engine/ref/lowering guarantees проверены повторно, не заменены собственным solver.

## Проверки

Лично выполнены на отдельной копии:

- Root quoted/provider/recursion/nullable/history/dialect/public stream/reference/compiler fidelity tests — exit0.
- internal/exactjson, parser/yaml, manifest composition suites — exit0.
- Generator method collision, unsupported projection/field-name tests и compiled generated presence subprocess — exit0.
- Compiled gen_out schema fractional/presence/late/recipe/checkpoint behavioral tests — exit0.
- Independent previous-round probes: nullable/dialect pass; generator open projection expected rejection; closed nullable-array consumer PASS.

Просмотрены полные `/tmp/task38-lint-r3.log` и `/tmp/task38-test-r3.log`: all modules/zero lint issues; all-modules race + explicit generated testdata consumer проходят. Checks-r3 фиксирует exit0 и git diff --check. Live provider conformance не проверялся и не утверждается. Для локальных probes использовались tmp Go cache и отдельный go.work с module paths на копию frozen snapshot; implementation dependencies не менялись.

## Границы решения

Core остаётся BYOT: caller-owned DTO/domain restrictions, небольшие существующие schema/execution ports. Generator имеет явно диагностируемый typed subset; inference/composition также возвращают errors вместо truncated/overridden success. Не добавлены universal application DTO, agent loop, persistence, routing/retries, SDK types в core, неявные network/filesystem resolvers, фоновые процессы или compatibility wrappers. Provider SDK numerical fidelity остаётся task42. Примеры/fixtures/docs обновлены и проверены всеми модулями.

**Итог:** все18 обязательных пунктов выполнены в проверенном snapshot. Открытых blockers полноты не обнаружено. Коммит возможен после отдельного положительного решения аудитора ошибок на этом же snapshot; этот отчёт его не заменяет.
