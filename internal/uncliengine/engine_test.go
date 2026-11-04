package uncliengine

import (
	"net/http"
	"net/http/httptest"
	"testing"
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
	e.Close()
}

// The engine runs a real UNCLI in BRUV's config folder and answers
// through its handler.
func TestStart(t *testing.T) {
	if serviceAccount() != "" {
		t.Skip("running as a service account")
	}
	e := Start(t.TempDir())
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
