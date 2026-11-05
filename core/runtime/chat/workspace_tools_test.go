package chat

// The workspace tools are the registry's now; each chat still offers them
// as it did: project chat by workspace and context level, card chat only
// its own files.

import (
	"slices"
	"testing"

	"bruv/internal/llm"
	"bruv/internal/model"
)

func toolNames(defs []llm.ToolDef) []string {
	out := make([]string, len(defs))
	for i, d := range defs {
		out[i] = d.Name
	}
	return out
}

func TestProjectWorkspaceTools(t *testing.T) {
	var all []llm.ToolDef
	for _, n := range append([]string{"get_card", llm.CardFileTool}, llm.ProjectWorkspaceTools...) {
		all = append(all, llm.ToolDef{Name: n})
	}
	for _, tc := range []struct {
		name         string
		hasWorkspace bool
		level        model.ProjectChatContextLevel
		want         []string
	}{
		{"no workspace", false, model.ProjectChatContextAll, []string{"get_card"}},
		{"none", true, model.ProjectChatContextNone, []string{"get_card"}},
		{"metadata", true, model.ProjectChatContextMetadata, []string{"get_card", "workspace_index", "workspace_status", "template_list", "template_params"}},
		{"all", true, model.ProjectChatContextAll, append([]string{"get_card"}, llm.ProjectWorkspaceTools...)},
	} {
		if got := toolNames(projectWorkspaceTools(all, tc.hasWorkspace, tc.level)); !slices.Equal(got, tc.want) {
			t.Errorf("%s: %v, want %v", tc.name, got, tc.want)
		}
	}
}

func TestCardFileToolNamesTheCardsFiles(t *testing.T) {
	native := []llm.ToolDef{{Name: "get_card"}, {Name: llm.CardFileTool, Description: "Read a file."}}
	got := cardFileTool(native, []string{"a.md", "Docs/"})
	if len(got) != 1 || got[0].Description != "Read a file. This card names: `a.md`, `Docs/`; folders end in /." {
		t.Errorf("cardFileTool = %+v", got)
	}
	if native[1].Description != "Read a file." {
		t.Error("the native definition was changed")
	}
}
