# Independent checkpoint8 — observation/worklist diagnostics

Both reviewers completed on manifest `b8e2023409ba412f9924476396cf9fc83fcde617294c5aa5f09f8eed1e1545fd`
and task digest `10f68fba023f4919ef683429768549e7f7e9da8be8f92e886833ccc5d7c89e8e`.

Completeness: **143/160 required whole rows (89.38%)**, plus seventeen pending triggers.
IA07 and S09-11 independently complete. No source scope excluded for this percentage.
Seventeen remaining required rows cover four mixed-conditional items, final dispositions
and twelve future-delivery rows. Goal requires prepared release/closeout, not unauthorized
external publication; concrete author instructions exist, conditional dispositions do not.

Correctness: IA07/08/09 regressions independently confirmed corrected; no new confirmed
finding in inspected observation/worklist source. Original 1,000-ref reproduction now
61.7ms (previously56.81s), with all origins. Observation uses public engine metadata,
aborts before queued root processing and does not cache temporary map schemas.
Edges restored before stable existence compilation; invocation-local state not shared.
Actual engine edges discover extension/transitive targets; fresh final graph alone
proves source origins. Original rejection/cause retained, no custom resolver or caps.

Same-pattern unique-target comparison (only first reference changed):
1,000 valid21.52ms / diagnostic98.31ms;
10,000 valid1.345s / diagnostic6.589s.
Roughly4.6–4.9x overhead, not per-ref repeated whole-document amplification.
Intrinsic engine costs remain input-dependent, not constant or universally bounded.

Independent cyclic extensions, nested IDs, scoped anchors, Boolean targets,
malformed annotations and prior enum/annotation reproductions retained correct origins.
Repeated diagnostic race subset, core/shared/mediafetch/tracing race and benchmarks EXIT0.
All-module make lint/test including race/generator fixtures terminal EXIT0 in observation-lint/tests logs.
No reviewer workspace edits, release, external comment or issue closure.

A subsequent test-only checkpoint adds exact unique-positive permanent baseline;
it still requires current full gates and paired verification, not borrowed final acceptance.
