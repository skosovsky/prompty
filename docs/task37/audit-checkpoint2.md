# Independent checkpoint reruns

Both original audit agents independently rechecked IA-01–03. They confirmed those fixes in their inspected boundaries, not a clean verdict for the full library. Reports describe the snapshot before subsequent registry-fixture, capture-document and reference-origin changes; no final acceptance or immutable snapshot attestation is claimed.

## Completeness

The auditor reported 133/154 required whole rows = 86.36%, partial rows receiving no credit, plus seventeen pending-trigger rows. The applicable denominator remains unresolved while five conditional decisions are unapproved. This is historical original-source delivery accounting; actual release/comment/issue closure is not authorized or required for the goal's prepared closeout.

IA-01–03 and S04-04 were independently confirmed. S12-03/S12-08 lost credit because two capture paragraphs still described previous incomplete reports rather than acknowledged canceled closeout. S12-06 lost credit because a fresh full-root race command panicked at `remoteregistry/registry_test.go:416` despite previous passing gates. Two concurrent compose probes can both pass select/default before close(started); canceled uses the same unsafe pattern. A targeted repeat passing ten times did not invalidate the observed failure. Parent subsequently replaced these fixture notifications with sync.Once without removing assertions and synchronized both paragraphs; fresh verification is separately recorded in progress.

## Correctness

Original local reproductions now show one consumer capture closeout, retained sentinel error, a closed required-terminal report, accepted valid shadowed default and typed zero-dispatch rejection of an unrepresentable saved Gemini number. Fresh race tests for root, adapter, Anthropic and Gemini exited zero. No workspace edits or live provider calls were made by the auditor.

New confirmed IA-04 / P1: actual Gemini SDK/local HTTP response originally contains signed args.big=9007199254740993, but checked parsing returns canonical Args and envelope with 9007199254740992 and no error. AssistantMessage → second checked Execute then sends the rounded value with the original signature, also without error. The outgoing guard is correct only for its saved-payload scope; it cannot detect prior SDK response rounding. Original reproduction is `/private/tmp/prompty-correctness-audit.WeWIxI/gemini_inbound.go`. Permanent regression must not depend on that temporary file.

Relevant source is `adapter/gemini/gemini.go` SDK Execute and `adapter/gemini/state.go` SDK Part canonicalization/envelope encoding. The SDK's unary decoder uses ordinary JSON map decoding and does not populate original response Body for this method; blindly trusting the returned Part is not original-payload evidence. Preserve authoritative pre-loss numeric payload or fail closed when it cannot be established. IA-04 remains open; exact provider signed round-trip is not certified.

## Remaining work

Address IA-04, finish reference-origin coverage (including missing fragments/anchors), reconcile documentation/evidence and obtain explicit conditional dispositions. Rerun both agents against the actual final state and fresh full gates. Neither report permits publication, release or issue closure.
