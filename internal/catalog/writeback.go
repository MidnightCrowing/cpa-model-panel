package catalog

import (
	"encoding/json"
	"net/url"
	"sort"
	"strings"

	"github.com/local/cpa-model-panel/internal/clean"
	"github.com/local/cpa-model-panel/internal/cpa"
)

// WriteResult is the exact configuration to send back to CPA.
type WriteResult struct {
	Channels map[cpa.Channel][]cpa.Provider
	Changed  map[cpa.Channel]bool
	Kept     int
	Removed  int
	Restored int
	Moved    int
	// Created lists providers the write had to add because a site had no entry
	// in the channel a model was routed to.
	Created []string
}

// ChannelForProtocol maps a protocol tag to the CPA list that serves it.
func ChannelForProtocol(protocol string) cpa.Channel {
	switch protocol {
	case clean.ProtocolCodex:
		return cpa.ChannelCodex
	case clean.ProtocolClaude:
		return cpa.ChannelClaude
	default:
		return cpa.ChannelOpenAI
	}
}

// BuildWrite renders the three channel lists from the catalog.
//
// Two modes, chosen by Settings.RouteByProtocol:
//
//   - off: every model goes back into the provider, channel and position it
//     was read from, so an empty draft reproduces CPA byte for byte.
//   - on: every model goes into the channel its protocol tag names, so
//     codex-api-key holds gpt models and nothing else. When the site has no
//     entry in the target channel one is created from the site's existing
//     credentials — the failure that made the original implementation delete
//     models was dropping them here instead.
//
// Providers the panel does not know about are copied through untouched.
func BuildWrite(cat *Catalog, view View, snap *cpa.Snapshot, priorities map[string]map[string]int) WriteResult {
	models := make(map[EntryRef]ModelView, len(view.Models))
	for _, m := range view.Models {
		models[EntryRef{Site: m.Site, Upstream: m.Upstream}] = m
	}

	owner := make(map[cpa.Channel]map[int]string)
	for _, ch := range cpa.AllChannels {
		owner[ch] = map[int]string{}
	}
	for _, site := range cat.Sites() {
		for ch, idx := range site.Providers {
			owner[ch][idx] = site.ID
		}
	}

	result := WriteResult{
		Channels: make(map[cpa.Channel][]cpa.Provider, len(cpa.AllChannels)),
		Changed:  make(map[cpa.Channel]bool, len(cpa.AllChannels)),
	}

	next := make(map[cpa.Channel][]cpa.Provider, len(cpa.AllChannels))
	for _, ch := range cpa.AllChannels {
		source := snap.Providers(ch)
		clones := make([]cpa.Provider, 0, len(source))
		for _, provider := range source {
			clones = append(clones, cpa.CloneProvider(provider))
		}
		next[ch] = clones
	}

	type slot struct {
		order int
		model cpa.Model
	}
	buckets := make(map[cpa.Channel]map[int][]slot, len(cpa.AllChannels))
	for _, ch := range cpa.AllChannels {
		buckets[ch] = map[int][]slot{}
	}

	// created remembers providers added during this write so several models
	// routed to the same new place share one entry.
	created := map[string]int{}

	place := func(site *Site, ch cpa.Channel) (int, bool) {
		if idx, ok := site.Providers[ch]; ok {
			return idx, true
		}
		key := site.ID + "\x00" + string(ch)
		if idx, ok := created[key]; ok {
			return idx, true
		}
		provider, ok := newProviderFor(*site, ch, snap)
		if !ok {
			return 0, false
		}
		next[ch] = append(next[ch], provider)
		idx := len(next[ch]) - 1
		created[key] = idx
		result.Created = append(result.Created, site.Name+" → "+string(ch))
		return idx, true
	}

	for entryIdx := range cat.Entries {
		entry := &cat.Entries[entryIdx]
		ref := EntryRef{Site: entry.Site, Upstream: entry.Upstream}
		info := models[ref]

		// Record whether this write leaves the model out, so a later reconcile
		// can tell the panel's own removals from CPA-side deletions.
		entry.Withheld = info.Excluded != "" || info.Disabled
		if entry.Withheld {
			continue
		}
		site := cat.Site(entry.Site)
		if site == nil {
			continue
		}

		targets := entryTargets(entry, info, view.Settings.RouteByProtocol)
		alias := entry.Alias()
		for _, target := range targets {
			providerIdx, ok := place(site, target)
			if !ok {
				continue
			}
			order := appendOrder + entryIdx
			var raw map[string]any
			if occ := entry.occurrence(target); occ != nil && occ.providerIdx == providerIdx {
				// Already in the right place: keep its position and payload.
				order = occ.modelIdx
				raw = occ.Raw
			} else if source := entry.anyOccurrence(); source != nil {
				raw = source.Raw
				result.Moved++
			}
			buckets[target][providerIdx] = append(buckets[target][providerIdx], slot{
				order: order,
				model: cpa.Model{Name: entry.Upstream, Alias: alias, Raw: raw},
			})
		}
	}

	for _, ch := range cpa.AllChannels {
		source := snap.Providers(ch)
		for providerIdx := range next[ch] {
			_, known := owner[ch][providerIdx]
			isNew := providerIdx >= len(source)
			if !known && !isNew {
				continue
			}

			slots := buckets[ch][providerIdx]
			sort.SliceStable(slots, func(i, j int) bool { return slots[i].order < slots[j].order })
			written := make([]cpa.Model, 0, len(slots))
			for _, s := range slots {
				written = append(written, s.model)
				if s.order >= appendOrder {
					result.Restored++
				}
			}
			result.Kept += len(written)
			if !isNew {
				result.Removed += len(source[providerIdx].Models) - countPresent(source[providerIdx].Models, written)
			}
			next[ch][providerIdx].Models = written
		}

		// Apply priorities after all models are placed
		for providerIdx := range next[ch] {
			siteID, known := owner[ch][providerIdx]
			if !known {
				continue
			}
			if channelPrios, ok := priorities[siteID]; ok {
				if priority, ok := channelPrios[string(ch)]; ok {
					next[ch][providerIdx].Priority = priority
				}
			}
		}

		result.Channels[ch] = next[ch]
		result.Changed[ch] = !samePayload(source, next[ch])
	}

	return result
}

// entryTargets is where this model should live after the write.
func entryTargets(entry *Entry, info ModelView, route bool) []cpa.Channel {
	if route {
		return []cpa.Channel{ChannelForProtocol(info.Protocol)}
	}
	out := make([]cpa.Channel, 0, len(entry.Occurrences))
	for _, ch := range cpa.AllChannels {
		if entry.occurrence(ch) != nil {
			out = append(out, ch)
		}
	}
	return out
}

// newProviderFor builds a provider entry for a channel the site does not have
// yet, copying its credentials and writing the base URL in the shape that
// channel uses: openai-compatibility and codex-api-key address `/v1`, while
// claude-api-key takes the bare origin.
func newProviderFor(site Site, ch cpa.Channel, snap *cpa.Snapshot) (cpa.Provider, bool) {
	var source *cpa.Provider
	for _, candidate := range cpa.AllChannels {
		if idx, ok := site.Providers[candidate]; ok {
			providers := snap.Providers(candidate)
			if idx < len(providers) {
				source = &providers[idx]
				break
			}
		}
	}
	if source == nil {
		return cpa.Provider{}, false
	}

	keys := source.APIKeys()
	if len(keys) == 0 {
		return cpa.Provider{}, false
	}
	base := channelBaseURL(source.BaseURL, ch)
	if base == "" {
		return cpa.Provider{}, false
	}

	raw := map[string]any{"base-url": base}
	if ch == cpa.ChannelOpenAI {
		raw["name"] = site.Name
		entries := []any{map[string]any{"api-key": keys[0]}}
		raw["api-key-entries"] = entries
	} else {
		raw["api-key"] = keys[0]
	}
	if len(source.Headers) > 0 {
		headers := map[string]any{}
		for k, v := range source.Headers {
			headers[k] = v
		}
		raw["headers"] = headers
	}
	raw["models"] = []any{}

	provider := cpa.Provider{
		BaseURL:       base,
		Headers:       source.Headers,
		APIKeyEntries: []cpa.APIKeyEntry{{APIKey: keys[0]}},
		Raw:           raw,
	}
	if ch == cpa.ChannelOpenAI {
		provider.Name = site.Name
	}
	return provider, true
}

func channelBaseURL(base string, ch cpa.Channel) string {
	trimmed := strings.TrimRight(strings.TrimSpace(base), "/")
	if trimmed == "" {
		return ""
	}
	parsed, err := url.Parse(trimmed)
	if err != nil || parsed.Host == "" {
		return trimmed
	}
	path := strings.TrimRight(parsed.Path, "/")
	if ch == cpa.ChannelClaude {
		parsed.Path = strings.TrimSuffix(path, "/v1")
		return strings.TrimRight(parsed.String(), "/")
	}
	if !strings.HasSuffix(strings.ToLower(path), "/v1") {
		parsed.Path = path + "/v1"
	}
	return parsed.String()
}

// appendOrder is larger than any real model index, so restored, moved and
// newly discovered models sort after the ones already in the provider.
const appendOrder = 1 << 30

func countPresent(before []cpa.Model, after []cpa.Model) int {
	names := make(map[string]bool, len(after))
	for _, m := range after {
		names[m.Name] = true
	}
	n := 0
	for _, m := range before {
		if names[m.Name] {
			n++
		}
	}
	return n
}

func samePayload(a, b []cpa.Provider) bool {
	left, err := json.Marshal(cpa.ChannelPayload(a))
	if err != nil {
		return false
	}
	right, err := json.Marshal(cpa.ChannelPayload(b))
	if err != nil {
		return false
	}
	return string(left) == string(right)
}
