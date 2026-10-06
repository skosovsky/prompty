# Task40 — author checks, round 1

База c8141c5. Приёмка ожидается; до неё коммит не создавать.

- Source snapshot boundary tests с race: OLD-cache/NEW-descriptor mismatch, explicit Reload, partial-only identity change, source disappearance, remote one-read capture, TTL identity retention, embed caller-FS isolation, concurrent Reload/Plan/materialization — passed.
- Custom mutable materializer rejection и defensive copies/canonical map ordering — passed.
- Raw BindRuntime и typed recipes обходят unrelated Plan/Verify и принимают минимальный capture-порт — passed.
- Descriptor executable schema/wire legacy rejection и path/digest conformance — passed.
- Git immutable commit blob fixture, moving checkout, unavailable/moving revision rejection — passed with race.
- Codegen captured index/source replacement and generated golden/consumer checks — passed.
- Full final make lint: exit 0, all modules, zero issues. Log /tmp/task40-lint-final-r1c.log.
- Full final make test: exit 0, все модули/race/generated consumers. Log /tmp/task40-test-final-r1b.log. После финальных lint-only правок (явные zero fields и форматирование) boundary suite повторена с race: /tmp/task40-final-boundary-r1.log, exit 0.
- git diff --check: exit 0.

Targeted logs: /tmp/task40-final-boundary-r1.log, /tmp/task40-source-gen-final.log, /tmp/task40-codegen-capture-final.log, /tmp/task40-git-tests-01.log. Go/race and all module lint use /tmp/prompty-go-cache and /tmp/prompty-lint-cache. Git fixture локальный; live remote/network conformance не заявляется.

Frozen snapshot /tmp/prompty-task40-snapshot-01; manifest/identity/diff рядом. SHA identity передаётся обоим аудиторам. Source snapshot не менять во время приёмки; независимые probes выполняются в отдельных копиях. Полноту считать по всему заданию; текущие peer reports не читать.
