# Task42 — implementation closeout

Статус: принято r4, 18/18 (100%), 0 открытых подтверждённых ошибок; green lint/test/race. База d829ab2.

Gemini восстанавливает exact source parts для unsigned/signed calls одинаково; direct SDK numeric floats без исходника отвергаются рекурсивно (typed maps/arrays/pointers/structs тоже), malformed/cyclic JSON не вызывает recursive panic. Go integers/json.Number сохраняются без произвольного depth policy. SDK нормализация остаётся intermediate step с явной finite-range ошибкой; exact source DTO contents восстанавливаются после conversion через existing ExtrasRequestProvider подход.

Общий native encoding используется в sync/native generation, inline/file batch submission и cache creation. Explicit empty args object не теряется из-за SDK omitempty; signed пустой объект проходит original/state/wire/continuation без смены signature/ordinal/scope. Cache content digest исключает dispatch callback/options. Source numbers, empty object, nested arrays, fractions/exponents/negative large numbers проверены actual HTTP/SSE/JSONL fixtures.

Gemini FunctionResponse переносит native call ID; output/error ключ соответствует IsError. Repeated-name IDs сохраняются в wire/history и actual SDK request. Correlation validator отвергает duplicate IDs/results, unknown/changed-name results в supplied history, repeated name-only и mixed-ID ambiguity. Checked preflight выводит name-correlation requirement из ID-less history и проверяет profile и mapping port; low-level protocol/profile guarantees документированы отдельно. ID не выдумывается.

Ollama Thinking сохраняется из contiguous reasoning prefix, text отделён; непредставимый interleaving explicitly unsupported. Actual sync/buffered/native HTTP -> canonical -> wire -> request fixture сохраняет thinking. OpenAI Chat/Ollama IsError explicitly unsupported до dispatch; OpenAI Responses уже отвергает IsError; Anthropic native is_error сохранён; Gemini native error/output реализован. Existing unsupported multimodal result shapes продолжают отказ вместо lossy conversion; новых mappings не добавлено.

Общий adapter.HasUnverifiedJSONNumbers проверяет Go numeric representations без provider-specific core types; Gemini/Ollama используют его вместо узкого map[string]any-only обхода. Parse failure после accepted prefix возвращает prefix с error, unknown finish/outcome и unavailable continuation; библиотека не запускает retry/handler admission для такой ошибки. Existing signed/partial sequence validation, cancellation/race/scope/schema contracts сохранены. Никаких provider-specific core numeric/state типов и новых agent loop/storage/policy frameworks.

Документация: docs/provider-exchange-contract.md и README. Breaking migration: подтверждённая name correlation для ID-less history, IsError без native mapping отвергается, SDK округлённые signed args больше не блокируются когда exact encoding гарантирован; старые неподдержанные codec/wire формы не восстанавливаются.

Ollama sync/native callback SDK DTO с typed nested float/cycle сохраняет известные thinking/text, возвращает ошибку и не публикует finish. Prefix response явно сбрасывает default completed outcome конструктора.

R4 closeout: exact source binding выполняется per-leaf; float не разрешает конфликт соседнего exact integer/native field. Ollama HTTP decoder сохраняет prefix при malformed arguments и отвергает nested/escaped duplicate keys до map conversion. Buffered error без outcome выдаёт delta, затем исходную ошибку. Late empty-frame failure тоже отзывает continuation.

Универсальный Stream closeout консервативно отмечает retained nonempty exchange как unavailable при error/cancel, включая до-callback JSON/transport error/EOF, conflicting tool assembly, consumer break. Provider content и уже известные finish metadata не переписываются. Флаг виден до capture terminal observation и final lifecycle status. Никаких provider-specific core types.

Gemini native same-event parse failure передаёт известный prefix вместо nil frame; annotations/state updates для rejected parts отфильтрованы, prefix identities и valid signed state сохранены, finish не фабрикуется. Actual SSE fixtures text/signed reasoning/exact complete call + unsupported response подтверждают сохранность и zero follow-up dispatch.
