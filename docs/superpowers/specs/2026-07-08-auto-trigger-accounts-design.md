# Auto-Trigger Accounts (OpenAI/Codex only) — Design Spec

- **Date:** 2026-07-08
- **Status:** Approved design, pending spec review
- **Scope:** codex-lover desktop app — new Settings feature to auto-open the Codex 5H quota window on selected accounts at a fixed daily time.

---

## 1. Problem & Goal

When a user works across multiple Codex accounts, the 5-hour rate-limit window is
anchored to the **first request** of each account. Today the user manually sends a
junk message into each account at the start of the day so all windows open at the same
time (e.g. trigger at 08:00 → all reset ~13:00). Doing this by hand for every account
is tedious and easy to forget.

**Goal:** Let the user configure codex-lover to automatically send one tiny "trigger"
request to a chosen set of Codex accounts at a fixed daily time, using each account's
**cached OAuth token in the background** — without switching the active account.

**Non-goals (v1):**
- Trigger while the desktop app is closed (scheduler runs inside desktop runtime; app must be open at trigger time — same constraint as notifications/auto-switch today).
- Support providers other than Codex/OpenAI (Claude/Kimi excluded).
- Per-account individual trigger times (a single daily time applies to the whole selected set — matching the user's intent that all windows share one boundary).
- Timezone configuration (uses the machine's local time).

---

## 2. Feasibility (verified)

Verified on-machine by grepping the real `codex.exe` (Rust binary) string table:

| Item | Confirmed value |
|---|---|
| Model request endpoint | `https://chatgpt.com/backend-api/codex/responses` |
| Auth headers | `Authorization: Bearer <access_token>`, `chatgpt-account-id`, `OpenAI-Beta`, `originator: codex_cli_rs` |
| Payload knobs present | `reasoning_effort: "minimal"`, `store`, `stream` |
| Cheap models present in binary | `gpt-5.4-nano`, `gpt-5.4-mini`, `o4-mini` (besides `-codex` variants) |
| Usage verification | existing `https://chatgpt.com/backend-api/wham/usage` (already used by `internal/codex/usage.go`) |

codex-lover already loads each account's `access_token` + `account_id` from cached auth
(`internal/codex/auth.go: LoadProfileAuth`) and already refreshes tokens on 401
(`internal/codex/usage.go: refreshAuth`). The trigger feature reuses this infrastructure.

**Residual risks (non-blocking):**
1. The exact JSON body of `/responses` is ~90% known from the binary but must be locked
   with a real request → handled by **Phase 0 probe** before building the rest.
2. Automating requests across multiple accounts to pre-open rate-limit windows is a gray
   area under OpenAI ToS. These are the user's own accounts/quota for personal management,
   not abuse. Flagged for the user's awareness; the user has accepted proceeding.

---

## 3. Decisions (locked)

- **Delivery:** background using cached tokens; the active account is **never** switched.
- **Model:** auto-pick cheapest, with fallback — try `gpt-5.4-nano` → `gpt-5.4-mini` →
  `gpt-5.1-codex-mini` → `gpt-5.1-codex`; first accepted model wins. (List refined by Phase 0.)
- **Scheduler location:** inside `desktop-app/runtime.go` (fires only while the app is open).
- **Time selection UI:** preset dropdown at 30-minute granularity (`00:00 … 23:30`).
- **Custom account picker UI:** reuse the main-page quota card, scaled down (compact),
  multi-select via checkbox.

---

## 4. Data Model (`internal/model/types.go`)

```go
type TriggerConfig struct {
    Enabled    bool     `json:"enabled"`
    TimeOfDay  string   `json:"time_of_day"`   // "HH:MM" 24h, machine local
    Mode       string   `json:"mode"`          // "all" | "top_n" | "custom"
    Count      int      `json:"count"`         // used when mode == "top_n"
    ProfileIDs []string `json:"profile_ids"`   // used when mode == "custom"
    GraceMins  int      `json:"grace_minutes"` // catch-up window; default 60
}
```

Add to `Config`: `Trigger TriggerConfig \`json:"trigger"\``.

Run results live in `State` (for the UI "last run" panel):

```go
type TriggerRun struct {
    RanAt   time.Time              `json:"ran_at"`
    Manual  bool                   `json:"manual"`
    Results []TriggerAccountResult `json:"results"`
}

type TriggerAccountResult struct {
    ProfileID string `json:"profile_id"`
    Label     string `json:"label"`
    Status    string `json:"status"`     // "opened" | "skipped_no_auth" | "not_eligible" | "error"
    ModelUsed string `json:"model_used,omitempty"`
    Verified  bool   `json:"verified"`
    Error     string `json:"error,omitempty"`
}
```

Add to `State`: `LastTriggerRun *TriggerRun` and `LastTriggerDate string` (`"YYYY-MM-DD"`
local, prevents double-fire in one day).

Config defaults on first load: `Enabled=false`, `TimeOfDay="08:00"`, `Mode="all"`,
`Count=2`, `GraceMins=60`.

---

## 5. Components

| File | Responsibility |
|---|---|
| `internal/codex/trigger.go` *(new)* | Build minimal `/responses` payload, POST it, model fallback, refresh-on-401, return model used + refreshed auth |
| `internal/service/trigger_select.go` *(new)* | `SelectTriggerTargets` — choose accounts by mode; eligibility filtering |
| `desktop-app/runtime.go` | `shouldFireTrigger` (pure) + wire trigger into the existing runtime tick; run set; persist `TriggerRun`; notify |
| `desktop-app/app.go` | Bindings: `GetTriggerSettings`, `SaveTriggerSettings`, `TriggerNow`, `PreviewTriggerSelection`, `GetLastTriggerRun` |
| `desktop-app/frontend/src/App.tsx` + `style.css` | Settings panel + compact multi-select card grid |

No changes to existing codex/claude/kimi refresh/notify/auto-switch flows.

---

## 6. Selection Algorithm (`SelectTriggerTargets`)

```go
func SelectTriggerTargets(profiles []ProfileStatus, cfg TriggerConfig) (selected, skipped []ProfileStatus)
```

**Eligible** = `Tool == "codex"` (OpenAI only) **and** `Enabled` **and** has cached auth
(the same "switchable" condition `AutoSwitchLimitedCodex` already uses — an account with
no cached auth cannot be triggered in the background).

- `mode == "all"` → all eligible accounts.
- `mode == "top_n"` → eligible accounts sorted by **weekly (secondary) `RemainingPercent`
  descending**; tie-break by primary `RemainingPercent` desc, then `Label`; take `Count`.
  (This is the "accounts with good weekly quota" the user described.)
- `mode == "custom"` → intersection of `cfg.ProfileIDs` with the eligible set.

Any chosen-but-not-eligible account goes into `skipped` with a reason
(`skipped_no_auth` / `not_eligible`) and is shown in the UI.

Missing weekly window (nil secondary) sorts last (treated as 0% remaining).

---

## 7. Trigger Core (`TriggerWindow`)

```go
var DefaultTriggerModels = []string{"gpt-5.4-nano", "gpt-5.4-mini", "gpt-5.1-codex-mini", "gpt-5.1-codex"}

type TriggerResult struct {
    ModelUsed string
    Status    int
}

func TriggerWindow(auth *ProfileAuth, models []string) (*TriggerResult, *AuthFile, error)
```

Request:
- `POST https://chatgpt.com/backend-api/codex/responses`
- Headers: `Authorization: Bearer`, `chatgpt-account-id`, `OpenAI-Beta: responses=experimental`,
  `originator: codex_cli_rs`, `User-Agent`, `session_id` (uuid), `Content-Type: application/json`.
- Body (minimal; exact shape locked in Phase 0):
  ```json
  {
    "model": "<candidate>",
    "instructions": "",
    "input": [{"type":"message","role":"user","content":[{"type":"input_text","text":"ok"}]}],
    "reasoning": {"effort":"minimal"},
    "store": false,
    "stream": true,
    "max_output_tokens": 16
  }
  ```
  Response stream is read to completion and discarded.

Control flow:
- Try each model in order. On a model-rejected error (4xx indicating unknown/forbidden model)
  → try the next candidate.
- On `401` → refresh via existing `refreshAuth`, persist new tokens (as `FetchUsage` does),
  retry the same model once.
- On the first `2xx` → success; record `ModelUsed`.
- If all candidates fail → return error (`no accepted model` or the last transport error).

Verification (soft): after a successful trigger, call `FetchUsage` and check the primary
window now has a future `ResetsAt` (~ now + 5h) → set `Verified=true`. Verify failure does
not mark the trigger as failed (the window may still have opened).

Security: never log tokens or full auth payloads (per `AGENTS.md`).

---

## 8. Scheduler (`shouldFireTrigger`, pure)

```go
func shouldFireTrigger(now time.Time, cfg TriggerConfig, lastDate string) (fire bool, reason string)
```

Logic:
- `!cfg.Enabled` → false.
- Parse `TimeOfDay` → today's scheduled local time.
- `today := now.Format("2006-01-02")`; if `lastDate == today` → false (already ran).
- `now < scheduled` → false (not yet).
- `now > scheduled + GraceMins` → false, reason `"missed"` (no surprise late firing).
- Otherwise → true.

Wiring: the existing runtime tick (or a dedicated ~30–60s ticker) calls `shouldFireTrigger`.
On fire:
1. `selected, skipped := SelectTriggerTargets(currentStatuses, cfg)`
2. For each `selected` account **sequentially** (avoid hammering the backend), call
   `TriggerWindow`; collect `TriggerAccountResult`.
3. Save `TriggerRun` to state; set `LastTriggerDate = today`.
4. Emit a desktop event + notification: `"Đã trigger N accounts lúc HH:MM"`.

Manual `TriggerNow()` bypasses the schedule and runs the selected set immediately
(`Manual=true`), without touching `LastTriggerDate`.

---

## 9. Settings UI (`App.tsx` + `style.css`)

A gear button in the header opens a Settings panel:

- **Toggle** — "Bật auto-trigger (OpenAI only)".
- **Time** — dropdown of preset slots at 30-minute steps (`00:00, 00:30, … 23:30`),
  bound to `TimeOfDay`.
- **Mode** — selector: `All` / `Top N` / `Custom`.
  - `Top N` — numeric `Count` input + **live preview** of the accounts that would be
    picked right now (via `PreviewTriggerSelection`), e.g. "sẽ trigger: A (tuần 80%), B (70%)".
  - `Custom` — the main-page quota card **reused but compact**, rendered as a grid with a
    **checkbox per card** for multi-select; shows the same quota bars so the user can decide
    by looking at quota. Selected IDs → `ProfileIDs`.
- **"Trigger ngay"** button → `TriggerNow()`.
- **Last-run panel** — from `GetLastTriggerRun()`: per-account `✓ opened` / `✗ error` /
  `skip (no auth)`, model used, verified flag, and run time.

---

## 10. Bindings (`desktop-app/app.go`)

- `GetTriggerSettings() TriggerConfig`
- `SaveTriggerSettings(cfg TriggerConfig) error` — validate `TimeOfDay` (`HH:MM`, 30-min
  slot), `Mode` in enum, `Count >= 1` when `top_n`.
- `TriggerNow() TriggerRun` — manual run.
- `PreviewTriggerSelection(cfg TriggerConfig) (selected, skipped []ProfileStatus)` — for UI preview.
- `GetLastTriggerRun() *TriggerRun`.

---

## 11. Error Handling & Edge Cases

- Account without cached auth → `skipped_no_auth` (shown, not an error).
- One account's failure never blocks the others (sequential, independent results).
- Token expired → refresh once + persist; if refresh fails → `error` for that account.
- All models rejected → `error` for that account (Phase 0 should prevent this in practice).
- App closed at scheduled time → missed; if opened after `scheduled + grace`, shown as
  "bỏ lỡ hôm nay".
- Local timezone from the machine (no TZ config in v1).
- Never print tokens or full auth payloads.

---

## 12. Testing

- `internal/service`: `SelectTriggerTargets` — table-driven (all / top_n / custom,
  eligibility filtering, tie-break, nil-weekly-window).
- `desktop-app`: `shouldFireTrigger` — table-driven (disabled, before-time, already-ran,
  past-grace, in-window boundaries).
- `internal/codex`: `TriggerWindow` with `httptest` mock — model fallback (400 → 200),
  401 → refresh → retry, header/payload assertions. No real network in unit tests.
- Phase 0 live probe is manual (real account), not part of CI.

---

## 13. Phase 0 — Live Probe (de-risk first)

Before building the UI, add a temporary `codex-lover trigger --probe` CLI subcommand that:
1. Loads the **currently active** account's auth (this account already has an open 5H
   window, so the probe does not waste a fresh window).
2. Sends the minimal `/responses` request, trying the model list.
3. Prints HTTP status + which model was accepted + the primary window `reset_at` from a
   follow-up usage fetch.

This locks the exact payload and the real cheap-model list. If it fails, adjust before
writing the remaining code. The probe never prints tokens.

---

## 14. Implementation Order

1. **Phase 0** — probe subcommand; confirm endpoint/payload/model list.
2. Data model (`TriggerConfig`, `TriggerRun`, `State` fields) + config defaults.
3. `internal/codex/trigger.go` + unit tests.
4. `internal/service/trigger_select.go` + unit tests.
5. `desktop-app/runtime.go` scheduler + `shouldFireTrigger` tests + notification.
6. `desktop-app/app.go` bindings.
7. `App.tsx` + `style.css` Settings panel (time dropdown, compact multi-select cards, preview, last-run).
8. Manual end-to-end verification with the desktop app.
