# Codex Account Audience Filter Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Let the user mark each Codex account as personal-use or customer-use, visually distinguish customer cards, and filter cards by usage audience.

**Architecture:** Persist a small local `Profile.Audience` string in config. Default empty/personal values render as personal so existing accounts keep current behavior. Extend the existing `UpdateProfileMeta` path to save date, price, and audience together; expose audience on desktop `ProfileCard`; filter/render audience entirely in the React desktop UI.

**Tech Stack:** Go stdlib, Wails v2 bindings, React + TypeScript, CSS.

---

## Decisions

- Applies to Codex cards only.
- Audience values: `personal` and `customer`.
- Existing accounts with empty audience are treated as `personal`.
- New accounts default to `personal`, including the price prompt shown after login.
- Customer accounts use a clearly different light card style: white background and dark text.
- Filter defaults to `all` and has `Tất cả`, `Cá nhân`, `Khách hàng` options under the top action buttons.

---

## Tasks

### Task 1: Model And Meta Service

**Files:**
- Modify: `internal/model/types.go`
- Modify: `internal/service/profile_meta.go`
- Modify: `internal/service/profile_meta_test.go`

- [ ] Add `Audience string json:"audience,omitempty"` to `model.Profile`.
- [ ] Add constants `ProfileAudiencePersonal = "personal"` and `ProfileAudienceCustomer = "customer"`.
- [ ] Extend `applyProfileMeta` and `UpdateProfileMeta` to accept audience.
- [ ] Normalize empty/invalid audience to `personal`.
- [ ] Add failing tests first, then implementation.

### Task 2: Desktop Snapshot And Binding

**Files:**
- Modify: `desktop-app/app.go`
- Modify: `desktop-app/app_snapshot_test.go`

- [ ] Add `Audience string json:"audience"` to `ProfileCard`.
- [ ] Update `UpdateProfileMeta(profileID, createdAtISO, price, audience)` Wails method.
- [ ] Populate card audience with normalized value.
- [ ] Add snapshot test for default personal and explicit customer.

### Task 3: Frontend Editing And New-Account Prompt

**Files:**
- Modify: `desktop-app/frontend/src/App.tsx`

- [ ] Extend `ProfileCard` type with `audience`.
- [ ] Add edit modal radio options: `Tài khoản sử dụng cho cá nhân`, `Tài khoản sử dụng cho khách`.
- [ ] Add same radio choices to the new-account price prompt, default `personal`.
- [ ] Pass audience to `UpdateProfileMeta`.

### Task 4: Customer Theme And Audience Filter

**Files:**
- Modify: `desktop-app/frontend/src/App.tsx`
- Modify: `desktop-app/frontend/src/style.css`

- [ ] Add audience filter state default `all`.
- [ ] Render filter buttons under top actions: `Tất cả`, `Cá nhân`, `Khách hàng`.
- [ ] Filter Codex profiles by audience while preserving existing provider filter.
- [ ] Add `account-card-customer` light style for customer Codex cards.
- [ ] Personal cards keep existing appearance.

### Task 5: Verification And Install

**Files:**
- Generated: Wails bindings under `desktop-app/frontend/wailsjs`.

- [ ] Run `go test ./...`.
- [ ] Run `npm run build` in `desktop-app/frontend`.
- [ ] Run `wails build -clean` in `desktop-app`.
- [ ] Run `./install.cmd` and relaunch app.
