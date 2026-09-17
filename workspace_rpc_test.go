package main

// Reproduces the exact RPC path the frontends use for the Workspace M1
// surface: JSON positional params → reflection dispatch → Runtime methods.
// Guards against dispatch / param-marshaling drift the service unit tests
// can't catch (the "method X expects N params, got M" class of failure).

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"bruv/core/supervisor"
	"bruv/internal/config"
	"bruv/internal/repo"
	transporthttp "bruv/transport/http"
)

func TestWorkspaceOverRPC(t *testing.T) {
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

	brand, err := rt.CreateBrand("Acme")
	if err != nil {
		t.Fatal(err)
	}
	stream, err := rt.CreateStream(brand.Slug, "Films")
	if err != nil {
		t.Fatal(err)
	}
	project, err := rt.CreateProject(brand.Slug, stream.Slug, "Big Movie")
	if err != nil {
		t.Fatal(err)
	}

	wsDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(wsDir, "notes.md"), []byte("# hello"), 0o644); err != nil {
		t.Fatal(err)
	}

	disp := transporthttp.NewDispatcher(rt, transporthttp.DefaultDeniedMethods())
	raw := func(v any) json.RawMessage { b, _ := json.Marshal(v); return b }
	call := func(method string, params ...any) json.RawMessage {
		t.Helper()
		msgs := make([]json.RawMessage, len(params))
		for i, p := range params {
			msgs[i] = raw(p)
		}
		result, rpcErr := disp.Dispatch(context.Background(), method, msgs)
		if rpcErr != nil {
			t.Fatalf("%s: code=%d msg=%q", method, rpcErr.Code, rpcErr.Message)
		}
		b, _ := json.Marshal(result)
		return b
	}

	// Fresh project: attached=false, no error.
	var state struct {
		Attached bool `json:"attached"`
	}
	_ = json.Unmarshal(call("GetWorkspaceState", brand.Slug, stream.Slug, project.Slug), &state)
	if state.Attached {
		t.Fatal("fresh project must report attached=false")
	}

	// Attach → read → write → read.
	var ws struct {
		ID      string `json:"id"`
		Adapter string `json:"adapter"`
	}
	_ = json.Unmarshal(call("AttachWorkspace", brand.Slug, stream.Slug, project.Slug, wsDir), &ws)
	if ws.ID == "" || ws.Adapter != "plain-folder" {
		t.Fatalf("AttachWorkspace result: %+v", ws)
	}

	var content string
	_ = json.Unmarshal(call("ReadWorkspaceFile", brand.Slug, stream.Slug, project.Slug, "notes.md"), &content)
	if content != "# hello" {
		t.Fatalf("ReadWorkspaceFile = %q", content)
	}
	// The editor's stamped open → guarded save → re-open round trip (the
	// 6-arg SaveWorkspaceFile signature is the positional-param guard here).
	var opened struct {
		Content string `json:"content"`
		Stamp   struct {
			Hash string `json:"hash"`
		} `json:"stamp"`
	}
	_ = json.Unmarshal(call("OpenWorkspaceFile", brand.Slug, stream.Slug, project.Slug, "notes.md"), &opened)
	if opened.Content != "# hello" || opened.Stamp.Hash == "" {
		t.Fatalf("OpenWorkspaceFile = %+v", opened)
	}
	var saved struct {
		Diverged bool `json:"diverged"`
		Stamp    struct {
			Hash string `json:"hash"`
		} `json:"stamp"`
	}
	_ = json.Unmarshal(call("SaveWorkspaceFile", brand.Slug, stream.Slug, project.Slug, "notes.md", "# edited", opened.Stamp.Hash), &saved)
	if saved.Diverged || saved.Stamp.Hash == "" || saved.Stamp.Hash == opened.Stamp.Hash {
		t.Fatalf("SaveWorkspaceFile = %+v", saved)
	}
	_ = json.Unmarshal(call("ReadWorkspaceFile", brand.Slug, stream.Slug, project.Slug, "notes.md"), &content)
	if content != "# edited" {
		t.Fatalf("after write: %q", content)
	}
	var stat struct {
		Hash string `json:"hash"`
	}
	_ = json.Unmarshal(call("StatWorkspaceFile", brand.Slug, stream.Slug, project.Slug, "notes.md"), &stat)
	if stat.Hash != saved.Stamp.Hash {
		t.Fatalf("StatWorkspaceFile hash %q != saved %q", stat.Hash, saved.Stamp.Hash)
	}

	// Escape attempts must fail at the RPC boundary.
	if _, rpcErr := disp.Dispatch(context.Background(), "ReadWorkspaceFile",
		[]json.RawMessage{raw(brand.Slug), raw(stream.Slug), raw(project.Slug), raw("../../manifest.json")}); rpcErr == nil {
		t.Fatal("path escape must be rejected over RPC")
	}

	// State now reports the index.
	var full struct {
		Attached bool `json:"attached"`
		Index    *struct {
			Summary string `json:"summary"`
		} `json:"index"`
	}
	_ = json.Unmarshal(call("GetWorkspaceState", brand.Slug, stream.Slug, project.Slug), &full)
	if !full.Attached || full.Index == nil || full.Index.Summary == "" {
		t.Fatalf("GetWorkspaceState after attach: %+v", full)
	}

	call("DetachWorkspace", brand.Slug, stream.Slug, project.Slug)
	_ = json.Unmarshal(call("GetWorkspaceState", brand.Slug, stream.Slug, project.Slug), &state)
	if state.Attached {
		t.Fatal("detached project must report attached=false")
	}

	// --- Structure actions over the same dispatcher (positional-param guard
	// for the 8-arg GenerateWorkspaceTemplate signature). ---
	_ = json.Unmarshal(call("AttachWorkspace", brand.Slug, stream.Slug, project.Slug, wsDir), &ws)
	tplDir := filepath.Join(wsDir, "_tpl-{bruvCard}")
	if err := os.MkdirAll(filepath.Join(tplDir, ".ft"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(tplDir, ".ft", "template.json"),
		[]byte(`{"name":"Ep","parameters":[{"name":"strip","match":"^_tpl-","replaceInFileNames":true}]}`), 0o644); err != nil {
		t.Fatal(err)
	}
	var tpls []struct {
		ID    string `json:"id"`
		Scope string `json:"scope"`
	}
	_ = json.Unmarshal(call("ListProjectTemplates", brand.Slug, stream.Slug, project.Slug), &tpls)
	if len(tpls) == 0 || tpls[0].Scope != "workspace" {
		t.Fatalf("ListProjectTemplates over RPC: %+v", tpls)
	}
	var generated string
	_ = json.Unmarshal(call("GenerateWorkspaceTemplate", brand.Slug, stream.Slug, project.Slug,
		tpls[0].ID, "", "Pilot", map[string]string{}), &generated)
	if generated != "Pilot" {
		t.Fatalf("GenerateWorkspaceTemplate over RPC = %q, want Pilot", generated)
	}
	var made string
	_ = json.Unmarshal(call("CreateWorkspaceDir", brand.Slug, stream.Slug, project.Slug, "Pilot/Drafts"), &made)
	if made != "Pilot/Drafts" {
		t.Fatalf("CreateWorkspaceDir over RPC = %q", made)
	}
	_ = json.Unmarshal(call("CreateWorkspaceFile", brand.Slug, stream.Slug, project.Slug, "Pilot/Drafts/scene.md"), &made)
	if made != "Pilot/Drafts/scene.md" {
		t.Fatalf("CreateWorkspaceFile over RPC = %q", made)
	}
	if _, err := os.Stat(filepath.Join(wsDir, "Pilot", "Drafts", "scene.md")); err != nil {
		t.Fatalf("created file missing on disk: %v", err)
	}
	var loc struct {
		ProjectSlug string `json:"project_slug"`
	}
	_ = json.Unmarshal(call("ResolveWorkspace", ws.ID), &loc)
	if loc.ProjectSlug != project.Slug {
		t.Fatalf("ResolveWorkspace over RPC: %+v", loc)
	}
}
