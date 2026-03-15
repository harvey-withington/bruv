package config

import (
	"encoding/json"
	"os"
	"path/filepath"
)

// WindowBounds stores the last known window position and size.
type WindowBounds struct {
	X         int  `json:"x"`
	Y         int  `json:"y"`
	Width     int  `json:"width"`
	Height    int  `json:"height"`
	Maximised bool `json:"maximised"`
}

func windowFilePath() (string, error) {
	dir, err := configDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "window.json"), nil
}

// LoadWindowBounds reads the saved window bounds. Returns nil if none saved.
func LoadWindowBounds() *WindowBounds {
	path, err := windowFilePath()
	if err != nil {
		return nil
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return nil
	}
	var wb WindowBounds
	if err := json.Unmarshal(data, &wb); err != nil {
		return nil
	}
	// Basic sanity: width/height must be positive
	if wb.Width < 200 || wb.Height < 200 {
		return nil
	}
	return &wb
}

// SaveWindowBounds persists window bounds to disk.
func SaveWindowBounds(wb *WindowBounds) error {
	path, err := windowFilePath()
	if err != nil {
		return err
	}
	data, err := json.MarshalIndent(wb, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0o644)
}

// ClampToVisible adjusts window bounds so that at least minVisible pixels
// of the window are within the given screen area. screenW/screenH represent
// the combined desktop dimensions available. This is a simple heuristic
// that ensures the window isn't entirely off-screen.
func ClampToVisible(wb *WindowBounds, screenW, screenH int) {
	const minVisible = 100

	// Ensure window is at least partially on-screen horizontally
	if wb.X+wb.Width < minVisible {
		wb.X = 0
	}
	if wb.X > screenW-minVisible {
		wb.X = screenW - wb.Width
		if wb.X < 0 {
			wb.X = 0
		}
	}

	// Ensure window is at least partially on-screen vertically
	if wb.Y+wb.Height < minVisible {
		wb.Y = 0
	}
	if wb.Y > screenH-minVisible {
		wb.Y = screenH - wb.Height
		if wb.Y < 0 {
			wb.Y = 0
		}
	}
}
