package supervisor

// nativeTools binds the native tool registry (core/boardtools) to this
// runtime for the chat dispatcher and agents — the same tools the MCP
// server serves, called in-process instead of over MCP.

import (
	"bruv/core/boardtools"
	"bruv/core/runtime/tools"
	"bruv/internal/llm"
)

var _ boardtools.Board = (*Runtime)(nil)

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
func (n nativeTools) Check(scope *tools.ProjectChatScope, name string, args map[string]any) error {
	return boardtools.CheckScope(n.r, scope, name, args)
}
func (n nativeTools) Call(scope *tools.ProjectChatScope, name string, args map[string]any) (string, bool) {
	return boardtools.CallNative(n.r, scope, name, args)
}
func (n nativeTools) Summary(name string, args map[string]any, result string) string {
	return boardtools.Summary(name, args, result)
}
