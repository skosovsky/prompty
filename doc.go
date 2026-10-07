// Package prompty provides caller-owned prompt materialization and canonical
// executions for LLM applications. Direct BYOT executions and optional manifest
// recipes share explicit JSON Schema, tool-scope and validation boundaries.
// Vendor SDKs live in separate adapter modules; the checked adapter client owns
// capability admission and canonical provider projection.
//
// Captured source snapshots and render plans retain owned material; execution,
// recipe and wire DTOs remain mutable. Streams are lazy, single-consumer handles;
// Close requests cancellation and does not join or roll back remote work.
// Optional structured executors own one serial attempt chain. Retries, approval,
// persistence, routing and reconciliation remain caller responsibilities.
//
// Current contracts: docs/schema-data-contract.md,
// docs/tool-execution-contract.md, docs/provider-exchange-contract.md and
// docs/source-snapshot-contract.md. See docs/migration.md for breaking changes.
package prompty
