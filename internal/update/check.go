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
	"regexp"
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
	// Status is one of: "up_to_date", "update_available", "error".
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
// currentVersion. The current version string may carry a leading "v" and
// a "-dev"/"-beta" suffix — both are normalised before comparison.
func Check(currentVersion string) Result {
	result := Result{CurrentVersion: currentVersion}

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

	if compareVersions(currentVersion, rel.TagName) < 0 {
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

// versionPart captures the major/minor/patch and optional suffix stage
// of a semver-ish version tag.
var versionRegex = regexp.MustCompile(`^v?(\d+)(?:\.(\d+))?(?:\.(\d+))?(.*)$`)

// compareVersions returns -1, 0, or 1 depending on whether a is less than,
// equal to, or greater than b. Designed to handle BRUV's "v1.0b",
// "v1.0b-dev", and "v1.0.1" style tags without importing a full semver
// library — good enough for a linear release history.
//
// Rules:
//   - Leading "v" is optional and ignored.
//   - Numeric major/minor/patch segments are compared as integers.
//   - A "-dev" or "-beta" suffix on a is considered LESS than the same
//     version without the suffix (so v1.0b-dev < v1.0b).
//   - Non-numeric trailing segments (like "b" in "v1.0b") are compared
//     lexically as a tiebreaker.
func compareVersions(a, b string) int {
	am, an, ap, asuf := parseVersion(a)
	bm, bn, bp, bsuf := parseVersion(b)

	if c := intCmp(am, bm); c != 0 {
		return c
	}
	if c := intCmp(an, bn); c != 0 {
		return c
	}
	if c := intCmp(ap, bp); c != 0 {
		return c
	}
	return suffixCmp(asuf, bsuf)
}

func parseVersion(v string) (major, minor, patch int, suffix string) {
	m := versionRegex.FindStringSubmatch(strings.TrimSpace(v))
	if m == nil {
		return 0, 0, 0, v
	}
	major, _ = strconv.Atoi(m[1])
	minor, _ = strconv.Atoi(m[2])
	patch, _ = strconv.Atoi(m[3])
	suffix = m[4]
	return
}

func intCmp(a, b int) int {
	switch {
	case a < b:
		return -1
	case a > b:
		return 1
	default:
		return 0
	}
}

// suffixCmp treats "-dev" and "-beta" as earlier than no suffix, and
// otherwise falls back to lexical comparison. This matches how you'd
// want "v1.0b-dev" to sort before "v1.0b".
func suffixCmp(a, b string) int {
	// Split "b-dev" into ("b", "dev") so the character portion dominates
	// and the pre-release tag is only a tiebreaker on matching characters.
	aChar, aPre := splitSuffix(a)
	bChar, bPre := splitSuffix(b)

	if aChar != bChar {
		if aChar < bChar {
			return -1
		}
		return 1
	}
	// Equal character portion — a missing pre-release is greater than a
	// present one (v1.0b > v1.0b-dev).
	if aPre == "" && bPre != "" {
		return 1
	}
	if aPre != "" && bPre == "" {
		return -1
	}
	if aPre < bPre {
		return -1
	}
	if aPre > bPre {
		return 1
	}
	return 0
}

// splitSuffix separates a trailing "-something" pre-release tag from the
// leading character portion of the version suffix.
func splitSuffix(s string) (char, pre string) {
	if i := strings.Index(s, "-"); i >= 0 {
		return s[:i], s[i+1:]
	}
	return s, ""
}
