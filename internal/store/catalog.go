package store

import (
	"database/sql"
	"encoding/json"
	"time"

	"github.com/local/cpa-model-panel/internal/catalog"
)

// LoadCatalog returns the cached model catalog, or nil when the panel has
// never built one.
func (s *Store) LoadCatalog() (*catalog.Catalog, error) {
	var raw string
	err := s.db.QueryRow(`SELECT payload_json FROM model_catalog WHERE id=1`).Scan(&raw)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var out catalog.Catalog
	if err := json.Unmarshal([]byte(raw), &out); err != nil {
		return nil, err
	}
	return &out, nil
}

func (s *Store) SaveCatalog(cat *catalog.Catalog) error {
	encoded, err := json.Marshal(cat)
	if err != nil {
		return err
	}
	_, err = s.db.Exec(`INSERT INTO model_catalog(id, payload_json, updated_at) VALUES(1, ?, ?)
		ON CONFLICT(id) DO UPDATE SET payload_json=excluded.payload_json, updated_at=excluded.updated_at`,
		string(encoded), time.Now().UTC().Format(time.RFC3339))
	return err
}
