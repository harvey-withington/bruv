package http

// End-to-end tests for the workspace git transport, driven by the real git
// binary. Hand-rolling a pkt-line assertion would prove the bytes match
// what I *think* git wants; cloning proves it works, which is the only
// claim worth making about a protocol implementation.

import (
	nethttp "net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func gitOrSkip(t *testing.T) {
	t.Helper()
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git binary not available")
	}
}

func runGit(t *testing.T, dir string, args ...string) string {
	t.Helper()
	full := append([]string{"-C", dir,
		"-c", "user.name=Test", "-c", "user.email=test@localhost",
		"-c", "commit.gpgsign=false"}, args...)
	out, err := exec.Command("git", full...).CombinedOutput()
	if err != nil {
		t.Fatalf("git %s: %v\n%s", strings.Join(args, " "), err, out)
	}
	return strings.TrimSpace(string(out))
}

// publishedRepo makes a repository with one commit, configured the way
// git_serve.go configures a published workspace.
func publishedRepo(t *testing.T, files map[string]string) string {
	t.Helper()
	dir := t.TempDir()
	for name, content := range files {
		path := filepath.Join(dir, filepath.FromSlash(name))
		os.MkdirAll(filepath.Dir(path), 0o755)
		if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	runGit(t, dir, "init", "-b", "main")
	runGit(t, dir, "add", "-A")
	runGit(t, dir, "commit", "-m", "initial")
	runGit(t, dir, "config", "receive.denyCurrentBranch", "updateInstead")
	return dir
}

// cloneURL is what the desktop builds for a published workspace.
func cloneURL(base, wsID string) string {
	return base + "/repos/stub/workspaces/" + wsID + "/git"
}

// gitClone clones with the device token supplied the way the shell does.
func gitClone(t *testing.T, url, token, dest string) ([]byte, error) {
	t.Helper()
	// credential.helper= / GIT_TERMINAL_PROMPT=0 mirror what the shell does
	// (app_workspace_checkout.go): without them a rejected token makes git
	// sit waiting on a credential dialog instead of failing.
	args := []string{"-c", "credential.helper="}
	if token != "" {
		args = append(args, "-c", "http.extraHeader=Authorization: Bearer "+token)
	}
	args = append(args, "clone", url, dest)
	cmd := exec.Command("git", args...)
	cmd.Env = append(cmd.Environ(), "GIT_TERMINAL_PROMPT=0")
	return cmd.CombinedOutput()
}

func TestGitCloneOverTheConnection(t *testing.T) {
	gitOrSkip(t)
	origin := publishedRepo(t, map[string]string{
		"README.md":   "# Song Alpha",
		"mix/one.txt": "take one",
	})
	_, base, token, _ := buildTestServer(t, func(rt *RepoTarget) {
		rt.GitRepos = func(id string) (string, bool) { return origin, id == "ws-1" }
	})

	dest := filepath.Join(t.TempDir(), "copy")
	if out, err := gitClone(t, cloneURL(base, "ws-1"), token, dest); err != nil {
		t.Fatalf("clone failed: %v\n%s", err, out)
	}
	for _, want := range []string{"README.md", "mix/one.txt"} {
		if _, err := os.Stat(filepath.Join(dest, filepath.FromSlash(want))); err != nil {
			t.Errorf("%s missing from the working copy: %v", want, err)
		}
	}
}

func TestGitPushBackToTheHost(t *testing.T) {
	gitOrSkip(t)
	origin := publishedRepo(t, map[string]string{"song.md": "verse one"})
	_, base, token, _ := buildTestServer(t, func(rt *RepoTarget) {
		rt.GitRepos = func(id string) (string, bool) { return origin, id == "ws-1" }
	})

	dest := filepath.Join(t.TempDir(), "copy")
	if out, err := gitClone(t, cloneURL(base, "ws-1"), token, dest); err != nil {
		t.Fatalf("clone failed: %v\n%s", err, out)
	}

	// Edit on the "laptop" and send it back.
	if err := os.WriteFile(filepath.Join(dest, "song.md"), []byte("verse one\nverse two"), 0o644); err != nil {
		t.Fatal(err)
	}
	runGit(t, dest, "add", "-A")
	runGit(t, dest, "commit", "-m", "second verse")
	out, err := exec.Command("git", "-C", dest,
		"-c", "http.extraHeader=Authorization: Bearer "+token, "push").CombinedOutput()
	if err != nil {
		t.Fatalf("push failed: %v\n%s", err, out)
	}

	// updateInstead means the host's own working tree moves with the push —
	// which is the whole point: the files on the server are now current.
	got, err := os.ReadFile(filepath.Join(origin, "song.md"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(got), "verse two") {
		t.Errorf("host working tree not updated by the push; got %q", got)
	}
}

func TestGitRequiresAuthAndChallenges(t *testing.T) {
	gitOrSkip(t)
	origin := publishedRepo(t, map[string]string{"a.txt": "one"})
	_, base, _, _ := buildTestServer(t, func(rt *RepoTarget) {
		rt.GitRepos = func(id string) (string, bool) { return origin, id == "ws-1" }
	})

	// No credential at all: refused.
	dest := filepath.Join(t.TempDir(), "copy")
	if out, err := gitClone(t, cloneURL(base, "ws-1"), "", dest); err == nil {
		t.Fatalf("an unauthenticated clone must fail\n%s", out)
	}

	// ...and the refusal must carry the Basic challenge, or git never
	// offers the credentials it holds and a hand-run clone can't work.
	resp, err := nethttp.Get(base + "/repos/stub/workspaces/ws-1/git/info/refs?service=git-upload-pack")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 401 {
		t.Errorf("status = %d, want 401", resp.StatusCode)
	}
	if got := resp.Header.Get("WWW-Authenticate"); !strings.HasPrefix(got, "Basic ") {
		t.Errorf("WWW-Authenticate = %q, want a Basic challenge", got)
	}
}

func TestGitBasicAuthAcceptsTheDeviceToken(t *testing.T) {
	gitOrSkip(t)
	origin := publishedRepo(t, map[string]string{"a.txt": "one"})
	_, base, token, _ := buildTestServer(t, func(rt *RepoTarget) {
		rt.GitRepos = func(id string) (string, bool) { return origin, id == "ws-1" }
	})

	// The hand-run path: credentials in the URL, which is all a bare
	// `git clone` can offer.
	withCreds := strings.Replace(cloneURL(base, "ws-1"), "http://", "http://bruv:"+token+"@", 1)
	dest := filepath.Join(t.TempDir(), "copy")
	if out, err := exec.Command("git", "clone", withCreds, dest).CombinedOutput(); err != nil {
		t.Fatalf("clone with URL credentials failed: %v\n%s", err, out)
	}
	if _, err := os.Stat(filepath.Join(dest, "a.txt")); err != nil {
		t.Errorf("working copy incomplete: %v", err)
	}
}

func TestGitUnpublishedWorkspaceIsNotOnTheNetwork(t *testing.T) {
	gitOrSkip(t)
	origin := publishedRepo(t, map[string]string{"a.txt": "one"})
	_, base, token, _ := buildTestServer(t, func(rt *RepoTarget) {
		// Only ws-1 is published; ws-2 exists but isn't.
		rt.GitRepos = func(id string) (string, bool) { return origin, id == "ws-1" }
	})

	dest := filepath.Join(t.TempDir(), "copy")
	if out, err := gitClone(t, cloneURL(base, "ws-2"), token, dest); err == nil {
		t.Fatalf("an unpublished workspace must not be clonable\n%s", out)
	}
}
