package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	wailsruntime "github.com/wailsapp/wails/v2/pkg/runtime"
	"github.com/xuri/excelize/v2"
)

func (a *App) ExportProfilesToExcel(cards []ProfileCard) ActionResponse {
	if err := a.ensureReady(); err != nil {
		return ActionResponse{Message: "Export failed", Error: err.Error(), Snapshot: a.mustSnapshotFallback()}
	}
	if len(cards) == 0 {
		return ActionResponse{Message: "Export failed", Error: "No visible accounts to export", Snapshot: a.mustSnapshotFallback()}
	}

	path, err := wailsruntime.SaveFileDialog(a.ctx, wailsruntime.SaveDialogOptions{
		Title:                "Export Excel",
		DefaultDirectory:     defaultExportDirectory(),
		DefaultFilename:      defaultExportFilename(),
		CanCreateDirectories: true,
		Filters: []wailsruntime.FileFilter{{
			DisplayName: "Excel workbook (*.xlsx)",
			Pattern:     "*.xlsx",
		}},
	})
	if err != nil {
		return ActionResponse{Message: "Export failed", Error: err.Error(), Snapshot: a.mustSnapshotFallback()}
	}
	if strings.TrimSpace(path) == "" {
		return ActionResponse{Message: "Export cancelled", Snapshot: a.mustSnapshotFallback()}
	}
	if strings.ToLower(filepath.Ext(path)) != ".xlsx" {
		path += ".xlsx"
	}
	if err := writeProfilesExcel(path, cards); err != nil {
		return ActionResponse{Message: "Export failed", Error: err.Error(), Snapshot: a.mustSnapshotFallback()}
	}
	return ActionResponse{Message: fmt.Sprintf("Exported %d account(s) to Excel", len(cards)), Snapshot: a.mustSnapshotFallback()}
}

func writeProfilesExcel(path string, cards []ProfileCard) error {
	f := excelize.NewFile()
	defer func() { _ = f.Close() }()

	const sheet = "Accounts"
	if err := f.SetSheetName("Sheet1", sheet); err != nil {
		return err
	}
	headers, rows := profileExportTable(cards)
	headerStyle, err := f.NewStyle(&excelize.Style{
		Font:      &excelize.Font{Bold: true, Family: "Arial", Color: "FFFFFF"},
		Fill:      excelize.Fill{Type: "pattern", Color: []string{"1F2937"}, Pattern: 1},
		Alignment: &excelize.Alignment{Horizontal: "center", Vertical: "center"},
	})
	if err != nil {
		return err
	}

	headerRow := make([]interface{}, len(headers))
	for i, header := range headers {
		headerRow[i] = header
	}
	if err := f.SetSheetRow(sheet, "A1", &headerRow); err != nil {
		return err
	}
	lastHeaderCell, err := excelize.CoordinatesToCellName(len(headers), 1)
	if err != nil {
		return err
	}
	if err := f.SetCellStyle(sheet, "A1", lastHeaderCell, headerStyle); err != nil {
		return err
	}

	for i, row := range rows {
		cell, err := excelize.CoordinatesToCellName(1, i+2)
		if err != nil {
			return err
		}
		if err := f.SetSheetRow(sheet, cell, &row); err != nil {
			return err
		}
	}

	widths := map[string]float64{
		"A": 6, "B": 12, "C": 28, "D": 34, "E": 14, "F": 10, "G": 14,
		"H": 22, "I": 24, "J": 14, "K": 14, "L": 14, "M": 16, "N": 32,
	}
	for col, width := range widths {
		if err := f.SetColWidth(sheet, col, col, width); err != nil {
			return err
		}
	}
	if err := f.SetPanes(sheet, &excelize.Panes{Freeze: true, Split: false, XSplit: 0, YSplit: 1, TopLeftCell: "A2", ActivePane: "bottomLeft"}); err != nil {
		return err
	}
	return f.SaveAs(path)
}

func profileExportTable(cards []ProfileCard) ([]string, [][]interface{}) {
	headers := []string{"STT", "Provider", "Account", "Email", "Trạng thái", "Blocked", "Phân loại", "Tên shop", "Tên khách", "Giá (VNĐ)", "Ngày add", "Ngày hết hạn", "Đã dùng", "Ghi chú"}
	rows := make([][]interface{}, 0, len(cards))
	for i, card := range cards {
		rows = append(rows, []interface{}{
			i + 1,
			card.Provider,
			card.Label,
			card.Email,
			strings.ReplaceAll(card.AuthStatus, "_", " "),
			exportBool(card.Blocked),
			exportAudience(card.Audience),
			card.ShopName,
			card.CustomerName,
			card.Price,
			card.CreatedAtText,
			card.EndAtText,
			card.DaysUsedText,
			card.Note,
		})
	}
	return headers, rows
}

func toExportText(value interface{}) string {
	return fmt.Sprint(value)
}

func exportBool(value bool) string {
	if value {
		return "Có"
	}
	return ""
}

func exportAudience(value string) string {
	if strings.EqualFold(strings.TrimSpace(value), "customer") {
		return "Khách hàng"
	}
	return "Cá nhân"
}

func defaultExportFilename() string {
	return "codex-lover-accounts-" + time.Now().Format("20060102-150405") + ".xlsx"
}

func defaultExportDirectory() string {
	home, err := os.UserHomeDir()
	if err != nil || strings.TrimSpace(home) == "" {
		return ""
	}
	downloads := filepath.Join(home, "Downloads")
	if info, err := os.Stat(downloads); err == nil && info.IsDir() {
		return downloads
	}
	return home
}
