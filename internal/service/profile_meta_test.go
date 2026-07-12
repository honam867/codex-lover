package service

import (
	"testing"
	"time"

	"codex-lover/internal/model"
)

func TestApplyProfileMeta(t *testing.T) {
	created := time.Date(2026, 6, 5, 0, 0, 0, 0, time.UTC)
	now := time.Date(2026, 7, 10, 8, 0, 0, 0, time.UTC)
	base := model.Profile{ID: "codex-a", Label: "a", Tool: model.ToolCodex, Email: "a@x.com"}

	got := applyProfileMeta(base, created, 300000, now)
	if !got.CreatedAt.Equal(created) {
		t.Fatalf("CreatedAt = %v, want %v", got.CreatedAt, created)
	}
	if got.Price != 300000 {
		t.Fatalf("Price = %d, want 300000", got.Price)
	}
	if !got.UpdatedAt.Equal(now) {
		t.Fatalf("UpdatedAt not set to now")
	}
	if got.Email != "a@x.com" || got.Label != "a" {
		t.Fatalf("other fields must be preserved")
	}

	// zero createdAt keeps the existing date
	withDate := model.Profile{ID: "b", CreatedAt: created}
	got2 := applyProfileMeta(withDate, time.Time{}, 500000, now)
	if !got2.CreatedAt.Equal(created) {
		t.Fatalf("zero createdAt must keep existing date, got %v", got2.CreatedAt)
	}
	if got2.Price != 500000 {
		t.Fatalf("Price = %d, want 500000", got2.Price)
	}
}
