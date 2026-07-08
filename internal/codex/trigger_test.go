package codex

import (
	"encoding/json"
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

func TestTriggerWindowRefreshOn401Succeeds(t *testing.T) {
	respServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") == "Bearer old-token" {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer respServer.Close()
	refreshServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"access_token":"new-token","refresh_token":"new-refresh"}`))
	}))
	defer refreshServer.Close()

	oldResp, oldRefresh := responsesURL, refreshTokenURL
	responsesURL, refreshTokenURL = respServer.URL, refreshServer.URL
	defer func() { responsesURL, refreshTokenURL = oldResp, oldRefresh }()

	auth := &ProfileAuth{AccessToken: "old-token", RefreshToken: "old-refresh", AccountID: "a"}
	result, refreshedFile, err := TriggerWindow(auth, []string{"m1"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result == nil || refreshedFile == nil {
		t.Fatalf("expected result and a refreshed AuthFile")
	}
	if refreshedFile.Tokens.AccessToken != "new-token" || refreshedFile.Tokens.RefreshToken != "new-refresh" {
		t.Fatalf("AuthFile must carry the new tokens, got %+v", refreshedFile.Tokens)
	}
	if auth.AccessToken != "new-token" {
		t.Fatalf("auth should be updated in place to new-token")
	}
}

func TestTriggerWindowRetryFailStillReturnsRefreshedToken(t *testing.T) {
	respServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") == "Bearer old-token" {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		w.WriteHeader(http.StatusBadRequest) // post-refresh retry fails (non-401)
	}))
	defer respServer.Close()
	refreshServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"access_token":"new-token","refresh_token":"new-refresh"}`))
	}))
	defer refreshServer.Close()

	oldResp, oldRefresh := responsesURL, refreshTokenURL
	responsesURL, refreshTokenURL = respServer.URL, refreshServer.URL
	defer func() { responsesURL, refreshTokenURL = oldResp, oldRefresh }()

	auth := &ProfileAuth{AccessToken: "old-token", RefreshToken: "old-refresh", AccountID: "a"}
	result, refreshedFile, err := TriggerWindow(auth, []string{"m1"})
	if err == nil {
		t.Fatalf("expected error when post-refresh retry fails")
	}
	if result != nil {
		t.Fatalf("expected nil result on failure")
	}
	if refreshedFile == nil || refreshedFile.Tokens.AccessToken != "new-token" {
		t.Fatalf("refreshed token must be returned for persistence even when retry fails, got %+v", refreshedFile)
	}
}

func TestBuildTriggerBodyPayload(t *testing.T) {
	raw, err := buildTriggerBody("gpt-5.4-mini")
	if err != nil {
		t.Fatalf("buildTriggerBody error: %v", err)
	}
	var payload map[string]any
	if err := json.Unmarshal(raw, &payload); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if payload["model"] != "gpt-5.4-mini" {
		t.Fatalf("model = %v", payload["model"])
	}
	if _, present := payload["max_output_tokens"]; present {
		t.Fatalf("max_output_tokens must not be present (endpoint rejects it)")
	}
	reasoning, ok := payload["reasoning"].(map[string]any)
	if !ok || reasoning["effort"] != "low" {
		t.Fatalf("reasoning.effort must be \"low\", got %v", payload["reasoning"])
	}
}
