package main

// ShellAPI is the narrow Wails-bound surface exposed to the frontend.
// It holds ONLY the methods that must execute inside the Wails shell:
// native dialogs, shell-open helpers, force-quit, the bootstrap call
// the cloud adapter uses to discover the loopback HTTP address +
// token, and per-device connection management (which has to keep
// working when the active connection's backend is unreachable —
// otherwise a misconfigured remote locks the user out).
//
// Everything else — repo registry CRUD, per-machine settings, every
// per-repo RPC — travels over HTTP through the cloud adapter, hitting
// the multi-repo transport at /repos/<id>/rpc or /server/rpc.

// ShellAPI is bound via wails.Bind(shellAPI) in main.go. Methods on
// *App are intentionally NOT bound; the domain surface is reached
// over HTTP through the transport package.
type ShellAPI struct{ app *App }

func newShellAPI(app *App) *ShellAPI { return &ShellAPI{app: app} }

// GetHTTPTransportInfo returns the loopback (or active-remote)
// address + bearer token + active repoID. The cloud adapter calls
// this once at boot and routes every domain call to the resolved
// URL. Must remain Shell-bound — the cloud adapter has nothing else
// to bootstrap from.
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

func (s *ShellAPI) OpenConfigFolder() error  { return s.app.OpenConfigFolder() }
func (s *ShellAPI) OpenLogsFolder() error    { return s.app.OpenLogsFolder() }
func (s *ShellAPI) OpenBugReportURL() error  { return s.app.OpenBugReportURL() }

// --- Process control ---

// ForceQuit stays Shell-bound because it mutates the forceQuit flag
// that beforeClose reads to decide between "hide to tray" and
// "actually quit". Shell-lifecycle concern.
func (s *ShellAPI) ForceQuit() { s.app.ForceQuit() }

// --- Connections (per-device local state) ---
//
// Connection management lives on the Shell binding (not the cloud
// adapter) because it's strictly per-device and must stay reachable
// when the active connection's backend is unreachable. Without this
// a misconfigured remote breaks every RPC including the one the user
// would need to call to switch back to Local.

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

// SetActiveRepo is Shell-bound for the same reason as the connection
// methods: per-device picker state must be settable even when the
// active backend is unreachable. After the picker, the frontend calls
// this then reloads — the cloud adapter then re-reads
// GetHTTPTransportInfo and includes the new repo ID in every URL.
func (s *ShellAPI) SetActiveRepo(repoID string) error {
	return s.app.SetActiveRepo(repoID)
}

// SetActiveRepoForConnection lets the picker pre-set ANY connection's
// last-active-repo BEFORE switching to it. Without this, switching
// connections always lands on the picker (cloud adapter resolves with
// no repoID set for the new connection's first request). Stays
// Shell-bound because it has to be callable from the OLD connection.
func (s *ShellAPI) SetActiveRepoForConnection(connectionID, repoID string) error {
	return s.app.SetActiveRepoForConnection(connectionID, repoID)
}
