package boardtools

// Workspace tools: read-only access to a project's workspace (its files,
// index and status) and to the vault's folder templates, plus the files a
// card names in its Workspace Files block. AI write access to workspace
// files is out of scope by spec, and generating from a template is always
// the user's, in the UI. Which of these a chat is offered is the surface's
// call (project chat's context level, card chat's files; see
// llm.ProjectWorkspaceTools).

import (
	"context"
	"fmt"
	"strings"
	"time"

	cardtools "bruv/core/runtime/tools"
	"bruv/internal/model"
)

// Tool-facing caps, tighter than the editor's: results land in the
// model's context.
const (
	workspaceReadBytes   = 100 << 10 // a file's content
	workspaceTreeEntries = 300       // files listed by workspace_index
)

// workspaceError keeps "no workspace" a plain answer the model passes on
// rather than an error it retries.
func workspaceError(err error) (string, bool) {
	if strings.Contains(err.Error(), "has no workspace") {
		return "No workspace is attached to this project. The user can attach one from the Workspace panel.", false
	}
	return errResult("%v", err)
}

func truncateFile(content string) string {
	if len(content) > workspaceReadBytes {
		return content[:workspaceReadBytes] + fmt.Sprintf("\n\n[truncated — file is %d bytes, showing first %d]", len(content), workspaceReadBytes)
	}
	return content
}

func hWorkspaceIndex(rt Board, a map[string]any) (string, bool) {
	p, err := existingProject(rt, a)
	if err != nil {
		return errResult("%v", err)
	}
	svc := rt.WorkspaceService()
	idx, err := svc.GetIndex(p.brand, p.stream, p.project)
	if err != nil {
		// No index yet (or a stale attach): build one now.
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		idx, err = svc.RefreshIndex(ctx, p.brand, p.stream, p.project)
		cancel()
	}
	if err != nil {
		return workspaceError(err)
	}
	var sb strings.Builder
	sb.WriteString(idx.Summary + "\n")
	for k, v := range idx.Details {
		fmt.Fprintf(&sb, "%s: %s\n", k, v)
	}
	sb.WriteString("\nFiles:\n")
	shown := 0
	for _, e := range idx.Tree {
		if e.IsDir {
			continue
		}
		if shown >= workspaceTreeEntries {
			fmt.Fprintf(&sb, "… and %d more files (tree truncated)\n", len(idx.Tree)-shown)
			break
		}
		sb.WriteString(e.Path + "\n")
		shown++
	}
	return sb.String(), false
}

func hWorkspaceStatus(rt Board, a map[string]any) (string, bool) {
	p, err := existingProject(rt, a)
	if err != nil {
		return errResult("%v", err)
	}
	svc := rt.WorkspaceService()
	ws, err := svc.Get(p.brand, p.stream, p.project)
	if err != nil {
		return workspaceError(err)
	}
	var sb strings.Builder
	fmt.Fprintf(&sb, "Origin: %s", ws.Origin.Kind)
	if ws.Origin.URL != "" {
		sb.WriteString(" (" + ws.Origin.URL + ")")
	}
	fmt.Fprintf(&sb, "\nAdapter: %s\n", ws.Adapter)
	if ws.Origin.Kind == model.OriginLocal {
		sb.WriteString("Files are on this machine — workspace_read_file works.\n")
	} else {
		sb.WriteString("Files are not on this machine.\n")
	}
	if ws.Claim != nil {
		fmt.Fprintf(&sb, "Checked out on %s (state: %s).\n", ws.Claim.Device, ws.Claim.State)
	}
	if idx, err := svc.GetIndex(p.brand, p.stream, p.project); err == nil {
		sb.WriteString("Index last refreshed: " + idx.GeneratedAt.Format(time.RFC3339) + "\n")
	}
	return sb.String(), false
}

func hWorkspaceReadFile(rt Board, a map[string]any) (string, bool) {
	path := argStr(a, "path")
	if path == "" {
		return errResult("path is required")
	}
	p, err := existingProject(rt, a)
	if err != nil {
		return errResult("%v", err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	content, err := rt.WorkspaceService().ReadFile(ctx, p.brand, p.stream, p.project, path)
	cancel()
	if err != nil {
		return workspaceError(err)
	}
	return truncateFile(content), false
}

func hTemplateList(rt Board, _ map[string]any) (string, bool) {
	entries, err := rt.WorkspaceService().ListTemplates()
	if err != nil {
		return errResult("%v", err)
	}
	if len(entries) == 0 {
		return "No folder templates in this vault.", false
	}
	var sb strings.Builder
	for _, e := range entries {
		fmt.Fprintf(&sb, "- %s (id: %s, scope: %s)", e.Name, e.ID, e.Scope)
		if e.Description != "" {
			sb.WriteString(" — " + e.Description)
		}
		sb.WriteString("\n")
	}
	return sb.String(), false
}

func hTemplateParams(rt Board, a map[string]any) (string, bool) {
	ref := argStr(a, "template_id")
	if ref == "" {
		return errResult("template_id is required")
	}
	params, err := rt.WorkspaceService().GetTemplateParams(ref)
	if err != nil {
		return errResult("%v", err)
	}
	var sb strings.Builder
	for _, p := range params {
		if p.Name == "" {
			continue // anonymous rename rules aren't the user's to fill in
		}
		line := "- " + p.Name
		if p.Prompt != nil && *p.Prompt != "" {
			line += fmt.Sprintf(" (prompt: %q)", *p.Prompt)
		} else {
			line += " (internal)"
		}
		if p.DefaultValue != nil && *p.DefaultValue != "" {
			line += fmt.Sprintf(" default: %q", *p.DefaultValue)
		}
		sb.WriteString(line + "\n")
	}
	if sb.Len() == 0 {
		return "This template has no parameters.", false
	}
	return sb.String(), false
}

// hReadCardFile reads one of the files a card names (or a file inside a
// folder it names); nothing else in the workspace. The block is the scope.
func hReadCardFile(rt Board, a map[string]any) (string, bool) {
	cardID, path := argStr(a, "card_id"), argStr(a, "path")
	if cardID == "" || path == "" {
		return errResult("card_id and path are required")
	}
	card, err := rt.GetCard(cardID)
	if err != nil {
		return errResult("%v", err)
	}
	entry, ok := cardtools.AllowedCardFile(cardtools.CardFileEntries(card), path)
	if !ok {
		names := cardtools.CardFilePaths(card)
		if len(names) == 0 {
			return errResult("card %s names no workspace files", cardID)
		}
		return errResult("%s isn't one of the card's workspace files (%s) or inside one of its folders", path, strings.Join(names, ", "))
	}
	refs, err := rt.Repo().ListWorkspaces()
	if err != nil {
		return workspaceError(err)
	}
	for _, ref := range refs {
		if ref.Workspace.ID != entry.WorkspaceID {
			continue
		}
		ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		content, err := rt.WorkspaceService().ReadFile(ctx, ref.BrandSlug, ref.StreamSlug, ref.ProjectSlug, strings.Trim(path, "/"))
		cancel()
		if err != nil {
			return workspaceError(err)
		}
		return truncateFile(content), false
	}
	return errResult("the workspace this file belongs to is no longer attached")
}
