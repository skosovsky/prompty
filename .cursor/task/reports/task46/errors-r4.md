# Task46 — final independent error acceptance, round 4

Verdict: **PASS**. Все ранее подтверждённые ошибки исправлены; после обновления зависимостей и публикации новых подтверждённых ошибок, regressions или нарушений архитектурных границ не обнаружено. Published integration и фактическая публикация refs теперь имеют прямые подтверждения. Issue closeout остаётся отдельным pending gate, пока нет внешнего комментария с миграцией и подтверждения закрытия.

## Аудит новых refs и артефактов

- Actual manifest: contexty v0.13.1, routery v0.7.0, toolsy v0.18.0. CI pins соответствуют их published `Origin.Hash`: `3ce901169b09b7136b126d9baa98e1875b33fa82`, `fe61308e030e163195ccdb4d548109a5a4ebdde1`, `6ed0a8a6a4e664c25f32397fa63dd6f0a46cba09`. Подтверждено независимым `GOWORK=off go list -m -json`.
- Перmalink docs/host-tool-dispatch.md указывает exact contexty commit, предоставляющий supported-mapping.md. Проверены содержимое actual published cache и соответствие raw-sidecar/opaque-storage declarations consumer.
- `ci-source-r2.json`: success=true, exact pinned source commits, dirty='' для всех, semantic race tests/lint/demo exit 0. `local-r8.json` честно сохраняет actual local HEAD/dirty отдельно.
- `published-r1.json`: success=true, root + OpenAI adapter v0.16.0, external manifest correct, no replacements (`Replace` absent), race/lint/demo exit 0. Published mode действительно GOWORK=off, не использует local sources.
- Published root и adapter `Origin.Hash` совпадают: `4d028f87265f687580ab6614dfcf36bb6d125b45`. Byte comparison текущего code/docs/runner/CI с downloaded published root показывает equality для tool_admission.go, tool_orchestration.go, docs/host-tool-dispatch.md, scripts/host_dispatch_checks.py, .github/workflows/ci.yml.
- Независимый `git ls-remote origin 'refs/tags/*v0.16.0'` вернул все 13 expected refs с exact published commit `4d028f87265f687580ab6614dfcf36bb6d125b45`. Это согласуется с remote-refs.json (missing_or_wrong=[]; success=true).
- release-break-r2.log заканчивается `Release v0.16.0 published; exact refs verified`; successful full checks/gen/release unit checks присутствуют. Clear-break release соответствует существенному публичному admission contract, заявленному scope.

## Независимая runtime перепроверка published dependencies

Создан временный copied optional module `/private/tmp/prompty-task46-errors-r4`; его manifest адаптирован только на опубликованные root/adapter v0.16.0 через go mod edit + tidy. GOWORK=off, никаких replaces/workspace/local source imports. Использованы actual published external версии из current manifest.

Env: GOPATH=/private/tmp/prompty-task46-go GOCACHE=/private/tmp/prompty-task46-build GOMODCACHE=/private/tmp/prompty-task46-mod.

`go test -race -count=1 ./...`: **PASS** (optional package и demo build). Вместе с полным semantic suite выполнены independent R3 adversarial probes для residual UTF-8 rejection и exact two-call raw sidecar round-trip/identity/index/numeric-value/invalid-UTF8 tampering. Тем самым fixes повторно доказаны уже на actual published core + consumers.

Подтверждены host-owned approval/pending policy, no completed replay, full-batch pure admission/original-schema boundary, raw int64 correlation, business error/private projection, explicit unsupported mapping, opaque continuation state и callback-safe caller-owned stream cleanup. Core не получил runtime/policy/consumer-specific зависимостей.

## Остаточный completion gate

Ошибок implementation нет. Issue closeout этим отчётом не объявляется выполненным: нужны публичный migration comment с runnable guidance/release/evidence и подтверждение closed state. Агент implementation не редактировал, сообщений наружу и release не запускал.
