# Task40 — checks round2

База c8141c5. Приёмка r1 отклонена: completeness 15/17 = 88.24%, errors 4 confirmed (3 P1, 1 P2). Все четыре исправления включены в r2; независимая повторная приёмка ожидается.

- Targeted race: root, manifest, embedregistry, cmd/prompty-gen — exit0 (`/tmp/task40-r2-targeted.log`).
- Полный make test: все 12 modules + generated consumers, race — exit0 (`/tmp/task40-test-r2.log`).
- Полный make lint: все 12 modules — exit0 (`/tmp/task40-lint-r2c.log`). Предыдущие r2/r2b logs содержат устранённые lint-only findings и не являются финальными.
- После последней правки assertion повторён manifest race — exit0 (`/tmp/task40-ownership-r2-final.log`).
- git diff --check — exit0.

Проверены parser input mutation rejection, retained parsed output sealing, nested ComposeValues/Lookup ownership, logical-ID/path alias collisions в обоих порядках. Source changes frozen before auditor dispatch; acceptance percentages provided only by independent auditors.
