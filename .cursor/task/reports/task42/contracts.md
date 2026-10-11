# Task42 — conformance contract

База реализации d829ab2. Scope: все существующие adapter result paths; новые provider features из task43 исключены. Spec-first: contract fixture и actual SDK HTTP/SSE/JSONL должны подтверждать данные на реальном IO пути.

## Подтверждённые протоколы и SDK

- google.golang.org/genai v1.48.0: FunctionCall.ID и FunctionResponse.ID связаны native ID; FunctionResponse.Response поддерживает output/error. SDK common.go deepMarshal использует json.Unmarshal в map и теряет точные interface numbers. Уже существующие batchExactContents/encodeFileGeneration восстанавливают exact contents после conversion через ExtrasRequestProvider; этот механизм следует обобщить для generation, streaming и cache content, не писать новый транспорт.
- github.com/ollama/ollama v0.17.4: Message.Thinking отделён от Content. [Официальный tool protocol](https://docs.ollama.com/capabilities/tool-calling) требует вернуть accumulated thinking/content/tool_calls вместе в follow-up. [Thinking protocol](https://docs.ollama.com/capabilities/thinking) выделяет отдельное native поле. Следует явно задать допустимую гранулярность без потери canonical частей.
- [Gemini function protocol](https://ai.google.dev/gemini-api/docs/function-calling), использованные native type contracts сверены с локальным SDK. Не добавлять новые Interactions API и model catalogue в этот scope.

## Требуемые invariants

1. Unsigned и signed args восстанавливаются из наблюдённого JSON до необратимой SDK rounding; source проверяется против SDK DTO с теми же ordinal/shape constraints. Signed source/state binding не ослаблять. Source overflow/incomplete и unknown precision не должны публиковать якобы точные calls.
2. Direct DTO без source: объявить допустимые representations, numeric ambiguity явно error, retaining available partial result. Exact JSON representations и Go integer representations не запрещать. No guessing/repair/retry.
3. Native outbound FunctionCall args должны сохранять значения через финальный SDK serialization во всех существующих dispatch paths; простого UseNumber недостаточно.
4. Gemini FunctionResponse переносит ToolCallID, errors через native error key. Name-only path допускается только при проверенной correlation capability; duplicate/ambiguous mapping отклоняется. Никаких synthetic IDs.
5. Ollama continuation переносит reasoning в Thinking, без ordinary text/logging. Native single-field гранулярность фиксируется и adversarial interleaving проверяется.
6. Проверить IsError и multimodal mappings каждого адаптера. Native error flag/field переносится, непредставимые формы отклоняются доdispatch. Новые multimodal mappings не реализовать; canonical partial results сохранять при parse failures.
7. Sync/stream/batch parse/history/wire conformance и cancellation/race обязательны; невозможный path документируется как unsupported.
8. Core остаётся provider-neutral; policies/transport/handlers/DTO/state storage у host. Clear break, несовместимые версии отвергать.

Статус: реализовано, приёмка ожидается. Точный договор: docs/provider-exchange-contract.md; executable fixtures: exchange_precision/contract, part encoding/signed source, batch_exchange_precision, cache_resources, thinking continuation, tool_error_contract. Checked ID-less history требует подтверждённую capability автоматически. Direct floats во всех containers/pointers/struct fields reject; integers/json.Number допустимы; recursive inspection отслеживает container identity без arbitrary depth limit. Native request SDK intermediate finite range — explicit error. Prefix-only reasoning granularity; text-only IsError unsupported для OpenAI Chat/Ollama; Gemini native error/output и IDs сохраняются.

R4 уточнение harness contract: failed/canceled stream с известным prefix сохраняет evidence/известные finish metadata, но помечает continuation unavailable до terminal capture/status. Parse failure внутри SSE event сохраняет decoded prefix и только associations существующих частей; no fabricated finish. Source numeric rounding допустима только на соответствующем SDK float leaf, exact caller fields проверяются независимо.
