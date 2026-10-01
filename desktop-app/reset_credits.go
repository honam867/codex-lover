package main

import (
	"fmt"
	"time"

	"codex-lover/internal/codex"
	"codex-lover/internal/model"
	"codex-lover/internal/service"
)

type ResetCreditItem struct {
	ID          string `json:"id"`
	Title       string `json:"title"`
	ExpiresText string `json:"expiresText"`
}

type ResetHistoryItem struct {
	ID     string `json:"id"`
	Kind   string `json:"kind"`
	Text   string `json:"text"`
	AtText string `json:"atText"`
}

type ResetCreditsView struct {
	AvailableCount int                `json:"availableCount"`
	Credits        []ResetCreditItem  `json:"credits"`
	History        []ResetHistoryItem `json:"history"`
	HistoryError   string             `json:"historyError,omitempty"`
	Error          string             `json:"error,omitempty"`
}

// GetResetCredits lists a Codex account's rate-limit reset credits and history.
func (a *App) GetResetCredits(profileID string) ResetCreditsView {
	if err := a.ensureReady(); err != nil {
		return ResetCreditsView{Error: "Desktop service is not ready"}
	}

	// The whole call holds a.mu: the background refresh loop may refresh the same
	// single-use refresh token, so the two must never run concurrently.
	a.mu.Lock()
	defer a.mu.Unlock()

	statuses, err := a.svc.ProfileStatuses()
	if err != nil {
		return ResetCreditsView{Error: err.Error()}
	}
	credits, err := a.svc.ListCodexResetCredits(statuses, profileID)
	if err != nil {
		return ResetCreditsView{Error: err.Error()}
	}
	// Best effort: the card count catches up on the next usage refresh anyway.
	_ = a.svc.SaveCodexResetCreditCount(profileID, credits.List.AvailableCount)

	return buildResetCreditsView(credits)
}

// UseResetCredit redeems one reset credit, then refreshes that account's usage.
func (a *App) UseResetCredit(profileID string, creditID string) ActionResponse {
	if err := a.ensureReady(); err != nil {
		return ActionResponse{Message: "Reset failed", Error: "Desktop service is not ready", Snapshot: a.mustSnapshotFallback()}
	}

	// Held across the network call for the same token-rotation reason as GetResetCredits.
	a.mu.Lock()
	statuses, err := a.svc.ProfileStatuses()
	if err != nil {
		a.mu.Unlock()
		return ActionResponse{Message: "Reset failed", Error: err.Error(), Snapshot: a.mustSnapshotFallback()}
	}
	result, consumeErr := a.svc.ConsumeCodexResetCredit(statuses, profileID, creditID)
	wasReset := result != nil && result.Code == codex.ResetCodeReset
	isActive := profileIsActive(statuses, profileID)
	if wasReset && !isActive {
		_, _, _ = a.svc.RefreshLoggedOutCachedUsageForProfile(statuses, profileID)
	}
	a.mu.Unlock()

	if result == nil {
		// A timeout or unreadable reply may still have spent the credit server-side.
		return ActionResponse{
			Message:  "Reset failed",
			Error:    "Không chắc lượt đã được dùng hay chưa (" + errorText(consumeErr) + "). Kiểm tra lại danh sách và lịch sử.",
			Snapshot: a.mustSnapshotFallback(),
		}
	}

	message, isErr := resetConsumeMessage(*result)
	snapshot, err := a.snapshot(wasReset && isActive)
	if err != nil {
		snapshot = a.mustSnapshotFallback()
	}

	response := ActionResponse{Message: message, Snapshot: snapshot}
	switch {
	case consumeErr != nil:
		response.Error = consumeErr.Error()
	case isErr:
		response.Error = message
	}
	return response
}

func profileIsActive(statuses []model.ProfileStatus, profileID string) bool {
	for _, status := range statuses {
		if status.Profile.ID == profileID {
			return status.State.AuthStatus == model.AuthStatusActive
		}
	}
	return false
}

func errorText(err error) string {
	if err == nil {
		return "unknown error"
	}
	return err.Error()
}

func buildResetCreditsView(credits service.CodexResetCredits) ResetCreditsView {
	view := ResetCreditsView{
		Credits:      []ResetCreditItem{},
		History:      []ResetHistoryItem{},
		HistoryError: credits.HistoryError,
	}
	if credits.List != nil {
		view.AvailableCount = credits.List.AvailableCount
		for _, credit := range credits.List.Credits {
			if credit.Status != "available" {
				continue
			}
			item := ResetCreditItem{ID: credit.ID, Title: resetCreditTitleVI(credit.Title)}
			if expires := formatVNDayMonth(credit.ExpiresAt); expires != "" {
				item.ExpiresText = "Hết hạn vào " + expires
			}
			view.Credits = append(view.Credits, item)
		}
	}
	if credits.History != nil {
		for _, event := range credits.History.Events {
			view.History = append(view.History, ResetHistoryItem{
				ID:     event.ID,
				Kind:   event.Kind,
				Text:   resetHistoryEventText(event.Kind),
				AtText: formatVNDateTime(event.OccurredAt),
			})
		}
	}
	return view
}

func resetCreditTitleVI(title string) string {
	switch title {
	case "":
		return "Lượt đặt lại"
	case "Full reset (Weekly + 5 hr)":
		return "Đặt lại toàn bộ (Hằng tuần + 5 giờ)"
	default:
		return title
	}
}

func resetHistoryEventText(kind string) string {
	switch kind {
	case "used":
		return "Đã dùng một lượt đặt lại"
	case "granted":
		return "Nhận một lượt đặt lại"
	case "expired":
		return "Một lượt đặt lại đã hết hạn"
	default:
		return kind
	}
}

func resetConsumeMessage(result codex.ConsumeResetResult) (string, bool) {
	switch result.Code {
	case codex.ResetCodeReset:
		if result.WindowsReset > 0 {
			return fmt.Sprintf("Đã đặt lại giới hạn sử dụng (%d cửa sổ)", result.WindowsReset), false
		}
		return "Đã đặt lại giới hạn sử dụng", false
	case codex.ResetCodeNothingToReset:
		return "Không có giới hạn nào cần đặt lại", true
	case codex.ResetCodeNoCredit:
		return "Tài khoản đã hết lượt đặt lại", true
	case codex.ResetCodeAlreadyRedeemed:
		return "Lượt đặt lại này đã được dùng trước đó", true
	default:
		return "Kết quả không xác định: " + result.Code, true
	}
}

func formatVNDayMonth(value *time.Time) string {
	if value == nil {
		return ""
	}
	local := value.Local()
	return fmt.Sprintf("%d tháng %d", local.Day(), int(local.Month()))
}

func formatVNDateTime(value *time.Time) string {
	if value == nil {
		return ""
	}
	local := value.Local()
	return fmt.Sprintf("%d tháng %d, %s", local.Day(), int(local.Month()), local.Format("15:04"))
}
