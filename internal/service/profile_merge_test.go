package service

import (
	"testing"
	"time"

	"codex-lover/internal/model"
)

func TestMergeCanonicalProfilePreservesPrice(t *testing.T) {
	now := time.Now()
	// canonical has no price, duplicate has one -> take duplicate's
	got := mergeCanonicalProfile(model.Profile{ID: "a", Price: 0}, model.Profile{ID: "b", Price: 300000}, now)
	if got.Price != 300000 {
		t.Fatalf("expected duplicate price 300000 carried over, got %d", got.Price)
	}
	// canonical has a price -> keep it (do not overwrite with duplicate's)
	got2 := mergeCanonicalProfile(model.Profile{ID: "a", Price: 500000}, model.Profile{ID: "b", Price: 300000}, now)
	if got2.Price != 500000 {
		t.Fatalf("expected canonical price 500000 preserved, got %d", got2.Price)
	}
}

func TestMergeCanonicalProfilePreservesBlocked(t *testing.T) {
	cases := []struct {
		name      string
		canonical bool
		duplicate bool
		want      bool
	}{
		{name: "neither blocked", want: false},
		{name: "canonical blocked", canonical: true, want: true},
		{name: "duplicate blocked", duplicate: true, want: true},
		{name: "both blocked", canonical: true, duplicate: true, want: true},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := mergeCanonicalProfile(
				model.Profile{ID: "canonical", Blocked: tc.canonical},
				model.Profile{ID: "duplicate", Blocked: tc.duplicate},
				time.Now().UTC(),
			)
			if got.Blocked != tc.want {
				t.Fatalf("Blocked = %v, want %v", got.Blocked, tc.want)
			}
		})
	}
}
