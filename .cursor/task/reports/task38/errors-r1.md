# Task38 — независимый аудит ошибок, round 1

**Решение: не принято.** Найдены 5 подтверждённых ошибок реализации и 1 ошибка документированного контракта. Аудитор не участвовал в реализации; отчёт второго аудитора не читался.

Проверен только frozen snapshot `/tmp/prompty-task38-snapshot-01`, baseline `70fc301`. SHA256 manifest: `e75841b2084db5b5ec1dd0e6bc6d67cc35be71835585758a6cdfacbd77f1e884`. Проверка `shasum -a 256 -c /tmp/prompty-task38-snapshot-01.sha256` завершилась exit 0. Snapshot и рабочий код не менялись; probes выполнялись в отдельной копии `/tmp/prompty-task38-errors-probes`.

## Подтверждённые findings

### R1 — P2: output helpers теряют dialect явного SchemaProvider

**Место:** `structured_output_contract.go:28`, `render_plan.go:188`.

Provider с валидной schema draft-07 успешно проходит `ExtractSchema`, однако `ResponseFormatFromStruct[T]` возвращает `SchemaDefinition{Schema: doc}` без `Dialect`. `WithResponseFormatFromStruct` делает то же самое. CompileSchema выбирает default 2020-12 и отвергает объявленный draft-07, поэтому корректный caller contract перестаёт работать в structured output. Документация обещает сохранение выбранного dialect downstream.

Воспроизведение в `/tmp/prompty-task38-errors-probes/audit_errors_test.go`, `TestAuditDraft7ResponseHelper`: тип `AuditDraft7` возвращает `{"$schema":"http://json-schema.org/draft-07/schema#","type":"object","properties":{"n":{"type":"integer"}},"required":["n"]}`. Helper успешно возвращает definition с `Dialect == ""`; `ValidateSchemaJSON(def, {"n":1})` возвращает `prompty: JSON Schema compilation failed`.

**Нужно:** переносить выбранный dialect во все output helpers и проверить публичный structured execution path.

### R2 — P2: json.Number с json string option не проходит собственную inferred schema

**Место:** `schema_reflect.go:191–198`, `schema_reflect.go:351–367`.

Reflection отдельно распознаёт json.Number, но обработка `,string` затем классифицирует его как обычный Go string. Для `type DTO struct { N json.Number `json:"n,string"` }` Marshal значения `9007199254740993` даёт `{"n":"9007199254740993"}`, а успешно извлечённая схема ожидает строку с дополнительными внутренними JSON-кавычками. Actual encoding/json payload отвергается.

Воспроизведение: `TestAuditQuotedNumber` в root probe. `ValidateSchemaJSON` возвращает mismatch на `/n` с pattern `^"..."$`. Это затрагивает введённый точный numeric DTO type и заявленную поддержку `,string`.

**Нужно:** учитывать специальную encoding/json семантику json.Number и проверить value/pointer + integer/decimal/exponent.

### R3 — P2: новые generated methods конфликтуют с допустимыми именами input/output properties

**Место:** `cmd/prompty-gen/gen/schema.go:448–454`, `schema.go:472–483`; collision guard `validatePropNames` проверяет только поля друг против друга.

Для input schema `{"type":"object","properties":{"validate_schema":{"type":"string"}},"required":["validate_schema"]}` `GenerateManifestTypes` успешно генерирует DTO, но consumer не компилируется: `field and method with the same name ValidateSchema`. Аналогичная проверка нужна для JSONSchema и UnmarshalJSON, root Input/LateInput/Output. Ошибка возникла из-за добавленных методов; valid manifest получает успешную генерацию неработающего Go-кода.

Воспроизведение: `TestAuditMethodFieldCollision` в `/tmp/prompty-task38-errors-probes/cmd/prompty-gen/gen/audit_errors_test.go`; сохранённый `audit_collision_gen.go` затем проверен `go test ./cmd/prompty-gen/testdata/gen_out -run '^$'` и дал compile error. После фиксации вывода файл удалён только из probe copy для следующих тестов.

**Нужно:** предотвратить столкновение имён либо явно reject до emission; проверить все автоматически добавленные methods.

### R4 — P2: optional slice/map presence теряется, source-valid input отвергается typed validation

**Место:** `cmd/prompty-gen/gen/schema.go:412–438`.

Генератор определяет optional только по root `required`, оставляет slice без pointer и добавляет `omitempty`. При schema `{"type":"object","properties":{"items":{"type":"array","items":{"type":"integer"}}},"allOf":[{"required":["items"]}]}` сырой `{"items":[]}` валиден. Generated UnmarshalJSON успешно принимает его, но remarshal даёт `{}` и `input.ValidateSchema()` отвергает его как missing items. Поэтому публичный recipe constructor также не может принять допустимый original-schema input. Это не ошибка schema engine: presence уничтожается generated representation. Аналогичный риск существует у optional map и explicit null в optional nullable поле; подтверждённый probe здесь именно slice/allOf.

Воспроизведение: `TestAuditEmptyArrayPresence` сначала генерирует `AuditPresenceInput`, затем consumer test с тем же именем фиксирует remarshal `{}` и mismatch `allOf -> missing property items`. Файлы находятся в probe copy.

**Нужно:** сохранять presence пустых коллекций (и явно определить null/absence для всех поддерживаемых shapes) либо диагностируемо отвергать представления, которые генератор не может сохранить. Не сужать schema через успешную генерацию.

### R5 — P2: reflection принимает JSON tag name, который encoding/json обрабатывает иначе

**Место:** `schema_reflect.go:258–275` (`parseJSONSchemaTag`), field name emission `181–184`.

Tag parser переносит имя без проверки правил encoding/json. Probe `type DTO struct { N int `json:"a\\b"` }` успешно получает schema с required property `a\b`, но encoding/json Marshal на установленном Go выдаёт другой ключ; в фактическом probe вывод `{"a":1}`. Собственная схема отвергает actual JSON с missing и additional-property errors. Неподдерживаемый tag следует отвергать, если exact encoding/json naming semantics не реализуются.

Воспроизведение: `TestAuditInvalidJSONTag`, полный код и вывод в root probe; ошибка `missing property a\b; additional properties a not allowed`. Сохранить сам reproduction test, поскольку escapes в Go struct tags существенны.

**Нужно:** использовать правила encoding/json для допустимых tag names или возвращать явный unsupported вместо успешной несовпадающей схемы.

### R6 — P3: документация обещает rejection пустого json.Number, которого нет

**Место:** `docs/schema-data-contract.md:7`.

Текст утверждает, что required numeric field с пустым Go zero value invalid. В encoding/json пустой json.Number маршалится как `0`. `SchemaFidelityInput{Enabled: &false}` с пустыми Amount/Count сериализуется как `{"amount":0,"count":0,"enabled":false,"text":""}` и `ValidateSchema()` возвращает nil. Acceptance task38 допускает required zero, поэтому допустимо исправить именно обещание документации; если выбирается explicit-token presence, реализация должна обеспечить её отдельно.

Воспроизведение: consumer `TestAuditNumericZeroContract` в probe copy; root `TestAuditEmptyNumber` отдельно фиксирует marshal пустого json.Number в `0`.

## Проверки и пределы

Независимо выполнено в frozen snapshot: `GOCACHE=/tmp/prompty-go-cache go test ./... ./cmd/prompty-gen/gen ./cmd/prompty-gen/testdata/gen_out` — exit 0; log `/tmp/prompty-task38-errors-baseline.log`. Предоставленные до freeze результаты make lint и make test (race/all modules/generated consumers) приняты как evidence автора; повторная независимая проверка здесь ограничивалась указанной suite и adversarial probes, а не full lint/race всех adapter modules.

Просмотрены JSON exact comparison, parser exact YAML conversion, malformed provider propagation, recursion/custom-codec rejection, composition guards, generated schema validation/defaults/split и full runtime validation. Независимые probes подтверждают проблемы, не покрытые passing suite. Наличие findings блокирует error acceptance текущего snapshot; отсутствие прочих найденных ошибок не является абсолютной гарантией корректности.
