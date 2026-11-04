package uncliengine

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"bruv/internal/config"
)

// A host without an engine (the desktop app's own server) says so.
func TestNilEngine(t *testing.T) {
	var e *Engine
	if s := e.Status(); s.Ready || s.Reason != ReasonNotHosted {
		t.Errorf("status = %+v", s)
	}
	if e.Handler() != nil {
		t.Error("a nil engine has a handler")
	}
	e.ServeBoard(Board{})
	e.Close()
}

// The engine runs a real UNCLI in BRUV's config folder and answers
// through its handler.
func TestStart(t *testing.T) {
	if serviceAccount() != "" {
		t.Skip("running as a service account")
	}
	e := Start(t.TempDir(), nil)
	t.Cleanup(e.Close)
	if s := e.Status(); !s.Ready {
		t.Fatalf("status = %+v", s)
	}
	rec := httptest.NewRecorder()
	e.Handler().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/version", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("/version = %d %s", rec.Code, rec.Body.String())
	}
}

// A card's session gets its repo's board tools with the engine's token;
// a session that isn't a card's chat gets none.
func TestBoardTools(t *testing.T) {
	config.SetConfigDir(t.TempDir())
	t.Cleanup(func() { config.SetConfigDir("") })

	e := &Engine{}
	if e.boardTools("s1") != nil {
		t.Error("tools before the server serves them")
	}
	e.ServeBoard(Board{Base: "http://127.0.0.1:9870", Token: "tok"})
	if err := config.SetCardUncliSession("repo-1", "card-1", "s1"); err != nil {
		t.Fatal(err)
	}
	got := e.boardTools("s1")["bruv"]
	if got.URL != "http://127.0.0.1:9870/repos/repo-1/mcp" || got.Headers["Authorization"] != "Bearer tok" {
		t.Errorf("board tools = %+v", got)
	}
}
