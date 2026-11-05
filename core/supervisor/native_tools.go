package supervisor

// nativeTools binds the native tool registry (core/boardtools) to this
// runtime for the chat dispatcher and agents — the same tools the MCP
// server serves, called in-process instead of over MCP.

import (
	"bruv/core/boardtools"
	"bruv/core/runtime/tools"
	workspacesvc "bruv/core/services/workspace"
	"bruv/internal/llm"
	"bruv/internal/model"
)

var _ boardtools.Board = (*Runtime)(nil)

// SchemaBlocks returns a card type's schema fields as empty blocks.
func (r *Runtime) SchemaBlocks(cardType string) []model.Block {
	if r.registry == nil || cardType == "" {
		return nil
	}
	return r.registry.SchemaToBlocks(cardType)
}

// WorkspaceService is the workspace service, for the workspace tools.
func (r *Runtime) WorkspaceService() *workspacesvc.Service { return r.Workspace }

type nativeTools struct{ r *Runtime }

func (n nativeTools) boardName() string {
	if n.r.repo != nil && n.r.repo.Manifest.Name != "" {
		return n.r.repo.Manifest.Name
	}
	return "this"
}

func (n nativeTools) Defs(scoped bool) []llm.ToolDef {
	return boardtools.LLMDefs(n.r, n.boardName(), scoped, nil)
}
func (n nativeTools) Has(name string) bool     { return boardtools.Has(name) }
func (n nativeTools) IsWrite(name string) bool { return boardtools.IsWrite(name) }

// Check vets a write before it's staged: in scope, and able to apply.
func (n nativeTools) Check(scope *tools.ProjectChatScope, name string, args map[string]any) error {
	if err := boardtools.CheckScope(n.r, scope, name, args); err != nil {
		return err
	}
	return boardtools.Preflight(n.r, name, args)
}
func (n nativeTools) Call(scope *tools.ProjectChatScope, name string, args map[string]any) (string, bool) {
	return boardtools.CallNative(n.r, scope, name, args)
}
func (n nativeTools) Summary(name string, args map[string]any, result string) string {
	return boardtools.Summary(name, args, result)
}
