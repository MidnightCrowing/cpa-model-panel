package clean

import (
	"regexp"
	"strconv"
	"strings"
)

type SeriesThreshold struct {
	Series     string  `json:"series"`
	MinVersion float64 `json:"min_version"`
}

type VersionFilterConfig struct {
	Enabled       bool              `json:"enabled"`
	Thresholds    []SeriesThreshold `json:"thresholds"`
	ExemptPattern string            `json:"exempt_pattern"`
}

func DefaultVersionFilterConfig() VersionFilterConfig {
	return VersionFilterConfig{
		Enabled: false,
		Thresholds: []SeriesThreshold{
			{Series: "grok", MinVersion: 4.0},
			{Series: "gpt", MinVersion: 5.0},
			{Series: "claude", MinVersion: 4.0},
			{Series: "gemini", MinVersion: 2.5},
			{Series: "glm", MinVersion: 4.5},
		},
		ExemptPattern: `(ocr|chat|embed|rerank|speech|coder|vision|image|audio|tts|whisper|moderation|search|r1|m3)`,
	}
}

// versionAfterSeries matches a version number that follows the series name,
// allowing at most one intervening word ("doubao-seed-1.6") and an optional
// single letter glued to the number ("kimi-k2", "gpt-4o", "deepseek-v3.2").
//
// Anchoring matters: a free search reads "grok-code-fast-1" as version 1. The
// shape above is what real model names actually use.
var versionAfterSeries = regexp.MustCompile(`^[-_. ]?(?:[a-z]+[-_. ])?[a-z]?(\d+(?:[.-]\d+)?)`)

// ExtractVersion returns the version number attached to series inside name.
func ExtractVersion(name, series string) (float64, bool) {
	lowerName := strings.ToLower(name)
	lowerSeries := strings.ToLower(strings.TrimSpace(series))
	if lowerSeries == "" {
		return 0, false
	}

	from := 0
	for {
		idx := strings.Index(lowerName[from:], lowerSeries)
		if idx < 0 {
			return 0, false
		}
		idx += from
		rest := lowerName[idx+len(lowerSeries):]
		if match := versionAfterSeries.FindStringSubmatch(rest); len(match) >= 2 {
			value, err := strconv.ParseFloat(strings.ReplaceAll(match[1], "-", "."), 64)
			if err == nil {
				return value, true
			}
		}
		from = idx + len(lowerSeries)
		if from >= len(lowerName) {
			return 0, false
		}
	}
}

// VersionVerdict explains why a model was kept or dropped.
type VersionVerdict struct {
	Keep       bool
	Series     string
	Version    float64
	MinVersion float64
	Exempt     bool
}

// Evaluate decides whether a model survives the version thresholds. It checks
// the upstream name, the alias and the cleaned name so a site that prefixes
// everything still gets classified.
func Evaluate(cfg VersionFilterConfig, names ...string) VersionVerdict {
	if !cfg.Enabled {
		return VersionVerdict{Keep: true}
	}

	candidates := make([]string, 0, len(names))
	for _, name := range names {
		if strings.TrimSpace(name) != "" {
			candidates = append(candidates, name)
		}
	}

	if pattern := strings.TrimSpace(cfg.ExemptPattern); pattern != "" {
		if re, err := regexp.Compile("(?i)" + pattern); err == nil {
			for _, name := range candidates {
				if re.MatchString(name) {
					return VersionVerdict{Keep: true, Exempt: true}
				}
			}
		}
	}

	for _, threshold := range cfg.Thresholds {
		series := strings.TrimSpace(threshold.Series)
		if series == "" {
			continue
		}
		for _, name := range candidates {
			version, ok := ExtractVersion(name, series)
			if !ok {
				continue
			}
			return VersionVerdict{
				Keep:       version >= threshold.MinVersion,
				Series:     series,
				Version:    version,
				MinVersion: threshold.MinVersion,
			}
		}
	}

	return VersionVerdict{Keep: true}
}
