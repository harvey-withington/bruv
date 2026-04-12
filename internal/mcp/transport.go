package mcp

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"sync"
)

// maxMessageSize caps one JSON message at 10 MiB. The default
// bufio.Scanner buffer is 64 KiB which is too small — tool results
// from filesystem or web servers easily exceed that. 10 MiB is
// generous enough that normal usage will never hit it and small
// enough that a buggy server can't OOM us.
const maxMessageSize = 10 * 1024 * 1024

// Transport is the stdio framing layer sitting between the raw
// subprocess pipes and the MCP client protocol. It handles:
//
//   - Reading newline-delimited JSON off the server's stdout and
//     dispatching each message either to a pending-request channel
//     (for responses) or to a server-request handler (for incoming
//     requests and notifications).
//   - Writing outgoing JSON under a mutex so concurrent callers never
//     interleave partial messages.
//   - Sending -32601 replies for any server→client request the handler
//     rejects, so the server doesn't hang waiting forever.
//   - Forwarding the server's stderr to BRUV's normal log output with
//     a [mcp:<name>] prefix for debugging.
//
// Transport is NOT aware of MCP semantics (initialize, tools, etc.) —
// it just moves framed JSON messages. The Client layer sits above
// this and knows about MCP method names.
type Transport struct {
	name   string // human-readable identifier used in log output
	stdin  io.WriteCloser
	stdout io.ReadCloser
	stderr io.ReadCloser

	// writeMu serialises all outbound writes. Go's os.File.Write is
	// not guaranteed atomic for partial writes, and we need whole
	// JSON messages to land as one contiguous chunk so the server
	// can parse them line by line.
	writeMu sync.Mutex

	// pending maps our outgoing request ID to the channel that will
	// receive the matching response. Access guarded by pendingMu.
	pending   map[int64]chan *Response
	pendingMu sync.Mutex

	// handler is called for every server→client request that has an
	// ID (needs a response). Notifications are logged and dropped —
	// we declare empty client capabilities so well-behaved servers
	// won't send any handler-requiring requests, but misbehaving
	// servers get a -32601 reply.
	handler func(req *Response) *ErrorObject

	// closed is set once by Close() to prevent further writes and to
	// make the read loop exit cleanly on stdout EOF.
	closed   bool
	closedMu sync.Mutex
}

// NewTransport wraps a subprocess's pipes in a Transport. The caller
// is responsible for spawning the subprocess and passing the three
// pipes — this keeps Transport testable with pure io.Pipe pairs.
//
// handler receives any server→client request that needs a response.
// Returning a non-nil ErrorObject causes that error to be sent back
// to the server; returning nil sends an empty success result. If
// handler is nil, every incoming request gets -32601 Method not found.
func NewTransport(name string, stdin io.WriteCloser, stdout io.ReadCloser, stderr io.ReadCloser, handler func(*Response) *ErrorObject) *Transport {
	return &Transport{
		name:    name,
		stdin:   stdin,
		stdout:  stdout,
		stderr:  stderr,
		pending: make(map[int64]chan *Response),
		handler: handler,
	}
}

// Start kicks off the two background goroutines that pump data:
//
//   - readLoop reads lines from stdout and dispatches them.
//   - stderrLoop forwards stderr to the BRUV log.
//
// Both exit when their underlying pipe closes. The caller should
// call Close() before the subprocess exits to drain outstanding
// pending requests cleanly.
func (t *Transport) Start() {
	go t.readLoop()
	if t.stderr != nil {
		go t.stderrLoop()
	}
}

// readLoop is the main incoming dispatcher. It reads one line at a
// time from stdout, parses each as a Response (our permissive type
// that covers both responses and incoming requests/notifications),
// and routes it based on which fields are populated:
//
//   - ID + Method set → incoming server request. Call the handler if
//     we have one, then send a response (or -32601) back.
//   - ID set, Method unset, Result/Error set → response to one of our
//     pending outgoing requests. Look up the channel and deliver.
//   - ID unset, Method set → notification. Log and drop.
//   - anything else → log as malformed.
func (t *Transport) readLoop() {
	scanner := bufio.NewScanner(t.stdout)
	scanner.Buffer(make([]byte, 64*1024), maxMessageSize)

	for scanner.Scan() {
		line := scanner.Bytes()
		if len(line) == 0 {
			continue
		}
		var msg Response
		if err := json.Unmarshal(line, &msg); err != nil {
			log.Printf("mcp[%s]: malformed message: %v: %s", t.name, err, string(line))
			continue
		}

		switch {
		case msg.ID != nil && msg.Method != "":
			// Incoming server→client request.
			t.handleIncomingRequest(&msg)
		case msg.ID != nil && msg.Method == "":
			// Response to one of our outgoing requests.
			t.deliverResponse(&msg)
		case msg.ID == nil && msg.Method != "":
			// Server-initiated notification. We deliberately drop
			// all of these for a tools-only client — see package
			// doc. logging/message is the only one worth surfacing
			// but logging those at BRUV level spams the console
			// for chatty servers. Drop silently.
			continue
		default:
			log.Printf("mcp[%s]: unclassifiable message: %s", t.name, string(line))
		}
	}

	// Scanner exit — either EOF (normal shutdown) or an error. Drain
	// any pending requests with a transport-error response so callers
	// don't block forever.
	if err := scanner.Err(); err != nil {
		log.Printf("mcp[%s]: read loop error: %v", t.name, err)
	}
	t.drainPending(errors.New("transport closed"))
}

// stderrLoop forwards the subprocess's stderr to BRUV's log so
// diagnostic output from the server is visible to users via the
// normal log stream. One log line per stderr line.
func (t *Transport) stderrLoop() {
	scanner := bufio.NewScanner(t.stderr)
	scanner.Buffer(make([]byte, 64*1024), maxMessageSize)
	for scanner.Scan() {
		log.Printf("mcp[%s] stderr: %s", t.name, scanner.Text())
	}
}

// handleIncomingRequest processes a server→client request that has
// an ID (needs a response). If no handler is configured or the
// handler returns an error, the response is -32601 Method not found
// or the returned error respectively. If the handler returns nil,
// we send a trivial success response with an empty result object.
func (t *Transport) handleIncomingRequest(req *Response) {
	var errObj *ErrorObject
	if t.handler != nil {
		errObj = t.handler(req)
	} else {
		errObj = &ErrorObject{
			Code:    ErrCodeMethodNotFound,
			Message: fmt.Sprintf("method %q not implemented by this client", req.Method),
		}
	}

	resp := outgoingResponse{
		JSONRPC: "2.0",
		ID:      *req.ID,
	}
	if errObj != nil {
		resp.Error = errObj
	} else {
		resp.Result = json.RawMessage(`{}`)
	}
	if err := t.writeJSON(&resp); err != nil {
		log.Printf("mcp[%s]: failed to send response to incoming request %q: %v", t.name, req.Method, err)
	}
}

// deliverResponse looks up the pending channel for an incoming
// response's ID and sends it. If the ID isn't in the pending map
// (because the caller timed out or was cancelled) we log and drop —
// we never want a leaked goroutine holding the map lock.
func (t *Transport) deliverResponse(resp *Response) {
	id := *resp.ID
	t.pendingMu.Lock()
	ch, ok := t.pending[id]
	if ok {
		delete(t.pending, id)
	}
	t.pendingMu.Unlock()
	if !ok {
		log.Printf("mcp[%s]: response for unknown or cancelled request id %d", t.name, id)
		return
	}
	// Non-blocking send — the receiver owns the channel and we
	// deleted it from pending so nothing else will try to write.
	ch <- resp
	close(ch)
}

// drainPending wakes every goroutine blocked in SendRequest when the
// transport shuts down so they return with the given error instead
// of hanging forever.
func (t *Transport) drainPending(err error) {
	t.pendingMu.Lock()
	defer t.pendingMu.Unlock()
	for id, ch := range t.pending {
		ch <- &Response{
			Error: &ErrorObject{
				Code:    ErrCodeInternalError,
				Message: err.Error(),
			},
		}
		close(ch)
		delete(t.pending, id)
	}
}

// SendRequest writes an outgoing request and blocks until a matching
// response arrives or ctx is cancelled. The caller is responsible for
// setting a sensible timeout via ctx — tools/list and initialize
// should use short timeouts, tools/call may need minutes for slow
// servers.
func (t *Transport) SendRequest(ctx context.Context, req *Request) (*Response, error) {
	t.closedMu.Lock()
	if t.closed {
		t.closedMu.Unlock()
		return nil, errors.New("transport closed")
	}
	t.closedMu.Unlock()

	ch := make(chan *Response, 1)
	t.pendingMu.Lock()
	t.pending[req.ID] = ch
	t.pendingMu.Unlock()

	if err := t.writeJSON(req); err != nil {
		// Roll back the pending entry — we never got on the wire.
		t.pendingMu.Lock()
		delete(t.pending, req.ID)
		t.pendingMu.Unlock()
		return nil, fmt.Errorf("write request: %w", err)
	}

	select {
	case resp := <-ch:
		return resp, nil
	case <-ctx.Done():
		// The response may still arrive later but we've given up
		// waiting. Remove the pending entry so deliverResponse
		// doesn't panic on a closed channel.
		t.pendingMu.Lock()
		delete(t.pending, req.ID)
		t.pendingMu.Unlock()
		return nil, ctx.Err()
	}
}

// SendNotification writes an outgoing notification (no id, no
// response expected). Used for notifications/initialized and nothing
// else in this client.
func (t *Transport) SendNotification(n *Notification) error {
	t.closedMu.Lock()
	if t.closed {
		t.closedMu.Unlock()
		return errors.New("transport closed")
	}
	t.closedMu.Unlock()
	return t.writeJSON(n)
}

// Close shuts down the transport gracefully. It closes stdin (which
// is MCP's shutdown signal — see package doc) and flips the closed
// flag so future SendRequest calls fail fast. It does NOT wait for
// the read loop to exit; the caller (server.go) does that via the
// os.Process wait semantics.
func (t *Transport) Close() error {
	t.closedMu.Lock()
	defer t.closedMu.Unlock()
	if t.closed {
		return nil
	}
	t.closed = true
	if t.stdin != nil {
		return t.stdin.Close()
	}
	return nil
}

// writeJSON marshals msg to JSON, appends a newline, and writes it to
// stdin under the write mutex. This is the ONE place where outbound
// bytes hit the wire — every SendRequest/SendNotification goes
// through here — so we can guarantee message atomicity.
func (t *Transport) writeJSON(msg interface{}) error {
	data, err := json.Marshal(msg)
	if err != nil {
		return fmt.Errorf("marshal: %w", err)
	}
	if bytesContainNewline(data) {
		// MCP stdio spec: messages MUST NOT contain embedded
		// newlines. json.Marshal never produces them for compact
		// output but we check anyway — a buggy custom MarshalJSON
		// implementation could and it would silently corrupt the
		// stream.
		return errors.New("marshalled message contains embedded newline")
	}
	data = append(data, '\n')

	t.writeMu.Lock()
	defer t.writeMu.Unlock()
	if t.stdin == nil {
		return errors.New("stdin closed")
	}
	_, err = t.stdin.Write(data)
	return err
}

// outgoingResponse is the shape we use when replying to incoming
// server→client requests. Same fields as Response but with a
// non-pointer ID (required for responses) and only the fields we
// populate on the outbound path.
type outgoingResponse struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      int64           `json:"id"`
	Result  json.RawMessage `json:"result,omitempty"`
	Error   *ErrorObject    `json:"error,omitempty"`
}

// bytesContainNewline checks the marshalled JSON for stray newlines.
// Kept separate so the spec check in writeJSON reads cleanly.
func bytesContainNewline(data []byte) bool {
	for _, b := range data {
		if b == '\n' {
			return true
		}
	}
	return false
}
