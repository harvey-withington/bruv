package model

import "testing"

func TestCardIDFromFileName(t *testing.T) {
	cases := []struct {
		name   string
		wantID string
		wantOK bool
	}{
		{"0b7c1a52-9f1e-4c55-8d0a-1f2e3d4c5b6a.json", "0b7c1a52-9f1e-4c55-8d0a-1f2e3d4c5b6a", true},
		{"card-001.json", "card-001", true},
		// Sidecars sharing the cards directory are not cards.
		{"card-001.comments.json", "", false},
		{"card-001.agent.json", "", false},
		{"card-001.messages.json", "", false},
		// Atomic-write temps and sync artefacts.
		{"card-001.json.tmp", "", false},
		{"card-001.sync-conflict-20260101-120000-ABCDEFG.json", "", false},
		{".syncthing.card-001.json.tmp", "", false},
		{".json", "", false},
		{"card-001", "", false},
		{"card-001.JSON", "", false},
		{`..\escape.json`, "", false},
		{"a/b.json", "", false},
	}
	for _, c := range cases {
		id, ok := CardIDFromFileName(c.name)
		if id != c.wantID || ok != c.wantOK {
			t.Errorf("CardIDFromFileName(%q) = (%q, %v), want (%q, %v)", c.name, id, ok, c.wantID, c.wantOK)
		}
	}
}
