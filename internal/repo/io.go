package repo

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"

	"bruv/internal/fsutil"
)

// SanitizeText replaces reserved internal characters in user-supplied text.
// U+203A (›) is used as a delimiter in project context breadcrumbs and must
// not appear in user data.
func SanitizeText(s string) string {
	return strings.ReplaceAll(s, "\u203a", ">")
}

// readJSON reads a JSON file from disk and unmarshals it into dest.
func readJSON(path string, dest any) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("read %s: %w", path, err)
	}
	if err := json.Unmarshal(data, dest); err != nil {
		return fmt.Errorf("parse %s: %w", path, err)
	}
	return nil
}

// writeJSON atomically writes a JSON file to disk.
// Pattern: serialize → write to a unique temp file → fsync → rename over
// original (fsutil.WriteFileAtomic). A crash mid-write never produces a
// corrupt file and concurrent writers never share a temp file. A
// read-modify-write must also hold r.lockPath(path) across the sequence.
func writeJSON(path string, v any) error {
	data, err := marshalSorted(v)
	if err != nil {
		return fmt.Errorf("marshal for %s: %w", path, err)
	}

	return fsutil.WriteFileAtomic(path, data, 0o644)
}

// fileLocks serializes read-modify-write sequences per file. Package-level
// (not per Repository) because several Repository values can be open on
// the same root at once (runtime, MCP server, CLI helpers).
var fileLocks fsutil.KeyedMutex

// lockPath takes the RMW lock for one file and returns its unlock func.
// Hold it across load → mutate → writeJSON. Not reentrant: never call
// another locking method for the same file while holding it.
func lockPath(path string) func() {
	key := path
	if abs, err := filepath.Abs(path); err == nil {
		key = abs
	}
	key = filepath.Clean(key)
	if runtime.GOOS == "windows" {
		key = strings.ToLower(key)
	}
	return fileLocks.Lock(key)
}

// marshalSorted produces JSON with sorted map keys and consistent indentation
// for git-diff-friendly output.
func marshalSorted(v any) ([]byte, error) {
	data, err := json.Marshal(v)
	if err != nil {
		return nil, err
	}

	// Re-decode into an interface{} to sort map keys, then re-encode with indent
	var raw any
	if err := json.Unmarshal(data, &raw); err != nil {
		return nil, err
	}
	sortKeys(raw)

	return json.MarshalIndent(raw, "", "  ")
}

// sortKeys recursively sorts map keys in a decoded JSON value.
func sortKeys(v any) {
	switch val := v.(type) {
	case map[string]any:
		for _, v := range val {
			sortKeys(v)
		}
	case []any:
		for _, item := range val {
			sortKeys(item)
		}
	}
}

// fileExists returns true if the given path exists.
func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

// uniqueSlug appends a numeric suffix (-2, -3, …) if the base slug is already taken.
func uniqueSlug(base string, taken func(string) bool) string {
	if !taken(base) {
		return base
	}
	for i := 2; ; i++ {
		candidate := fmt.Sprintf("%s-%d", base, i)
		if !taken(candidate) {
			return candidate
		}
	}
}

// Slugify converts a name to a filesystem-safe slug.
func Slugify(name string) string {
	result := make([]byte, 0, len(name))
	prevDash := false
	for _, r := range name {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9':
			result = append(result, byte(r))
			prevDash = false
		case r >= 'A' && r <= 'Z':
			result = append(result, byte(r-'A'+'a'))
			prevDash = false
		case r == ' ' || r == '-' || r == '_' || r == '.':
			if !prevDash && len(result) > 0 {
				result = append(result, '-')
				prevDash = true
			}
		}
	}
	// Trim trailing dash
	if len(result) > 0 && result[len(result)-1] == '-' {
		result = result[:len(result)-1]
	}
	return string(result)
}

// listSubdirs returns the names of immediate subdirectories in a directory.
func listSubdirs(dir string) ([]string, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	var names []string
	for _, e := range entries {
		if e.IsDir() {
			names = append(names, e.Name())
		}
	}
	sort.Strings(names)
	return names, nil
}

// listJSONFiles returns filenames (without extension) of .json files in a directory.
func listJSONFiles(dir string) ([]string, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	var names []string
	for _, e := range entries {
		if !e.IsDir() && filepath.Ext(e.Name()) == ".json" {
			name := e.Name()
			names = append(names, name[:len(name)-5])
		}
	}
	sort.Strings(names)
	return names, nil
}
