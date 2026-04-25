// Package server is the headless BRUV backend — same domain code the
// desktop App wires up, but without Wails. Used by:
//
//   - The unified bruv.exe binary's `--server` mode (entry point in
//     main.go forwards here when the flag is set).
//   - The thin cmd/bruv-server wrapper, kept for `go run` ergonomics
//     during development.
//
// Single-binary deployment: the same .exe runs as the desktop client
// (no flag) or as a headless server (--server). The logic below
// builds the open repository, wires every service + LLM-runtime piece,
// stands up the HTTP transport, and blocks on signals until shutdown.
package server

import (
	"context"
	"fmt"
	"io/fs"
	"log/slog"
	"os"
	"os/signal"
	"path/filepath"
	"sync"
	"syscall"
	"time"

	"bruv/core/events"
	"bruv/core/reposync"
	agentrt "bruv/core/runtime/agent"
	chatrt "bruv/core/runtime/chat"
	"bruv/core/runtime/prompts"
	"bruv/core/runtime/tools"
	agentsvc "bruv/core/services/agentsvc"
	"bruv/core/services/card"
	"bruv/core/services/catalog"
	chatsvc "bruv/core/services/chat"
	llmsvc "bruv/core/services/llm"
	"bruv/core/services/mcpsvc"
	"bruv/core/services/notify"
	projectsvc "bruv/core/services/project"
	reposvc "bruv/core/services/repository"
	"bruv/core/services/search"
	"bruv/core/services/settings"
	"bruv/internal/config"
	"bruv/internal/index"
	"bruv/internal/mcp"
	"bruv/internal/model"
	"bruv/internal/repo"
	"bruv/internal/schema"
	transporthttp "bruv/transport/http"
)

// Options captures everything the entry points need to configure
// before starting the server. Defaults are filled in by Run.
type Options struct {
	RepoPath  string // required
	Addr      string // default 127.0.0.1:9870
	ConfigDir string // default <user-config>/bruv/
	Version   string // build-stamped, defaults to "dev"
	BuildDate string // build-stamped, defaults to "unknown"
	// Assets is the embedded Svelte bundle to serve at /app/*.
	// Pass frontend.Assets() at the call site so this package
	// stays free of the frontend embed (which would otherwise
	// import-cycle through anything that depends on it).
	Assets fs.FS
}

// Run starts the headless server, blocks until SIGINT/SIGTERM, then
// shuts down cleanly. Returns the first fatal error encountered, or
// nil on a clean signal-driven shutdown.
func Run(opts Options) error {
	if opts.RepoPath == "" {
		return fmt.Errorf("server.Run: RepoPath is required")
	}
	if opts.Addr == "" {
		opts.Addr = "127.0.0.1:9870"
	}
	if opts.Version == "" {
		opts.Version = "dev"
	}
	if opts.BuildDate == "" {
		opts.BuildDate = "unknown"
	}
	if opts.ConfigDir == "" {
		dir, err := config.ConfigDir()
		if err != nil {
			return fmt.Errorf("resolve config dir: %w", err)
		}
		opts.ConfigDir = dir
	}

	slog.Info("bruv-server starting", "version", opts.Version, "build_date", opts.BuildDate)

	rt, err := buildHeadlessRuntime(opts.RepoPath, opts.ConfigDir)
	if err != nil {
		return fmt.Errorf("runtime build: %w", err)
	}
	defer rt.Close()

	srv, err := transporthttp.New(transporthttp.Config{
		Addr:         opts.Addr,
		ConfigDir:    opts.ConfigDir,
		Version:      opts.Version,
		BuildDate:    opts.BuildDate,
		StaticAssets: opts.Assets,
		Attachments: &transporthttp.AttachmentConfig{
			Secret:  config.LoadServerSecret(),
			Resolve: rt.resolveAttachment,
		},
	}, rt, rt.bus)
	if err != nil {
		return fmt.Errorf("http transport construct: %w", err)
	}
	if err := srv.Start(); err != nil {
		return fmt.Errorf("http transport start: %w", err)
	}
	slog.Info("bruv-server listening",
		"addr", srv.Addr(),
		"bootstrap_token", filepath.Join(opts.ConfigDir, "bootstrap-token.txt"))

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	sig := <-sigCh
	slog.Info("signal received, shutting down", "signal", sig)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_ = srv.Stop()
	_ = ctx
	return nil
}

// ---------------------------------------------------------------------
// Headless runtime — holds the open repo + wired-up services and
// satisfies every service's Deps interface via method receivers on
// itself. Same shape as the desktop App but without Wails.
// ---------------------------------------------------------------------

type headlessRuntime struct {
	ctx       context.Context
	cancelCtx context.CancelFunc

	repo        *repo.Repository
	idx         *index.Index
	registry    *schema.Registry
	mcpRegistry *mcp.Registry
	bus         *events.MemBus
	watcher     *reposync.Watcher

	// llmActors tracks active chat/agent sessions by cardID → actor
	// string so logActivity can attribute edits correctly.
	llmActors sync.Map

	// Services — exposed via reflection to the HTTP /rpc dispatcher.
	Search     *search.Service
	Notify     *notify.Service
	MCP        *mcpsvc.Service
	LLM        *llmsvc.Service
	Settings   *settings.Service
	Catalog    *catalog.Service
	Project    *projectsvc.Service
	Card       *card.Service
	Chat       *chatsvc.Service
	Agent      *agentsvc.Service
	Repository *reposvc.Service

	tools   *tools.Dispatcher
	prompts *prompts.Builder
	chatRT  *chatrt.Runtime
	agentRT *agentrt.Runtime
}

func buildHeadlessRuntime(repoPath, configDir string) (*headlessRuntime, error) {
	ctx, cancel := context.WithCancel(context.Background())
	r := &headlessRuntime{
		ctx:       ctx,
		cancelCtx: cancel,
		bus:       events.NewMemBus(128),
	}

	if reg, err := schema.NewRegistry(); err == nil {
		r.registry = reg
	} else {
		slog.Warn("schema registry load failed", "err", err)
		r.registry = &schema.Registry{}
	}

	repoObj, err := repo.Open(repoPath)
	if err != nil {
		return nil, fmt.Errorf("open repo: %w", err)
	}
	r.repo = repoObj

	if err := repo.EnsureSyncHygiene(repoObj.Root); err != nil {
		slog.Warn("ensure sync hygiene failed", "err", err)
	}
	runsDir := filepath.Join(configDir, "runs", repoObj.Manifest.ID)
	if err := repoObj.SetRunsDir(runsDir); err != nil {
		slog.Warn("set runs dir failed", "err", err)
	}

	idxPath := filepath.Join(repoObj.Root, ".bruv", "index.db")
	if idx, err := index.Open(idxPath); err == nil {
		r.idx = idx
		if _, err := idx.IncrementalRefresh(repoObj.Root); err != nil {
			slog.Warn("index refresh failed", "err", err)
		}
	} else {
		slog.Warn("index open failed", "err", err)
	}

	r.Search = search.New(searchDeps{r})
	r.Notify = notify.New()
	r.MCP = mcpsvc.New(mcpDeps{r})
	r.LLM = llmsvc.New(llmDeps{r})
	r.Settings = settings.New()
	r.Catalog = catalog.New(catalogDeps{r})
	r.Project = projectsvc.New(projectDeps{r})
	r.Card = card.New(cardDeps{r})
	r.Chat = chatsvc.New(chatDeps{r})
	r.Agent = agentsvc.New(agentDeps{r})
	r.Repository = reposvc.New(repoDeps{r})

	r.tools = tools.New(toolsRTDeps{r})
	r.prompts = prompts.New(promptsRTDeps{r})
	r.chatRT = chatrt.New(chatRTDeps{r})
	r.agentRT = agentrt.New(agentRTDeps{r})

	if w, err := reposync.Start(repoObj.Root, r.bus); err == nil {
		r.watcher = w
	} else {
		slog.Warn("repo watcher start failed", "err", err)
	}

	r.reloadMCPRegistry()
	r.agentRT.StartScheduler()
	r.agentRT.StartDueDateScanner()

	return r, nil
}

// Agent control surface — thin forwarders so the JSON-RPC reflection
// dispatcher exposes the same method names the desktop Wails binding
// does.

func (r *headlessRuntime) CancelAgent(cardID string) error  { return r.agentRT.CancelAgent(cardID) }
func (r *headlessRuntime) TriggerAgent(cardID string) error { return r.agentRT.TriggerAgent(cardID) }
func (r *headlessRuntime) PauseAllAgents() error            { return r.agentRT.PauseAllAgents() }
func (r *headlessRuntime) ResumeAllAgents() error           { return r.agentRT.ResumeAllAgents() }
func (r *headlessRuntime) GetAgentSchedulerStatus() map[string]any {
	return r.agentRT.GetAgentSchedulerStatus()
}
func (r *headlessRuntime) GetAllAgents() ([]map[string]any, error) {
	return r.agentRT.GetAllAgents()
}
func (r *headlessRuntime) GetAllAgentRuns(limit int) ([]map[string]any, error) {
	return r.agentRT.GetAllAgentRuns(limit)
}
func (r *headlessRuntime) GetAgentAnalytics() (map[string]any, error) {
	return r.agentRT.GetAgentAnalytics()
}

// resolveAttachment is the bridge from the transport HTTP handler
// (which doesn't know about internal/repo) into the open repository.
func (r *headlessRuntime) resolveAttachment(cardID, attachmentID string) (path, mime, name string, ok bool) {
	if r.repo == nil {
		return "", "", "", false
	}
	att, err := r.repo.FindAttachment(cardID, attachmentID)
	if err != nil || att == nil {
		return "", "", "", false
	}
	return r.repo.AttachmentPath(cardID, attachmentID), att.Mime, att.Name, true
}

func (r *headlessRuntime) Close() {
	if r.agentRT != nil {
		r.agentRT.StopScheduler()
		r.agentRT.StopDueDateScanner()
	}
	if r.watcher != nil {
		r.watcher.Stop()
	}
	if r.mcpRegistry != nil {
		r.mcpRegistry.Shutdown()
	}
	if r.idx != nil {
		_ = r.idx.Close()
	}
	r.cancelCtx()
}

// reloadMCPRegistry rebuilds the MCP subprocess registry. Called from
// the mcp service when configuration mutates.
func (r *headlessRuntime) reloadMCPRegistry() {
	if r.mcpRegistry != nil {
		r.mcpRegistry.Shutdown()
		r.mcpRegistry = nil
	}
	if r.repo == nil {
		return
	}
	store, err := r.repo.LoadMCPServerStore()
	if err != nil {
		slog.Warn("mcp load failed", "err", err)
		return
	}
	reg := mcp.NewRegistry(r.repo.Manifest.ID, config.MCPSecretResolver{})
	startCtx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	errs := reg.LoadAndStart(startCtx, store.Servers)
	for name, err := range errs {
		slog.Warn("mcp server startup failed", "server", name, "err", err)
	}
	r.mcpRegistry = reg
}

// ---------------------------------------------------------------------
// Deps adapters. One per service so `Registry()` can return both
// *schema.Registry (catalog) and *mcp.Registry (mcpsvc) without
// clashing. All adapters close over the single *headlessRuntime.
// ---------------------------------------------------------------------

type searchDeps struct{ r *headlessRuntime }

func (d searchDeps) Repo() *repo.Repository { return d.r.repo }
func (d searchDeps) Index() *index.Index    { return d.r.idx }

type mcpDeps struct{ r *headlessRuntime }

func (d mcpDeps) Repo() *repo.Repository  { return d.r.repo }
func (d mcpDeps) Registry() *mcp.Registry { return d.r.mcpRegistry }
func (d mcpDeps) ReloadRegistry()         { d.r.reloadMCPRegistry() }

type llmDeps struct{ r *headlessRuntime }

func (d llmDeps) Ctx() context.Context { return d.r.ctx }

type catalogDeps struct{ r *headlessRuntime }

func (d catalogDeps) Repo() *repo.Repository     { return d.r.repo }
func (d catalogDeps) Registry() *schema.Registry { return d.r.registry }
func (d catalogDeps) Index() *index.Index        { return d.r.idx }
func (d catalogDeps) UpdateCardBlocks(id string, blocks []model.Block) (*model.Card, error) {
	return d.r.Card.UpdateBlocks(id, blocks)
}
func (d catalogDeps) Publish(topic string, payload any) { d.r.bus.Publish(topic, payload) }

type projectDeps struct{ r *headlessRuntime }

func (d projectDeps) Repo() *repo.Repository      { return d.r.repo }
func (d projectDeps) Index() *index.Index         { return d.r.idx }
func (d projectDeps) Publish(topic string, p any) { d.r.bus.Publish(topic, p) }

type cardDeps struct{ r *headlessRuntime }

func (d cardDeps) Repo() *repo.Repository { return d.r.repo }
func (d cardDeps) Index() *index.Index    { return d.r.idx }
func (d cardDeps) ApplyTypeBlocks(cardID, cardType string) {
	d.r.Catalog.ApplyTypeBlocks(cardID, cardType)
}

// LogActivity on headless is a no-op stub. Desktop resolves
// user-vs-LLM actor context via llmActors; headless doesn't yet
// have the llmActors loop mirrored here.
func (d cardDeps) LogActivity(cardID, action, field string) {}
func (d cardDeps) LogActivityWithContext(cardID, action, field, cardTitle string, breadcrumbs []card.CategoryPath) {
}
func (d cardDeps) Publish(topic string, payload any) { d.r.bus.Publish(topic, payload) }

type chatDeps struct{ r *headlessRuntime }

func (d chatDeps) Repo() *repo.Repository { return d.r.repo }

type agentDeps struct{ r *headlessRuntime }

func (d agentDeps) Repo() *repo.Repository { return d.r.repo }
func (d agentDeps) Index() *index.Index    { return d.r.idx }

type repoDeps struct{ r *headlessRuntime }

func (d repoDeps) Repo() *repo.Repository { return d.r.repo }
func (d repoDeps) Index() *index.Index    { return d.r.idx }

// ---------------------------------------------------------------------
// LLM runtime Deps adapters. "RT" suffix to avoid colliding with the
// service-layer adapters above.
// ---------------------------------------------------------------------

type toolsRTDeps struct{ r *headlessRuntime }

func (d toolsRTDeps) Repo() *repo.Repository            { return d.r.repo }
func (d toolsRTDeps) Registry() *schema.Registry        { return d.r.registry }
func (d toolsRTDeps) Publish(topic string, payload any) { d.r.bus.Publish(topic, payload) }
func (d toolsRTDeps) Card() *card.Service               { return d.r.Card }
func (d toolsRTDeps) Project() *projectsvc.Service      { return d.r.Project }
func (d toolsRTDeps) Catalog() *catalog.Service         { return d.r.Catalog }

type promptsRTDeps struct{ r *headlessRuntime }

func (d promptsRTDeps) Repo() *repo.Repository     { return d.r.repo }
func (d promptsRTDeps) Registry() *schema.Registry { return d.r.registry }
func (d promptsRTDeps) Card() *card.Service        { return d.r.Card }
func (d promptsRTDeps) Search() *search.Service    { return d.r.Search }

type chatRTDeps struct{ r *headlessRuntime }

func (d chatRTDeps) Repo() *repo.Repository     { return d.r.repo }
func (d chatRTDeps) Registry() *schema.Registry { return d.r.registry }
func (d chatRTDeps) Ctx() context.Context       { return d.r.ctx }
func (d chatRTDeps) LLM() *llmsvc.Service       { return d.r.LLM }
func (d chatRTDeps) Card() *card.Service        { return d.r.Card }
func (d chatRTDeps) Tools() *tools.Dispatcher   { return d.r.tools }
func (d chatRTDeps) Prompts() *prompts.Builder  { return d.r.prompts }
func (d chatRTDeps) MCPRegistry() *mcp.Registry { return d.r.mcpRegistry }
func (d chatRTDeps) LLMActors() *sync.Map       { return &d.r.llmActors }

type agentRTDeps struct{ r *headlessRuntime }

func (d agentRTDeps) Repo() *repo.Repository            { return d.r.repo }
func (d agentRTDeps) Index() *index.Index               { return d.r.idx }
func (d agentRTDeps) Registry() *schema.Registry        { return d.r.registry }
func (d agentRTDeps) Ctx() context.Context              { return d.r.ctx }
func (d agentRTDeps) Publish(topic string, payload any) { d.r.bus.Publish(topic, payload) }
func (d agentRTDeps) LLM() *llmsvc.Service              { return d.r.LLM }
func (d agentRTDeps) Card() *card.Service               { return d.r.Card }
func (d agentRTDeps) Project() *projectsvc.Service      { return d.r.Project }
func (d agentRTDeps) Catalog() *catalog.Service         { return d.r.Catalog }
func (d agentRTDeps) Prompts() *prompts.Builder         { return d.r.prompts }
func (d agentRTDeps) ChatRT() *chatrt.Runtime           { return d.r.chatRT }
func (d agentRTDeps) MCPRegistry() *mcp.Registry        { return d.r.mcpRegistry }
func (d agentRTDeps) LLMActors() *sync.Map              { return &d.r.llmActors }
