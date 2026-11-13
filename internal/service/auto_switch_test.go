package service

import (
	"testing"

	"codex-lover/internal/model"
)

func autoSwitchCase(t *testing.T, activeLeft float64, others map[string]float64) SwitchResult {
	t.Helper()
	svc, st, home := newBlockTestService(t)
	statuses := []model.ProfileStatus{blockTestStatus("active", home, model.AuthStatusActive, activeLeft, false)}
	for id, left := range others {
		statuses = append(statuses, blockTestStatus(id, home, model.AuthStatusLoggedOut, left, false))
		writeBlockTestCache(t, st, id, id+"-auth")
	}
	result, err := svc.AutoSwitchLimitedCodex(statuses, 5)
	if err != nil {
		t.Fatalf("AutoSwitchLimitedCodex: %v", err)
	}
	return result
}

func TestAutoSwitchBeforeQuotaRunsOut(t *testing.T) {
	// 4% left is under the 5% threshold: move now, before requests fail.
	if r := autoSwitchCase(t, 4, map[string]float64{"low": 3, "good": 60}); !r.Changed || r.To.ID != "good" {
		t.Fatalf("got %+v, want switch to good", r)
	}
	// Above the threshold: stay.
	if r := autoSwitchCase(t, 30, map[string]float64{"good": 90}); r.Changed {
		t.Fatalf("switched with 30%% left: %+v", r)
	}
}

func TestAutoSwitchNoPingPongBetweenLowAccounts(t *testing.T) {
	// Active is low but not out, the only other account is lower than the
	// threshold too: keep the active one instead of hopping.
	if r := autoSwitchCase(t, 4, map[string]float64{"low": 3}); r.Changed {
		t.Fatalf("hopped to another low account: %+v", r)
	}
	// Once the active one is fully out, any account with quota beats it.
	if r := autoSwitchCase(t, 0, map[string]float64{"low": 3}); !r.Changed || r.To.ID != "low" {
		t.Fatalf("got %+v, want switch to low when active is exhausted", r)
	}
}

func TestAutoSwitchKeepsActiveWhenAllExhausted(t *testing.T) {
	if r := autoSwitchCase(t, 0, map[string]float64{"a": 0, "b": 0}); r.Changed {
		t.Fatalf("switched although every account is out: %+v", r)
	}
	// Later one account's quota comes back: the next refresh moves to it.
	if r := autoSwitchCase(t, 0, map[string]float64{"a": 0, "b": 100}); !r.Changed || r.To.ID != "b" {
		t.Fatalf("got %+v, want switch to recovered b", r)
	}
}
