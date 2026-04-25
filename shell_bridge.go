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
