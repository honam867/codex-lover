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
	"sort"
	"strings"
	"time"
)

var (
	responsesURL = "https://chatgpt.com/backend-api/codex/responses"
	modelsURL    = "https://chatgpt.com/backend-api/codex/models?client_version=0.160.1"

	// DefaultTriggerModels is only the fallback when the server's model list
	// cannot be read. OpenAI renames ChatGPT-account Codex models often (from
	// 2026-11 gpt-5.5 and gpt-5.4 return 400 model_not_found), so the live list
	// from modelsURL is used whenever possible.
	DefaultTriggerModels = []string{"gpt-5.6-luna", "gpt-6-luna"}
)

// TriggerModels returns the models this account may use on /responses, lowest
// priority (smallest) first, or DefaultTriggerModels if the list is unavailable.
func TriggerModels(accessToken string, accountID string) []string {
	req, err := http.NewRequest(http.MethodGet, modelsURL, nil)
	if err != nil {
		return DefaultTriggerModels
	}
	req.Header.Set("Authorization", "Bearer "+accessToken)
	req.Header.Set("User-Agent", defaultUserAgent)
	req.Header.Set("originator", "codex_cli_rs")
	if accountID != "" {
		req.Header.Set("chatgpt-account-id", accountID)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return DefaultTriggerModels
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return DefaultTriggerModels
	}
	var payload struct {
		Models []struct {
			Slug           string `json:"slug"`
			Visibility     string `json:"visibility"`
			SupportedInAPI bool   `json:"supported_in_api"`
			Priority       int    `json:"priority"`
		} `json:"models"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		return DefaultTriggerModels
	}
	listed := payload.Models[:0]
	for _, m := range payload.Models {
		if m.Slug != "" && m.Visibility == "list" && m.SupportedInAPI {
			listed = append(listed, m)
		}
	}
	if len(listed) == 0 {
		return DefaultTriggerModels
	}
	sort.SliceStable(listed, func(i, j int) bool { return listed[i].Priority > listed[j].Priority })
	models := make([]string, 0, len(listed))
	for _, m := range listed {
		models = append(models, m.Slug)
	}
	return models
}

// TriggerResult reports the outcome of a successful trigger.
type TriggerResult struct {
	ModelUsed string
	Status    int
}

type TriggerError struct {
	StatusCode int
	Model      string
	Message    string
	Cause      error
}

func (e *TriggerError) Error() string {
	if e == nil {
		return "trigger error"
	}
	if e.Model != "" {
		return fmt.Sprintf("%s for model %s", e.Message, e.Model)
	}
	return e.Message
}

func (e *TriggerError) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.Cause
}

// TriggerWindow sends one minimal request so the account's 5h quota window
// opens. It tries models in order until one is accepted, and refreshes the
// token once on 401. With no models given it asks the server which ones the
// account may use. On refresh it returns a non-nil *AuthFile the caller must
// persist. The active account is never touched.
func TriggerWindow(auth *ProfileAuth, models []string) (*TriggerResult, *AuthFile, error) {
	pick := func(accessToken, accountID string) []string {
		if len(models) > 0 {
			return models
		}
		return TriggerModels(accessToken, accountID)
	}
	result, status, err := doTriggerOnce(auth.AccessToken, auth.AccountID, pick(auth.AccessToken, auth.AccountID))
	if err == nil {
		return result, nil, nil
	}
	if status != http.StatusUnauthorized || auth.RefreshToken == "" {
		return nil, nil, err
	}

	refreshed, rerr := refreshAuth(auth)
	if rerr != nil {
		return nil, nil, &TriggerError{StatusCode: http.StatusUnauthorized, Message: "trigger unauthorized and refresh failed", Cause: rerr}
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
	result, _, err = doTriggerOnce(refreshed.AccessToken, refreshed.AccountID, pick(refreshed.AccessToken, refreshed.AccountID))
	if err != nil {
		// The refresh succeeded and may have rotated the refresh token; return the
		// new tokens so the caller persists them even though the trigger failed.
		return nil, refreshedFile, err
	}
	return result, refreshedFile, nil
}

// TriggerHealthProbe deliberately uses a single model: a fallback would turn a
// 429 (limited) into a later model's error and mark the account Dead.
func TriggerHealthProbe(auth *ProfileAuth) (*TriggerResult, *AuthFile, error) {
	return TriggerWindow(auth, TriggerModels(auth.AccessToken, auth.AccountID)[:1])
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
		var detail []byte
		if resp.StatusCode >= 300 {
			detail, _ = io.ReadAll(io.LimitReader(resp.Body, 300))
		}
		_, _ = io.Copy(io.Discard, resp.Body)
		resp.Body.Close()
		lastStatus = resp.StatusCode

		if resp.StatusCode >= 200 && resp.StatusCode < 300 {
			return &TriggerResult{ModelUsed: modelName, Status: resp.StatusCode}, resp.StatusCode, nil
		}
		if resp.StatusCode == http.StatusUnauthorized {
			return nil, resp.StatusCode, &TriggerError{StatusCode: resp.StatusCode, Model: modelName, Message: "trigger unauthorized"}
		}
		message := fmt.Sprintf("trigger failed with %d", resp.StatusCode)
		if text := strings.TrimSpace(string(detail)); text != "" {
			message += " (" + text + ")" // server reason, e.g. model_not_found
		}
		lastErr = &TriggerError{StatusCode: resp.StatusCode, Model: modelName, Message: message}
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
		"reasoning": map[string]any{"effort": "low"},
		"store":     false,
		"stream":    true,
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
	result, authFile, triggerErr := TriggerWindow(auth, models)
	if authFile != nil {
		if perr := persistRefreshedTokensAtPath(authPath, authFile); perr != nil {
			if triggerErr != nil {
				return nil, triggerErr
			}
			return nil, perr
		}
	}
	if triggerErr != nil {
		return nil, triggerErr
	}
	return result, nil
}

func TriggerHealthProbeFromCachedAuth(cacheRoot string, profileID string) (*TriggerResult, error) {
	authPath := cachedAuthPath(cacheRoot, profileID)
	auth, err := LoadCachedProfileAuth(cacheRoot, profileID)
	if err != nil {
		return nil, err
	}
	result, authFile, triggerErr := TriggerHealthProbe(auth)
	if authFile != nil {
		if perr := persistRefreshedTokensAtPath(authPath, authFile); perr != nil {
			if triggerErr != nil {
				return nil, triggerErr
			}
			return nil, perr
		}
	}
	if triggerErr != nil {
		return nil, triggerErr
	}
	return result, nil
}
