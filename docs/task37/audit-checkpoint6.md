# Independent checkpoint6 — not final acceptance

Both reviewers checked frozen manifest `6007e6844eab9607e28e1369837762f2874ce8a77e6742ac311330f188ed5d9b`
and task digest `e883f35311fe71bdb63d74050fcad0b8938a7678c608a385ddd7986139feb91c`.
Full all-module lint/test logs probe-cache-lint/tests both terminal EXIT0, race and generator fixtures included.

Completeness: **140/159 required whole rows (88.05%)**, plus seventeen pending triggers.
IA08's original enum collapse and S12-03 documentation drift independently corrected.
S09-11 and IA07 remain partial; conditional decisions and future delivery unchanged.
This historical denominator excludes the subsequent new IA09 row, not an accepted scope reduction.

Correctness confirms IA08 and no observed success-cache contamination in repeated
invalid → valid → invalid fixtures. IA07 still open: whole-root queue work remains.

**New IA09 / P2:** legal annotation/assertion data aborts all reference diagnostics.
`{"properties":{"a":{"$ref":"#/missing"}},"examples":[{"$ref":"first","type":"not-a-schema-type"}]}`
and `enum:[{"$ref":"first","type":42}]` produce empty Locations in both dialects,
despite preserved original pointer-not-found cause. Expected `/properties/a/$ref`.
Generic candidate is not schema-valid; non-reference probe error incorrectly aborts
all diagnostics. Require inconclusive handling with unchanged data and final engine
root-graph verification; referenced invalid extension nodes must not be declared valid.

After any correction both audits and full gates require rerun. No external action occurred.
