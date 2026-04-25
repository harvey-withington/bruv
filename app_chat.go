package main

// LLM chat — Wails-bound surface + thin forwarders.
//
// The chat runtime (runChatLoop, SendCard/SendProject cores,
// saveUserMessage helper, chatLoopConfig struct) lives in
// core/runtime/chat. The system-prompt builders live in
// core/runtime/prompts. Value-coercion + tool dispatch live in
// core/runtime/tools. This file keeps only the Wails-bound method
// forwarders that the generated ShellAPI-era bindings used to expose,
// plus chat-history shims and local wrappers around the pure
// formatting helpers in core/runtime/promptfmt.

import (
	"bruv/core/runtime/promptfmt"
	chatsvc "bruv/core/services/chat"
	"bruv/internal/model"
)

// --- Chat history (forwarders to core/services/chat) ---

func (a *App) LoadChatHistory(cardID string) (*model.ChatFile, error) {
	return a.chat.LoadCardHistory(cardID)
}

// projectChatID returns the synthetic chat ID used to store project
// chat messages. Kept as a package-level helper so code outside the
// chat runtime (notifications, debugging) can compute it.
func projectChatID(projectID string) string { return chatsvc.ProjectChatID(projectID) }

func (a *App) LoadProjectChatHistory(brandSlug, streamSlug, projectSlug string) (*model.ChatFile, error) {
	return a.chat.LoadProjectHistory(brandSlug, streamSlug, projectSlug)
}

func (a *App) ClearProjectChatHistory(brandSlug, streamSlug, projectSlug string) error {
	return a.chat.ClearProjectHistory(brandSlug, streamSlug, projectSlug)
}

func (a *App) ClearCardChatHistory(cardID string) error {
	return a.chat.ClearCardHistory(cardID)
}

// --- Chat runtime forwarders (core/runtime/chat) ---

// SendChatMessage is the Wails-bound entry point for per-card chat.
// Forwards to the chat runtime.
func (a *App) SendChatMessage(cardID, userMessage string) (*model.ChatFile, error) {
	return a.chatRT.SendCard(cardID, userMessage)
}

// SendProjectChatMessage is the Wails-bound entry point for
// project-level chat. Forwards to the chat runtime.
func (a *App) SendProjectChatMessage(brandSlug, streamSlug, projectSlug, userMessage, contextLevel string) (*model.ChatFile, error) {
	return a.chatRT.SendProject(brandSlug, streamSlug, projectSlug, userMessage, contextLevel)
}

// --- Promptfmt wrappers ---
//
// availableIconList + renderCategoryHeader are one-line wrappers so
// other main-package files (app_agent.go in particular) don't each
// need to import core/runtime/promptfmt. The compiler inlines these.

func availableIconList() string { return promptfmt.AvailableIconList() }
func renderCategoryHeader(cat model.Category) string {
	return promptfmt.RenderCategoryHeader(cat)
}
