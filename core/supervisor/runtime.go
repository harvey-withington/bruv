// Package supervisor owns the per-repo Runtime + the multi-repo
// Supervisor that hosts them. Same code path the desktop App and the
// headless server both use — what used to be `headlessRuntime` in
// internal/server now lives here so both sides can share it.
//
// The runtime holds an open repository plus every wired-up service
// and LLM-runtime piece. Dispatched against by the JSON-RPC reflection
// dispatcher in transport/http (via the per-repo Resolve path) — the
// methods on *Runtime ARE the per-repo RPC surface.
//
// This package does NOT depend on transport/http; the wiring layer
// (internal/server for the headless binary, app.go for the desktop
// shell) constructs an adapter that satisfies transport's RepoBackend
// interface from a *Supervisor. Keeps core/ free of HTTP concerns.
package supervisor

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"sync/atomic"
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
	workspacesvc "bruv/core/services/workspace"
	"bruv/internal/config"
	"bruv/internal/fsutil"
	"bruv/internal/index"
	"bruv/internal/llm"
	"bruv/internal/logging"
	"bruv/internal/mcp"
	"bruv/internal/model"
	"bruv/internal/repo"
	"bruv/internal/schema"

	"github.com/google/uuid"
)

// Runtime is the per-repo bundle: open repo + services + LLM stack +
// supporting infra (event bus, file watcher, MCP registry). Satisfies
// every service's Deps interface via method receivers on itself.
//
// Methods that mutate or read repo state live on *Runtime — the JSON-RPC
// dispatcher reflects on this type. See runtime_methods.go for the full
// per-repo surface.
type Runtime struct {
	ctx       context.Context
	cancelCtx context.CancelFunc

	repo     *repo.Repository
	idx      *index.Index
	registry *schema.Registry

	// releaseRepoLock drops this process's claim on the repo folder
	// (.bruv/instance.lock) — the last thing close() does.
	releaseRepoLock func()

	// completingClips holds the card IDs a CompleteCapture is ingesting
	// right now, so a second browser completing the same pending clip
	// is refused instead of ingesting its media twice.
	completingClips sync.Map
	bus             *events.MemBus
	watcher         *reposync.Watcher

	// mcpRegistry is swapped atomically on reload: readers always see
	// either the old or the new registry, never nil while one exists.
	// mcpMu serialises reloads with each other and with Close, so a
	// reload can't install a registry after Close shut the last one
	// down (and concurrent reloads can't leak a set of subprocesses).
	mcpRegistry atomic.Pointer[mcp.Registry]
	mcpMu       sync.Mutex
	mcpClosed   bool

	closeOnce sync.Once

	// bg tracks fire-and-forget background writes (see goBackground)
	// so Close can wait for them; bgClosing refuses new ones.
	bgMu      sync.Mutex
	bgClosing bool
	bg        sync.WaitGroup

	// secret is the host's HMAC key for signed-attachment-URL minting.
	// Passed in from the supervisor at build time (one secret per
	// host machine; shared across all runtimes hosted by that machine).
	// Used only by SignAttachmentURL.
	secret []byte

	// llmActors tracks active chat/agent sessions by cardID → actor
	// string so logActivity can attribute edits correctly.
	llmActors sync.Map

	// liveState holds transient per-block runtime state (e.g. a slide
	// deck's live presentation position), keyed "cardID/blockID".
	// In-memory only — never persisted, lost on restart by design.
	// See livestate.go.
	liveStateMu sync.RWMutex
	liveState   map[string]map[string]any

	// presentGate tracks which cards are actively presenting (explicit
	// start/stop from the Present button). See presentwatch.go.
	presentMu   sync.Mutex
	presentGate map[string]bool

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
	Workspace  *workspacesvc.Service

	tools   *tools.Dispatcher
	prompts *prompts.Builder
	chatRT  *chatrt.Runtime
	agentRT *agentrt.Runtime
}

// buildRuntime opens repoPath, wires every service, and starts the
// background workers (file watcher, MCP subprocesses, agent scheduler).
// Returns a fully-armed *Runtime ready to accept RPCs. Caller must
// Close() to release the lock + index + watcher when done.
func buildRuntime(repoPath, configDir string, secret []byte) (*Runtime, error) {
	ctx, cancel := context.WithCancel(context.Background())
	r := &Runtime{
		ctx:       ctx,
		cancelCtx: cancel,
		bus:       events.NewMemBus(128),
		secret:    secret,
	}

	if reg, err := schema.NewRegistry(); err == nil {
		r.registry = reg
	} else {
		slog.Warn("schema registry load failed", "err", err)
		r.registry = &schema.Registry{}
	}

	repoObj, err := repo.Open(repoPath)
	if err != nil {
		cancel()
		return nil, fmt.Errorf("open repo: %w", err)
	}
	// One BRUV process per repo folder: a second one (the desktop opening
	// the folder the local BRUV Server already serves, say) would run a
	// second agent scheduler and alarm scanner and race the first on
	// every file — the in-process write locks can't see another process.
	release, err := fsutil.TryLock(filepath.Join(repoObj.Root, ".bruv", "instance.lock"), instanceHolder())
	if err != nil {
		cancel()
		var held *fsutil.LockHeldError
		if errors.As(err, &held) {
			return nil, fmt.Errorf("this repository is already open in another BRUV process on this computer (%s) — connect to that BRUV Server instead of opening the folder directly", held.Holder)
		}
		return nil, fmt.Errorf("lock repo: %w", err)
	}
	r.releaseRepoLock = release
	r.repo = repoObj

	if err := repo.EnsureSyncHygiene(repoObj.Root); err != nil {
		slog.Warn("ensure sync hygiene failed", "err", err)
	}
	runsDir := filepath.Join(configDir, "runs", repoObj.Manifest.ID)
	if err := repoObj.SetRunsDir(runsDir); err != nil {
		slog.Warn("set runs dir failed", "err", err)
	}

	// Repair load-time damage (duplicate pins from the pre-2026-09-17
	// move bug, stale pins) before the index is built from disk. It is
	// non-destructive under partial failure: an unreadable level skips
	// the stale-pin pass, and orphans are only reported.
	if stats, err := repoObj.Revalidate(); err != nil {
		slog.Warn("revalidation failed", "err", err)
	} else {
		slog.Info("revalidate ok", "stats", stats.String())
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
	r.Workspace = workspacesvc.New(workspaceDeps{r})
	// Built-in Folder Templates land in the vault once; a failure here
	// only means the picker lacks them, so it must never block the load.
	if seeded, err := r.Workspace.SeedBuiltinTemplates(); err != nil {
		slog.Warn("seed built-in templates failed", "err", err)
	} else if len(seeded) > 0 {
		slog.Info("seeded built-in templates", "names", seeded)
	}

	r.tools = tools.New(toolsRTDeps{r})
	r.prompts = prompts.New(promptsRTDeps{r})
	r.chatRT = chatrt.New(chatRTDeps{r})
	r.agentRT = agentrt.New(agentRTDeps{r})

	if w, err := reposync.Start(repoObj.Root, r.bus); err == nil {
		r.watcher = w
		// Wire the watcher into Repository's pre-rename / pre-delete
		// hook so directory mutations on Windows don't trip over the
		// fsnotify pending-IRP / ACCESS_DENIED issue. See
		// reposync.Watcher doc for the full reason. Cleanup re-attaches
		// the parent so children moved/created during the op are
		// re-watched fresh — covers both renames (target moved within
		// parent) and deletes (target gone, but parent's other children
		// still need watching).
		watcher := w
		repoObj.BeforeDirOp = func(target string) func() {
			watcher.DetachSubtree(target)
			parent := filepath.Dir(target)
			return func() { watcher.AttachSubtree(parent) }
		}
	} else {
		slog.Warn("repo watcher start failed", "err", err)
	}

	r.reloadMCPRegistry()
	r.agentRT.StartScheduler()
	r.agentRT.StartDueDateScanner()

	return r, nil
}

// Bus exposes the runtime's event bus to wiring code (transport adapter
// uses this for SSE).
func (r *Runtime) Bus() *events.MemBus { return r.bus }

// Accessors used by the desktop App to mirror per-repo state without
// owning it. Server-side callers don't need these — they hold the
// *Runtime directly and reach in through the public service fields.
// Kept as methods so the desktop's existing field-based call sites
// (a.repo, a.idx, a.cardService, ...) can be populated from a single
// `app.bindToRuntime(rt)` step.
func (r *Runtime) Repo() *repo.Repository           { return r.repo }
func (r *Runtime) Index() *index.Index              { return r.idx }
func (r *Runtime) SchemaRegistry() *schema.Registry { return r.registry }
func (r *Runtime) MCPRegistry() *mcp.Registry       { return r.mcpRegistry.Load() }
func (r *Runtime) Watcher() *reposync.Watcher       { return r.watcher }
func (r *Runtime) LLMActors() *sync.Map             { return &r.llmActors }
func (r *Runtime) Tools() *tools.Dispatcher         { return r.tools }
func (r *Runtime) Prompts() *prompts.Builder        { return r.prompts }
func (r *Runtime) ChatRT() *chatrt.Runtime          { return r.chatRT }
func (r *Runtime) AgentRT() *agentrt.Runtime        { return r.agentRT }

// Service accessors satisfy boardtools.Board, whose interface can't
// reach the exported fields directly.
func (r *Runtime) CardService() *card.Service          { return r.Card }
func (r *Runtime) ProjectService() *projectsvc.Service { return r.Project }
func (r *Runtime) CatalogService() *catalog.Service    { return r.Catalog }
func (r *Runtime) AgentService() *agentsvc.Service     { return r.Agent }

// Agent control surface — thin forwarders so the JSON-RPC reflection
// dispatcher exposes the same method names the desktop binding does.

func (r *Runtime) CancelAgent(cardID string) error  { return r.agentRT.CancelAgent(cardID) }
func (r *Runtime) TriggerAgent(cardID string) error { return r.agentRT.TriggerAgent(cardID) }
func (r *Runtime) PauseAllAgents() error            { return r.agentRT.PauseAllAgents() }
func (r *Runtime) ResumeAllAgents() error           { return r.agentRT.ResumeAllAgents() }
func (r *Runtime) GetAgentSchedulerStatus() map[string]any {
	return r.agentRT.GetAgentSchedulerStatus()
}
func (r *Runtime) GetAllAgents() ([]map[string]any, error) {
	return r.agentRT.GetAllAgents()
}
func (r *Runtime) GetAllAgentRuns(limit int) ([]map[string]any, error) {
	return r.agentRT.GetAllAgentRuns(limit)
}
func (r *Runtime) GetAgentAnalytics() (map[string]any, error) {
	return r.agentRT.GetAgentAnalytics()
}

// SignAttachmentURL mints a short-lived signed URL the client can drop
// straight into <img src> / <a href>. The 5-minute TTL bounds how long
// a leaked URL stays usable; download URLs are renewed on every page
// load anyway, so a short lifetime is invisible to users.
//
// Returns a server-relative path including this repo's /repos/<id>/
// prefix, so the client (cloud adapter) just has to prepend its
// scheme://host base. The HMAC signing keeps the secret server-side.
func (r *Runtime) SignAttachmentURL(cardID, attachmentID string) (string, error) {
	if cardID == "" || attachmentID == "" {
		return "", fmt.Errorf("cardID and attachmentID are required")
	}
	if r.repo == nil || r.repo.Manifest == nil {
		return "", fmt.Errorf("repo not loaded")
	}
	const ttl = 5 * time.Minute
	exp := time.Now().Add(ttl).Unix()
	mac := hmac.New(sha256.New, r.secret)
	fmt.Fprintf(mac, "%s|%s|%d", cardID, attachmentID, exp)
	sig := mac.Sum(nil)
	return fmt.Sprintf("/repos/%s/attachments/%s/%s?exp=%d&sig=%s",
		r.repo.Manifest.ID, cardID, attachmentID, exp, hex.EncodeToString(sig)), nil
}

// ResolveAttachment is the bridge from the transport HTTP handler
// (which doesn't know about internal/repo) into the open repository.
// Exported so the wiring layer can plug it into AttachmentConfig.
func (r *Runtime) ResolveAttachment(cardID, attachmentID string) (path, mime, name string, ok bool) {
	if r.repo == nil {
		return "", "", "", false
	}
	att, err := r.repo.FindAttachment(cardID, attachmentID)
	if err != nil || att == nil {
		return "", "", "", false
	}
	mime = att.Mime
	// Mime is frozen at upload from the extension table of that day;
	// attachments added before the table learned .md/.csv/.yaml are stuck
	// at octet-stream in the card JSON. Re-derive at serve time so they
	// render as what they are.
	if mime == "" || mime == "application/octet-stream" {
		if detected := repo.DetectMime(att.Name); detected != "application/octet-stream" {
			mime = detected
		}
	}
	return r.repo.AttachmentPath(cardID, attachmentID), mime, att.Name, true
}

// CurrentRepoInfo is the JSON-stable view of "which repo is this".
// Mirrors the App's same-named struct so the dispatcher returns the
// same shape regardless of which backend the client is talking to.
type CurrentRepoInfo struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Path        string `json:"path"`
	Description string `json:"description"`
}

// GetCurrentRepo reports the repo this runtime hosts. The client uses
// this at boot to skip the local-only "open a repo" welcome flow when
// connected to a remote — the server's repo is fixed and always open.
func (r *Runtime) GetCurrentRepo() *CurrentRepoInfo {
	if r.repo == nil || r.repo.Manifest == nil {
		return nil
	}
	return &CurrentRepoInfo{
		ID:          r.repo.Manifest.ID,
		Name:        r.repo.Manifest.Name,
		Path:        r.repo.Root,
		Description: r.repo.Manifest.Description,
	}
}

// Close stops the runtime's background workers and releases resources.
// Idempotent. Safe to call from a defer.
//
// Order matters: the runtime context is cancelled FIRST so in-flight
// LLM runs (5-minute timeouts) and MCP start-ups abort instead of
// holding up disable / unload / quit; the scheduler then waits for
// those runs to record their cancelled state while the repo and index
// are still open. The bus closes last, after the final events, which
// ends every SSE stream bound to this runtime so clients reconnect to
// its replacement.
func (r *Runtime) Close() {
	r.closeOnce.Do(r.close)
}

func (r *Runtime) close() {
	r.cancelCtx()
	if r.agentRT != nil {
		// Stop via the handles rather than StopScheduler /
		// StopDueDateScanner, which clear the pointers that
		// TriggerAgent reads concurrently. A stopped scheduler
		// refuses new triggers.
		if s := r.agentRT.Scheduler(); s != nil {
			s.Stop()
		}
		if d := r.agentRT.DueDateScanner(); d != nil {
			d.Stop()
		}
	}
	if r.watcher != nil {
		r.watcher.Stop()
	}
	// After the runs have finished (their final activity entries are
	// already queued), refuse new background writes and drain the rest.
	r.bgMu.Lock()
	r.bgClosing = true
	r.bgMu.Unlock()
	r.bg.Wait()
	r.mcpMu.Lock()
	r.mcpClosed = true
	reg := r.mcpRegistry.Swap(nil)
	r.mcpMu.Unlock()
	if reg != nil {
		reg.Shutdown()
	}
	if r.idx != nil {
		_ = r.idx.Close()
	}
	r.bus.Close()
	if r.releaseRepoLock != nil {
		r.releaseRepoLock()
	}
}

// instanceHolder describes this process for the repo lock file, so a
// second process can say who has the folder open.
func instanceHolder() string {
	role := "BRUV desktop"
	if os.Getenv("BRUV_MODE") == "server" || strings.Contains(filepath.Base(os.Args[0]), "server") || slices.Contains(os.Args[1:], "--server") {
		role = "BRUV Server"
	}
	return fmt.Sprintf("%s, pid %d, since %s", role, os.Getpid(), time.Now().Format("2006-01-02 15:04"))
}

// reloadMCPRegistry rebuilds the MCP subprocess registry. Called from
// the mcp service when configuration mutates. The new registry is
// fully started before it replaces the old one, so tool dispatch
// never sees a nil registry mid-reload; the old one is shut down
// after the swap. If the config can't be read the current registry
// stays in place.
func (r *Runtime) reloadMCPRegistry() {
	if r.repo == nil {
		return
	}
	r.mcpMu.Lock()
	defer r.mcpMu.Unlock()
	if r.mcpClosed {
		return
	}
	store, err := r.repo.LoadMCPServerStore()
	if err != nil {
		slog.Warn("mcp load failed", "err", err)
		return
	}
	reg := mcp.NewRegistry(r.repo.Manifest.ID, config.MCPSecretResolver{})
	// Derived from the runtime context so Close aborts a slow start-up
	// instead of waiting out the full timeout.
	startCtx, cancel := context.WithTimeout(r.ctx, 60*time.Second)
	defer cancel()
	// Only servers this machine approved run: the repo file's "enabled"
	// flag travels with shared repos, so it can't be trusted alone.
	approved := mcpApprovalFunc(r.repo.Manifest.ID, store.Servers)
	errs := reg.LoadAndStart(startCtx, store.Servers, approved)
	for name, err := range errs {
		slog.Warn("mcp server startup failed", "server", name, "err", err)
	}
	if old := r.mcpRegistry.Swap(reg); old != nil {
		old.Shutdown()
	}
}

// logActivity / logActivityWithContext capture a card mutation in the
// activity feed, attributing the actor through llmActors when chat/agent
// initiated, otherwise through the user profile + device ID.
func (r *Runtime) logActivity(cardID, action, field string) {
	if r.repo == nil {
		return
	}
	cardTitle := ""
	if c, err := r.repo.GetCard(cardID); err == nil {
		cardTitle = c.Title
	}
	breadcrumbs, _ := r.GetCardPinBreadcrumbs(cardID)
	r.logActivityWithContext(cardID, action, field, cardTitle, breadcrumbs)
}

func (r *Runtime) logActivityWithContext(cardID, action, field, cardTitle string, breadcrumbs []card.CategoryPath) {
	if r.repo == nil {
		return
	}
	r.goBackground(func() {
		defer logging.Recover("supervisor.logActivityWithContext")
		var actorID, actor, actorType string
		if v, ok := r.llmActors.Load(cardID); ok {
			actor = v.(string)
			actorID = actor
			actorType = "llm"
		} else {
			p, _ := config.LoadProfile()
			actor = p.DisplayName
			if actor == "" {
				actor = "User"
			}
			actorID = config.LoadDeviceID()
			actorType = "user"
		}

		entry := model.ActivityEntry{
			ID:        uuid.New().String(),
			Timestamp: time.Now().UTC(),
			ActorID:   actorID,
			Actor:     actor,
			ActorType: actorType,
			Action:    action,
			Field:     field,
			CardID:    cardID,
			CardTitle: cardTitle,
		}

		if len(breadcrumbs) > 0 {
			bc := breadcrumbs[0]
			entry.BrandSlug = bc.BrandSlug
			entry.StreamSlug = bc.StreamSlug
			entry.ProjectSlug = bc.ProjectSlug
			entry.BrandName = bc.BrandName
			entry.StreamName = bc.StreamName
			entry.ProjectName = bc.ProjectName
			entry.CategoryName = bc.CategoryName
		}

		r.repo.AppendActivity(entry)
	})
}

// goBackground runs fn on a goroutine that Close waits for, so
// fire-and-forget repo writes (activity log) never land after the
// runtime has closed. Dropped once Close has begun.
func (r *Runtime) goBackground(fn func()) {
	r.bgMu.Lock()
	if r.bgClosing {
		r.bgMu.Unlock()
		return
	}
	r.bg.Add(1)
	r.bgMu.Unlock()
	go func() {
		defer r.bg.Done()
		fn()
	}()
}

// ---------------------------------------------------------------------
// Deps adapters. One per service so `Registry()` can return both
// *schema.Registry (catalog) and *mcp.Registry (mcpsvc) without
// clashing. All adapters close over the single *Runtime.
// ---------------------------------------------------------------------

type searchDeps struct{ r *Runtime }

func (d searchDeps) Repo() *repo.Repository { return d.r.repo }
func (d searchDeps) Index() *index.Index    { return d.r.idx }

type mcpDeps struct{ r *Runtime }

func (d mcpDeps) Repo() *repo.Repository  { return d.r.repo }
func (d mcpDeps) Registry() *mcp.Registry { return d.r.MCPRegistry() }
func (d mcpDeps) ReloadRegistry()         { d.r.reloadMCPRegistry() }

type llmDeps struct{ r *Runtime }

func (d llmDeps) Ctx() context.Context { return d.r.ctx }

type catalogDeps struct{ r *Runtime }

func (d catalogDeps) Repo() *repo.Repository            { return d.r.repo }
func (d catalogDeps) Registry() *schema.Registry        { return d.r.registry }
func (d catalogDeps) Index() *index.Index               { return d.r.idx }
func (d catalogDeps) Publish(topic string, payload any) { d.r.bus.Publish(topic, payload) }

type projectDeps struct{ r *Runtime }

func (d projectDeps) Repo() *repo.Repository      { return d.r.repo }
func (d projectDeps) Index() *index.Index         { return d.r.idx }
func (d projectDeps) Publish(topic string, p any) { d.r.bus.Publish(topic, p) }

type cardDeps struct{ r *Runtime }

func (d cardDeps) Repo() *repo.Repository { return d.r.repo }
func (d cardDeps) Index() *index.Index    { return d.r.idx }
func (d cardDeps) ApplyTypeBlocks(cardID, cardType string) error {
	return d.r.Catalog.ApplyTypeBlocks(cardID, cardType)
}
func (d cardDeps) CardTypeExists(cardType string) bool { return d.r.Catalog.CardTypeExists(cardType) }
func (d cardDeps) LogActivity(cardID, action, field string) {
	d.r.logActivity(cardID, action, field)
}
func (d cardDeps) LogActivityWithContext(cardID, action, field, cardTitle string, breadcrumbs []card.CategoryPath) {
	d.r.logActivityWithContext(cardID, action, field, cardTitle, breadcrumbs)
}
func (d cardDeps) Publish(topic string, payload any) { d.r.bus.Publish(topic, payload) }

type chatDeps struct{ r *Runtime }

func (d chatDeps) Repo() *repo.Repository { return d.r.repo }

type agentDeps struct{ r *Runtime }

func (d agentDeps) Repo() *repo.Repository            { return d.r.repo }
func (d agentDeps) Index() *index.Index               { return d.r.idx }
func (d agentDeps) Publish(topic string, payload any) { d.r.bus.Publish(topic, payload) }
func (d agentDeps) LLM() *llmsvc.Service              { return d.r.LLM }
func (d agentDeps) MCPRegistry() *mcp.Registry        { return d.r.MCPRegistry() }
func (d agentDeps) NativeToolDefs() []llm.ToolDef     { return nativeTools{d.r}.Defs(false) }

type repoDeps struct{ r *Runtime }

func (d repoDeps) Repo() *repo.Repository { return d.r.repo }
func (d repoDeps) Index() *index.Index    { return d.r.idx }

type workspaceDeps struct{ r *Runtime }

func (d workspaceDeps) Repo() *repo.Repository            { return d.r.repo }
func (d workspaceDeps) Publish(topic string, payload any) { d.r.bus.Publish(topic, payload) }

// ---------------------------------------------------------------------
// LLM runtime Deps adapters. "RT" suffix to avoid colliding with the
// service-layer adapters above.
// ---------------------------------------------------------------------

type toolsRTDeps struct{ r *Runtime }

func (d toolsRTDeps) Repo() *repo.Repository            { return d.r.repo }
func (d toolsRTDeps) Registry() *schema.Registry        { return d.r.registry }
func (d toolsRTDeps) Publish(topic string, payload any) { d.r.bus.Publish(topic, payload) }
func (d toolsRTDeps) Card() *card.Service               { return d.r.Card }
func (d toolsRTDeps) Workspace() *workspacesvc.Service  { return d.r.Workspace }
func (d toolsRTDeps) Project() *projectsvc.Service      { return d.r.Project }
func (d toolsRTDeps) Catalog() *catalog.Service         { return d.r.Catalog }
func (d toolsRTDeps) Agent() *agentsvc.Service          { return d.r.Agent }
func (d toolsRTDeps) Native() tools.NativeTools         { return nativeTools{d.r} }

type promptsRTDeps struct{ r *Runtime }

func (d promptsRTDeps) Repo() *repo.Repository     { return d.r.repo }
func (d promptsRTDeps) Registry() *schema.Registry { return d.r.registry }
func (d promptsRTDeps) Card() *card.Service        { return d.r.Card }
func (d promptsRTDeps) Search() *search.Service    { return d.r.Search }
func (d promptsRTDeps) Catalog() *catalog.Service  { return d.r.Catalog }

type chatRTDeps struct{ r *Runtime }

func (d chatRTDeps) Repo() *repo.Repository     { return d.r.repo }
func (d chatRTDeps) Registry() *schema.Registry { return d.r.registry }
func (d chatRTDeps) Ctx() context.Context       { return d.r.ctx }
func (d chatRTDeps) LLM() *llmsvc.Service       { return d.r.LLM }
func (d chatRTDeps) Card() *card.Service        { return d.r.Card }
func (d chatRTDeps) Catalog() *catalog.Service  { return d.r.Catalog }
func (d chatRTDeps) Tools() *tools.Dispatcher   { return d.r.tools }
func (d chatRTDeps) Prompts() *prompts.Builder  { return d.r.prompts }
func (d chatRTDeps) MCPRegistry() *mcp.Registry { return d.r.MCPRegistry() }
func (d chatRTDeps) LLMActors() *sync.Map       { return &d.r.llmActors }
func (d chatRTDeps) Publish(topic string, payload any) {
	d.r.bus.Publish(topic, payload)
}

type agentRTDeps struct{ r *Runtime }

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
func (d agentRTDeps) MCPRegistry() *mcp.Registry        { return d.r.MCPRegistry() }
func (d agentRTDeps) LLMActors() *sync.Map              { return &d.r.llmActors }
func (d agentRTDeps) Native() tools.NativeTools         { return nativeTools{d.r} }

// Suppress unused-import warning for chatsvc / agentsvc / projectsvc /
// reposvc when the build decides to flag them — the deps adapters use
// them as receiver field types but Go still wants an explicit reference.
// (No-op in practice; kept to make refactors safe.)
var _ = chatsvc.New
var _ = agentsvc.New
var _ = projectsvc.New
var _ = reposvc.New
var _ = notify.New
var _ = settings.New
