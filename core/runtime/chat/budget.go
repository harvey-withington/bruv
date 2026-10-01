package chat

// Chat output budgets (Harvey, 2026-10-01). A thinking model can spend a
// whole fixed output cap thinking and come back with nothing, so a chat
// turn runs against the user's budget instead — thinking, text and tool
// calls across every call in the turn:
//   - hard: each call may generate what is left of the budget; reaching
//     it cuts the reply off with a notice.
//   - warn: calls run up to config.MaxChatOutputTokens; passing the
//     budget is reported live (the user can press Stop) and noted on the
//     reply.
//   - off: calls run up to config.MaxChatOutputTokens, no warnings.

import (
	"time"

	"bruv/internal/config"
	"bruv/internal/llm"
	"bruv/internal/model"
)

// TopicChatProgress is the event a running chat turn publishes.
const TopicChatProgress = "chat:progress"

// progressInterval throttles streamed progress events. Crossing the
// budget and finishing a call always publish at once.
const progressInterval = time.Second

// TurnProgress is the chat:progress payload. Exactly one of CardID and
// ProjectPath ("brand/stream/project") names the chat, in the terms each
// client already holds.
type TurnProgress struct {
	CardID      string `json:"card_id,omitempty"`
	ProjectPath string `json:"project_path,omitempty"`
	Used        int    `json:"used"` // output tokens so far; streamed text is estimated
	Budget      int    `json:"budget"`
	OverBudget  bool   `json:"over_budget"` // warn mode only
}

// turnBudget tracks one turn's output tokens against its budget. All
// methods run on the turn's goroutine (providers call OnOutput from
// inside ChatCompletion), so it needs no locking.
type turnBudget struct {
	budget     config.ChatBudget // zero Mode: no budget (agents)
	used       int               // output tokens of finished calls
	report     func(used int, over bool)
	lastReport time.Time
	lastOver   bool
}

func newTurnBudget(b config.ChatBudget, report func(used int, over bool)) *turnBudget {
	return &turnBudget{budget: b, report: report}
}

// maxTokens is the next call's output cap; 0 leaves the provider default.
func (b *turnBudget) maxTokens() int {
	switch b.budget.Mode {
	case "":
		return 0
	case config.ChatBudgetHard:
		return b.budget.Tokens - b.used
	default:
		return config.MaxChatOutputTokens
	}
}

// exhausted reports whether a hard budget has nothing left for another call.
func (b *turnBudget) exhausted() bool {
	return b.budget.Mode == config.ChatBudgetHard && b.used >= b.budget.Tokens
}

func (b *turnBudget) overAt(used int) bool {
	return b.budget.Mode == config.ChatBudgetWarn && used > b.budget.Tokens
}

// streamed takes a provider's running estimate for the call in flight.
func (b *turnBudget) streamed(estimate int) { b.publish(b.used+estimate, false) }

// callFinished adds a finished call's real output to the turn.
func (b *turnBudget) callFinished(u *llm.Usage) {
	if u != nil {
		b.used += u.CompletionTokens
	}
	b.publish(b.used, true)
}

func (b *turnBudget) publish(used int, force bool) {
	if b.report == nil || b.budget.Mode == "" {
		return
	}
	over := b.overAt(used)
	if !force && over == b.lastOver && time.Since(b.lastReport) < progressInterval {
		return
	}
	b.lastReport, b.lastOver = time.Now(), over
	b.report(used, over)
}

// notice builds a notice carrying the turn's budget and usage.
func (b *turnBudget) notice(code string) *model.ChatNotice {
	return &model.ChatNotice{Code: code, Budget: b.budget.Tokens, Used: b.used}
}

// overNotice is the over-budget note for a reply that finished past a
// warn budget, or nil.
func (b *turnBudget) overNotice() *model.ChatNotice {
	if !b.overAt(b.used) {
		return nil
	}
	return b.notice(model.ChatNoticeOverBudget)
}

// cutOffNotice explains a reply that hit its call's output cap: under a
// hard budget the cap was the budget; otherwise it was the ceiling.
func (b *turnBudget) cutOffNotice() *model.ChatNotice {
	if b.budget.Mode == config.ChatBudgetHard {
		return b.notice(model.ChatNoticeBudgetReached)
	}
	return b.notice(model.ChatNoticeCutOff)
}

// progressReporter publishes chat:progress for one chat.
func (rt *Runtime) progressReporter(base TurnProgress) func(used int, over bool) {
	return func(used int, over bool) {
		p := base
		p.Used, p.OverBudget = used, over
		rt.deps.Publish(TopicChatProgress, p)
	}
}
