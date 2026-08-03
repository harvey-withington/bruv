package main

// Reading git's failures well enough to tell a person what happened.
//
// Every sample here is real output, captured by driving two clones of one
// repository (2026-08-02). The bug being pinned: "last non-empty line" —
// which reads correctly for `fatal:` errors — turns a rejected push into
// advice about a manual page, because git follows the actual reason with
// five lines of hints.

import (
	"strings"
	"testing"
)

func TestGitReason(t *testing.T) {
	cases := []struct {
		name string
		out  string
		want string
	}{
		{
			// The two-laptop collision. The reason is in the `!` line; the
			// last line is "hint: See the 'Note about fast-forwards'…".
			name: "push rejected as non-fast-forward",
			out: `To http://ripped:9870/repos/r1/workspaces/ws-1/git
 ! [rejected]        main -> main (fetch first)
error: failed to push some refs to 'http://ripped:9870/repos/r1/workspaces/ws-1/git'
hint: Updates were rejected because the remote contains work that you do not
hint: have locally. This is usually caused by another repository pushing to
hint: the same ref. If you want to integrate the remote changes, use
hint: 'git pull' before pushing again.
hint: See the 'Note about fast-forwards' in 'git push --help' for details.`,
			want: "fetch first",
		},
		{
			// Someone typed straight into the folder on the server. This is
			// the protection working, and it must read as such.
			name: "push refused because the host has uncommitted edits",
			out: `To http://ripped:9870/repos/r1/workspaces/ws-1/git
 ! [remote rejected] main -> main (Working directory has unstaged changes)
error: failed to push some refs to 'http://ripped:9870/repos/r1/workspaces/ws-1/git'`,
			want: "Working directory has unstaged changes",
		},
		{
			name: "ff-only pull on diverged branches",
			out: `hint: You have divergent branches and need to specify how to reconcile them.
hint: 	git rebase
hint:
fatal: Not possible to fast-forward, aborting.`,
			want: "Not possible to fast-forward, aborting.",
		},
		{
			name: "authentication failure",
			out: `warning: redirecting to http://ripped:9870/
fatal: Authentication failed for 'http://ripped:9870/repos/r1/workspaces/ws-1/git'`,
			want: "Authentication failed for 'http://ripped:9870/repos/r1/workspaces/ws-1/git'",
		},
		{
			// The generic wrapper restates the exit code and must never be
			// the whole message when something more specific exists.
			name: "generic push wrapper alone still says something",
			out:  `error: failed to push some refs to 'http://ripped:9870/x'`,
			want: "error: failed to push some refs to 'http://ripped:9870/x'",
		},
		{
			name: "windows line endings parse the same",
			out:  "To http://x\r\n ! [remote rejected] main -> main (Working directory has unstaged changes)\r\n",
			want: "Working directory has unstaged changes",
		},
		{
			name: "no recognisable shape falls back to the text",
			out:  "something unexpected happened",
			want: "something unexpected happened",
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := gitReason(c.out); got != c.want {
				t.Errorf("gitReason() = %q, want %q", got, c.want)
			}
		})
	}
}

// The reasons gitReason produces are what the sync methods classify on, so
// the two must agree — a wording change in either is a silent behaviour
// change otherwise.
func TestPushRejectionsClassifyAsStatuses(t *testing.T) {
	diverged := gitReason(` ! [rejected]        main -> main (fetch first)`)
	if diverged != "fetch first" {
		t.Fatalf("reason = %q", diverged)
	}
	busy := gitReason(` ! [remote rejected] main -> main (Working directory has unstaged changes)`)
	if !strings.Contains(busy, "unstaged changes") {
		t.Errorf("the server-dirty branch keys off %q — it would no longer match", busy)
	}
}

func TestFirstMeaningfulLine(t *testing.T) {
	if got := firstMeaningfulLine("warning: LF will be replaced\nUpdating a1b2c3..d4e5f6\nFast-forward"); got != "Updating a1b2c3..d4e5f6" {
		t.Errorf("got %q", got)
	}
	if got := firstMeaningfulLine(""); got != "" {
		t.Errorf("got %q, want empty", got)
	}
}
