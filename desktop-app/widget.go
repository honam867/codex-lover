package main

import (
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"time"

	"codex-lover/internal/store"
	wailsruntime "github.com/wailsapp/wails/v2/pkg/runtime"
)

// The quota widget is the same executable started with --widget: a small
// frameless always-on-top window showing the active Codex account. It only
// reads the store; the main app keeps refreshing the data.

const widgetFlag = "--widget"

type widgetSettings struct {
	Enabled bool `json:"enabled"`
	HasPos  bool `json:"has_pos"`
	X       int  `json:"x"`
	Y       int  `json:"y"`
}

func widgetSettingsPath() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".codex-lover", "widget.json")
}

func loadWidgetSettings() widgetSettings {
	var s widgetSettings
	if data, err := os.ReadFile(widgetSettingsPath()); err == nil {
		_ = json.Unmarshal(data, &s)
	}
	return s
}

func saveWidgetSettings(s widgetSettings) {
	data, _ := json.Marshal(s)
	path := widgetSettingsPath()
	tmp := path + ".tmp"
	if os.WriteFile(tmp, data, 0o644) == nil {
		_ = os.Rename(tmp, path)
	}
}

func launchWidget() {
	exe, err := os.Executable()
	if err != nil {
		return
	}
	_ = exec.Command(exe, widgetFlag).Start() // single-instance lock drops duplicates
}

// ToggleWidget shows or hides the floating quota widget and returns the new state.
func (a *App) ToggleWidget() bool {
	s := loadWidgetSettings()
	s.Enabled = !s.Enabled
	saveWidgetSettings(s)
	if s.Enabled {
		launchWidget()
	}
	return s.Enabled
}

func (a *App) WidgetEnabled() bool {
	return loadWidgetSettings().Enabled
}

type Widget struct {
	ctx      context.Context
	isWidget bool
	store    *store.Store
}

func (w *Widget) startup(ctx context.Context) {
	w.ctx = ctx
	if s := loadWidgetSettings(); s.HasPos {
		wailsruntime.WindowSetPosition(ctx, s.X, s.Y)
	}
	go w.watch()
}

// watch closes the widget when it is toggled off from the main app and
// remembers where it was dragged to.
func (w *Widget) watch() {
	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-w.ctx.Done():
			return
		case <-ticker.C:
			s := loadWidgetSettings()
			if !s.Enabled {
				wailsruntime.Quit(w.ctx)
				return
			}
			x, y := wailsruntime.WindowGetPosition(w.ctx)
			if !s.HasPos || x != s.X || y != s.Y {
				s.HasPos, s.X, s.Y = true, x, y
				saveWidgetSettings(s)
			}
		}
	}
}

func (w *Widget) IsWidget() bool {
	return w.isWidget
}

// GetActive returns the active Codex account card, or nil when none is active.
func (w *Widget) GetActive() *ProfileCard {
	if w.store == nil {
		return nil
	}
	statuses, err := w.store.ProfileStatuses()
	if err != nil {
		return nil
	}
	for _, card := range buildSnapshot(statuses, nil).Profiles {
		if card.IsActive && card.Provider == "codex" {
			return &card
		}
	}
	return nil
}

// Close hides the widget and keeps it off until it is toggled on again.
func (w *Widget) Close() {
	s := loadWidgetSettings()
	s.Enabled = false
	saveWidgetSettings(s)
	wailsruntime.Quit(w.ctx)
}

func widgetWebviewDataPath() string {
	dir, err := os.UserCacheDir() // %LOCALAPPDATA% on Windows
	if err != nil {
		return ""
	}
	return filepath.Join(dir, "codex-lover", "widget-webview")
}
