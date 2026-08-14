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

func TestBuildSnapshotIncludesShopCustomerAndNote(t *testing.T) {
	snapshot := buildSnapshot([]model.ProfileStatus{{
		Profile: model.Profile{ID: "codex-a", Tool: model.ToolCodex, Provider: model.ToolCodex, ShopName: "Shop A", CustomerName: "Customer A", Note: "private note"},
	}}, nil)

	if len(snapshot.Profiles) != 1 {
		t.Fatalf("profiles len = %d, want 1", len(snapshot.Profiles))
	}
	card := snapshot.Profiles[0]
	if card.ShopName != "Shop A" {
		t.Fatalf("ShopName = %q, want Shop A", card.ShopName)
	}
	if card.CustomerName != "Customer A" {
		t.Fatalf("CustomerName = %q, want Customer A", card.CustomerName)
	}
	if card.Note != "private note" {
		t.Fatalf("Note = %q, want private note", card.Note)
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

func TestBuildSnapshotHealthLabel(t *testing.T) {
	snapshot := buildSnapshot([]model.ProfileStatus{
		{Profile: model.Profile{ID: "ok", Tool: model.ToolCodex, Provider: model.ToolCodex}, State: model.ProfileState{HealthStatus: model.HealthStatusOK}},
		{Profile: model.Profile{ID: "limited", Tool: model.ToolCodex, Provider: model.ToolCodex}, State: model.ProfileState{HealthStatus: model.HealthStatusLimited}},
		{Profile: model.Profile{ID: "failed", Tool: model.ToolCodex, Provider: model.ToolCodex}, State: model.ProfileState{HealthStatus: model.HealthStatusFailed}},
		{Profile: model.Profile{ID: "no-auth", Tool: model.ToolCodex, Provider: model.ToolCodex}, State: model.ProfileState{HealthStatus: model.HealthStatusNoAuth}},
		{Profile: model.Profile{ID: "unknown", Tool: model.ToolCodex, Provider: model.ToolCodex}, State: model.ProfileState{HealthStatus: model.HealthStatusUnknown}},
	}, nil)

	wants := []string{"Still Alive", "Still Alive", "Dead", "Dead", ""}
	for i, want := range wants {
		if got := snapshot.Profiles[i].HealthLabel; got != want {
			t.Fatalf("profile %d HealthLabel = %q, want %q", i, got, want)
		}
	}
}

func TestBuildSnapshotSuppressesDaysUsedForDeadCodexHealth(t *testing.T) {
	createdAt := time.Now().AddDate(0, 0, -3)
	snapshot := buildSnapshot([]model.ProfileStatus{
		{
			Profile: model.Profile{ID: "dead", Tool: model.ToolCodex, Provider: model.ToolCodex, CreatedAt: createdAt},
			State:   model.ProfileState{HealthStatus: model.HealthStatusFailed, HealthMessage: "Forbidden or blocked"},
		},
		{
			Profile: model.Profile{ID: "no-auth", Tool: model.ToolCodex, Provider: model.ToolCodex, CreatedAt: createdAt},
			State:   model.ProfileState{HealthStatus: model.HealthStatusNoAuth},
		},
		{
			Profile: model.Profile{ID: "probe-failed", Tool: model.ToolCodex, Provider: model.ToolCodex, CreatedAt: createdAt},
			State:   model.ProfileState{HealthStatus: model.HealthStatusFailed, HealthMessage: "Probe request failed"},
		},
		{
			Profile: model.Profile{ID: "limited", Tool: model.ToolCodex, Provider: model.ToolCodex, CreatedAt: createdAt},
			State:   model.ProfileState{HealthStatus: model.HealthStatusLimited},
		},
	}, nil)

	if got := snapshot.Profiles[0].DaysUsedText; got != "" {
		t.Fatalf("failed health DaysUsedText = %q, want empty", got)
	}
	if got := snapshot.Profiles[1].DaysUsedText; got != "" {
		t.Fatalf("no_auth health DaysUsedText = %q, want empty", got)
	}
	if got := snapshot.Profiles[2].DaysUsedText; got == "" {
		t.Fatalf("transient probe failure should keep counting DaysUsedText")
	}
	if got := snapshot.Profiles[3].DaysUsedText; got == "" {
		t.Fatalf("limited health should keep counting DaysUsedText")
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
