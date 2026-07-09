package service

import (
	"testing"
	"time"

	"codex-lover/internal/model"
)

func TestAppendDeletionRecordPrependsAndCaps(t *testing.T) {
	var history []model.DeletedAccountRecord
	for i := 0; i < 3; i++ {
		rec := model.DeletedAccountRecord{ProfileID: string(rune('a' + i)), DeletedAt: time.Unix(int64(i), 0)}
		history = appendDeletionRecord(history, rec, 2)
	}
	if len(history) != 2 {
		t.Fatalf("history should be capped at 2, got %d", len(history))
	}
	if history[0].ProfileID != "c" {
		t.Fatalf("newest ('c') must be first, got %q", history[0].ProfileID)
	}
	if history[1].ProfileID != "b" {
		t.Fatalf("second-newest ('b') must be second, got %q", history[1].ProfileID)
	}
}
