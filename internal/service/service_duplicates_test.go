package service

import (
	"testing"
	"time"

	"codex-lover/internal/model"
)

func TestProfileIdentityKeyPrefersAccountID(t *testing.T) {
	profile := model.Profile{
		AccountID: "ABC-123",
		Email:     "user@example.com",
	}

	got := profileIdentityKey(profile)
	if got != "account:abc-123" {
		t.Fatalf("expected account identity key, got %q", got)
	}
}

func TestMergeCanonicalProfileReplacesDefaultLabel(t *testing.T) {
	now := time.Now().UTC()
	canonical := model.Profile{
		ID:        "codex-default",
		Label:     "default",
		Tool:      model.ToolCodex,
		Provider:  model.ToolCodex,
		HomePath:  `testdata\.codex`,
		CreatedAt: now,
		UpdatedAt: now,
	}
	duplicate := model.Profile{
		ID:             "codex-ec61",
		Label:          "account-example-com",
		Tool:           model.ToolCodex,
		Provider:       model.ToolCodex,
		HomePath:       `testdata\.codex`,
		Email:          "account@example.com",
		AccountID:      "ec61",
		Plan:           "plus",
		AutoDiscovered: true,
		CreatedAt:      now.Add(-time.Hour),
		UpdatedAt:      now,
	}

	merged := mergeCanonicalProfile(canonical, duplicate, now)
	if merged.Label != "account-example-com" {
		t.Fatalf("expected merged label to be updated, got %q", merged.Label)
	}
	if merged.Email != "account@example.com" {
		t.Fatalf("expected merged email, got %q", merged.Email)
	}
	if merged.AccountID != "ec61" {
		t.Fatalf("expected merged account id, got %q", merged.AccountID)
	}
	if !merged.AutoDiscovered {
		t.Fatal("expected merged profile to preserve auto discovered flag")
	}
}

func TestMergeProfileStatesPrefersActiveAndNewerUsage(t *testing.T) {
	now := time.Now().UTC()
	activeTime := now
	loggedOutTime := now.Add(-time.Hour)
	active := model.ProfileState{
		AuthStatus:       model.AuthStatusActive,
		LastRefreshedAt:  &activeTime,
		LastSeenActiveAt: &activeTime,
		Usage: &model.UsageSnapshot{
			PlanType: "plus",
		},
	}
	loggedOut := model.ProfileState{
		AuthStatus:          model.AuthStatusLoggedOut,
		LastRefreshedAt:     &loggedOutTime,
		LastSeenLoggedOutAt: &loggedOutTime,
	}

	merged := mergeProfileStates(loggedOut, active)
	if merged.AuthStatus != model.AuthStatusActive {
		t.Fatalf("expected active state to win, got %q", merged.AuthStatus)
	}
	if merged.Usage == nil || merged.Usage.PlanType != "plus" {
		t.Fatal("expected active usage to be preserved")
	}
	if merged.LastSeenLoggedOutAt == nil || !merged.LastSeenLoggedOutAt.Equal(loggedOutTime) {
		t.Fatal("expected logged out timestamp to be preserved")
	}
}
