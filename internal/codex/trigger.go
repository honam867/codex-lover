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

	// DefaultTriggerModels is the cheapest-first preference order for
	// ChatGPT-account Codex, confirmed by the Phase 0 live probe. Only these
	// models are accepted on the /responses endpoint for a ChatGPT account.
	DefaultTriggerModels = []string{"gpt-5.4-mini", "gpt-5.5", "gpt-5.4"}
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
	result, _, err = doTriggerOnce(refreshed.AccessToken, refreshed.AccountID, models)
	if err != nil {
		// The refresh succeeded and may have rotated the refresh token; return the
		// new tokens so the caller persists them even though the trigger failed.
		return nil, refreshedFile, err
	}
	return result, refreshedFile, nil
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
