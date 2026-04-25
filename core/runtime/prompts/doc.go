// Package prompts owns the three LLM system-prompt builders that
// prepare the conversational surface for card chat, project chat,
// and agent runs.
//
// Extracted from the main-package app_chat.go + app_agent.go as
// stage 2 of the LLM-runtime extraction (see
// plan/llm-runtime-extraction-2026-04-24.md). Builds on stage 1
// (core/runtime/tools) and on the pure formatting helpers in
// core/runtime/promptfmt.
//
// The Builder accesses the world via a narrow Deps interface so
// both the Wails desktop App and the headless cmd/bruv-server can
// produce identical prompts without dragging either host's
// dependencies into the other.
package prompts
