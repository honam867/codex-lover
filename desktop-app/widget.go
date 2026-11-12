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
// reads the store; the main app keeps refreshing the data and does the
// switching, so only one process ever changes accounts.

const (
	widgetFlag     = "--widget"
	widgetTitle    = "codex-lover quota"
	switchNextFlag = "--switch-next"
)

type widgetSettings struct {
	Enabled bool    `json:"enabled"`
	HasPos  bool    `json:"has_pos"`
	X       int     `json:"x"`
	Y       int     `json:"y"`
	Width   int     `json:"width"`
	Opacity float64 `json:"opacity"` // 0 = default
}

func codexLoverPath(name string) string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".codex-lover", name)
}

func readJSONFile(path string, target any) {
	if data, err := os.ReadFile(path); err == nil {
		_ = json.Unmarshal(data, target)
	}
}

func writeJSONFile(path string, value any) {
	data, _ := json.Marshal(value)
	tmp := path + ".tmp"
	if os.WriteFile(tmp, data, 0o644) == nil {
		_ = os.Rename(tmp, path)
	}
}

func loadWidgetSettings() widgetSettings {
	var s widgetSettings
	readJSONFile(codexLoverPath("widget.json"), &s)
	return s
}

func saveWidgetSettings(s widgetSettings) {
	writeJSONFile(codexLoverPath("widget.json"), s)
}

// switchResult is the main app's answer to a widget switch request.
type switchResult struct {
	At      int64  `json:"at"` // unix ms
	Message string `json:"message"`
}

func launchSelf(args ...string) {
	exe, err := os.Executable()
	if err != nil {
		return
	}
	_ = exec.Command(exe, args...).Start() // single-instance lock routes it to the running app
}

func launchWidget() {
	launchSelf(widgetFlag)
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

// SwitchToNextCodex switches to the Codex account with the most quota left and
// writes the outcome for the widget.
func (a *App) SwitchToNextCodex() {
	writeJSONFile(codexLoverPath("widget-switch.json"), switchResult{
		At:      time.Now().UnixMilli(),
		Message: a.switchToNextCodex(),
	})
}

func (a *App) switchToNextCodex() string {
	if err := a.ensureReady(); err != nil {
		return "Lỗi: " + err.Error()
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	statuses, err := a.svc.ProfileStatuses()
	if err != nil {
		return "Lỗi: " + err.Error()
	}
	// Pick from fresh quota, not numbers up to 15 minutes old.
	if fresh, _, err := a.svc.RefreshLoggedOutCachedUsage(statuses); err == nil {
		statuses = fresh
	}
	result, err := a.svc.SwitchToBestCodex(statuses)
	if err != nil {
		return "Lỗi: " + err.Error()
	}
	if !result.Changed {
		return "Không còn acc nào có quota"
	}
	_, _ = a.refreshLocked(false)
	return "Đã chuyển sang " + profileLabel(result.To)
}

type Widget struct {
	ctx      context.Context
	isWidget bool
	store    *store.Store
}

// WidgetStatus is what the widget shows: the active account and quota counts.
type WidgetStatus struct {
	Active    *ProfileCard `json:"active"`
	Total     int          `json:"total"`
	Available int          `json:"available"`
	Out       int          `json:"out"`
}

func (w *Widget) startup(ctx context.Context) {
	w.ctx = ctx
	s := loadWidgetSettings()
	if s.Width > 0 {
		_, height := wailsruntime.WindowGetSize(ctx)
		wailsruntime.WindowSetSize(ctx, s.Width, height)
	}
	if s.HasPos {
		wailsruntime.WindowSetPosition(ctx, s.X, s.Y)
	}
	setWindowOpacity(widgetTitle, s.Opacity)
	go w.watch()
}

// watch closes the widget when it is toggled off from the main app and
// remembers where it was dragged to and how wide it is.
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
			width, _ := wailsruntime.WindowGetSize(w.ctx)
			if !s.HasPos || x != s.X || y != s.Y || width != s.Width {
				s.HasPos, s.X, s.Y, s.Width = true, x, y, width
				saveWidgetSettings(s)
			}
		}
	}
}

func (w *Widget) IsWidget() bool {
	return w.isWidget
}

func (w *Widget) GetStatus() WidgetStatus {
	var status WidgetStatus
	if w.store == nil {
		return status
	}
	statuses, err := w.store.ProfileStatuses()
	if err != nil {
		return status
	}
	for _, card := range buildSnapshot(statuses, nil).Profiles {
		if card.Provider != "codex" {
			continue
		}
		status.Total++
		if card.Blocked || card.OutOfQuota {
			status.Out++
		} else {
			status.Available++
		}
		if card.IsActive {
			active := card
			status.Active = &active
		}
	}
	return status
}

// SwitchNext asks the main app to switch to the account with the most quota
// left. It returns the request time to pass to GetSwitchResult.
func (w *Widget) SwitchNext() int64 {
	requestedAt := time.Now().UnixMilli()
	launchSelf(switchNextFlag)
	return requestedAt
}

// GetSwitchResult returns the main app's answer to a switch requested at
// requestedAt, or "" while it is still working.
func (w *Widget) GetSwitchResult(requestedAt int64) string {
	var result switchResult
	readJSONFile(codexLoverPath("widget-switch.json"), &result)
	if result.At < requestedAt {
		return ""
	}
	return result.Message
}

// SetOpacity changes how see-through the widget is (0.3-1) and remembers it.
func (w *Widget) SetOpacity(opacity float64) float64 {
	opacity = clampOpacity(opacity)
	setWindowOpacity(widgetTitle, opacity)
	s := loadWidgetSettings()
	s.Opacity = opacity
	saveWidgetSettings(s)
	return opacity
}

func (w *Widget) GetOpacity() float64 {
	return clampOpacity(loadWidgetSettings().Opacity)
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
