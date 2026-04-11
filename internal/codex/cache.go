package codex

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"codex-lover/internal/model"
)

func CacheHomeAuth(cacheRoot string, profileID string, homePath string) error {
	source := filepath.Join(homePath, authFileName)
	target := cachedAuthPath(cacheRoot, profileID)
	data, err := os.ReadFile(source)
	if err != nil {
		return fmt.Errorf("read Codex auth for cache: %w", err)
	}
	if err := os.MkdirAll(filepath.Dir(target), 0o700); err != nil {
		return fmt.Errorf("create Codex auth cache dir: %w", err)
	}
	if err := os.WriteFile(target, data, 0o600); err != nil {
		return fmt.Errorf("write Codex auth cache: %w", err)
	}
	return nil
}

func HasCachedHomeAuth(cacheRoot string, profileID string) bool {
	_, err := os.Stat(cachedAuthPath(cacheRoot, profileID))
	return err == nil
}

func RestoreCachedHomeAuth(cacheRoot string, profileID string, homePath string) error {
	source := cachedAuthPath(cacheRoot, profileID)
	target := filepath.Join(homePath, authFileName)
	data, err := os.ReadFile(source)
	if err != nil {
		return fmt.Errorf("read cached Codex auth: %w", err)
	}
	if err := os.MkdirAll(homePath, 0o700); err != nil {
		return fmt.Errorf("create Codex home: %w", err)
	}
	if _, err := os.Stat(target); err == nil {
		backupPath := target + ".bak-" + time.Now().Format("20060102-150405")
		current, err := os.ReadFile(target)
		if err != nil {
			return fmt.Errorf("read current Codex auth for backup: %w", err)
		}
		if err := os.WriteFile(backupPath, current, 0o600); err != nil {
			return fmt.Errorf("backup current Codex auth: %w", err)
		}
	}
	if err := os.WriteFile(target, data, 0o600); err != nil {
		return fmt.Errorf("restore Codex auth: %w", err)
	}
	return nil
}

func DeleteCachedHomeAuth(cacheRoot string, profileID string) error {
	target := cachedAuthPath(cacheRoot, profileID)
	if err := os.Remove(target); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("delete cached Codex auth: %w", err)
	}
	return nil
}

func LoadCachedProfileAuth(cacheRoot string, profileID string) (*ProfileAuth, error) {
	authPath := cachedAuthPath(cacheRoot, profileID)
	data, err := os.ReadFile(authPath)
	if err != nil {
		return nil, fmt.Errorf("read cached Codex auth: %w", err)
	}

	var authFile AuthFile
	if err := json.Unmarshal(data, &authFile); err != nil {
		return nil, fmt.Errorf("parse cached Codex auth: %w", err)
	}
	if authFile.Tokens == nil || authFile.Tokens.AccessToken == "" {
		return nil, fmt.Errorf("%s does not contain ChatGPT tokens", authPath)
	}

	claims, err := ParseIDClaims(authFile.Tokens.IDToken)
	if err != nil {
		return nil, err
	}

	accountID := authFile.Tokens.AccountID
	if accountID == "" {
		accountID = claims.ChatGPTAccountID
	}

	return &ProfileAuth{
		HomePath:     filepath.Dir(authPath),
		AccessToken:  authFile.Tokens.AccessToken,
		RefreshToken: authFile.Tokens.RefreshToken,
		AccountID:    accountID,
		Email:        claims.Email,
		Plan:         claims.ChatGPTPlanType,
	}, nil
}

func FetchUsageFromCachedAuth(cacheRoot string, profileID string) (*model.UsageSnapshot, *ProfileAuth, error) {
	authPath := cachedAuthPath(cacheRoot, profileID)
	auth, err := LoadCachedProfileAuth(cacheRoot, profileID)
	if err != nil {
		return nil, nil, err
	}

	snapshot, authFile, err := fetchUsageWithRefresh(auth)
	if err != nil {
		return nil, nil, err
	}
	if authFile != nil {
		if err := persistRefreshedTokensAtPath(authPath, authFile); err != nil {
			return nil, nil, err
		}
	}
	return snapshot, auth, nil
}

func cachedAuthPath(cacheRoot string, profileID string) string {
	return filepath.Join(cacheRoot, safeCacheName(profileID)+".json")
}

func safeCacheName(value string) string {
	value = strings.TrimSpace(strings.ToLower(value))
	if value == "" {
		return "unknown"
	}
	re := regexp.MustCompile(`[^a-z0-9._-]+`)
	value = re.ReplaceAllString(value, "-")
	value = strings.Trim(value, "-")
	if value == "" {
		return "unknown"
	}
	return value
}
