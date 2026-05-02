package http

import (
	"encoding/json"
	nethttp "net/http"
	"net/http/httptest"
	"testing"
)

func TestManifestLabelFor(t *testing.T) {
	cases := []struct {
		host string
		want string
	}{
		{"deviant.tail2ebd58.ts.net", "deviant"},
		{"ripped.tail2ebd58.ts.net", "ripped"},
		{"single-segment-host", "single-segment-host"},
		{"localhost", ""},
		{"127.0.0.1", ""},
		{"100.66.105.59", ""},
		{"::1", ""},
		{"[::1]", ""},
		{"", ""},
	}
	for _, c := range cases {
		if got := manifestLabelFor(c.host); got != c.want {
			t.Errorf("manifestLabelFor(%q) = %q, want %q", c.host, got, c.want)
		}
	}
}

func TestMobileManifestHandlerTemplatesNameFromHost(t *testing.T) {
	handler := mobileManifestHandler()

	req := httptest.NewRequest(nethttp.MethodGet, "/m/manifest.webmanifest", nil)
	req.Host = "deviant.tail2ebd58.ts.net"
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if w.Code != nethttp.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	if got := w.Header().Get("Content-Type"); got != "application/manifest+json" {
		t.Errorf("expected Content-Type application/manifest+json, got %q", got)
	}

	var manifest map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &manifest); err != nil {
		t.Fatalf("manifest is not valid JSON: %v", err)
	}
	if got := manifest["name"]; got != "BRUV — deviant" {
		t.Errorf("expected name 'BRUV — deviant', got %v", got)
	}
	if got := manifest["short_name"]; got != "BRUV deviant" {
		t.Errorf("expected short_name 'BRUV deviant', got %v", got)
	}
	// Spot-check that other manifest fields aren't dropped.
	if manifest["start_url"] != "/m/" {
		t.Errorf("expected start_url /m/, got %v", manifest["start_url"])
	}
	if manifest["scope"] != "/m/" {
		t.Errorf("expected scope /m/, got %v", manifest["scope"])
	}
}

func TestMobileManifestHandlerHonoursXForwardedHost(t *testing.T) {
	handler := mobileManifestHandler()

	req := httptest.NewRequest(nethttp.MethodGet, "/m/manifest.webmanifest", nil)
	req.Host = "127.0.0.1:9870"
	req.Header.Set("X-Forwarded-Host", "ripped.tail2ebd58.ts.net")
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	var manifest map[string]any
	_ = json.Unmarshal(w.Body.Bytes(), &manifest)
	if got := manifest["name"]; got != "BRUV — ripped" {
		t.Errorf("expected forwarded host to win, got name=%v", got)
	}
}

func TestMobileManifestHandlerDeclaresShareTargetAsGET(t *testing.T) {
	// Regression guard: a previous version declared the share_target
	// as POST + multipart/form-data, which Chrome filters out of the
	// Android share sheet when no `files` entry is declared. GET is
	// the right form for text/url/title-only shares.
	handler := mobileManifestHandler()

	req := httptest.NewRequest(nethttp.MethodGet, "/m/manifest.webmanifest", nil)
	req.Host = "deviant.tail2ebd58.ts.net"
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	var manifest map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &manifest); err != nil {
		t.Fatalf("manifest is not valid JSON: %v", err)
	}
	st, ok := manifest["share_target"].(map[string]any)
	if !ok {
		t.Fatal("expected share_target object in manifest")
	}
	if st["method"] != "GET" {
		t.Errorf("share_target.method must be GET (POST without files filters out of share sheet), got %v", st["method"])
	}
	// enctype is meaningless / ignored on GET — make sure we don't
	// accidentally re-add multipart/form-data.
	if _, present := st["enctype"]; present {
		t.Errorf("share_target.enctype shouldn't be set on GET shares; got %v", st["enctype"])
	}
	if st["action"] != "/m/share" {
		t.Errorf("expected share_target.action /m/share, got %v", st["action"])
	}
	params, ok := st["params"].(map[string]any)
	if !ok {
		t.Fatal("expected share_target.params object")
	}
	for _, k := range []string{"title", "text", "url"} {
		if _, ok := params[k]; !ok {
			t.Errorf("expected params.%s mapping", k)
		}
	}
}

func TestMobileManifestHandlerFallsBackForLoopback(t *testing.T) {
	handler := mobileManifestHandler()

	req := httptest.NewRequest(nethttp.MethodGet, "/m/manifest.webmanifest", nil)
	req.Host = "127.0.0.1:9870"
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	var manifest map[string]any
	_ = json.Unmarshal(w.Body.Bytes(), &manifest)
	// No useful label → bare "BRUV" so we don't end up with "BRUV 127" tiles.
	if got := manifest["name"]; got != "BRUV" {
		t.Errorf("expected bare 'BRUV' for loopback, got %v", got)
	}
	if got := manifest["short_name"]; got != "BRUV" {
		t.Errorf("expected bare 'BRUV' short_name for loopback, got %v", got)
	}
}
