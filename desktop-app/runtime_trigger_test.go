package main

import (
	"testing"

	"codex-lover/internal/model"
)

func TestCountOpenedResults(t *testing.T) {
	run := model.TriggerRun{Results: []model.TriggerAccountResult{
		{Status: model.TriggerStatusOpened},
		{Status: model.TriggerStatusError},
		{Status: model.TriggerStatusOpened},
		{Status: model.TriggerStatusSkippedNoAuth},
	}}
	if got := countOpenedResults(run); got != 2 {
		t.Fatalf("countOpenedResults = %d, want 2", got)
	}
}
