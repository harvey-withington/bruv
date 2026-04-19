package repo

import (
	"reflect"
	"testing"

	"bruv/internal/model"
)

// pending is a convenience constructor for brevity in test fixtures.
func pending(id, status string) model.PendingEdit {
	return model.PendingEdit{ID: id, Status: status}
}

func TestSplitPendingEdits(t *testing.T) {
	tests := []struct {
		name      string
		edits     []model.PendingEdit
		acceptIDs []string
		wantAcc   []string
		wantRej   []string
	}{
		{
			name:      "empty input yields empty outputs",
			edits:     nil,
			acceptIDs: nil,
			wantAcc:   nil,
			wantRej:   nil,
		},
		{
			name: "accept none: all pending edits go to reject",
			edits: []model.PendingEdit{
				pending("e1", "pending"),
				pending("e2", "pending"),
				pending("e3", "pending"),
			},
			acceptIDs: nil,
			wantAcc:   nil,
			wantRej:   []string{"e1", "e2", "e3"},
		},
		{
			name: "accept all: nothing to reject",
			edits: []model.PendingEdit{
				pending("e1", "pending"),
				pending("e2", "pending"),
			},
			acceptIDs: []string{"e1", "e2"},
			wantAcc:   []string{"e1", "e2"},
			wantRej:   nil,
		},
		{
			name: "accept subset: remainder rejected",
			edits: []model.PendingEdit{
				pending("e1", "pending"),
				pending("e2", "pending"),
				pending("e3", "pending"),
			},
			acceptIDs: []string{"e2"},
			wantAcc:   []string{"e2"},
			wantRej:   []string{"e1", "e3"},
		},
		{
			name: "preserves input order in both partitions",
			edits: []model.PendingEdit{
				pending("e1", "pending"),
				pending("e2", "pending"),
				pending("e3", "pending"),
				pending("e4", "pending"),
			},
			acceptIDs: []string{"e4", "e1"}, // deliberately out of input order
			wantAcc:   []string{"e1", "e4"}, // input order, not acceptIDs order
			wantRej:   []string{"e2", "e3"},
		},
		{
			name: "skips edits that are already accepted or rejected",
			edits: []model.PendingEdit{
				pending("e1", "accepted"),
				pending("e2", "pending"),
				pending("e3", "rejected"),
				pending("e4", "pending"),
			},
			acceptIDs: []string{"e2", "e3"}, // e3 is in acceptIDs but not pending
			wantAcc:   []string{"e2"},
			wantRej:   []string{"e4"},
		},
		{
			name: "acceptIDs referencing non-existent edits are ignored",
			edits: []model.PendingEdit{
				pending("e1", "pending"),
			},
			acceptIDs: []string{"e999", "e1"},
			wantAcc:   []string{"e1"},
			wantRej:   nil,
		},
		{
			name: "duplicate acceptIDs are deduplicated via the set",
			edits: []model.PendingEdit{
				pending("e1", "pending"),
				pending("e2", "pending"),
			},
			acceptIDs: []string{"e1", "e1", "e1"},
			wantAcc:   []string{"e1"},
			wantRej:   []string{"e2"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotAcc, gotRej := SplitPendingEdits(tt.edits, tt.acceptIDs)
			if !reflect.DeepEqual(gotAcc, tt.wantAcc) {
				t.Errorf("toAccept = %v, want %v", gotAcc, tt.wantAcc)
			}
			if !reflect.DeepEqual(gotRej, tt.wantRej) {
				t.Errorf("toReject = %v, want %v", gotRej, tt.wantRej)
			}
		})
	}
}
