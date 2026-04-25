package repo

// Activity log — per-actor sharded JSONL.
//
// All shards live under <root>/activity/. Each writer (a user
// instance, an LLM model) appends only to its own file:
//
//   <root>/activity/<actorID>.jsonl
//
// Sharding by actor sidesteps the merge-conflict / interleaved-line
// nightmare that a single shared append-only file would produce when
// the repo is synced via git/Dropbox/Syncthing — two machines never
// touch the same file. Readers enumerate every *.jsonl in the
// directory and merge by timestamp; the legacy pre-shard file from
// older repos lives at activity/legacy.jsonl and participates in the
// merge transparently.

import (
	"bruv/internal/model"
	"bufio"
	"encoding/json"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
)

// activityDirPath returns the directory holding all per-actor shards.
func (r *Repository) activityDirPath() string {
	return filepath.Join(r.Root, activityDir)
}

// safeShardSegment is a defensive sanitiser. ActorID should be a UUID
// or model name without path separators, but treating any caller as
// trusted with filesystem paths is a foot-gun. Replace anything that
// isn't ASCII alphanumeric or `-_.` with `_`. An empty result falls
// back to "unknown" so we still write somewhere rather than silently
// dropping the entry.
var unsafeShardChars = regexp.MustCompile(`[^A-Za-z0-9._-]+`)

func shardFileName(actorID string) string {
	cleaned := unsafeShardChars.ReplaceAllString(actorID, "_")
	cleaned = strings.Trim(cleaned, ".")
	if cleaned == "" {
		cleaned = "unknown"
	}
	return cleaned + ".jsonl"
}

// AppendActivity writes a single ActivityEntry to the writer's shard
// file. The file is created on first write. Errors are silently
// swallowed — activity logging must never block a real user action.
func (r *Repository) AppendActivity(entry model.ActivityEntry) {
	dir := r.activityDirPath()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return
	}
	path := filepath.Join(dir, shardFileName(entry.ActorID))

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

// ListActivity returns up to limit entries from the activity log,
// most-recent first. Reads every *.jsonl in the activity directory
// (each writer's shard plus the legacy pre-shard file if present)
// and merges by timestamp.
func (r *Repository) ListActivity(limit int) ([]model.ActivityEntry, error) {
	dir := r.activityDirPath()
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}

	// Per-shard tail buffer cap. Each shard contributes at most this
	// many of its newest lines to the merge; afterwards we global-sort
	// and trim to `limit`. Capping per shard bounds memory regardless
	// of how many shards or how big each one is.
	const perShardCap = 1024
	tail := perShardCap
	if limit > tail {
		tail = limit
	}

	merged := make([]model.ActivityEntry, 0, len(entries)*16)
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".jsonl") {
			continue
		}
		shardEntries, err := readShardTail(filepath.Join(dir, e.Name()), tail)
		if err != nil {
			// One bad shard shouldn't take down the whole listing —
			// skip it and keep going.
			continue
		}
		merged = append(merged, shardEntries...)
	}

	// Most-recent first.
	sort.Slice(merged, func(i, j int) bool {
		return merged[i].Timestamp.After(merged[j].Timestamp)
	})
	if len(merged) > limit {
		merged = merged[:limit]
	}
	return merged, nil
}

// readShardTail reads up to tail entries from the end of one JSONL
// shard. Bounds memory per shard rather than loading the whole file.
func readShardTail(path string, tail int) ([]model.ActivityEntry, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	lines := make([]string, 0, tail*2)
	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 64*1024), 1024*1024)
	for scanner.Scan() {
		line := scanner.Text()
		if line == "" {
			continue
		}
		lines = append(lines, line)
		if len(lines) > tail*2 {
			lines = lines[len(lines)-tail:]
		}
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}
	if len(lines) > tail {
		lines = lines[len(lines)-tail:]
	}

	out := make([]model.ActivityEntry, 0, len(lines))
	for _, line := range lines {
		var e model.ActivityEntry
		if err := json.Unmarshal([]byte(line), &e); err != nil {
			continue
		}
		out = append(out, e)
	}
	return out, nil
}
