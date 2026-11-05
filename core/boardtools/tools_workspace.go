package boardtools

import "bruv/internal/mcp"

// workspaceDefs are the definitions of the workspace tools (handlers_workspace.go).
func workspaceDefs(board string) []mcp.Tool {
	project := func(props map[string]any, required ...string) map[string]any {
		props["brand"] = strProp("The project's brand (name or slug).")
		props["stream"] = strProp("The project's stream (name or slug).")
		props["project"] = strProp("The project (name or slug).")
		return obj(props, append([]string{"brand", "stream", "project"}, required...)...)
	}
	return []mcp.Tool{
		{
			Name: "workspace_index",
			Description: "Read a project's workspace in " + board + ": its indexed file tree and a summary (file names and sizes, " +
				"git branch and commits, or vault stats). Call this first when the user asks about the workspace, its files or its structure.",
			InputSchema: project(map[string]any{}),
		},
		{
			Name: "workspace_status",
			Description: "A project's workspace status in " + board + ": where its files come from (local folder, git, rclone), " +
				"whether they're on this machine, and when the index was last refreshed.",
			InputSchema: project(map[string]any{}),
		},
		{
			Name: "workspace_read_file",
			Description: "Read one text file from a project's workspace in " + board + ", by its path as workspace_index lists it. " +
				"Long files are truncated; binaries are refused. Read-only: nothing here writes to the workspace.",
			InputSchema: project(map[string]any{
				"path": strProp(`Path within the workspace, e.g. "chapters/01.md".`),
			}, "path"),
		},
		{
			Name: "template_list",
			Description: "List the folder templates in " + board + "'s vault (global and brand-scoped). Templates make new workspace " +
				"folders; the user does that in the app — suggest one, don't try to run it.",
			InputSchema: obj(map[string]any{}),
		},
		{
			Name:        "template_params",
			Description: "A folder template's parameters in " + board + " (names, prompts, defaults), to suggest values the user confirms in the app's form.",
			InputSchema: obj(map[string]any{
				"template_id": strProp("The template's id, from template_list."),
			}, "template_id"),
		},
		{
			Name: "read_card_file",
			Description: "Read a workspace file a card is about in " + board + ": one its Workspace Files block names, or a file inside " +
				"a folder it names (get_card shows the block). Call this whenever the user asks about, quotes, summarises or wants " +
				"changes to the card's files. Text files only; long ones are truncated.",
			InputSchema: obj(map[string]any{
				"card_id": strProp("The card's id."),
				"path":    strProp("The file's workspace path: one the card names, or one inside a folder it names."),
			}, "card_id", "path"),
		},
	}
}
