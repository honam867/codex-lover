package app

import (
	"fmt"
	"time"

	"codex-lover/internal/model"
	"codex-lover/internal/service"
)

type watchNotifications struct {
	windows map[string]windowThresholdState
}

type thresholdEvent struct {
	Title   string
	Message string
}

type windowThresholdState struct {
	ResetKey             string
	LastRemainingPercent float64
	HasLastRemaining     bool
	Notified20           bool
	Notified10           bool
}

func newWatchNotifications() *watchNotifications {
	return &watchNotifications{
		windows: map[string]windowThresholdState{},
	}
}

func (watcher *watchNotifications) collectThresholdEvents(statuses []model.ProfileStatus) []thresholdEvent {
	now := time.Now()
	events := []thresholdEvent{}

	for _, status := range statuses {
		if status.Profile.Tool != model.ToolCodex || status.State.AuthStatus != model.AuthStatusActive {
			continue
		}
		events = append(events, watcher.collectWindowThresholdEvents(status, "5H", profileStatusPrimary(status), now)...)
		events = append(events, watcher.collectWindowThresholdEvents(status, "WEEKLY", profileStatusSecondary(status), now)...)
	}

	return events
}

func (watcher *watchNotifications) collectWindowThresholdEvents(
	status model.ProfileStatus,
	windowLabel string,
	window *model.UsageWindow,
	now time.Time,
) []thresholdEvent {
	if window == nil {
		return nil
	}

	displayWindow := service.EffectiveWindowForDisplay(window, status.State.AuthStatus, now)
	if displayWindow == nil {
		return nil
	}

	key := status.Profile.ID + "\x00" + windowLabel
	state := watcher.windows[key]
	resetKey := notificationResetKey(displayWindow)
	if state.ResetKey != resetKey {
		state = windowThresholdState{ResetKey: resetKey}
	}

	current := displayWindow.RemainingPercent
	events := []thresholdEvent{}
	if state.HasLastRemaining {
		switch {
		case !state.Notified10 && state.LastRemainingPercent > 10 && current <= 10:
			state.Notified10 = true
			state.Notified20 = true
			events = append(events, buildThresholdEvent(status, windowLabel, 10, displayWindow))
		case !state.Notified20 && state.LastRemainingPercent > 20 && current <= 20:
			state.Notified20 = true
			events = append(events, buildThresholdEvent(status, windowLabel, 20, displayWindow))
		}
	}

	state.LastRemainingPercent = current
	state.HasLastRemaining = true
	watcher.windows[key] = state
	return events
}

func buildThresholdEvent(
	status model.ProfileStatus,
	windowLabel string,
	threshold int,
	window *model.UsageWindow,
) thresholdEvent {
	account := thresholdAccountLabel(status)
	return thresholdEvent{
		Title: fmt.Sprintf("Codex account reached %d%%", threshold),
		Message: fmt.Sprintf(
			"Account %s da cham toi %d%% %s (%s)",
			account,
			threshold,
			windowLabel,
			service.FormatWindowSummary(window),
		),
	}
}

func thresholdAccountLabel(status model.ProfileStatus) string {
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

func notificationResetKey(window *model.UsageWindow) string {
	if window == nil || window.ResetsAt == nil {
		return ""
	}
	return window.ResetsAt.UTC().Format(time.RFC3339)
}
