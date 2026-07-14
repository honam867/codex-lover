package main

import (
	"testing"

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
