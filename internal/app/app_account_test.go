package app

import (
	"path/filepath"
	"testing"
)

func TestPrepareManagedCodexLoginHome(t *testing.T) {
	root := t.TempDir()

	basePath, homePath, err := prepareManagedCodexLoginHome(root)
	if err != nil {
		t.Fatalf("prepare managed home: %v", err)
	}
	if filepath.Dir(homePath) != basePath {
		t.Fatalf("expected home parent %q, got %q", basePath, filepath.Dir(homePath))
	}
	if filepath.Base(homePath) != ".codex" {
		t.Fatalf("expected .codex dir, got %q", homePath)
	}
}

func TestSetEnvValueReplacesExistingEntry(t *testing.T) {
	env := []string{"HOME=old", "PATH=x"}
	updated := setEnvValue(env, "HOME", "new")
	if updated[0] != "HOME=new" {
		t.Fatalf("expected HOME to be replaced, got %#v", updated)
	}
}
