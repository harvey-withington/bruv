package main

// Pending edits (Suggest mode) — accept/reject/apply workflow.
//
// Suggest mode stages tool calls as PendingEdits on the chat message
// rather than executing them immediately. This file owns the user-driven
// side of that flow: a user ticks which edits to apply, clicks Apply,
// and the staged tool calls get replayed through executeToolCall.
//
// The load-bearing invariant — "accepted IDs get applied in message
// order; everything else gets rejected" — is tested via
// internal/repo/pending_test.go through the SplitPendingEdits helper.
// See that package for the partition semantics in detail.
//
// Pin suggestions live alongside pending edits because a user can
// stage a pin via suggest_pin in Suggest mode; Accept/RejectPinSuggestion
// are the per-message resolutions of those.

import (
	"bruv/internal/config"
	"bruv/internal/llm"
	"bruv/internal/model"
	"bruv/internal/repo"
	"fmt"
	"strings"
)

// ApplyProjectPendingEdits applies the subset of accepted edits and rejects
// the rest, for a project chat message. Mirrors ApplyPendingEdits but uses the
// project executor so tool calls resolve against the project scope at apply
// time (which can differ from staging time if cards were moved/deleted).
func (a *App) ApplyProjectPendingEdits(brandSlug, streamSlug, projectSlug, msgID string, acceptIDs []string) (*model.ChatFile, error) {
	if a.repo == nil {
		return nil, fmt.Errorf("no repository open")
	}
	project, err := a.repo.GetProject(brandSlug, streamSlug, projectSlug)
	if err != nil {
		return nil, err
	}
	chatID := projectChatID(project.ID)

	acceptSet := make(map[string]bool, len(acceptIDs))
	for _, id := range acceptIDs {
		acceptSet[id] = true
	}

	cf, err := config.LoadChatFor(a.repo.Manifest.ID, chatID)
	if err != nil {
		return nil, err
	}

	// Recompute project scope at apply time. This catches cases where the
	// chat session staged edits referencing cards that no longer belong to the
	// project (moved or deleted between staging and apply), in addition to
	// the original LLM-hallucination defence.
	categories, _ := a.repo.ListCategories(brandSlug, streamSlug, projectSlug)
	applyScope := projectChatScope{
		brandSlug:   brandSlug,
		streamSlug:  streamSlug,
		projectSlug: projectSlug,
		cardIDs:     make(map[string]bool),
	}
	for _, cat := range categories {
		pins, _ := a.repo.ListCardsInCategory(cat.ID, cat.ID)
		for _, p := range pins {
			applyScope.cardIDs[p.CardID] = true
		}
	}

	// Walk the target message, applying accepted edits in order and marking
	// the rest rejected. Edits run synchronously through the project executor.
	// Failures are stamped into the edit's Detail so the user can hover to see
	// them, and we collect a count to surface as a returned error after save —
	// the frontend uses that to fire a toast.
	var failures int
	for i, m := range cf.Messages {
		if m.ID != msgID {
			continue
		}
		for j, edit := range m.PendingEdits {
			if edit.Status != "pending" {
				continue
			}
			if acceptSet[edit.ID] {
				tc := llm.ToolCall{ID: edit.ID, Name: edit.Tool, Arguments: edit.Input}
				result, _ := a.executeProjectToolCall(tc, applyScope)
				if strings.HasPrefix(result, "error:") {
					// Leave it pending so the user can retry; record the error in detail.
					cf.Messages[i].PendingEdits[j].Detail = result
					failures++
					continue
				}
				cf.Messages[i].PendingEdits[j].Status = "accepted"
			} else {
				cf.Messages[i].PendingEdits[j].Status = "rejected"
			}
		}
		break
	}

	if err := config.SaveChatFor(a.repo.Manifest.ID, cf); err != nil {
		return nil, err
	}
	// Failures are surfaced via the per-edit Detail field (which starts with
	// "error:" for failed rows). The frontend scans for those after a refresh
	// and toasts the user. We don't return a Go error here because Wails would
	// drop the cf value, and the user needs to see the updated rows so they
	// can retry the failed ones.
	_ = failures
	return cf, nil
}

// AcceptPendingEdit applies a single pending edit from Suggest mode and marks it accepted.
func (a *App) AcceptPendingEdit(cardID, msgID, editID string) (*model.ChatFile, error) {
	if a.repo == nil {
		return nil, fmt.Errorf("no repository open")
	}
	cf, err := config.LoadChatFor(a.repo.Manifest.ID, cardID)
	if err != nil {
		return nil, err
	}
	card, _ := a.repo.GetCard(cardID)
	allCats, _ := a.ListAllCategories()
	for i, m := range cf.Messages {
		if m.ID != msgID {
			continue
		}
		for j, edit := range m.PendingEdits {
			if edit.ID != editID {
				continue
			}
			if edit.Status != "pending" {
				return cf, nil
			}
			tc := llm.ToolCall{ID: editID, Name: edit.Tool, Arguments: edit.Input}
			result, _, _ := a.executeToolCall(cardID, card, tc, allCats)
			if strings.HasPrefix(result, "error:") {
				return nil, fmt.Errorf("could not apply edit: %s", result)
			}
			card, _ = a.repo.GetCard(cardID) // refresh for subsequent edits in same batch
			cf.Messages[i].PendingEdits[j].Status = "accepted"
			if err := config.SaveChatFor(a.repo.Manifest.ID, cf); err != nil {
				return nil, err
			}
			return cf, nil
		}
	}
	return nil, fmt.Errorf("pending edit not found")
}

// RejectPendingEdit dismisses a single pending edit without applying it.
func (a *App) RejectPendingEdit(cardID, msgID, editID string) (*model.ChatFile, error) {
	if a.repo == nil {
		return nil, fmt.Errorf("no repository open")
	}
	cf, err := config.LoadChatFor(a.repo.Manifest.ID, cardID)
	if err != nil {
		return nil, err
	}
	for i, m := range cf.Messages {
		if m.ID != msgID {
			continue
		}
		for j, edit := range m.PendingEdits {
			if edit.ID != editID {
				continue
			}
			if edit.Status != "pending" {
				return cf, nil
			}
			cf.Messages[i].PendingEdits[j].Status = "rejected"
			if err := config.SaveChatFor(a.repo.Manifest.ID, cf); err != nil {
				return nil, err
			}
			return cf, nil
		}
	}
	return nil, fmt.Errorf("pending edit not found")
}

// ApplyPendingEdits accepts the specified edits (in order) and rejects the rest.
// This is the primary batch action for the Suggest mode UI.
func (a *App) ApplyPendingEdits(cardID, msgID string, acceptIDs []string) (*model.ChatFile, error) {
	if a.repo == nil {
		return nil, fmt.Errorf("no repository open")
	}

	cf, err := config.LoadChatFor(a.repo.Manifest.ID, cardID)
	if err != nil {
		return nil, err
	}

	// Find the message and split its pending edits via the shared
	// helper so the accept/reject partition is covered by tests in
	// internal/repo/pending_test.go.
	var toAccept, toReject []string
	for _, m := range cf.Messages {
		if m.ID != msgID {
			continue
		}
		toAccept, toReject = repo.SplitPendingEdits(m.PendingEdits, acceptIDs)
		break
	}

	var firstErr error
	for _, eid := range toAccept {
		if updated, err2 := a.AcceptPendingEdit(cardID, msgID, eid); err2 == nil {
			cf = updated
		} else if firstErr == nil {
			firstErr = err2
		}
	}
	for _, eid := range toReject {
		if updated, err2 := a.RejectPendingEdit(cardID, msgID, eid); err2 == nil {
			cf = updated
		}
	}
	return cf, firstErr
}

// AcceptAllPendingEdits applies all pending edits on a message in order.
func (a *App) AcceptAllPendingEdits(cardID, msgID string) (*model.ChatFile, error) {
	if a.repo == nil {
		return nil, fmt.Errorf("no repository open")
	}
	// Collect IDs first so we iterate a stable snapshot
	cf, err := config.LoadChatFor(a.repo.Manifest.ID, cardID)
	if err != nil {
		return nil, err
	}
	var pendingIDs []string
	for _, m := range cf.Messages {
		if m.ID == msgID {
			for _, e := range m.PendingEdits {
				if e.Status == "pending" {
					pendingIDs = append(pendingIDs, e.ID)
				}
			}
			break
		}
	}
	for _, eid := range pendingIDs {
		if updated, err := a.AcceptPendingEdit(cardID, msgID, eid); err == nil {
			cf = updated
		}
	}
	return cf, nil
}

// RejectAllPendingEdits dismisses all pending edits on a message.
func (a *App) RejectAllPendingEdits(cardID, msgID string) (*model.ChatFile, error) {
	if a.repo == nil {
		return nil, fmt.Errorf("no repository open")
	}
	cf, err := config.LoadChatFor(a.repo.Manifest.ID, cardID)
	if err != nil {
		return nil, err
	}
	for i, m := range cf.Messages {
		if m.ID == msgID {
			for j, e := range m.PendingEdits {
				if e.Status == "pending" {
					cf.Messages[i].PendingEdits[j].Status = "rejected"
				}
			}
			return cf, config.SaveChatFor(a.repo.Manifest.ID, cf)
		}
	}
	return cf, nil
}

// --- Pin suggestions ---

// AcceptPinSuggestion accepts a pending pin suggestion on a chat message and performs the pin.
func (a *App) AcceptPinSuggestion(cardID, messageID string) error {
	if a.repo == nil {
		return fmt.Errorf("no repository open")
	}
	cf, err := config.LoadChatFor(a.repo.Manifest.ID, cardID)
	if err != nil {
		return err
	}
	for i, m := range cf.Messages {
		if m.ID == messageID && m.PinSuggestion != nil && m.PinSuggestion.Status == "pending" {
			// Pin convention: projectID == categoryID
			if err := a.PinCard(cardID, m.PinSuggestion.CategoryID, m.PinSuggestion.CategoryID); err != nil {
				return err
			}
			cf.Messages[i].PinSuggestion.Status = "accepted"
			return config.SaveChatFor(a.repo.Manifest.ID, cf)
		}
	}
	return fmt.Errorf("pin suggestion not found or already resolved")
}

// RejectPinSuggestion dismisses a pending pin suggestion on a chat message.
func (a *App) RejectPinSuggestion(cardID, messageID string) error {
	if a.repo == nil {
		return fmt.Errorf("no repository open")
	}
	cf, err := config.LoadChatFor(a.repo.Manifest.ID, cardID)
	if err != nil {
		return err
	}
	for i, m := range cf.Messages {
		if m.ID == messageID && m.PinSuggestion != nil && m.PinSuggestion.Status == "pending" {
			cf.Messages[i].PinSuggestion.Status = "rejected"
			return config.SaveChatFor(a.repo.Manifest.ID, cf)
		}
	}
	return fmt.Errorf("pin suggestion not found or already resolved")
}
