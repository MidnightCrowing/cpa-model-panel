package store

import (
	"encoding/json"
	"fmt"
)

const keyAutoSync = "auto_sync_config"

// AutoSyncConfig controls the background upstream-model synchronization job.
// Each wait is sampled uniformly from IntervalMinutes ± JitterMinutes.
type AutoSyncConfig struct {
	Enabled         bool `json:"enabled"`
	IntervalMinutes int  `json:"interval_minutes"`
	JitterMinutes   int  `json:"jitter_minutes"`
}

func DefaultAutoSyncConfig() AutoSyncConfig {
	return AutoSyncConfig{IntervalMinutes: 360, JitterMinutes: 30}
}

func (c AutoSyncConfig) Validate() error {
	if c.IntervalMinutes < 1 || c.IntervalMinutes > 525600 {
		return fmt.Errorf("定时间隔必须在 1 到 525600 分钟之间")
	}
	if c.JitterMinutes < 0 {
		return fmt.Errorf("随机误差不能小于 0 分钟")
	}
	if c.JitterMinutes >= c.IntervalMinutes {
		return fmt.Errorf("随机误差必须小于定时间隔")
	}
	return nil
}

func (s *Store) AutoSyncConfig() (AutoSyncConfig, error) {
	out := DefaultAutoSyncConfig()
	found, err := s.getJSON(keyAutoSync, &out)
	if err != nil {
		return out, err
	}
	if !found {
		return out, nil
	}
	return out, out.Validate()
}

func (s *Store) SetAutoSyncConfig(cfg AutoSyncConfig) error {
	if err := cfg.Validate(); err != nil {
		return err
	}
	return s.setJSON(keyAutoSync, cfg)
}

// AutoSyncLog is one complete scheduler run. Partial means model discovery
// failed for some sites but successful sites were still mapped and saved.
type AutoSyncLog struct {
	ID         int64    `json:"id"`
	StartedAt  string   `json:"started_at"`
	FinishedAt string   `json:"finished_at"`
	Status     string   `json:"status"`
	Refreshed  int      `json:"refreshed"`
	Added      int      `json:"added"`
	Dropped    int      `json:"dropped"`
	Failed     int      `json:"failed"`
	Failures   []string `json:"failures,omitempty"`
	Suggested  int      `json:"suggested"`
	Renamed    int      `json:"renamed"`
	Removed    int      `json:"removed"`
	Restored   int      `json:"restored"`
	Moved      int      `json:"moved"`
	Written    []string `json:"written,omitempty"`
	Snapshot   int64    `json:"snapshot,omitempty"`
	Error      string   `json:"error,omitempty"`
}

func (s *Store) AddAutoSyncLog(entry AutoSyncLog) (int64, error) {
	encoded, err := json.Marshal(entry)
	if err != nil {
		return 0, err
	}
	res, err := s.db.Exec(`INSERT INTO auto_sync_logs(started_at, status, payload_json) VALUES(?,?,?)`,
		entry.StartedAt, entry.Status, string(encoded))
	if err != nil {
		return 0, err
	}
	return res.LastInsertId()
}

func (s *Store) ListAutoSyncLogs(limit int) ([]AutoSyncLog, error) {
	if limit <= 0 {
		limit = 20
	}
	if limit > 200 {
		limit = 200
	}
	rows, err := s.db.Query(`SELECT id, payload_json FROM auto_sync_logs ORDER BY id DESC LIMIT ?`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]AutoSyncLog, 0, limit)
	for rows.Next() {
		var entry AutoSyncLog
		var id int64
		var raw string
		if err := rows.Scan(&id, &raw); err != nil {
			return nil, err
		}
		if err := json.Unmarshal([]byte(raw), &entry); err != nil {
			return nil, err
		}
		entry.ID = id
		out = append(out, entry)
	}
	return out, rows.Err()
}

func (s *Store) PruneAutoSyncLogs(keep int) error {
	if keep < 1 {
		keep = 100
	}
	_, err := s.db.Exec(`DELETE FROM auto_sync_logs WHERE id NOT IN (
		SELECT id FROM auto_sync_logs ORDER BY id DESC LIMIT ?
	)`, keep)
	return err
}
