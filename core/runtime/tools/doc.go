// Package tools is the LLM tool execution surface — every tool the
// chat loop and agent runner can invoke lives here. Dispatcher owns
// the two primary entry points (ExecuteCardTool, ExecuteProjectTool)
// plus their suggest-mode staging counterparts (StageCardTool,
// StageProjectTool), along with per-tool implementations and a
// deep bench of value-coercion helpers that normalise the LLM's
// habit of sending numbers as strings and booleans as "yes".
//
// Extracted from the main-package app_tools.go in the
// LLM-runtime-extraction pass (see
// plan/llm-runtime-extraction-2026-04-24.md).
//
// The Dispatcher accesses the world via a narrow Deps interface so
// both the Wails desktop App and the headless cmd/bruv-server can
// drive the same code without dragging either host's dependencies
// into the other. Matches the pattern established by the service
// layer in core/services/*.
package tools
