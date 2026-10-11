# Task38: контракт реализации

Статус: принято round 4 — полнота 100% (18/18), открытых подтверждённых ошибок 0; make lint/test green. База: 70fc301.

- Dynamic JSON maps используют json.Number. Caller-selected Go numeric fields сохраняют выбранную caller семантику. Malformed/trailing JSON отклоняется.
- Structural JSON equality сравнивает числа по точному десятичному значению, включая exponent; не проверяет логическую эквивалентность JSON Schema. Malformed документы не равны.
- ExtractSchema возвращает (schema, error). Явный SchemaProvider с ошибкой не включает reflection fallback. Inference не поддерживает custom JSON/text codecs без явной schema и рекурсивные типы: возвращает ErrSchemaUnsupported. JSON string/omitempty/omitzero и nullable поля имеют явную семантику.
- Composition объединяет пространство input полей; повторные поля должны иметь одинаковые контракты, включая nested constraints. Различия отклоняются, без override/solver. Root constraints сохраняются либо несовместимое объединение отклоняется.
- Generated numeric DTO fields используют json.Number; пустой Go zero value json.Number сериализуется как 0 по правилам encoding/json и считается присутствующим значением. Root constraints, которые нельзя корректно спроецировать на early/late DTO, явно отклоняются генератором.
- JSON Schema остаётся источником generated validation. Generated ValidateSchema не добавляет nonzero/domain restrictions и не занимает доменный Validate. JSON entrypoints проверяют исходный документ до typed decode, чтобы missing и zero различались. Typed values представляют присутствующие required поля, optional поля управляются JSON encoding.
- Early/late partitions проверяются как schema contracts; optional defaults не заменяют present null/zero/empty. Domain Validate и caller validation tags остаются отдельными ограничениями.

Дальнейшие API детали и evidence добавляются по реализации. Документ не утверждает полноту или отсутствие ошибок.

- Inferred stream document wrapper и item имеют разные resource identities. Item `$id` разрешается относительно прежнего source address; пустой и отсутствующий `$id` сохраняют исходный reference scope. Compiler использует absolute root identity как retrieval address, если она не занята supplied resource; diagnostics продолжают указывать на supplied source через Resource="". Network/resolver policy не меняется.
- Generated root object с `$ref`/`$dynamicRef`/`$recursiveRef` явно unsupported: closed siblings не доказывают closure referenced object, особенно в draft-07. Runtime engine и caller-owned types продолжают поддерживать references.
