package boardtools

// Agent tools: read, configure and trigger a card's autonomous agent.
// Reading and configuring are agentsvc.Describe / agentsvc.Configure —
// the same implementation card and project chat call — so every surface
// validates, schedules and reports identically.

import (
	"strings"
)

func hGetCardAgent(rt Board, a map[string]any) (string, bool) {
	cardID := argStr(a, "card_id")
	if cardID == "" {
		return errResult("card_id is required")
	}
	view, err := rt.AgentService().Describe(cardID)
	if err != nil {
		return errResult("%v", err)
	}
	return jsonResult(view)
}

func hConfigureCardAgent(rt Board, a map[string]any) (string, bool) {
	cardID := argStr(a, "card_id")
	if cardID == "" {
		return errResult("card_id is required")
	}
	res, err := rt.AgentService().Configure(cardID, a)
	if err != nil {
		return errResult("%v", err)
	}
	return jsonResult(res)
}

func hRunCardAgent(rt Board, a map[string]any) (string, bool) {
	cardID := argStr(a, "card_id")
	if cardID == "" {
		return errResult("card_id is required")
	}
	af, err := rt.GetAgentConfig(cardID)
	if err != nil {
		return errResult("%v", err)
	}
	if strings.TrimSpace(af.Config.Goal) == "" {
		return errResult("card %s has no agent goal; set one with configure_card_agent first", cardID)
	}
	if err := rt.TriggerAgent(cardID); err != nil {
		return errResult("%v", err)
	}
	return jsonResult(map[string]any{
		"card_id": cardID, "started": true,
		"note": "The run is asynchronous; call get_card_agent shortly to see its result in recent_runs.",
	})
}
