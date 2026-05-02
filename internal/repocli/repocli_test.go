package repocli

import (
	"bytes"
	"strings"
	"testing"

	"bruv/internal/config"
)

// withTempConfigDir points the config package at a temp dir for the
// duration of a test, then resets it. Lets each test work with its own
// repos.json without trampling the user's real registry.
func withTempConfigDir(t *testing.T) {
	t.Helper()
	dir := t.TempDir()
	config.SetConfigDir(dir)
	t.Cleanup(func() { config.SetConfigDir("") })
}

func seed(t *testing.T, entries []config.RepoEntry) {
	t.Helper()
	if err := config.SaveRepos(config.ReposStore{Repos: entries}); err != nil {
		t.Fatalf("seed: %v", err)
	}
}

func TestResolveIDExactMatch(t *testing.T) {
	withTempConfigDir(t)
	seed(t, []config.RepoEntry{
		{ID: "abcd1234-aaaa-aaaa-aaaa-aaaaaaaaaaaa", Name: "First", Path: "/v/a"},
		{ID: "ef560000-bbbb-bbbb-bbbb-bbbbbbbbbbbb", Name: "Second", Path: "/v/b"},
	})

	got, err := resolveID("abcd1234-aaaa-aaaa-aaaa-aaaaaaaaaaaa")
	if err != nil {
		t.Fatalf("exact ID: %v", err)
	}
	if got != "abcd1234-aaaa-aaaa-aaaa-aaaaaaaaaaaa" {
		t.Errorf("expected exact ID, got %q", got)
	}
}

func TestResolveIDPrefixMatch(t *testing.T) {
	withTempConfigDir(t)
	seed(t, []config.RepoEntry{
		{ID: "abcd1234-aaaa-aaaa-aaaa-aaaaaaaaaaaa", Name: "First", Path: "/v/a"},
		{ID: "ef560000-bbbb-bbbb-bbbb-bbbbbbbbbbbb", Name: "Second", Path: "/v/b"},
	})

	got, err := resolveID("abcd")
	if err != nil {
		t.Fatalf("prefix: %v", err)
	}
	if got != "abcd1234-aaaa-aaaa-aaaa-aaaaaaaaaaaa" {
		t.Errorf("expected prefix to resolve, got %q", got)
	}
}

func TestResolveIDNameMatch(t *testing.T) {
	withTempConfigDir(t)
	seed(t, []config.RepoEntry{
		{ID: "abcd1234-aaaa-aaaa-aaaa-aaaaaaaaaaaa", Name: "First", Path: "/v/a"},
		{ID: "ef560000-bbbb-bbbb-bbbb-bbbbbbbbbbbb", Name: "Second", Path: "/v/b"},
	})

	got, err := resolveID("Second")
	if err != nil {
		t.Fatalf("name match: %v", err)
	}
	if got != "ef560000-bbbb-bbbb-bbbb-bbbbbbbbbbbb" {
		t.Errorf("expected name to resolve, got %q", got)
	}
}

func TestResolveIDPrefixTooShort(t *testing.T) {
	withTempConfigDir(t)
	seed(t, []config.RepoEntry{
		{ID: "abcd1234-aaaa-aaaa-aaaa-aaaaaaaaaaaa", Name: "First", Path: "/v/a"},
	})

	// 3 chars is below the min prefix length — guard against unintended
	// "a" matching everything.
	_, err := resolveID("abc")
	if err == nil {
		t.Fatal("expected short prefix to be rejected")
	}
}

func TestResolveIDAmbiguousPrefix(t *testing.T) {
	withTempConfigDir(t)
	seed(t, []config.RepoEntry{
		{ID: "abcd1111-aaaa-aaaa-aaaa-aaaaaaaaaaaa", Name: "First", Path: "/v/a"},
		{ID: "abcd2222-bbbb-bbbb-bbbb-bbbbbbbbbbbb", Name: "Second", Path: "/v/b"},
	})

	_, err := resolveID("abcd")
	if err == nil {
		t.Fatal("expected ambiguous prefix to error")
	}
	if !strings.Contains(err.Error(), "ambiguous") {
		t.Errorf("expected 'ambiguous' in error, got: %v", err)
	}
}

func TestResolveIDEmptyRegistry(t *testing.T) {
	withTempConfigDir(t)
	// no seed — registry is empty
	_, err := resolveID("anything")
	if err == nil {
		t.Fatal("expected empty registry to error")
	}
}

func TestRunListEmpty(t *testing.T) {
	withTempConfigDir(t)
	var out, errOut bytes.Buffer
	code := Run([]string{"list"}, &out, &errOut)
	if code != 0 {
		t.Fatalf("expected exit 0, got %d (stderr: %s)", code, errOut.String())
	}
	if !strings.Contains(out.String(), "No vaults registered") {
		t.Errorf("expected empty-registry message, got: %s", out.String())
	}
}

func TestRunUnknownSubcommand(t *testing.T) {
	withTempConfigDir(t)
	var out, errOut bytes.Buffer
	code := Run([]string{"frobnicate"}, &out, &errOut)
	if code != 2 {
		t.Errorf("expected exit 2 for unknown subcommand, got %d", code)
	}
	if !strings.Contains(errOut.String(), "unknown") {
		t.Errorf("expected 'unknown' in stderr, got: %s", errOut.String())
	}
}

func TestRunSetDisabledRoundTrip(t *testing.T) {
	withTempConfigDir(t)
	seed(t, []config.RepoEntry{
		{ID: "abcd1234-aaaa-aaaa-aaaa-aaaaaaaaaaaa", Name: "Test", Path: "/v/t", Disabled: true},
	})

	var out, errOut bytes.Buffer
	code := Run([]string{"enable", "abcd1234"}, &out, &errOut)
	if code != 0 {
		t.Fatalf("enable: exit %d (stderr: %s)", code, errOut.String())
	}

	store, err := config.LoadRepos()
	if err != nil {
		t.Fatalf("LoadRepos: %v", err)
	}
	if store.Repos[0].Disabled {
		t.Error("expected repo to be enabled after `enable`")
	}

	// And back the other way.
	out.Reset()
	errOut.Reset()
	code = Run([]string{"disable", "Test"}, &out, &errOut)
	if code != 0 {
		t.Fatalf("disable: exit %d (stderr: %s)", code, errOut.String())
	}
	store, _ = config.LoadRepos()
	if !store.Repos[0].Disabled {
		t.Error("expected repo to be disabled after `disable`")
	}
}
