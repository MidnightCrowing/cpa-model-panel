package config

import (
	"fmt"
	"os"
	"strconv"
	"strings"
)

type Config struct {
	Listen              string
	AdminToken          string
	CPABaseURL          string
	CPAManagementSecret string
	DataDir             string
	SnapshotRetain      int
	// Keeper supplies per-request success/failure statistics. Optional: the
	// panel works without it, just without the health column.
	KeeperURL      string
	KeeperPassword string
}

func Load() (Config, error) {
	cfg := Config{
		Listen:              envOr("LISTEN", ":5006"),
		AdminToken:          strings.TrimSpace(os.Getenv("ADMIN_TOKEN")),
		CPABaseURL:          strings.TrimRight(envOr("CPA_BASE_URL", "http://127.0.0.1:5000"), "/"),
		CPAManagementSecret: strings.TrimSpace(os.Getenv("CPA_MANAGEMENT_SECRET")),
		DataDir:             envOr("DATA_DIR", "./data"),
		SnapshotRetain:      envInt("SNAPSHOT_RETAIN", 20),
		KeeperURL:           strings.TrimRight(envOr("KEEPER_URL", ""), "/"),
		KeeperPassword:      strings.TrimSpace(os.Getenv("KEEPER_PASSWORD")),
	}
	if cfg.AdminToken == "" {
		return Config{}, fmt.Errorf("ADMIN_TOKEN is required")
	}
	if cfg.CPAManagementSecret == "" {
		return Config{}, fmt.Errorf("CPA_MANAGEMENT_SECRET is required")
	}
	if cfg.SnapshotRetain < 1 {
		cfg.SnapshotRetain = 20
	}
	return cfg, nil
}

func envOr(key, fallback string) string {
	if v := strings.TrimSpace(os.Getenv(key)); v != "" {
		return v
	}
	return fallback
}

func envInt(key string, fallback int) int {
	v := strings.TrimSpace(os.Getenv(key))
	if v == "" {
		return fallback
	}
	n, err := strconv.Atoi(v)
	if err != nil {
		return fallback
	}
	return n
}
