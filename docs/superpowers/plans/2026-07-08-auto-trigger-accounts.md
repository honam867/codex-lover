# Auto-Trigger Accounts Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add a Settings feature that, at a fixed daily time, sends one tiny request to a chosen set of Codex accounts (using their cached OAuth tokens, in the background) to open each account's 5-hour quota window without switching the active account.

**Architecture:** A new `internal/codex/trigger.go` posts a minimal request to the Codex `/responses` backend. A pure selector in `internal/service` chooses which accounts to trigger (all / top-N-by-weekly-quota / custom). The desktop runtime loop (`desktop-app/runtime.go`) checks a pure `shouldFireTrigger` decision on its existing 15s tick and runs the set when due. A Settings UI in `App.tsx` configures it.

**Tech Stack:** Go 1.x (stdlib `net/http`, `crypto/rand`, `testing`, `net/http/httptest`), Wails v2 bindings, React + TypeScript, lucide-react, existing cyber-CSS.

## Global Constraints

- Module path is `codex-lover`; imports use `codex-lover/internal/...`.
- OpenAI/Codex only — never trigger `claude` or `kimi` accounts.
- Background delivery only — never switch the active account to trigger.
- Never log, print, or serialize raw tokens or full auth payloads (per `AGENTS.md`).
- Config JSON uses snake_case tags (matches existing `model.Config`).
- `gofmt`/`goimports` mandatory; wrap errors with `fmt.Errorf("...: %w", err)`.
- Go tests are table-driven; no real network in unit tests (use `httptest`).
- The scheduler only fires while the desktop app is open (same constraint as notifications/auto-switch).
- Defaults: `Enabled=false`, `TimeOfDay="08:00"`, `Mode="all"`, `Count=2`, `GraceMins=60`.
- Trigger model preference order (refined by Task 3): `gpt-5.4-nano` → `gpt-5.4-mini` → `gpt-5.1-codex-mini` → `gpt-5.1-codex`.

---

## File Structure

| File | Create/Modify | Responsibility |
|---|---|---|
| `internal/model/types.go` | Modify | `TriggerConfig`, `TriggerRun`, `TriggerAccountResult`, status consts; add fields to `Config` and `State` |
| `internal/store/store.go` | Modify | `DefaultConfig` trigger defaults + backfill in `loadConfigUnlocked` |
| `internal/codex/trigger.go` | Create | `TriggerWindow`, `TriggerFromCachedAuth`, minimal `/responses` request, model fallback, refresh-on-401 |
| `internal/codex/trigger_test.go` | Create | httptest unit tests for model fallback + headers/payload |
| `internal/app/app.go` | Modify | `trigger --probe` CLI subcommand (Phase 0) + usage text |
| `internal/service/trigger_select.go` | Create | pure `selectTriggerTargets` + `shouldFireTrigger` + `parseTimeOfDay` |
| `internal/service/trigger_select_test.go` | Create | table-driven tests for selection + schedule decision |
| `internal/service/trigger.go` | Create | `RunTrigger`, `RunScheduledTrigger`, `PreviewTriggerSelection`, `LastTriggerRun`, verify helper |
| `desktop-app/app.go` | Modify | bindings: `GetTriggerSettings`, `SaveTriggerSettings`, `TriggerNow`, `PreviewTriggerSelection`, `GetLastTriggerRun`, `normalizeTriggerConfig`, `TriggerPreview` type |
| `desktop-app/runtime.go` | Modify | call `maybeRunScheduledTriggerLocked` on the 15s tick + notification |
| `desktop-app/frontend/src/App.tsx` | Modify | Settings panel: toggle, time dropdown, mode, count, compact multi-select, Trigger-now, last-run |
| `desktop-app/frontend/src/style.css` | Modify | compact card + time-select styles |

---

### Task 1: Data model + config defaults

**Files:**
- Modify: `internal/model/types.go`
- Modify: `internal/store/store.go`
- Test: `internal/store/store_trigger_test.go`

**Interfaces:**
- Produces: `model.TriggerConfig{Enabled bool; TimeOfDay string; Mode string; Count int; ProfileIDs []string; GraceMins int}`, `model.TriggerRun{RanAt time.Time; Manual bool; Results []model.TriggerAccountResult}`, `model.TriggerAccountResult{ProfileID, Label, Status, ModelUsed string; Verified bool; Error string}`; consts `TriggerModeAll/TopN/Custom`, `TriggerStatusOpened/SkippedNoAuth/NotEligible/Error`; `model.Config.Trigger`, `model.State.LastTriggerRun *TriggerRun`, `model.State.LastTriggerDate string`.

- [ ] **Step 1: Write the failing test**

Create `internal/store/store_trigger_test.go`:

```go
package store

import "testing"

func TestDefaultConfigTriggerDefaults(t *testing.T) {
	cfg := DefaultConfig()
	if cfg.Trigger.Enabled {
		t.Fatalf("expected trigger disabled by default")
	}
	if cfg.Trigger.TimeOfDay != "08:00" {
		t.Fatalf("time_of_day = %q, want 08:00", cfg.Trigger.TimeOfDay)
	}
	if cfg.Trigger.Mode != "all" {
		t.Fatalf("mode = %q, want all", cfg.Trigger.Mode)
	}
	if cfg.Trigger.Count != 2 {
		t.Fatalf("count = %d, want 2", cfg.Trigger.Count)
	}
	if cfg.Trigger.GraceMins != 60 {
		t.Fatalf("grace = %d, want 60", cfg.Trigger.GraceMins)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/store/ -run TestDefaultConfigTriggerDefaults`
Expected: FAIL — `cfg.Trigger` undefined (compile error).

- [ ] **Step 3: Add the model types**

In `internal/model/types.go`, add the consts block near the existing `ToolCodex` consts:

```go
const (
	TriggerModeAll    = "all"
	TriggerModeTopN   = "top_n"
	TriggerModeCustom = "custom"

	TriggerStatusOpened        = "opened"
	TriggerStatusSkippedNoAuth = "skipped_no_auth"
	TriggerStatusNotEligible   = "not_eligible"
	TriggerStatusError         = "error"
)
```

Add `Trigger` to `Config`:

```go
type Config struct {
	Version             int          `json:"version"`
	PollIntervalSeconds int          `json:"poll_interval_seconds"`
	Daemon              DaemonConfig `json:"daemon"`
	Profiles            []Profile    `json:"profiles"`
	AutoRotateCodex     bool         `json:"auto_rotate_codex"`
	AutoRotateThreshold float64      `json:"auto_rotate_threshold"`
	Trigger             TriggerConfig `json:"trigger"`
}

type TriggerConfig struct {
	Enabled    bool     `json:"enabled"`
	TimeOfDay  string   `json:"time_of_day"`
	Mode       string   `json:"mode"`
	Count      int      `json:"count"`
	ProfileIDs []string `json:"profile_ids"`
	GraceMins  int      `json:"grace_minutes"`
}

type TriggerRun struct {
	RanAt   time.Time              `json:"ran_at"`
	Manual  bool                   `json:"manual"`
	Results []TriggerAccountResult `json:"results"`
}

type TriggerAccountResult struct {
	ProfileID string `json:"profile_id"`
	Label     string `json:"label"`
	Status    string `json:"status"`
	ModelUsed string `json:"model_used,omitempty"`
	Verified  bool   `json:"verified"`
	Error     string `json:"error,omitempty"`
}
```

Add two fields to `State`:

```go
type State struct {
	Version         int                     `json:"version"`
	UpdatedAt       time.Time               `json:"updated_at"`
	Profiles        map[string]ProfileState `json:"profiles"`
	Sessions        []Session               `json:"sessions"`
	LastTriggerRun  *TriggerRun             `json:"last_trigger_run,omitempty"`
	LastTriggerDate string                  `json:"last_trigger_date,omitempty"`
}
```

- [ ] **Step 4: Set defaults + backfill in store**

In `internal/store/store.go`, add to the `model.Config` literal returned by `DefaultConfig()`:

```go
		Trigger: model.TriggerConfig{
			Enabled:    false,
			TimeOfDay:  "08:00",
			Mode:       model.TriggerModeAll,
			Count:      2,
			ProfileIDs: []string{},
			GraceMins:  60,
		},
```

In `loadConfigUnlocked()`, right after the `AutoRotateThreshold` backfill and before `return cfg, nil`, add:

```go
	if strings.TrimSpace(cfg.Trigger.TimeOfDay) == "" {
		cfg.Trigger.TimeOfDay = "08:00"
	}
	if strings.TrimSpace(cfg.Trigger.Mode) == "" {
		cfg.Trigger.Mode = model.TriggerModeAll
	}
	if cfg.Trigger.Count <= 0 {
		cfg.Trigger.Count = 2
	}
	if cfg.Trigger.GraceMins <= 0 {
		cfg.Trigger.GraceMins = 60
	}
```

(`strings` is already imported in store.go.)

- [ ] **Step 5: Run test to verify it passes**

Run: `go test ./internal/store/ -run TestDefaultConfigTriggerDefaults`
Expected: PASS

- [ ] **Step 6: Build the whole module + commit**

Run: `go build ./...`
Expected: no errors.

```bash
git add internal/model/types.go internal/store/store.go internal/store/store_trigger_test.go
git commit -m "feat: add trigger config + run state model"
```

---

### Task 2: Codex trigger core

**Files:**
- Create: `internal/codex/trigger.go`
- Test: `internal/codex/trigger_test.go`

**Interfaces:**
- Consumes: `ProfileAuth`, `AuthFile`, `TokenData`, `refreshAuth`, `persistRefreshedTokensAtPath`, `LoadCachedProfileAuth`, `cachedAuthPath`, `ptrTime`, `defaultUserAgent` (all existing in package `codex`).
- Produces: `codex.TriggerResult{ModelUsed string; Status int}`, `codex.DefaultTriggerModels []string`, `func TriggerWindow(auth *ProfileAuth, models []string) (*TriggerResult, *AuthFile, error)`, `func TriggerFromCachedAuth(cacheRoot, profileID string, models []string) (*TriggerResult, error)`.

- [ ] **Step 1: Write the failing test**

Create `internal/codex/trigger_test.go`:

```go
package codex

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestTriggerWindowModelFallback(t *testing.T) {
	var seenModels []string
	var gotAuth, gotAccount, gotOriginator string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		gotAccount = r.Header.Get("chatgpt-account-id")
		gotOriginator = r.Header.Get("originator")
		// First model rejected, second accepted.
		if len(seenModels) == 0 {
			seenModels = append(seenModels, "first")
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		seenModels = append(seenModels, "second")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("data: [DONE]\n"))
	}))
	defer server.Close()

	old := responsesURL
	responsesURL = server.URL
	defer func() { responsesURL = old }()

	auth := &ProfileAuth{AccessToken: "tok-abc", AccountID: "acct-1"}
	result, refreshed, err := TriggerWindow(auth, []string{"m1", "m2"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if refreshed != nil {
		t.Fatalf("did not expect a refresh")
	}
	if result.ModelUsed != "m2" {
		t.Fatalf("ModelUsed = %q, want m2", result.ModelUsed)
	}
	if gotAuth != "Bearer tok-abc" {
		t.Fatalf("Authorization = %q", gotAuth)
	}
	if gotAccount != "acct-1" {
		t.Fatalf("chatgpt-account-id = %q", gotAccount)
	}
	if gotOriginator != "codex_cli_rs" {
		t.Fatalf("originator = %q", gotOriginator)
	}
}

func TestTriggerWindowAllModelsRejected(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
	}))
	defer server.Close()
	old := responsesURL
	responsesURL = server.URL
	defer func() { responsesURL = old }()

	auth := &ProfileAuth{AccessToken: "tok", AccountID: "a"}
	if _, _, err := TriggerWindow(auth, []string{"m1", "m2"}); err == nil {
		t.Fatalf("expected error when all models rejected")
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/codex/ -run TestTriggerWindow`
Expected: FAIL — `responsesURL`, `TriggerWindow` undefined.

- [ ] **Step 3: Write the implementation**

Create `internal/codex/trigger.go`:

```go
package codex

import (
	"bytes"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"time"
)

var (
	responsesURL = "https://chatgpt.com/backend-api/codex/responses"

	// DefaultTriggerModels is the cheapest-first preference order. The first
	// model the backend accepts is used. Confirmed/adjusted by the Phase 0 probe.
	DefaultTriggerModels = []string{"gpt-5.4-nano", "gpt-5.4-mini", "gpt-5.1-codex-mini", "gpt-5.1-codex"}
)

// TriggerResult reports the outcome of a successful trigger.
type TriggerResult struct {
	ModelUsed string
	Status    int
}

// TriggerWindow sends one minimal request so the account's 5h quota window
// opens. It tries models in order until one is accepted, and refreshes the
// token once on 401. On refresh it returns a non-nil *AuthFile the caller must
// persist. The active account is never touched.
func TriggerWindow(auth *ProfileAuth, models []string) (*TriggerResult, *AuthFile, error) {
	if len(models) == 0 {
		models = DefaultTriggerModels
	}
	result, status, err := doTriggerOnce(auth.AccessToken, auth.AccountID, models)
	if err == nil {
		return result, nil, nil
	}
	if status != http.StatusUnauthorized || auth.RefreshToken == "" {
		return nil, nil, err
	}

	refreshed, rerr := refreshAuth(auth)
	if rerr != nil {
		return nil, nil, fmt.Errorf("trigger unauthorized and refresh failed: %w", rerr)
	}
	result, _, err = doTriggerOnce(refreshed.AccessToken, refreshed.AccountID, models)
	if err != nil {
		return nil, nil, err
	}

	auth.AccessToken = refreshed.AccessToken
	auth.RefreshToken = refreshed.RefreshToken
	if refreshed.AccountID != "" {
		auth.AccountID = refreshed.AccountID
	}
	return result, &AuthFile{
		Tokens: &TokenData{
			AccessToken:  refreshed.AccessToken,
			RefreshToken: refreshed.RefreshToken,
			AccountID:    refreshed.AccountID,
		},
		LastRefresh: ptrTime(time.Now().UTC()),
	}, nil
}

func doTriggerOnce(accessToken string, accountID string, models []string) (*TriggerResult, int, error) {
	var lastStatus int
	var lastErr error
	for _, modelName := range models {
		body, err := buildTriggerBody(modelName)
		if err != nil {
			return nil, 0, err
		}
		req, err := http.NewRequest(http.MethodPost, responsesURL, bytes.NewReader(body))
		if err != nil {
			return nil, 0, err
		}
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Authorization", "Bearer "+accessToken)
		req.Header.Set("User-Agent", defaultUserAgent)
		req.Header.Set("OpenAI-Beta", "responses=experimental")
		req.Header.Set("originator", "codex_cli_rs")
		req.Header.Set("session_id", newSessionID())
		if accountID != "" {
			req.Header.Set("chatgpt-account-id", accountID)
		}

		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			lastErr = fmt.Errorf("trigger request: %w", err)
			continue
		}
		_, _ = io.Copy(io.Discard, resp.Body)
		resp.Body.Close()
		lastStatus = resp.StatusCode

		if resp.StatusCode >= 200 && resp.StatusCode < 300 {
			return &TriggerResult{ModelUsed: modelName, Status: resp.StatusCode}, resp.StatusCode, nil
		}
		if resp.StatusCode == http.StatusUnauthorized {
			return nil, resp.StatusCode, errors.New("trigger unauthorized")
		}
		lastErr = fmt.Errorf("trigger failed with %d for model %s", resp.StatusCode, modelName)
	}
	if lastErr == nil {
		lastErr = errors.New("no trigger model accepted")
	}
	return nil, lastStatus, lastErr
}

func buildTriggerBody(modelName string) ([]byte, error) {
	payload := map[string]any{
		"model":        modelName,
		"instructions": "You are a status probe. Reply with a single character.",
		"input": []map[string]any{
			{
				"type": "message",
				"role": "user",
				"content": []map[string]any{
					{"type": "input_text", "text": "ok"},
				},
			},
		},
		"reasoning":         map[string]any{"effort": "minimal"},
		"store":             false,
		"stream":            true,
		"max_output_tokens": 16,
	}
	return json.Marshal(payload)
}

func newSessionID() string {
	buf := make([]byte, 16)
	if _, err := rand.Read(buf); err != nil {
		return "codex-lover-trigger"
	}
	return hex.EncodeToString(buf)
}

// TriggerFromCachedAuth loads a profile's cached auth, triggers it, and
// persists any refreshed token back to the cache file.
func TriggerFromCachedAuth(cacheRoot string, profileID string, models []string) (*TriggerResult, error) {
	authPath := cachedAuthPath(cacheRoot, profileID)
	auth, err := LoadCachedProfileAuth(cacheRoot, profileID)
	if err != nil {
		return nil, err
	}
	result, authFile, err := TriggerWindow(auth, models)
	if err != nil {
		return nil, err
	}
	if authFile != nil {
		if err := persistRefreshedTokensAtPath(authPath, authFile); err != nil {
			return nil, err
		}
	}
	return result, nil
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./internal/codex/ -run TestTriggerWindow`
Expected: PASS (both cases)

- [ ] **Step 5: Commit**

```bash
git add internal/codex/trigger.go internal/codex/trigger_test.go
git commit -m "feat: add codex trigger request with model fallback"
```

---

### Task 3: Phase 0 live probe (validate the real payload)

**Files:**
- Modify: `internal/app/app.go`

**Interfaces:**
- Consumes: `codex.LoadProfileAuth`, `codex.TriggerWindow`, `codex.FetchUsage`, `service` default codex home logic (reimplement inline via `os.UserHomeDir` + `.codex`).
- Produces: CLI `codex-lover trigger --probe`.

> This task is the de-risk gate. It sends ONE real request to the **currently active** Codex account (`%USERPROFILE%\.codex\auth.json`), which already has an open window, so no fresh window is wasted. If the request fails, adjust `buildTriggerBody`/`DefaultTriggerModels` in Task 2 before proceeding.

- [ ] **Step 1: Add the CLI branch**

In `internal/app/app.go`, add a case to the `switch args[0]` in `Run`:

```go
	case "trigger":
		return runTriggerProbe(args[1:])
```

Add the function (uses only `codex`, `os`, `path/filepath`, `fmt`, `strings` — add `"codex-lover/internal/codex"` to imports):

```go
func runTriggerProbe(args []string) error {
	probe := false
	for _, a := range args {
		if a == "--probe" {
			probe = true
		}
	}
	if !probe {
		return fmt.Errorf("usage: codex-lover trigger --probe")
	}

	home, err := os.UserHomeDir()
	if err != nil {
		return err
	}
	codexHome := filepath.Join(home, ".codex")
	auth, err := codex.LoadProfileAuth(codexHome)
	if err != nil {
		return fmt.Errorf("load active codex auth: %w", err)
	}

	fmt.Printf("Probing trigger for active account: %s\n", emptyDash(auth.Email))
	result, refreshed, err := codex.TriggerWindow(auth, codex.DefaultTriggerModels)
	if err != nil {
		return fmt.Errorf("trigger failed: %w", err)
	}
	if refreshed != nil {
		fmt.Println("(token was refreshed during trigger)")
	}
	fmt.Printf("OK: model=%s status=%d\n", result.ModelUsed, result.Status)

	usage, uerr := codex.FetchUsage(auth)
	if uerr != nil {
		fmt.Printf("usage check failed: %v\n", uerr)
		return nil
	}
	if usage.Primary != nil && usage.Primary.ResetsAt != nil {
		fmt.Printf("primary window resets at: %s (used %.0f%%)\n",
			usage.Primary.ResetsAt.Local().Format("2006-01-02 15:04"), usage.Primary.UsedPercent)
	} else {
		fmt.Println("primary window not present in usage response")
	}
	return nil
}
```

Add `trigger --probe` to `printUsage()`'s command list.

- [ ] **Step 2: Build the CLI**

Run: `go build -o codex-lover.exe ./cmd/codex-lover`
Expected: no errors.

- [ ] **Step 3: Run the live probe (manual)**

Run: `./codex-lover.exe trigger --probe`
Expected: `OK: model=<something> status=200` and a `primary window resets at: ...` line.

- If it prints `OK` with a 2xx and a valid reset time → payload confirmed. Note the accepted model.
- If it fails with a 4xx body about the request shape → adjust `buildTriggerBody` (Task 2) per the error, rebuild, re-run. If a cheap model is rejected but a `-codex` model works, keep the fallback list as-is (it will just skip to the working one).

- [ ] **Step 4: Commit**

```bash
git add internal/app/app.go
git commit -m "feat: add trigger --probe CLI to validate responses endpoint"
```

---

### Task 4: Selection + schedule decision (pure)

**Files:**
- Create: `internal/service/trigger_select.go`
- Test: `internal/service/trigger_select_test.go`

**Interfaces:**
- Consumes: `model.ProfileStatus`, `model.TriggerConfig`, existing `weeklyRemaining`, `fiveHourRemaining`, `profileLabel` (package `service`).
- Produces: `triggerTarget{Status model.ProfileStatus; SourceProfileID string}`, `triggerSkip{Status model.ProfileStatus; Reason string}`, `func selectTriggerTargets(statuses []model.ProfileStatus, cfg model.TriggerConfig, cachedSource func(model.Profile) (string, bool)) (selected []triggerTarget, skipped []triggerSkip)`, `func shouldFireTrigger(now time.Time, cfg model.TriggerConfig, lastDate string) (bool, string)`, `func parseTimeOfDay(now time.Time, hhmm string) (time.Time, error)`.

- [ ] **Step 1: Write the failing tests**

Create `internal/service/trigger_select_test.go`:

```go
package service

import (
	"testing"
	"time"

	"codex-lover/internal/model"
)

func codexStatus(id string, weekly float64, enabled bool) model.ProfileStatus {
	return model.ProfileStatus{
		Profile: model.Profile{ID: id, Label: id, Tool: model.ToolCodex, Enabled: enabled},
		State: model.ProfileState{
			AuthStatus: model.AuthStatusLoggedOut,
			Usage:      &model.UsageSnapshot{Secondary: &model.UsageWindow{RemainingPercent: weekly}},
		},
	}
}

func allCached(model.Profile) (string, bool) { return "src", true }

func TestSelectTriggerTargetsTopN(t *testing.T) {
	statuses := []model.ProfileStatus{
		codexStatus("a", 30, true),
		codexStatus("b", 90, true),
		codexStatus("c", 60, true),
	}
	cfg := model.TriggerConfig{Mode: model.TriggerModeTopN, Count: 2}
	selected, _ := selectTriggerTargets(statuses, cfg, func(p model.Profile) (string, bool) { return p.ID, true })
	if len(selected) != 2 {
		t.Fatalf("selected %d, want 2", len(selected))
	}
	if selected[0].Status.Profile.ID != "b" || selected[1].Status.Profile.ID != "c" {
		t.Fatalf("wrong order: %s,%s", selected[0].Status.Profile.ID, selected[1].Status.Profile.ID)
	}
}

func TestSelectTriggerTargetsAllSkipsNoAuth(t *testing.T) {
	statuses := []model.ProfileStatus{codexStatus("a", 10, true), codexStatus("b", 20, true)}
	cfg := model.TriggerConfig{Mode: model.TriggerModeAll}
	cached := func(p model.Profile) (string, bool) { return p.ID, p.ID == "a" }
	selected, skipped := selectTriggerTargets(statuses, cfg, cached)
	if len(selected) != 1 || selected[0].Status.Profile.ID != "a" {
		t.Fatalf("selected wrong set")
	}
	if len(skipped) != 1 || skipped[0].Reason != model.TriggerStatusSkippedNoAuth {
		t.Fatalf("expected b skipped no-auth")
	}
}

func TestSelectTriggerTargetsCustom(t *testing.T) {
	statuses := []model.ProfileStatus{codexStatus("a", 10, true), codexStatus("b", 20, true)}
	cfg := model.TriggerConfig{Mode: model.TriggerModeCustom, ProfileIDs: []string{"b"}}
	selected, _ := selectTriggerTargets(statuses, cfg, allCached)
	if len(selected) != 1 || selected[0].Status.Profile.ID != "b" {
		t.Fatalf("custom did not pick b")
	}
}

func TestSelectTriggerTargetsSkipsNonCodex(t *testing.T) {
	claude := model.ProfileStatus{Profile: model.Profile{ID: "cl", Tool: model.ToolClaude, Enabled: true}}
	statuses := []model.ProfileStatus{codexStatus("a", 10, true), claude}
	selected, _ := selectTriggerTargets(statuses, model.TriggerConfig{Mode: model.TriggerModeAll}, allCached)
	if len(selected) != 1 || selected[0].Status.Profile.ID != "a" {
		t.Fatalf("should only pick codex")
	}
}

func TestShouldFireTrigger(t *testing.T) {
	loc := time.Local
	base := time.Date(2026, 7, 8, 8, 0, 0, 0, loc)
	cfg := model.TriggerConfig{Enabled: true, TimeOfDay: "08:00", GraceMins: 60}
	cases := []struct {
		name    string
		now     time.Time
		lastDay string
		cfg     model.TriggerConfig
		want    bool
	}{
		{"disabled", base, "", model.TriggerConfig{Enabled: false, TimeOfDay: "08:00", GraceMins: 60}, false},
		{"at time", base, "", cfg, true},
		{"within grace", base.Add(30 * time.Minute), "", cfg, true},
		{"before time", base.Add(-time.Minute), "", cfg, false},
		{"past grace", base.Add(61 * time.Minute), "", cfg, false},
		{"already ran", base, "2026-07-08", cfg, false},
	}
	for _, tc := range cases {
		got, _ := shouldFireTrigger(tc.now, tc.cfg, tc.lastDay)
		if got != tc.want {
			t.Fatalf("%s: got %v want %v", tc.name, got, tc.want)
		}
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./internal/service/ -run "TestSelectTriggerTargets|TestShouldFireTrigger"`
Expected: FAIL — undefined `selectTriggerTargets`, `shouldFireTrigger`.

- [ ] **Step 3: Write the implementation**

Create `internal/service/trigger_select.go`:

```go
package service

import (
	"sort"
	"strings"
	"time"

	"codex-lover/internal/model"
)

type triggerTarget struct {
	Status          model.ProfileStatus
	SourceProfileID string
}

type triggerSkip struct {
	Status model.ProfileStatus
	Reason string
}

// selectTriggerTargets picks which Codex accounts to trigger. cachedSource
// resolves the profile ID whose cached auth backs a profile (and whether one
// exists). Only enabled Codex accounts with cached auth are eligible.
func selectTriggerTargets(
	statuses []model.ProfileStatus,
	cfg model.TriggerConfig,
	cachedSource func(model.Profile) (string, bool),
) (selected []triggerTarget, skipped []triggerSkip) {
	wanted := map[string]bool{}
	for _, id := range cfg.ProfileIDs {
		wanted[id] = true
	}

	var eligible []triggerTarget
	for _, st := range statuses {
		if st.Profile.Tool != model.ToolCodex || !st.Profile.Enabled {
			continue
		}
		src, ok := cachedSource(st.Profile)
		if !ok {
			if cfg.Mode != model.TriggerModeCustom || wanted[st.Profile.ID] {
				skipped = append(skipped, triggerSkip{Status: st, Reason: model.TriggerStatusSkippedNoAuth})
			}
			continue
		}
		eligible = append(eligible, triggerTarget{Status: st, SourceProfileID: src})
	}

	switch cfg.Mode {
	case model.TriggerModeCustom:
		for _, t := range eligible {
			if wanted[t.Status.Profile.ID] {
				selected = append(selected, t)
			}
		}
	case model.TriggerModeTopN:
		sort.SliceStable(eligible, func(i, j int) bool {
			wi, wj := weeklyRemaining(eligible[i].Status), weeklyRemaining(eligible[j].Status)
			if wi != wj {
				return wi > wj
			}
			pi, pj := fiveHourRemaining(eligible[i].Status), fiveHourRemaining(eligible[j].Status)
			if pi != pj {
				return pi > pj
			}
			return strings.ToLower(profileLabel(eligible[i].Status.Profile)) <
				strings.ToLower(profileLabel(eligible[j].Status.Profile))
		})
		n := cfg.Count
		if n < 1 {
			n = 1
		}
		if n > len(eligible) {
			n = len(eligible)
		}
		selected = append(selected, eligible[:n]...)
	default: // all
		selected = eligible
	}
	return selected, skipped
}

func shouldFireTrigger(now time.Time, cfg model.TriggerConfig, lastDate string) (bool, string) {
	if !cfg.Enabled {
		return false, "disabled"
	}
	scheduled, err := parseTimeOfDay(now, cfg.TimeOfDay)
	if err != nil {
		return false, "invalid time_of_day"
	}
	if now.Format("2006-01-02") == lastDate {
		return false, "already ran today"
	}
	if now.Before(scheduled) {
		return false, "before scheduled time"
	}
	grace := cfg.GraceMins
	if grace <= 0 {
		grace = 60
	}
	if now.After(scheduled.Add(time.Duration(grace) * time.Minute)) {
		return false, "missed grace window"
	}
	return true, "in window"
}

func parseTimeOfDay(now time.Time, hhmm string) (time.Time, error) {
	t, err := time.ParseInLocation("15:04", strings.TrimSpace(hhmm), now.Location())
	if err != nil {
		return time.Time{}, err
	}
	return time.Date(now.Year(), now.Month(), now.Day(), t.Hour(), t.Minute(), 0, 0, now.Location()), nil
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./internal/service/ -run "TestSelectTriggerTargets|TestShouldFireTrigger"`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add internal/service/trigger_select.go internal/service/trigger_select_test.go
git commit -m "feat: add pure trigger selection and schedule decision"
```

---

### Task 5: Service orchestration

**Files:**
- Create: `internal/service/trigger.go`

**Interfaces:**
- Consumes: `s.store`, `s.codexAuthCacheRoot()`, `s.cachedAuthSourceProfileID`, `statusesToProfiles`, `profileLabel`, `codex.TriggerFromCachedAuth`, `codex.FetchUsageFromCachedAuth`, `codex.DefaultTriggerModels`, `selectTriggerTargets`, `shouldFireTrigger`.
- Produces: `func (s *Service) RunTrigger(statuses []model.ProfileStatus, cfg model.TriggerConfig, manual bool) (model.TriggerRun, error)`, `func (s *Service) RunScheduledTrigger(now time.Time, statuses []model.ProfileStatus) (bool, model.TriggerRun, error)`, `func (s *Service) LastTriggerRun() (*model.TriggerRun, error)`, `TriggerSelectionItem{ProfileID, Label, Reason string}`, `func (s *Service) PreviewTriggerSelection(statuses []model.ProfileStatus, cfg model.TriggerConfig) (selectedIDs []string, skipped []TriggerSelectionItem)`.

- [ ] **Step 1: Write the implementation**

Create `internal/service/trigger.go`:

```go
package service

import (
	"time"

	"codex-lover/internal/codex"
	"codex-lover/internal/model"
)

func (s *Service) cachedSourceFunc(profiles []model.Profile) func(model.Profile) (string, bool) {
	return func(p model.Profile) (string, bool) {
		return s.cachedAuthSourceProfileID(p, profiles)
	}
}

// RunTrigger selects and triggers the configured Codex accounts, persists the
// run to state, and returns the run summary. It never switches the active account.
func (s *Service) RunTrigger(statuses []model.ProfileStatus, cfg model.TriggerConfig, manual bool) (model.TriggerRun, error) {
	profiles := statusesToProfiles(statuses)
	selected, skipped := selectTriggerTargets(statuses, cfg, s.cachedSourceFunc(profiles))

	run := model.TriggerRun{RanAt: time.Now().UTC(), Manual: manual}
	for _, t := range selected {
		res := model.TriggerAccountResult{
			ProfileID: t.Status.Profile.ID,
			Label:     profileLabel(t.Status.Profile),
		}
		result, err := codex.TriggerFromCachedAuth(s.codexAuthCacheRoot(), t.SourceProfileID, codex.DefaultTriggerModels)
		if err != nil {
			res.Status = model.TriggerStatusError
			res.Error = err.Error()
		} else {
			res.Status = model.TriggerStatusOpened
			res.ModelUsed = result.ModelUsed
			res.Verified = s.verifyTriggerOpened(t.SourceProfileID)
		}
		run.Results = append(run.Results, res)
	}
	for _, sk := range skipped {
		run.Results = append(run.Results, model.TriggerAccountResult{
			ProfileID: sk.Status.Profile.ID,
			Label:     profileLabel(sk.Status.Profile),
			Status:    sk.Reason,
		})
	}

	if err := s.persistLastTriggerRun(run); err != nil {
		return run, err
	}
	return run, nil
}

func (s *Service) verifyTriggerOpened(sourceProfileID string) bool {
	usage, _, err := codex.FetchUsageFromCachedAuth(s.codexAuthCacheRoot(), sourceProfileID)
	if err != nil || usage == nil || usage.Primary == nil || usage.Primary.ResetsAt == nil {
		return false
	}
	return usage.Primary.ResetsAt.After(time.Now())
}

func (s *Service) persistLastTriggerRun(run model.TriggerRun) error {
	state, err := s.store.LoadState()
	if err != nil {
		return err
	}
	saved := run
	state.LastTriggerRun = &saved
	return s.store.SaveState(state)
}

// RunScheduledTrigger runs the trigger set only if it is due. Returns ran=false
// when not due. On a successful run it stamps LastTriggerDate so it fires once/day.
func (s *Service) RunScheduledTrigger(now time.Time, statuses []model.ProfileStatus) (bool, model.TriggerRun, error) {
	cfg, err := s.store.LoadConfig()
	if err != nil {
		return false, model.TriggerRun{}, err
	}
	state, err := s.store.LoadState()
	if err != nil {
		return false, model.TriggerRun{}, err
	}
	fire, _ := shouldFireTrigger(now, cfg.Trigger, state.LastTriggerDate)
	if !fire {
		return false, model.TriggerRun{}, nil
	}

	run, err := s.RunTrigger(statuses, cfg.Trigger, false)
	if err != nil {
		return true, run, err
	}
	// Reload (RunTrigger saved LastTriggerRun) then stamp the date.
	state, err = s.store.LoadState()
	if err != nil {
		return true, run, err
	}
	state.LastTriggerDate = now.Format("2006-01-02")
	if err := s.store.SaveState(state); err != nil {
		return true, run, err
	}
	return true, run, nil
}

func (s *Service) LastTriggerRun() (*model.TriggerRun, error) {
	state, err := s.store.LoadState()
	if err != nil {
		return nil, err
	}
	return state.LastTriggerRun, nil
}

// TriggerSelectionItem is a UI-facing preview row.
type TriggerSelectionItem struct {
	ProfileID string `json:"profileId"`
	Label     string `json:"label"`
	Reason    string `json:"reason"`
}

func (s *Service) PreviewTriggerSelection(statuses []model.ProfileStatus, cfg model.TriggerConfig) ([]string, []TriggerSelectionItem) {
	profiles := statusesToProfiles(statuses)
	selected, skipped := selectTriggerTargets(statuses, cfg, s.cachedSourceFunc(profiles))
	selectedIDs := make([]string, 0, len(selected))
	for _, t := range selected {
		selectedIDs = append(selectedIDs, t.Status.Profile.ID)
	}
	skips := make([]TriggerSelectionItem, 0, len(skipped))
	for _, sk := range skipped {
		skips = append(skips, TriggerSelectionItem{
			ProfileID: sk.Status.Profile.ID,
			Label:     profileLabel(sk.Status.Profile),
			Reason:    sk.Reason,
		})
	}
	return selectedIDs, skips
}
```

- [ ] **Step 2: Build + verify existing tests still pass**

Run: `go build ./...` then `go test ./internal/...`
Expected: build OK, all tests PASS.

- [ ] **Step 3: Commit**

```bash
git add internal/service/trigger.go
git commit -m "feat: add service trigger orchestration and scheduling"
```

---

### Task 6: Runtime scheduler wiring

**Files:**
- Modify: `desktop-app/runtime.go`
- Test: `desktop-app/runtime_trigger_test.go`

**Interfaces:**
- Consumes: `a.svc.ProfileStatuses`, `a.svc.RunScheduledTrigger`, `notify`, `model.TriggerStatusOpened`.
- Produces: `func (a *App) maybeRunScheduledTriggerLocked()`.

- [ ] **Step 1: Write the failing test**

Create `desktop-app/runtime_trigger_test.go` (verifies the opened-count helper used in the notification):

```go
package main

import (
	"testing"

	"codex-lover/internal/model"
)

func TestCountOpenedResults(t *testing.T) {
	run := model.TriggerRun{Results: []model.TriggerAccountResult{
		{Status: model.TriggerStatusOpened},
		{Status: model.TriggerStatusError},
		{Status: model.TriggerStatusOpened},
		{Status: model.TriggerStatusSkippedNoAuth},
	}}
	if got := countOpenedResults(run); got != 2 {
		t.Fatalf("countOpenedResults = %d, want 2", got)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./desktop-app/ -run TestCountOpenedResults`
Expected: FAIL — `countOpenedResults` undefined.

- [ ] **Step 3: Wire the scheduler + helper**

In `desktop-app/runtime.go`, change the 15s tick case inside `runBackgroundLoops` from:

```go
		case <-refreshTicker.C:
			a.mu.Lock()
			_, _ = a.refreshLockedWithOptions(true, a.backgroundRefreshOptionsLocked())
			a.mu.Unlock()
```

to:

```go
		case <-refreshTicker.C:
			a.mu.Lock()
			_, _ = a.refreshLockedWithOptions(true, a.backgroundRefreshOptionsLocked())
			a.maybeRunScheduledTriggerLocked()
			a.mu.Unlock()
```

Add these functions to `desktop-app/runtime.go` (the file already imports `codex-lover/internal/model` and `codex-lover/internal/notify`; add `"fmt"`):

```go
func (a *App) maybeRunScheduledTriggerLocked() {
	statuses, err := a.svc.ProfileStatuses()
	if err != nil {
		return
	}
	ran, run, err := a.svc.RunScheduledTrigger(time.Now(), statuses)
	if err != nil || !ran {
		return
	}
	opened := countOpenedResults(run)
	_ = notify.New().Send(notify.Event{
		Title:   "Codex accounts triggered",
		Message: fmt.Sprintf("Da trigger %d account luc %s", opened, run.RanAt.Local().Format("15:04")),
		Level:   notify.LevelInfo,
	})
}

func countOpenedResults(run model.TriggerRun) int {
	count := 0
	for _, r := range run.Results {
		if r.Status == model.TriggerStatusOpened {
			count++
		}
	}
	return count
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./desktop-app/ -run TestCountOpenedResults`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add desktop-app/runtime.go desktop-app/runtime_trigger_test.go
git commit -m "feat: run scheduled trigger from desktop runtime loop"
```

---

### Task 7: Desktop bindings

**Files:**
- Modify: `desktop-app/app.go`

**Interfaces:**
- Consumes: `a.ensureReady`, `a.svc.LoadConfig/SaveConfig/ProfileStatuses/RunTrigger/PreviewTriggerSelection/LastTriggerRun`, `a.snapshot`, `model.TriggerConfig`.
- Produces (bound to frontend): `GetTriggerSettings() model.TriggerConfig`, `SaveTriggerSettings(model.TriggerConfig) error`, `TriggerNow() ActionResponse`, `PreviewTriggerSelection(model.TriggerConfig) TriggerPreview`, `GetLastTriggerRun() *model.TriggerRun`. Type `TriggerPreview{SelectedIds []string; Skipped []service.TriggerSelectionItem}`.

- [ ] **Step 1: Add the bindings**

In `desktop-app/app.go` add (the file already imports `model` and `service`):

```go
type TriggerPreview struct {
	SelectedIds []string                        `json:"selectedIds"`
	Skipped     []service.TriggerSelectionItem  `json:"skipped"`
}

func (a *App) GetTriggerSettings() model.TriggerConfig {
	if err := a.ensureReady(); err != nil {
		return model.TriggerConfig{}
	}
	cfg, err := a.svc.LoadConfig()
	if err != nil {
		return model.TriggerConfig{}
	}
	return cfg.Trigger
}

func (a *App) SaveTriggerSettings(trigger model.TriggerConfig) error {
	if err := a.ensureReady(); err != nil {
		return err
	}
	cfg, err := a.svc.LoadConfig()
	if err != nil {
		return err
	}
	cfg.Trigger = normalizeTriggerConfig(trigger)
	return a.svc.SaveConfig(cfg)
}

func (a *App) PreviewTriggerSelection(trigger model.TriggerConfig) TriggerPreview {
	if err := a.ensureReady(); err != nil {
		return TriggerPreview{SelectedIds: []string{}, Skipped: []service.TriggerSelectionItem{}}
	}
	statuses, err := a.svc.ProfileStatuses()
	if err != nil {
		return TriggerPreview{SelectedIds: []string{}, Skipped: []service.TriggerSelectionItem{}}
	}
	ids, skipped := a.svc.PreviewTriggerSelection(statuses, normalizeTriggerConfig(trigger))
	if ids == nil {
		ids = []string{}
	}
	if skipped == nil {
		skipped = []service.TriggerSelectionItem{}
	}
	return TriggerPreview{SelectedIds: ids, Skipped: skipped}
}

func (a *App) GetLastTriggerRun() *model.TriggerRun {
	if err := a.ensureReady(); err != nil {
		return nil
	}
	run, err := a.svc.LastTriggerRun()
	if err != nil {
		return nil
	}
	return run
}

func (a *App) TriggerNow() ActionResponse {
	if err := a.ensureReady(); err != nil {
		return ActionResponse{Message: "Trigger failed", Error: err.Error(), Snapshot: a.mustSnapshotFallback()}
	}
	a.mu.Lock()
	statuses, err := a.svc.ProfileStatuses()
	if err != nil {
		a.mu.Unlock()
		return ActionResponse{Message: "Trigger failed", Error: err.Error(), Snapshot: a.mustSnapshotFallback()}
	}
	cfg, err := a.svc.LoadConfig()
	if err != nil {
		a.mu.Unlock()
		return ActionResponse{Message: "Trigger failed", Error: err.Error(), Snapshot: a.mustSnapshotFallback()}
	}
	run, runErr := a.svc.RunTrigger(statuses, cfg.Trigger, true)
	a.mu.Unlock()

	opened := 0
	for _, r := range run.Results {
		if r.Status == model.TriggerStatusOpened {
			opened++
		}
	}
	message := fmt.Sprintf("Triggered %d account(s)", opened)
	errText := ""
	if runErr != nil {
		errText = runErr.Error()
	}
	snapshot, snapErr := a.snapshot(true)
	if snapErr != nil {
		snapshot = a.mustSnapshotFallback()
	}
	return ActionResponse{Message: message, Error: errText, Snapshot: snapshot}
}

func normalizeTriggerConfig(trigger model.TriggerConfig) model.TriggerConfig {
	if strings.TrimSpace(trigger.TimeOfDay) == "" {
		trigger.TimeOfDay = "08:00"
	}
	switch trigger.Mode {
	case model.TriggerModeAll, model.TriggerModeTopN, model.TriggerModeCustom:
	default:
		trigger.Mode = model.TriggerModeAll
	}
	if trigger.Count < 1 {
		trigger.Count = 1
	}
	if trigger.GraceMins <= 0 {
		trigger.GraceMins = 60
	}
	if trigger.ProfileIDs == nil {
		trigger.ProfileIDs = []string{}
	}
	return trigger
}
```

(`fmt` and `strings` are already imported in `desktop-app/app.go`.)

- [ ] **Step 2: Regenerate Wails bindings + build**

Run: `cd desktop-app && wails build -clean`
Expected: build succeeds; `desktop-app/frontend/wailsjs/go/main/App.d.ts` now includes `GetTriggerSettings`, `SaveTriggerSettings`, `TriggerNow`, `PreviewTriggerSelection`, `GetLastTriggerRun`.

- [ ] **Step 3: Commit**

```bash
git add desktop-app/app.go desktop-app/frontend/wailsjs
git commit -m "feat: add desktop bindings for auto-trigger settings"
```

---

### Task 8: Settings UI (time dropdown + compact multi-select)

**Files:**
- Modify: `desktop-app/frontend/src/App.tsx`
- Modify: `desktop-app/frontend/src/style.css`

**Interfaces:**
- Consumes bindings: `GetTriggerSettings`, `SaveTriggerSettings`, `TriggerNow`, `GetLastTriggerRun` from `../wailsjs/go/main/App`.

- [ ] **Step 1: Add imports + types + state**

At the top of `App.tsx`, extend the bindings import:

```tsx
import {
  ActivateProfile,
  AddAccount,
  GetConfig,
  GetInitialSnapshot,
  GetSnapshot,
  GetSystemStatus,
  GetTriggerSettings,
  GetLastTriggerRun,
  LogoutProfile,
  OpenCodexInstallPage,
  RefreshSnapshot,
  SaveTriggerSettings,
  SetAutoRotateCodex,
  SetAutoRotateThreshold,
  TriggerNow,
} from "../wailsjs/go/main/App";
```

Add types after the existing `SystemStatus` type:

```tsx
type TriggerMode = "all" | "top_n" | "custom";

type TriggerConfig = {
  enabled: boolean;
  time_of_day: string;
  mode: TriggerMode;
  count: number;
  profile_ids: string[];
  grace_minutes: number;
};

type TriggerAccountResult = {
  profile_id: string;
  label: string;
  status: string;
  model_used?: string;
  verified: boolean;
  error?: string;
};

type TriggerRun = {
  ran_at: string;
  manual: boolean;
  results: TriggerAccountResult[];
};

const TIME_SLOTS: string[] = Array.from({ length: 48 }, (_, i) => {
  const h = String(Math.floor(i / 2)).padStart(2, "0");
  const m = i % 2 === 0 ? "00" : "30";
  return `${h}:${m}`;
});

const DEFAULT_TRIGGER: TriggerConfig = {
  enabled: false,
  time_of_day: "08:00",
  mode: "all",
  count: 2,
  profile_ids: [],
  grace_minutes: 60,
};
```

Add state inside `App()` next to the other `useState` hooks:

```tsx
  const [trigger, setTrigger] = useState<TriggerConfig>(DEFAULT_TRIGGER);
  const [lastRun, setLastRun] = useState<TriggerRun | null>(null);
```

- [ ] **Step 2: Load trigger config + last run**

In the first `useEffect` add `void loadTrigger();`. Add these functions near `loadConfig`:

```tsx
  async function loadTrigger() {
    try {
      const t = (await GetTriggerSettings()) as unknown as TriggerConfig;
      setTrigger({ ...DEFAULT_TRIGGER, ...t, profile_ids: t.profile_ids ?? [] });
    } catch {}
    try {
      const run = (await GetLastTriggerRun()) as unknown as TriggerRun | null;
      setLastRun(run);
    } catch {}
  }

  async function saveTrigger(next: TriggerConfig) {
    setTrigger(next);
    try {
      await SaveTriggerSettings(next as any);
    } catch {
      setStatusText("ERROR: SAVE FAILED");
    }
  }

  async function onTriggerNow() {
    setStatusText("TRIGGERING...");
    try {
      const result = await TriggerNow();
      applyAction(result);
      const run = (await GetLastTriggerRun()) as unknown as TriggerRun | null;
      setLastRun(run);
    } catch {
      setStatusText("ERROR: TRIGGER FAILED");
    }
  }

  function toggleCustomProfile(id: string) {
    const has = trigger.profile_ids.includes(id);
    const profile_ids = has
      ? trigger.profile_ids.filter((p) => p !== id)
      : [...trigger.profile_ids, id];
    void saveTrigger({ ...trigger, profile_ids });
  }
```

Add a memo for the codex-only cards (for custom picker), after `sortedProfiles`:

```tsx
  const codexProfiles = useMemo(
    () => snapshot.profiles.filter((p) => p.provider.toLowerCase() === "codex"),
    [snapshot.profiles]
  );
```

- [ ] **Step 3: Add the Auto-Trigger section into the Settings modal**

In `App.tsx`, inside the settings modal's `<div className="space-y-8">`, after the SWITCH_THRESHOLD block (before the closing `</div>` of `space-y-8`), insert:

```tsx
              <div className="bg-[rgba(0,243,255,0.05)] p-4 border border-[rgba(0,243,255,0.1)] space-y-4">
                <div className="flex justify-between items-center">
                  <div>
                    <div className="font-bold text-sm">AUTO_TRIGGER (OPENAI ONLY)</div>
                    <div className="text-[10px] text-dim">Open 5H quota window on a schedule</div>
                  </div>
                  <label className="relative inline-flex items-center cursor-pointer">
                    <input
                      type="checkbox"
                      className="sr-only peer"
                      checked={trigger.enabled}
                      onChange={(e) => void saveTrigger({ ...trigger, enabled: e.target.checked })}
                    />
                    <div className="w-11 h-6 bg-gray-700 rounded-full peer peer-checked:after:translate-x-full after:content-[''] after:absolute after:top-[2px] after:left-[2px] after:bg-white after:rounded-full after:h-5 after:w-5 after:transition-all peer-checked:bg-neon-cyan"></div>
                  </label>
                </div>

                {trigger.enabled && (
                  <>
                    <div className="flex justify-between items-center">
                      <span className="text-[11px] text-dim">TRIGGER_TIME</span>
                      <select
                        className="trigger-select"
                        value={trigger.time_of_day}
                        onChange={(e) => void saveTrigger({ ...trigger, time_of_day: e.target.value })}
                      >
                        {TIME_SLOTS.map((slot) => (
                          <option key={slot} value={slot}>{slot}</option>
                        ))}
                      </select>
                    </div>

                    <div className="flex gap-2">
                      {(["all", "top_n", "custom"] as TriggerMode[]).map((m) => (
                        <button
                          key={m}
                          onClick={() => void saveTrigger({ ...trigger, mode: m })}
                          className={clsx("cyber-btn flex-1 text-[10px]", trigger.mode === m && "cyber-btn-solid")}
                        >
                          {m === "top_n" ? "TOP N" : m.toUpperCase()}
                        </button>
                      ))}
                    </div>

                    {trigger.mode === "top_n" && (
                      <div className="flex justify-between items-center">
                        <span className="text-[11px] text-dim">ACCOUNT_COUNT (best weekly quota)</span>
                        <input
                          type="number"
                          min={1}
                          max={Math.max(1, codexProfiles.length)}
                          value={trigger.count}
                          onChange={(e) => void saveTrigger({ ...trigger, count: Math.max(1, Number(e.target.value)) })}
                          className="trigger-select w-16 text-center"
                        />
                      </div>
                    )}

                    {trigger.mode === "custom" && (
                      <div className="trigger-picker">
                        {codexProfiles.map((p) => (
                          <label key={p.id} className={clsx("trigger-pick-row", trigger.profile_ids.includes(p.id) && "selected")}>
                            <input
                              type="checkbox"
                              checked={trigger.profile_ids.includes(p.id)}
                              onChange={() => toggleCustomProfile(p.id)}
                            />
                            <span className="trigger-pick-name" title={p.label}>{p.label}</span>
                            <span className="trigger-pick-quota">5H {p.primaryPercent}% · WK {p.secondaryPercent}%</span>
                          </label>
                        ))}
                        {codexProfiles.length === 0 && (
                          <div className="text-[10px] text-dim">No Codex accounts.</div>
                        )}
                      </div>
                    )}

                    <button onClick={() => void onTriggerNow()} className="cyber-btn cyber-btn-solid w-full flex items-center justify-center gap-2">
                      <Activity size={14} /> TRIGGER NOW
                    </button>

                    {lastRun && (
                      <div className="trigger-lastrun">
                        <div className="text-[10px] text-dim mb-1">
                          LAST_RUN {new Date(lastRun.ran_at).toLocaleString()}
                        </div>
                        {lastRun.results.map((r) => (
                          <div key={r.profile_id} className="trigger-lastrun-row">
                            <span>{r.status === "opened" ? "✓" : r.status === "error" ? "✗" : "•"} {r.label}</span>
                            <span className="text-dim">
                              {r.status === "opened" ? (r.model_used || "opened") : r.status.replace("_", " ")}
                            </span>
                          </div>
                        ))}
                      </div>
                    )}
                  </>
                )}
              </div>
```

(`Activity` is already imported from lucide-react.)

- [ ] **Step 4: Add CSS**

Append to `desktop-app/frontend/src/style.css`:

```css
.trigger-select {
  background: rgba(0, 0, 0, 0.4);
  border: 1px solid rgba(0, 243, 255, 0.2);
  color: #d6faff;
  font-size: 11px;
  padding: 4px 8px;
  border-radius: 4px;
}
.trigger-picker {
  max-height: 220px;
  overflow-y: auto;
  display: flex;
  flex-direction: column;
  gap: 6px;
}
.trigger-pick-row {
  display: flex;
  align-items: center;
  gap: 8px;
  padding: 6px 8px;
  border: 1px solid rgba(0, 243, 255, 0.12);
  border-radius: 4px;
  font-size: 11px;
  cursor: pointer;
}
.trigger-pick-row.selected {
  border-color: rgba(0, 243, 255, 0.5);
  background: rgba(0, 243, 255, 0.08);
}
.trigger-pick-name {
  flex: 1;
  min-width: 0;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}
.trigger-pick-quota {
  font-size: 9px;
  color: #7fe9ff;
  white-space: nowrap;
}
.trigger-lastrun {
  border-top: 1px dashed rgba(0, 243, 255, 0.15);
  padding-top: 8px;
}
.trigger-lastrun-row {
  display: flex;
  justify-content: space-between;
  font-size: 10px;
  padding: 2px 0;
}
```

- [ ] **Step 5: Build + manual verification**

Run: `cd desktop-app && wails build -clean` then from repo root `./install.cmd` and `codex-lover run` (close the app first if open).
Expected in the app:
- Core Settings → AUTO_TRIGGER toggle appears.
- Toggling on reveals a 30-min time dropdown, mode buttons, and (for Custom) a compact multi-select list of Codex accounts showing 5H/WK quota.
- "TRIGGER NOW" runs immediately; the last-run panel lists per-account results.

- [ ] **Step 6: Commit**

```bash
git add desktop-app/frontend/src/App.tsx desktop-app/frontend/src/style.css
git commit -m "feat: add auto-trigger settings UI with time slots and multi-select"
```

---

## Self-Review Notes

- **Spec coverage:** toggle (T8) · time dropdown 30-min (T8) · mode all/top_n/custom (T4/T8) · top-N by weekly quota (T4) · custom compact multi-select with quota (T8) · background delivery via cached tokens, no switch (T2/T5) · cheapest-model + fallback (T2) · scheduler when app open (T6) · verify window opened (T5) · last-run status (T5/T7/T8) · Phase 0 probe (T3) · OpenAI-only + no-token-logging (T2/T4 global constraints). All covered.
- **Model list** is a package `var` so Task 3 can adjust it after the live probe without touching call sites.
- **Ordering:** Task 3 (live probe) runs right after the core so the real payload is confirmed before the service/UI are built on top of it.
