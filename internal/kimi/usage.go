package kimi

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"codex-lover/internal/model"
)

type usagesPayload struct {
	User     *usagesUser     `json:"user"`
	Usage    *usagesDetail   `json:"usage"`
	Limits   []usagesLimit   `json:"limits"`
	Parallel *usagesParallel `json:"parallel"`
	SubType  string          `json:"subType"`
}

type usagesUser struct {
	UserID     string              `json:"userId"`
	Region     string              `json:"region"`
	Membership *usagesMembership   `json:"membership"`
	BusinessID string              `json:"businessId"`
}

type usagesMembership struct {
	Level string `json:"level"`
}

type usagesDetail struct {
	Limit     string `json:"limit"`
	Used      string `json:"used"`
	Remaining string `json:"remaining"`
	ResetTime string `json:"resetTime"`
}

type usagesLimit struct {
	Window *usagesWindow `json:"window"`
	Detail *usagesDetail `json:"detail"`
}

type usagesWindow struct {
	Duration int    `json:"duration"`
	TimeUnit string `json:"timeUnit"`
}

type usagesParallel struct {
	Limit   string   `json:"limit"`
	Details []string `json:"details"`
}

func FetchUsage(auth *ProfileAuth) (*model.UsageSnapshot, error) {
	req, err := http.NewRequest(http.MethodGet, defaultUsageURL, nil)
	if err != nil {
		return nil, fmt.Errorf("build Kimi usage request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+auth.AccessToken)
	req.Header.Set("User-Agent", defaultUserAgent)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("request Kimi usage: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read Kimi usage response: %w", err)
	}
	if resp.StatusCode >= 300 {
		return nil, fmt.Errorf("Kimi usage request failed with %d: %s", resp.StatusCode, string(body))
	}

	var payload usagesPayload
	if err := json.Unmarshal(body, &payload); err != nil {
		return nil, fmt.Errorf("parse Kimi usage payload: %w", err)
	}

	snapshot := convertUsagesPayload(&payload)
	
	// Update plan from membership level
	if payload.User != nil && payload.User.Membership != nil {
		auth.Plan = payload.User.Membership.Level
	}
	
	return snapshot, nil
}

func convertUsagesPayload(payload *usagesPayload) *model.UsageSnapshot {
	now := time.Now().UTC()
	snapshot := &model.UsageSnapshot{
		CapturedAt: now,
	}

	// Primary window = weekly limit (from usage field)
	if payload.Usage != nil {
		snapshot.Secondary = toUsageWindow(payload.Usage, 7*24*60) // weekly
	}

	// Find 5h limit from limits array
	for _, limit := range payload.Limits {
		if limit.Detail == nil {
			continue
		}
		windowMinutes := extractWindowMinutes(limit.Window)
		if windowMinutes == 5*60 { // 5 hours
			snapshot.Primary = toUsageWindow(limit.Detail, windowMinutes)
			break
		}
	}

	// If no 5h limit found, use the first limit as primary
	if snapshot.Primary == nil && len(payload.Limits) > 0 && payload.Limits[0].Detail != nil {
		windowMinutes := extractWindowMinutes(payload.Limits[0].Window)
		snapshot.Primary = toUsageWindow(payload.Limits[0].Detail, windowMinutes)
	}

	return snapshot
}

func extractWindowMinutes(window *usagesWindow) int {
	if window == nil {
		return 0
	}
	switch {
	case strings.Contains(window.TimeUnit, "MINUTE"):
		return window.Duration
	case strings.Contains(window.TimeUnit, "HOUR"):
		return window.Duration * 60
	case strings.Contains(window.TimeUnit, "DAY"):
		return window.Duration * 24 * 60
	default:
		return window.Duration
	}
}

func toUsageWindow(detail *usagesDetail, windowMinutes int) *model.UsageWindow {
	if detail == nil {
		return nil
	}

	limit := parseIntString(detail.Limit)
	used := parseIntString(detail.Used)
	remaining := parseIntString(detail.Remaining)

	if limit == 0 && used == 0 && remaining == 0 {
		return nil
	}

	// Calculate percentages
	var usedPercent float64
	var remainingPercent float64
	if limit > 0 {
		usedPercent = float64(used) / float64(limit) * 100
		remainingPercent = 100 - usedPercent
	} else if remaining >= 0 {
		remainingPercent = 100
		usedPercent = 0
	}

	var resetsAt *time.Time
	if detail.ResetTime != "" {
		if parsed, err := time.Parse(time.RFC3339Nano, detail.ResetTime); err == nil {
			value := parsed.UTC()
			resetsAt = &value
		}
	}

	return &model.UsageWindow{
		UsedPercent:      usedPercent,
		RemainingPercent: remainingPercent,
		WindowMinutes:    windowMinutes,
		ResetsAt:         resetsAt,
	}
}

func parseIntString(s string) int {
	v, _ := strconv.Atoi(strings.TrimSpace(s))
	return v
}
