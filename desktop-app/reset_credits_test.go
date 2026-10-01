package main

import (
	"testing"
	"time"

	"codex-lover/internal/codex"
	"codex-lover/internal/model"
	"codex-lover/internal/service"
)

func TestResetCreditTitleVI(t *testing.T) {
	tests := map[string]string{
		"Full reset (Weekly + 5 hr)": "Đặt lại toàn bộ (Hằng tuần + 5 giờ)",
		"":                           "Lượt đặt lại",
		"Something new":              "Something new",
	}
	for input, want := range tests {
		if got := resetCreditTitleVI(input); got != want {
			t.Errorf("resetCreditTitleVI(%q) = %q, want %q", input, got, want)
		}
	}
}

func TestFormatVNDayMonth(t *testing.T) {
	value := time.Date(2026, 10, 23, 2, 54, 0, 0, time.Local)
	if got := formatVNDayMonth(&value); got != "23 tháng 10" {
		t.Fatalf("got %q", got)
	}
	if got := formatVNDayMonth(nil); got != "" {
		t.Fatalf("nil time must format empty, got %q", got)
	}
}

func TestResetConsumeMessage(t *testing.T) {
	tests := []struct {
		result  codex.ConsumeResetResult
		wantErr bool
		want    string
	}{
		{codex.ConsumeResetResult{Code: codex.ResetCodeReset, WindowsReset: 2}, false, "Đã đặt lại giới hạn sử dụng (2 cửa sổ)"},
		{codex.ConsumeResetResult{Code: codex.ResetCodeReset}, false, "Đã đặt lại giới hạn sử dụng"},
		{codex.ConsumeResetResult{Code: codex.ResetCodeNothingToReset}, true, "Không có giới hạn nào cần đặt lại"},
		{codex.ConsumeResetResult{Code: codex.ResetCodeNoCredit}, true, "Tài khoản đã hết lượt đặt lại"},
		{codex.ConsumeResetResult{Code: codex.ResetCodeAlreadyRedeemed}, true, "Lượt đặt lại này đã được dùng trước đó"},
		{codex.ConsumeResetResult{Code: "weird"}, true, "Kết quả không xác định: weird"},
	}
	for _, tt := range tests {
		got, isErr := resetConsumeMessage(tt.result)
		if got != tt.want || isErr != tt.wantErr {
			t.Errorf("resetConsumeMessage(%+v) = %q,%v want %q,%v", tt.result, got, isErr, tt.want, tt.wantErr)
		}
	}
}

func TestBuildResetCreditsView(t *testing.T) {
	expires := time.Date(2026, 10, 30, 10, 0, 0, 0, time.Local)
	usedAt := time.Date(2026, 10, 1, 13, 48, 0, 0, time.Local)
	view := buildResetCreditsView(service.CodexResetCredits{
		List: &codex.ResetCreditList{
			AvailableCount: 1,
			Credits: []codex.ResetCredit{
				{ID: "c1", Status: "available", Title: "Full reset (Weekly + 5 hr)", ExpiresAt: &expires},
				{ID: "c2", Status: "redeemed", Title: "Full reset (Weekly + 5 hr)"},
			},
		},
		History: &codex.ResetCreditHistory{Events: []codex.ResetCreditEvent{
			{ID: "c0:used", Kind: "used", OccurredAt: &usedAt},
			{ID: "c0:granted", Kind: "granted"},
		}},
	})

	if view.AvailableCount != 1 || len(view.Credits) != 1 {
		t.Fatalf("only available credits must be listed: %+v", view)
	}
	if view.Credits[0].ID != "c1" || view.Credits[0].ExpiresText != "Hết hạn vào 30 tháng 10" {
		t.Fatalf("unexpected credit: %+v", view.Credits[0])
	}
	if len(view.History) != 2 || view.History[0].Text != "Đã dùng một lượt đặt lại" || view.History[0].AtText != "1 tháng 10, 13:48" {
		t.Fatalf("unexpected history: %+v", view.History)
	}
	if view.History[1].Text != "Nhận một lượt đặt lại" || view.History[1].AtText != "" {
		t.Fatalf("unexpected granted event: %+v", view.History[1])
	}
}

func TestBuildSnapshotExposesCodexResetCreditCount(t *testing.T) {
	four := 4
	statuses := []model.ProfileStatus{
		{
			Profile: model.Profile{ID: "codex-a", Tool: model.ToolCodex},
			State:   model.ProfileState{Usage: &model.UsageSnapshot{ResetCreditsAvailable: &four}},
		},
		{
			Profile: model.Profile{ID: "codex-b", Tool: model.ToolCodex},
			State:   model.ProfileState{Usage: &model.UsageSnapshot{}},
		},
		{
			Profile: model.Profile{ID: "claude-a", Tool: model.ToolClaude},
			State:   model.ProfileState{Usage: &model.UsageSnapshot{ResetCreditsAvailable: &four}},
		},
	}
	snapshot := buildSnapshot(statuses, nil)
	if got := snapshot.Profiles[0].ResetCreditsAvailable; got == nil || *got != 4 {
		t.Fatalf("codex-a should expose 4, got %v", got)
	}
	if snapshot.Profiles[1].ResetCreditsAvailable != nil {
		t.Fatalf("unknown count must stay nil")
	}
	if snapshot.Profiles[2].ResetCreditsAvailable != nil {
		t.Fatalf("non-codex accounts must not expose reset credits")
	}
}
