package app

import (
	"strings"
	"testing"
	"time"

	"codex-lover/internal/model"
)

func TestWatchNotificationsThresholds(t *testing.T) {
	watcher := newWatchNotifications()
	resetAt := time.Date(2026, 4, 11, 17, 27, 0, 0, time.UTC)

	events := watcher.collectThresholdEvents([]model.ProfileStatus{testActiveStatus("tester@example.com", 30, 40, resetAt)})
	if len(events) != 0 {
		t.Fatalf("expected no events on first sample, got %d", len(events))
	}

	events = watcher.collectThresholdEvents([]model.ProfileStatus{testActiveStatus("tester@example.com", 19, 40, resetAt)})
	if len(events) != 1 || events[0].Title != "Codex account reached 20%" {
		t.Fatalf("expected one 20%% event, got %#v", events)
	}

	events = watcher.collectThresholdEvents([]model.ProfileStatus{testActiveStatus("tester@example.com", 9, 40, resetAt)})
	if len(events) != 1 || events[0].Title != "Codex account reached 10%" {
		t.Fatalf("expected one 10%% event, got %#v", events)
	}

	events = watcher.collectThresholdEvents([]model.ProfileStatus{testActiveStatus("tester@example.com", 8, 40, resetAt)})
	if len(events) != 0 {
		t.Fatalf("expected no duplicate event below 10%%, got %#v", events)
	}

	nextReset := resetAt.Add(5 * time.Hour)
	events = watcher.collectThresholdEvents([]model.ProfileStatus{testActiveStatus("tester@example.com", 100, 100, nextReset)})
	if len(events) != 0 {
		t.Fatalf("expected no event on reset cycle seed, got %#v", events)
	}

	events = watcher.collectThresholdEvents([]model.ProfileStatus{testActiveStatus("tester@example.com", 18, 100, nextReset)})
	if len(events) != 1 || events[0].Title != "Codex account reached 20%" {
		t.Fatalf("expected 20%% event after reset cycle, got %#v", events)
	}
}

func TestWatchNotificationsWeeklyThresholds(t *testing.T) {
	watcher := newWatchNotifications()
	resetAt := time.Date(2026, 4, 11, 17, 27, 0, 0, time.UTC)

	events := watcher.collectThresholdEvents([]model.ProfileStatus{testActiveStatus("tester@example.com", 100, 30, resetAt)})
	if len(events) != 0 {
		t.Fatalf("expected no events on first weekly sample, got %d", len(events))
	}

	events = watcher.collectThresholdEvents([]model.ProfileStatus{testActiveStatus("tester@example.com", 100, 19, resetAt)})
	if len(events) != 1 || events[0].Title != "Codex account reached 20%" || !contains(events[0].Message, "WEEKLY") {
		t.Fatalf("expected one weekly 20%% event, got %#v", events)
	}

	events = watcher.collectThresholdEvents([]model.ProfileStatus{testActiveStatus("tester@example.com", 100, 9, resetAt)})
	if len(events) != 1 || events[0].Title != "Codex account reached 10%" || !contains(events[0].Message, "WEEKLY") {
		t.Fatalf("expected one weekly 10%% event, got %#v", events)
	}
}

func testActiveStatus(email string, primaryRemaining float64, secondaryRemaining float64, resetAt time.Time) model.ProfileStatus {
	secondaryReset := resetAt.Add(7 * 24 * time.Hour)
	return model.ProfileStatus{
		Profile: model.Profile{
			ID:    "codex-test",
			Tool:  model.ToolCodex,
			Email: email,
		},
		State: model.ProfileState{
			AuthStatus: model.AuthStatusActive,
			Usage: &model.UsageSnapshot{
				Primary: &model.UsageWindow{
					RemainingPercent: primaryRemaining,
					ResetsAt:         &resetAt,
				},
				Secondary: &model.UsageWindow{
					RemainingPercent: secondaryRemaining,
					ResetsAt:         &secondaryReset,
				},
			},
		},
	}
}

func contains(value string, pattern string) bool {
	return strings.Contains(value, pattern)
}
