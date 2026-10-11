# Task41 — implementation closeout

Статус: принято r2, 17/17 (100%), 0 открытых подтверждённых ошибок, full lint/test/race green. База 39b6c4e.

Старые DropOldest/DropOldestStrategy/TruncateWithStrategy и message-based Strategy.Truncate удалены. Новый Apply принимает full execution, estimator и universal index-based protection/removal-ordering contracts; возвращает owned execution с report и диагностируемой ошибкой невозможного бюджета.

System/developer и default current turn защищены; без user marker — вся история. Explicit boundary/protected indices/dependencies host-owned. Turn groups не разрываются system insertions; call/result native IDs+names связывают dependencies через всю историю, parallel calls сохраняются целиком. Unfinished/orphan/ambiguous/duplicate/ID-less exchange защищён без fabricated IDs/heuristics. Opaque state-bearing сообщения объединяются консервативно, payload не декодируется, signed content и associations не редактируются. Canonical projection validation before/after не даёт удалить malformed older history ради success.

MessageEstimator объявляет message coverage и ловит nil/negative/overflow/error. BYOT EstimatorFunc позволяет execution-wide host projection с tools/schema/native overhead/reserve; existing provider estimator не объявлен whole-request по одному method name. Known zero и unknown различны, negative/unknown/changed scope fail. После каждого удаления весь кандидат оценивается заново без аддитивного предположения. Counter/strategy получают clone/metadata copies, input и middleware policy snapshot не меняются.

Report: before/after scope/estimate, original removed/protected indices, satisfied/unsatisfied/disabled и reason. Protected/fixed material above limit не становится success. Contextx default blocks any error/unmet budget before upstream; explicit advisory permits только valid unmet budget, observer failure тоже stop. Native/buffered используют тот же transform и реальный lazy/single-use upstream handle без goroutine или surrogate stream. Invalid input/unreachable budget не создают upstream handle; actual provider dispatch остаётся lazy. Disable explicit, zero/negative/nil не bypass.

README ext/middleware, root link и examples/context_budget runnable offline показывают scopes, current turn, conservative opaque/ID-less limitations, ports ownership и migration. Никаких LLM compaction/retrieval/model limits/tokenizer auto-loading/storage/agent concepts. Checks-r1 содержит targeted/full lint/test/race/generated evidence.

R2: caller CurrentTurnStart теперь сбрасывает обычную turn grouping до проверки system/developer skip. Protected suffix начинается на любом заявленном original index; linked native call/state dependencies всё ещё не разрываются. System и developer boundary regression tests проходят; исходные r1 reports сохранены.
