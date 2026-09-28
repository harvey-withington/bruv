package mcpserver

// End-to-end coverage for the card-agent tools: an MCP client builds a
// working agent from scratch using only get_card_agent's options.

import (
	"strings"
	"testing"
)

func createTestCard(t *testing.T, h *Handler, cardType string) string {
	t.Helper()
	var created struct {
		CardID string `json:"card_id"`
	}
	decodeJSON(t, mustCallTool(t, h, "create_card", map[string]any{"title": "Price watch", "card_type": cardType}), &created)
	return created.CardID
}

func TestConfigureCardAgentEndToEnd(t *testing.T) {
	h, _ := newTestHandler(t)
	cardID := createTestCard(t, h, "agent")

	var before struct {
		Config struct {
			Enabled bool `json:"enabled"`
		} `json:"config"`
		Options struct {
			Tools []struct {
				ID string `json:"id"`
			} `json:"tools"`
			NotifyOn []string `json:"notify_on"`
		} `json:"options"`
	}
	decodeJSON(t, mustCallTool(t, h, "get_card_agent", map[string]any{"card_id": cardID}), &before)
	if before.Config.Enabled {
		t.Fatal("new card already has an enabled agent")
	}
	var toolIDs []any
	for _, tool := range before.Options.Tools {
		if tool.ID == "web_search" || tool.ID == "update_self" {
			toolIDs = append(toolIDs, tool.ID)
		}
	}
	if len(toolIDs) != 2 || len(before.Options.NotifyOn) == 0 {
		t.Fatalf("options missing built-ins or notify values: %+v", before.Options)
	}

	var saved struct {
		Config struct {
			Enabled      bool     `json:"enabled"`
			Goal         string   `json:"goal"`
			Status       string   `json:"status"`
			NextRunAt    string   `json:"next_run_at"`
			AllowedTools []string `json:"allowed_tools"`
			Timezone     string   `json:"timezone"`
		} `json:"config"`
		Warnings []string `json:"warnings"`
	}
	decodeJSON(t, mustCallTool(t, h, "configure_card_agent", map[string]any{
		"card_id": cardID, "enabled": true, "goal": "Search for flight prices and record the cheapest.",
		"schedule": "0 9 * * *", "timezone": "Europe/London", "allowed_tools": toolIDs,
		"notify_on": []any{"success", "failure"}, "max_retries": 2,
	}), &saved)
	if !saved.Config.Enabled || saved.Config.Status != "idle" || saved.Config.NextRunAt == "" {
		t.Fatalf("agent not scheduled: %+v", saved.Config)
	}
	if len(saved.Config.AllowedTools) != 2 || saved.Config.Timezone != "Europe/London" {
		t.Errorf("config not applied: %+v", saved.Config)
	}

	// Partial update keeps the rest.
	decodeJSON(t, mustCallTool(t, h, "configure_card_agent", map[string]any{
		"card_id": cardID, "goal": "Updated goal",
	}), &saved)
	if saved.Config.Goal != "Updated goal" || !saved.Config.Enabled || len(saved.Config.AllowedTools) != 2 {
		t.Errorf("partial update clobbered config: %+v", saved.Config)
	}
}

func TestConfigureCardAgentRejects(t *testing.T) {
	h, _ := newTestHandler(t)
	cardID := createTestCard(t, h, "brainstorm")
	cases := map[string]map[string]any{
		"unknown tool":     {"card_id": cardID, "goal": "g", "allowed_tools": []any{"launch_rockets"}},
		"bad schedule":     {"card_id": cardID, "goal": "g", "schedule": "whenever"},
		"no goal":          {"card_id": cardID, "enabled": true},
		"unknown account":  {"card_id": cardID, "llm_account_id": "nope"},
		"nothing to set":   {"card_id": cardID},
		"missing card":     {"card_id": "no-such-card", "goal": "g"},
		"wrong value type": {"card_id": cardID, "enabled": "true"},
	}
	for name, args := range cases {
		if text, isErr := callToolRPC(t, h, "configure_card_agent", args); !isErr {
			t.Errorf("%s: expected an error, got %s", name, text)
		}
	}
}

func TestConfigureCardAgentWarnsWhenUnscheduled(t *testing.T) {
	h, _ := newTestHandler(t)
	cardID := createTestCard(t, h, "agent")
	text := mustCallTool(t, h, "configure_card_agent", map[string]any{"card_id": cardID, "enabled": true, "goal": "g"})
	if !strings.Contains(text, "run_card_agent") {
		t.Errorf("expected a no-schedule warning pointing at run_card_agent, got %s", text)
	}
}

func TestRunCardAgentNeedsGoal(t *testing.T) {
	h, _ := newTestHandler(t)
	cardID := createTestCard(t, h, "agent")
	if text, isErr := callToolRPC(t, h, "run_card_agent", map[string]any{"card_id": cardID}); !isErr {
		t.Errorf("expected an error for a card with no goal, got %s", text)
	}
}
