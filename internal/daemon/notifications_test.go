package daemon

import (
	"strings"
	"testing"
	"time"

	"codex-lover/internal/model"
)

func daemonTestActiveStatus(email string, primaryRemaining, secondaryRemaining float64, resetAt time.Time) model.ProfileStatus {
	secondaryReset := resetAt.Add(7 * 24 * time.Hour)
	return model.ProfileStatus{
		Profile: model.Profile{ID: "codex-test", Tool: model.ToolCodex, Email: email},
		State: model.ProfileState{
			AuthStatus: model.AuthStatusActive,
			Usage: &model.UsageSnapshot{
				Primary:   &model.UsageWindow{RemainingPercent: primaryRemaining, ResetsAt: &resetAt},
				Secondary: &model.UsageWindow{RemainingPercent: secondaryRemaining, ResetsAt: &secondaryReset},
			},
		},
	}
}

func TestThresholdNotifierPrimaryCrossings(t *testing.T) {
	n := newThresholdNotifier()
	resetAt := time.Date(2026, 4, 11, 17, 27, 0, 0, time.UTC)
	now := time.Date(2026, 4, 11, 12, 0, 0, 0, time.UTC)

	if ev := n.collect([]model.ProfileStatus{daemonTestActiveStatus("t@e.com", 30, 40, resetAt)}, now); len(ev) != 0 {
		t.Fatalf("expected no events on first sample, got %#v", ev)
	}
	if ev := n.collect([]model.ProfileStatus{daemonTestActiveStatus("t@e.com", 19, 40, resetAt)}, now); len(ev) != 1 || ev[0].Title != "Codex account reached 20%" {
		t.Fatalf("expected one 20%% event, got %#v", ev)
	}
	if ev := n.collect([]model.ProfileStatus{daemonTestActiveStatus("t@e.com", 9, 40, resetAt)}, now); len(ev) != 1 || ev[0].Title != "Codex account reached 10%" {
		t.Fatalf("expected one 10%% event, got %#v", ev)
	}
	if ev := n.collect([]model.ProfileStatus{daemonTestActiveStatus("t@e.com", 8, 40, resetAt)}, now); len(ev) != 0 {
		t.Fatalf("expected no duplicate below 10%%, got %#v", ev)
	}
}

func TestThresholdNotifierReArmsAfterReset(t *testing.T) {
	n := newThresholdNotifier()
	resetAt := time.Date(2026, 4, 11, 17, 27, 0, 0, time.UTC)
	now := time.Date(2026, 4, 11, 12, 0, 0, 0, time.UTC)

	n.collect([]model.ProfileStatus{daemonTestActiveStatus("t@e.com", 30, 40, resetAt)}, now)
	n.collect([]model.ProfileStatus{daemonTestActiveStatus("t@e.com", 9, 40, resetAt)}, now)

	nextReset := resetAt.Add(5 * time.Hour)
	now2 := resetAt.Add(1 * time.Minute)
	if ev := n.collect([]model.ProfileStatus{daemonTestActiveStatus("t@e.com", 100, 40, nextReset)}, now2); len(ev) != 0 {
		t.Fatalf("expected no event on reset seed, got %#v", ev)
	}
	if ev := n.collect([]model.ProfileStatus{daemonTestActiveStatus("t@e.com", 18, 40, nextReset)}, now2); len(ev) != 1 || ev[0].Title != "Codex account reached 20%" {
		t.Fatalf("expected 20%% event after reset, got %#v", ev)
	}
}

func TestThresholdNotifierWeekly(t *testing.T) {
	n := newThresholdNotifier()
	resetAt := time.Date(2026, 4, 11, 17, 27, 0, 0, time.UTC)
	now := time.Date(2026, 4, 11, 12, 0, 0, 0, time.UTC)

	n.collect([]model.ProfileStatus{daemonTestActiveStatus("t@e.com", 100, 30, resetAt)}, now)
	if ev := n.collect([]model.ProfileStatus{daemonTestActiveStatus("t@e.com", 100, 19, resetAt)}, now); len(ev) != 1 || !strings.Contains(ev[0].Message, "WEEKLY") {
		t.Fatalf("expected one weekly 20%% event, got %#v", ev)
	}
}

func TestThresholdNotifierIgnoresInactiveAccount(t *testing.T) {
	n := newThresholdNotifier()
	resetAt := time.Date(2026, 4, 11, 17, 27, 0, 0, time.UTC)
	now := time.Date(2026, 4, 11, 12, 0, 0, 0, time.UTC)

	s := daemonTestActiveStatus("t@e.com", 30, 40, resetAt)
	s.State.AuthStatus = model.AuthStatusLoggedOut
	n.collect([]model.ProfileStatus{s}, now)
	s.State.Usage.Primary.RemainingPercent = 9
	if ev := n.collect([]model.ProfileStatus{s}, now); len(ev) != 0 {
		t.Fatalf("expected no events for inactive account, got %#v", ev)
	}
}
