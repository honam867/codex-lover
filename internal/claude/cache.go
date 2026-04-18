package claude

import (
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
		return fmt.Errorf("read Claude auth for cache: %w", err)
	}
	if err := os.MkdirAll(filepath.Dir(target), 0o700); err != nil {
		return fmt.Errorf("create Claude auth cache dir: %w", err)
	}
	if err := os.WriteFile(target, data, 0o600); err != nil {
		return fmt.Errorf("write Claude auth cache: %w", err)
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
		return fmt.Errorf("read cached Claude auth: %w", err)
	}
	if err := os.MkdirAll(homePath, 0o700); err != nil {
		return fmt.Errorf("create Claude home: %w", err)
	}
	if _, err := os.Stat(target); err == nil {
		backupPath := target + ".bak-" + time.Now().Format("20060102-150405")
		current, err := os.ReadFile(target)
		if err != nil {
			return fmt.Errorf("read current Claude auth for backup: %w", err)
		}
		if err := os.WriteFile(backupPath, current, 0o600); err != nil {
			return fmt.Errorf("backup current Claude auth: %w", err)
		}
	}
	if err := os.WriteFile(target, data, 0o600); err != nil {
		return fmt.Errorf("restore Claude auth: %w", err)
	}
	return nil
}

func DeleteCachedHomeAuth(cacheRoot string, profileID string) error {
	target := cachedAuthPath(cacheRoot, profileID)
	if err := os.Remove(target); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("delete cached Claude auth: %w", err)
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
		return fmt.Errorf("read cached Claude auth: %w", err)
	}
	if err := os.MkdirAll(filepath.Dir(target), 0o700); err != nil {
		return fmt.Errorf("create Claude auth cache dir: %w", err)
	}
	if err := os.WriteFile(target, data, 0o600); err != nil {
		return fmt.Errorf("write moved Claude auth cache: %w", err)
	}
	if err := os.Remove(source); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("remove old cached Claude auth: %w", err)
	}
	return nil
}

func FetchUsageFromCachedAuth(cacheRoot string, profileID string) (*model.UsageSnapshot, *ProfileAuth, error) {
	auth, err := LoadCachedProfileAuth(cacheRoot, profileID)
	if err != nil {
		return nil, nil, err
	}

	snapshot, err := FetchUsage(auth)
	if err != nil {
		return nil, nil, err
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
