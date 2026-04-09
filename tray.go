package main

import (
	"bruv/internal/config"
	_ "embed"
	"fmt"
	"runtime"

	"github.com/energye/systray"
	wailsRuntime "github.com/wailsapp/wails/v2/pkg/runtime"
)

//go:embed build/windows/icon.ico
var trayIconData []byte

// refreshTrayTooltip updates the tray tooltip to show unread notification count.
func (a *App) refreshTrayTooltip() {
	notifications, err := config.LoadNotifications()
	if err != nil {
		return
	}
	unread := 0
	for _, n := range notifications {
		if !n.Read {
			unread++
		}
	}
	if unread > 0 {
		systray.SetTooltip(fmt.Sprintf("BRUV — %d unread notification%s", unread, pluralS(unread)))
	} else {
		systray.SetTooltip("BRUV — Your most organised best bud")
	}
}

func pluralS(n int) string {
	if n == 1 {
		return ""
	}
	return "s"
}

// showWindow brings the main window to the foreground.
func (a *App) showWindow() {
	wailsRuntime.WindowShow(a.ctx)
	wailsRuntime.WindowUnminimise(a.ctx)
}

// setupTray initialises the system tray icon with a context menu.
// systray.Run blocks, so it runs on a dedicated goroutine pinned to
// a single OS thread — Windows requires the message pump to stay on
// the thread that created the systray hidden window.
func (a *App) setupTray() {
	go func() {
		runtime.LockOSThread()
		systray.Run(func() {
			systray.SetIcon(trayIconData)
			systray.SetTitle("BRUV")
			systray.SetTooltip("BRUV — Your most organised best bud")

			// Left-click / double-click: show window (async to not block message pump)
			systray.SetOnClick(func(menu systray.IMenu) {
				go a.showWindow()
			})
			systray.SetOnDClick(func(menu systray.IMenu) {
				go a.showWindow()
			})

			// Right-click: default behaviour shows context menu (no override needed)

			// Context menu items
			mShow := systray.AddMenuItem("Show BRUV", "Show the main window")
			mShow.Click(func() { go a.showWindow() })

			systray.AddSeparator()

			mPause := systray.AddMenuItem("Pause Agents", "Pause all scheduled agents")
			a.trayPauseItem = mPause
			mPause.Click(func() {
				go func() {
					if mPause.Checked() {
						mPause.Uncheck()
						a.ResumeAllAgents()
					} else {
						mPause.Check()
						a.PauseAllAgents()
					}
				}()
			})

			systray.AddSeparator()

			mQuit := systray.AddMenuItem("Quit", "Quit BRUV entirely")
			mQuit.Click(func() { go a.ForceQuit() })
		}, func() {
			// onExit — nothing to clean up
		})
	}()
}
