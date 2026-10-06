# Host redaction boundary

Credentials belong to injected SDK/transport configuration, not portable profile identity or provider envelopes. Checked endpoint identity rejects userinfo/query/fragment instead of silently rewriting destination scope. Provider-state codecs preserve only their supported provider continuation shape; core cannot interpret arbitrary caller payload as credential-free.

`LoggingExecution` and `LoggingResponse` omit opaque state and mark omitted required continuation unavailable. They are intermediate projections, not permission to emit the entire DTO to a logger: canonical prompt/media, tool args/results, annotations, provider settings, provenance and arbitrary caller metadata can contain secrets. Persistence is a separate, explicit `StatePreserve` decision with application-owned authorization/retention. Logging omission must not mutate the source or remove state from that separately approved persistence path.

Prefer an application allowlist rather than attempting a universal substring detector:

```go
wire, err := prompty.LoggingExecution(execution)
if err != nil {
    return err
}
// Application-defined logging DTO: no content, metadata or transport config.
event := struct {
    MessageCount int `json:"message_count"`
}{MessageCount: len(wire.Messages)}
// Pass event, not wire, to the application's logger.
_ = event
```

An application may approve more fields explicitly. Do not reuse a logging projection as continuation or claim it preserves the raw response. Rebuild signatures/state from the independently preserved source, not from redacted text.

Capture uses the existing injected `CaptureProjector`. The default emits no payload, removes request/response IDs, retains observed usage/outcome/digests and marks completeness false; record endpoint is never delivered to the sink. A custom projector owns both payload and `Projection.Metadata`, including error bodies, raw output and opaque state. Use approved correlation/profile/SDK identifiers too: the library cannot detect a token hidden in an arbitrary caller-owned string. HTTP authorization headers are not invented by SDK DTO capture, and unavailable transport-byte coverage cannot be requested successfully.

For an approved summary projection, construct fresh metadata containing only explicitly approved observations, return a valid semantic JSON summary, set `Redacted=true` and `Complete=false`. Do not keep `CaptureBytes` or `Complete=true` after editing/omitting payload. Required byte-exact capture with such a projector fails before dispatch or sink delivery; redaction is not byte-exact replay. A closed redacted report can have no failures and still remain incomplete.

Evidence: `host_redaction_conformance_test.go` proves separate approved logging/persistence projections, removal of caller-owned text/annotation/settings secrets plus opaque state, and unchanged persistence source. Its capture allowlist covers execution/translated/provider/terminal stages, omits credentials/error body/output/state and sensitive request/response identities, retains approved usage/outcome, and closes an explicitly incomplete report without changing source payload/metadata. Existing default projection and metadata isolation fixtures cover default omissions and callback mutation. These fixtures demonstrate policy composition, not a claim that core automatically sanitizes every arbitrary field.
