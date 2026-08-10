package catalog

import (
	"encoding/json"
	"sort"

	"github.com/local/cpa-model-panel/internal/cpa"
)

// WriteResult is the exact configuration to send back to CPA.
type WriteResult struct {
	Channels map[cpa.Channel][]cpa.Provider
	Changed  map[cpa.Channel]bool
	Kept     int
	Removed  int
	Restored int
}

// BuildWrite renders the three channel lists from the catalog.
//
// The single most important property: a model is written back into the very
// provider and channel it was read from, in its original position, with its
// original JSON object. With an empty draft this reproduces CPA's current
// configuration byte for byte — which is exactly what the previous
// implementation failed to do (it re-routed models between channels by regex
// and silently dropped the ones whose target list had no entry for the site).
//
// Providers the panel does not know about are copied through untouched.
func BuildWrite(cat *Catalog, view View, snap *cpa.Snapshot, priorities map[string]int) WriteResult {
	excluded := make(map[EntryRef]bool, len(view.Models))
	disabled := make(map[EntryRef]bool, len(view.Models))
	for _, m := range view.Models {
		ref := EntryRef{Site: m.Site, Upstream: m.Upstream}
		if m.Excluded != "" {
			excluded[ref] = true
		}
		if m.Disabled {
			disabled[ref] = true
		}
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

	type slot struct {
		order int
		model cpa.Model
	}
	pending := make(map[cpa.Channel]map[int][]slot)
	for _, ch := range cpa.AllChannels {
		pending[ch] = map[int][]slot{}
	}

	// Bucket every writable occurrence into its target provider.
	for entryIdx := range cat.Entries {
		entry := &cat.Entries[entryIdx]
		ref := EntryRef{Site: entry.Site, Upstream: entry.Upstream}
		if excluded[ref] || disabled[ref] {
			continue
		}
		site := cat.Site(entry.Site)
		if site == nil {
			continue
		}
		for _, occ := range entry.Occurrences {
			providerIdx := occ.providerIdx
			order := occ.modelIdx
			if providerIdx < 0 {
				// Absent (filter-removed or freshly discovered): put it back
				// into the site's provider for that channel.
				idx, ok := site.Providers[occ.Channel]
				if !ok {
					continue
				}
				providerIdx = idx
				order = appendOrder + entryIdx
			}
			pending[occ.Channel][providerIdx] = append(pending[occ.Channel][providerIdx], slot{
				order: order,
				model: cpa.Model{Name: entry.Upstream, Alias: occ.Alias, Raw: occ.Raw},
			})
		}
	}

	result := WriteResult{
		Channels: make(map[cpa.Channel][]cpa.Provider, len(cpa.AllChannels)),
		Changed:  make(map[cpa.Channel]bool, len(cpa.AllChannels)),
	}

	for _, ch := range cpa.AllChannels {
		source := snap.Providers(ch)
		next := make([]cpa.Provider, 0, len(source))
		for providerIdx, provider := range source {
			out := cpa.CloneProvider(provider)
			siteID, known := owner[ch][providerIdx]
			if !known {
				next = append(next, out)
				continue
			}

			slots := pending[ch][providerIdx]
			sort.SliceStable(slots, func(i, j int) bool { return slots[i].order < slots[j].order })
			models := make([]cpa.Model, 0, len(slots))
			for _, s := range slots {
				models = append(models, s.model)
				if s.order >= appendOrder {
					result.Restored++
				}
			}
			result.Kept += len(models)
			result.Removed += len(provider.Models) - countPresent(provider.Models, models)
			out.Models = models

			if priority, ok := priorities[siteID]; ok {
				out.Priority = priority
			}
			next = append(next, out)
		}
		result.Channels[ch] = next
		result.Changed[ch] = !samePayload(source, next)
	}

	return result
}

// appendOrder is larger than any real model index, so restored and newly
// discovered models sort after the ones already in the provider.
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
