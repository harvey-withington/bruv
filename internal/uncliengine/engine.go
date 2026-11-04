// Package uncliengine runs BRUV's own UNCLI (UNCLI decision 0027): the
// chat engine cards can use instead of BRUV's built-in chat, opted into
// with the chat_engine setting. The headless server starts it; the
// transport mounts its handler at /uncli/ behind BRUV's sign-in
// (transport/http/uncli.go). Each session is one card's chat
// (config.SetCardUncliSession) and gets that repo's board tools over
// BRUV's own MCP endpoint.
package uncliengine

import (
	"log/slog"
	"net/http"
	"net/url"
	"path/filepath"
	"sync/atomic"
	"time"

	"bruv/internal/config"

	"github.com/harvey-withington/uncli/app/uncli"
)

// Why the engine isn't running (Status.Reason).
const (
	ReasonNotHosted      = "not_hosted"      // this host runs no UNCLI (the desktop app's own server)
	ReasonServiceAccount = "service_account" // the server runs as a Windows service account
	ReasonFailed         = "failed"          // it failed to start (Status.Detail)
)

// Status is whether the engine runs, and why not.
type Status struct {
	Ready   bool   `json:"ready"`
	Reason  string `json:"reason,omitempty"`
	Account string `json:"account,omitempty"` // the service account, for ReasonServiceAccount
	Detail  string `json:"detail,omitempty"`  // the error, for ReasonFailed
}

// Board is where sessions reach BRUV's board tools: the server's own
// address on loopback and a device token for it.
type Board struct {
	Base  string // http://127.0.0.1:9870
	Token string
}

// sessionCardWait bounds how long a starting CLI waits for the page to
// record which card its new session is (config.UncliSessionCard).
const sessionCardWait = 5 * time.Second

// Engine is the server's UNCLI. One that couldn't start still answers
// Status; a nil Engine is a host that runs none.
type Engine struct {
	core   *uncli.Core
	status Status
	board  atomic.Pointer[Board]
}

// RunHook answers a CLI's hook and exits when the program was started
// for one (Options.Hooks); otherwise it returns at once. Call it first
// thing in main.
func RunHook() { uncli.RunHook() }

// Start runs the engine with its data in <configDir>/uncli, admitting
// pages whose origin allowOrigin accepts. It never fails: an engine that
// can't run says why in its Status, so the chat setting can show the
// reason and cards keep BRUV's own chat.
func Start(configDir string, allowOrigin func(origin string, r *http.Request) bool) *Engine {
	// UNCLI refuses service accounts itself, but its error isn't exported,
	// so BRUV checks first to tell the user which account and what to do.
	if a := serviceAccount(); a != "" {
		slog.Warn("uncli: not started, the server runs as a service account", "account", a)
		return &Engine{status: Status{Reason: ReasonServiceAccount, Account: a}}
	}
	e := &Engine{}
	core, err := uncli.Start(uncli.Options{
		DataDir:     filepath.Join(configDir, "uncli"),
		AllowOrigin: allowOrigin,
		Name:        "BRUV",
		Hooks:       true,
		MCPServers:  e.boardTools,
	})
	if err != nil {
		slog.Warn("uncli: not started", "err", err)
		return &Engine{status: Status{Reason: ReasonFailed, Detail: err.Error()}}
	}
	slog.Info("uncli: started", "data", filepath.Join(configDir, "uncli"))
	e.core, e.status = core, Status{Ready: true}
	return e
}

// ServeBoard says where sessions reach the board tools, once the server
// is listening. Sessions started before it get none.
func (e *Engine) ServeBoard(b Board) {
	if e != nil {
		e.board.Store(&b)
	}
}

// boardTools gives a session the board tools of the repo its card is in.
func (e *Engine) boardTools(session string) map[string]uncli.MCPServer {
	b := e.board.Load()
	if b == nil {
		return nil
	}
	card, ok := config.UncliSessionCard(session, sessionCardWait)
	if !ok {
		slog.Warn("uncli: session isn't a card's chat, so it gets no board tools", "session", session)
		return nil
	}
	return map[string]uncli.MCPServer{
		"bruv": {
			URL:     b.Base + "/repos/" + url.PathEscape(card.RepoID) + "/mcp",
			Headers: map[string]string{"Authorization": "Bearer " + b.Token},
		},
	}
}

// Status says whether the engine runs, and why not.
func (e *Engine) Status() Status {
	if e == nil {
		return Status{Reason: ReasonNotHosted}
	}
	return e.status
}

// Handler is UNCLI's routes, for the transport to mount behind BRUV's
// sign-in; nil when the engine isn't running.
func (e *Engine) Handler() http.Handler {
	if e == nil || e.core == nil {
		return nil
	}
	return e.core.Handler()
}

// Close stops the sessions and the CLIs they run.
func (e *Engine) Close() {
	if e != nil && e.core != nil {
		e.core.Close()
	}
}
