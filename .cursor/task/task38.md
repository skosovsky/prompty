# task38 — Точные JSON-данные и единый контракт схем, DTO и codegen

**Статус:** accepted — round 4: completeness 100% (18/18), open confirmed errors 0; full lint/test green.
**Приоритет:** P1 для сохранности данных, P2 для schema/codegen discrepancies.
**База проверки:** `70fc301`, 6 октября 2026.
**Общие правила:** [README](README.md), включая clear break и границы библиотеки.

## Цель

Сделать JSON Schema исполняемым исходным контрактом на всём пути: JSON input → runtime/late binding → composition → caller DTO / generated DTO → tool/output validation. Успешное прохождение пути не должно менять числа, ослаблять явно переданную схему или создавать ограничения, которых в исходной схеме нет.

## Основание в коде

- [runtime.go](../../runtime.go), `jsonDocumentObject`: обычный `json.Unmarshal` в `map[string]any`; late input `9007199254740993` превращается в `9007199254740992`.
- [tool_contract.go](../../tool_contract.go), `jsonDocumentsEqual`: сравнение через `float64` считает разные большие числовые `const` одинаковыми.
- [schema.go](../../schema.go), `schemaFromProvider`: ошибка явного `SchemaProvider` становится признаком отсутствующего provider и включает reflection fallback.
- [schema_reflect.go](../../schema_reflect.go): рекурсия заменяется закрытым пустым object; JSON option `string` не отражается в схеме. DTO может не проходить собственную inferred schema.
- [parser/yaml/yaml.go](../../parser/yaml/yaml.go), `rawToSchemaDefinition`: ошибка сериализации `.nan` превращается в `nil` response format без ошибки parse.
- [manifest/compose.go](../../manifest/compose.go), `MergeInputSchemas` / `samePropertyType`: совпадения type/format/late достаточно, чтобы потерять imported `maxLength`, enum или другие ограничения.
- [cmd/prompty-gen/gen/schema.go](../../cmd/prompty-gen/gen/schema.go), `buildValidateTags`: `minimum`/`maximum` приводятся к `int`; JSON Schema required отображается в nonzero-проверку Go validator.
- Уже есть number-preserving decoding в [jsondoc.go](../../jsondoc.go) и [tool_binding.go](../../tool_binding.go). Переиспользовать подход, не создавать независимые несовместимые JSON-пути.

## Требуемый контракт

1. Dynamic JSON values сохраняют точное значение integer/decimal/exponent. Не пропускать их через `float64`, если caller явно не выбрал такой DTO-тип. Формат числа в тексте может нормализоваться только без изменения значения; гарантии lexical identity отдельно не обещать.
2. Сравнение JSON schema documents структурное и точное: порядок ключей несущественен, `1`, `1.0`, `1e0` численно равны, соседние большие integer различны. Это не алгоритм доказательства логической эквивалентности произвольных JSON Schema. Malformed JSON не считается совпадающим контрактом даже при одинаковых bytes.
3. Явная schema имеет приоритет. Ошибка её чтения/компиляции возвращается caller с причиной и местом; fallback допустим только при отсутствии явного provider. Изменить публичный error contract `ExtractSchema` и callers, если текущая сигнатура скрывает причину.
4. Reflection соответствует `encoding/json` для заявленного поддерживаемого подмножества. Поддержать `json:",string"`; рекурсивные типы представлять корректными `$defs/$ref` либо явно отвергать до dispatch. Custom marshalers и неподдерживаемые конструкции требуют явного `SchemaProvider` или диагностируемой ошибки. Нельзя возвращать успешную усечённую схему. Проверить `omitempty`, `omitzero`, pointers/null и embedded fields.
5. YAML/JSON parser возвращает ошибку при невозможности представить schema как JSON. Невалидный output/input/tool contract не превращается в отсутствие контракта.
6. Composition сохраняет ограничения всех задействованных input contracts. Для совпадающего свойства выбрать пересечение через существующий schema engine либо reject несовместимого/неподдерживаемого объединения; не оставлять неявный local override. Не писать собственный solver совместимости. Type/late metadata должны оставаться согласованными. Зафиксировать эту clear-break семантику в документации.
7. Codegen сохраняет исходную schema и использует её как источник validation semantics. Generated tags не должны сужать/расширять допустимые данные: presence отличается от nonzero, дробные границы не округляются. Для raw JSON missing и explicit zero различаются; для caller-created typed value правила presence должны быть явными. При необходимости менять generated DTO/presence representation, без universal application DTO.
8. Schema validation и domain `Validate()` остаются разными этапами. Domain restrictions принадлежат caller; генератор не должен незаметно добавлять их сам.

## Acceptance и воспроизведения

- [x] Runtime checkpoint input, overlay и late binding сохраняют `9007199254740993`, отрицательные большие числа, вложенные arrays/objects и дроби; round-trip проверен через публичные API и шаблон.
- [x] Схемы с `const:9007199254740992` и `const:9007199254740993` не проходят проверку совпадения; эквивалентная запись одного числа обрабатывается по заданному контракту.
- [x] Malformed `SchemaProvider` отвергается в output helper и `NewTypedTool`; reflection fallback не включается.
- [x] DTO с `json:"count,string"` проходит собственную schema после JSON marshal; recursive DTO либо корректно round-trips, либо получает явный unsupported до IO.
- [x] YAML `minimum: .nan` / `.inf` в schema возвращает ошибку manifest; `ResponseFormat` не исчезает с успешным parse.
- [x] Parent `maxLength:100` + child `maxLength:3` не допускают длину 4 как успешно проверенный общий input; nested constraints/enum/required покрыты аналогично.
- [x] Сгенерированный и скомпилированный consumer принимает `3.4` при `maximum:3.5`, отвергает `3.6`; required `0`, `false`, `""` разрешены, когда исходная schema не запрещает их. Missing required field отвергается в JSON entrypoint.
- [x] Golden fixtures проверяют не только текст кода, но поведение generated recipes/late input/checkpoints.
- [x] Существующие draft selection, local `$ref`, original-schema validation и schema lowering guarantees не ослаблены. Нет неявных network/filesystem resolver.

## Границы и завершение

Provider SDK numerical fidelity реализуется в [task42](task42.md) по этому контракту. Не заменять SDK mapping изменением domain IDs на string и не навязывать caller новый тип ID. Использовать существующий JSON Schema engine. Обновить root, parser/manifest, генератор, fixtures и примеры; проверки по общему README.

## Приёмка

Frozen snapshot round 4: `a1337cdb30d32aa3e25b4d2f8d859736814c38bc5a476bfb92cabc9cd23c09f7`, 672 файла. Независимые [полнота](reports/task38/completeness-r4.md) и [ошибки](reports/task38/errors-r4.md), [проверки](reports/task38/checks-r4.md), [closeout и migration](reports/task38/closeout.md). Runtime/code после frozen acceptance не менялись; финальные изменения касаются только статуса документов приёмки.
