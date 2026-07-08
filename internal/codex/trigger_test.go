package codex

import (
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
