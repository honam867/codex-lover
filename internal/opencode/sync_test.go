package opencode

import (
	"encoding/base64"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"codex-lover/internal/codex"
)

func TestSyncOpenAIFromCodexWritesOpenAISection(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	auth := &codex.ProfileAuth{
		AccessToken:  testJWTWithExp(1893456000),
		RefreshToken: "refresh-token",
		AccountID:    "acct-123",
	}

	result, err := SyncOpenAIFromCodex(auth)
	if err != nil {
		t.Fatalf("sync opencode auth: %v", err)
	}
	if !result.Changed {
		t.Fatal("expected sync to write OpenCode auth")
	}

	written := loadWrittenOpenCodeAuth(t, filepath.Join(home, ".local", "share", "opencode", "auth.json"))
	openai := objectValue(written["openai"])
	if got := stringValue(openai["type"]); got != "oauth" {
		t.Fatalf("expected openai.type oauth, got %q", got)
	}
	if got := stringValue(openai["access"]); got != auth.AccessToken {
		t.Fatalf("expected access token to be synced, got %q", got)
	}
	if got := stringValue(openai["refresh"]); got != auth.RefreshToken {
		t.Fatalf("expected refresh token to be synced, got %q", got)
	}
	if got := stringValue(openai["accountId"]); got != auth.AccountID {
		t.Fatalf("expected account id %q, got %q", auth.AccountID, got)
	}
	if got := int64Value(openai["expires"]); got != 1893456000*1000 {
		t.Fatalf("expected expires to come from access token, got %d", got)
	}
}

func TestSyncOpenAIFromCodexNoChangeWhenAlreadySynced(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	authPath := filepath.Join(home, ".local", "share", "opencode", "auth.json")
	if err := os.MkdirAll(filepath.Dir(authPath), 0o700); err != nil {
		t.Fatalf("create auth dir: %v", err)
	}

	accessToken := testJWTWithExp(1893456000)
	current := map[string]any{
		"openai": map[string]any{
			"type":      "oauth",
			"access":    accessToken,
			"refresh":   "refresh-token",
			"accountId": "acct-123",
			"expires":   int64(1893456000 * 1000),
		},
	}
	data, err := json.MarshalIndent(current, "", "  ")
	if err != nil {
		t.Fatalf("marshal current auth: %v", err)
	}
	data = append(data, '\n')
	if err := os.WriteFile(authPath, data, 0o600); err != nil {
		t.Fatalf("write auth file: %v", err)
	}

	result, err := SyncOpenAIFromCodex(&codex.ProfileAuth{
		AccessToken:  accessToken,
		RefreshToken: "refresh-token",
		AccountID:    "acct-123",
	})
	if err != nil {
		t.Fatalf("sync existing auth: %v", err)
	}
	if result.Changed {
		t.Fatal("expected sync to be a no-op when OpenCode auth already matches")
	}

	entries, err := os.ReadDir(filepath.Dir(authPath))
	if err != nil {
		t.Fatalf("read auth dir: %v", err)
	}
	if len(entries) != 1 || entries[0].Name() != "auth.json" {
		t.Fatalf("expected no backup file on no-op sync, got %#v", entries)
	}
}

func loadWrittenOpenCodeAuth(t *testing.T, path string) map[string]any {
	t.Helper()

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read OpenCode auth: %v", err)
	}
	var out map[string]any
	if err := json.Unmarshal(data, &out); err != nil {
		t.Fatalf("parse OpenCode auth: %v", err)
	}
	return out
}

func testJWTWithExp(exp int64) string {
	header, _ := json.Marshal(map[string]any{"alg": "none", "typ": "JWT"})
	payload, _ := json.Marshal(map[string]any{"exp": exp})
	return base64.RawURLEncoding.EncodeToString(header) + "." + base64.RawURLEncoding.EncodeToString(payload) + ".sig"
}
