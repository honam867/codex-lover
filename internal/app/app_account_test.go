package app

import (
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"codex-lover/internal/model"
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

func TestWindowDisplayLabelUsesKnownWindows(t *testing.T) {
	if got := windowDisplayLabel(&model.UsageWindow{WindowMinutes: 5 * 60}, "primary"); got != "5h" {
		t.Fatalf("expected 5h label, got %q", got)
	}
	if got := windowDisplayLabel(&model.UsageWindow{WindowMinutes: 7 * 24 * 60}, "primary"); got != "weekly" {
		t.Fatalf("expected weekly label, got %q", got)
	}
	if got := windowDisplayLabel(&model.UsageWindow{WindowMinutes: 24 * 60}, "primary"); got != "1d" {
		t.Fatalf("expected 1d label, got %q", got)
	}
}

func TestPrintAccountListAlwaysShowsWeekly(t *testing.T) {
	tests := []struct {
		name       string
		authStatus string
		wantLine   string
	}{
		{
			name:       "active account without weekly window",
			authStatus: model.AuthStatusActive,
			wantLine:   "weekly: unavailable",
		},
		{
			name:       "logged out account without weekly window",
			authStatus: model.AuthStatusLoggedOut,
			wantLine:   "weekly: no cached usage",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			output := stripANSICodes(captureStdout(t, func() {
				printAccountList([]model.ProfileStatus{{
					Profile: model.Profile{
						ID:    "codex-test",
						Tool:  model.ToolCodex,
						Email: "tester@example.com",
					},
					State: model.ProfileState{
						AuthStatus: tt.authStatus,
						Usage: &model.UsageSnapshot{
							Primary: &model.UsageWindow{WindowMinutes: 5 * 60, RemainingPercent: 80},
						},
					},
				}}, nil)
			}))

			if !strings.Contains(output, tt.wantLine) {
				t.Fatalf("expected output to contain %q, got %q", tt.wantLine, output)
			}
		})
	}
}

func TestPrintStatusesAlwaysShowsWeekly(t *testing.T) {
	output := stripANSICodes(captureStdout(t, func() {
		printStatuses([]model.ProfileStatus{{
			Profile: model.Profile{
				ID:    "codex-test",
				Tool:  model.ToolCodex,
				Email: "tester@example.com",
			},
			State: model.ProfileState{
				AuthStatus: model.AuthStatusLoggedOut,
				Usage: &model.UsageSnapshot{
					Primary: &model.UsageWindow{WindowMinutes: 5 * 60, RemainingPercent: 80},
				},
			},
		}}, nil)
	}))

	if !strings.Contains(output, "weekly: no cached usage") {
		t.Fatalf("expected output to contain weekly line, got %q", output)
	}
}

func TestRenderWatchAlwaysShowsWeekly(t *testing.T) {
	t.Setenv("TERM", "xterm-256color")
	output := stripANSICodes(captureStdout(t, func() {
		renderWatch([]model.ProfileStatus{{
			Profile: model.Profile{
				ID:    "codex-test",
				Tool:  model.ToolCodex,
				Email: "tester@example.com",
			},
			State: model.ProfileState{
				AuthStatus: model.AuthStatusLoggedOut,
				Usage: &model.UsageSnapshot{
					Primary: &model.UsageWindow{WindowMinutes: 5 * 60, RemainingPercent: 80},
				},
			},
		}}, nil)
	}))

	if !strings.Contains(output, "weekly: no cached usage") {
		t.Fatalf("expected watch output to contain weekly line, got %q", output)
	}
}

func captureStdout(t *testing.T, fn func()) string {
	t.Helper()

	originalStdout := os.Stdout
	reader, writer, err := os.Pipe()
	if err != nil {
		t.Fatalf("create pipe: %v", err)
	}

	os.Stdout = writer
	defer func() {
		os.Stdout = originalStdout
	}()

	fn()

	if err := writer.Close(); err != nil {
		t.Fatalf("close writer: %v", err)
	}

	output, err := io.ReadAll(reader)
	if err != nil {
		t.Fatalf("read output: %v", err)
	}
	if err := reader.Close(); err != nil {
		t.Fatalf("close reader: %v", err)
	}
	return string(output)
}

func stripANSICodes(value string) string {
	ansiPattern := regexp.MustCompile(`\x1b\[[0-9;]*m`)
	return ansiPattern.ReplaceAllString(value, "")
}
