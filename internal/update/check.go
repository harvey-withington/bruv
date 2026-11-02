// Package update checks GitHub Releases for new BRUV versions.
//
// This is a manual-check mechanism: the user clicks "Check for updates" in
// the About dialog, we hit the GitHub Releases API, compare versions, and
// return a result the frontend can render. We deliberately do not download
// or apply updates automatically — that requires code signing to be safe,
// and BRUV ships unsigned during the beta.
package update

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"
)

// Default upstream GitHub repository. Exposed as a var so tests and
// alternative forks can override it without code changes.
var ReleasesURL = "https://api.github.com/repos/harvey-withington/bruv/releases/latest"

// HTTP client with a sensible timeout — GitHub's API is generally fast but
// we don't want a stuck socket to freeze the About dialog.
var httpClient = &http.Client{Timeout: 10 * time.Second}

// Result is the shape returned to the frontend. Status indicates which
// branch the UI should render; the other fields are populated selectively.
type Result struct {
	// Status is one of: "up_to_date", "update_available", "dev_build",
	// "error". "dev_build": this build isn't a release version, so there is
	// nothing to compare (the UI says update checks need a release build).
	Status string `json:"status"`

	// CurrentVersion is the version the app was built as (always populated).
	CurrentVersion string `json:"current_version"`

	// LatestVersion is the upstream tag name. Populated on success.
	LatestVersion string `json:"latest_version,omitempty"`

	// ReleaseURL is the HTML page for the release. Populated when an update
	// is available so the UI can link users to the download.
	ReleaseURL string `json:"release_url,omitempty"`

	// ReleaseNotes is the body of the release (markdown). Populated when an
	// update is available so we can preview changes.
	ReleaseNotes string `json:"release_notes,omitempty"`

	// PublishedAt is the ISO-8601 timestamp of the release. Populated on
	// success so the UI can show "released N days ago".
	PublishedAt string `json:"published_at,omitempty"`

	// Error is a human-readable reason the check failed. Populated when
	// Status == "error".
	Error string `json:"error,omitempty"`
}

// githubRelease is the subset of GitHub's release JSON we care about.
type githubRelease struct {
	TagName     string `json:"tag_name"`
	Name        string `json:"name"`
	HTMLURL     string `json:"html_url"`
	Body        string `json:"body"`
	Draft       bool   `json:"draft"`
	Prerelease  bool   `json:"prerelease"`
	PublishedAt string `json:"published_at"`
}

// Check queries the configured releases URL and compares its tag against
// currentVersion. Both may carry a leading "v"; build metadata is ignored.
//
// A current version that isn't a release version (a deploy build's
// "dev-<sha>", the unstamped "v1.0a-dev") can't be ordered against a
// release, so it is reported as "dev_build" without contacting GitHub —
// never as an available update.
func Check(currentVersion string) Result {
	result := Result{CurrentVersion: currentVersion}

	if !Valid(currentVersion) {
		result.Status = "dev_build"
		return result
	}

	req, err := http.NewRequest("GET", ReleasesURL, nil)
	if err != nil {
		result.Status = "error"
		result.Error = "could not build request: " + err.Error()
		return result
	}
	// GitHub API recommends setting Accept for forward compatibility.
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("User-Agent", "BRUV/"+currentVersion)

	resp, err := httpClient.Do(req)
	if err != nil {
		result.Status = "error"
		result.Error = "could not reach GitHub: " + err.Error()
		return result
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNotFound {
		// No releases published yet — treat as up-to-date so the dialog
		// doesn't scare users.
		result.Status = "up_to_date"
		return result
	}
	if resp.StatusCode >= 400 {
		result.Status = "error"
		result.Error = fmt.Sprintf("GitHub API returned HTTP %d", resp.StatusCode)
		return result
	}

	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20)) // 1 MiB cap
	if err != nil {
		result.Status = "error"
		result.Error = "could not read response: " + err.Error()
		return result
	}

	var rel githubRelease
	if err := json.Unmarshal(body, &rel); err != nil {
		result.Status = "error"
		result.Error = "could not parse release JSON: " + err.Error()
		return result
	}

	if rel.Draft {
		result.Status = "up_to_date"
		return result
	}

	result.LatestVersion = rel.TagName
	result.ReleaseURL = rel.HTMLURL
	result.ReleaseNotes = truncate(rel.Body, 2000)
	result.PublishedAt = rel.PublishedAt

	if Compare(currentVersion, rel.TagName) < 0 {
		result.Status = "update_available"
	} else {
		result.Status = "up_to_date"
	}
	return result
}

// truncate limits a string to n characters, appending an ellipsis if
// trimmed. Used to keep release notes from ballooning the IPC payload.
func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "…"
}

// Valid says v is a version BRUV releases use: 1.2.3, or 1.2.3 with a
// pre-release part (1.2.3-alpha, 1.2.3-rc.1), and perhaps build metadata
// (a test build's +dev.66). A leading "v" is allowed. Dev builds stamped
// "dev-<sha>" or the unstamped default "v1.0a-dev" are not valid.
func Valid(v string) bool {
	core, _, _ := strings.Cut(bare(v), "-")
	parts := strings.Split(core, ".")
	if len(parts) != 3 {
		return false
	}
	for _, p := range parts {
		if _, err := strconv.Atoi(p); err != nil || p == "" {
			return false
		}
	}
	return true
}

// Compare orders versions as semantic versioning does: by the numbers, then
// a release before any of its pre-releases (1.0.0 > 1.0.0-rc.1), whose
// parts compare as numbers when both are, else as text. Build metadata
// counts for nothing, so a test build (0.3.0-alpha+dev.66) is the release
// it was built after. A leading "v" is ignored. -1, 0 or 1.
func Compare(a, b string) int {
	ac, ap, _ := strings.Cut(bare(a), "-")
	bc, bp, _ := strings.Cut(bare(b), "-")
	an, bn := strings.Split(ac, "."), strings.Split(bc, ".")
	for i := 0; i < 3; i++ {
		if c := cmpInt(at(an, i), at(bn, i)); c != 0 {
			return c
		}
	}
	switch {
	case ap == bp:
		return 0
	case ap == "":
		return 1
	case bp == "":
		return -1
	}
	ai, bi := strings.Split(ap, "."), strings.Split(bp, ".")
	for i := 0; i < len(ai) && i < len(bi); i++ {
		x, xerr := strconv.Atoi(ai[i])
		y, yerr := strconv.Atoi(bi[i])
		var c int
		switch {
		case xerr == nil && yerr == nil:
			c = cmp(x, y)
		case xerr == nil:
			c = -1 // numbers come before words
		case yerr == nil:
			c = 1
		default:
			c = strings.Compare(ai[i], bi[i])
		}
		if c != 0 {
			return c
		}
	}
	return cmp(len(ai), len(bi))
}

// bare is a version without surrounding space, its leading "v" or its build
// metadata (+...).
func bare(v string) string {
	v = strings.TrimPrefix(strings.TrimSpace(v), "v")
	v, _, _ = strings.Cut(v, "+")
	return v
}

func at(s []string, i int) string {
	if i < len(s) {
		return s[i]
	}
	return "0"
}

func cmpInt(a, b string) int {
	x, _ := strconv.Atoi(a)
	y, _ := strconv.Atoi(b)
	return cmp(x, y)
}

func cmp(a, b int) int {
	switch {
	case a < b:
		return -1
	case a > b:
		return 1
	}
	return 0
}
