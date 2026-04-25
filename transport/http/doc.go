// Package http is the HTTP + SSE transport for BRUV's core services.
//
// Runs on 127.0.0.1:<random-port> from the Wails desktop binary today
// (phase 3 — transport exists but the frontend still uses Wails IPC).
// Phase 4 will pivot the frontend cloud adapter to call through here,
// at which point the same server code serves Mode A remote backends
// and Mode B hosted deployments.
//
// Protocol summary:
//
//   - POST /rpc — JSON-RPC 2.0 with positional params. A reflection
//     dispatcher maps method names to App methods. No codegen. Domain
//     types stay pure Go structs.
//   - GET  /events — Server-Sent Events stream subscribed to
//     core/events.Bus. Chosen over WebSocket because events flow
//     server→client only (RPC handles writes) and SSE needs zero new
//     dependencies, reconnects itself in EventSource, and parses with
//     the standard library.
//   - GET  /healthz — liveness probe, unauthenticated
//   - GET  /version — build info, unauthenticated
//
// Auth: a single bearer token stored at <serverdata>/auth-token.txt
// (generated on first boot, chmod 0600). Phase 5 replaces this with a
// bootstrap → per-device token enrolment flow. Until then the
// loopback-only binding keeps the single token safe — the server
// refuses to bind non-loopback without an already-populated token.
package http
