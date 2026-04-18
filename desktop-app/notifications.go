package main

import (
	"fmt"
	"strings"
	"time"

	"codex-lover/internal/model"
	"codex-lover/internal/service"
)

type desktopWatchNotifications struct {
	windows map[string]desktopWindowThresholdState
}

type desktopThresholdEvent struct {
	Title   string
	Message string
}

type desktopWindowThresholdState struct {
	ResetKey             string
	LastRemainingPercent float64
	HasLastRemaining     bool
	Notified20           bool
	Notified10           bool
}

func newDesktopWatchNotifications() *desktopWatchNotifications {
	return &desktopWatchNotifications{
		windows: map[string]desktopWindowThresholdState{},
	}
}

func (watcher *desktopWatchNotifications) collectThresholdEvents(statuses []model.ProfileStatus) []desktopThresholdEvent {
	now := time.Now()
	events := []desktopThresholdEvent{}

	for _, status := range statuses {
		if status.State.AuthStatus != model.AuthStatusActive {
			continue
		}
		events = append(events, watcher.collectWindowThresholdEvents(status, "5H", status.State.UsageWindowPrimary(), now)...)
		events = append(events, watcher.collectWindowThresholdEvents(status, "WEEKLY", status.State.UsageWindowSecondary(), now)...)
	}

	return events
}

func (watcher *desktopWatchNotifications) collectWindowThresholdEvents(
	status model.ProfileStatus,
	windowLabel string,
	window *model.UsageWindow,
	now time.Time,
) []desktopThresholdEvent {
	if window == nil {
		return nil
	}

	displayWindow := service.EffectiveWindowForDisplay(window, status.State.AuthStatus, now)
	if displayWindow == nil {
		return nil
	}

	key := status.Profile.ID + "\x00" + windowLabel
	state := watcher.windows[key]
	resetKey := desktopNotificationResetKey(displayWindow)
	if state.ResetKey != resetKey {
		state = desktopWindowThresholdState{ResetKey: resetKey}
	}

	current := displayWindow.RemainingPercent
	events := []desktopThresholdEvent{}
	if state.HasLastRemaining {
		switch {
		case !state.Notified10 && state.LastRemainingPercent > 10 && current <= 10:
			state.Notified10 = true
			state.Notified20 = true
			events = append(events, buildDesktopThresholdEvent(status, windowLabel, 10, displayWindow))
		case !state.Notified20 && state.LastRemainingPercent > 20 && current <= 20:
			state.Notified20 = true
			events = append(events, buildDesktopThresholdEvent(status, windowLabel, 20, displayWindow))
		}
	}

	state.LastRemainingPercent = current
	state.HasLastRemaining = true
	watcher.windows[key] = state
	return events
}

func buildDesktopThresholdEvent(
	status model.ProfileStatus,
	windowLabel string,
	threshold int,
	window *model.UsageWindow,
) desktopThresholdEvent {
	account := desktopThresholdAccountLabel(status)
	provider := strings.TrimSpace(status.Profile.Provider)
	if provider == "" {
		provider = status.Profile.Tool
	}
	if provider == "" {
		provider = "account"
	}
	titleProvider := provider
	if titleProvider != "" {
		titleProvider = strings.ToUpper(titleProvider[:1]) + titleProvider[1:]
	}
	return desktopThresholdEvent{
		Title: fmt.Sprintf("%s account reached %d%%", titleProvider, threshold),
		Message: fmt.Sprintf(
			"Account %s da cham toi %d%% %s (%s)",
			account,
			threshold,
			windowLabel,
			service.FormatWindowSummary(window),
		),
	}
}

func desktopThresholdAccountLabel(status model.ProfileStatus) string {
	if status.Profile.Email != "" {
		return status.Profile.Email
	}
	if status.Profile.Label != "" {
		return status.Profile.Label
	}
	if status.Profile.AccountID != "" {
		return status.Profile.AccountID
	}
	return status.Profile.ID
}

func desktopNotificationResetKey(window *model.UsageWindow) string {
	if window == nil || window.ResetsAt == nil {
		return ""
	}
	return window.ResetsAt.UTC().Format(time.RFC3339)
}
