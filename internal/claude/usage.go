package claude

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"codex-lover/internal/model"
)

type usagePayload struct {
	FiveHour      *usageWindowRaw `json:"five_hour"`
	SevenDay      *usageWindowRaw `json:"seven_day"`
	ExtraUsage    *extraUsageRaw  `json:"extra_usage"`
	SevenDayOpus  *usageWindowRaw `json:"seven_day_opus"`
	SevenDaySonnet *usageWindowRaw `json:"seven_day_sonnet"`
}

type usageWindowRaw struct {
	Utilization float64 `json:"utilization"`
	ResetsAt    string  `json:"resets_at"`
}

type extraUsageRaw struct {
	IsEnabled   bool    `json:"is_enabled"`
	MonthlyLimit float64 `json:"monthly_limit"`
	UsedCredits float64 `json:"used_credits"`
	Utilization float64 `json:"utilization"`
	Currency    string  `json:"currency"`
}

func FetchUsage(auth *ProfileAuth) (*model.UsageSnapshot, error) {
	req, err := http.NewRequest(http.MethodGet, defaultUsageURL, nil)
	if err != nil {
		return nil, fmt.Errorf("build Claude usage request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+auth.AccessToken)
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Accept-Encoding", "identity")
	req.Header.Set("User-Agent", defaultUserAgent)
	req.Header.Set("anthropic-beta", defaultOAuthBeta)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("request Claude usage: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read Claude usage response: %w", err)
	}
	if resp.StatusCode >= 300 {
		return nil, fmt.Errorf("Claude usage request failed with %d: %s", resp.StatusCode, string(body))
	}

	var payload usagePayload
	if err := json.Unmarshal(body, &payload); err != nil {
		return nil, fmt.Errorf("parse Claude usage payload: %w", err)
	}

	snapshot := &model.UsageSnapshot{
		PlanType:   auth.Plan,
		Primary:    toUsageWindow(payload.FiveHour, defaultWindowMinutes5),
		Secondary:  toUsageWindow(payload.SevenDay, defaultWindowMinutes7),
		CapturedAt: time.Now().UTC(),
	}
	if payload.ExtraUsage != nil && payload.ExtraUsage.IsEnabled {
		snapshot.Credits = &model.CreditsSnapshot{
			HasCredits: true,
			Balance:    formatExtraUsageBalance(payload.ExtraUsage),
		}
	}
	return snapshot, nil
}

func toUsageWindow(raw *usageWindowRaw, windowMinutes int) *model.UsageWindow {
	if raw == nil {
		return nil
	}
	used := raw.Utilization
	if used < 0 {
		used = 0
	}
	if used > 100 {
		used = 100
	}
	remaining := 100 - used

	var resetsAt *time.Time
	if raw.ResetsAt != "" {
		if parsed, err := time.Parse(time.RFC3339Nano, raw.ResetsAt); err == nil {
			value := parsed.UTC()
			resetsAt = &value
		}
	}

	return &model.UsageWindow{
		UsedPercent:      used,
		RemainingPercent: remaining,
		WindowMinutes:    windowMinutes,
		ResetsAt:         resetsAt,
	}
}

func formatExtraUsageBalance(value *extraUsageRaw) string {
	if value == nil {
		return ""
	}
	return fmt.Sprintf("%.0f/%.0f %s", value.UsedCredits, value.MonthlyLimit, value.Currency)
}
