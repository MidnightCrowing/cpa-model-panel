package store

import (
	"github.com/local/cpa-model-panel/internal/catalog"
)

// MigrateLegacyDisabled converts the pre-catalog `disabled_models` table into
// the new two-set model, and runs exactly once.
//
// The old schema keyed rows by CPA provider *name* and used source='deleted'
// to mean "removed from the naming page" and anything else to mean "turned off
// for this site" — the two concepts the panel now keeps properly apart.
func (s *Store) MigrateLegacyDisabled(sites []catalog.Site) error {
	var done bool
	if found, err := s.getJSON(keyLegacyDone, &done); err != nil {
		return err
	} else if found && done {
		return nil
	}

	legacy, err := s.readLegacyRows()
	if err != nil {
		return err
	}

	byName := make(map[string]string, len(sites)*2)
	for _, site := range sites {
		byName[site.ID] = site.ID
	}
	for _, site := range sites {
		if _, taken := byName[site.Name]; !taken {
			byName[site.Name] = site.ID
		}
	}

	exclusions, err := s.Exclusions()
	if err != nil {
		return err
	}
	disabled, err := s.Disabled()
	if err != nil {
		return err
	}

	migrated := 0
	for _, row := range legacy {
		siteID, ok := byName[row.provider]
		if !ok {
			continue
		}
		ref := catalog.EntryRef{Site: siteID, Upstream: row.upstream}
		if row.source == "deleted" {
			exclusions[ref] = true
		} else {
			disabled[ref] = true
		}
		migrated++
	}

	if migrated > 0 {
		if err := s.SetExclusions(exclusions); err != nil {
			return err
		}
		if err := s.SetDisabled(disabled); err != nil {
			return err
		}
	}
	return s.setJSON(keyLegacyDone, true)
}

type legacyRow struct {
	provider string
	upstream string
	source   string
}

// readLegacyRows drains the cursor before anything else touches the database:
// the pool is capped at one connection, so a query issued while these rows are
// still open would deadlock.
func (s *Store) readLegacyRows() ([]legacyRow, error) {
	rows, err := s.db.Query(`SELECT provider_name, upstream_name, source FROM disabled_models`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []legacyRow
	for rows.Next() {
		var row legacyRow
		if err := rows.Scan(&row.provider, &row.upstream, &row.source); err != nil {
			return nil, err
		}
		out = append(out, row)
	}
	return out, rows.Err()
}
