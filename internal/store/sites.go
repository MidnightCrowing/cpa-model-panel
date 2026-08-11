package store

import (
	"database/sql"
	"time"
)

// TempSite is a "鸡蛋": a short-lived endpoint shared somewhere, added to try
// out for a while and thrown away when it stops working. The panel remembers
// where it came from so it can be revisited, and marks it in the UI so it is
// never mistaken for a stable site.
type TempSite struct {
	Site      string `json:"site"`
	SourceURL string `json:"source_url"`
	CreatedAt string `json:"created_at"`
}

func (s *Store) TempSites() (map[string]TempSite, error) {
	rows, err := s.db.Query(`SELECT site, source_url, created_at FROM temp_sites`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[string]TempSite{}
	for rows.Next() {
		var t TempSite
		if err := rows.Scan(&t.Site, &t.SourceURL, &t.CreatedAt); err != nil {
			return nil, err
		}
		out[t.Site] = t
	}
	return out, rows.Err()
}

func (s *Store) AddTempSite(site, sourceURL string) error {
	_, err := s.db.Exec(`INSERT INTO temp_sites(site, source_url, created_at) VALUES(?,?,?)
		ON CONFLICT(site) DO UPDATE SET source_url=excluded.source_url`,
		site, sourceURL, time.Now().UTC().Format(time.RFC3339))
	return err
}

func (s *Store) DeleteTempSite(site string) error {
	_, err := s.db.Exec(`DELETE FROM temp_sites WHERE site=?`, site)
	return err
}

// SiteHealth is what the last model-list probe of a site reported. It is how a
// dead key or a vanished host becomes visible instead of just producing an
// error line in a toast that scrolls away.
type SiteHealth struct {
	Site      string `json:"site"`
	LastOKAt  string `json:"last_ok_at,omitempty"`
	LastError string `json:"last_error,omitempty"`
	LastTryAt string `json:"last_try_at,omitempty"`
	Failures  int    `json:"failures"`
}

func (s *Store) SiteHealth() (map[string]SiteHealth, error) {
	rows, err := s.db.Query(`SELECT site, last_ok_at, last_error, last_try_at, failures FROM site_health`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[string]SiteHealth{}
	for rows.Next() {
		var h SiteHealth
		var okAt, lastErr, tryAt sql.NullString
		if err := rows.Scan(&h.Site, &okAt, &lastErr, &tryAt, &h.Failures); err != nil {
			return nil, err
		}
		h.LastOKAt, h.LastError, h.LastTryAt = okAt.String, lastErr.String, tryAt.String
		out[h.Site] = h
	}
	return out, rows.Err()
}

// RecordProbe stores the outcome of one model-list probe. Failures accumulate
// so a site that has been broken for a while can be told apart from one that
// blipped once.
func (s *Store) RecordProbe(site string, err error) error {
	now := time.Now().UTC().Format(time.RFC3339)
	if err == nil {
		_, dbErr := s.db.Exec(`INSERT INTO site_health(site, last_ok_at, last_error, last_try_at, failures)
			VALUES(?,?,'',?,0)
			ON CONFLICT(site) DO UPDATE SET last_ok_at=excluded.last_ok_at, last_error='', last_try_at=excluded.last_try_at, failures=0`,
			site, now, now)
		return dbErr
	}
	_, dbErr := s.db.Exec(`INSERT INTO site_health(site, last_ok_at, last_error, last_try_at, failures)
		VALUES(?,'',?,?,1)
		ON CONFLICT(site) DO UPDATE SET last_error=excluded.last_error, last_try_at=excluded.last_try_at, failures=site_health.failures+1`,
		site, err.Error(), now)
	return dbErr
}

func (s *Store) ForgetSite(site string) error {
	if _, err := s.db.Exec(`DELETE FROM site_health WHERE site=?`, site); err != nil {
		return err
	}
	return s.DeleteTempSite(site)
}
