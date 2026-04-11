package codex

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"codex-lover/internal/model"
)

type usagePayload struct {
	PlanType             string                `json:"plan_type"`
	RateLimit            *rateLimitStatus      `json:"rate_limit"`
	Credits              *creditStatus         `json:"credits"`
	AdditionalRateLimits []additionalRateLimit `json:"additional_rate_limits"`
}

type rateLimitStatus struct {
	Allowed         bool                `json:"allowed"`
	LimitReached    bool                `json:"limit_reached"`
	PrimaryWindow   *rateLimitWindowRaw `json:"primary_window"`
	SecondaryWindow *rateLimitWindowRaw `json:"secondary_window"`
}

type rateLimitWindowRaw struct {
	UsedPercent        int `json:"used_percent"`
	LimitWindowSeconds int `json:"limit_window_seconds"`
	ResetAt            int `json:"reset_at"`
}

type creditStatus struct {
	HasCredits bool    `json:"has_credits"`
	Unlimited  bool    `json:"unlimited"`
	Balance    *string `json:"balance"`
}

type additionalRateLimit struct {
	LimitName      string           `json:"limit_name"`
	MeteredFeature string           `json:"metered_feature"`
	RateLimit      *rateLimitStatus `json:"rate_limit"`
}

type refreshRequest struct {
	ClientID     string `json:"client_id"`
	GrantType    string `json:"grant_type"`
	RefreshToken string `json:"refresh_token"`
}

type refreshResponse struct {
	IDToken      string `json:"id_token"`
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token"`
}

func FetchUsage(auth *ProfileAuth) (*model.UsageSnapshot, error) {
	snapshot, authFile, err := fetchUsageWithRefresh(auth)
	if err != nil {
		return nil, err
	}
	if authFile != nil {
		if err := persistRefreshedTokens(auth.HomePath, authFile); err != nil {
			return nil, err
		}
	}
	return snapshot, nil
}

func fetchUsageWithRefresh(auth *ProfileAuth) (*model.UsageSnapshot, *AuthFile, error) {
	payload, statusCode, err := doUsageRequest(auth.AccessToken, auth.AccountID)
	if err == nil {
		return convertUsagePayload(payload), nil, nil
	}
	if statusCode != http.StatusUnauthorized || auth.RefreshToken == "" {
		return nil, nil, err
	}

	refreshed, err := refreshAuth(auth)
	if err != nil {
		return nil, nil, fmt.Errorf("usage unauthorized and refresh failed: %w", err)
	}

	payload, _, err = doUsageRequest(refreshed.AccessToken, refreshed.AccountID)
	if err != nil {
		return nil, nil, err
	}

	auth.AccessToken = refreshed.AccessToken
	auth.RefreshToken = refreshed.RefreshToken
	if refreshed.AccountID != "" {
		auth.AccountID = refreshed.AccountID
	}
	if refreshed.Email != "" {
		auth.Email = refreshed.Email
	}
	if refreshed.Plan != "" {
		auth.Plan = refreshed.Plan
	}

	return convertUsagePayload(payload), &AuthFile{
		Tokens: &TokenData{
			IDToken:      refreshedTokenIDToken(refreshed),
			AccessToken:  refreshed.AccessToken,
			RefreshToken: refreshed.RefreshToken,
			AccountID:    refreshed.AccountID,
		},
		LastRefresh: ptrTime(time.Now().UTC()),
	}, nil
}

func doUsageRequest(accessToken string, accountID string) (*usagePayload, int, error) {
	req, err := http.NewRequest(http.MethodGet, defaultUsageURL, nil)
	if err != nil {
		return nil, 0, err
	}
	req.Header.Set("Authorization", "Bearer "+accessToken)
	req.Header.Set("User-Agent", defaultUserAgent)
	if accountID != "" {
		req.Header.Set("ChatGPT-Account-Id", accountID)
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, 0, fmt.Errorf("request usage: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, resp.StatusCode, fmt.Errorf("read usage response: %w", err)
	}
	if resp.StatusCode >= 300 {
		return nil, resp.StatusCode, fmt.Errorf("usage request failed with %d: %s", resp.StatusCode, string(body))
	}

	var payload usagePayload
	if err := json.Unmarshal(body, &payload); err != nil {
		return nil, resp.StatusCode, fmt.Errorf("parse usage payload: %w", err)
	}
	return &payload, resp.StatusCode, nil
}

func refreshAuth(auth *ProfileAuth) (*ProfileAuth, error) {
	body, err := json.Marshal(refreshRequest{
		ClientID:     refreshClientID,
		GrantType:    "refresh_token",
		RefreshToken: auth.RefreshToken,
	})
	if err != nil {
		return nil, fmt.Errorf("marshal refresh request: %w", err)
	}
	req, err := http.NewRequest(http.MethodPost, refreshTokenURL, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("build refresh request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", defaultUserAgent)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("refresh token request: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read refresh response: %w", err)
	}
	if resp.StatusCode >= 300 {
		return nil, fmt.Errorf("refresh token failed with %d: %s", resp.StatusCode, string(respBody))
	}

	var refreshed refreshResponse
	if err := json.Unmarshal(respBody, &refreshed); err != nil {
		return nil, fmt.Errorf("parse refresh response: %w", err)
	}

	claims, err := ParseIDClaims(refreshed.IDToken)
	if err != nil && refreshed.IDToken != "" {
		return nil, err
	}

	accountID := auth.AccountID
	if claims.ChatGPTAccountID != "" {
		accountID = claims.ChatGPTAccountID
	}

	return &ProfileAuth{
		HomePath:     auth.HomePath,
		AccessToken:  chooseNonEmpty(refreshed.AccessToken, auth.AccessToken),
		RefreshToken: chooseNonEmpty(refreshed.RefreshToken, auth.RefreshToken),
		AccountID:    accountID,
		Email:        chooseNonEmpty(claims.Email, auth.Email),
		Plan:         chooseNonEmpty(claims.ChatGPTPlanType, auth.Plan),
	}, nil
}

func persistRefreshedTokens(homePath string, updated *AuthFile) error {
	authPath := filepath.Join(homePath, authFileName)
	currentBytes, err := os.ReadFile(authPath)
	if err != nil {
		return fmt.Errorf("read auth for persist: %w", err)
	}
	var current AuthFile
	if err := json.Unmarshal(currentBytes, &current); err != nil {
		return fmt.Errorf("parse auth for persist: %w", err)
	}
	if current.Tokens == nil {
		current.Tokens = &TokenData{}
	}
	if updated.Tokens != nil {
		if updated.Tokens.IDToken != "" {
			current.Tokens.IDToken = updated.Tokens.IDToken
		}
		if updated.Tokens.AccessToken != "" {
			current.Tokens.AccessToken = updated.Tokens.AccessToken
		}
		if updated.Tokens.RefreshToken != "" {
			current.Tokens.RefreshToken = updated.Tokens.RefreshToken
		}
		if updated.Tokens.AccountID != "" {
			current.Tokens.AccountID = updated.Tokens.AccountID
		}
	}
	current.LastRefresh = updated.LastRefresh

	encoded, err := json.MarshalIndent(current, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal updated auth: %w", err)
	}
	return os.WriteFile(authPath, encoded, 0o644)
}

func convertUsagePayload(payload *usagePayload) *model.UsageSnapshot {
	now := time.Now().UTC()
	snapshot := &model.UsageSnapshot{
		PlanType:   payload.PlanType,
		CapturedAt: now,
	}
	if payload.RateLimit != nil {
		snapshot.Primary = toUsageWindow(payload.RateLimit.PrimaryWindow)
		snapshot.Secondary = toUsageWindow(payload.RateLimit.SecondaryWindow)
	}
	if payload.Credits != nil {
		snapshot.Credits = &model.CreditsSnapshot{
			HasCredits: payload.Credits.HasCredits,
			Unlimited:  payload.Credits.Unlimited,
			Balance:    derefString(payload.Credits.Balance),
		}
	}
	for _, item := range payload.AdditionalRateLimits {
		limit := model.AdditionalLimit{
			LimitName: chooseNonEmpty(item.LimitName, item.MeteredFeature),
		}
		if item.RateLimit != nil {
			limit.Primary = toUsageWindow(item.RateLimit.PrimaryWindow)
			limit.Secondary = toUsageWindow(item.RateLimit.SecondaryWindow)
		}
		snapshot.AdditionalLimits = append(snapshot.AdditionalLimits, limit)
	}
	return snapshot
}

func toUsageWindow(raw *rateLimitWindowRaw) *model.UsageWindow {
	if raw == nil {
		return nil
	}
	used := float64(raw.UsedPercent)
	remaining := 100.0 - used
	windowMinutes := raw.LimitWindowSeconds / 60
	var resetsAt *time.Time
	if raw.ResetAt > 0 {
		value := time.Unix(int64(raw.ResetAt), 0).UTC()
		resetsAt = &value
	}
	return &model.UsageWindow{
		UsedPercent:      used,
		RemainingPercent: remaining,
		WindowMinutes:    windowMinutes,
		ResetsAt:         resetsAt,
	}
}

func ptrTime(t time.Time) *time.Time {
	return &t
}

func derefString(value *string) string {
	if value == nil {
		return ""
	}
	return *value
}

func chooseNonEmpty(primary string, fallback string) string {
	if primary != "" {
		return primary
	}
	return fallback
}

func refreshedTokenIDToken(auth *ProfileAuth) string {
	_ = auth
	return ""
}
