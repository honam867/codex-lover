package daemon

import (
	"fmt"
	"time"

	"codex-lover/internal/model"
	"codex-lover/internal/service"
)

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

// thresholdNotifier tracks per-window usage and reports when the active account
// crosses the 20% / 10% remaining thresholds, re-arming after each reset.
type thresholdNotifier struct {
	windows map[string]windowThresholdState
}

func newThresholdNotifier() *thresholdNotifier {
	return &thresholdNotifier{windows: map[string]windowThresholdState{}}
}

func (n *thresholdNotifier) collect(statuses []model.ProfileStatus, now time.Time) []thresholdEvent {
	events := []thresholdEvent{}
	for _, status := range statuses {
		if status.Profile.Tool != model.ToolCodex || status.State.AuthStatus != model.AuthStatusActive {
			continue
		}
		events = append(events, n.collectWindow(status, "5H", status.State.UsageWindowPrimary(), now)...)
		events = append(events, n.collectWindow(status, "WEEKLY", status.State.UsageWindowSecondary(), now)...)
	}
	return events
}

func (n *thresholdNotifier) collectWindow(status model.ProfileStatus, label string, window *model.UsageWindow, now time.Time) []thresholdEvent {
	if window == nil {
		return nil
	}
	display := service.EffectiveWindowForDisplay(window, status.State.AuthStatus, now)
	if display == nil {
		return nil
	}

	key := status.Profile.ID + "\x00" + label
	state := n.windows[key]
	resetKey := thresholdResetKey(display)
	if state.ResetKey != resetKey {
		state = windowThresholdState{ResetKey: resetKey}
	}

	current := display.RemainingPercent
	events := []thresholdEvent{}
	if state.HasLastRemaining {
		switch {
		case !state.Notified10 && state.LastRemainingPercent > 10 && current <= 10:
			state.Notified10 = true
			state.Notified20 = true
			events = append(events, buildThresholdEvent(status, label, 10, display))
		case !state.Notified20 && state.LastRemainingPercent > 20 && current <= 20:
			state.Notified20 = true
			events = append(events, buildThresholdEvent(status, label, 20, display))
		}
	}

	state.LastRemainingPercent = current
	state.HasLastRemaining = true
	n.windows[key] = state
	return events
}

func buildThresholdEvent(status model.ProfileStatus, label string, threshold int, window *model.UsageWindow) thresholdEvent {
	return thresholdEvent{
		Title: fmt.Sprintf("Codex account reached %d%%", threshold),
		Message: fmt.Sprintf(
			"Account %s reached %d%% %s (%s)",
			thresholdAccountLabel(status),
			threshold,
			label,
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

func thresholdResetKey(window *model.UsageWindow) string {
	if window == nil || window.ResetsAt == nil {
		return ""
	}
	return window.ResetsAt.UTC().Format(time.RFC3339)
}
