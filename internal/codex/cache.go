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

// CacheHomeAuth copies the home auth.json into profileID's cache. When an
// expected account ID or email is given, the copy is refused if the home file
// now holds a different account (it was switched while we were working), so
// one profile's cache never ends up with another account's tokens.
func CacheHomeAuth(cacheRoot string, profileID string, homePath string, expectAccountID string, expectEmail string) error {
	source := filepath.Join(homePath, authFileName)
	target := cachedAuthPath(cacheRoot, profileID)
	data, err := os.ReadFile(source)
	if err != nil {
		return fmt.Errorf("read Codex auth for cache: %w", err)
	}
	if expectAccountID != "" || expectEmail != "" {
		accountID, email, err := authIdentity(data)
		if err != nil {
			return err
		}
		if !SameIdentity(accountID, email, expectAccountID, expectEmail) {
			return fmt.Errorf("home Codex auth belongs to %s, not %s; cache not updated", email, expectEmail)
		}
	}
	if err := os.MkdirAll(filepath.Dir(target), 0o700); err != nil {
		return fmt.Errorf("create Codex auth cache dir: %w", err)
	}
	if err := os.WriteFile(target, data, 0o600); err != nil {
		return fmt.Errorf("write Codex auth cache: %w", err)
	}
	return nil
}

func authIdentity(data []byte) (string, string, error) {
	var authFile AuthFile
	if err := json.Unmarshal(data, &authFile); err != nil {
		return "", "", fmt.Errorf("parse Codex auth: %w", err)
	}
	if authFile.Tokens == nil {
		return "", "", fmt.Errorf("Codex auth has no tokens")
	}
	claims, err := ParseIDClaims(authFile.Tokens.IDToken)
	if err != nil {
		return "", "", err
	}
	return chooseNonEmpty(authFile.Tokens.AccountID, claims.ChatGPTAccountID), claims.Email, nil
}

// SameIdentity reports whether two (accountID, email) pairs name the same
// account. Unknown (empty) values on either side are treated as a match.
func SameIdentity(accountID, email, expectAccountID, expectEmail string) bool {
	// Email first: team workspaces share one account ID across several users.
	if email != "" && expectEmail != "" {
		return strings.EqualFold(email, expectEmail)
	}
	if accountID != "" && expectAccountID != "" {
		return strings.EqualFold(accountID, expectAccountID)
	}
	return true
}

// QuarantineCachedAuth renames a bad cache file out of the way so it is no
// longer used, keeping it on disk for manual recovery.
func QuarantineCachedAuth(cacheRoot string, profileID string) error {
	path := cachedAuthPath(cacheRoot, profileID)
	return os.Rename(path, path+".mismatch-"+time.Now().Format("20060102-150405"))
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

func MoveCachedHomeAuth(cacheRoot string, sourceProfileID string, targetProfileID string) error {
	source := cachedAuthPath(cacheRoot, sourceProfileID)
	target := cachedAuthPath(cacheRoot, targetProfileID)

	data, err := os.ReadFile(source)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return fmt.Errorf("read cached Codex auth: %w", err)
	}
	if err := os.MkdirAll(filepath.Dir(target), 0o700); err != nil {
		return fmt.Errorf("create Codex auth cache dir: %w", err)
	}
	if err := os.WriteFile(target, data, 0o600); err != nil {
		return fmt.Errorf("write moved Codex auth cache: %w", err)
	}
	if err := os.Remove(source); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("remove old cached Codex auth: %w", err)
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
