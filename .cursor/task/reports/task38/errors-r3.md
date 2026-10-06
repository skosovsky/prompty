# Task38 — независимый аудит ошибок, round 3

**Решение: не принято. Открыты 2 подтверждённых P2.** Исправления r2 закрыли проверенные numerical/null loss, recursion и silent required/provider overwrite, но local-ref streaming и dialect-aware object projection остаются неполными.

Проверен только frozen `/tmp/prompty-task38-snapshot-03`, baseline `70fc301`. SHA256 file manifest `/tmp/prompty-task38-snapshot-03.sha256`: `59fb5176fd4f172c72039e1ac9e83b6eec8a0de804abdabc9058da24c5355881`. Integrity verification `shasum -a 256 -c` — exit 0, log `/tmp/task38-errors-r3-integrity.log`. Аудитор в implementation не участвовал; текущий completeness-r3 другого аудитора не читался. Source/frozen не менялись. Executed probes находятся в отдельной `/tmp/prompty-task38-errors-probes-r3`.

## Подтверждённые ошибки

### R3.1 — P2: inferred stream item boundary не работает для empty/colliding $id

**Место:** `stream_structured_output.go:158–175`, особенно `161–163`.

R2 wrapper исправлен для item schema без `$id`, а также для обычного distinct absolute/relative `$id`. Однако сам факт наличия `$id` не гарантирует отдельную resource boundary. Валидный `$id:""` разрешается в идентификатор текущего root; `$id:"https://prompty.invalid/source-schema"` совпадает с URI, на котором compiler регистрирует stream wrapper. Fragment references item schema снова разрешаются в неправильном resource scope/конфликтуют с wrapper.

**Executed reproduction:** root `audit_r3_test.go`, `TestAuditR3StreamIDs`. Для каждого id сначала успешно вызывается `ExtractSchema` и original standalone `ValidateSchemaJSON` на `{"n":1}`; schema содержит `$defs.positive`, `properties.n.$ref = "#/$defs/positive"`, required n. Затем public `StreamStructuredOutput[AuditR3Item]` получает invoker с valid `[{"n":1}]`.

- `$id:""`: compile error, `called=false`, values empty.
- `$id:"https://prompty.invalid/source-schema"`: тот же compile error до invoker.
- `$id:"item.json"`: invoker вызван, value `{N:1}` получен, nil error.
- `$id:"https://example.com/item"`: также success.

Это воспроизводится на валидных caller schemas, а не на unresolved external refs. Local resource/dialect guarantee task38 требует корректного item scope; объявленное в docs сохранение referenced item contracts не выполнено для этих ids.

**Нужно:** гарантировать отдельную document/item resource identity после URI resolution, включая empty, relative и colliding ids. Не подменять constraint или включать implicit network/filesystem loader. Проверить object/array forms и draft7/2020; валидный original item schema не должен случайно ломаться от internal wrapper URI.

### R3.2 — P2: draft7 $ref siblings ошибочно считаются closed object для codegen projection

**Мсто:** `cmd/prompty-gen/gen/schema.go:592–600`, unconditional early return при explicit `additionalProperties:false`.

В draft-07 sibling keywords рядом с `$ref` игнорируются. Guard смотрит на синтаксический `additionalProperties:false`, не на dialect/reference semantics, и разрешает generated declared-field projection, хотя actual referenced schema открыта и может требовать unknown fields для root presence constraints.

**Executed reproduction:** generator `cmd/prompty-gen/gen/audit_r3_test.go`, `TestAuditR3Draft7ReferenceProjection`, создаёт input definition с Dialect draft7 и schema:

```json
{
  "$schema": "http://json-schema.org/draft-07/schema#",
  "$ref": "#/definitions/open",
  "type": "object",
  "properties": {"a": {"type": "string"}},
  "additionalProperties": false,
  "definitions": {"open": {"type": "object", "minProperties": 1}}
}
```

GenerateManifestTypes успешно выдаёт и сохраняет `audit_r3_projection_gen.go`. Consumer `cmd/prompty-gen/testdata/gen_out/audit_r3_test.go`, `TestAuditR3Draft7ReferenceProjection`, успешно декодирует исходно валидный `{"extra":1}` через generated raw decoder. Typed remarshal даёт `{}`; `ValidateSchema()` возвращает mismatch `minProperties: got 0, want 1`. Emitted consumer компилируется, ошибка именно semantic projection.

Для unsupported shape допустим explicit diagnostic по C7; не требуется universal unknown-field container или собственный solver. Но успешная генерация не должна сужать accepted source data из-за ошибочного closed-object shortcut. Корректность source engine здесь сохранена: он правильно учитывает draft7 semantics, ошибается guard.

**Нужно:** учитывать `$ref` sibling semantics declared dialect при выборе supported projection subset или явно отвергать такие reference/object комбинации до emission. Проверить nested и output schemas тоже.

## Повторная проверка r1/r2 и полный scope

- Named recursive slice/array/pointer: общий depth guard и cycle-safe normalization возвращают ErrSchemaUnsupported; старые fatal/hang reproductions покрыты passing regression tests и code inspection.
- Field provider + json string option: explicit unsupported diagnostic вместо overwrite; это разрешённый C4 вариант, enclosing provider остаётся escape hatch. Builtin quoted scalars/json.Number продолжают проходить actual encoding/json round-trip.
- Nullable array items, nested arrays/objects и typed map values: recursive nullable type mapping и compiled emitted consumer regressions проходят; r2 null-to-zero loss на проверенных shapes устранён.
- Root required вне properties, nullable root struct и open presence/conditional shapes: explicit generator rejection, no-late schema больше не удаляет required. Проверенные r2 простые reproductions устранены; R3.2 обнаруживает отдельный draft7 reference bypass.
- Local-ref stream tests без `$id` и с distinct draft7 id проходят; R3.1 оставляет empty/collision boundary случаи открытыми.
- Повторно просмотрены/запущены dynamic numeric runtime/checkpoint/overlay/late, parser exact/non-finite schema errors, exact comparator, composition whole-contract/root-namespace rejection, source schema defaults/raw decoder/presence, original early+late runtime validation/history values, output/tool/stream number decoding, malformed providers и separate domain validation. Не обнаружено новых подтверждённых ошибок в этих проверенных paths.
- Unsupported diagnostics оценивались по C4/C7 и объявленному typed subset: сами по себе explicit rejects не считались ошибкой, если сохраняют возможность caller-owned types/explicit contracts и предотвращают silent weakening. Implicit IO, compatibility fallback, agent loop или universal DTO не добавлены.

## Проверки и пределы

1. Независимо в frozen запущен `GOCACHE=/tmp/prompty-go-cache go test ./... ./cmd/prompty-gen/gen ./cmd/prompty-gen/testdata/gen_out` — exit 0, log `/tmp/task38-errors-r3-baseline.log`.
2. Прочитаны `checks-r3.md`, closeout, logs `/tmp/task38-lint-r3.log` и `/tmp/task38-test-r3.log`: full modules, race и generated consumers passed; lint zero issues. Эти full make executions являются evidence автора; независимый execution ограничен указанной suite и дополнительными probes.
3. Независимый exact dynamic JSON probe проходит; stream id и emitted draft7 consumer probes воспроизводят оба findings. Source/frozen не изменялись.
4. Live provider calls отсутствуют. Нет утверждения об абсолютной корректности всех JSON Schema/encoding/json сочетаний. Наличие двух confirmed findings блокирует текущий error acceptance; после исправлений нужен новый frozen snapshot.
