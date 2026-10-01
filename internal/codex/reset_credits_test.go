package codex

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func withResetCreditsServer(t *testing.T, handler http.HandlerFunc) {
	t.Helper()
	server := httptest.NewServer(handler)
	t.Cleanup(server.Close)
	old := resetCreditsBaseURL
	resetCreditsBaseURL = server.URL
	t.Cleanup(func() { resetCreditsBaseURL = old })
}

func TestListResetCreditsParsesCredits(t *testing.T) {
	var gotPath, gotAuth, gotAccount string
	withResetCreditsServer(t, func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		gotAuth = r.Header.Get("Authorization")
		gotAccount = r.Header.Get("ChatGPT-Account-Id")
		_, _ = w.Write([]byte(`{
			"credits": [{
				"id": "credit-1",
				"reset_type": "codex_rate_limits",
				"status": "available",
				"granted_at": "2026-09-22T19:54:14.042982Z",
				"expires_at": "2026-10-22T19:54:14.042982Z",
				"redeemed_at": null,
				"title": "Full reset (Weekly + 5 hr)",
				"description": "Ready"
			}],
			"available_count": 1
		}`))
	})

	auth := &ProfileAuth{AccessToken: "tok", AccountID: "acct-1"}
	list, refreshed, err := ListResetCredits(auth)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if refreshed != nil {
		t.Fatalf("did not expect a refresh")
	}
	if gotPath != "" && gotPath != "/" {
		t.Fatalf("list should hit the base URL, got path %q", gotPath)
	}
	if gotAuth != "Bearer tok" || gotAccount != "acct-1" {
		t.Fatalf("missing auth headers: %q %q", gotAuth, gotAccount)
	}
	if list.AvailableCount != 1 || len(list.Credits) != 1 {
		t.Fatalf("unexpected list: %+v", list)
	}
	credit := list.Credits[0]
	if credit.ID != "credit-1" || credit.Status != "available" || credit.Title != "Full reset (Weekly + 5 hr)" {
		t.Fatalf("unexpected credit: %+v", credit)
	}
	if credit.ExpiresAt == nil || credit.ExpiresAt.Day() != 22 || credit.RedeemedAt != nil {
		t.Fatalf("unexpected timestamps: %+v", credit)
	}
}

func TestListResetCreditsCountsAvailableWhenCountMissing(t *testing.T) {
	withResetCreditsServer(t, func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"credits":[{"id":"a","status":"available"},{"id":"b","status":"redeemed"},{"id":"c","status":"available"}]}`))
	})

	list, _, err := ListResetCredits(&ProfileAuth{AccessToken: "tok"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if list.AvailableCount != 2 {
		t.Fatalf("expected 2 available, got %d", list.AvailableCount)
	}
}

func TestListResetCreditHistoryParsesEvents(t *testing.T) {
	withResetCreditsServer(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/history" {
			t.Errorf("unexpected path %q", r.URL.Path)
		}
		_, _ = w.Write([]byte(`{"events":[
			{"id":"credit-1:used","kind":"used","occurred_at":"2026-10-01T06:48:51.0906Z"},
			{"id":"credit-1:granted","kind":"granted","occurred_at":"2026-09-04T23:02:41.636446Z"}
		],"next_cursor":null}`))
	})

	history, _, err := ListResetCreditHistory(&ProfileAuth{AccessToken: "tok"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(history.Events) != 2 || history.Events[0].Kind != "used" || history.Events[1].OccurredAt.Month() != 9 {
		t.Fatalf("unexpected history: %+v", history)
	}
}

func TestConsumeResetCreditSendsIdempotencyKeyAndCreditID(t *testing.T) {
	var gotMethod, gotPath, gotContentType string
	var gotBody map[string]string
	withResetCreditsServer(t, func(w http.ResponseWriter, r *http.Request) {
		gotMethod = r.Method
		gotPath = r.URL.Path
		gotContentType = r.Header.Get("Content-Type")
		raw, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(raw, &gotBody)
		_, _ = w.Write([]byte(`{"code":"reset","windows_reset":2}`))
	})

	result, _, err := ConsumeResetCredit(&ProfileAuth{AccessToken: "tok"}, "redeem-1", "credit-1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if gotMethod != http.MethodPost || gotPath != "/consume" || !strings.HasPrefix(gotContentType, "application/json") {
		t.Fatalf("unexpected request: %s %s %s", gotMethod, gotPath, gotContentType)
	}
	if gotBody["redeem_request_id"] != "redeem-1" || gotBody["credit_id"] != "credit-1" {
		t.Fatalf("unexpected body: %+v", gotBody)
	}
	if result.Code != ResetCodeReset || result.WindowsReset != 2 {
		t.Fatalf("unexpected result: %+v", result)
	}
}

func TestConsumeResetCreditRefreshesOn401AndReusesIdempotencyKey(t *testing.T) {
	var redeemIDs []string
	var auths []string
	withResetCreditsServer(t, func(w http.ResponseWriter, r *http.Request) {
		var body map[string]string
		raw, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(raw, &body)
		redeemIDs = append(redeemIDs, body["redeem_request_id"])
		auths = append(auths, r.Header.Get("Authorization"))
		if len(auths) == 1 {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		_, _ = w.Write([]byte(`{"code":"nothing_to_reset"}`))
	})
	refreshServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"access_token":"new-tok","refresh_token":"new-refresh"}`))
	}))
	defer refreshServer.Close()
	oldRefresh := refreshTokenURL
	refreshTokenURL = refreshServer.URL
	defer func() { refreshTokenURL = oldRefresh }()

	auth := &ProfileAuth{AccessToken: "old-tok", RefreshToken: "old-refresh"}
	result, refreshed, err := ConsumeResetCredit(auth, "redeem-9", "credit-1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Code != ResetCodeNothingToReset {
		t.Fatalf("unexpected code %q", result.Code)
	}
	if refreshed == nil || refreshed.Tokens.RefreshToken != "new-refresh" {
		t.Fatalf("expected rotated tokens to persist, got %+v", refreshed)
	}
	if len(auths) != 2 || auths[1] != "Bearer new-tok" {
		t.Fatalf("expected retry with new token, got %v", auths)
	}
	if redeemIDs[0] != "redeem-9" || redeemIDs[1] != "redeem-9" {
		t.Fatalf("retry must reuse the idempotency key, got %v", redeemIDs)
	}
}

func TestListResetCreditsReportsHTTPError(t *testing.T) {
	withResetCreditsServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusForbidden)
		_, _ = w.Write([]byte(`{"detail":"nope"}`))
	})

	_, _, err := ListResetCredits(&ProfileAuth{AccessToken: "tok"})
	if err == nil || !strings.Contains(err.Error(), "403") {
		t.Fatalf("expected 403 error, got %v", err)
	}
}

func TestConvertUsagePayloadReadsResetCreditCount(t *testing.T) {
	var payload usagePayload
	if err := json.Unmarshal([]byte(`{"plan_type":"plus","rate_limit_reset_credits":{"available_count":3}}`), &payload); err != nil {
		t.Fatal(err)
	}
	snapshot := convertUsagePayload(&payload)
	if snapshot.ResetCreditsAvailable == nil || *snapshot.ResetCreditsAvailable != 3 {
		t.Fatalf("expected 3 reset credits, got %v", snapshot.ResetCreditsAvailable)
	}

	for _, raw := range []string{`{"plan_type":"plus"}`, `{"rate_limit_reset_credits":{}}`} {
		var missing usagePayload
		_ = json.Unmarshal([]byte(raw), &missing)
		if convertUsagePayload(&missing).ResetCreditsAvailable != nil {
			t.Fatalf("missing count in %s must stay unknown (nil)", raw)
		}
	}
}
