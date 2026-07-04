package tools

// Workspace tool execution (project scope). All read-only: these run
// directly even in suggest mode — like web_fetch/web_search, there is
// nothing to stage. Availability is gated where the tool definitions are
// assembled (core/runtime/chat) per the context-level mapping; handlers
// here just do the work for whichever tools were offered.

import (
	"context"
	"fmt"
	"strings"
	"time"

	"bruv/internal/llm"
	"bruv/internal/model"
)

// workspaceToolNames routes ExecuteProject/StageProject dispatch here.
var workspaceToolNames = map[string]bool{
	"workspace_index":     true,
	"workspace_status":    true,
	"workspace_read_file": true,
	"template_list":       true,
	"template_params":     true,
}

// Tool-facing caps — deliberately tighter than the service's editor caps:
// tool output lands in the LLM context window.
const (
	workspaceToolReadBytes   = 100 << 10 // workspace_read_file content cap
	workspaceToolTreeEntries = 300       // workspace_index tree listing cap
)

// IsWorkspaceTool reports whether name is one of the workspace tools.
func IsWorkspaceTool(name string) bool { return workspaceToolNames[name] }

// execWorkspaceTool runs one workspace tool inside the project scope.
func (d *Dispatcher) execWorkspaceTool(tc llm.ToolCall, scope ProjectChatScope) (string, *model.ToolAction) {
	b, s, p := scope.BrandSlug, scope.StreamSlug, scope.ProjectSlug
	svc := d.deps.Workspace()
	if svc == nil {
		return "error: workspace service unavailable", nil
	}

	result := ""
	switch tc.Name {
	case "workspace_index":
		idx, err := svc.GetIndex(b, s, p)
		if err != nil {
			// No cached index yet (or stale attach) — build one now.
			ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			idx, err = svc.RefreshIndex(ctx, b, s, p)
			cancel()
		}
		if err != nil {
			return workspaceToolError(err), nil
		}
		var sb strings.Builder
		sb.WriteString(idx.Summary + "\n")
		for k, v := range idx.Details {
			sb.WriteString(fmt.Sprintf("%s: %s\n", k, v))
		}
		sb.WriteString("\nFiles:\n")
		shown := 0
		for _, e := range idx.Tree {
			if e.IsDir {
				continue
			}
			if shown >= workspaceToolTreeEntries {
				sb.WriteString(fmt.Sprintf("… and %d more files (tree truncated)\n", len(idx.Tree)-shown))
				break
			}
			sb.WriteString(e.Path + "\n")
			shown++
		}
		result = sb.String()

	case "workspace_status":
		ws, err := svc.Get(b, s, p)
		if err != nil {
			return workspaceToolError(err), nil
		}
		var sb strings.Builder
		sb.WriteString(fmt.Sprintf("Origin: %s", ws.Origin.Kind))
		if ws.Origin.URL != "" {
			sb.WriteString(" (" + ws.Origin.URL + ")")
		}
		sb.WriteString(fmt.Sprintf("\nAdapter: %s\n", ws.Adapter))
		if ws.Origin.Kind == model.OriginLocal {
			sb.WriteString("Files are on this machine (Tier 1) — workspace_read_file works.\n")
		} else {
			sb.WriteString("Files are not materialized on this machine (Tier 0).\n")
		}
		if ws.Claim != nil {
			sb.WriteString(fmt.Sprintf("Checked out on %s (state: %s).\n", ws.Claim.Device, ws.Claim.State))
		}
		if idx, err := svc.GetIndex(b, s, p); err == nil {
			sb.WriteString("Index last refreshed: " + idx.GeneratedAt.Format(time.RFC3339) + "\n")
		}
		result = sb.String()

	case "workspace_read_file":
		path, _ := tc.Arguments["path"].(string)
		if path == "" {
			return "error: path is required", nil
		}
		ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		content, err := svc.ReadFile(ctx, b, s, p, path)
		cancel()
		if err != nil {
			return workspaceToolError(err), nil
		}
		if len(content) > workspaceToolReadBytes {
			content = content[:workspaceToolReadBytes] + fmt.Sprintf("\n\n[truncated — file is %d bytes, showing first %d]", len(content), workspaceToolReadBytes)
		}
		result = content

	case "template_list":
		entries, err := svc.ListTemplates()
		if err != nil {
			return "error: " + err.Error(), nil
		}
		if len(entries) == 0 {
			result = "No folder templates in this vault."
		} else {
			var sb strings.Builder
			for _, e := range entries {
				sb.WriteString(fmt.Sprintf("- %s (id: %s, scope: %s)", e.Name, e.ID, e.Scope))
				if e.Description != "" {
					sb.WriteString(" — " + e.Description)
				}
				sb.WriteString("\n")
			}
			result = sb.String()
		}

	case "template_params":
		ref, _ := tc.Arguments["template_id"].(string)
		if ref == "" {
			return "error: template_id is required", nil
		}
		params, err := svc.GetTemplateParams(ref)
		if err != nil {
			return "error: " + err.Error(), nil
		}
		if len(params) == 0 {
			result = "This template has no parameters."
		} else {
			var sb strings.Builder
			for _, p := range params {
				if p.Name == "" {
					continue // anonymous rename rules aren't user-facing
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
			result = sb.String()
		}

	default:
		return "error: unknown workspace tool " + tc.Name, nil
	}

	action := &model.ToolAction{Tool: tc.Name, Input: tc.Arguments, Result: summarizeToolResult(tc.Name, result)}
	return result, action
}

// workspaceToolError keeps "no workspace" friendly instead of error-shaped —
// the LLM should tell the user, not retry.
func workspaceToolError(err error) string {
	msg := err.Error()
	if strings.Contains(msg, "has no workspace") {
		return "No workspace is attached to this project. The user can attach one from the Workspace panel."
	}
	return "error: " + msg
}

// summarizeToolResult keeps run-history rows short for bulky read results.
func summarizeToolResult(tool, result string) string {
	if len(result) <= 200 {
		return result
	}
	return fmt.Sprintf("%s returned %d chars", tool, len(result))
}
