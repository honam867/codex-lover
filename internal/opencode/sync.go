package opencode

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"codex-lover/internal/codex"
)

type SyncResult struct {
	Changed   bool
	AccountID string
	Path      string
}

type jwtClaims struct {
	Exp int64 `json:"exp"`
}

func SyncOpenAIFromCodex(auth *codex.ProfileAuth) (SyncResult, error) {
	if auth == nil {
		return SyncResult{}, fmt.Errorf("missing Codex auth")
	}
	if strings.TrimSpace(auth.AccessToken) == "" || strings.TrimSpace(auth.RefreshToken) == "" {
		return SyncResult{}, fmt.Errorf("Codex auth does not contain OAuth tokens")
	}

	authPath, err := authPath()
	if err != nil {
		return SyncResult{}, err
	}

	current, err := loadAuth(authPath)
	if err != nil {
		return SyncResult{}, err
	}

	openai := objectValue(current["openai"])
	expires := accessTokenExpiresMillis(auth.AccessToken)
	if expires == 0 {
		expires = int64Value(openai["expires"])
	}

	nextOpenAI := cloneObject(openai)
	nextOpenAI["type"] = "oauth"
	nextOpenAI["access"] = auth.AccessToken
	nextOpenAI["refresh"] = auth.RefreshToken
	nextOpenAI["accountId"] = auth.AccountID
	if expires > 0 {
		nextOpenAI["expires"] = expires
	}

	if openAIEqual(openai, nextOpenAI) {
		return SyncResult{
			Changed:   false,
			AccountID: auth.AccountID,
			Path:      authPath,
		}, nil
	}

	if _, err := os.Stat(authPath); err == nil {
		if err := backupAuth(authPath); err != nil {
			return SyncResult{}, err
		}
	}
	current["openai"] = nextOpenAI
	if err := saveAuth(authPath, current); err != nil {
		return SyncResult{}, err
	}

	return SyncResult{
		Changed:   true,
		AccountID: auth.AccountID,
		Path:      authPath,
	}, nil
}

func authPath() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("resolve user home: %w", err)
	}
	return filepath.Join(home, ".local", "share", "opencode", "auth.json"), nil
}

func loadAuth(path string) (map[string]any, error) {
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return map[string]any{}, nil
	}
	if err != nil {
		return nil, fmt.Errorf("read OpenCode auth: %w", err)
	}
	if len(strings.TrimSpace(string(data))) == 0 {
		return map[string]any{}, nil
	}
	var out map[string]any
	if err := json.Unmarshal(data, &out); err != nil {
		return nil, fmt.Errorf("parse OpenCode auth: %w", err)
	}
	if out == nil {
		out = map[string]any{}
	}
	return out, nil
}

func saveAuth(path string, value map[string]any) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return fmt.Errorf("create OpenCode auth dir: %w", err)
	}
	data, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal OpenCode auth: %w", err)
	}
	data = append(data, '\n')
	return os.WriteFile(path, data, 0o600)
}

func backupAuth(path string) error {
	backupPath := path + ".bak-" + time.Now().Format("20060102-150405")
	data, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("read OpenCode auth for backup: %w", err)
	}
	if err := os.WriteFile(backupPath, data, 0o600); err != nil {
		return fmt.Errorf("backup OpenCode auth: %w", err)
	}
	return nil
}

func accessTokenExpiresMillis(token string) int64 {
	var claims jwtClaims
	if err := decodeJWTPayload(token, &claims); err != nil {
		return 0
	}
	if claims.Exp <= 0 {
		return 0
	}
	return claims.Exp * 1000
}

func decodeJWTPayload(token string, target any) error {
	parts := strings.Split(token, ".")
	if len(parts) < 2 {
		return fmt.Errorf("invalid jwt")
	}
	payload, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return err
	}
	return json.Unmarshal(payload, target)
}

func objectValue(value any) map[string]any {
	if typed, ok := value.(map[string]any); ok {
		return typed
	}
	return map[string]any{}
}

func cloneObject(value map[string]any) map[string]any {
	out := make(map[string]any, len(value))
	for key, item := range value {
		out[key] = item
	}
	return out
}

func openAIEqual(left map[string]any, right map[string]any) bool {
	return stringValue(left["type"]) == stringValue(right["type"]) &&
		stringValue(left["access"]) == stringValue(right["access"]) &&
		stringValue(left["refresh"]) == stringValue(right["refresh"]) &&
		stringValue(left["accountId"]) == stringValue(right["accountId"]) &&
		int64Value(left["expires"]) == int64Value(right["expires"])
}

func stringValue(value any) string {
	if typed, ok := value.(string); ok {
		return typed
	}
	return ""
}

func int64Value(value any) int64 {
	switch typed := value.(type) {
	case int64:
		return typed
	case int:
		return int64(typed)
	case float64:
		return int64(typed)
	case json.Number:
		out, _ := typed.Int64()
		return out
	default:
		return 0
	}
}
