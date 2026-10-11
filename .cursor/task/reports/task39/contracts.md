# Task39 — контракт реализации

База: b8bd56c. Clear break. Принято round2: полнота100%(18/18), open confirmed errors0, full lint/test/race green.

- Runtime scope: nil означает отсутствие дополнительной границы, present empty Allowed запрещает все local tools. Execution хранит private snapshot; accessor возвращает clone, WithToolScope возвращает новый execution. Clone/message additions/wire сохраняют scope. Scope — caller declaration, не IAM.
- ValidateExecutionToolScope проверяет names/duplicates, required/forced definitions и каждый definition против explicit Allowed (точные schema documents, normalized dialect, capability inclusion). Runtime required tools обязаны присутствовать в execution. Проверки при materialization, checked adapter preflight, serialization/restore и one-shot до model и handler. Explicit caller-built unscoped execution поддержан.
- ToolManifest и ToolRequirement получают Dialect для согласованного сравнения. Required manifest requirements — subset capabilities; definition schema/dialect должны совпадать с Allowed, без nil wildcard. Hosted provider tools не используют local registry.
- Execution wire format: prompty.execution.v2, optional tool_scope с required Allowed/Required arrays. Старый prompty.execution явно отвергается. Recipe checkpoint продолжает требовать явную runtime scope при BindRuntime; его schema не меняется, потому что runtime binding не persist.
- ToolHandler.Handle, function constructor, TypedTool.Invoke, ToolInvoker.InvokeTool, registry и private invocation принимают context.Context. Нет adapters Background или goroutine cancellation. Context проверяется до model и handler, после handler. Регистрация завершена до concurrent invocation; вызовы не сохраняют context в registry.
- Все batch calls проходят schema/scope/correlation и registry validation до первого handler. Неуспешный model response не запускает handlers. ToolDispatchError хранит cloned known Response и исходную/ context причину; returned execution — snapshot до принятия такого response.
- TResult/raw result вместе с handler error сохраняется. Tool result message содержит известный result, IsError отмечает ошибку invocation, включая cancellation; это не утверждение об откате side effect; при отсутствии result — error text. Cancellation после последнего handler возвращает context error вместе с известным result; errors.Join сохраняет предыдущие причины. Следующий handler не стартует. Cancellation не rollback.

Приёмка: весь task39 + общий README, два независимых аудитора после full lint/test. Никаких retries, scheduling, approvals, storage, agent loop или provider SDK новых features.

## Уточнения после round 1

- Round1 отклонён: completeness94,44%17/18 и два error classes. Compiler port получает private source document; изменение constraint graph при successful compile диагностируется как ErrSchemaUnsupported до model. Scope дополнительно rechecks после compilation. Read-only source requirement соответствует исходному port обещанию enforce selected schema.
- Bound scope enforced независимо от checked adapter на root text/structured sync/stream/stateful helpers, context budget и tracing wrappers. Context cancellation до stream port не создаёт downstream handle/source; iterator возвращает context cause.
- После custom ToolInvoker handler cancellation присоединяется к invocation error до построения result message: IsError и joined cause согласованы с registry path. Known result остаётся доступным.
