# Task42 — independent errors audit r4

Decision: **ACCEPTED**. Открытых подтверждённых ошибок: **0**. Это результат выполненных проверок, не абсолютная гарантия отсутствия дефектов.

## Identity and independence

Frozen `/tmp/prompty-task42-snapshot-04`, implementation base `d829ab2`, 734 файла. SHA256 manifest `90a5514d872339f18618766a75f4d3cf361882a172cb4aa99abc93d7c7e1dfca` совпал. Все hashes проверены до и после: `/tmp/task42-errors-r4-integrity.log`, `/tmp/task42-errors-r4-integrity-final.log`. Прочитаны task42, общий README заданий, contracts, closeout, provider exchange contract и актуальный код. Peer r4 report не читала; в implementation не участвовала. Frozen/working source не менялись. Checks/probes выполнены в отдельных копиях `/tmp/prompty-task42-errors-checks-r4` и `/tmp/prompty-task42-errors-probes-r4`.

## Independent evidence

- Полный serial `make lint`: exit 0, все 12 модулей, `/tmp/task42-errors-r4-lint.log`.
- Полный `make test`: exit 0, все модули/race/generated consumers, `/tmp/task42-errors-r4-test.log`.
- Собственные Gemini adversarial probes с race: exit 0, `/tmp/task42-errors-r4-gemini-probes.log`.
- Собственные Ollama actual HTTP/adversarial probes с race: exit 0, `/tmp/task42-errors-r4-ollama-probes.log`.
- Дополнительный Gemini multi-event signed state + exact call + failed part/history association probe с race: exit 0, `/tmp/task42-errors-r4-state-probe.log`.

Caches: GOCACHE=/tmp/prompty-go-cache, GOLANGCI_LINT_CACHE=/tmp/prompty-lint-cache. Live calls не выполнялись.

## Previous confirmed classes

**R1 malformed HTTP arguments and known prefix**: actual Ollama HTTP fixture сохраняет thinking/text и preceding known calls в checked sync/buffered/native; malformed call не публикуется. Execute→ParseResponse phase pair возвращает известный prefix с error, unknown outcome и unavailable. HTTP consumer stop сохраняет cancellation и malformed error причины. Duplicate/nested/escaped argument keys явно reject до map conversion. Повторены собственные r1/r2 probes.

**R2 late malformed []args without own prefix**: actual HTTP frame1 reasoning/text/valid call, frame2 malformed arguments=[] без текста. Сохранены известные части, Outcome empty, unavailable=true. Предыдущий falling probe теперь pass.

**R3 F1 pre-callback/EOF/cancel continuation gap**: independently повторены actual Ollama HTTP EOF, error frame, malformed whole JSON, wrong message type; Gemini actual SDK SSE EOF/candidates:null. Nonempty known prefix остаётся доступным для inspection, unavailable=true, AssistantMessage сохраняет flag. Next checked Execute отвергает историю без второго HTTP dispatch (counter остаётся 1). Consumer break actual HTTP fixture также unavailable=true. Universal Stream closeout и revoke before capture inspection проверены в коде; full suite подтверждает assembly/capture/finalizer/cancellation lifecycle. Observed finish metadata не подменяется успешным завершением жизненного цикла.

**R3 F2 same-event Gemini prefix loss**: independent actual SSE parts=[text known, unsupported functionResponse] теперь сохраняет text, error, no successful finish и unavailable=true. Next dispatch отсутствует. Дополнительный multi-event probe: signed reasoning в первом event, text+exact call n=9007199254740993+unsupported result во втором. Сохранены все 3 known parts, exact arguments, одна signed state association; ValidatePartAssociations и AssistantMessage проходят. Next checked request не отправляется.

## Entire task42 scope

1. **Exact numeric IO/source binding.** Reviewed common exact native contents restoration, generation/native/inline and JSONL batch/cache paths, empty args {}, SDK finite intermediate range refusal. Full actual SDK HTTP/SSE/JSONL fixtures проходят. Собственные direct typed graphs проверяют slices/maps/pointers/struct float rejection, Go integers/json.Number acceptance и сохранность partial known text. Per-leaf source probes signed/unsigned подтверждают допустимое округление только соответствующего float, независимое rejection конфликтующего exact integer/native ID/name/signature. Cyclic/unserializable shapes explicitly fail; source comparisons не основаны на глобальном допуске float.
2. **Signed/partial state.** Reviewed ordinal/signature/destination binding, partial-call normalization, provenance associations, native continuation encoding и incompatible wire/state отказ. Repeated full race suite и собственный signed failed-prefix probe подтверждают сохранность associations. Rejected later parts не получают canonical state/annotation associations.
3. **Correlation and capability.** Reviewed FunctionResponse.ID mapping, repeated-name parallel calls, native output/error selection, duplicate/unknown/changed-name result rejection, unique ID-less path и автоматическое checked name-correlation requirement+adapter mapping port. Existing actual request/wire/history and adversarial profile fixtures проходят; IDs не генерируются.
4. **Reasoning/results.** Ollama Thinking отделён от ordinary text; contiguous reasoning prefix поддержан, interleaving явно reject. IsError сохраняется native Gemini/Anthropic либо явно reject OpenAI Chat/Responses/Ollama; existing unsupported multimodal forms fail beforedispatch. Thinking HTTP→canonical→wire→next request fixture и tool error fixtures проходят.
5. **Lifecycle and integrity.** Проверены sync/native/buffered partial failures, unknown completion, next-history admission, cancellation/consumer stop/concurrency/race и capture lifecycle. Нет automatic retry/fallback/handler execution для ошибочного результата. Полный suite содержит HTTP deadline/body-close, observer/finalizer и assembly failure checks.
6. **Library boundaries.** Provider mapping/state/encoding остаются в adapter modules; общий numeric reflection/helper и универсальный lifecycle не вводят vendor SDK types в core. DTO/transport/host callbacks, live capability evidence, loop/storage/retries/policies остаются у host. Explicit low-level versus checked admission граница документирована; raw SDK final serialization outside adapter-owned hook host-owned.

## Explicit existing limitations

Response persistence wire отвергает unknown Outcome явно; failure prefix доступен через Result/AssistantMessage с unavailable flag. В дополнительном probe попытка восстановить unknown-outcome ResponseWire получила ErrResponseOutcome — это existing explicit wire contract, а не lossless успешный wire путь. Probe скорректирован проверять явный отказ и доступную history projection; implementation не менялась. Successful response wire/history сохранность проверена actual fixtures. Никакой successful Outcome для ошибочного prefix ради wire не фабрикуется.

Finite-range SDK intermediate и custom JSON projections без удостоверяемого source binding explicitly unsupported. Новые provider features/multimodal mappings относятся к task43. Локальные fixtures не являются live model conformance.

Новых подтверждённых ошибок в полном просмотренном scope и выполненных adversarial scenarios не обнаружено. Оба r3 blockers устранены по независимым воспроизведениям. Приёмка r4 по ошибкам положительная.
