package main

// Reproduces the exact RPC path the desktop/cloud frontend uses for
// "Create Card Type from Card": JSON positional params → reflection
// dispatch → Runtime.CreateCardTypeFromCard. Guards against a dispatch /
// param-marshaling mismatch the direct unit test in core/supervisor
// can't catch.

import (
	"context"
	"encoding/json"
	"testing"

	"bruv/core/supervisor"
	"bruv/internal/config"
	"bruv/internal/model"
	"bruv/internal/repo"
	transporthttp "bruv/transport/http"
)

func TestCreateCardTypeFromCardOverRPC(t *testing.T) {
	cfgDir := t.TempDir()
	r, err := repo.InitAt(t.TempDir(), "Test Repo")
	if err != nil {
		t.Fatalf("InitAt: %v", err)
	}
	sup, err := supervisor.New([]config.RepoEntry{{ID: "r1", Name: "Test Repo", Path: r.Root}}, cfgDir)
	if err != nil {
		t.Fatalf("supervisor.New: %v", err)
	}
	t.Cleanup(sup.Close)
	rt, err := sup.Load("r1")
	if err != nil {
		t.Fatalf("Load: %v", err)
	}

	card, err := rt.CreateCard("", "Episode 12")
	if err != nil {
		t.Fatalf("CreateCard: %v", err)
	}
	if _, err := rt.UpdateCardBlocks(card.ID, []model.Block{
		{ID: "b1", Type: model.BlockText, Label: "Notes", Value: "draft"},
		{ID: "b2", Type: model.BlockChecklist, Label: "Shots", Value: []any{"intro"}},
	}); err != nil {
		t.Fatalf("UpdateCardBlocks: %v", err)
	}

	disp := transporthttp.NewDispatcher(rt, transporthttp.DefaultDeniedMethods())

	// Confirm the method is actually exposed (a stale binary / denial
	// would surface here as ErrMethodNotFound).
	raw := func(v any) json.RawMessage { b, _ := json.Marshal(v); return b }
	params := []json.RawMessage{
		raw(card.ID), raw("Episode"), raw("calendar"), raw("#ec4899"), raw([]string{"b1", "b2"}),
	}

	result, rpcErr := disp.Dispatch(context.Background(), "CreateCardTypeFromCard", params)
	if rpcErr != nil {
		t.Fatalf("RPC dispatch error: code=%d msg=%q", rpcErr.Code, rpcErr.Message)
	}
	// Result should be the new CardTypeInfo.
	b, _ := json.Marshal(result)
	var info struct {
		ID    string `json:"id"`
		Label string `json:"label"`
	}
	_ = json.Unmarshal(b, &info)
	if info.ID == "" || info.Label != "Episode" {
		t.Fatalf("unexpected result: %s", b)
	}
}
