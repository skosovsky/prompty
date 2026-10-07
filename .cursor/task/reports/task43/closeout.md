# Task43 closeout

Status: accepted by independent r2 completeness24/24=100% and errors0; commit preparation follows this acceptance.

## Changed contracts

- Canonical `PromptExecution.HostedTools` replaces the Responses-only declaration field. Declarations are deep-owned, portable through execution wire and included in batch execution digests. Adapter-owned versioned kinds/settings stay outside core.
- Optional `RemoteResponseStreamer` supplies foreground native creation without Get polling, fallback or automatic resume. Operation-local handles retain safe observed identity/status alongside partial canonical output.
- `StreamPartFinalization` seals exact previously observed logical fragments and binds final state/annotations without duplicated content. Canonical associations remain stable under interleaved native closes.
- Required scoped `MessageAnnotations` extend opaque continuation to hosted-only messages. Core enforces identity/shape; required adapter mappings and profile knowledge are checked before dispatch. State omission revokes required continuation. Unmarked hosted legacy history is rejected.
- `OutcomePaused` distinguishes resumable Anthropic pause from completed structured output and local handler permission. Continuation requires an explicit caller request.
- Responses reasoning controls and strict local schemas, Ollama Format/Think, Gemini function result media and pinned Anthropic hosted search/fetch use actual endpoint/model knowledge, original-schema validation and closed provider mappings.

## Removed legacy routes

`RemoteResponseRequest.HostedTools` has been removed; no alias or merge with the canonical execution field. Hosted history without required scope markers no longer silently replays. No prompt/synthetic fallback was introduced for unsupported native output/stream capabilities. Strict schemas are not rewritten into optional/nullable approximations.

## Boundaries and validation

The library owns contracts/provider translation, not loops, retries, discovery, browser execution, uploads/downloads, storage, IAM, credentials or session coordination. BYOT remains intact; SDK DTOs stay in adapters. Public docs/provider-capability-matrix.md and provider READMEs describe feature/model/operation gates and migration. Fixtures are offline SDK protocol evidence, not live model/account certification.

R1 full checks passed but adversarial audit failed: summary index -1 escaped as a Responses panic; malformed Anthropic delta source values were coerced into successful output. Full r1 reports retain exact findings and completeness denominator24. Corrections add declared/ranged summary indices and raw consumed-event type validation before SDK values mutate canonical state; legitimate extra provider fields remain allowed. Fresh full make test/lint r2 passed; both independent auditors accepted snapshot805files SHA256 fba7ed2e47262acd7ad3f876cc02fcc469c67c50d420152adc33b2fff0315d3b. Reports completeness-r2.md, errors-r2.md and checks-r2.md retain evidence. This status-only closeout update follows acceptance and does not change implementation.
