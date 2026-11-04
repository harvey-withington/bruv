// Package uncliengine runs BRUV's own UNCLI (UNCLI decision 0027): the
// chat engine cards can use instead of BRUV's built-in chat, opted into
// with the chat_engine setting. The headless server starts it; the
// transport mounts its handler at /uncli/ behind BRUV's sign-in
// (transport/http/uncli.go).
//
// uncli.Start sets CLAUDE_CONFIG_DIR for the whole process, so every
// child process BRUV starts afterwards inherits it. The server's are git
// and MCP stdio servers (which get a minimal environment of their own);
// a host that also runs user shell commands (the desktop app) would need
// to drop it from theirs before starting an engine.
package uncliengine

import (
	"log/slog"
	"net/http"
	"path/filepath"

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

// Engine is the server's UNCLI. One that couldn't start still answers
// Status; a nil Engine is a host that runs none.
type Engine struct {
	core   *uncli.Core
	status Status
}

// RunHook answers a CLI's hook and exits when the program was started
// for one (Options.Hooks); otherwise it returns at once. Call it first
// thing in main.
func RunHook() { uncli.RunHook() }

// Start runs the engine with its data in <configDir>/uncli. It never
// fails: an engine that can't run says why in its Status, so the chat
// setting can show the reason and cards keep BRUV's own chat.
func Start(configDir string) *Engine {
	// UNCLI refuses service accounts itself, but its error isn't exported,
	// so BRUV checks first to tell the user which account and what to do.
	if a := serviceAccount(); a != "" {
		slog.Warn("uncli: not started, the server runs as a service account", "account", a)
		return &Engine{status: Status{Reason: ReasonServiceAccount, Account: a}}
	}
	core, err := uncli.Start(uncli.Options{
		DataDir: filepath.Join(configDir, "uncli"),
		// No Origins: BRUV's guard checks each request's Origin itself
		// (the request's own host, which a fixed list can't express for a
		// server reached under several names) and drops it before the core.
		Name:  "BRUV",
		Hooks: true,
	})
	if err != nil {
		slog.Warn("uncli: not started", "err", err)
		return &Engine{status: Status{Reason: ReasonFailed, Detail: err.Error()}}
	}
	slog.Info("uncli: started", "data", filepath.Join(configDir, "uncli"))
	return &Engine{core: core, status: Status{Ready: true}}
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
