// Package uncliengine runs BRUV's own UNCLI (UNCLI decision 0027): the
// chat engine card and project chats can use instead of BRUV's built-in
// chat, opted into with the chat_engine setting. The headless server and
// the desktop app's local server each start one; the transport mounts
// its handler at /uncli/ behind BRUV's
// sign-in (transport/http/uncli.go). Each session is one card's or
// project's chat and gets that repo's board tools over BRUV's own MCP
// endpoint, with BRUV's Chat / Suggest / Edit mode deciding their writes.
package uncliengine

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"net/url"
	"path/filepath"
	"strings"
	"sync/atomic"

	"bruv/core/boardtools"
	"bruv/internal/config"

	"github.com/harvey-withington/uncli/app/uncli"
)

// Why the engine isn't running (Status.Reason).
const (
	ReasonNotHosted      = "not_hosted"      // this host runs no UNCLI
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

// boardServer is the name sessions know BRUV's board tools by.
const boardServer = "bruv"

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
// reason and chats keep BRUV's own engine.
func Start(configDir string, allowOrigin func(origin string, r *http.Request) bool) *Engine {
	e := &Engine{}
	core, err := uncli.Start(uncli.Options{
		DataDir:        filepath.Join(configDir, "uncli"),
		AllowOrigin:    allowOrigin,
		Name:           "BRUV",
		Hooks:          true,
		MCPServers:     e.boardTools,
		HostToolPolicy: boardToolPolicy,
	})
	var sa *uncli.ServiceAccountError
	switch {
	case errors.As(err, &sa):
		slog.Warn("uncli: not started, the server runs as a service account", "account", sa.Account)
		e.status = Status{Reason: ReasonServiceAccount, Account: sa.Account}
	case err != nil:
		slog.Warn("uncli: not started", "err", err)
		e.status = Status{Reason: ReasonFailed, Detail: err.Error()}
	default:
		slog.Info("uncli: started", "data", filepath.Join(configDir, "uncli"))
		e.core, e.status = core, Status{Ready: true}
	}
	return e
}

// ServeBoard says where sessions reach the board tools, once the server
// is listening. Sessions started before it get none.
func (e *Engine) ServeBoard(b Board) {
	if e != nil {
		e.board.Store(&b)
	}
}

// repoOf is the repo a session's chat is in: from what the page said when
// it made the session (chatMeta), or else from BRUV's record of it (a
// session made before this UNCLI restarted).
func (e *Engine) repoOf(session string) (string, bool) {
	if e.core != nil {
		if repoID, ok := RepoOfMeta(e.core.SessionMeta(session)); ok {
			return repoID, true
		}
	}
	if chat, found := config.UncliSessionChat(session); found {
		return chat.RepoID, true
	}
	return "", false
}

// boardTools gives a session the board tools of the repo its chat is in.
func (e *Engine) boardTools(session string) map[string]uncli.MCPServer {
	b := e.board.Load()
	if b == nil {
		return nil
	}
	repoID, ok := e.repoOf(session)
	if !ok {
		slog.Warn("uncli: session isn't a BRUV chat, so it gets no board tools", "session", session)
		return nil
	}
	headers := map[string]string{"Authorization": "Bearer " + b.Token, config.UncliChatHeader: session}
	if e.core != nil {
		if meta := e.core.SessionMeta(session); meta != "" {
			headers[config.UncliChatMetaHeader] = meta
		}
	}
	return map[string]uncli.MCPServer{
		boardServer: {URL: b.Base + "/repos/" + url.PathEscape(repoID) + "/mcp", Headers: headers},
	}
}

// boardToolPolicy is BRUV's Chat / Suggest / Edit mode (ai_mode) for its
// own board tools, asked at each call so a switch applies at once. Reads
// keep UNCLI's rules. Writes are refused in Chat, and otherwise run
// without UNCLI asking: in Edit they apply, and in Suggest BRUV's MCP
// endpoint stages them, for one review card when the turn ends
// (internal/mcpserver/uncli.go, shared/uncliChat.reviewStagedChanges).
// If the mode can't be read, UNCLI asks the user.
func boardToolPolicy(call uncli.HostToolCall) uncli.HostDecision {
	if call.Server != boardServer || !boardtools.IsWrite(call.Tool) {
		return uncli.HostDecision{}
	}
	cfg, err := config.LoadLLMConfig()
	if err != nil {
		return uncli.HostDecision{Decision: uncli.PolicyAsk}
	}
	if cfg.AIMode == "chat" {
		return uncli.HostDecision{Decision: uncli.PolicyDeny, Reason: config.UncliChatModeRefusal}
	}
	return uncli.HostDecision{Decision: uncli.PolicyAllow}
}

// RepoOfMeta reads the repo from a session's meta, as the pages write it
// (shared/uncliChat.chatMeta): "bruv/<repo id>/card/<card id>" or
// "bruv/<repo id>/project/<brand>/<stream>/<project>".
func RepoOfMeta(meta string) (string, bool) {
	parts := strings.Split(meta, "/")
	if len(parts) < 4 || parts[0] != "bruv" || parts[1] == "" {
		return "", false
	}
	return parts[1], true
}

// Provider is a CLI the engine can run chats on (uncli.Provider): whether
// it can now, and its models.
type Provider struct {
	ID           string  `json:"id"`
	Name         string  `json:"name"`
	Ready        bool    `json:"ready"`
	Reason       string  `json:"reason,omitempty"` // why not, a sentence to show
	Models       []Model `json:"models"`
	DefaultModel string  `json:"default_model,omitempty"` // "" where the CLI picks
}

// Model is one of a CLI's models.
type Model struct {
	ID    string `json:"id"`
	Label string `json:"label"`
}

// Providers lists the CLIs, the default first. An engine that isn't
// running has none to list, and says why.
func (e *Engine) Providers(ctx context.Context) ([]Provider, error) {
	if e == nil || e.core == nil {
		return nil, fmt.Errorf("UNCLI isn't running on this server (%s)", e.Status().Reason)
	}
	out := []Provider{}
	for _, p := range e.core.Providers(ctx) {
		q := Provider{ID: p.ID, Name: p.Name, Ready: p.Ready, Reason: p.Reason, DefaultModel: p.DefaultModel, Models: []Model{}}
		for _, m := range p.Models {
			q.Models = append(q.Models, Model{ID: m.ID, Label: m.Label})
		}
		out = append(out, q)
	}
	return out, nil
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
