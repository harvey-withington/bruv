// Package mcp implements a Model Context Protocol client for BRUV.
//
// Scope for v1.0b:
//
//   - Stdio transport only (subprocess with newline-delimited JSON on
//     stdin/stdout, logs on stderr).
//   - Tools only — we declare empty client capabilities, so well-behaved
//     servers will not send resources, prompts, sampling, or roots
//     requests. Unexpected server→client requests are rejected with
//     JSON-RPC -32601 "Method not found" so the server doesn't hang.
//   - Per-repo configuration. Each repo's .bruv/mcp_servers.json lists
//     the servers that travel with the project. Secret env var values
//     live in the OS keychain, keyed by repo ID + server name + var
//     name, so sharing a repo never leaks API keys.
//
// Out of scope (tracked for follow-ups, not in this sprint):
//
//   - Resources, prompts, sampling, elicitation, logging notifications
//   - HTTP/SSE transport
//   - Live tool-list refresh on notifications/tools/list_changed
//   - OAuth flows for remote servers
//   - Exponential backoff on crash restarts (we do a simple retry)
//
// This file contains the pure protocol types — envelope shapes, error
// codes, and MCP-specific request/response payloads. No I/O, no state.
// The transport and client layers above this file depend on these types
// but nothing here depends on anything else in the package.

package mcp

import "encoding/json"

// ProtocolVersion is the MCP spec revision we target for the initialize
// handshake. Servers may echo this back or downgrade us to an older
// version; we accept any of the three known revisions. See
// AcceptedProtocolVersions.
const ProtocolVersion = "2025-06-18"

// AcceptedProtocolVersions lists every MCP spec revision we know how to
// speak, in preference order. If a server downgrades us to one of these
// we keep talking; anything else is a hard failure at initialize time.
var AcceptedProtocolVersions = []string{
	"2025-06-18",
	"2025-03-26",
	"2024-11-05",
}

// ClientName and ClientVersion are what we report in clientInfo during
// the initialize handshake. Servers typically log these so it's useful
// for debugging why a given server is behaving oddly for BRUV
// specifically. ClientVersion tracks the BRUV release so we can
// correlate server-side issues with BRUV releases.
const (
	ClientName    = "bruv"
	ClientVersion = "1.0"
)

// JSON-RPC standard error codes. MCP does not define additional numeric
// ranges as of 2025-06-18 so these cover everything we might emit.
const (
	ErrCodeParseError     = -32700
	ErrCodeInvalidRequest = -32600
	ErrCodeMethodNotFound = -32601
	ErrCodeInvalidParams  = -32602
	ErrCodeInternalError  = -32603
)

// --- JSON-RPC 2.0 envelope ---
//
// MCP tightens JSON-RPC 2.0 in two subtle ways:
//   - IDs MUST NOT be null (vanilla JSON-RPC allows null).
//   - An ID MUST NOT be reused within a session.
//
// Our ID generator is a monotonic int64 counter (see client.go), which
// satisfies both constraints trivially.

// Request is an outgoing JSON-RPC request. Notifications omit ID
// entirely — see Notification below — so this type is used only when
// we expect a response.
type Request struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      int64           `json:"id"`
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params,omitempty"`
}

// Notification is an outgoing JSON-RPC notification — same shape as a
// request but without the ID field. The spec is explicit that the ID
// field MUST be absent, not null.
type Notification struct {
	JSONRPC string          `json:"jsonrpc"`
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params,omitempty"`
}

// Response is a JSON-RPC response or an incoming server→client request.
// We use one type for both because they share almost all fields and the
// read loop needs to branch on which combination is present to decide
// how to handle the message.
//
// Interpretation table:
//
//	ID set, Method set, Result/Error unset   → incoming request from server
//	ID set, Method unset, Result set         → successful response to our request
//	ID set, Method unset, Error set          → error response to our request
//	ID unset, Method set                     → notification from server
//
// We use a pointer for ID so we can distinguish "absent" from zero.
type Response struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      *int64          `json:"id,omitempty"`
	Method  string          `json:"method,omitempty"`
	Params  json.RawMessage `json:"params,omitempty"`
	Result  json.RawMessage `json:"result,omitempty"`
	Error   *ErrorObject    `json:"error,omitempty"`
}

// ErrorObject is the JSON-RPC error payload. Data is free-form; we log
// it but never depend on its shape since MCP doesn't standardise it
// beyond a few informational cases (protocol version mismatch puts
// `{supported: [...], requested: "..."}` here).
type ErrorObject struct {
	Code    int             `json:"code"`
	Message string          `json:"message"`
	Data    json.RawMessage `json:"data,omitempty"`
}

// Error implements the error interface so ErrorObject values can flow
// through Go's normal error handling without wrapping.
func (e *ErrorObject) Error() string {
	if e == nil {
		return ""
	}
	return e.Message
}

// --- MCP payload types (initialize, tools/list, tools/call) ---
//
// These are the `params` and `result` payloads for the three method
// calls we actually make. Everything else we either ignore (incoming
// notifications) or reject with -32601 (incoming requests for
// capabilities we didn't declare).

// InitializeParams is sent as params in the initialize request. The
// capabilities object is always empty for a tools-only client — see
// the package doc comment for why.
type InitializeParams struct {
	ProtocolVersion string                 `json:"protocolVersion"`
	Capabilities    map[string]interface{} `json:"capabilities"`
	ClientInfo      ClientInfo             `json:"clientInfo"`
}

// ClientInfo identifies BRUV to the server in initialize.
type ClientInfo struct {
	Name    string `json:"name"`
	Version string `json:"version"`
}

// InitializeResult is what servers return in response to initialize.
// ProtocolVersion may be the one we requested or a downgrade — see
// AcceptedProtocolVersions.
type InitializeResult struct {
	ProtocolVersion string                 `json:"protocolVersion"`
	Capabilities    map[string]interface{} `json:"capabilities"`
	ServerInfo      ServerInfo             `json:"serverInfo"`
	Instructions    string                 `json:"instructions,omitempty"`
}

// ServerInfo is the server-side counterpart to ClientInfo.
type ServerInfo struct {
	Name    string `json:"name"`
	Version string `json:"version"`
}

// ListToolsParams is the params for tools/list. Cursor is opaque — we
// pass whatever the server gave us in the previous response without
// interpreting it.
type ListToolsParams struct {
	Cursor string `json:"cursor,omitempty"`
}

// ListToolsResult is the response shape. NextCursor is present and
// non-empty when there are more pages.
type ListToolsResult struct {
	Tools      []Tool `json:"tools"`
	NextCursor string `json:"nextCursor,omitempty"`
}

// Tool describes one callable operation exposed by a server. The
// InputSchema is a JSON Schema object — we pass it verbatim to the LLM
// via the existing llm.ToolDef plumbing so the model can see what
// arguments are expected.
type Tool struct {
	Name        string                 `json:"name"`
	Title       string                 `json:"title,omitempty"`
	Description string                 `json:"description"`
	InputSchema map[string]interface{} `json:"inputSchema"`
}

// CallToolParams is the params for tools/call. Arguments is a free-form
// JSON object keyed by input-schema property names.
type CallToolParams struct {
	Name      string                 `json:"name"`
	Arguments map[string]interface{} `json:"arguments"`
}

// CallToolResult is the response shape. Content is a list of typed
// items — see Content — and IsError distinguishes tool-level failures
// (which should flow back to the LLM) from JSON-RPC protocol errors
// (which are in Response.Error and should be raised as Go errors).
//
// A missing IsError field is treated as false. Spec says successful
// results may omit it.
type CallToolResult struct {
	Content []Content `json:"content"`
	IsError bool      `json:"isError,omitempty"`
}

// Content is one item in a tool result. The Type field is the
// discriminator; the other fields are populated per-type.
//
// Flattening for LLM consumption:
//   - text:          Text is appended verbatim.
//   - image/audio:   a placeholder "[image: type, N bytes]" is appended
//                    rather than embedding megabytes of base64 into the
//                    LLM context (which would blow up token budgets and
//                    accomplish nothing — current LLMs don't see the
//                    image through this channel anyway).
//   - resource_link: "[resource: uri]" placeholder.
//   - resource:      the embedded resource's Text is appended if
//                    present, otherwise a blob placeholder.
//
// See Content.Flatten.
type Content struct {
	Type     string    `json:"type"`
	Text     string    `json:"text,omitempty"`
	Data     string    `json:"data,omitempty"`
	MimeType string    `json:"mimeType,omitempty"`
	URI      string    `json:"uri,omitempty"`
	Name     string    `json:"name,omitempty"`
	Resource *Resource `json:"resource,omitempty"`
}

// Resource is the embedded-resource payload inside a content item of
// type "resource". Either Text or Blob is set, never both.
type Resource struct {
	URI      string `json:"uri,omitempty"`
	MimeType string `json:"mimeType,omitempty"`
	Text     string `json:"text,omitempty"`
	Blob     string `json:"blob,omitempty"`
}
