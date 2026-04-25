// Package agent is the agent execution runtime — scheduler + due-date
// scanner + the executeAgent end-to-end run loop + the agent-specific
// tool dispatchers (including MCP tool bridging).
//
// Stage 4 of the LLM-runtime extraction (see
// plan/llm-runtime-extraction-2026-04-24.md). Builds on:
//   - core/runtime/tools for shared value coercion + a few helpers
//   - core/runtime/prompts for agent system prompt assembly
//   - core/runtime/chat for the shared LLM tool-calling loop
//
// The runtime accesses the world via a narrow Deps interface so both
// the Wails desktop App and the headless cmd/bruv-server can drive
// agents identically.
//
// Wails-only concerns (tray state, force-quit, refreshTrayTooltip)
// stay on the App shell — the Runtime only touches event-bus publish
// for any user-visible signalling, which works in both hosts.
package agent
