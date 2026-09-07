package workspace

import (
	"embed"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"sort"
)

// Built-in Folder Templates ship inside the binary and are SEEDED into a
// vault's templates/ folder — once, never overwritten (Harvey's ruling,
// 2026-09-07). Templates are vault-resident by design (they ride the
// vault's backup/sync and show up in the picker like any other), so a
// built-in one is just a first copy the user then owns: edit it, delete
// it, replace it — the seeder will not put it back. The marker file
// travels with the vault, so a vault shared between devices is seeded by
// exactly one of them.
//
// Layout: builtin_templates/<Name>/{title}/… — the outer folder is what
// lands in templates/, the inner one is the template root (its name is
// the generated folder's name, with {title} replaced).

//go:embed all:builtin_templates
var builtinTemplatesFS embed.FS

const (
	builtinTemplatesDir = "builtin_templates"
	seedMarkerName      = ".bruv-builtin.json"
	seedMarkerVersion   = 1
)

// seedMarker is <vault>/templates/.bruv-builtin.json.
type seedMarker struct {
	Version int      `json:"version"`
	Seeded  []string `json:"seeded"`
}

// SeedBuiltinTemplates copies every built-in template the vault has not
// seen yet into <vault>/templates/. Returns the names it wrote. A folder
// that already exists under the same name is left untouched and recorded
// as seen; a template the user deleted after seeding stays deleted.
func (s *Service) SeedBuiltinTemplates() ([]string, error) {
	r, err := s.repo()
	if err != nil {
		return nil, err
	}
	dir := filepath.Join(r.Root, templatesDirName)
	marker, err := readSeedMarker(filepath.Join(dir, seedMarkerName))
	if err != nil {
		return nil, err
	}
	seen := map[string]bool{}
	for _, n := range marker.Seeded {
		seen[n] = true
	}

	names, err := builtinTemplateNames()
	if err != nil {
		return nil, err
	}
	var written []string
	changed := false
	for _, name := range names {
		if seen[name] {
			continue
		}
		target := filepath.Join(dir, name)
		if _, statErr := os.Stat(target); statErr == nil {
			// The user has their own folder by this name: theirs wins, forever.
			marker.Seeded = append(marker.Seeded, name)
			changed = true
			continue
		}
		if err := copyEmbeddedDir(path.Join(builtinTemplatesDir, name), target); err != nil {
			return written, fmt.Errorf("seed template %s: %w", name, err)
		}
		marker.Seeded = append(marker.Seeded, name)
		written = append(written, name)
		changed = true
	}
	if changed {
		if err := writeSeedMarker(filepath.Join(dir, seedMarkerName), marker); err != nil {
			return written, err
		}
	}
	return written, nil
}

func builtinTemplateNames() ([]string, error) {
	entries, err := fs.ReadDir(builtinTemplatesFS, builtinTemplatesDir)
	if err != nil {
		return nil, err
	}
	names := make([]string, 0, len(entries))
	for _, e := range entries {
		if e.IsDir() {
			names = append(names, e.Name())
		}
	}
	sort.Strings(names)
	return names, nil
}

func readSeedMarker(p string) (*seedMarker, error) {
	raw, err := os.ReadFile(p)
	if errors.Is(err, os.ErrNotExist) {
		return &seedMarker{Version: seedMarkerVersion}, nil
	}
	if err != nil {
		return nil, err
	}
	var m seedMarker
	if err := json.Unmarshal(raw, &m); err != nil {
		return nil, fmt.Errorf("%s: %w", p, err)
	}
	return &m, nil
}

func writeSeedMarker(p string, m *seedMarker) error {
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		return err
	}
	raw, err := json.MarshalIndent(m, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(p, append(raw, '\n'), 0o644)
}

// copyEmbeddedDir writes the embedded tree rooted at src to dst, byte for
// byte — the template engine does its own substitution at generate time.
func copyEmbeddedDir(src, dst string) error {
	return fs.WalkDir(builtinTemplatesFS, src, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(filepath.FromSlash(src), filepath.FromSlash(p))
		if err != nil {
			return err
		}
		out := filepath.Join(dst, rel)
		if d.IsDir() {
			return os.MkdirAll(out, 0o755)
		}
		raw, err := builtinTemplatesFS.ReadFile(p)
		if err != nil {
			return err
		}
		return os.WriteFile(out, raw, 0o644)
	})
}
