package service

import (
	"testing"
	"time"

	"codex-lover/internal/model"
)

func TestUsageLimitReachedOnEitherWindow(t *testing.T) {
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
		t.Fatal("expected exhausted secondary window to trigger limit reached")
	}
}

func TestQuotaScoreRejectsCandidateLimitedOnEitherWindow(t *testing.T) {
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
		t.Fatalf("expected limited candidate to be rejected, got score=%v ok=%v", score, ok)
	}
}

func TestQuotaScoreUsesMinOfBothWindows(t *testing.T) {
	now := time.Now()
	status := model.ProfileStatus{
		Profile: model.Profile{Tool: model.ToolCodex},
		State: model.ProfileState{
			AuthStatus: model.AuthStatusLoggedOut,
			Usage: &model.UsageSnapshot{
				Primary: &model.UsageWindow{
					RemainingPercent: 60,
				},
				Secondary: &model.UsageWindow{
					RemainingPercent: 30,
				},
			},
		},
	}

	if score, ok := quotaScore(status, now); !ok || score != 30 {
		t.Fatalf("expected score=30 ok=true (min of both windows), got score=%v ok=%v", score, ok)
	}
}
