// Command bruv-server runs the BRUV HTTP transport without a Wails
// shell — the standalone server binary for Mode A/B deployments. Bind
// to 0.0.0.0:<port> behind Tailscale and the same frontend that
// speaks to the desktop loopback server Just Works against this one.
//
// This binary is intentionally minimal right now. It proves:
//
//   - The core packages (events, services, transport) compile and run
//     with zero Wails dependencies.
//   - A repo can be opened, indexed, and served over HTTP + SSE.
//   - The device-token auth flow bootstraps correctly headless.
//
// What it does NOT yet carry:
//
//   - The LLM runtime (agent scheduler + chat loop + tool dispatch).
//     That's entangled with the Wails event emitter in app_agent.go /
//     app_chat.go / app_tools.go today and awaits the "LLM runtime
//     extraction" pass. Agent config CRUD works; agent *execution*
//     does not.
//   - System tray, native notifications, native file dialogs. These
//     are intentionally absent on a headless server.
//
// Flags:
//
//	--repo      Path to the BRUV repo to open (required).
//	--addr      HTTP listen address. Default: 127.0.0.1:9870.
//	            Set to 0.0.0.0:<port> for tailnet-exposed deployment.
//	--config    Config directory. Default: <user-config>/bruv/.
//
// Signals: SIGINT / SIGTERM trigger graceful shutdown.
package main

import (
	"context"
	"flag"
	"fmt"
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

// Version is stamped at build time via -ldflags "-X main.Version=...".
var Version = "dev"

// BuildDate is stamped at build time via -ldflags "-X main.BuildDate=...".
var BuildDate = "unknown"

func main() {
	repoPath := flag.String("repo", "", "path to BRUV repo to open (required)")
	addr := flag.String("addr", "127.0.0.1:9870", "HTTP listen address")
	configDir := flag.String("config", "", "config directory (default: <user-config>/bruv/)")
	flag.Parse()

	if *repoPath == "" {
		fmt.Fprintln(os.Stderr, "error: --repo is required")
		flag.Usage()
		os.Exit(2)
	}

	slog.Info("bruv-server starting", "version", Version, "build_date", BuildDate)

	// Resolve config dir (same layout the desktop uses).
	if *configDir == "" {
		dir, err := config.ConfigDir()
		if err != nil {
			slog.Error("resolve config dir failed", "err", err)
			os.Exit(1)
		}
		*configDir = dir
	}

	rt, err := buildHeadlessRuntime(*repoPath, *configDir)
	if err != nil {
		slog.Error("runtime build failed", "err", err)
		os.Exit(1)
	}
	defer rt.Close()

	// Start HTTP transport bound to the user's chosen address.
	srv, err := transporthttp.New(transporthttp.Config{
		Addr:      *addr,
		ConfigDir: *configDir,
		Version:   Version,
		BuildDate: BuildDate,
		// No StaticAssets — headless server doesn't carry the bundle
		// by default. Add an embed later when wiring up Mode B for
		// cloud-only deployments that don't have a Wails shell to
		// serve the UI.
	}, rt, rt.bus)
	if err != nil {
		slog.Error("http transport construct failed", "err", err)
		os.Exit(1)
	}
	if err := srv.Start(); err != nil {
		slog.Error("http transport start failed", "err", err)
		os.Exit(1)
	}
	slog.Info("bruv-server listening", "addr", srv.Addr(),
		"bootstrap_token", filepath.Join(*configDir, "bootstrap-token.txt"))

	// Block on signal, then shut down cleanly.
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	sig := <-sigCh
	slog.Info("signal received, shutting down", "signal", sig)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_ = srv.Stop()
	_ = ctx // reserved for future graceful-shutdown chains
}

// ---------------------------------------------------------------------
// Headless runtime — holds the open repo + wired-up services and
// satisfies every service's Deps interface via method receivers on
// itself. Same shape as the desktop App but without Wails.
// ---------------------------------------------------------------------

type runtime struct {
	ctx       context.Context
	cancelCtx context.CancelFunc

	repo        *repo.Repository
	idx         *index.Index
	registry    *schema.Registry
	mcpRegistry *mcp.Registry
	bus         *events.MemBus
	watcher     *reposync.Watcher

	// llmActors tracks active chat/agent sessions by cardID → actor
	// string so logActivity can attribute edits correctly. Shared by
	// chatRT and agentRT, same as the desktop App.
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

	// LLM runtime — the tool dispatcher, prompt builder, chat loop,
	// and agent execution surface. Constructed after services because
	// they depend on them. See core/runtime/*.
	tools   *tools.Dispatcher
	prompts *prompts.Builder
	chatRT  *chatrt.Runtime
	agentRT *agentrt.Runtime
}

func buildHeadlessRuntime(repoPath, configDir string) (*runtime, error) {
	ctx, cancel := context.WithCancel(context.Background())
	r := &runtime{
		ctx:       ctx,
		cancelCtx: cancel,
		bus:       events.NewMemBus(128),
	}

	// Load the card-type schema registry (reads built-in schemas; no
	// repo I/O). Falls back to an empty registry on failure.
	if reg, err := schema.NewRegistry(); err == nil {
		r.registry = reg
	} else {
		slog.Warn("schema registry load failed", "err", err)
		r.registry = &schema.Registry{}
	}

	// Open the repo.
	repoObj, err := repo.Open(repoPath)
	if err != nil {
		return nil, fmt.Errorf("open repo: %w", err)
	}
	r.repo = repoObj

	// Ensure sync hygiene + route runs to serverdata, same as desktop.
	if err := repo.EnsureSyncHygiene(repoObj.Root); err != nil {
		slog.Warn("ensure sync hygiene failed", "err", err)
	}
	runsDir := filepath.Join(configDir, "runs", repoObj.Manifest.ID)
	if err := repoObj.SetRunsDir(runsDir); err != nil {
		slog.Warn("set runs dir failed", "err", err)
	}

	// Open the SQLite index.
	idxPath := filepath.Join(repoObj.Root, ".bruv", "index.db")
	if idx, err := index.Open(idxPath); err == nil {
		r.idx = idx
		if _, err := idx.IncrementalRefresh(repoObj.Root); err != nil {
			slog.Warn("index refresh failed", "err", err)
		}
	} else {
		slog.Warn("index open failed", "err", err)
	}

	// Wire services. Each service takes its own Deps adapter so
	// method-name collisions across interfaces (e.g. catalog.Deps'
	// Registry() returns *schema.Registry while mcpsvc.Deps' returns
	// *mcp.Registry) stay resolved. Mirrors the desktop App pattern.
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

	// LLM runtime. Built after services because each runtime package
	// pulls the services it writes through via its own Deps adapter.
	// Agent runtime depends on chat runtime so order matters.
	r.tools = tools.New(toolsRTDeps{r})
	r.prompts = prompts.New(promptsRTDeps{r})
	r.chatRT = chatrt.New(chatRTDeps{r})
	r.agentRT = agentrt.New(agentRTDeps{r})

	// Start file watcher so external (sync-tool) changes publish
	// events identical to user-driven mutations.
	if w, err := reposync.Start(repoObj.Root, r.bus); err == nil {
		r.watcher = w
	} else {
		slog.Warn("repo watcher start failed", "err", err)
	}

	// Boot MCP registry for this repo before starting the scheduler —
	// agents expect MCP tools to be discoverable at execute time.
	r.reloadMCPRegistry()

	// Start the agent scheduler + due-date scanner so the headless
	// binary runs agents end-to-end, not just exposes config CRUD.
	r.agentRT.StartScheduler()
	r.agentRT.StartDueDateScanner()

	return r, nil
}

// ---------------------------------------------------------------------
// Agent control surface — thin forwarders so the JSON-RPC reflection
// dispatcher exposes the same method names the desktop Wails binding
// does. The runtime struct is the RPC target, so any exported method
// on it becomes a callable RPC method. Canonical implementations live
// on *agentrt.Runtime (see core/runtime/agent/control.go).
// ---------------------------------------------------------------------

func (r *runtime) CancelAgent(cardID string) error  { return r.agentRT.CancelAgent(cardID) }
func (r *runtime) TriggerAgent(cardID string) error { return r.agentRT.TriggerAgent(cardID) }
func (r *runtime) PauseAllAgents() error            { return r.agentRT.PauseAllAgents() }
func (r *runtime) ResumeAllAgents() error           { return r.agentRT.ResumeAllAgents() }
func (r *runtime) GetAgentSchedulerStatus() map[string]any {
	return r.agentRT.GetAgentSchedulerStatus()
}
func (r *runtime) GetAllAgents() ([]map[string]any, error) { return r.agentRT.GetAllAgents() }
func (r *runtime) GetAllAgentRuns(limit int) ([]map[string]any, error) {
	return r.agentRT.GetAllAgentRuns(limit)
}
func (r *runtime) GetAgentAnalytics() (map[string]any, error) { return r.agentRT.GetAgentAnalytics() }

func (r *runtime) Close() {
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

// ---------------------------------------------------------------------
// Deps adapters. One per service so `Registry()` can return both
// *schema.Registry (for catalog) and *mcp.Registry (for mcpsvc)
// without clashing. All adapters close over the single *runtime so
// they share state.
// ---------------------------------------------------------------------

type searchDeps struct{ r *runtime }

func (d searchDeps) Repo() *repo.Repository { return d.r.repo }
func (d searchDeps) Index() *index.Index    { return d.r.idx }

type mcpDeps struct{ r *runtime }

func (d mcpDeps) Repo() *repo.Repository  { return d.r.repo }
func (d mcpDeps) Registry() *mcp.Registry { return d.r.mcpRegistry }
func (d mcpDeps) ReloadRegistry()         { d.r.reloadMCPRegistry() }

type llmDeps struct{ r *runtime }

func (d llmDeps) Ctx() context.Context { return d.r.ctx }

type catalogDeps struct{ r *runtime }

func (d catalogDeps) Repo() *repo.Repository     { return d.r.repo }
func (d catalogDeps) Registry() *schema.Registry { return d.r.registry }
func (d catalogDeps) Index() *index.Index        { return d.r.idx }
func (d catalogDeps) UpdateCardBlocks(id string, blocks []model.Block) (*model.Card, error) {
	return d.r.Card.UpdateBlocks(id, blocks)
}
func (d catalogDeps) Publish(topic string, payload any) { d.r.bus.Publish(topic, payload) }

type projectDeps struct{ r *runtime }

func (d projectDeps) Repo() *repo.Repository         { return d.r.repo }
func (d projectDeps) Index() *index.Index            { return d.r.idx }
func (d projectDeps) Publish(topic string, p any)    { d.r.bus.Publish(topic, p) }

type cardDeps struct{ r *runtime }

func (d cardDeps) Repo() *repo.Repository { return d.r.repo }
func (d cardDeps) Index() *index.Index    { return d.r.idx }
func (d cardDeps) ApplyTypeBlocks(cardID, cardType string) {
	d.r.Catalog.ApplyTypeBlocks(cardID, cardType)
}

// LogActivity on headless is a no-op stub. Desktop resolves
// user-vs-LLM actor context via llmActors; headless doesn't have
// that loop yet (arrives with the LLM-runtime extraction).
func (d cardDeps) LogActivity(cardID, action, field string) {}
func (d cardDeps) LogActivityWithContext(cardID, action, field, cardTitle string, breadcrumbs []card.CategoryPath) {
}
func (d cardDeps) Publish(topic string, payload any) { d.r.bus.Publish(topic, payload) }

type chatDeps struct{ r *runtime }

func (d chatDeps) Repo() *repo.Repository { return d.r.repo }

type agentDeps struct{ r *runtime }

func (d agentDeps) Repo() *repo.Repository { return d.r.repo }
func (d agentDeps) Index() *index.Index    { return d.r.idx }

type repoDeps struct{ r *runtime }

func (d repoDeps) Repo() *repo.Repository { return d.r.repo }
func (d repoDeps) Index() *index.Index    { return d.r.idx }

// ---------------------------------------------------------------------
// LLM runtime Deps adapters. Named with the "RT" suffix to avoid
// colliding with the service-layer adapters above (chatDeps/agentDeps
// already belong to the CRUD services). Each matches the respective
// Deps interface in core/runtime/<pkg>.
// ---------------------------------------------------------------------

type toolsRTDeps struct{ r *runtime }

func (d toolsRTDeps) Repo() *repo.Repository            { return d.r.repo }
func (d toolsRTDeps) Registry() *schema.Registry        { return d.r.registry }
func (d toolsRTDeps) Publish(topic string, payload any) { d.r.bus.Publish(topic, payload) }
func (d toolsRTDeps) Card() *card.Service               { return d.r.Card }
func (d toolsRTDeps) Project() *projectsvc.Service      { return d.r.Project }
func (d toolsRTDeps) Catalog() *catalog.Service         { return d.r.Catalog }

type promptsRTDeps struct{ r *runtime }

func (d promptsRTDeps) Repo() *repo.Repository     { return d.r.repo }
func (d promptsRTDeps) Registry() *schema.Registry { return d.r.registry }
func (d promptsRTDeps) Card() *card.Service        { return d.r.Card }
func (d promptsRTDeps) Search() *search.Service    { return d.r.Search }

type chatRTDeps struct{ r *runtime }

func (d chatRTDeps) Repo() *repo.Repository     { return d.r.repo }
func (d chatRTDeps) Registry() *schema.Registry { return d.r.registry }
func (d chatRTDeps) Ctx() context.Context       { return d.r.ctx }
func (d chatRTDeps) LLM() *llmsvc.Service       { return d.r.LLM }
func (d chatRTDeps) Card() *card.Service        { return d.r.Card }
func (d chatRTDeps) Tools() *tools.Dispatcher   { return d.r.tools }
func (d chatRTDeps) Prompts() *prompts.Builder  { return d.r.prompts }
func (d chatRTDeps) MCPRegistry() *mcp.Registry { return d.r.mcpRegistry }
func (d chatRTDeps) LLMActors() *sync.Map       { return &d.r.llmActors }

type agentRTDeps struct{ r *runtime }

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

// reloadMCPRegistry rebuilds the MCP subprocess registry. Called from
// the mcp service when configuration mutates.
func (r *runtime) reloadMCPRegistry() {
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
