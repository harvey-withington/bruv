// Package mcpserver exposes a BRUV repo to external agentic chat apps
// (Claude Desktop, etc.) over the Model Context Protocol.
//
// Scope and shape:
//
//   - Streamable HTTP transport. One JSON-RPC message per POST; the
//     synchronous request/response style (no server-initiated SSE) is
//     enough for a tools-only server, so GET returns 405.
//   - Tools only. We advertise `{"tools": {}}` capabilities — no
//     resources, prompts, or sampling.
//   - One server per Repo. The repo is fixed by the connector URL
//     (/repos/<id>/mcp), NOT chosen by the LLM. There is no repo
//     argument on any tool, so an assistant cannot write to the wrong
//     repo. See plan/bruv-mcp-server-for-third-party-agents-*.md.
//   - Capture-focused tool set: create Brands / Streams / Projects /
//     Categories / Cards, populate cards, plus list/get/search for
//     grounding. No destructive operations in v1.
//
// Auth is handled upstream: the route is mounted behind the same
// requireAuth(device-token) wrapper as the rest of /repos/<id>/...
//
// Naming: this is the MCP *server*. internal/mcp is the MCP *client*
// (BRUV consuming other servers); core/services/mcpsvc is the client's
// config CRUD. The three are distinct on purpose.
package mcpserver

import (
	"bytes"
	"encoding/json"
	"io"
	nethttp "net/http"
	"strings"

	"bruv/core/supervisor"
	"bruv/internal/mcp"

	"github.com/google/uuid"
)

// maxBody caps a single JSON-RPC POST body. Capture payloads (a card
// plus a handful of blocks) are tiny; 5 MiB is generous headroom and a
// backstop against a misbehaving client.
const maxBody = 5 * 1024 * 1024

// Handler serves the MCP endpoint for whichever repo the request URL
// names. It resolves the repo per request via the Supervisor, so a
// single Handler instance backs every repo's /repos/<id>/mcp route.
type Handler struct {
	sup     *supervisor.Supervisor
	version string
}

// New builds the MCP handler. version is stamped into the initialize
// handshake's serverInfo so clients can correlate behaviour with a
// BRUV release.
func New(sup *supervisor.Supervisor, version string) *Handler {
	if version == "" {
		version = "dev"
	}
	return &Handler{sup: sup, version: version}
}

// rpcRequest is one incoming JSON-RPC message. ID is kept as raw JSON so
// we can echo back the client's id verbatim (it may be a string or a
// number) and tell requests from notifications (no id) apart.
type rpcRequest struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id,omitempty"`
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params,omitempty"`
}

// rpcResponse is one outgoing JSON-RPC response. Reuses internal/mcp's
// ErrorObject for the error shape and codes.
type rpcResponse struct {
	JSONRPC string           `json:"jsonrpc"`
	ID      json.RawMessage  `json:"id,omitempty"`
	Result  any              `json:"result,omitempty"`
	Error   *mcp.ErrorObject `json:"error,omitempty"`
}

// ServeHTTP implements the Streamable HTTP transport for one repo.
func (h *Handler) ServeHTTP(w nethttp.ResponseWriter, r *nethttp.Request) {
	// Surface the session id to clients that read response headers
	// (the shared CORS wrapper handles request-side headers).
	w.Header().Set("Access-Control-Expose-Headers", "Mcp-Session-Id")

	repoID := parseRepoID(r.URL.Path)
	if repoID == "" {
		nethttp.NotFound(w, r)
		return
	}
	// Lazy-load like the rest of the transport — the headless server
	// warms everything at boot, the desktop loads on first touch.
	rt, err := h.sup.Load(repoID)
	if err != nil || rt == nil {
		nethttp.NotFound(w, r)
		return
	}
	repoName := h.repoName(repoID)

	switch r.Method {
	case nethttp.MethodPost:
		// handled below
	case nethttp.MethodDelete:
		// Session teardown — we hold no per-session state, so just ack.
		w.WriteHeader(nethttp.StatusNoContent)
		return
	default:
		// No server-initiated SSE stream for a tools-only server.
		w.Header().Set("Allow", "POST, DELETE")
		nethttp.Error(w, "method not allowed", nethttp.StatusMethodNotAllowed)
		return
	}

	raw, err := io.ReadAll(io.LimitReader(r.Body, maxBody))
	if err != nil {
		writeRPCError(w, nil, mcp.ErrCodeParseError, "read body: "+err.Error())
		return
	}
	body := bytes.TrimSpace(raw)
	if len(body) == 0 {
		writeRPCError(w, nil, mcp.ErrCodeInvalidRequest, "empty request body")
		return
	}

	// JSON-RPC batch (array of messages).
	if body[0] == '[' {
		h.serveBatch(w, rt, repoName, body)
		return
	}

	var req rpcRequest
	if err := json.Unmarshal(body, &req); err != nil {
		writeRPCError(w, nil, mcp.ErrCodeParseError, err.Error())
		return
	}
	if req.Method == "initialize" {
		w.Header().Set("Mcp-Session-Id", uuid.NewString())
	}
	resp, isNotification := h.dispatch(rt, repoName, &req)
	if isNotification {
		w.WriteHeader(nethttp.StatusAccepted)
		return
	}
	writeJSON(w, resp)
}

// serveBatch processes an array of JSON-RPC messages, returning an array
// of responses (notifications produce none). An all-notification batch
// gets a bare 202.
func (h *Handler) serveBatch(w nethttp.ResponseWriter, rt *supervisor.Runtime, repoName string, body []byte) {
	var reqs []rpcRequest
	if err := json.Unmarshal(body, &reqs); err != nil {
		writeRPCError(w, nil, mcp.ErrCodeParseError, err.Error())
		return
	}
	out := make([]*rpcResponse, 0, len(reqs))
	for i := range reqs {
		resp, isNotification := h.dispatch(rt, repoName, &reqs[i])
		if !isNotification {
			out = append(out, resp)
		}
	}
	if len(out) == 0 {
		w.WriteHeader(nethttp.StatusAccepted)
		return
	}
	writeJSON(w, out)
}

// dispatch routes one JSON-RPC message. The bool return is true when the
// message was a notification (no id) and therefore produces no response.
func (h *Handler) dispatch(rt *supervisor.Runtime, repoName string, req *rpcRequest) (*rpcResponse, bool) {
	if len(req.ID) == 0 {
		// Notification — we only expect notifications/initialized, and
		// there's nothing to do for a stateless server. Ack with no body.
		return nil, true
	}

	resp := &rpcResponse{JSONRPC: "2.0", ID: req.ID}
	switch req.Method {
	case "initialize":
		resp.Result = h.initializeResult(repoName, req.Params)
	case "ping":
		resp.Result = map[string]any{}
	case "tools/list":
		resp.Result = map[string]any{"tools": toolDefs(repoName)}
	case "tools/call":
		resp.Result = callTool(rt, req.Params)
	default:
		resp.Error = &mcp.ErrorObject{
			Code:    mcp.ErrCodeMethodNotFound,
			Message: "method not found: " + req.Method,
		}
	}
	return resp, false
}

// initializeResult builds the handshake reply, negotiating the protocol
// version down to one we speak and naming the bound repo so a user with
// several BRUV connectors can tell them apart.
func (h *Handler) initializeResult(repoName string, params json.RawMessage) map[string]any {
	proto := mcp.ProtocolVersion
	if len(params) > 0 {
		var p struct {
			ProtocolVersion string `json:"protocolVersion"`
		}
		if json.Unmarshal(params, &p) == nil && p.ProtocolVersion != "" {
			for _, v := range mcp.AcceptedProtocolVersions {
				if v == p.ProtocolVersion {
					proto = p.ProtocolVersion
					break
				}
			}
		}
	}
	return map[string]any{
		"protocolVersion": proto,
		"capabilities":    map[string]any{"tools": map[string]any{}},
		"serverInfo": map[string]any{
			"name":    "BRUV — " + repoName,
			"version": h.version,
		},
		"instructions": "Capture ideas and inspiration into the \"" + repoName +
			"\" BRUV board. A BRUV board organises ideas as Brand → Stream → " +
			"Project → Category → Card. Use create_card to capture an idea; pass " +
			"brand/stream/project/category names to file it (they're created if " +
			"they don't exist). Call the list_* tools first if you need to match " +
			"existing names. Everything you do here stays within this one board.",
	}
}

// repoName looks up the registry display name for a repo id, falling
// back to the id itself.
func (h *Handler) repoName(id string) string {
	for _, e := range h.sup.List() {
		if e.ID == id {
			if e.Name != "" {
				return e.Name
			}
			break
		}
	}
	return id
}

// parseRepoID extracts <id> from a "/repos/<id>/mcp" path.
func parseRepoID(p string) string {
	p = strings.TrimPrefix(p, "/repos/")
	p = strings.TrimSuffix(p, "/")
	p = strings.TrimSuffix(p, "/mcp")
	if p == "" || strings.Contains(p, "/") {
		return ""
	}
	return p
}

func writeJSON(w nethttp.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(v)
}

func writeRPCError(w nethttp.ResponseWriter, id json.RawMessage, code int, msg string) {
	writeJSON(w, rpcResponse{
		JSONRPC: "2.0",
		ID:      id,
		Error:   &mcp.ErrorObject{Code: code, Message: msg},
	})
}
