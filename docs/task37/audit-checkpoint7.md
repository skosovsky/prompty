> Historical task37 evidence; superseded by [current contracts](../README.md) and [migration](../migration.md). Original audit content below is retained; old API snippets and test counts are not current acceptance.

# Independent checkpoint7 — IA09 correction

Both reviewers completed on manifest
`9e669920e2c1fffe4ea7c02a43daf7ca20f52fa17daa78eff5eea3d5c227c56f`
and task digest `10f68fba023f4919ef683429768549e7f7e9da8be8f92e886833ccc5d7c89e8e`.

Completeness: **141/160 required whole rows (88.13%)**, plus seventeen pending-trigger rows.
IA09 independently complete. Five conditional demand decisions remain unanswered;
applicable denominator and final acceptance remain unresolved. Future delivery is
distinct from goal closeout preparation; no unauthorized release/comment/closure required.

Correctness: IA09 examples/enum reproductions now retain `/properties/a/$ref`
in both dialects. Inconclusive candidates are restored unchanged; final engine
root graph proves actual edges. Invalid referenced extension remains rejected,
without usable validator or fabricated `/custom/$ref`. No new confirmed finding
in the inspected correction. **IA07/P2 CPU amplification remains open**, as does S09-11.

Independent race runs: diagnostic subset repeated three times; root/shared adapter,
mediafetch and tracing; separate completeness reference/dialect/cache/scale/zero-HTTP subset.
All EXIT0. Full current `make lint` and `make test` terminal EXIT0, all modules/race/generator fixtures:
`/private/tmp/prompty-task37-inconclusive-lint.log`,
`/private/tmp/prompty-task37-inconclusive-tests.log`.
`git diff --check` clean. No workspace edits by reviewers or external delivery.

This is not an absence-of-errors verdict or final acceptance. Further implementation
requires both reviews and full gates again on the actual final production snapshot.
