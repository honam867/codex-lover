package main

import (
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/xuri/excelize/v2"
)

func TestProfileExportRowsExcludeQuotaAndHealth(t *testing.T) {
	cards := []ProfileCard{{
		Label:            "codex-a",
		Email:            "a@example.com",
		Provider:         "codex",
		Audience:         "customer",
		AuthStatus:       "logged_out",
		Blocked:          true,
		Price:            80000,
		ShopName:         "Shop A",
		CustomerName:     "Customer A",
		CreatedAtText:    "04/08/2026",
		EndAtText:        "04/09/2026",
		DaysUsedText:     "đã dùng 3 ngày",
		Note:             "note",
		PrimarySummary:   "97% left resets 2026-08-09 08:00",
		HealthStatus:     "ok",
		HealthMessage:    "Probe OK",
		PrimaryPercent:   97,
		SecondaryPercent: 50,
		SecondarySummary: "50% left",
	}}

	headers, rows := profileExportTable(cards)

	for _, forbidden := range []string{"Quota", "Health", "Primary", "Secondary"} {
		if slices.Contains(headers, forbidden) {
			t.Fatalf("headers include forbidden %q: %v", forbidden, headers)
		}
	}
	if len(rows) != 1 {
		t.Fatalf("rows len = %d, want 1", len(rows))
	}
	joined := ""
	for _, value := range rows[0] {
		joined += "|" + toExportText(value)
	}
	for _, forbidden := range []string{"97% left", "Probe OK", "ok", "50% left"} {
		if strings.Contains(joined, forbidden) {
			t.Fatalf("export row contains forbidden value %q: %s", forbidden, joined)
		}
	}
	for _, want := range []string{"codex-a", "a@example.com", "Shop A", "Customer A", "80000"} {
		if !strings.Contains(joined, want) {
			t.Fatalf("export row missing %q: %s", want, joined)
		}
	}
}

func TestWriteProfilesExcelCreatesWorkbook(t *testing.T) {
	path := filepath.Join(t.TempDir(), "accounts.xlsx")
	cards := []ProfileCard{{Label: "codex-a", Email: "a@example.com", Provider: "codex", Price: 80000}}

	if err := writeProfilesExcel(path, cards); err != nil {
		t.Fatalf("writeProfilesExcel returned error: %v", err)
	}

	f, err := excelize.OpenFile(path)
	if err != nil {
		t.Fatalf("OpenFile returned error: %v", err)
	}
	defer func() { _ = f.Close() }()

	got, err := f.GetCellValue("Accounts", "B2")
	if err != nil {
		t.Fatalf("GetCellValue returned error: %v", err)
	}
	if got != "codex" {
		t.Fatalf("B2 = %q, want codex", got)
	}
}
