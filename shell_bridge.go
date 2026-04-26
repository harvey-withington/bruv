package main

// ShellAPI is the narrow Wails-bound surface exposed to the frontend.
// It holds ONLY the methods that must execute inside the Wails shell:
// native dialogs, shell-open helpers, force-quit, and the bootstrap
// call the cloud adapter uses to discover the loopback HTTP address
// and bearer token.
//
// Everything else — all ~130 domain methods — travels over HTTP+SSE
// via the cloud adapter. That's the entire point of the phase-4
// pivot: Wails binds the GUI shell and nothing more.

// ShellAPI is bound via wails.Bind(shellAPI) in main.go. Methods on
// *App are intentionally NOT bound; the domain surface is reached
// over HTTP through the transport package.
type ShellAPI struct{ app *App }

func newShellAPI(app *App) *ShellAPI { return &ShellAPI{app: app} }

// GetHTTPTransportInfo returns the loopback address + bearer token of
// the Go HTTP server. The cloud adapter calls this once at boot and
// then every domain call goes over HTTP. Must remain bound for the
// adapter to bootstrap.
func (s *ShellAPI) GetHTTPTransportInfo() map[string]string {
	return s.app.GetHTTPTransportInfo()
}

// --- Native dialogs (must run in the shell process) ---

func (s *ShellAPI) PickFolder(title string) (string, error) {
	return s.app.PickFolder(title)
}

func (s *ShellAPI) PickFile(title, filterName, filterPattern string) (string, error) {
	return s.app.PickFile(title, filterName, filterPattern)
}

func (s *ShellAPI) PickSaveFile(title, defaultName, filterName, filterPattern string) (string, error) {
	return s.app.PickSaveFile(title, defaultName, filterName, filterPattern)
}

// --- Shell-open helpers (Explorer/Finder/browser) ---

func (s *ShellAPI) OpenConfigFolder() error {
	return s.app.OpenConfigFolder()
}

func (s *ShellAPI) OpenLogsFolder() error {
	return s.app.OpenLogsFolder()
}

func (s *ShellAPI) OpenBugReportURL() error {
	return s.app.OpenBugReportURL()
}

// --- Process control ---

// ForceQuit is kept on the shell surface because it mutates the
// forceQuit flag that beforeClose reads to decide between "hide
// to tray" and "actually quit". That's a shell-lifecycle concern.
func (s *ShellAPI) ForceQuit() {
	s.app.ForceQuit()
}

// --- Connections (per-machine local state) ---
//
// Connection management lives on the shell surface, not the cloud
// adapter, because it's strictly per-machine state (`<clientdata>/
// connections.json`) and must stay reachable when the active
// connection's backend is unreachable. Without this the user gets
// stuck: a misconfigured remote breaks every RPC including the one
// they'd need to call to switch back to Local.

func (s *ShellAPI) ListConnections() (any, error) {
	return s.app.ListConnections()
}

func (s *ShellAPI) AddConnection(name, url, deviceToken string) (any, error) {
	return s.app.AddConnection(name, url, deviceToken)
}

func (s *ShellAPI) RemoveConnection(id string) error {
	return s.app.RemoveConnection(id)
}

func (s *ShellAPI) UpdateConnection(id, name, url, deviceToken string) (any, error) {
	return s.app.UpdateConnection(id, name, url, deviceToken)
}

func (s *ShellAPI) SetActiveConnection(id string) error {
	return s.app.SetActiveConnection(id)
}

// SetActiveRepo lives on the shell surface for the same reason as
// the connection methods: per-device picker state must be settable
// even when the active connection's backend is unreachable. After
// the picker, the frontend calls this then reloads — the cloud
// adapter then re-reads GetHTTPTransportInfo and includes the new
// repo ID in every request URL.
func (s *ShellAPI) SetActiveRepo(repoID string) error {
	return s.app.SetActiveRepo(repoID)
}

// --- Local repo management ---
//
// Operations against the *Local* (desktop loopback) repo registry
// must always run on the desktop App, regardless of which Remote
// connection happens to be active. The picker shows Local rows
// even when a Remote is active; clicking + / pencil / X on a Local
// row needs to manipulate `<userConfigDir>/repos.json`, which only
// the desktop App can do. Routing these through the cloud adapter
// would hit whatever bare-`/rpc` the active connection exposes —
// 404 on a multi-repo Remote with no repoID selected.

func (s *ShellAPI) InspectRepoPath(path string) (any, error) {
	return s.app.InspectRepoPath(path)
}

func (s *ShellAPI) InitRepository(path, name string) (string, error) {
	return s.app.InitRepository(path, name)
}

func (s *ShellAPI) OpenRepository(path string) error {
	return s.app.OpenRepository(path)
}

func (s *ShellAPI) ListLocalRepos() (any, error) {
	return s.app.ListLocalRepos()
}

func (s *ShellAPI) RemoveLocalRepo(id string) error {
	return s.app.RemoveLocalRepo(id)
}

func (s *ShellAPI) RenameLocalRepo(id, name string) error {
	return s.app.RenameLocalRepo(id, name)
}

func (s *ShellAPI) SetLocalRepoEnabled(id string, enabled bool) error {
	return s.app.SetLocalRepoEnabled(id, enabled)
}

func (s *ShellAPI) GetLastOpenedLocalRepoPath() string {
	return s.app.GetLastOpenedLocalRepoPath()
}

// CloseRepository on Shell so the back-to-picker action works
// regardless of which connection is currently active. Cloud-adapter
// routing would send it to /repos/<id>/rpc on a Remote, where there
// is no CloseRepository (it's an App-only concept) — the call would
// silently 404 and the back button would do nothing user-visible.
func (s *ShellAPI) CloseRepository() {
	s.app.CloseRepository()
}

// SetActiveRepoForConnection lets the frontend pre-set the target
// connection's last-active-repo BEFORE switching to that connection.
// Without it, switching connections always lands on the picker
// (cloud adapter resolves with no repoID set for the new connection).
func (s *ShellAPI) SetActiveRepoForConnection(connectionID, repoID string) error {
	return s.app.SetActiveRepoForConnection(connectionID, repoID)
}
