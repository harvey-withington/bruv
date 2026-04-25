// Package chat is the LLM chat runtime — shared tool-calling loop
// (runChatLoop) plus the two user-facing entry points for card chat
// and project chat. Extracted from the main-package app_chat.go as
// stage 3 of the LLM-runtime extraction (see
// plan/llm-runtime-extraction-2026-04-24.md).
//
// Builds on:
//   - core/runtime/tools for tool dispatch + staging
//   - core/runtime/prompts for system prompt assembly
//   - core/runtime/promptfmt for pure formatting helpers
//
// Import alias: the main package uses `chatrt` to avoid a name
// collision with `bruv/core/services/chat` (which owns chat-history
// persistence, not the runtime).
package chat
