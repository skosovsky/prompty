# Task39 — closeout и migration

База: `b8bd56c`. Реализация breaking; независимая приёмка round2 завершена: полнота100%, open confirmed errors0.

## Изменённые контракты

Execution хранит private snapshot ToolScope, accessor возвращает clone, WithToolScope возвращает validated copy. Scope nil и explicit empty Allowed различаются. Каждый definition проверяется после runtime/template overrides: name/duplicates, exact JSON schema/dialect и capabilities. Required/forced definitions должны существовать, runtime Required также присутствует в execution. Scope сохраняется всеми execution clone/message/history paths и wire restore. Checked adapter ValidateExecution, materialization, one-shot, serialization и restore повторяют boundary validation. Model получает отдельный clone, не retained handler boundary.

ToolManifest/ToolRequirement имеют Dialect; unset нормализуется в текущий default draft2020 для сравнения definitions. Required requirements могут задавать subset capabilities; definition schema не является wildcard. Framework не принимает IAM решений и не объединяет hosted tools с local registry.

ToolHandler, constructor callback, TypedTool.Invoke и ToolInvoker.InvokeTool принимают context.Context. Registry не сохраняет context, concurrent invokes допустимы после завершения регистрации; Definitions возвращает deep clone. Весь batch валидируется до handlers, context проверяется до model/handlers и после handlers. Операции синхронные и кооперативные.

ToolDispatchError сохраняет cloned known Response при неуспешном model/post-dispatch result; handlers не исполняются. ToolCallError сохраняет known Response при batch-validation error. Handler result+error сохраняется как result message плюс joined cause; cancellation не стирает completed result и не запускает следующий handler. Internal started flag предотвращает synthetic zero result, если direct registry вызов отменён до handler. Retained results не означают rollback; IsError отражает invocation/cancellation error.

## Wire и migration

ExecutionWireFormat теперь `prompty.execution.v2`; bundled executable schema содержит optional tool_scope с Allowed/Required arrays. Старый marker отвергается без compatibility loader. Обновить записи/consumers, использующие прежний execution wire; recipe checkpoint format не меняется, потому что его runtime scope по-прежнему явно rebind через BindRuntime. Private scope не является обещанием для произвольного json.Marshal(PromptExecution): persistence contract — designated execution wire API.

Обновить handlers на func(context.Context,Args)(Result,error), ToolHandler.Handle(ctx,args), прямые Invoke/InvokeTool calls. Перенести context в HTTP/DB клиента. Для required tools добавить execution definitions, не полагаться на required names без tools. При reconciliation читать returned execution и ToolDispatchError.Response/ToolCallError.Response; errors.Is работает для исходной причины и cancellation. Удалены прежние no-context signatures, silent missing-definition path и unreachable name-only required fallback. Background compatibility adapters и goroutine cancellation не добавлены.

## Evidence и границы

Public regressions: unauthorized template без RequiredTools; explicit-empty/duplicate/missing/different schema scope; exact large const, scope clone/getter/message/wire isolation, changed-definition before-model/checked dispatch zero counts, required+forced+allowed positive execution with broader registry, adversarial model mutation, cancellation before call, last handler result+error, HTTP deadline/cancel propagation, no synthetic result, model response+error clone and zero handlers, own contexts concurrent registry. Existing batch-invalid/correlation/schema tests сохранены. Все in-repo constructors/callers и raw wire fixtures обновлены.

Обязательные проверки и точный snapshot identity фиксируются в checks-r2.md после завершения final runs. Никаких live-provider assertions: adapter SDK новых features не менялись. Host владеет auth, retries/compensation, parallel scheduling, storage и agent loop. Нет автоматического повторного вызова или rollback.

## Исправления после round 1

R1 полнота94,44%(17/18), C3partial; errors2classes (P1otherhelpersdispatch,P2custominvokercancellationstatus). Дополнительно полнота воспроизвела compiler source alias mutation afterscopecheck. Tool/output runtime compiler boundaries получают cloned documents и reject detected constraint mutation; Tool helper повторно checks scope после compilation. Root text/structured sync/stream/stateful paths, context-window и tracing wrappers сами проверяют scope перед BYOT next/Invoker dispatch. Stream precancel short-circuit теперь возвращает error без создания downstreamhandle, с нулём source calls; existing lifecycle fixture обновлена на этот before-dispatch outcome. Custom ToolInvoker cancellation присоединяется к handler error до result-message status, без потери результата. Public regressions воспроизводят оба errorclasses и compilerprobe; все старые tests/ports остаются отдельными contract gates.

## Финальная приёмка

Оба независимых аудитора приняли один frozen snapshot round2: `41748a679acbf36a40cdfb6a3b446e407379de33eb9a4f27f5f0dc5f809d84ed`, 685files. Полнота100%(18/18), открытых подтверждённых ошибок0. Аудитор ошибок независимо выполнил full make lint (12modules) и make test (allmodules/race/gen_out), оба exit0; его additional untouched r1 probes и compiler/result/ctx adversarial probes passed. Рабочие frozen files перед final closeout сравнили по SHA: delta0. После приёмки меняются только acceptance statuses/report metadata.
