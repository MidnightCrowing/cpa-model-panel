package store

import (
	"database/sql"
	"encoding/json"

	"github.com/local/cpa-model-panel/internal/catalog"
	"github.com/local/cpa-model-panel/internal/clean"
)

const (
	keyCleanPrefixes = "clean_prefixes"
	keyCleanSuffixes = "clean_suffixes"
	keyCleanProtect  = "clean_protect"
	keyCleanRewrites = "clean_rewrites"
	keyWhitelist     = "model_whitelist_regex"
	keyVersionFilter = "version_filter_config"
	keyProtocolRegex = "protocol_regex"
	keyLegacyDone    = "legacy_disabled_migrated"
)

func (s *Store) getJSON(key string, dest any) (bool, error) {
	var raw string
	err := s.db.QueryRow(`SELECT value_json FROM settings WHERE key=?`, key).Scan(&raw)
	if err == sql.ErrNoRows {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	if err := json.Unmarshal([]byte(raw), dest); err != nil {
		return false, err
	}
	return true, nil
}

func (s *Store) setJSON(key string, value any) error {
	encoded, err := json.Marshal(value)
	if err != nil {
		return err
	}
	_, err = s.db.Exec(`INSERT INTO settings(key, value_json) VALUES(?, ?)
		ON CONFLICT(key) DO UPDATE SET value_json=excluded.value_json`, key, string(encoded))
	return err
}

// Settings returns the full pipeline configuration, filling in defaults for
// anything never configured.
func (s *Store) Settings() (catalog.Settings, error) {
	out := catalog.Settings{
		Prefixes: append([]string(nil), clean.DefaultPrefixes...),
		Suffixes: append([]string(nil), clean.DefaultSuffixes...),
		Version:  clean.DefaultVersionFilterConfig(),
		Protocol: clean.DefaultProtocolConfig(),
	}

	var prefixes []string
	if found, err := s.getJSON(keyCleanPrefixes, &prefixes); err != nil {
		return out, err
	} else if found && len(prefixes) > 0 {
		out.Prefixes = prefixes
	}

	var suffixes []string
	if found, err := s.getJSON(keyCleanSuffixes, &suffixes); err != nil {
		return out, err
	} else if found && len(suffixes) > 0 {
		out.Suffixes = suffixes
	}

	var protect string
	if found, err := s.getJSON(keyCleanProtect, &protect); err != nil {
		return out, err
	} else if found {
		out.Protect = protect
	}

	var rewrites []clean.Rewrite
	if found, err := s.getJSON(keyCleanRewrites, &rewrites); err != nil {
		return out, err
	} else if found {
		out.Rewrites = rewrites
	}

	var whitelist string
	if found, err := s.getJSON(keyWhitelist, &whitelist); err != nil {
		return out, err
	} else if found {
		out.Whitelist = whitelist
	}

	var version clean.VersionFilterConfig
	if found, err := s.getJSON(keyVersionFilter, &version); err != nil {
		return out, err
	} else if found {
		out.Version = version
	}

	var protocol clean.ProtocolConfig
	if found, err := s.getJSON(keyProtocolRegex, &protocol); err != nil {
		return out, err
	} else if found {
		// Used exactly as stored: an empty pattern tags nothing.
		out.Protocol = protocol
	}

	return out, nil
}

func (s *Store) SetCleanRules(cfg clean.RulesConfig) error {
	if err := s.setJSON(keyCleanPrefixes, cfg.Prefixes); err != nil {
		return err
	}
	if err := s.setJSON(keyCleanSuffixes, cfg.Suffixes); err != nil {
		return err
	}
	if err := s.setJSON(keyCleanProtect, cfg.Protect); err != nil {
		return err
	}
	return s.setJSON(keyCleanRewrites, cfg.Rewrites)
}

func (s *Store) SetWhitelist(pattern string) error {
	return s.setJSON(keyWhitelist, pattern)
}

func (s *Store) SetVersionFilter(cfg clean.VersionFilterConfig) error {
	return s.setJSON(keyVersionFilter, cfg)
}

func (s *Store) SetProtocolRegex(cfg clean.ProtocolConfig) error {
	return s.setJSON(keyProtocolRegex, cfg)
}
