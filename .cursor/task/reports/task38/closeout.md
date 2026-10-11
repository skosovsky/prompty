# Task38 — closeout реализации

Breaking change; приёмка round 3 ожидается. Первый snapshot отклонён: полнота 15/18 (83,33%), пять ошибок реализации и одна ошибка документации. Round 2 отклонён: полнота 17/18 (94,44%), пять дополнительных error classes. Коммита задачи пока нет.

## Контракты и миграция

- `ExtractSchema` теперь возвращает `(map[string]any, error)`. Explicit provider компилируется в объявленном dialect, ошибки не включают reflection fallback. Output helpers/stream и typed tools сохраняют dialect. Caller должен обработать ошибку до IO.
- Dynamic JSON использует `json.Number`: runtime/checkpoint/overlay/late, manifest, compiled metadata, tool decode и structured output. Caller-owned numeric DTO types сохраняют семантику выбранного Go type. Обновить assertions и casts, которые ожидали float64.
- JSON equality структурное, с точным десятичным сравнением числа и без построения огромных exponent-expanded значений. Оно не доказывает логическую эквивалентность схем.
- Reflection возвращает ошибки для recursion/custom codecs без explicit provider, неподдерживаемых types и JSON tag names. Поддерживаемые `string`, pointer/null, omission и embedding проверяются против encoding/json. `json.Number` с `,string` использует numeric string codec.
- Generated numeric поля — `json.Number`; required означает presence, а не nonzero. `JSONSchema`, `ValidateSchema`, raw `UnmarshalJSON` используют исходную schema. Domain `Validate` и caller validation tags остаются отдельными стадиями. Regenerate DTO и fixtures; удалить ожидания generated go-validator tags.
- Optional collections теперь pointer fields. Optional nullable scalar/object — два уровня pointer; null коллекции — ненулевой outer pointer к nil collection. Decode сохраняет absent/empty/null, включая nested fields. Names, конфликтующие с generated methods или не представимые JSON tags, отвергаются при генерации. Nullable items/typed map values используют pointer representation рекурсивно; nullable root structs явно отвергаются.
- Caller-created required zero fields присутствуют по правилам encoding/json. Пустой Go `json.Number` сериализуется как 0; explicit-token presence отдельно не обещается.
- Binding сохраняет nil/null, не создаёт zero DTO, false или empty collections. Шаблоны должны явно обрабатывать null. Presence пустого/null history сохраняется отдельно от flattened render history; history-only payload с пустой history допустим. `prompt` omission независимо от `json` tags; generated optionals имеют `prompt:"name,omitempty"`.
- Composition объединяет required и properties, требует совпадения всего repeated-property contract и root constraints; несовместимые namespaces/ограничения явно отвергаются. Нет local override или solver. Полный original input проверяется после объединения early/late, до rendering defaults.
- YAML сохраняет exact integer/decimal/exponent, включая переполнение numeric resolver; quoted/explicit strings остаются строками. Non-finite values, unsupported tags, recursive aliases и non-string keys возвращают manifest error.

## Удалённые paths

Удалены silent provider fallback, silent schema serialization drop, ordinary float64 decoding на dynamic JSON paths, прежний float comparator и composition type-only/local override. Удалены generated validation tags и округление дробных границ, implicit required-nonzero check, synthesis zero/nested objects для nil bindings. Не добавлены compatibility wrappers.

## Проверки

В раунде исправлений проходят targeted root suite, parser/yaml, все prompty-gen packages и скомпилированный gen_out. Golden fixtures и gen_out обновлены генератором. Новый subprocess test компилирует отдельный emitted consumer и проверяет presence при decode → typed validation → marshal; это behavioral проверка, не сравнение текста.

Полные `make lint` / `make test` и идентификатор следующего frozen snapshot будут записаны в `checks-r4.md`. Два независимых отчёта приёмки сохраняются отдельно. Этот closeout не заменяет их решений.

## Ограничения и границы

- Inference — диагностируемое подмножество encoding/json, без synthesis рекурсивных $defs; custom codecs требуют explicit provider. Генератор поддерживает своё typed schema subset, unsupported shapes возвращают ошибки.
- Required names вне declared properties и open-object presence/conditional/reference constraints, которым нужна lossless unknown-field representation, явно отвергаются до generation. Root constraints, которые нельзя корректно проецировать across early/late partitions, отвергаются; runtime дополнительно проверяет combined original schema. Optional defaults — rendering behavior, не способ подменить present null/zero.
- Open-schema DTO follows encoding/json projection onto declared fields; исходный raw JSON проверяется до decode, typed представление повторно проверяется отдельно. Не обещается lexical identity чисел или lossless raw unknown-field storage.
- Используется существующий изолированный JSON Schema engine и его local ref/dialect/lowering contracts. Implicit network/filesystem resolvers не добавлены.
- Provider SDK fidelity остаётся task42. Не добавлены agent loop, memory, persistence, retries, model catalogs или универсальный application DTO. Live provider conformance не проверялся.

## Исправления после round 2

Общая depth guard и cycle-safe pointer normalization устраняют recursive slice/array/pointer overflow и hang. Field provider + json string option явно unsupported, с рекомендацией полного wire provider для enclosing DTO. Inferred streaming сохраняет item reference scope через один embedded resource и refs на него; полные explicit document schemas остаются caller-owned. Nullable collection elements/map values сохраняются рекурсивным mapping. No-late schemas больше не фильтруют required; неподдерживаемые object projections диагностируются до emission, без универсального DTO/storage. Regressions компилируют emitted consumer с nullable primitives/objects/nested arrays/map values и проверяют no-loss, reject unknown presence/root nullable/name conflicts.

## Исправления после round 3

Round 3: полнота 100% (18/18), audit errors — два открытых P2. Исправлены resource collision при пустом/совпадающем item `$id` и доверие генератора ignored draft-07 `$ref` siblings. Streaming сохраняет canonical item identity, wrapper имеет отдельный absolute identity; compiler выбирает соответствующий retrieval address, сохраняя supplied-resource ownership и нормализованные source diagnostics. Генератор root referenced objects требует caller-owned type независимо от sibling closure. Regressions проверяют item IDs, object/array validation, before-invocation compilation, invalid values, root-ref input/output rejection в обоих dialects и отсутствие implicit IO. Приёмка round 4 выполняется обоими аудиторами заново.

## Финальная приёмка

Оба независимых аудитора приняли один frozen snapshot round 4: `a1337cdb30d32aa3e25b4d2f8d859736814c38bc5a476bfb92cabc9cd23c09f7`, 672 файла. Полнота 100% (18/18), открытых подтверждённых ошибок 0; independent probes и обязательные full lint/test green. Отчёты: completeness-r4.md, errors-r4.md, checks-r4.md. Сравнение рабочей реализации со snapshot перед закрытием подтвердило отсутствие изменений всех frozen files. После приёмки изменены только статусы task/contract/closeout документов. Это результат проведённых проверок, не доказательство отсутствия всех возможных дефектов.
