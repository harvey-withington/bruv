package repo

import (
	"bruv/internal/config"
	"bruv/internal/model"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"
)

// Regression tests for the 2026-09-29 pre-release sweep (write
// serialization, never-write-after-failed-read, copy / rename / pin
// integrity).

const concurrentWriters = 40

func runConcurrently(n int, fn func(i int)) {
	var wg sync.WaitGroup
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			fn(i)
		}(i)
	}
	wg.Wait()
}

func TestUpdateCardConcurrentNoLostUpdates(t *testing.T) {
	r := setupTestRepo(t)
	card, _ := r.CreateCard("task", "Busy")

	errs := make(chan error, concurrentWriters)
	runConcurrently(concurrentWriters, func(i int) {
		if _, err := r.UpdateCard(card.ID, func(c *model.Card) {
			c.Tags = append(c.Tags, fmt.Sprintf("t%d", i))
		}); err != nil {
			errs <- err
		}
	})
	close(errs)
	for err := range errs {
		t.Fatalf("UpdateCard: %v", err)
	}

	got, _ := r.GetCard(card.ID)
	if len(got.Tags) != concurrentWriters {
		t.Fatalf("tags = %d, want %d (lost updates)", len(got.Tags), concurrentWriters)
	}
	leftovers, _ := filepath.Glob(filepath.Join(r.cardsPath(), "*.tmp"))
	if len(leftovers) != 0 {
		t.Errorf("temp files left behind: %v", leftovers)
	}
}

func TestMutateCardNoChangeAndAbort(t *testing.T) {
	r := setupTestRepo(t)
	card, _ := r.CreateCard("task", "Original")

	got, err := r.MutateCard(card.ID, func(c *model.Card) error {
		c.Title = "not saved"
		return ErrNoChange
	})
	if err != nil || got == nil {
		t.Fatalf("ErrNoChange: got (%v, %v), want card and nil error", got, err)
	}
	boom := errors.New("boom")
	if _, err := r.MutateCard(card.ID, func(c *model.Card) error {
		c.Title = "not saved either"
		return boom
	}); !errors.Is(err, boom) {
		t.Fatalf("abort error = %v, want boom", err)
	}
	if onDisk, _ := r.GetCard(card.ID); onDisk.Title != "Original" {
		t.Errorf("title = %q, want the card untouched", onDisk.Title)
	}
}

func TestUpdateAgentConfig(t *testing.T) {
	r := setupTestRepo(t)
	card, _ := r.CreateCard("task", "Agent")

	if _, err := r.UpdateAgentConfig(card.ID, func(*model.AgentConfig) error { return nil }); !errors.Is(err, ErrAgentNotFound) {
		t.Fatalf("missing agent: err = %v, want ErrAgentNotFound", err)
	}
	if fileExists(r.agentFilePath(card.ID)) {
		t.Fatal("UpdateAgentConfig must not create the agent file")
	}

	if err := r.SaveAgentConfig(card.ID, model.AgentConfig{Goal: "g", AllowedTools: []string{}}); err != nil {
		t.Fatal(err)
	}
	runConcurrently(concurrentWriters, func(i int) {
		_, _ = r.UpdateAgentConfig(card.ID, func(cfg *model.AgentConfig) error {
			cfg.AllowedTools = append(cfg.AllowedTools, fmt.Sprintf("tool%d", i))
			return nil
		})
	})
	af, err := r.GetAgentConfig(card.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(af.Config.AllowedTools) != concurrentWriters || af.Config.Goal != "g" {
		t.Errorf("config = %+v, want %d tools and goal kept", af.Config, concurrentWriters)
	}
}

func TestAppendAgentRunConcurrent(t *testing.T) {
	r := setupTestRepo(t)
	if err := r.SetRunsDir(t.TempDir()); err != nil {
		t.Fatal(err)
	}
	card, _ := r.CreateCard("task", "Runs")
	runConcurrently(20, func(i int) {
		_ = r.AppendAgentRun(card.ID, model.AgentRun{Status: "success"})
	})
	runs, _ := r.GetAgentRuns(card.ID)
	if len(runs) != 20 {
		t.Errorf("runs = %d, want 20", len(runs))
	}
}

func TestAddCardCommentConcurrent(t *testing.T) {
	r := setupTestRepo(t)
	card, _ := r.CreateCard("task", "Chatty")
	runConcurrently(concurrentWriters, func(i int) {
		_, _ = r.AddCardComment(card.ID, "me", fmt.Sprintf("c%d", i), time.Time{})
	})
	cf, _ := r.LoadComments(card.ID)
	if len(cf.Comments) != concurrentWriters {
		t.Errorf("comments = %d, want %d", len(cf.Comments), concurrentWriters)
	}
}

func TestProjectLabelsNeverWipedAfterFailedRead(t *testing.T) {
	r, b, s, p := setupLabelTestRepo(t)
	if _, err := r.AddProjectLabel(b, s, p, "Keep", "#111"); err != nil {
		t.Fatal(err)
	}
	path := r.labelsPath(b, s, p)
	torn := []byte(`{"labels":[{"id":"x","name":"Keep"`)
	if err := os.WriteFile(path, torn, 0o644); err != nil {
		t.Fatal(err)
	}

	if _, err := r.GetProjectLabels(b, s, p); err == nil {
		t.Error("GetProjectLabels on a torn file must return an error")
	}
	if _, err := r.AddProjectLabel(b, s, p, "New", "#222"); err == nil {
		t.Error("AddProjectLabel must refuse to write after a failed read")
	}
	if got, _ := os.ReadFile(path); string(got) != string(torn) {
		t.Errorf("labels file was rewritten: %s", got)
	}
}

func TestTagColorsNeverWipedAfterFailedRead(t *testing.T) {
	r := setupTestRepo(t)
	torn := []byte(`{"bug":"#eb5a46",`)
	if err := os.WriteFile(r.tagsPath(), torn, 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := r.GetTagColors(); err == nil {
		t.Error("GetTagColors on a torn file must return an error")
	}
	for name, write := range map[string]func() error{
		"SetTagColor":    func() error { _, err := r.SetTagColor("x", "#000"); return err },
		"AssignTagColor": func() error { _, err := r.AssignTagColor("x"); return err },
		"RemoveTagColor": func() error { _, err := r.RemoveTagColor("bug"); return err },
	} {
		if err := write(); err == nil {
			t.Errorf("%s must refuse to write after a failed read", name)
		}
	}
	if got, _ := os.ReadFile(r.tagsPath()); string(got) != string(torn) {
		t.Errorf("tags file was rewritten: %s", got)
	}
}

func TestTagColorsMissingFileIsEmpty(t *testing.T) {
	r := setupTestRepo(t)
	tc, err := r.GetTagColors()
	if err != nil || len(tc) != 0 {
		t.Fatalf("GetTagColors on fresh repo = (%v, %v), want empty, nil", tc, err)
	}
}

func TestUserTypeStoreLoadErrorIsNotEmpty(t *testing.T) {
	r := setupTestRepo(t)
	if _, err := r.LoadUserTypeStore(); err != nil {
		t.Fatalf("missing store: %v, want nil", err)
	}
	torn := []byte(`{"seeded":true,"types":[`)
	if err := os.WriteFile(r.cardTypesPath(), torn, 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := r.LoadUserTypeStore(); err == nil {
		t.Error("LoadUserTypeStore on a torn file must return an error")
	}
	called := false
	if _, err := r.UpdateUserTypeStore(func(*config.UserTypeStore) error { called = true; return nil }); err == nil || called {
		t.Errorf("UpdateUserTypeStore after failed load: err=%v called=%v, want error and fn not run", err, called)
	}
	if got, _ := os.ReadFile(r.cardTypesPath()); string(got) != string(torn) {
		t.Errorf("card types file was rewritten: %s", got)
	}
}

func TestPinCardAppendsAfterCategoryMax(t *testing.T) {
	r := setupTestRepo(t)
	r.CreateBrand("Brand")
	r.CreateStream("brand", "Stream")
	r.CreateProject("brand", "stream", "Project")
	cat, _ := r.CreateCategory("brand", "stream", "project", "Backlog", 0)
	other, _ := r.CreateCategory("brand", "stream", "project", "Other", 1)

	a, _ := r.CreateCard("task", "A")
	b, _ := r.CreateCard("task", "B")
	if err := r.PinCardAt(a.ID, cat.ID, 5); err != nil {
		t.Fatal(err)
	}
	// B's own pin count (1, after this) must not decide its position.
	if err := r.PinCard(b.ID, other.ID); err != nil {
		t.Fatal(err)
	}
	if err := r.PinCard(b.ID, cat.ID); err != nil {
		t.Fatal(err)
	}
	pins, _ := r.ListCardsInCategory(cat.ID)
	if len(pins) != 2 || pins[1].CardID != b.ID || pins[1].Position != 6 {
		t.Errorf("pins = %+v, want B at position 6 after A", pins)
	}
	if otherPins, _ := r.ListCardsInCategory(other.ID); len(otherPins) != 1 || otherPins[0].Position != 0 {
		t.Errorf("empty category: pins = %+v, want position 0", otherPins)
	}
}

func TestRenameCategoryKeepsOneFile(t *testing.T) {
	r := setupTestRepo(t)
	r.CreateBrand("Brand")
	r.CreateStream("brand", "Stream")
	r.CreateProject("brand", "stream", "Project")
	cat, _ := r.CreateCategory("brand", "stream", "project", "Backlog", 0)

	renamed, err := r.RenameCategory("brand", "stream", "project", "backlog", "Later")
	if err != nil {
		t.Fatal(err)
	}
	if renamed.ID != cat.ID || renamed.Slug != "later" {
		t.Errorf("renamed = %+v", renamed)
	}
	if fileExists(r.categoryFilePath("brand", "stream", "project", "backlog")) {
		t.Error("old category file should be gone")
	}
	cats, _ := r.ListCategories("brand", "stream", "project")
	if len(cats) != 1 || cats[0].ID != cat.ID {
		t.Errorf("categories = %+v, want exactly the renamed one", cats)
	}
}

func TestCopyProjectSkipsWorkspaceAndKeepsMetadata(t *testing.T) {
	r := setupCopyTestRepo(t)
	if _, err := r.UpdateProjectDescription("brand-a", "stream-1", "project-x", "about"); err != nil {
		t.Fatal(err)
	}
	if _, err := r.UpdateProjectIcon("brand-a", "stream-1", "project-x", "star"); err != nil {
		t.Fatal(err)
	}
	if err := r.SaveWorkspace("brand-a", "stream-1", "project-x", &model.Workspace{ID: "ws-1", Adapter: "plain-folder"}); err != nil {
		t.Fatal(err)
	}

	copied, err := r.CopyProject("brand-a", "stream-1", "project-x", "brand-a", "stream-1", -1)
	if err != nil {
		t.Fatal(err)
	}
	if copied.Description != "about" || copied.Icon != "star" {
		t.Errorf("copy lost metadata: %+v", copied)
	}
	if r.HasWorkspace("brand-a", "stream-1", copied.Slug) {
		t.Error("copy must not clone the source workspace identity")
	}
	if !r.HasWorkspace("brand-a", "stream-1", "project-x") {
		t.Error("source workspace must be untouched")
	}
}

func TestCopyBrandSkipsWorkspacesAndKeepsMetadata(t *testing.T) {
	r := setupCopyTestRepo(t)
	if _, err := r.UpdateBrand("brand-a", func(b *model.Brand) {
		b.Description, b.Icon, b.Logo, b.Website, b.SystemPrompt = "d", "i", "l", "w", "sp"
	}); err != nil {
		t.Fatal(err)
	}
	// A stream slugged "workspace" is not a workspace folder and must copy.
	if _, err := r.CreateStream("brand-a", "Workspace"); err != nil {
		t.Fatal(err)
	}
	if err := r.SaveWorkspace("brand-a", "stream-1", "project-x", &model.Workspace{ID: "ws-1", Adapter: "plain-folder"}); err != nil {
		t.Fatal(err)
	}

	copied, err := r.CopyBrand("brand-a")
	if err != nil {
		t.Fatal(err)
	}
	if copied.Description != "d" || copied.Icon != "i" || copied.Logo != "l" || copied.Website != "w" || copied.SystemPrompt != "sp" {
		t.Errorf("copy lost metadata: %+v", copied)
	}
	if r.HasWorkspace(copied.Slug, "stream-1", "project-x") {
		t.Error("brand copy must not clone a project's workspace")
	}
	if _, err := r.GetStream(copied.Slug, "workspace"); err != nil {
		t.Errorf("stream slugged workspace was not copied: %v", err)
	}

	// Every category ID in the copy is new.
	srcCats, _ := r.ListCategories("brand-a", "stream-1", "project-x")
	dstCats, _ := r.ListCategories(copied.Slug, "stream-1", "project-x")
	if len(dstCats) != len(srcCats) {
		t.Fatalf("categories: %d copied, want %d", len(dstCats), len(srcCats))
	}
	seen := map[string]bool{}
	for _, c := range srcCats {
		seen[c.ID] = true
	}
	for _, c := range dstCats {
		if seen[c.ID] {
			t.Errorf("category %q kept its source ID", c.Slug)
		}
	}
}

func TestManifestUpdateRereadsDisk(t *testing.T) {
	r := setupTestRepo(t)
	// A second handle on the same repo renames it; the first handle's
	// stale in-memory manifest must not revert that name.
	other, err := Open(r.Root)
	if err != nil {
		t.Fatal(err)
	}
	if err := other.UpdateManifestName("Renamed"); err != nil {
		t.Fatal(err)
	}
	if err := r.UpdateManifestDescription("desc"); err != nil {
		t.Fatal(err)
	}
	reopened, _ := Open(r.Root)
	if reopened.Manifest.Name != "Renamed" || reopened.Manifest.Description != "desc" {
		t.Errorf("manifest = %+v, want both edits kept", reopened.Manifest)
	}
	if r.Manifest.Name != "Renamed" {
		t.Errorf("in-memory manifest not refreshed: %q", r.Manifest.Name)
	}
}

func TestLockPathKeyIsCanonical(t *testing.T) {
	dir := t.TempDir()
	a := filepath.Join(dir, "x.json")
	b := filepath.Join(dir, ".", "x.json")
	unlock := lockPath(a)
	acquired := make(chan struct{})
	go func() {
		u := lockPath(b)
		close(acquired)
		u()
	}()
	select {
	case <-acquired:
		t.Fatal("equivalent paths must share one lock")
	default:
	}
	unlock()
	<-acquired
}
