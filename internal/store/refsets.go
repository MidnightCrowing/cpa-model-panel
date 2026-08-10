package store

import (
	"time"

	"github.com/local/cpa-model-panel/internal/catalog"
)

// Exclusions returns the manually deleted models (naming page "删除").
func (s *Store) Exclusions() (catalog.RefSet, error) {
	return s.refSet(`SELECT site, upstream FROM model_exclusions`)
}

func (s *Store) SetExclusions(refs catalog.RefSet) error {
	return s.replaceRefSet("model_exclusions", refs)
}

// Disabled returns per-site disabled models (matrix page toggle).
func (s *Store) Disabled() (catalog.RefSet, error) {
	return s.refSet(`SELECT site, upstream FROM site_disabled`)
}

func (s *Store) SetDisabled(refs catalog.RefSet) error {
	return s.replaceRefSet("site_disabled", refs)
}

// Keeps returns models explicitly kept despite the whitelist / version rules.
func (s *Store) Keeps() (catalog.RefSet, error) {
	return s.refSet(`SELECT site, upstream FROM model_keeps`)
}

func (s *Store) SetKeeps(refs catalog.RefSet) error {
	return s.replaceRefSet("model_keeps", refs)
}

func (s *Store) refSet(query string) (catalog.RefSet, error) {
	rows, err := s.db.Query(query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := catalog.RefSet{}
	for rows.Next() {
		var ref catalog.EntryRef
		if err := rows.Scan(&ref.Site, &ref.Upstream); err != nil {
			return nil, err
		}
		out[ref] = true
	}
	return out, rows.Err()
}

// replaceRefSet rewrites a set table in one transaction. The sets are small
// (hundreds of rows at most) and always written as a whole, so a diff would
// only add failure modes.
func (s *Store) replaceRefSet(table string, refs catalog.RefSet) error {
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()

	if _, err := tx.Exec(`DELETE FROM ` + table); err != nil {
		return err
	}
	stmt, err := tx.Prepare(`INSERT INTO ` + table + `(site, upstream, created_at) VALUES(?,?,?)`)
	if err != nil {
		return err
	}
	defer stmt.Close()

	now := time.Now().UTC().Format(time.RFC3339)
	for ref, on := range refs {
		if !on {
			continue
		}
		if _, err := stmt.Exec(ref.Site, ref.Upstream, now); err != nil {
			return err
		}
	}
	return tx.Commit()
}
