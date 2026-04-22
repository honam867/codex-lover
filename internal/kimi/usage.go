package kimi

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
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
	snapshot, _, err := fetchUsageInternal(auth)
	if err == nil {
		return snapshot, nil
	}
	if respErr, ok := err.(*usageError); !ok || respErr.statusCode != http.StatusUnauthorized {
		return nil, err
	}
	if auth.RefreshToken == "" {
		return nil, err
	}
	if err := refreshToken(auth); err != nil {
		return nil, fmt.Errorf("refresh Kimi token: %w", err)
	}
	snapshot, _, err = fetchUsageInternal(auth)
	if err != nil {
		return nil, err
	}
	return snapshot, nil
}

type usageError struct {
	statusCode int
	body       string
}

func (e *usageError) Error() string {
	return fmt.Sprintf("Kimi usage request failed with %d: %s", e.statusCode, e.body)
}

func fetchUsageInternal(auth *ProfileAuth) (*model.UsageSnapshot, bool, error) {
	req, err := http.NewRequest(http.MethodGet, defaultUsageURL, nil)
	if err != nil {
		return nil, false, fmt.Errorf("build Kimi usage request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+auth.AccessToken)
	req.Header.Set("User-Agent", defaultUserAgent)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, false, fmt.Errorf("request Kimi usage: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, false, fmt.Errorf("read Kimi usage response: %w", err)
	}
	if resp.StatusCode >= 300 {
		return nil, false, &usageError{statusCode: resp.StatusCode, body: string(body)}
	}

	var payload usagesPayload
	if err := json.Unmarshal(body, &payload); err != nil {
		return nil, false, fmt.Errorf("parse Kimi usage payload: %w", err)
	}

	snapshot := convertUsagesPayload(&payload)
	
	// Update plan from membership level
	if payload.User != nil && payload.User.Membership != nil {
		auth.Plan = payload.User.Membership.Level
	}
	
	return snapshot, false, nil
}

const kimiCodeClientID = "17e5f671-d194-4dfb-9706-5516cb48c098"

func refreshToken(auth *ProfileAuth) error {
	data := url.Values{}
	data.Set("client_id", kimiCodeClientID)
	data.Set("grant_type", "refresh_token")
	data.Set("refresh_token", auth.RefreshToken)

	req, err := http.NewRequest(http.MethodPost, "https://auth.kimi.com/api/oauth/token", strings.NewReader(data.Encode()))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return fmt.Errorf("request: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("read: %w", err)
	}
	if resp.StatusCode >= 300 {
		return fmt.Errorf("refresh failed %d: %s", resp.StatusCode, string(respBody))
	}

	var result struct {
		AccessToken  string `json:"access_token"`
		RefreshToken string `json:"refresh_token"`
		ExpiresIn    int    `json:"expires_in"`
	}
	if err := json.Unmarshal(respBody, &result); err != nil {
		return fmt.Errorf("parse: %w", err)
	}

	auth.AccessToken = result.AccessToken
	if result.RefreshToken != "" {
		auth.RefreshToken = result.RefreshToken
	}

	// Persist refreshed tokens to the auth file
	if err := persistRefreshedTokens(auth); err != nil {
		return fmt.Errorf("persist: %w", err)
	}
	return nil
}

func persistRefreshedTokens(auth *ProfileAuth) error {
	authPath := filepath.Join(auth.HomePath, authFileName)
	data, err := os.ReadFile(authPath)
	if err != nil {
		return err
	}
	var f AuthFile
	if err := json.Unmarshal(data, &f); err != nil {
		return err
	}
	f.AccessToken = auth.AccessToken
	if auth.RefreshToken != "" {
		f.RefreshToken = auth.RefreshToken
	}
	out, _ := json.MarshalIndent(f, "", "  ")
	return os.WriteFile(authPath, out, 0o600)
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
