package repo

import (
	"bruv/internal/model"
	"bufio"
	"encoding/json"
	"os"
	"path/filepath"
)

const activityFile = "activity.jsonl"

func (r *Repository) activityPath() string {
	return filepath.Join(r.Root, bruvDir, activityFile)
}

// AppendActivity writes a single ActivityEntry as a JSON line to the activity log.
// The file is created on first write. Errors are silently swallowed — activity logging
// must never block a real user action.
func (r *Repository) AppendActivity(entry model.ActivityEntry) {
	path := r.activityPath()
	f, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		return
	}
	defer f.Close()

	line, err := json.Marshal(entry)
	if err != nil {
		return
	}
	_, _ = f.Write(append(line, '\n'))
}

// ListActivity returns up to limit entries from the activity log, most-recent first.
func (r *Repository) ListActivity(limit int) ([]model.ActivityEntry, error) {
	path := r.activityPath()
	f, err := os.Open(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	defer f.Close()

	// Read all lines into a ring buffer so we avoid loading the whole file into memory
	// when only a tail is needed.
	lines := make([]string, 0, limit*2)
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := scanner.Text()
		if line == "" {
			continue
		}
		lines = append(lines, line)
		// Keep only the last limit*2 lines in memory to bound usage
		if len(lines) > limit*4 {
			lines = lines[len(lines)-limit*2:]
		}
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}

	// Take the last `limit` lines
	if len(lines) > limit {
		lines = lines[len(lines)-limit:]
	}

	// Parse and reverse (most-recent first)
	entries := make([]model.ActivityEntry, 0, len(lines))
	for _, line := range lines {
		var e model.ActivityEntry
		if err := json.Unmarshal([]byte(line), &e); err != nil {
			continue
		}
		entries = append(entries, e)
	}
	// Reverse in-place
	for i, j := 0, len(entries)-1; i < j; i, j = i+1, j-1 {
		entries[i], entries[j] = entries[j], entries[i]
	}
	return entries, nil
}
