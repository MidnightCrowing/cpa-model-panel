package store

import (
	"database/sql"
	"os"
	"path/filepath"

	_ "modernc.org/sqlite"
)

type Store struct {
	db *sql.DB
}

func Open(dataDir string) (*Store, error) {
	if dataDir == "" {
		dataDir = "./data"
	}
	if err := os.MkdirAll(dataDir, 0o755); err != nil {
		return nil, err
	}
	db, err := sql.Open("sqlite", filepath.Join(dataDir, "panel.db"))
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(1)
	s := &Store{db: db}
	if err := s.migrate(); err != nil {
		_ = db.Close()
		return nil, err
	}
	return s, nil
}

func (s *Store) Close() error { return s.db.Close() }

func (s *Store) Ping() error { return s.db.Ping() }

func (s *Store) migrate() error {
	_, err := s.db.Exec(`
CREATE TABLE IF NOT EXISTS settings (
  key TEXT PRIMARY KEY,
  value_json TEXT NOT NULL
);

CREATE TABLE IF NOT EXISTS config_snapshots (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  created_at TEXT NOT NULL,
  fingerprint TEXT NOT NULL,
  payload_json TEXT NOT NULL,
  note TEXT NOT NULL DEFAULT ''
);

-- Full model catalog cache. Keeping every model the panel has ever seen is
-- what makes whitelist / version filtering reversible.
CREATE TABLE IF NOT EXISTS model_catalog (
  id INTEGER PRIMARY KEY CHECK (id = 1),
  payload_json TEXT NOT NULL,
  updated_at TEXT NOT NULL
);

-- Manually excluded models: the naming page's "删除". These never reach CPA
-- and are hidden from the matrix page, but stay listed with a restore button.
CREATE TABLE IF NOT EXISTS model_exclusions (
  site TEXT NOT NULL,
  upstream TEXT NOT NULL,
  created_at TEXT NOT NULL,
  PRIMARY KEY (site, upstream)
);

-- Per-site disable: the matrix page's toggle. A different concept from an
-- exclusion — the model still exists, it is just turned off at this site.
CREATE TABLE IF NOT EXISTS site_disabled (
  site TEXT NOT NULL,
  upstream TEXT NOT NULL,
  created_at TEXT NOT NULL,
  PRIMARY KEY (site, upstream)
);

-- Models explicitly kept despite the whitelist / version rules.
CREATE TABLE IF NOT EXISTS model_keeps (
  site TEXT NOT NULL,
  upstream TEXT NOT NULL,
  created_at TEXT NOT NULL,
  PRIMARY KEY (site, upstream)
);

-- Short-lived endpoints ("鸡蛋") and where they were shared.
CREATE TABLE IF NOT EXISTS temp_sites (
  site TEXT PRIMARY KEY,
  source_url TEXT NOT NULL DEFAULT '',
  created_at TEXT NOT NULL
);

-- Outcome of the last model-list probe per site.
CREATE TABLE IF NOT EXISTS site_health (
  site TEXT PRIMARY KEY,
  last_ok_at TEXT NOT NULL DEFAULT '',
  last_error TEXT NOT NULL DEFAULT '',
  last_try_at TEXT NOT NULL DEFAULT '',
  failures INTEGER NOT NULL DEFAULT 0
);

-- Persistent history for the optional upstream-model synchronization job.
-- The payload is JSON so the summary can grow without a schema migration.
CREATE TABLE IF NOT EXISTS auto_sync_logs (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  started_at TEXT NOT NULL,
  status TEXT NOT NULL,
  payload_json TEXT NOT NULL
);

-- Legacy table from the pre-catalog schema; kept only so MigrateLegacy can
-- read it once.
CREATE TABLE IF NOT EXISTS disabled_models (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  provider_name TEXT NOT NULL,
  upstream_name TEXT NOT NULL,
  canonical_alias TEXT NOT NULL DEFAULT '',
  model_json TEXT NOT NULL,
  disabled_at TEXT NOT NULL,
  source TEXT NOT NULL DEFAULT 'panel',
  UNIQUE(provider_name, upstream_name)
);
`)
	return err
}
