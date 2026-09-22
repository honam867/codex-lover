package service

import (
	"testing"

	"codex-lover/internal/model"
)

func statusWithWindows(primary, secondary *model.UsageWindow) model.ProfileStatus {
	return model.ProfileStatus{
		Profile: model.Profile{ID: "a", Tool: model.ToolCodex},
		State: model.ProfileState{
			AuthStatus: model.AuthStatusActive,
			Usage:      &model.UsageSnapshot{Primary: primary, Secondary: secondary},
		},
	}
}

func TestFiveHourRemainingReadsPrimaryWithSecondaryFallback(t *testing.T) {
	st := statusWithWindows(&model.UsageWindow{RemainingPercent: 73}, &model.UsageWindow{RemainingPercent: 20})
	if got := fiveHourRemaining(st); got != 73 {
		t.Fatalf("fiveHourRemaining = %v, want 73", got)
	}
	// falls back to Secondary if Primary is nil
	fallback := statusWithWindows(nil, &model.UsageWindow{RemainingPercent: 40})
	if got := fiveHourRemaining(fallback); got != 40 {
		t.Fatalf("fiveHourRemaining fallback = %v, want 40", got)
	}
	if got := fiveHourRemaining(model.ProfileStatus{}); got != 0 {
		t.Fatalf("fiveHourRemaining nil-usage = %v, want 0", got)
	}
}

func TestWeeklyRemainingReadsSecondaryWithPrimaryFallback(t *testing.T) {
	st := statusWithWindows(&model.UsageWindow{RemainingPercent: 20}, &model.UsageWindow{RemainingPercent: 73})
	if got := weeklyRemaining(st); got != 73 {
		t.Fatalf("weeklyRemaining = %v, want 73", got)
	}
	// falls back to Primary if Secondary is nil
	fallback := statusWithWindows(&model.UsageWindow{RemainingPercent: 40}, nil)
	if got := weeklyRemaining(fallback); got != 40 {
		t.Fatalf("weeklyRemaining fallback = %v, want 40", got)
	}
	if got := weeklyRemaining(model.ProfileStatus{}); got != 0 {
		t.Fatalf("weeklyRemaining nil-usage = %v, want 0", got)
	}
}
