@skosovsky, реализован контракт полной host-композиции и опубликован **v0.16.0** ([код релиза](https://github.com/skosovsky/prompty/tree/v0.16.0)).

Обновите core и используемый provider adapter:

```sh
go get github.com/skosovsky/prompty@v0.16.0 github.com/skosovsky/prompty/adapter/openai@v0.16.0
```

Что изменить в вызывающем коде:

1. **Простой JSON one-shot:** можно оставить `ExecuteWithTypedTools` и существующую сигнатуру `InvokeTool(ctx, name, argsJSON)`. Native ID в handler не передаётся. Обычная handler error сохраняет исходную причину и продолжает admitted batch; cancellation останавливает следующие вызовы. Approval/pause нельзя трактовать как остановку этого helper. Ответ с недоступным continuation теперь отвергается до effects; обработайте эту ошибку вместо попытки исполнить частичный ответ.
2. **Native ID, approval и richer outcomes:** используйте checked `Invoker.Execute`, затем новый pure `AdmitToolCalls(ctx, execution, response, validator)` и свой dispatcher. Проверяйте `Execute` error до admission. Передавайте исполнителю `call.ID`, `call.Name` и исходные `call.Args`; храните completed outcomes и pending cursor у host. После host approval продолжайте именно pending ID, без повторного исполнения завершённых effects. Все calls проверяются против original schemas до первого effect; provider-lowered schema не заменяет их. Validator выполняет только проверку.
До перехода ID/approval исполнение запускалось через `ExecuteWithTypedTools`. После перехода admission отделён от effects:

```go
response, err := invoker.Execute(ctx, execution)
if err != nil {
    return err
}
calls, err := prompty.AdmitToolCalls(ctx, execution, response, validator)
if err != nil {
    return err
}
// Host dispatcher получает calls с ID/name/raw Args;
// host хранит курсор, принимает approval и проецирует outcomes.
```

3. **Business error при nil Go error:** проверяйте outcome отдельно. Преобразуйте её в безопасный `IsError` result только там, где provider mapping поддерживает error flag. OpenAI Chat такой result отвергает до HTTP dispatch. Controls/effects и private audience не отправляйте модели; binary, unknown MIME и неподдерживаемые mappings должны завершаться явной ошибкой.
4. **История и persistence:** замените сборку истории из `response.TextContent()` на полный assistant message. Например:

```go
assistant, err := response.AssistantMessage()
if err != nil {
    return err
}
next := execution.AddMessage(assistant)
```

Сохраняйте response через `MarshalResponse` с `WirePolicy{State: StatePreserve}`; перед replay проверяйте destination/scope/expiry и required state. Logging/state-omitted checkpoint не годится для required continuation. Для semantic context persistence используйте зарегистрированный host codec: raw аргументы должны сохраняться отдельным sidecar и сверяться с semantic JSON при восстановлении. Rich state/annotations сохраняются opaque либо отвергаются явно.
5. **Stream cleanup:** `Stream.Close()` только запрашивает cancellation и остаётся callback-safe/nonblocking. Подтверждайте cleanup отдельным completion signal после выхода владельца из consumption; callback не должен ждать собственный unwind. Cancellation не доказывает rollback remote effects.

[Runnable offline example и команды матрицы](https://github.com/skosovsky/prompty/blob/v0.16.0/examples/host_dispatch/README.md), [контракт и mapping-таблица](https://github.com/skosovsky/prompty/blob/v0.16.0/docs/host-tool-dispatch.md). Example использует настоящие публичные consumer API, provider SDK и локальный HTTP fixture; credentials не нужны. Core не получил обязательных зависимостей на consumers, approval runtime или общих доменных DTO.

Проверки: `make lint`, `make test`, `make release-check`, `make release-break`; actual-current-local и clean-pinned-source suites с `-race -count=1`, lint и demo; опубликованный набор с `GOWORK=off` без replace прошёл semantic suite/lint/demo. Два независимых отчёта: полнота **100%**, открытых подтверждённых ошибок **нет**. Теги релиза проверены по remote refs. Source CI фиксирует release commits consumers; их default branches сейчас старее нужных контрактов, и успешная проверка HEAD не заявляется.

Ограничения: локальные fixtures не доказывают live provider support, production IAM/durable storage, подписи или remote rollback; hosted CI run не заявляется. Evidence сохранён в `.cursor/task/reports/task46/`.
