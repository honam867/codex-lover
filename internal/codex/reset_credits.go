package codex

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"path/filepath"
	"time"
)

// Rate-limit reset credits ("Đặt lại giới hạn sử dụng"). Contract mirrors the
// official Codex CLI backend client (codex-rs/backend-client rate_limit_resets.rs).
var resetCreditsBaseURL = "https://chatgpt.com/backend-api/wham/rate-limit-reset-credits"

const resetCreditsRequestTimeout = 15 * time.Second

var resetCreditsHTTPClient = &http.Client{Timeout: resetCreditsRequestTimeout}

const (
	ResetCodeReset           = "reset"
	ResetCodeNothingToReset  = "nothing_to_reset"
	ResetCodeNoCredit        = "no_credit"
	ResetCodeAlreadyRedeemed = "already_redeemed"
)

type ResetCredit struct {
	ID          string     `json:"id"`
	ResetType   string     `json:"reset_type"`
	Status      string     `json:"status"`
	GrantedAt   *time.Time `json:"granted_at"`
	ExpiresAt   *time.Time `json:"expires_at"`
	RedeemedAt  *time.Time `json:"redeemed_at"`
	Title       string     `json:"title"`
	Description string     `json:"description"`
}

type ResetCreditList struct {
	Credits        []ResetCredit `json:"credits"`
	AvailableCount int           `json:"available_count"`
}

type ResetCreditEvent struct {
	ID         string     `json:"id"`
	Kind       string     `json:"kind"`
	OccurredAt *time.Time `json:"occurred_at"`
}

type ResetCreditHistory struct {
	Events []ResetCreditEvent `json:"events"`
}

type ConsumeResetResult struct {
	Code         string `json:"code"`
	WindowsReset int    `json:"windows_reset"`
}

type consumeResetRequest struct {
	RedeemRequestID string `json:"redeem_request_id"`
	CreditID        string `json:"credit_id,omitempty"`
}

// ListResetCredits returns the account's reset credits. A non-nil *AuthFile
// means the token was refreshed and the caller must persist it.
func ListResetCredits(auth *ProfileAuth) (*ResetCreditList, *AuthFile, error) {
	var raw struct {
		Credits        []ResetCredit `json:"credits"`
		AvailableCount *int          `json:"available_count"`
	}
	refreshed, err := withTokenRefresh(auth, func(token string, accountID string) (int, error) {
		return doResetCreditsRequest(http.MethodGet, resetCreditsBaseURL, nil, token, accountID, &raw)
	})
	if err != nil {
		return nil, refreshed, err
	}
	list := &ResetCreditList{Credits: raw.Credits}
	if raw.AvailableCount != nil {
		list.AvailableCount = *raw.AvailableCount
	} else {
		list.AvailableCount = countAvailableCredits(raw.Credits)
	}
	return list, refreshed, nil
}

func countAvailableCredits(credits []ResetCredit) int {
	count := 0
	for _, credit := range credits {
		if credit.Status == "available" {
			count++
		}
	}
	return count
}

func ListResetCreditHistory(auth *ProfileAuth) (*ResetCreditHistory, *AuthFile, error) {
	var out ResetCreditHistory
	refreshed, err := withTokenRefresh(auth, func(token string, accountID string) (int, error) {
		return doResetCreditsRequest(http.MethodGet, resetCreditsBaseURL+"/history", nil, token, accountID, &out)
	})
	if err != nil {
		return nil, refreshed, err
	}
	return &out, refreshed, nil
}

// ConsumeResetCredit redeems one credit. redeemRequestID is the backend
// idempotency key, so the post-refresh retry cannot spend a second credit.
func ConsumeResetCredit(auth *ProfileAuth, redeemRequestID string, creditID string) (*ConsumeResetResult, *AuthFile, error) {
	body, err := json.Marshal(consumeResetRequest{RedeemRequestID: redeemRequestID, CreditID: creditID})
	if err != nil {
		return nil, nil, fmt.Errorf("marshal reset consume request: %w", err)
	}
	var out ConsumeResetResult
	refreshed, err := withTokenRefresh(auth, func(token string, accountID string) (int, error) {
		return doResetCreditsRequest(http.MethodPost, resetCreditsBaseURL+"/consume", body, token, accountID, &out)
	})
	if err != nil {
		return nil, refreshed, err
	}
	return &out, refreshed, nil
}

// withTokenRefresh runs call once and, on 401, refreshes the token and retries.
// The rotated tokens are returned even when the retry fails so they are not lost.
func withTokenRefresh(auth *ProfileAuth, call func(token string, accountID string) (int, error)) (*AuthFile, error) {
	status, err := call(auth.AccessToken, auth.AccountID)
	if err == nil {
		return nil, nil
	}
	if status != http.StatusUnauthorized || auth.RefreshToken == "" {
		return nil, err
	}

	refreshed, rerr := refreshAuth(auth)
	if rerr != nil {
		return nil, fmt.Errorf("unauthorized and refresh failed: %w", rerr)
	}
	auth.AccessToken = refreshed.AccessToken
	auth.RefreshToken = refreshed.RefreshToken
	if refreshed.AccountID != "" {
		auth.AccountID = refreshed.AccountID
	}
	refreshedFile := &AuthFile{
		Tokens: &TokenData{
			AccessToken:  refreshed.AccessToken,
			RefreshToken: refreshed.RefreshToken,
			AccountID:    refreshed.AccountID,
		},
		LastRefresh: ptrTime(time.Now().UTC()),
	}
	_, err = call(auth.AccessToken, auth.AccountID)
	return refreshedFile, err
}

func doResetCreditsRequest(method string, url string, body []byte, accessToken string, accountID string, out any) (int, error) {
	var reader io.Reader
	if body != nil {
		reader = bytes.NewReader(body)
	}
	req, err := http.NewRequest(method, url, reader)
	if err != nil {
		return 0, err
	}
	req.Header.Set("Authorization", "Bearer "+accessToken)
	req.Header.Set("User-Agent", defaultUserAgent)
	if accountID != "" {
		req.Header.Set("ChatGPT-Account-Id", accountID)
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}

	resp, err := resetCreditsHTTPClient.Do(req)
	if err != nil {
		return 0, fmt.Errorf("reset credits request: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return resp.StatusCode, fmt.Errorf("read reset credits response: %w", err)
	}
	if resp.StatusCode >= 300 {
		return resp.StatusCode, fmt.Errorf("reset credits request failed with %d: %s", resp.StatusCode, truncateForError(respBody))
	}
	if err := json.Unmarshal(respBody, out); err != nil {
		return resp.StatusCode, fmt.Errorf("parse reset credits response: %w", err)
	}
	return resp.StatusCode, nil
}

func truncateForError(body []byte) string {
	const limit = 300
	if len(body) > limit {
		return string(body[:limit]) + "..."
	}
	return string(body)
}

// PersistRefreshedAuthAtPath merges refreshed tokens into the auth file at authPath.
func PersistRefreshedAuthAtPath(authPath string, updated *AuthFile) error {
	return persistRefreshedTokensAtPath(authPath, updated)
}

func CachedAuthPath(cacheRoot string, profileID string) string {
	return cachedAuthPath(cacheRoot, profileID)
}

func HomeAuthPath(homePath string) string {
	return filepath.Join(homePath, authFileName)
}
