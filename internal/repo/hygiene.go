package repo

import (
	"os"
	"path/filepath"
)

// EnsureSyncHygiene writes the canonical `.gitignore` and
// `.gitattributes` files into a repo root so users who put the repo
// under git / Syncthing / Dropbox get sensible defaults out of the
// box. Idempotent — leaves existing files alone so users who have
// customised either file don't get silently clobbered.
//
// Files shipped:
//
//   - `.gitignore` excludes `.bruv/` (the SQLite index, caches, lock
//     file). These are derived state and should be rebuilt from the
//     synced repo data on another machine.
//
//   - `.gitattributes` forces LF line endings for all JSON files.
//     Windows + Unix users sharing a repo via git without this are
//     guaranteed to hit mysterious merge conflicts on every commit.
func EnsureSyncHygiene(root string) error {
	files := []struct {
		name, contents string
	}{
		{
			name: ".gitignore",
			contents: `# BRUV: ignore derived state.
# The SQLite index, caches, and lock file are rebuilt on demand.
.bruv/
`,
		},
		{
			name: ".gitattributes",
			contents: `# BRUV: force LF line endings on JSON across Windows/Unix.
# Without this, CRLF-aware git on Windows and LF-only git on Unix
# will flag every JSON file as modified and produce merge pain.
*.json text eol=lf
`,
		},
	}

	for _, f := range files {
		path := filepath.Join(root, f.name)
		if _, err := os.Stat(path); err == nil {
			continue // leave existing files alone
		}
		if err := os.WriteFile(path, []byte(f.contents), 0o644); err != nil {
			return err
		}
	}
	return nil
}
