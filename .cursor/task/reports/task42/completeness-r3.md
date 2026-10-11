# Task42 — независимая приёмка полноты r3

**Решение: REJECT. Полностью выполнено 17/18 = 94.44%.** Частично выполненное C7 не засчитано. Это результат независимого аудита, не математическая гарантия отсутствия других дефектов.

## Объект и метод

Проверен только frozen `/tmp/prompty-task42-snapshot-03`, база `d829ab2`. SHA256 manifest identity: `64025d9711139051bbc03aa66d19373a18264227ab769e74acb564eefcbe01b4`. Проверены все 730 file hashes до и после аудита; logs `/tmp/task42-integrity-r3-before.log`, `/tmp/task42-integrity-r3-after.log`. Frozen и рабочая реализация не изменялись. Probes и тестирование — отдельная копия `/tmp/prompty-task42-audit-r3`. Текущий отчёт второго аудитора не читался; реализация и делегирование не выполнялись.

Основания: весь task42.md (8 требований + 9 acceptance), общий task README, contracts.md, docs/provider-exchange-contract.md, closeout и checks-r3. Denominator сохранён 18: C1–C8, A1–A9, M1. Не заменяла требования чекбоксами и не считала документированное ограничение автоматическим исполнением.

## Матрица

| ID | Обязательное требование | Код и проверяемые evidence | Статус |
|---|---|---|---|
| C1 | Exact numeric arguments во всех signed/unsigned IO путях, включая финальную SDK сериализацию | Gemini response_source.go + argument_binding.go восстанавливают source до потерь; exact_contents.go, part_encoding.go восстанавливают outgoing contents после SDK conversion. Actual HTTP/SSE exchange_precision, batch_exchange_precision, cache_resources тестируют большой integer, дробь/exponent, nested values, пустой объект. | complete |
| C2 | Direct SDK DTO без source: допустимые представления и explicit reject unknown precision, без blanket reject чисел | Shared adapter/numeric_source.go рекурсивно проверяет floats, pointers, structs, arrays/maps, container identity. Integers/json.Number допускаются; float ambiguity/cycles explicitly fail и сохраняют известный prefix. Gemini exchange_contract; Ollama tool number/unknown precision tests. | complete |
| C3 | Native FunctionResponse ID и repeated-name correlation; ID-less только подтверждённый name protocol | Gemini correlation.go, toolResultContent передают ToolCallID в native ID и проверяют duplicate/ambiguous history. Adapter preflight автоматически требует name-correlation profile для ID-less history. Actual SDK fixture проверяет reversed results двух lookup IDs и confirmed/unknown capability. | complete |
| C4 | Ollama Thinking continuation и явная представимая гранулярность | ollamaThinking переводит contiguous reasoning prefix отдельно от Content; interleaved reasoning после ordinary parts explicit reject. ThinkingWireContinuationActualHTTP покрывает sync/buffered/native, wire и следующий HTTP request; interleaving tests. | complete |
| C5 | IsError и существующие multimodal result mappings сохраняют смысл или явно reject | Gemini native error/output и Anthropic is_error сохраняются; OpenAI Chat/Responses и Ollama отвергают непредставимый IsError. Existing content switches отвергают неподдержанные формы до dispatch. tool_error_contract и существующие multimodal tests; task43 features не добавлены. | complete |
| C6 | Signed ordinal/signature/scope, partial completion, unknown outcome, original schema не ослаблены | Gemini source/state/partial codecs проверяют ordinal, canonical fields, signature и endpoint/model scope; provisional incremental args не становятся completed до подтверждения. Новое per-leaf source binding разрешает rounding только конкретного SDK float leaf, независимые fields сравниваются отдельно. Signed fixtures, original schema tests, собственный mixed exact/source probe r1 повторён успешно. | complete |
| C7 | Эквивалентные гарантии во всех существующих sync/native/batch/history paths, реальные IO fixtures | Основные sync/native/batch/cached/history fixtures проходят. R3 callback fix сохраняет unavailable при malformed arguments, включая поздний frame. Однако whole-frame decode/read error до callback оставляет известный непустой prefix с ContinuationUnavailable=false; его AssistantMessage успешно создаётся. Незавершённый exchange остаётся replayable. См. blocker ниже. | **partial** |
| C8 | Provider-neutral core, BYOT/host boundaries, incompatible formats rejected, no legacy repair | Core не получил SDK/provider number types; generic continuation flag/JSON remain neutral. Host выбирает DTO, handlers/endpoint/transport/policies. Existing wire/state version validation rejects incompatible formats; нет loop/retry/catalog/state storage. Checked admission отделён от low-level DTO conversion. | complete |
| A1 | Unsigned HTTP 9007199254740993 exact args и typed int64 | TestUnsignedExchangeActualSDKPreservesExactArgumentsAndIDs проверяет actual HTTP/SSE, typed NewTypedTool int64 получает exact value и native call ID. | complete |
| A2 | Следующий actual SDK request сохраняет nested/fraction/exponent/negative/{} | Тот же exchange fixture, SignedNumbersAreExactOnSDKWireOrRejectedBeforeDispatch, cache resource и batch outgoing fixtures проверяют фактическое HTTP body. Finite SDK bounds дают explicit error до IO, не округление. | complete |
| A3 | Equivalent sync/native completed arguments и применимые batch paths | Actual unsigned exchange modes + TestBatchInlineAndJSONLUnsignedArgumentsAreExact (inline and downloaded JSONL); native completed calls exact. Не существующие provider batch paths явно unsupported. | complete |
| A4 | Wire → AssistantMessage → next request native IDs и parallel repeated-name results | Actual fixture round-trip через state wire, reversed second error/first success results, native FunctionResponse IDs проверяются в HTTP body. | complete |
| A5 | Name-only explicit supported; malformed/duplicate/ambiguous rejected | correlation tests и TestIDlessHistoryRequiresConfirmedNameCorrelation с capability supported/unsupported/unknown; no fake ID or name-as-ID. | complete |
| A6 | Ollama thinking canonical/wire/nextrequest отдельно от ordinary text, no automatic logging | TestThinkingWireContinuationActualHTTP во всех modes; parser и outgoing Thinking отдельны, adversarial interleaving rejected, automatic prompt/thinking logging отсутствует. | complete |
| A7 | Signed/partial порядок и signatures; altered/missing required state rejected | Gemini signed state/ordinal/scope/encoding/source tests и actual signed numeric/empty-object fixtures; partial native sequences remain provisional. | complete |
| A8 | Unknown precision/unsupported result semantics explicit error с partial result, no retry | Direct float/nested/cyclic representation probes и HTTP malformed-arguments prefix fixtures сохраняют available thinking/text/valid calls; unsupported IsError/multimodal explicitly fail. Автоматического retry нет. Whole-JSON failure относится к C7, не к unknown numeric precision/result semantics. | complete |
| A9 | Actual SDK fixtures исходных дефектов, race/cancellation conformance | HTTP/SSE/JSONL actual SDK tests описанных исходных numeric/ID/thinking defects и race/cancellation tests проходят. Проверки не являются доказательством C7 для всех необработанных transport frames; отдельный adversarial probe выявил gap. | complete |
| M1 | README mandatory closeout: contract/migration/legacy removal/checks/limits | contracts, docs/provider-exchange-contract, closeout/checks фиксируют direct DTO representations, checked vs low-level, SDK finite bounds, incompatible state, migration и boundary decisions. Все 12 modules lint/test и generated consumers представлены author logs; audit decision не наследуется от них. | complete |

## Подтверждённый blocker C7

**Поздняя ошибка всего NDJSON frame обходит отзыв continuation.** В `adapter/ollama/chat_http.go:76–81` ошибка decode, не содержащая partialChatDecodeError, возвращается без consume callback. Новый r3 guard `adapter/ollama/stream.go:52–56` поэтому не вызывается. Generic `stream.go:219–238` закрывает stream с ошибкой, но не отзывает continuation у retained result. Это применимо также к read/transport errors до callback; воспроизведение ниже подтверждает именно malformed whole JSON.

Самостоятельный actual HTTP probe: `/tmp/prompty-task42-audit-r3/adapter/ollama/task42_audit_wholejson_test.go`, test `TestAuditMalformedWholeJSONDisablesContinuation`. Server возвращает:

```ndjson
{"message":{"role":"assistant","thinking":"known reasoning","content":"known text","tool_calls":[{"id":"valid","function":{"name":"lookup","arguments":{}}}]},"done":false}
{"message":
```

Checked adapter native stream после consumption возвращает error, retained Content из reasoning/text/valid call, Outcome и FinishReason пустые, **ContinuationUnavailable=false**. `status.Result.AssistantMessage()` возвращает nil error. Probe ожидает unavailable=true и падает. Не требуется удалять known prefix: требуется сохранить его с запретом continuation, как при malformed argument frame.

Команда (из audit copy):

```sh
GOCACHE=/tmp/prompty-go-cache go test -race ./adapter/ollama -run TestAuditMalformedWholeJSONDisablesContinuation -v
```

Exit 1; полный log `/tmp/task42-completeness-wholejson-r3.log`. Исправления working tree не изучались и не учитывались при оценке frozen03.

## Проверки

- Независимый `go test -race ./... ./adapter/gemini/... ./adapter/ollama/... ./adapter/openai/... ./adapter/anthropic/... ./cmd/prompty-gen/testdata/gen_out` из audit copy: exit 0; `/tmp/task42-completeness-independent-r3.log`.
- Исходный independent mixed exact/source Gemini r1 probe повторён без изменения: unsigned и signed обе формы явно rejected, exit 0; `/tmp/task42-completeness-gemini-probe-r3.log`.
- Исходный independent поздний malformed-arguments r2 probe повторён без изменения: known three parts retained, unavailable=true; public TestLaterMalformedHTTPFrameDisablesContinuation дополнительно доказывает marker in history и reject следующего checked request. Same-frame duplicate/escaped duplicate malformed args также проходят; `/tmp/task42-completeness-ollama-probe-r3.log`, exit 0.
- Новый whole-JSON probe: exit 1, подтверждённый C7 blocker.
- Author make lint и make test all12modules/race/generated: `/tmp/task42-lint-final-r3b.log`, `/tmp/task42-test-final-r3b.log`, exit 0 по завершённым checks. Локальные проверки audit дополняют эти logs.
- Все 730 frozen hashes unchanged before/after. Читала полный scope code/contracts; documented finite SDK numeric limits и host-owned raw SDK pipeline не использованы как blanket excuse для отсутствующих guarantees.

Повторная приёмка нужна на новом immutable snapshot после устранения C7. Процент вычислен по полностью выполненным обязательным требованиям, partial не засчитан.
