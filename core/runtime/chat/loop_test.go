package chat

// RunLoop's turn-limit contract: running out of MaxIter is reported via
// Exhausted (so an agent run can be marked failed rather than silently
// logged as a success), and WrapUpPrompt buys one tool-less turn so the
// model still writes a real report instead of FallbackContent.

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"bruv/internal/config"
	"bruv/internal/llm"
	"bruv/internal/model"
	"bruv/internal/repo"
)

// repoDeps is stubDeps with a real repository, which RunLoop needs to
// persist messages.
type repoDeps struct {
	stubDeps
	r *repo.Repository
}

func (d *repoDeps) Repo() *repo.Repository { return d.r }

// scriptedProvider keeps calling a tool until toolTurns responses have
// been sent, then answers in text; every request is recorded.
type scriptedProvider struct {
	toolTurns int
	requests  []llm.ChatRequest
}

func (p *scriptedProvider) Name() string { return "scripted" }

func (p *scriptedProvider) ChatCompletion(_ context.Context, req llm.ChatRequest) (*llm.ChatResponse, error) {
	p.requests = append(p.requests, req)
	if len(req.Tools) > 0 && len(p.requests) <= p.toolTurns {
		return &llm.ChatResponse{ToolCalls: []llm.ToolCall{{
			ID: "c", Name: "web_search", Arguments: map[string]any{"q": len(p.requests)},
		}}}, nil
	}
	return &llm.ChatResponse{Content: "final report"}, nil
}

func newLoopRuntime(t *testing.T) *Runtime {
	t.Helper()
	config.SetConfigDir(t.TempDir())
	t.Cleanup(func() { config.SetConfigDir("") })
	r, err := repo.InitAt(t.TempDir(), "Loop")
	if err != nil {
		t.Fatal(err)
	}
	return New(&repoDeps{r: r})
}

func runLoop(t *testing.T, rt *Runtime, p *scriptedProvider, maxIter int, wrapUp string, exhausted *bool) string {
	t.Helper()
	cf := &model.ChatFile{CardID: "c1", Messages: []model.ChatMessage{{Role: model.RoleUser, Content: "go"}}}
	out, err := rt.RunLoop(context.Background(), p, "m", cf, LoopConfig{
		ChatID:          "loop-test",
		Tools:           []llm.ToolDef{{Name: "web_search"}},
		MaxIter:         maxIter,
		ExecuteTool:     func(llm.ToolCall) (string, *model.ToolAction, *model.PinSuggestion) { return "ok", nil, nil },
		FallbackContent: "fallback",
		WrapUpPrompt:    wrapUp,
		Exhausted:       exhausted,
	})
	if err != nil {
		t.Fatalf("RunLoop: %v", err)
	}
	return out.Messages[len(out.Messages)-1].Content
}

func TestRunLoopExhaustedWrapsUpWithoutTools(t *testing.T) {
	rt := newLoopRuntime(t)
	p := &scriptedProvider{toolTurns: 100}
	var exhausted bool
	got := runLoop(t, rt, p, 3, "report now", &exhausted)

	if !exhausted {
		t.Error("Exhausted not set after MaxIter ran out")
	}
	if got != "final report" {
		t.Errorf("last message = %q, want the wrap-up report", got)
	}
	if len(p.requests) != 4 {
		t.Fatalf("requests = %d, want 3 turns + 1 wrap-up", len(p.requests))
	}
	last := p.requests[3]
	if len(last.Tools) != 0 {
		t.Error("the wrap-up turn must offer no tools")
	}
	if m := last.Messages[len(last.Messages)-1]; m.Role != model.RoleUser || m.Content != "report now" {
		t.Errorf("wrap-up prompt not sent last: %+v", m)
	}
}

func TestRunLoopExhaustedWithoutWrapUpUsesFallback(t *testing.T) {
	rt := newLoopRuntime(t)
	var exhausted bool
	if got := runLoop(t, rt, &scriptedProvider{toolTurns: 100}, 2, "", &exhausted); got != "fallback" || !exhausted {
		t.Errorf("got %q exhausted=%v, want fallback + exhausted", got, exhausted)
	}
}

// A reply that can't be saved is an error, not a silent (nil, nil): the
// reply used to vanish while an agent run recorded success with no
// summary.
func TestRunLoopReportsUnsavedReply(t *testing.T) {
	rt := newLoopRuntime(t)
	blocker := filepath.Join(t.TempDir(), "not-a-dir")
	if err := os.WriteFile(blocker, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	config.SetConfigDir(blocker) // every chat save now fails
	cf := &model.ChatFile{CardID: "c1", Messages: []model.ChatMessage{{Role: model.RoleUser, Content: "go"}}}
	out, err := rt.RunLoop(context.Background(), &scriptedProvider{}, "m", cf, LoopConfig{
		ChatID:      "loop-test",
		MaxIter:     3,
		ExecuteTool: func(llm.ToolCall) (string, *model.ToolAction, *model.PinSuggestion) { return "ok", nil, nil },
	})
	if err == nil {
		t.Fatal("RunLoop must report a reply it could not save")
	}
	if out == nil {
		t.Error("the caller should keep the history it passed in")
	}
}

func TestRunLoopFinishingEarlyIsNotExhausted(t *testing.T) {
	rt := newLoopRuntime(t)
	var exhausted bool
	if got := runLoop(t, rt, &scriptedProvider{toolTurns: 1}, 5, "report now", &exhausted); got != "final report" || exhausted {
		t.Errorf("got %q exhausted=%v, want the model's own reply, not exhausted", got, exhausted)
	}
}
