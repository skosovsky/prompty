# Task38 — независимый аудит ошибок, round 2

**Решение: не принято. Открыты 5 подтверждённых classes ошибок.** Findings первого раунда в проверенных воспроизведениях устранены, однако полный error acceptance пока не выполнен.

Аудитор не участвовал в реализации; отчёт второго аудитора round 2 не читался. Проверен frozen `/tmp/prompty-task38-snapshot-02`, baseline `70fc301`. SHA256 file manifest: `66b0198f32730a1147dd84e5b30e8a15098c4b523aa3828e0371f720d7a582b6`. `shasum -a 256 -c /tmp/prompty-task38-snapshot-02.sha256` завершился exit 0. Snapshot и рабочая implementation не изменялись. Все дополнительные tests выполнены в отдельной копии `/tmp/prompty-task38-errors-probes-r2`.

## Подтверждённые findings

### R2.1 — P1: recursion guard не охватывает named slices и named pointers

**Место:** `schema_reflect.go:41–49, 73` и struct-only guard ниже; `schema.go:52–57`.

Публичный `ExtractSchema` для `type Recursive []Recursive` бесконечно вызывает schemaForType через slice items: depth растёт, но проверка depth/active существует только в обработке structs. Результат — fatal stack overflow, а не обещанный ErrSchemaUnsupported. Named recursive pointer `type RecursivePointer *RecursivePointer` зависает раньше: normalizedSchemaType бесконечно удаляет pointer, поскольку Elem возвращает тот же named type.

**Воспроизведение:** root probe `audit_errors_r2_test.go`, `TestAuditRecursiveSlice` и `TestAuditRecursivePointer`. Slice subprocess ограничил stack через `debug.SetMaxStack(128<<10)`, чтобы безопасно воспроизвести fatal; log `/tmp/task38-r2-recursive-slice.log`. Pointer subprocess запущен с `go test -run TestAuditRecursivePointer -timeout=1s -v .`; log `/tmp/task38-r2-recursive-pointer.log` фиксирует timeout и goroutine в `normalizedSchemaType -> reflect.Elem`. Frozen не менялся.

**Нужно:** единая проверка циклов и глубины всех поддерживаемых type shapes, включая pointer normalization, с error return до IO. Explicit providers должны оставаться применимыми там, где способны задать правильную schema.

### R2.2 — P2: json string option стирает constraints явного field SchemaProvider

**Место:** `schema_reflect.go:196–205`.

Сначала schemaForType корректно возвращает explicit field schema, затем ветка json string option полностью заменяет её inferred pattern. `type AuditStrictCount int` с provider `{"type":"integer","const":42}` в DTO поле с tag `json:"n,string"` теряет const. Значение 43 маршалится в `{"n":"43"}` и успешно валидируется успешно извлечённой schema, содержащей только type string и integer pattern. Explicit provider контракт ослабляется без ошибки.

**Воспроизведение:** root `TestAuditFieldProviderString`; вывод показывает `properties.n = {type:string,pattern:integer}` без const и validation success для 43.

**Нужно:** сохранять явный wire contract для actual codec либо возвращать диагностируемый unsupported для комбинации, которую нельзя корректно преобразовать. Нельзя оставлять успешный silent overwrite. Проверить также custom codec с provider, на котором encoding/json string option не действует как на builtin primitive.

### R2.3 — P2: inferred structured streaming ломает local ref scope

**Место:** `stream_structured_output.go:151–153`.

Валидная provider item schema с `$defs` и `$ref:"#/$defs/positive"` вставляется дважды в новый anyOf wrapper. Fragment ref теперь указывает на root wrapper, у которого нужных `$defs` нет. Публичный `StreamStructuredOutput[T]` отвергает такую schema до вызова invoker, хотя extraction и обычный schema validator принимают item contract. Это нарушает требуемое сохранение local refs и заявленный source schema contract для stream пути.

**Воспроизведение:** root `AuditLocalRef` provider возвращает `{"type":"object","$defs":{"positive":{"type":"integer","minimum":1}},"properties":{"n":{"$ref":"#/$defs/positive"}},"required":["n"]}`. `TestAuditStreamLocalRef` вызывает public iterator с valid JSON `[{"n":1}]`; получает `called=false`, пустые values, `prompty: JSON Schema compilation failed`.

**Нужно:** сохранять идентичность/schema resource scope item contract при wrapping, через существующий schema engine и local resources, без implicit network/filesystem resolver. Проверить draft7/2020, `$id`, fragment refs и array/single object forms.

### R2.4 — P1: nullable array scalar members становятся zero без ошибок

**Место:** `cmd/prompty-gen/gen/schema.go:232–254` (`mapSchemaToGo` игнорирует nullable вне struct field shaping).

Для schema `{"type":"object","properties":{"numbers":{"type":"array","items":{"type":["integer","null"]}}}}` успешно генерируется `[]json.Number`. Generated decoder принимает `{"numbers":[null]}`, но remarshal даёт `{"numbers":[0]}`. ValidateSchema возвращает nil, поскольку оба значения допустимы source schema. Это именно silent loss actual value, а не только отсутствие поддержки формы. Top-level property pointer fixes не решают nullable members внутри arrays. Аналогичные primitive item shapes требуют проверки.

**Воспроизведение:** generator `TestAuditNullableMembers` пишет `audit_nullable_gen.go`, consumer `TestAuditNullableMembers` фиксирует `source={"numbers":[null]} result={"numbers":[0]} validation=<nil>`. Оба tests находятся в probe copy. Nullable primitive additionalProperties с union type сейчас явно rejected, поэтому для map union не утверждается подтверждённая silent-loss ошибка.

**Нужно:** корректно представлять nullable child types рекурсивно либо явно reject их до emission, не пропуская null через zero scalar. Проверить nested arrays, primitive kinds и typed map value shapes.

### R2.5 — P1: codegen удаляет required имена вне properties даже без late partition

**Место:** `cmd/prompty-gen/gen/input_split.go:49, 57–77`.

JSON Schema допускает required имя без его декларации в properties. Schema `{"type":"object","properties":{"a":{"type":"string"}},"required":["unknown"]}` валидна и требует presence unknown. Генератор успешно её компилирует, затем cloneObjectSchemaWithProps удаляет required и заново строит его только по keys properties. Это происходит даже при отсутствии late fields. Generated JSONSchema и raw/typed validation уже не содержат required constraint. Последующая projection dropping unknown поэтому не приводит к error: `ValidateSchema()` принимает `{"a":"ok"}`, который исходная schema запрещает.

**Воспроизведение:** generator `TestAuditOpenRequired` и consumer `TestAuditOpenRequired`. Вывод consumer: `projection={"a":"ok"} emitted_schema={"properties":{"a":{"type":"string"}},"type":"object"} validation=<nil>`. Это подтверждённое ослабление source schema, независимо от документированного unknown-field projection. Не заменять finding только оговоркой о lossless unknown storage.

**Нужно:** не терять required constraints; если generated typed representation/partition не может обеспечить их, явно reject schema на этапе генерации. В более общих open-object presence/conditional cases сохранять constraint либо reject unsupported projection.

## Закрытые findings round 1 и coverage

Независимо просмотрены исправления r1 и повторно запущены root/generator/compiled consumer suites: explicit draft7 output helpers и stream, json.Number string option, method collision checks для Input/LateInput/Output, empty slice/map и nested null presence, invalid JSON tag name errors, numeric Go zero documentation. В проверенных r1 воспроизведениях ошибок больше не найдено; R2.2 относится к дополнительному explicit field provider сочетанию, R2.3 — к local refs, R2.4 — к array item nullable shapes.

Аудит охватывал task38 и общий README: exact JSON dynamic paths (checkpoint, runtime, overlay, late, compiled metadata, structured/tool/stream decode), numeric comparator, provider errors/dialects, reflection embedding/omission/pointers/custom codecs/recursion, YAML numeric tokens и non-finite rejection, composition whole-property/root namespace checks, generated schema preservation/defaults/raw decode/presence/split, PlanInputFrom и combined runtime validation/historyValues. Проверены новые empty/null history tests и code path, сохраняющий field presence отдельно от flattened history; дополнительных подтверждённых history ошибок не найдено в проверенных сценариях.

## Проверки и пределы

- Snapshot integrity — exit 0, `/tmp/task38-errors-r2-integrity.log`.
- Независимый `GOCACHE=/tmp/prompty-go-cache go test ./... ./cmd/prompty-gen/gen ./cmd/prompty-gen/testdata/gen_out` в frozen snapshot — exit 0, `/tmp/task38-errors-r2-baseline.log`.
- Проверены предоставленные `/tmp/task38-lint-r2.log` и `/tmp/task38-test-r2.log`: все перечисленные workspace modules, race и generated consumer pass; lint 0 issues. Full make lint/make test здесь повторно не запускались, они являются evidence автора, а независимый execution указан отдельно.
- Все пять findings имеют собственные executed probes. Recursive probes запускались отдельными subprocess с ограниченным stack/timeout; остальные tests также не меняли frozen/source.
- Live providers не вызывались. Не доказана абсолютная корректность всех допустимых encoding/json/schema combinations. Подтверждённые ошибки достаточно блокируют принятие snapshot; после исправлений требуется новый freeze и повторный полный аудит.
