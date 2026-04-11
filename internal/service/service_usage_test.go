package service

import (
	"testing"
	"time"

	"codex-lover/internal/model"
)

func TestUsageLimitReachedOnWeeklyWindow(t *testing.T) {
	status := model.ProfileStatus{
		Profile: model.Profile{Tool: model.ToolCodex},
		State: model.ProfileState{
			Usage: &model.UsageSnapshot{
				Primary: &model.UsageWindow{
					RemainingPercent: 80,
				},
				Secondary: &model.UsageWindow{
					RemainingPercent: 0.5,
				},
			},
		},
	}

	if !usageLimitReached(status) {
		t.Fatal("expected weekly window to trigger limit reached")
	}
}

func TestQuotaScoreRejectsWeeklyLimitedCandidate(t *testing.T) {
	now := time.Now()
	status := model.ProfileStatus{
		Profile: model.Profile{Tool: model.ToolCodex},
		State: model.ProfileState{
			AuthStatus: model.AuthStatusLoggedOut,
			Usage: &model.UsageSnapshot{
				Primary: &model.UsageWindow{
					RemainingPercent: 80,
				},
				Secondary: &model.UsageWindow{
					RemainingPercent: 0.4,
				},
			},
		},
	}

	if score, ok := quotaScore(status, now); ok || score != 0 {
		t.Fatalf("expected weekly-limited candidate to be rejected, got score=%v ok=%v", score, ok)
	}
}
