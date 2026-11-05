package uncliengine

import (
	"context"
	"net/http"
	"strings"
	"net/http/httptest"
	"testing"

	"bruv/internal/config"

	"github.com/harvey-withington/uncli/app/uncli"
)

// A host without an engine says so.
func TestNilEngine(t *testing.T) {
	var e *Engine
	if s := e.Status(); s.Ready || s.Reason != ReasonNotHosted {
		t.Errorf("status = %+v", s)
	}
	if e.Handler() != nil {
		t.Error("a nil engine has a handler")
	}
	if _, err := e.Providers(context.Background()); err == nil || !strings.Contains(err.Error(), ReasonNotHosted) {
		t.Errorf("a nil engine's providers: %v", err)
	}
	e.ServeBoard(Board{})
	e.Close()
}

// The engine runs a real UNCLI in BRUV's config folder and answers
// through its handler.
func TestStart(t *testing.T) {
	e := Start(t.TempDir(), nil)
	t.Cleanup(e.Close)
	if s := e.Status(); s.Reason == ReasonServiceAccount {
		t.Skip("running as a service account")
	}
	if s := e.Status(); !s.Ready {
		t.Fatalf("status = %+v", s)
	}
	rec := httptest.NewRecorder()
	e.Handler().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/version", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("/version = %d %s", rec.Code, rec.Body.String())
	}
	// Its CLIs, the default first; ready or not, each says which.
	providers, err := e.Providers(context.Background())
	if err != nil || len(providers) == 0 || providers[0].ID == "" || providers[0].Models == nil {
		t.Fatalf("providers = %+v, %v", providers, err)
	}
	for _, p := range providers {
		if !p.Ready && p.Reason == "" {
			t.Errorf("%s isn't ready and doesn't say why", p.ID)
		}
	}
}

// A chat's session gets its repo's board tools with the engine's token,
// found from BRUV's record when UNCLI has no meta for it.
func TestBoardTools(t *testing.T) {
	config.SetConfigDir(t.TempDir())
	t.Cleanup(func() { config.SetConfigDir("") })

	e := &Engine{}
	if e.boardTools("s1") != nil {
		t.Error("tools before the server serves them")
	}
	e.ServeBoard(Board{Base: "http://127.0.0.1:9870", Token: "tok"})
	if e.boardTools("s1") != nil {
		t.Error("tools for a session that isn't a BRUV chat")
	}
	if err := config.SetChatUncliSession("repo-1", "card-1", "s1"); err != nil {
		t.Fatal(err)
	}
	got := e.boardTools("s1")[boardServer]
	if got.URL != "http://127.0.0.1:9870/repos/repo-1/mcp" || got.Headers["Authorization"] != "Bearer tok" || got.Headers[config.UncliChatHeader] != "s1" {
		t.Errorf("board tools = %+v", got)
	}
}

func TestRepoOfMeta(t *testing.T) {
	for meta, want := range map[string]string{
		"bruv/repo-1/card/c1":       "repo-1",
		"bruv/repo-1/project/b/s/p": "repo-1",
		"bruv//card/c1":             "",
		"card:42":                   "",
		"":                          "",
	} {
		got, ok := RepoOfMeta(meta)
		if got != want || ok != (want != "") {
			t.Errorf("RepoOfMeta(%q) = %q, %v; want %q", meta, got, ok, want)
		}
	}
}

// BRUV's Chat / Suggest / Edit mode decides its own board tools' writes;
// reads, and every other server's tools, keep UNCLI's rules.
func TestBoardToolPolicy(t *testing.T) {
	config.SetConfigDir(t.TempDir())
	t.Cleanup(func() { config.SetConfigDir("") })
	mode := func(m string) {
		if err := config.SaveLLMConfig(config.LLMConfig{AIMode: m}); err != nil {
			t.Fatal(err)
		}
	}

	write := uncli.HostToolCall{Session: "s1", Server: boardServer, Tool: "set_card_title", Args: map[string]any{"card_id": "c1", "title": "Melbourne"}}

	mode("chat")
	if d := boardToolPolicy(write); d.Decision != uncli.PolicyDeny || d.Reason != config.UncliChatModeRefusal {
		t.Errorf("chat write = %+v", d)
	}
	if d := boardToolPolicy(uncli.HostToolCall{Session: "s1", Server: boardServer, Tool: "get_card"}); d.Decision != "" {
		t.Errorf("chat read = %+v, want UNCLI's rules", d)
	}
	if d := boardToolPolicy(uncli.HostToolCall{Session: "s1", Server: "someone-else", Tool: "set_card_title"}); d.Decision != "" {
		t.Errorf("another server's tool = %+v, want UNCLI's rules", d)
	}

	// Suggest (BRUV stages, and the page reviews the turn's changes) and
	// Edit (applied): UNCLI lets them through unasked.
	for _, m := range []string{"suggest", "edit", ""} {
		mode(m)
		if d := boardToolPolicy(write); d.Decision != uncli.PolicyAllow || d.Prompt != nil {
			t.Errorf("%q write = %+v, want allow", m, d)
		}
	}
}
