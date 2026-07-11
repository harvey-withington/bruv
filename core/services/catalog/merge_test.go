package catalog

import (
	"os"
	"path/filepath"
	"testing"

	"bruv/internal/index"
	"bruv/internal/model"
	"bruv/internal/repo"
	"bruv/internal/schema"
)

// mergeDeps is a minimal Deps over a real temp repo — UpdateCardBlocks
// writes straight through so the merge result can be read back.
type mergeDeps struct{ r *repo.Repository }

func (d *mergeDeps) Repo() *repo.Repository      { return d.r }
func (d *mergeDeps) Registry() *schema.Registry  { return nil }
func (d *mergeDeps) Index() *index.Index         { return nil }
func (d *mergeDeps) Publish(string, any)         {}
func (d *mergeDeps) UpdateCardBlocks(id string, blocks []model.Block) (*model.Card, error) {
	return d.r.UpdateCardBlocks(id, blocks)
}

func newMergeService(t *testing.T) (*Service, *repo.Repository) {
	t.Helper()
	dir := filepath.Join(t.TempDir(), "repo")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	r, err := repo.Init(dir, "Merge Test")
	if err != nil {
		t.Fatal(err)
	}
	return New(&mergeDeps{r: r}), r
}

// A hand-added block has an empty key; a template field carrying the
// same label (any casing) and block type must MERGE into it — adopting
// the template's key — not append a duplicate. Regression for the
// "Create Type from Card then apply the type elsewhere duplicates the
// same-named block" bug (Harvey, 2026-07-11).
func TestMergeTemplateBlocksClaimsFreeformByLabel(t *testing.T) {
	s, r := newMergeService(t)
	card, err := r.CreateCard("", "Card")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := r.UpdateCardBlocks(card.ID, []model.Block{
		{ID: "b1", Type: model.BlockText, Label: "Notes", Key: "", Value: "already written"},
	}); err != nil {
		t.Fatal(err)
	}

	s.mergeTemplateBlocks(card.ID, []model.Block{
		{ID: "t1", Type: model.BlockText, Label: "notes", Key: "notes", Value: ""},
	})

	got, err := r.GetCard(card.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Blocks) != 1 {
		t.Fatalf("blocks = %d, want 1 (merged, not duplicated): %+v", len(got.Blocks), got.Blocks)
	}
	if got.Blocks[0].Key != "notes" {
		t.Errorf("key = %q, want adopted template key %q", got.Blocks[0].Key, "notes")
	}
	if got.Blocks[0].Value != "already written" {
		t.Errorf("value = %v, want the card's existing value preserved", got.Blocks[0].Value)
	}
}

// Value-fill semantics for MATCHED blocks (ruled by Harvey 2026-07-11):
// an existing block with content always wins; an EMPTY existing block
// takes a pre-populated template value (e.g. a kept-value checklist from
// "Create Type from Card"). Note "empty" includes ''/0/false/[] — an
// untouched block is indistinguishable from a deliberately-zeroed one,
// and applying a type is a request for its defaults (deliberately the
// opposite call from the Markdown-export zero-is-not-nothing rule).
func TestMergeTemplateBlocksFillsOnlyEmptyValues(t *testing.T) {
	s, r := newMergeService(t)
	card, err := r.CreateCard("", "Card")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := r.UpdateCardBlocks(card.ID, []model.Block{
		{ID: "b1", Type: model.BlockText, Label: "Filled", Key: "filled", Value: "user content"},
		{ID: "b2", Type: model.BlockText, Label: "Blank", Key: "blank", Value: ""},
	}); err != nil {
		t.Fatal(err)
	}

	s.mergeTemplateBlocks(card.ID, []model.Block{
		{ID: "t1", Type: model.BlockText, Label: "Filled", Key: "filled", Value: "template default"},
		{ID: "t2", Type: model.BlockText, Label: "Blank", Key: "blank", Value: "template default"},
	})

	got, err := r.GetCard(card.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Blocks) != 2 {
		t.Fatalf("blocks = %d, want 2: %+v", len(got.Blocks), got.Blocks)
	}
	if got.Blocks[0].Value != "user content" {
		t.Errorf("populated block was overwritten: %v", got.Blocks[0].Value)
	}
	if got.Blocks[1].Value != "template default" {
		t.Errorf("empty block was not filled from the template: %v", got.Blocks[1].Value)
	}
}

// Boundary cases. A block's key is only meaningful relative to the
// incoming template: orphan keys (another schema's residue, e.g. a
// relabelled "content" field — the real-world case that surfaced this)
// ARE claimable by label; keys the template itself declares are claimed
// by their own field and can never be stolen by a label coincidence;
// label claims never cross block types.
func TestMergeTemplateBlocksFallbackLimits(t *testing.T) {
	s, r := newMergeService(t)
	card, err := r.CreateCard("", "Card")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := r.UpdateCardBlocks(card.ID, []model.Block{
		// Same label as the template field but a DIFFERENT type — never claimed.
		{ID: "b1", Type: model.BlockChecklist, Label: "Notes", Key: "", Value: []model.ChecklistItem{}},
		// Orphan key (not declared by this template) — claimable by label,
		// value preserved, key adopted. (Harvey's "content"/"Related links" case.)
		{ID: "b2", Type: model.BlockText, Label: "Status", Key: "other_status", Value: "keyed value"},
		// Key the template DOES declare — claimed by its own field below,
		// label collision with t3 must not steal it.
		{ID: "b3", Type: model.BlockText, Label: "Summary", Key: "summary", Value: "mine"},
	}); err != nil {
		t.Fatal(err)
	}

	s.mergeTemplateBlocks(card.ID, []model.Block{
		{ID: "t1", Type: model.BlockText, Label: "Notes", Key: "notes", Value: ""},
		{ID: "t2", Type: model.BlockText, Label: "Status", Key: "status", Value: ""},
		{ID: "t3", Type: model.BlockText, Label: "summary", Key: "summary_2", Value: ""},
		{ID: "t4", Type: model.BlockText, Label: "Summary", Key: "summary", Value: ""},
	})

	got, err := r.GetCard(card.ID)
	if err != nil {
		t.Fatal(err)
	}
	// b1 stays keyless (type mismatch) + t1 appends; b2 claimed by t2;
	// b3 key-matched by t4; t3 appends (b3 not stealable by label).
	if len(got.Blocks) != 5 {
		t.Fatalf("blocks = %d, want 5: %+v", len(got.Blocks), got.Blocks)
	}
	if got.Blocks[0].Key != "" {
		t.Errorf("checklist block was claimed across types (key %q)", got.Blocks[0].Key)
	}
	if got.Blocks[1].Key != "status" || got.Blocks[1].Value != "keyed value" {
		t.Errorf("orphan-keyed block not claimed correctly: key=%q value=%v", got.Blocks[1].Key, got.Blocks[1].Value)
	}
	if got.Blocks[2].Key != "summary" || got.Blocks[2].Value != "mine" {
		t.Errorf("template-owned key was disturbed: key=%q value=%v", got.Blocks[2].Key, got.Blocks[2].Value)
	}
}
