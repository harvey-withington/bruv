package mcp

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sync/atomic"
	"time"
)

// Client implements the MCP protocol state machine on top of a
// Transport. One Client corresponds to one connected server; the
// higher-level Registry manages N Clients across the configured
// servers.
//
// A Client's lifecycle:
//
//  1. NewClient(transport)
//  2. Initialize(ctx)          — handshake, must succeed before any tool calls
//  3. ListTools(ctx)            — discover available tools (can be called again to refresh)
//  4. CallTool(ctx, name, args) — invoke a tool, possibly many times
//  5. Close()                   — graceful shutdown
//
// The zero Client value is not usable — always go through NewClient.
type Client struct {
	transport *Transport

	// nextID is the source of JSON-RPC request IDs. We use a
	// monotonic counter behind atomic.Int64 so we never reuse an ID
	// within a session (forbidden by the MCP spec) and never
	// collide across concurrent callers.
	nextID atomic.Int64

	// protocolVersion is set by Initialize to whatever the server
	// accepted — may be our preferred version or a downgrade. Used
	// for logging and compatibility decisions.
	protocolVersion string
	serverInfo      ServerInfo
	initialised     bool
}

// NewClient wraps a Transport in a Client. The transport must already
// be Started; Client does not call Start itself because lifecycle
// ownership of the underlying subprocess belongs to ServerProcess.
func NewClient(transport *Transport) *Client {
	return &Client{transport: transport}
}

// Initialize performs the MCP handshake: sends an initialize request,
// verifies the server's protocol version is one we understand, and
// sends the required notifications/initialized notification.
//
// Must be called exactly once before any tool operations. Returns an
// error if the server's protocol version is not in
// AcceptedProtocolVersions, which is the only case where we refuse
// to talk to a server we successfully contacted.
func (c *Client) Initialize(ctx context.Context) error {
	if c.initialised {
		return errors.New("client already initialised")
	}

	params := InitializeParams{
		ProtocolVersion: ProtocolVersion,
		Capabilities:    map[string]interface{}{}, // tools-only client declares nothing
		ClientInfo: ClientInfo{
			Name:    ClientName,
			Version: ClientVersion,
		},
	}
	var result InitializeResult
	if err := c.call(ctx, "initialize", params, &result); err != nil {
		return fmt.Errorf("initialize: %w", err)
	}

	// Verify the server's chosen protocol version is one we speak.
	// Many older servers will downgrade us to 2024-11-05 — that's
	// fine, AcceptedProtocolVersions covers it.
	if !isAcceptedVersion(result.ProtocolVersion) {
		return fmt.Errorf("unsupported protocol version %q (we accept %v)", result.ProtocolVersion, AcceptedProtocolVersions)
	}
	c.protocolVersion = result.ProtocolVersion
	c.serverInfo = result.ServerInfo

	// Send the required "we're ready" notification. Per spec this
	// MUST happen before we send any other request, and the server
	// is not allowed to send us anything except ping/logging before
	// it arrives.
	initParams, _ := json.Marshal(map[string]interface{}{})
	if err := c.transport.SendNotification(&Notification{
		JSONRPC: "2.0",
		Method:  "notifications/initialized",
		Params:  initParams,
	}); err != nil {
		return fmt.Errorf("send initialized notification: %w", err)
	}

	c.initialised = true
	return nil
}

// ListTools returns every tool the server advertises. Handles
// pagination transparently by chasing nextCursor until the server
// returns an empty one. Typically called once at server startup and
// cached by the Registry; callers that need fresh data can call
// again at any time.
func (c *Client) ListTools(ctx context.Context) ([]Tool, error) {
	if !c.initialised {
		return nil, errors.New("client not initialised")
	}

	var all []Tool
	cursor := ""
	for {
		params := ListToolsParams{Cursor: cursor}
		var result ListToolsResult
		if err := c.call(ctx, "tools/list", params, &result); err != nil {
			return nil, fmt.Errorf("tools/list: %w", err)
		}
		all = append(all, result.Tools...)
		if result.NextCursor == "" {
			return all, nil
		}
		cursor = result.NextCursor
		// Safety: cap pagination at something reasonable to prevent
		// a buggy server from spinning forever. 1000 pages × a few
		// tools per page is way beyond anything realistic.
		if len(all) > 10000 {
			return all, fmt.Errorf("tools/list pagination exceeded 10000 tools, aborting")
		}
	}
}

// CallTool invokes a tool and returns the result. The error return
// separates transport/protocol errors (nil result, non-nil err) from
// tool execution errors (non-nil result with IsError=true, nil err).
// The caller is expected to surface tool execution errors back to
// the LLM as a failed tool result so it can adapt, while
// transport/protocol errors should be raised as run failures.
//
// This mapping is important: JSON-RPC errors mean "something is
// structurally wrong" (unknown tool, bad args, server crash) and
// should fail the agent run. Tool errors mean "the tool ran but
// didn't like what happened" (API rate limit, file not found) and
// the LLM should see them so it can retry or apologise.
func (c *Client) CallTool(ctx context.Context, name string, arguments map[string]interface{}) (*CallToolResult, error) {
	if !c.initialised {
		return nil, errors.New("client not initialised")
	}
	if arguments == nil {
		// Spec: arguments must be an object, not null. Pass an
		// empty object if the caller gave us nothing.
		arguments = map[string]interface{}{}
	}
	params := CallToolParams{Name: name, Arguments: arguments}
	var result CallToolResult
	if err := c.call(ctx, "tools/call", params, &result); err != nil {
		return nil, fmt.Errorf("tools/call %q: %w", name, err)
	}
	return &result, nil
}

// ProtocolVersion returns the version the server accepted during
// initialize, or "" if Initialize hasn't been called. Useful for
// diagnostics in the Settings UI.
func (c *Client) ProtocolVersion() string { return c.protocolVersion }

// ServerInfo returns identity information the server reported during
// initialize. Empty ServerInfo if Initialize hasn't been called.
func (c *Client) ServerInfo() ServerInfo { return c.serverInfo }

// Close shuts down the underlying transport. Safe to call multiple
// times.
func (c *Client) Close() error {
	if c.transport == nil {
		return nil
	}
	return c.transport.Close()
}

// call is the private implementation shared by all RPC-style methods.
// It marshals params, assigns a fresh ID, sends via the transport,
// and unmarshals the result into out. Returns a non-nil error if
// either the transport call fails OR the JSON-RPC response contains
// an error payload. Tool-level errors (isError in the result) are
// NOT surfaced here — callers that care (like CallTool) must inspect
// the decoded result themselves.
func (c *Client) call(ctx context.Context, method string, params interface{}, out interface{}) error {
	paramsJSON, err := json.Marshal(params)
	if err != nil {
		return fmt.Errorf("marshal params: %w", err)
	}
	id := c.nextID.Add(1)
	req := &Request{
		JSONRPC: "2.0",
		ID:      id,
		Method:  method,
		Params:  paramsJSON,
	}

	// Apply a default timeout if the caller didn't set one. This is
	// a safety net — not a substitute for per-operation timeouts
	// set by the Registry.
	if _, hasDeadline := ctx.Deadline(); !hasDeadline {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, 60*time.Second)
		defer cancel()
	}

	resp, err := c.transport.SendRequest(ctx, req)
	if err != nil {
		return err
	}
	if resp.Error != nil {
		return resp.Error
	}
	if out != nil && len(resp.Result) > 0 {
		if err := json.Unmarshal(resp.Result, out); err != nil {
			return fmt.Errorf("unmarshal result: %w", err)
		}
	}
	return nil
}

// isAcceptedVersion reports whether v is one of the protocol
// versions we know how to speak.
func isAcceptedVersion(v string) bool {
	for _, accepted := range AcceptedProtocolVersions {
		if accepted == v {
			return true
		}
	}
	return false
}
