package service

import (
	"strings"

	"codex-lover/internal/model"
)

func (s *Service) AddShop(name string) ([]string, error) {
	cfg, err := s.store.LoadConfig()
	if err != nil {
		return nil, err
	}
	cfg = addShopToConfig(cfg, name)
	if err := s.store.SaveConfig(cfg); err != nil {
		return nil, err
	}
	return cfg.Shops, nil
}

func (s *Service) RemoveShop(name string) ([]string, error) {
	cfg, err := s.store.LoadConfig()
	if err != nil {
		return nil, err
	}
	cfg = removeShopFromConfig(cfg, name)
	if err := s.store.SaveConfig(cfg); err != nil {
		return nil, err
	}
	return cfg.Shops, nil
}

func addShopToConfig(cfg model.Config, name string) model.Config {
	trimmed := strings.TrimSpace(name)
	if trimmed == "" {
		cfg.Shops = normalizeShopCatalog(cfg.Shops)
		return cfg
	}
	cfg.Shops = normalizeShopCatalog(append(cfg.Shops, trimmed))
	return cfg
}

func removeShopFromConfig(cfg model.Config, name string) model.Config {
	target := strings.ToLower(strings.TrimSpace(name))
	shops := make([]string, 0, len(cfg.Shops))
	for _, shop := range normalizeShopCatalog(cfg.Shops) {
		if strings.ToLower(shop) == target {
			continue
		}
		shops = append(shops, shop)
	}
	cfg.Shops = shops
	return cfg
}

func normalizeShopCatalog(shops []string) []string {
	out := make([]string, 0, len(shops))
	seen := map[string]bool{}
	for _, shop := range shops {
		trimmed := strings.TrimSpace(shop)
		if trimmed == "" {
			continue
		}
		key := strings.ToLower(trimmed)
		if seen[key] {
			continue
		}
		seen[key] = true
		out = append(out, trimmed)
	}
	return out
}
