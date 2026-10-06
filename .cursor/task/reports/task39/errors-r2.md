# Task39 — независимая приёмка ошибок, round 2

**Решение: принято по проверке ошибок. Открытых подтверждённых ошибок: 0.** Это результат выполненной проверки frozen версии, не абсолютная гарантия отсутствия любых дефектов. В реализации не участвовала; текущий completeness-r2 другого аудитора не читала.

## Identity и проверки

Frozen `/tmp/prompty-task39-snapshot-02`, база `b8bd56c`. SHA256 manifest `41748a679acbf36a40cdfb6a3b446e407379de33eb9a4f27f5f0dc5f809d84ed` совпал. Все **685 файлов** проверены до аудита и повторно после проверок: `/tmp/task39-errors-r2-integrity.log`, `/tmp/task39-errors-r2-integrity-final.log`. Frozen и рабочая implementation не изменялись; все дополнительные probes только в `/tmp/prompty-task39-errors-probes-r2`.

Прочитаны task39, общий README задач, публичный `docs/tool-execution-contract.md`, closeout и checks-r2. Проверка охватывает весь scope задачи, включая unchanged contracts и public boundaries, а не только исправленные строки.

Независимо выполнены на frozen:

- `GOCACHE=/tmp/prompty-go-cache GOLANGCI_LINT_CACHE=/tmp/prompty-lint-cache make lint`: **exit 0**, все 12 modules, zero issues. Лог `/tmp/task39-errors-r2-lint.log`.
- `GOCACHE=/tmp/prompty-go-cache make test`: **exit 0**, все modules с race и generated consumers. Лог `/tmp/task39-errors-r2-test.log`.
- В separate copy первоначальные четыре adversarial r1 tests: **exit 0**. Лог `/tmp/task39-errors-r2-probes.log`.
- Расширенные probes с race: `go test -race -run 'TestAudit|TestToolCompilerSourceMutationRejectedBeforeModel|TestCoreHelpersRecheckBoundScopeWithBYOTInvoker' -v .`: **exit 0**. Лог `/tmp/task39-errors-r2-probes-expanded.log`. Probe source `/tmp/prompty-task39-errors-probes-r2/audit_task39_r1_test.go`.

Live provider calls не выполнялись; локальные adapter suites не выдаются за live conformance.

## Закрытие подтверждённых r1 findings

**R1 F1 P1 — direct helper scope bypass: закрыт.** Прежние самостоятельные воспроизведения повторены без изменения assertions: structured sync и native stream с заменой разрешённого `read` на `delete_all`, text helper после удаления runtime Required `read`. Теперь все дают `ErrToolScope` и ноль model calls. Проверены public regression сценарии всех пяти helper paths: text, structured, stateful structured, native stream и buffered stream; renamed и missing-required варианты. `prepareStructuredExecution` проверяет cloned execution, перед model dispatch выполняется `validateDispatchBoundary`, model получает изолированный clone. Text helper также проверяет сохранённый scope до своей text-format проверки.

**R1 F2 P2 — custom ToolInvoker cancellation IsError: закрыт.** Первоначальный probe возвращает известный `"written"` и nil error после отмены context. Теперь helper возвращает context cause, сохраняет данные с `IsError=true`, второй handler не стартует. Дополнительный независимый probe возвращает известный JSON result вместе с исходной ошибкой и отменой: обе причины доступны через `errors.Is`, вызван только первый handler, retained result сохранён и помечен ошибкой. Post-handler cancellation присоединяется до формирования ToolResultPart.

## Дополнительные adversarial проверки и полный scope

**Compiler/source boundaries.** Tool compiler mutation regression использует настоящий стандартный engine на изменённых bytes, а не permissive fake validator; ошибка обнаруживается до model. Дополнительный собственный output compiler probe повторяет mutation для sync structured и native/buffered stream: `ErrSchemaUnsupported`, zero calls, caller ResponseFormat source bytes неизменны. `compileSchemaSnapshot` даёт compiler изолированные bytes и проверяет semantic contract change при успешной compilation; tool helper повторно проверяет scope после compilation. Доверие произвольному SchemaValidator как исполняющему port не превращается в новый IAM/backend contract.

**Scope и persistence.** Повторно проверены distinction absent/explicit-empty scope, all definition names/duplicates, required и forced existence, exact JSON schema identity, normalized dialect и capabilities, post-override materialization, private scope getter cloning, execution/message/history cloning и runtime rebinding recipe checkpoint. Execution wire v2 сохраняет scope, validates before serialization/after restore, rejects old marker; executable schema и точные большие numeric constants покрыты suite. Untrusted wire scope остаётся caller declaration: host отвечает за auth/rebinding. Registry не расширяет разрешённые execution tools; hosted provider tools не становятся local handlers.

**Actual delegation boundaries.** Checked adapter scope validation и core typed-tool preflight проверены вместе с standalone BYOT helpers. Context-window и tracing wrappers сами reject changed scope до custom next в sync/stream paths; их public tests вошли в независимый full run. Context-window делает isolated WithMessages copy; tracing rejection сохраняет observed stream lifecycle. Stateful structured delegation использует сохранённый cloned scope. Precanceled structured stream проверяет context до создания downstream handle/source; native и buffered paths сохраняют заявленные stream contracts.

**Cancellation и data retention.** Проверены публичные context signatures handler/constructor/Invoke/InvokeTool, cooperative HTTP deadline/cancel fixtures, registry concurrent calls с отдельными contexts и race, before-model/before-each-handler/post-handler checks. Context не хранится registry, detached cancellation goroutines не добавлены. Complete batch schema/scope/correlation validation завершается до первого handler; original model boundary защищён clone от mutation. Known model response+transport/context error сохраняется clone в ToolDispatchError, handlers не исполняются; batch-invalid known response также сохранён. Known handler result+error+cancel не теряется, no synthetic zero result before started handler, no retries/rollback/agent loop. Registry Definitions deep-clone и broader registry positive path проходят.

## Вывод

Оба подтверждённых класса r1 закрыты выполненными воспроизведениями. Compiler mutation correction и wrapper boundaries прошли code review и suites; новых подтверждённых errors/regressions не найдено. С учётом task39/README contract и универсальных BYOT границ ошибок, блокирующих принятие этой frozen версии, не обнаружено.
