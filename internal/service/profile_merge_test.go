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
