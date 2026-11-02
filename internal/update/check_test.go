package update

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestCompare(t *testing.T) {
	for _, c := range []struct {
		a, b string
		want int
	}{
		{"0.4.0-alpha", "0.3.0-alpha", 1},
		{"0.3.0-alpha", "0.3.0-alpha", 0},
		{"0.3.0", "0.3.0-alpha", 1},        // a release beats its pre-releases
		{"0.3.0-beta", "0.3.0-alpha", 1},   // words compare as text
		{"0.3.0-rc.10", "0.3.0-rc.2", 1},   // numbers as numbers
		{"0.3.0-rc.1", "0.3.0-rc", 1},      // more parts win when the rest match
		{"0.10.0-alpha", "0.9.0-alpha", 1}, // not as text
		{"1.0.0-alpha", "0.99.0", 1},
		// A test build is the release it was built after: offered the next
		// one, never the same or an older one.
		{"0.3.0-alpha+dev.66", "0.3.0-alpha", 0},
		{"0.4.0-alpha", "0.3.0-alpha+dev.66", 1},
		{"0.3.0-alpha+dev.66", "0.2.0-alpha", 1},
		// BRUV tags and AppVersion carry a leading "v".
		{"v0.3.0-alpha", "0.3.0-alpha", 0},
		{"v0.3.0-alpha", "v0.2.0-alpha", 1},
	} {
		if got := Compare(c.a, c.b); got != c.want {
			t.Errorf("Compare(%s, %s) = %d, want %d", c.a, c.b, got, c.want)
		}
		if got := Compare(c.b, c.a); got != -c.want {
			t.Errorf("Compare(%s, %s) = %d, want %d", c.b, c.a, got, -c.want)
		}
	}
}

func TestValid(t *testing.T) {
	for v, ok := range map[string]bool{
		"0.3.0-alpha":        true,
		"1.2.3":              true,
		"0.3.0-alpha+dev.66": true,
		"v0.2.0-alpha":       true,
		"dev":                false,
		"dev-a4d7860":        false,
		"v1.0a-dev":          false,
		"1.2":                false,
		"1.x.3":              false,
		"":                   false,
	} {
		if Valid(v) != ok {
			t.Errorf("Valid(%q) = %v", v, !ok)
		}
	}
}

// fakeLatest serves a GitHub "latest release" response with the given tag
// and points ReleasesURL at it for the duration of the test.
func fakeLatest(t *testing.T, tag string) {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(githubRelease{TagName: tag, HTMLURL: "https://example.invalid/" + tag})
	}))
	t.Cleanup(srv.Close)
	prev := ReleasesURL
	ReleasesURL = srv.URL
	t.Cleanup(func() { ReleasesURL = prev })
}

func TestCheckOffersNewerRelease(t *testing.T) {
	fakeLatest(t, "v0.3.0-alpha")
	if got := Check("v0.2.0-alpha").Status; got != "update_available" {
		t.Errorf("v0.2.0-alpha vs v0.3.0-alpha: status %q, want update_available", got)
	}
	if got := Check("v0.3.0-alpha").Status; got != "up_to_date" {
		t.Errorf("v0.3.0-alpha vs v0.3.0-alpha: status %q, want up_to_date", got)
	}
}

// A dev-stamped build can't be ordered against a release, so it must never
// be offered one — whatever the latest release is.
func TestCheckNeverOffersUpdateToDevBuild(t *testing.T) {
	fakeLatest(t, "v99.0.0")
	for _, v := range []string{"dev-a4d7860", "v1.0a-dev", "dev", ""} {
		res := Check(v)
		if res.Status == "update_available" {
			t.Errorf("Check(%q) offered an update", v)
		}
		if res.Status != "dev_build" {
			t.Errorf("Check(%q) = status %q, want dev_build", v, res.Status)
		}
		if res.CurrentVersion != v {
			t.Errorf("Check(%q).CurrentVersion = %q", v, res.CurrentVersion)
		}
	}
}
