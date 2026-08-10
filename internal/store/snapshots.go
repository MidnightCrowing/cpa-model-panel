package store

import (
	"encoding/json"
	"time"
)

type SnapshotMeta struct {
	ID          int64  `json:"id"`
	CreatedAt   string `json:"created_at"`
	Fingerprint string `json:"fingerprint"`
	Note        string `json:"note"`
}

func (s *Store) AddSnapshot(fingerprint, note string, payload any) (int64, error) {
	encoded, err := json.Marshal(payload)
	if err != nil {
		return 0, err
	}
	res, err := s.db.Exec(`INSERT INTO config_snapshots(created_at, fingerprint, payload_json, note) VALUES(?,?,?,?)`,
		time.Now().UTC().Format(time.RFC3339), fingerprint, string(encoded), note)
	if err != nil {
		return 0, err
	}
	return res.LastInsertId()
}

func (s *Store) ListSnapshots(limit int) ([]SnapshotMeta, error) {
	if limit <= 0 {
		limit = 20
	}
	rows, err := s.db.Query(`SELECT id, created_at, fingerprint, note FROM config_snapshots ORDER BY id DESC LIMIT ?`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]SnapshotMeta, 0, limit)
	for rows.Next() {
		var m SnapshotMeta
		if err := rows.Scan(&m.ID, &m.CreatedAt, &m.Fingerprint, &m.Note); err != nil {
			return nil, err
		}
		out = append(out, m)
	}
	return out, rows.Err()
}

func (s *Store) SnapshotPayload(id int64, dest any) error {
	var raw string
	if err := s.db.QueryRow(`SELECT payload_json FROM config_snapshots WHERE id=?`, id).Scan(&raw); err != nil {
		return err
	}
	return json.Unmarshal([]byte(raw), dest)
}

func (s *Store) PruneSnapshots(keep int) error {
	if keep < 1 {
		keep = 20
	}
	_, err := s.db.Exec(`DELETE FROM config_snapshots WHERE id NOT IN (
		SELECT id FROM config_snapshots ORDER BY id DESC LIMIT ?
	)`, keep)
	return err
}
