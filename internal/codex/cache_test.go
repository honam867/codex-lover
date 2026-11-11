package codex

import (
	"encoding/base64"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func writeHomeAuth(t *testing.T, home, email, accountID string) {
	t.Helper()
	payload := base64.RawURLEncoding.EncodeToString([]byte(`{"email":"` + email + `"}`))
	data := `{"tokens":{"id_token":"x.` + payload + `.y","access_token":"a","refresh_token":"r","account_id":"` + accountID + `"}}`
	if err := os.WriteFile(filepath.Join(home, authFileName), []byte(data), 0o600); err != nil {
		t.Fatal(err)
	}
}

// The home auth was switched to another account mid-refresh: caching it under
// the old profile must be refused, or that profile logs in the wrong account.
func TestCacheHomeAuthRefusesOtherAccount(t *testing.T) {
	home, cache := t.TempDir(), t.TempDir()
	writeHomeAuth(t, home, "b@x.com", "acct-b")

	if err := CacheHomeAuth(cache, "profile-a", home, "acct-a", "a@x.com"); err == nil {
		t.Fatal("expected refusal when home auth belongs to another account")
	}
	if HasCachedHomeAuth(cache, "profile-a") {
		t.Fatal("cache must not be written for a mismatched account")
	}
	if err := CacheHomeAuth(cache, "profile-b", home, "acct-b", "B@x.com"); err != nil {
		t.Fatalf("matching account should cache: %v", err)
	}
	auth, err := LoadCachedProfileAuth(cache, "profile-b")
	if err != nil || auth.Email != "b@x.com" {
		t.Fatalf("cached auth = %+v, %v", auth, err)
	}
	if !SameIdentity("shared", "a@x.com", "shared", "b@x.com") {
		return
	}
	t.Fatal("same team account ID with different emails must not match")
}

func TestToUsageWindowDropsPastResetTime(t *testing.T) {
	past := &rateLimitWindowRaw{UsedPercent: 100, ResetAt: int(time.Now().Add(-time.Hour).Unix())}
	if w := toUsageWindow(past); w.ResetsAt != nil || w.RemainingPercent != 0 {
		t.Fatalf("past reset must be dropped so 0%% is not shown as full: %+v", w)
	}
	future := &rateLimitWindowRaw{UsedPercent: 10, ResetAt: int(time.Now().Add(time.Hour).Unix())}
	if w := toUsageWindow(future); w.ResetsAt == nil {
		t.Fatal("future reset must be kept")
	}
}
