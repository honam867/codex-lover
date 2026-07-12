package service

import (
	"testing"
	"time"

	"codex-lover/internal/model"
)

func weeklyStatus(remaining float64) model.ProfileStatus {
	return model.ProfileStatus{
		Profile: model.Profile{ID: "a", Tool: model.ToolCodex},
		State: model.ProfileState{
			AuthStatus: model.AuthStatusActive,
			Usage:      &model.UsageSnapshot{Primary: &model.UsageWindow{RemainingPercent: remaining}},
		},
	}
}

func TestWeeklyRemainingReadsSingleWindow(t *testing.T) {
	if got := weeklyRemaining(weeklyStatus(73)); got != 73 {
		t.Fatalf("weeklyRemaining = %v, want 73", got)
	}
	// falls back to Secondary if Primary is nil
	st := model.ProfileStatus{State: model.ProfileState{Usage: &model.UsageSnapshot{Secondary: &model.UsageWindow{RemainingPercent: 40}}}}
	if got := weeklyRemaining(st); got != 40 {
		t.Fatalf("weeklyRemaining fallback = %v, want 40", got)
	}
	if got := weeklyRemaining(model.ProfileStatus{}); got != 0 {
		t.Fatalf("weeklyRemaining nil-usage = %v, want 0", got)
	}
}

func TestUsageLimitReachedWeekly(t *testing.T) {
	if !usageLimitReached(weeklyStatus(0.4)) {
		t.Fatalf("0.4%% remaining should be limit-reached")
	}
	if usageLimitReached(weeklyStatus(20)) {
		t.Fatalf("20%% remaining should NOT be limit-reached")
	}
}

func TestQuotaScoreWeekly(t *testing.T) {
	score, ok := quotaScore(weeklyStatus(55), time.Now())
	if !ok || score != 55 {
		t.Fatalf("quotaScore = (%v,%v), want (55,true)", score, ok)
	}
	if _, ok := quotaScore(weeklyStatus(0.3), time.Now()); ok {
		t.Fatalf("exhausted window should be ok=false")
	}
}
