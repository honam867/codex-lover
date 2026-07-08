package store

import "testing"

func TestDefaultConfigTriggerDefaults(t *testing.T) {
	cfg := DefaultConfig()
	if cfg.Trigger.Enabled {
		t.Fatalf("expected trigger disabled by default")
	}
	if cfg.Trigger.TimeOfDay != "08:00" {
		t.Fatalf("time_of_day = %q, want 08:00", cfg.Trigger.TimeOfDay)
	}
	if cfg.Trigger.Mode != "all" {
		t.Fatalf("mode = %q, want all", cfg.Trigger.Mode)
	}
	if cfg.Trigger.Count != 2 {
		t.Fatalf("count = %d, want 2", cfg.Trigger.Count)
	}
	if cfg.Trigger.GraceMins != 60 {
		t.Fatalf("grace = %d, want 60", cfg.Trigger.GraceMins)
	}
}
