// Package remoteregistry provides remote template loading via Fetcher (HTTP or Git).
// New creates a stateless Registry. Use WithCache to add explicit TTL cache and
// in-flight request dedupe when needed. WithCacheLimit opts into a positive FIFO
// entry limit; TTL cleanup runs on material operations, without background workers.
package remoteregistry
