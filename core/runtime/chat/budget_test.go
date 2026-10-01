package chat

// Chat output budgets and unusual endings: every turn that doesn't end
// in a normal reply ends in a notice the UI can word, never a blank
// assistant bubble (the Opus 5.5 empty-reply bug, 2026-10-01).

import (
	"context"
	"testing"
	"time"

	"bruv/internal/config"
	"bruv/internal/llm"
	"bruv/internal/model"
)

// funcProvider answers with a function, recording every request.
type funcProvider struct {
	answer   func(ctx context.Context, req llm.ChatRequest) (*llm.ChatResponse, error)
	requests []llm.ChatRequest
}

func (p *funcProvider) Name() string { return "func" }

func (p *funcProvider) ChatCompletion(ctx context.Context, req llm.ChatRequest) (*llm.ChatResponse, error) {
	p.requests = append(p.requests, req)
	return p.answer(ctx, req)
}

func usage(out int) *llm.Usage {
	return &llm.Usage{PromptTokens: 100, CompletionTokens: out, TotalTokens: 100 + out}
}

type progressEvent struct {
	used int
	over bool
}

// runBudgetLoop runs one turn against p and returns the closing message
// and the progress reports.
func runBudgetLoop(t *testing.T, ctx context.Context, rt *Runtime, p llm.Provider, b config.ChatBudget) (model.ChatMessage, []progressEvent) {
	t.Helper()
	var progress []progressEvent
	cf := &model.ChatFile{CardID: "c1", Messages: []model.ChatMessage{{Role: model.RoleUser, Content: "go"}}}
	out, err := rt.RunLoop(ctx, p, "m", cf, LoopConfig{
		ChatID:     "loop-test",
		Tools:      []llm.ToolDef{{Name: "web_search"}},
		MaxIter:    5,
		Budget:     b,
		OnProgress: func(used int, over bool) { progress = append(progress, progressEvent{used, over}) },
		ExecuteTool: func(llm.ToolCall) (string, *model.ToolAction, *model.PinSuggestion) {
			return "ok", &model.ToolAction{Tool: "web_search"}, nil
		},
	})
	if err != nil {
		t.Fatalf("RunLoop: %v", err)
	}
	return out.Messages[len(out.Messages)-1], progress
}

func wantNotice(t *testing.T, msg model.ChatMessage, role, code string) {
	t.Helper()
	if msg.Role != role {
		t.Errorf("role = %q, want %q", msg.Role, role)
	}
	if msg.Notice == nil || msg.Notice.Code != code {
		t.Fatalf("notice = %+v, want %q", msg.Notice, code)
	}
}

// Hard budget: each call may generate only what's left; reaching it
// keeps the partial reply and says it was cut off at the budget.
func TestHardBudgetCapsEachCallAndCutsOff(t *testing.T) {
	rt := newLoopRuntime(t)
	p := &funcProvider{answer: func(_ context.Context, req llm.ChatRequest) (*llm.ChatResponse, error) {
		if req.MaxTokens == 1000 {
			return &llm.ChatResponse{ToolCalls: []llm.ToolCall{{ID: "1", Name: "web_search"}}, Usage: usage(300), StopReason: llm.StopToolUse}, nil
		}
		return &llm.ChatResponse{Content: "The lyrics so f", Usage: usage(req.MaxTokens), StopReason: llm.StopMaxTokens}, nil
	}}
	msg, _ := runBudgetLoop(t, context.Background(), rt, p, config.ChatBudget{Mode: config.ChatBudgetHard, Tokens: 1000})

	if len(p.requests) != 2 || p.requests[1].MaxTokens != 700 {
		t.Fatalf("second call cap = %v, want 700 (1000 budget - 300 used)", p.requests)
	}
	wantNotice(t, msg, model.RoleAssistant, model.ChatNoticeBudgetReached)
	if msg.Content != "The lyrics so f" || len(msg.ToolActions) != 1 {
		t.Errorf("partial reply or tool actions lost: %+v", msg)
	}
	if msg.Notice.Budget != 1000 || msg.Notice.Used != 1000 {
		t.Errorf("notice = %+v, want budget 1000 used 1000", msg.Notice)
	}
}

// A hard budget spent by tool rounds ends the turn before another call.
func TestHardBudgetSpentStopsBeforeNextCall(t *testing.T) {
	rt := newLoopRuntime(t)
	p := &funcProvider{answer: func(context.Context, llm.ChatRequest) (*llm.ChatResponse, error) {
		return &llm.ChatResponse{ToolCalls: []llm.ToolCall{{ID: "1", Name: "web_search"}}, Usage: usage(500), StopReason: llm.StopToolUse}, nil
	}}
	msg, _ := runBudgetLoop(t, context.Background(), rt, p, config.ChatBudget{Mode: config.ChatBudgetHard, Tokens: 500})
	if len(p.requests) != 1 {
		t.Errorf("requests = %d, want 1", len(p.requests))
	}
	wantNotice(t, msg, model.RoleAssistant, model.ChatNoticeBudgetReached)
}

// Warn budget: calls run up to the ceiling; passing the budget is
// reported live and noted on the finished reply.
func TestWarnBudgetReportsAndNotesOverrun(t *testing.T) {
	rt := newLoopRuntime(t)
	p := &funcProvider{answer: func(_ context.Context, req llm.ChatRequest) (*llm.ChatResponse, error) {
		req.OnOutput(200) // streamed text so far: under budget
		req.OnOutput(1500)
		return &llm.ChatResponse{Content: "Verse one…", Usage: usage(2400), StopReason: llm.StopEnd}, nil
	}}
	msg, progress := runBudgetLoop(t, context.Background(), rt, p, config.ChatBudget{Mode: config.ChatBudgetWarn, Tokens: 1000})

	if p.requests[0].MaxTokens != config.MaxChatOutputTokens {
		t.Errorf("cap = %d, want the ceiling %d", p.requests[0].MaxTokens, config.MaxChatOutputTokens)
	}
	wantNotice(t, msg, model.RoleAssistant, model.ChatNoticeOverBudget)
	if msg.Content != "Verse one…" || msg.Notice.Used != 2400 || msg.Notice.Budget != 1000 {
		t.Errorf("msg = %+v notice = %+v", msg, msg.Notice)
	}
	// First report, the crossing (published at once despite the
	// throttle), and the call's real total.
	want := []progressEvent{{200, false}, {1500, true}, {2400, true}}
	if len(progress) != len(want) {
		t.Fatalf("progress = %v, want %v", progress, want)
	}
	for i := range want {
		if progress[i] != want[i] {
			t.Errorf("progress[%d] = %v, want %v", i, progress[i], want[i])
		}
	}
}

func TestWarnBudgetUnderBudgetHasNoNotice(t *testing.T) {
	rt := newLoopRuntime(t)
	p := &funcProvider{answer: func(context.Context, llm.ChatRequest) (*llm.ChatResponse, error) {
		return &llm.ChatResponse{Content: "Short.", Usage: usage(80), StopReason: llm.StopEnd}, nil
	}}
	msg, _ := runBudgetLoop(t, context.Background(), rt, p, config.ChatBudget{Mode: config.ChatBudgetWarn, Tokens: 1000})
	if msg.Notice != nil || msg.Role != model.RoleAssistant {
		t.Errorf("msg = %+v, want a plain reply", msg)
	}
}

// Off: the ceiling still applies, hitting it says "cut off", and nothing
// is ever reported as over budget.
func TestOffBudgetCutOffAtCeiling(t *testing.T) {
	rt := newLoopRuntime(t)
	p := &funcProvider{answer: func(context.Context, llm.ChatRequest) (*llm.ChatResponse, error) {
		return &llm.ChatResponse{Usage: usage(config.MaxChatOutputTokens), StopReason: llm.StopMaxTokens}, nil
	}}
	msg, progress := runBudgetLoop(t, context.Background(), rt, p, config.ChatBudget{Mode: config.ChatBudgetOff, Tokens: 1000})
	// Nothing arrived: a system message with the notice, not a blank reply.
	wantNotice(t, msg, model.RoleSystem, model.ChatNoticeCutOff)
	for _, e := range progress {
		if e.over {
			t.Errorf("off mode reported over budget: %v", progress)
		}
	}
}

// A cut-off response never runs the tool calls that came with it.
func TestCutOffRunsNoTools(t *testing.T) {
	rt := newLoopRuntime(t)
	ran := false
	p := &funcProvider{answer: func(context.Context, llm.ChatRequest) (*llm.ChatResponse, error) {
		return &llm.ChatResponse{ToolCalls: []llm.ToolCall{{ID: "1", Name: "web_search"}}, Usage: usage(64000), StopReason: llm.StopMaxTokens}, nil
	}}
	cf := &model.ChatFile{CardID: "c1", Messages: []model.ChatMessage{{Role: model.RoleUser, Content: "go"}}}
	if _, err := rt.RunLoop(context.Background(), p, "m", cf, LoopConfig{
		ChatID:      "loop-test",
		MaxIter:     3,
		Budget:      config.ChatBudget{Mode: config.ChatBudgetWarn, Tokens: 1000},
		ExecuteTool: func(llm.ToolCall) (string, *model.ToolAction, *model.PinSuggestion) { ran = true; return "", nil, nil },
	}); err != nil {
		t.Fatal(err)
	}
	if ran {
		t.Error("a tool call from a cut-off response ran")
	}
}

func TestRefusalAndEmptyRepliesEndInNotices(t *testing.T) {
	for name, c := range map[string]struct {
		resp *llm.ChatResponse
		code string
	}{
		"refused": {&llm.ChatResponse{StopReason: llm.StopRefusal}, model.ChatNoticeRefused},
		"empty":   {&llm.ChatResponse{Content: "  ", StopReason: llm.StopEnd}, model.ChatNoticeEmpty},
	} {
		t.Run(name, func(t *testing.T) {
			rt := newLoopRuntime(t)
			p := &funcProvider{answer: func(context.Context, llm.ChatRequest) (*llm.ChatResponse, error) { return c.resp, nil }}
			msg, _ := runBudgetLoop(t, context.Background(), rt, p, config.ChatBudget{Mode: config.ChatBudgetWarn, Tokens: 1000})
			wantNotice(t, msg, model.RoleSystem, c.code)
			if msg.Content == "" {
				t.Error("a notice message keeps its English text for logs and agent runs")
			}
		})
	}
}

// Stop cancels the call in flight; the turn closes with a "stopped"
// notice rather than a provider error.
func TestStopEndsTheTurnWithANotice(t *testing.T) {
	rt := newLoopRuntime(t)
	started := make(chan struct{})
	p := &funcProvider{answer: func(ctx context.Context, _ llm.ChatRequest) (*llm.ChatResponse, error) {
		close(started)
		<-ctx.Done()
		return nil, ctx.Err()
	}}
	ctx, endTurn := rt.beginTurn("loop-test")
	defer endTurn()
	go func() {
		<-started
		if !rt.Stop("loop-test") {
			t.Error("Stop found no running turn")
		}
	}()

	done := make(chan model.ChatMessage, 1)
	go func() {
		msg, _ := runBudgetLoop(t, ctx, rt, p, config.ChatBudget{Mode: config.ChatBudgetWarn, Tokens: 1000})
		done <- msg
	}()
	select {
	case msg := <-done:
		wantNotice(t, msg, model.RoleSystem, model.ChatNoticeStopped)
	case <-time.After(5 * time.Second):
		t.Fatal("the turn did not end after Stop")
	}
}

func TestStopWithNoTurnRunning(t *testing.T) {
	rt := newLoopRuntime(t)
	_, endTurn := rt.beginTurn("other")
	endTurn()
	if rt.Stop("other") || rt.Stop("never-started") {
		t.Error("Stop reported a turn that isn't running")
	}
}

// Agents set no chat budget: their calls keep the provider default cap.
func TestNoBudgetLeavesProviderDefault(t *testing.T) {
	rt := newLoopRuntime(t)
	p := &funcProvider{answer: func(context.Context, llm.ChatRequest) (*llm.ChatResponse, error) {
		return &llm.ChatResponse{Content: "done", Usage: usage(10), StopReason: llm.StopEnd}, nil
	}}
	msg, progress := runBudgetLoop(t, context.Background(), rt, p, config.ChatBudget{})
	if p.requests[0].MaxTokens != 0 || msg.Notice != nil || len(progress) != 0 {
		t.Errorf("cap = %d notice = %+v progress = %v, want default cap, no notice, no reports", p.requests[0].MaxTokens, msg.Notice, progress)
	}
}

func TestChatBudgetDefaults(t *testing.T) {
	for _, c := range []struct {
		cfg  config.LLMConfig
		want config.ChatBudget
	}{
		{config.LLMConfig{}, config.ChatBudget{Mode: config.ChatBudgetWarn, Tokens: config.DefaultChatBudgetTokens}},
		{config.LLMConfig{ChatBudgetMode: "bogus", ChatBudgetTokens: -5}, config.ChatBudget{Mode: config.ChatBudgetWarn, Tokens: config.DefaultChatBudgetTokens}},
		{config.LLMConfig{ChatBudgetMode: config.ChatBudgetHard, ChatBudgetTokens: 2000}, config.ChatBudget{Mode: config.ChatBudgetHard, Tokens: 2000}},
		{config.LLMConfig{ChatBudgetMode: config.ChatBudgetOff, ChatBudgetTokens: 999999}, config.ChatBudget{Mode: config.ChatBudgetOff, Tokens: config.MaxChatOutputTokens}},
	} {
		if got := c.cfg.ChatBudget(); got != c.want {
			t.Errorf("%+v.ChatBudget() = %+v, want %+v", c.cfg, got, c.want)
		}
	}
}
