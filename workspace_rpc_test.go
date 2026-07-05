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
	call("WriteWorkspaceFile", brand.Slug, stream.Slug, project.Slug, "notes.md", "# edited")
	_ = json.Unmarshal(call("ReadWorkspaceFile", brand.Slug, stream.Slug, project.Slug, "notes.md"), &content)
	if content != "# edited" {
		t.Fatalf("after write: %q", content)
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

	// --- Card Folders over the same dispatcher (positional-param guard for
	// the 7-arg GenerateCardFolder signature). ---
	call("AttachWorkspace", brand.Slug, stream.Slug, project.Slug, wsDir)
	tplDir := filepath.Join(wsDir, "_tpl-{bruvCard}")
	if err := os.MkdirAll(filepath.Join(tplDir, ".ft"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(tplDir, ".ft", "template.json"),
		[]byte(`{"name":"Ep","parameters":[{"name":"strip","match":"^_tpl-","replaceInFileNames":true}]}`), 0o644); err != nil {
		t.Fatal(err)
	}
	epCard, err := rt.CreateCard("", "Pilot")
	if err != nil {
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
	var boundCard struct {
		Folder *struct {
			Path string `json:"path"`
		} `json:"folder"`
	}
	_ = json.Unmarshal(call("GenerateCardFolder", brand.Slug, stream.Slug, project.Slug,
		epCard.ID, tpls[0].ID, "", map[string]string{}), &boundCard)
	if boundCard.Folder == nil || boundCard.Folder.Path != "Pilot" {
		t.Fatalf("GenerateCardFolder over RPC: %+v", boundCard)
	}
	// Fresh struct: omitempty means a cleared folder is ABSENT from the
	// JSON, and Unmarshal leaves absent fields untouched.
	var clearedCard struct {
		Folder *struct{} `json:"folder"`
	}
	_ = json.Unmarshal(call("ClearCardFolder", epCard.ID), &clearedCard)
	if clearedCard.Folder != nil {
		t.Fatal("ClearCardFolder over RPC must unbind")
	}
}
