package main

import (
	"testing"
	"time"

	"codex-lover/internal/model"
)

func TestBuildSnapshotIncludesBlocked(t *testing.T) {
	snapshot := buildSnapshot([]model.ProfileStatus{{
		Profile: model.Profile{
			ID:       "codex-a",
			Label:    "codex-a",
			Tool:     model.ToolCodex,
			Provider: model.ToolCodex,
			Blocked:  true,
		},
	}}, nil)

	if len(snapshot.Profiles) != 1 || !snapshot.Profiles[0].Blocked {
		t.Fatalf("snapshot profiles = %+v, want one blocked card", snapshot.Profiles)
	}
}

func TestBuildSnapshotIncludesAudience(t *testing.T) {
	snapshot := buildSnapshot([]model.ProfileStatus{
		{Profile: model.Profile{ID: "codex-a", Tool: model.ToolCodex, Provider: model.ToolCodex}},
		{Profile: model.Profile{ID: "codex-b", Tool: model.ToolCodex, Provider: model.ToolCodex, Audience: model.ProfileAudienceCustomer}},
	}, nil)

	if got := snapshot.Profiles[0].Audience; got != model.ProfileAudiencePersonal {
		t.Fatalf("default Audience = %q, want %q", got, model.ProfileAudiencePersonal)
	}
	if got := snapshot.Profiles[1].Audience; got != model.ProfileAudienceCustomer {
		t.Fatalf("customer Audience = %q, want %q", got, model.ProfileAudienceCustomer)
	}
}

func TestBuildSnapshotIncludesHealthFields(t *testing.T) {
	checkedAt := time.Date(2026, 8, 1, 9, 30, 0, 0, time.UTC)
	snapshot := buildSnapshot([]model.ProfileStatus{{
		Profile: model.Profile{ID: "codex-a", Label: "codex-a", Tool: model.ToolCodex, Provider: model.ToolCodex},
		State: model.ProfileState{
			HealthStatus:       model.HealthStatusFailed,
			HealthMessage:      "Forbidden or blocked",
			HealthCheckedAt:    &checkedAt,
			HealthCheckedModel: "gpt-5.4-mini",
		},
	}}, nil)

	if len(snapshot.Profiles) != 1 {
		t.Fatalf("profiles len = %d, want 1", len(snapshot.Profiles))
	}
	card := snapshot.Profiles[0]
	if card.HealthStatus != model.HealthStatusFailed {
		t.Fatalf("HealthStatus = %q, want %q", card.HealthStatus, model.HealthStatusFailed)
	}
	if card.HealthMessage != "Forbidden or blocked" {
		t.Fatalf("HealthMessage = %q", card.HealthMessage)
	}
	if card.HealthCheckedAtText == "" || card.HealthCheckedAtText == "-" {
		t.Fatalf("HealthCheckedAtText should be populated, got %q", card.HealthCheckedAtText)
	}
}

func TestCodexAccountExpiryTextsUseCalendarMonth(t *testing.T) {
	createdAt := time.Date(2026, 1, 31, 8, 0, 0, 0, time.Local)
	now := time.Date(2026, 2, 27, 12, 0, 0, 0, time.Local)

	endAt, remaining := codexAccountExpiryTexts(createdAt, now)

	if endAt != "03/03/2026" {
		t.Fatalf("endAt = %q, want 03/03/2026", endAt)
	}
	if remaining != "còn 4 ngày" {
		t.Fatalf("remaining = %q, want còn 4 ngày", remaining)
	}
}

func TestCodexAccountDaysUsedText(t *testing.T) {
	createdAt := time.Date(2026, 7, 25, 8, 0, 0, 0, time.Local)
	now := time.Date(2026, 8, 2, 21, 0, 0, 0, time.Local)

	got := codexAccountDaysUsedText(createdAt, now)

	if got != "đã dùng 8 ngày" {
		t.Fatalf("days used = %q, want đã dùng 8 ngày", got)
	}
}
