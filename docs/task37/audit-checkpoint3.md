> Historical task37 evidence; superseded by [current contracts](../README.md) and [migration](../migration.md). Original audit content below is retained; old API snippets and test counts are not current acceptance.

# Independent ingress checkpoint

Historical frozen snapshot, before IA-05/06 correction: tracked/nonignored-untracked manifest SHA-256 `f81685cb966744570341209a06cc47fae7935bf82cac972843b2092d27f2a382`; original task SHA-256 `80b0255e735b03df0335c781ce6dae13f2c3087e5dcf8f10472c5a50f086cfbe`. Manifest command: `git ls-files -c -o --exclude-standard -z | sort -zu | xargs -0 shasum -a 256 | shasum -a 256`. Both agents read the same frozen source/tests/docs and made no workspace changes. This is not a final acceptance report.

## Completeness

Independent source whole-row ledger: **135/155 required = 87.10%**, with seventeen pending-trigger rows; the applicable denominator remains unknown until five human demand decisions. Partial earns no credit. IA-05 was confirmed but not yet a separate matrix row at this checkpoint; subsequent additions require a new denominator, not reuse of this percentage.

IA-01–03 and bounded S06-05/11 original signed-number preservation are independently completed. S01-04, S04-02, S04-04, S06-01, S12-07 and S12-08 previous completions are confirmed; their parent partial placeholders are stale. S12-06 is restored after sync.Once fixture repair and fresh independent root/shared/all-provider race success. Capture paragraphs now match canceled acknowledged closeout.

IA-04 remains partial because malformed-body error privacy is broken and permanent negative association/auth/timeout/cancel fixtures are absent. S09-11 remains partial for missing fragment/anchor origins. S12-03 remains partial for the privacy contract mismatch and migration.md's obsolete S04-04 boundary question. The other uncompleted rows are mixed conditional S05-13/S09-06/S10-01/S10-07, final disposition S12-01 and source future-delivery S13-RELEASE/CLOSE/LINKS/S13-01–09. No requirements were excluded to improve the score.

Prepared release/author migration exists. Actual unauthorized publication/comment/issue closure is not required to satisfy the goal's preparation criterion; original delivery rows remain separately tracked rather than marking draft as delivery. Human decisions remain required for additional native adapters, Responses/hosted/background, offline batch, explicit cache resources and separate forced-tool output.

## Correctness

IA-01–03 original reproductions still pass. IA-04 original actual SDK/local HTTP reproduction now preserves canonical/envelope 9007199254740993, rejects its unsupported continuation and has only one HTTP call. SDK/config isolation, source bounds/body cleanup/association and diagnostic-only schema compiler were inspected. Independent uncached race tests passed for core, adapter, mediafetch and all four provider modules, including AUD-01–05 fixtures.

**Confirmed IA-05/P1:** response_source.go's successful-response error sanitizer requires successful signature parsing. Malformed signed HTTP200 JSON with trailing comma makes that detection return false, and SDK error exposes the complete private body/signature. Original safe local reproduction: `/private/tmp/prompty-correctness-audit.WeWIxI/gemini_malformed.go`; parent independently reproduced it. A permanent regression is required; temporary repro is not acceptance evidence. Sanitize complete observed HTTP2xx SDK response failures independently of signature detection while retaining cancellation/read/transport/host causes without raw-body SDK errors.

No other confirmed new runtime finding in the inspected checkpoint boundaries. Live providers, full auth/Vertex integration, fuzzing and all other modules were not independently certified. Subsequent parent regression exposed an additional SDK shape panic (IA-06); it was not part of this historical frozen audit.

## Next acceptance

Correct IA-05, complete ingress fixtures and reference-origin scope, reconcile evidence/docs and obtain conditional decisions. Both agents must rerun after source/tests/docs changes. Green checkpoint gates and this historical report do not authorize release or issue mutation.
