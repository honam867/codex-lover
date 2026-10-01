package main

import "testing"

func TestManualHoldDecision(t *testing.T) {
	tests := []struct {
		name     string
		activeID string
		limited  bool
		hold     string
		wantSkip bool
		wantHold string
	}{
		{name: "no hold: auto-switch runs", activeID: "a", limited: true, hold: "", wantSkip: false, wantHold: ""},
		{name: "held limited account stays", activeID: "a", limited: true, hold: "a", wantSkip: true, wantHold: "a"},
		{name: "held account got quota back: release", activeID: "a", limited: false, hold: "a", wantSkip: false, wantHold: ""},
		{name: "another account became active: release", activeID: "b", limited: true, hold: "a", wantSkip: false, wantHold: ""},
		{name: "no active codex account: release", activeID: "", limited: false, hold: "a", wantSkip: false, wantHold: ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			skip, hold := manualHoldDecision(tt.activeID, tt.limited, tt.hold)
			if skip != tt.wantSkip || hold != tt.wantHold {
				t.Fatalf("got skip=%v hold=%q, want skip=%v hold=%q", skip, hold, tt.wantSkip, tt.wantHold)
			}
		})
	}
}
