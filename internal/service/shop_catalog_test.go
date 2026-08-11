package service

import (
	"reflect"
	"testing"

	"codex-lover/internal/model"
)

func TestNormalizeShopCatalog(t *testing.T) {
	got := normalizeShopCatalog([]string{" Shop A ", "", "shop a", "Shop B", "  Shop B  "})
	want := []string{"Shop A", "Shop B"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("normalizeShopCatalog = %#v, want %#v", got, want)
	}
}

func TestAddShopToConfig(t *testing.T) {
	cfg := model.Config{Shops: []string{"Shop A"}}
	got := addShopToConfig(cfg, " Shop B ")
	want := []string{"Shop A", "Shop B"}
	if !reflect.DeepEqual(got.Shops, want) {
		t.Fatalf("Shops = %#v, want %#v", got.Shops, want)
	}

	got = addShopToConfig(got, "shop b")
	if !reflect.DeepEqual(got.Shops, want) {
		t.Fatalf("duplicate shop should be ignored, got %#v", got.Shops)
	}
}

func TestRemoveShopFromConfig(t *testing.T) {
	cfg := model.Config{Shops: []string{"Shop A", "Shop B"}}
	got := removeShopFromConfig(cfg, " shop a ")
	want := []string{"Shop B"}
	if !reflect.DeepEqual(got.Shops, want) {
		t.Fatalf("Shops = %#v, want %#v", got.Shops, want)
	}
}
