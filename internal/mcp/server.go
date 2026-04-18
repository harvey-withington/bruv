package mcp

import (
	"bruv/internal/logging"
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"runtime"
	"sync"
	"time"
)

// HealthStatus describes the current state of a server subprocess for
// display in the Settings UI and for decisions about whether to
// include its tools in the agent tool catalogue.
type HealthStatus string

const (
	// HealthDisabled means the user has disabled this server; we
	// don't spawn a process for it and no tools are offered.
	HealthDisabled HealthStatus = "disabled"
	// HealthStarting means we've spawned the subprocess but haven't
	// completed the initialize handshake yet.
	HealthStarting HealthStatus = "starting"
	// HealthReady means initialize succeeded and tools/list returned.
	HealthReady HealthStatus = "ready"
	// HealthFailed means startup failed or the process has crashed
	// and the retry budget is exhausted.
	HealthFailed HealthStatus = "failed"
	// HealthRestarting means the process crashed and we're about to
	// retry the spawn.
	HealthRestarting HealthStatus = "restarting"
)

// ServerSpec is the config-side description of one MCP server. This
// is the shape stored in .bruv/mcp_servers.json and used to drive the
// lifecycle manager. The Env map lists environment variable *names*
// only — values are fetched from the keychain at spawn time so they
// never touch the repo file and never leak when a repo is shared.
type ServerSpec struct {
	// Name is the user-chosen identifier (e.g. "filesystem",
	// "github"). Must be unique within a repo. Used as the namespace
	// prefix on tool names and as part of the keychain key for
	// secrets.
	Name string `json:"name"`

	// Description is a human-friendly label shown in the Settings
	// UI. Optional.
	Description string `json:"description,omitempty"`

	// Command is the executable to run — e.g. "npx",
	// "/usr/local/bin/my-mcp-server", "python".
	Command string `json:"command"`

	// Args are the command-line arguments passed to Command.
	Args []string `json:"args,omitempty"`

	// EnvNames lists the names of environment variables this server
	// requires. Values live in the OS keychain under
	// "mcp:<repoID>:<serverName>:<varName>". Listing names here but
	// not values lets repo sharing work safely — Bob gets Alice's
	// server config but has to supply his own credentials.
	EnvNames []string `json:"env_names,omitempty"`

	// Enabled controls whether this server is spawned at startup.
	// Disabled servers remain in config but don't consume any
	// resources and don't contribute tools to the agent catalogue.
	Enabled bool `json:"enabled"`

	// InitTimeout bounds how long we'll wait for the initialize
	// handshake to complete. Zero means use the default. The first
	// launch of an npx-based server has to download and install the
	// package (tens of seconds on a cold machine), which is why the
	// default is generous rather than millisecond-tight. Override
	// this in tests that want a tighter bound, or for servers known
	// to be pre-installed and thus expected to handshake fast.
	InitTimeout time.Duration `json:"init_timeout,omitempty"`
}

// defaultInitTimeout covers cold-start npx installs on the slowest
// reasonable target (Windows CI runners), where the first launch has
// to npm-install the package before the server can talk to us. Warm
// launches complete in milliseconds; the budget only matters on the
// first run after an install.
const defaultInitTimeout = 60 * time.Second

// SecretResolver fetches secret env var values at spawn time. This
// is an interface so the package doesn't directly depend on BRUV's
// internal/config/keychain; the registry wires in a concrete
// implementation from the app layer.
//
// Lookup returns the value and a boolean indicating whether the
// secret was found. A missing secret is not an error — we just pass
// an empty string for that env var so the server can decide whether
// to fail fast or run in a reduced mode.
type SecretResolver interface {
	Lookup(repoID, serverName, envVarName string) (string, bool)
}

// ServerProcess owns one MCP server subprocess and its Client. It
// handles:
//
//   - Spawning the subprocess with the right working directory, env,
//     and pipes.
//   - Driving the Initialize → ListTools startup sequence.
//   - Caching discovered tools for the Registry to expose.
//   - Health tracking and restart attempts on crash.
//   - Graceful shutdown when Close() is called.
//
// One ServerProcess per ServerSpec. Goroutine-safe for the operations
// the Registry needs; individual methods document their concurrency
// assumptions.
type ServerProcess struct {
	spec     ServerSpec
	repoID   string
	resolver SecretResolver

	// maxRetries caps the number of restart attempts per startup
	// window. Default 3 — zero means never restart.
	maxRetries int

	mu          sync.Mutex
	status      HealthStatus
	lastError   string
	tools       []Tool
	serverInfo  ServerInfo
	protoVer    string
	cmd         *exec.Cmd
	client      *Client
	transport   *Transport
	startedAt   time.Time
	failCount   int
	stopRequested bool
}

// NewServerProcess creates an unstarted ServerProcess for a single
// server spec. Call Start to actually spawn the subprocess.
func NewServerProcess(spec ServerSpec, repoID string, resolver SecretResolver) *ServerProcess {
	return &ServerProcess{
		spec:       spec,
		repoID:     repoID,
		resolver:   resolver,
		maxRetries: 3,
		status:     HealthDisabled,
	}
}

// Start spawns the subprocess and performs the full MCP handshake. On
// success, the ServerProcess is in HealthReady and its tools are
// available via Tools(). On failure, status is HealthFailed and
// LastError returns a human-readable reason.
//
// ctx is used for the initialize + tools/list timeout, not for
// ongoing tool calls — those get their own contexts from the caller.
func (s *ServerProcess) Start(ctx context.Context) error {
	s.mu.Lock()
	if !s.spec.Enabled {
		s.status = HealthDisabled
		s.mu.Unlock()
		return nil
	}
	s.status = HealthStarting
	s.lastError = ""
	s.stopRequested = false
	s.mu.Unlock()

	if err := s.spawnAndHandshake(ctx); err != nil {
		s.mu.Lock()
		s.status = HealthFailed
		s.lastError = err.Error()
		s.mu.Unlock()
		return err
	}

	// Kick off the supervisor goroutine that watches for process
	// exit and attempts restart if the exit was unexpected.
	go s.supervise()

	return nil
}

// spawnAndHandshake does the actual work of bringing the subprocess
// up to HealthReady. Called by Start and by the restart path.
// Assumes s.mu is NOT held.
func (s *ServerProcess) spawnAndHandshake(ctx context.Context) error {
	// Resolve command for the current platform. The Windows npx
	// shim is a .cmd file and Go's exec.Command won't find it via
	// PATH without help; we special-case that here so users can
	// just write "npx" in their config.
	command := s.spec.Command
	args := s.spec.Args
	if runtime.GOOS == "windows" && (command == "npx" || command == "npm") {
		// On Windows, npx lives as npx.cmd — exec.LookPath handles
		// .cmd resolution but only when we ask for it by full name
		// or let the shell invoke it. Using cmd /c is the robust
		// path that handles both installed-via-nvm and
		// installed-via-system setups.
		args = append([]string{"/c", command}, args...)
		command = "cmd"
	}

	cmd := exec.CommandContext(context.Background(), command, args...)

	// Build env: start with a minimal base (PATH, HOME,
	// SYSTEMROOT on Windows, LANG for locale-aware servers) and
	// layer the user-declared secrets on top. We do NOT inherit
	// the full parent environment — an MCP server should only see
	// what's explicitly configured for it plus the bare minimum
	// needed to run a subprocess at all.
	cmd.Env = s.buildEnv()

	stdin, err := cmd.StdinPipe()
	if err != nil {
		return fmt.Errorf("stdin pipe: %w", err)
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return fmt.Errorf("stdout pipe: %w", err)
	}
	stderr, err := cmd.StderrPipe()
	if err != nil {
		return fmt.Errorf("stderr pipe: %w", err)
	}

	if err := cmd.Start(); err != nil {
		return fmt.Errorf("spawn %q: %w", command, err)
	}

	transport := NewTransport(s.spec.Name, stdin, stdout, stderr, nil)
	transport.Start()
	client := NewClient(transport)

	// Initialize handshake. Warm launches complete in milliseconds;
	// cold-start npx installs can take 30–60s while npm pulls the
	// package. See ServerSpec.InitTimeout for the rationale.
	initTimeout := s.spec.InitTimeout
	if initTimeout == 0 {
		initTimeout = defaultInitTimeout
	}
	initCtx, cancel := context.WithTimeout(ctx, initTimeout)
	defer cancel()
	if err := client.Initialize(initCtx); err != nil {
		_ = transport.Close()
		_ = cmd.Process.Kill()
		return fmt.Errorf("initialize: %w", err)
	}

	// Discover tools. This is part of "ready" — a server that
	// handshakes but whose tools/list fails is not useful and
	// should be treated as failed so agents don't see half-working
	// state.
	listCtx, cancel2 := context.WithTimeout(ctx, 10*time.Second)
	defer cancel2()
	tools, err := client.ListTools(listCtx)
	if err != nil {
		_ = transport.Close()
		_ = cmd.Process.Kill()
		return fmt.Errorf("tools/list: %w", err)
	}

	s.mu.Lock()
	s.cmd = cmd
	s.client = client
	s.transport = transport
	s.tools = tools
	s.serverInfo = client.ServerInfo()
	s.protoVer = client.ProtocolVersion()
	s.status = HealthReady
	s.startedAt = time.Now()
	s.mu.Unlock()

	slog.Info("mcp server ready",
		"server", s.spec.Name,
		"tools", len(tools),
		"protocol", s.protoVer,
		"server_name", s.serverInfo.Name,
		"server_version", s.serverInfo.Version)
	return nil
}

// buildEnv constructs the child process's environment. Starts with a
// minimal base of system variables the server almost certainly needs,
// then adds each declared secret from the resolver.
func (s *ServerProcess) buildEnv() []string {
	// Base env: pass through the variables every subprocess needs
	// to function at all. We explicitly do NOT inherit the full
	// parent environment — that's a foot-gun where the user's
	// shell-level secrets (AWS keys etc.) could leak into servers
	// that should only see what they were configured with. But we
	// DO need enough for common runtimes to start:
	//
	//   PATH                — find the interpreter/binary
	//   HOME / USERPROFILE  — user-specific config paths
	//   APPDATA / LOCALAPPDATA — Windows roaming/local app data
	//                            (npm/npx absolutely require this
	//                            to locate their cache and config)
	//   SYSTEMROOT / WINDIR — Windows system paths
	//   TEMP / TMP          — scratch space
	//   LANG / LC_ALL       — locale for text-output tools
	//   NODE_PATH           — let Node find globally-installed
	//                         packages if the server config
	//                         already set it
	baseVars := []string{
		"PATH", "HOME", "USERPROFILE",
		"APPDATA", "LOCALAPPDATA",
		"SYSTEMROOT", "WINDIR",
		"TEMP", "TMP",
		"LANG", "LC_ALL",
		"NODE_PATH",
	}
	env := make([]string, 0, len(baseVars)+len(s.spec.EnvNames))
	for _, name := range baseVars {
		if val, ok := os.LookupEnv(name); ok {
			env = append(env, name+"="+val)
		}
	}
	// Secrets: every declared name gets looked up. Missing ones
	// are passed as empty strings rather than omitted — the server
	// can decide whether empty means "not set" or "reduced mode".
	for _, name := range s.spec.EnvNames {
		val := ""
		if s.resolver != nil {
			if v, ok := s.resolver.Lookup(s.repoID, s.spec.Name, name); ok {
				val = v
			}
		}
		env = append(env, name+"="+val)
	}
	return env
}

// supervise runs in a goroutine and watches cmd.Wait for unexpected
// process exit. If the process exits while we didn't request a stop,
// we attempt a restart up to maxRetries times before giving up.
func (s *ServerProcess) supervise() {
	defer logging.Recover("mcp-supervise-" + s.spec.Name)
	s.mu.Lock()
	cmd := s.cmd
	s.mu.Unlock()
	if cmd == nil {
		return
	}

	waitErr := cmd.Wait()

	s.mu.Lock()
	if s.stopRequested {
		s.status = HealthDisabled
		s.mu.Unlock()
		return
	}
	s.failCount++
	failCount := s.failCount
	retries := s.maxRetries
	name := s.spec.Name
	s.mu.Unlock()

	if waitErr != nil {
		slog.Warn("mcp subprocess exited unexpectedly",
			"server", name, "err", waitErr,
			"fail", failCount, "retries", retries)
	} else {
		slog.Warn("mcp subprocess exited unprompted",
			"server", name,
			"fail", failCount, "retries", retries)
	}

	if failCount > retries {
		s.mu.Lock()
		s.status = HealthFailed
		s.lastError = fmt.Sprintf("subprocess crashed %d times, giving up", failCount)
		s.mu.Unlock()
		return
	}

	s.mu.Lock()
	s.status = HealthRestarting
	s.mu.Unlock()

	// Wait a moment before retrying so a crash-loop doesn't burn
	// CPU. Not exponential — just a small linear delay. Real
	// backoff is a follow-up enhancement.
	time.Sleep(2 * time.Second)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	if err := s.spawnAndHandshake(ctx); err != nil {
		slog.Warn("mcp restart failed", "server", name, "err", err)
		s.mu.Lock()
		s.status = HealthFailed
		s.lastError = err.Error()
		s.mu.Unlock()
		return
	}
	go s.supervise()
}

// CallTool forwards a tool invocation to the underlying client. It
// returns a transport/protocol error if the server isn't currently
// ready, so callers get a clear failure rather than a nil-pointer
// panic when an agent tries to use a crashed server.
func (s *ServerProcess) CallTool(ctx context.Context, toolName string, args map[string]interface{}) (*CallToolResult, error) {
	s.mu.Lock()
	client := s.client
	status := s.status
	s.mu.Unlock()

	if status != HealthReady || client == nil {
		return nil, fmt.Errorf("server %q is not ready (status: %s)", s.spec.Name, status)
	}
	return client.CallTool(ctx, toolName, args)
}

// Tools returns a copy of the discovered tool list. Safe to call at
// any time; returns an empty slice if the server isn't ready.
func (s *ServerProcess) Tools() []Tool {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]Tool, len(s.tools))
	copy(out, s.tools)
	return out
}

// Health returns a snapshot of the server's current state for UI
// display. Safe to call at any time.
func (s *ServerProcess) Health() ServerHealth {
	s.mu.Lock()
	defer s.mu.Unlock()
	return ServerHealth{
		Name:            s.spec.Name,
		Status:          s.status,
		LastError:       s.lastError,
		ToolCount:       len(s.tools),
		ProtocolVersion: s.protoVer,
		ServerName:      s.serverInfo.Name,
		ServerVersion:   s.serverInfo.Version,
		StartedAt:       s.startedAt,
	}
}

// Spec returns the ServerSpec this process was created with. Used by
// the Registry to re-save configs that have been modified from the UI.
func (s *ServerProcess) Spec() ServerSpec {
	return s.spec
}

// Stop shuts down the subprocess gracefully. Closes stdin (MCP's
// shutdown signal), waits up to 3 seconds for clean exit, then kills
// if the process hasn't gone away.
func (s *ServerProcess) Stop() error {
	s.mu.Lock()
	s.stopRequested = true
	cmd := s.cmd
	transport := s.transport
	s.mu.Unlock()

	if transport != nil {
		_ = transport.Close() // closes stdin
	}
	if cmd == nil || cmd.Process == nil {
		return nil
	}

	// Wait for clean exit with a deadline. If the process takes
	// too long we kill it and move on — a hung server is worse
	// than a dead one.
	done := make(chan error, 1)
	go func() { done <- cmd.Wait() }()
	select {
	case <-done:
		return nil
	case <-time.After(3 * time.Second):
		if err := cmd.Process.Kill(); err != nil {
			return fmt.Errorf("kill hung server: %w", err)
		}
		return errors.New("server did not exit cleanly, killed")
	}
}

// ServerHealth is the external view of a ServerProcess's state.
type ServerHealth struct {
	Name            string       `json:"name"`
	Status          HealthStatus `json:"status"`
	LastError       string       `json:"last_error,omitempty"`
	ToolCount       int          `json:"tool_count"`
	ProtocolVersion string       `json:"protocol_version,omitempty"`
	ServerName      string       `json:"server_name,omitempty"`
	ServerVersion   string       `json:"server_version,omitempty"`
	StartedAt       time.Time    `json:"started_at,omitempty"`
}
