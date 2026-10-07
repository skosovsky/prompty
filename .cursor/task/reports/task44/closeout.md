# Task44 closeout

Breaking telemetry update: pinned Development OpenTelemetry GenAI conventions revision 4f85037ef86e92c510d2ef881a58f1076f6fc0e4 replaces legacy prompt-ID/raw exception/token attrs. Caller-owned tracer/provider and explicit projection keep OTel in the extension. Presence-aware input/output/total/cache/reasoning usage, independently observed response model, outcome and machine error categories retain known partial metadata. Known transport/protocol and joined independent failures remain errors alongside provider outcome.

Core Response/ResponseChunk.Model, ResponseWire schema and CloneResponse preserve observed metadata. StreamStatus.StopReason distinguishes Close/consumer/cancellation; RemoteResponseStream exposes mandatory original lifecycle observation. No extra producer loop or fictitious unused-stream duration. SDK source model coercion is rejected and source-derived history continuation revoked, with known partial facts preserved.

Explicit Responses Create/native stream/Get/Cancel, batch and cache wrappers distinguish generation usage from resource observations. Get/Cancel retain known model/outcome/finish without repeated tokens; batch item payloads and cache size stay resource facts. Unified capability matrix links implementation and offline fixtures; root/adapter README/godoc/examples and clear-break migration38–43 match current surfaces. Historical task37 evidence remains unchanged.

Removed legacy telemetry keys/raw error export; no compatibility aliases. Host owns exporter/projection/retention, model profiles, retries, remote continuation, durable state and pricing. No backend/dashboard/eval/agent loop/storage/secret scanner/token metrics added. Default tracing excludes content/IDs/URLs/error text. Development conventions are pinned, future/live conformance is not promised.

Accepted r3: completeness19/19=100%, errors0openconfirmed; same761-file immutable snapshot, parent/independent full lint/test/race/generated green. See acceptance.md, checks-r3.md, completeness-r3.md, errors-r3.md. R1/r2 findings and corrections retained. Acceptance-only status metadata follows audits; one short signed task commit, no publication.
