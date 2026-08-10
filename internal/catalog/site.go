package catalog

import (
	"net/url"
	"sort"
	"strconv"
	"strings"

	"github.com/local/cpa-model-panel/internal/cpa"
)

// BuildSites derives the logical site list from a CPA snapshot.
//
// openai-compatibility entries carry a human name and are the source of truth
// for site identity. codex-api-key / claude-api-key entries have no name at
// all, so they are matched back to an openai entry by, in order:
//
//  1. identical base-url
//  2. a shared API key   (disambiguates one host hosting several groups)
//  3. an unambiguous host match
//
// Anything still unmatched becomes its own site keyed by host.
func BuildSites(snap *cpa.Snapshot) []Site {
	sites := make([]Site, 0, 48)
	byID := make(map[string]int)
	byURL := make(map[string][]int)
	byKey := make(map[string][]int)
	byHost := make(map[string][]int)

	add := func(id, name string, ch cpa.Channel, idx int, p cpa.Provider) int {
		if existing, ok := byID[id]; ok {
			sites[existing].Providers[ch] = idx
			if sites[existing].BaseURL == "" {
				sites[existing].BaseURL = p.BaseURL
			}
			if sites[existing].APIKey == "" {
				if keys := p.APIKeys(); len(keys) > 0 {
					sites[existing].APIKey = keys[0]
				}
			}
			if sites[existing].Priority == 0 {
				sites[existing].Priority = p.Priority
			}
			return existing
		}
		site := Site{
			ID:        id,
			Name:      name,
			Priority:  p.Priority,
			BaseURL:   p.BaseURL,
			Headers:   p.Headers,
			Providers: map[cpa.Channel]int{ch: idx},
		}
		if keys := p.APIKeys(); len(keys) > 0 {
			site.APIKey = keys[0]
		}
		sites = append(sites, site)
		byID[id] = len(sites) - 1
		return len(sites) - 1
	}

	// Pass 1: openai entries define the sites.
	for idx, p := range snap.Providers(cpa.ChannelOpenAI) {
		id := strings.TrimSpace(p.Name)
		if id == "" {
			id = "url:" + normalizeURL(p.BaseURL)
		}
		for suffix := 2; ; suffix++ {
			if _, taken := byID[id]; !taken {
				break
			}
			id = strings.TrimSpace(p.Name) + " #" + strconv.Itoa(suffix)
		}
		name := strings.TrimSpace(p.Name)
		if name == "" {
			name = hostOf(p.BaseURL)
		}
		position := add(id, name, cpa.ChannelOpenAI, idx, p)

		byURL[normalizeURL(p.BaseURL)] = append(byURL[normalizeURL(p.BaseURL)], position)
		byHost[hostOf(p.BaseURL)] = append(byHost[hostOf(p.BaseURL)], position)
		for _, key := range p.APIKeys() {
			byKey[key] = append(byKey[key], position)
		}
	}

	// Pass 2: attach codex/claude entries to a site.
	for _, ch := range []cpa.Channel{cpa.ChannelCodex, cpa.ChannelClaude} {
		for idx, p := range snap.Providers(ch) {
			if position, ok := resolveSite(p, byURL, byKey, byHost); ok {
				sites[position].Providers[ch] = idx
				if sites[position].Priority == 0 {
					sites[position].Priority = p.Priority
				}
				continue
			}
			host := hostOf(p.BaseURL)
			id := "host:" + host
			if host == "" {
				id = "url:" + normalizeURL(p.BaseURL)
			}
			name := strings.TrimSpace(p.Name)
			if name == "" {
				name = host
			}
			if name == "" {
				name = p.BaseURL
			}
			position := add(id, name, ch, idx, p)
			byHost[host] = append(byHost[host], position)
			for _, key := range p.APIKeys() {
				byKey[key] = append(byKey[key], position)
			}
		}
	}

	sortSites(sites)
	return sites
}

func resolveSite(p cpa.Provider, byURL, byKey, byHost map[string][]int) (int, bool) {
	if matches := byURL[normalizeURL(p.BaseURL)]; len(matches) == 1 {
		return matches[0], true
	}
	for _, key := range p.APIKeys() {
		if matches := byKey[key]; len(matches) == 1 {
			return matches[0], true
		}
	}
	if matches := byHost[hostOf(p.BaseURL)]; len(matches) == 1 {
		return matches[0], true
	}
	return 0, false
}

// sortSites orders by priority (high first) then name, which is also the
// left-to-right column order of the matrix page.
func sortSites(sites []Site) {
	sort.SliceStable(sites, func(i, j int) bool {
		if sites[i].Priority != sites[j].Priority {
			return sites[i].Priority > sites[j].Priority
		}
		return strings.ToLower(sites[i].Name) < strings.ToLower(sites[j].Name)
	})
}

func normalizeURL(raw string) string {
	return strings.TrimRight(strings.ToLower(strings.TrimSpace(raw)), "/")
}

func hostOf(raw string) string {
	u, err := url.Parse(strings.TrimSpace(raw))
	if err != nil {
		return ""
	}
	return strings.ToLower(u.Host)
}
