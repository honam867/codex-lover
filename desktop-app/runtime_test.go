package main

import (
	"testing"
	"time"

	"codex-lover/internal/model"
	"codex-lover/internal/service"
)

func TestProviderUsageScheduleSkipsClaudeWithinSixtySeconds(t *testing.T) {
	schedule := newProviderUsageSchedule()
	start := time.Date(2026, 4, 17, 12, 0, 0, 0, time.UTC)

	schedule.MarkUsageAttempted(service.RefreshOptions{}, start)

	skipped := schedule.SkipUsageTools(start.Add(45 * time.Second))
	if !skipped[model.ToolClaude] {
		t.Fatalf("expected Claude usage to be skipped within 60 seconds")
	}
	if skipped[model.ToolCodex] {
		t.Fatalf("did not expect Codex usage to be skipped")
	}
}

func TestProviderUsageScheduleAllowsClaudeAfterSixtySeconds(t *testing.T) {
	schedule := newProviderUsageSchedule()
	start := time.Date(2026, 4, 17, 12, 0, 0, 0, time.UTC)

	schedule.MarkUsageAttempted(service.RefreshOptions{}, start)

	skipped := schedule.SkipUsageTools(start.Add(61 * time.Second))
	if skipped[model.ToolClaude] {
		t.Fatalf("expected Claude usage to be allowed after 60 seconds")
	}
}

func TestProviderUsageScheduleDoesNotAdvanceClaudeWhenSkipped(t *testing.T) {
	schedule := newProviderUsageSchedule()
	start := time.Date(2026, 4, 17, 12, 0, 0, 0, time.UTC)

	schedule.MarkUsageAttempted(service.RefreshOptions{}, start)
	schedule.MarkUsageAttempted(service.RefreshOptions{
		SkipUsageForTools: map[string]bool{model.ToolClaude: true},
	}, start.Add(15*time.Second))

	skipped := schedule.SkipUsageTools(start.Add(65 * time.Second))
	if skipped[model.ToolClaude] {
		t.Fatalf("expected skipped Claude refreshes to not extend the cadence window")
	}
}
