# Independent checkpoint5 — not final acceptance

Both reviewers completed on manifest `f80ecc59452c606163fceec1c8bcda46ac3acc638e841572d01c8ace2bb654a0`
and task digest `a7120b172f1e3c6e43afb7870e263e3d912cbda7ce412832c884b0b48945d830`.
Checkpoint4 usage-limit failures had produced no verdicts.

Completeness: **138/157 required whole rows (87.90%)**, plus seventeen pending-trigger rows.
Partial rows receive no credit; the applicable denominator awaits human conditional decisions.
S09-11 and S12-03 lose parent completion credit. IA04–06 and AUD01–05 were independently
confirmed in bounded scope, not live certification.

Correctness: two new confirmed P2 findings; no new P0/P1 in inspected scope.

- **IA07: diagnostic CPU amplification.** Each candidate starts a compiler and
  reparses/revalidates the entire resource set. A rejected 26,906-byte schema with
  1,000 missing local refs took 56.81 seconds; 500 refs took 10.02 seconds.
  Require local/remote/mixed-valid/annotation-heavy scale evidence, not arbitrary caps.
- **IA08: diagnostic enum collapse.** Draft7 source
  `{"properties":{"a":{"$ref":"#/missing"}},"enum":[{"$ref":"first"},{"$ref":"second"}]}`
  preserves the original pointer-not-found cause but has no source location.
  Expected: `/properties/a/$ref`. One placeholder collapses distinct enum values,
  making the diagnostic copy invalid. Preserve equality distinctions, source/cause,
  engine-owned edge verification; test enum/const/default/examples in both dialects.
  Ref-looking assertion data is not evidence of a schema edge.

S12-03: old unfinished-pointer paragraphs contradict later implemented-coverage claims.
Reconcile them with actual open limitations, not a fabricated completed contract.

Independent race tests passed for core/shared/mediafetch/tracing/all four providers,
including repeated targeted ingress/schema tests. Green tests do not negate the findings.
After corrections rerun full make gates and both audits on one final snapshot.
No release, commit, issue comment or closure was performed.
