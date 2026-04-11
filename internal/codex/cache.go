package codex

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"
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
