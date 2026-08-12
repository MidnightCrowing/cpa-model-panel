package catalog

import (
	"hash/fnv"
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
//  1. a shared API key
//  2. an identical base-url, when the keys do not disagree
//  3. an unambiguous host match, under the same guard
//
// The key comes first on purpose. Upstreams like new-api serve several groups
// from one host and give each its own key, so a matching base-url is not
// evidence of the same account — a matching key is. Anything still unmatched
// becomes its own site keyed by host.
//
// Entries CPA holds with an empty api-key can never authenticate and are
// dropped: CPA's own UI does not show them either.
func BuildSites(snap *cpa.Snapshot) []Site {
	sites := make([]Site, 0, 48)
	byID := make(map[string]int)
	byURL := make(map[string][]int)
	byKey := make(map[string][]int)
	byHost := make(map[string][]int)

	add := func(id, name string, ch cpa.Channel, idx int, p cpa.Provider) int {
		if existing, ok := byID[id]; ok {
			sites[existing].Providers[ch] = idx
			sites[existing].Priorities[ch] = p.Priority
			if sites[existing].BaseURL == "" {
				sites[existing].BaseURL = p.BaseURL
			}
			if sites[existing].APIKey == "" {
				if keys := p.APIKeys(); len(keys) > 0 {
					sites[existing].APIKey = keys[0]
				}
			}
			return existing
		}
		label, group := splitName(name)
		site := Site{
			ID:         id,
			Name:       name,
			Label:      label,
			Group:      group,
			Priorities: map[cpa.Channel]int{ch: p.Priority},
			BaseURL:    p.BaseURL,
			Headers:    p.Headers,
			Providers:  map[cpa.Channel]int{ch: idx},
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
	//
	// Each channel keeps its own priority: CPA lets one site rank differently
	// per protocol, so collapsing them onto a single number would silently
	// overwrite two of the three on the next save.
	for _, ch := range []cpa.Channel{cpa.ChannelCodex, cpa.ChannelClaude} {
		for idx, p := range snap.Providers(ch) {
			if position, ok := resolveSite(sites, p, byURL, byKey, byHost); ok {
				sites[position].Providers[ch] = idx
				sites[position].Priorities[ch] = p.Priority
				continue
			}
			host := hostOf(p.BaseURL)
			id := "host:" + host
			if host == "" {
				id = "url:" + normalizeURL(p.BaseURL)
			}
			// One host can serve several groups. Merging them would put two
			// different accounts in one matrix column, so a colliding id whose
			// key disagrees gets its own.
			if existing, taken := byID[id]; taken && !keysAgree(sites[existing], p.APIKeys()) {
				if keys := p.APIKeys(); len(keys) > 0 {
					id += " #" + keyFingerprint(keys[0])
				}
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

	// A provider with no api-key can never authenticate; CPA's own UI hides
	// these, so the panel drops them rather than showing a column that fails
	// every probe.
	filtered := make([]Site, 0, len(sites))
	for _, site := range sites {
		if strings.TrimSpace(site.APIKey) != "" {
			filtered = append(filtered, site)
		}
	}

	sortSites(filtered)
	return filtered
}

// splitName splits CPA's provider name on the "<站点> / <分组>" convention.
//
// The group is what distinguishes two entries that share a host, so the matrix
// shows the site name large and the group as a subtitle instead of repeating
// the whole string in every column header.
func splitName(name string) (label, group string) {
	name = strings.TrimSpace(name)
	if idx := strings.LastIndex(name, "/"); idx >= 0 {
		label = strings.TrimSpace(name[:idx])
		group = strings.TrimSpace(name[idx+1:])
		if label != "" && group != "" {
			return label, group
		}
	}
	return name, ""
}

// resolveSite finds the openai-defined site a codex/claude entry belongs to.
func resolveSite(sites []Site, p cpa.Provider, byURL, byKey, byHost map[string][]int) (int, bool) {
	keys := p.APIKeys()

	// A shared key is the only positive evidence of the same account.
	for _, key := range keys {
		if matches := byKey[key]; len(matches) == 1 {
			return matches[0], true
		}
	}

	// A shared base-url is circumstantial: accept it only when the keys do not
	// actively disagree, and never fall through to the weaker host match when
	// they do — that would just find the same site again.
	if matches := byURL[normalizeURL(p.BaseURL)]; len(matches) == 1 {
		if keysAgree(sites[matches[0]], keys) {
			return matches[0], true
		}
		return 0, false
	}

	if matches := byHost[hostOf(p.BaseURL)]; len(matches) == 1 {
		if keysAgree(sites[matches[0]], keys) {
			return matches[0], true
		}
	}
	return 0, false
}

// keysAgree reports whether an entry's keys are compatible with a site's.
//
// Either side having no key is not a contradiction — plenty of entries share a
// host with nothing to compare — but two different non-empty keys mean two
// different groups.
func keysAgree(site Site, keys []string) bool {
	if site.APIKey == "" || len(keys) == 0 {
		return true
	}
	for _, key := range keys {
		if key == site.APIKey {
			return true
		}
	}
	return false
}

// keyFingerprint is a short stable tag for disambiguating ids. The key itself
// never goes into an id: ids are persisted in the panel's store.
func keyFingerprint(key string) string {
	h := fnv.New32a()
	_, _ = h.Write([]byte(key))
	return strconv.FormatUint(uint64(h.Sum32()), 36)
}

// sortSites orders by the site's highest priority across channels (high
// first), then name — the left-to-right column order of the matrix page.
func sortSites(sites []Site) {
	sort.SliceStable(sites, func(i, j int) bool {
		pi, pj := topPriority(sites[i].Priorities), topPriority(sites[j].Priorities)
		if pi != pj {
			return pi > pj
		}
		return strings.ToLower(sites[i].Name) < strings.ToLower(sites[j].Name)
	})
}

func topPriority(priorities map[cpa.Channel]int) int {
	top := 0
	for _, p := range priorities {
		if p > top {
			top = p
		}
	}
	return top
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
