package catalog

import (
	"github.com/local/cpa-model-panel/internal/clean"
	"github.com/local/cpa-model-panel/internal/cpa"
)

// Reconcile merges the panel's cached catalog with a fresh CPA snapshot.
//
// CPA is the source of truth for everything it currently holds. Cached entries
// that CPA no longer has are kept with Present=false — those are the models the
// panel itself filtered out, and keeping them is what makes every filter
// reversible. Prune() later drops the ones no filter explains.
func Reconcile(cached *Catalog, snap *cpa.Snapshot) *Catalog {
	sites := BuildSites(snap)

	// provider index → site id, per channel
	owner := make(map[cpa.Channel]map[int]string, len(cpa.AllChannels))
	for _, ch := range cpa.AllChannels {
		owner[ch] = map[int]string{}
	}
	for _, site := range sites {
		for ch, idx := range site.Providers {
			owner[ch][idx] = site.ID
		}
	}

	out := &Catalog{}
	if cached != nil {
		out.FetchedAt = cached.FetchedAt
	}

	index := make(map[EntryRef]int)
	for _, ch := range cpa.AllChannels {
		for providerIdx, provider := range snap.Providers(ch) {
			siteID, ok := owner[ch][providerIdx]
			if !ok {
				continue
			}
			for modelIdx, model := range provider.Models {
				ref := EntryRef{Site: siteID, Upstream: model.Name}
				position, exists := index[ref]
				if !exists {
					out.Entries = append(out.Entries, Entry{
						Site:     siteID,
						Upstream: model.Name,
						Present:  true,
					})
					position = len(out.Entries) - 1
					index[ref] = position
				}
				out.Entries[position].Occurrences = append(out.Entries[position].Occurrences, Occurrence{
					Channel:     ch,
					Alias:       model.Alias,
					Raw:         model.Raw,
					providerIdx: providerIdx,
					modelIdx:    modelIdx,
				})
			}
		}
	}

	if cached != nil {
		known := make(map[string]bool, len(sites))
		for _, site := range sites {
			known[site.ID] = true
		}
		for _, entry := range cached.Entries {
			ref := EntryRef{Site: entry.Site, Upstream: entry.Upstream}
			if _, live := index[ref]; live {
				continue
			}
			if !known[entry.Site] {
				// The site itself is gone from CPA; drop its leftovers.
				continue
			}
			absent := entry
			absent.Present = false
			for i := range absent.Occurrences {
				absent.Occurrences[i].providerIdx = -1
				absent.Occurrences[i].modelIdx = -1
			}
			out.Entries = append(out.Entries, absent)
			index[ref] = len(out.Entries) - 1
		}
	}

	out.setSites(sites)
	sortEntries(out.Entries)
	return out
}

// MergeDiscovered folds a site's upstream model list into the catalog. Models
// already known are left untouched (their alias and extra fields survive);
// genuinely new ones are added as pending entries that the next save writes.
//
// A new model lands in the channel its protocol suggests when the site has
// that channel configured, otherwise openai-compatibility, otherwise whatever
// single channel the site does have.
func MergeDiscovered(cat *Catalog, siteID string, names []string, matcher *clean.ProtocolMatcher, rules clean.Rules) int {
	site := cat.Site(siteID)
	if site == nil {
		return 0
	}
	index := cat.entryIndex()
	added := 0
	for _, name := range names {
		ref := EntryRef{Site: siteID, Upstream: name}
		if _, exists := index[ref]; exists {
			continue
		}
		canonical := rules.Clean(name)
		if canonical == "" {
			canonical = name
		}
		channel := targetChannel(*site, matcher.Classify(canonical))
		if channel == "" {
			continue
		}
		cat.Entries = append(cat.Entries, Entry{
			Site:        siteID,
			Upstream:    name,
			Present:     false,
			Pending:     true,
			Occurrences: []Occurrence{{Channel: channel, providerIdx: -1, modelIdx: -1}},
		})
		index[ref] = len(cat.Entries) - 1
		added++
	}
	if added > 0 {
		sortEntries(cat.Entries)
	}
	return added
}

func targetChannel(site Site, protocol string) cpa.Channel {
	switch protocol {
	case clean.ProtocolCodex:
		if site.HasChannel(cpa.ChannelCodex) {
			return cpa.ChannelCodex
		}
	case clean.ProtocolClaude:
		if site.HasChannel(cpa.ChannelClaude) {
			return cpa.ChannelClaude
		}
	}
	if site.HasChannel(cpa.ChannelOpenAI) {
		return cpa.ChannelOpenAI
	}
	for _, ch := range cpa.AllChannels {
		if site.HasChannel(ch) {
			return ch
		}
	}
	return ""
}

// Prune drops absent entries that no longer have a reason to exist: if CPA
// does not have the model and no filter excluded it, it was deleted outside
// the panel and must not be resurrected on the next save.
func Prune(cat *Catalog, view View) {
	excluded := make(map[EntryRef]bool, len(view.Models))
	for _, m := range view.Models {
		if m.Excluded != "" {
			excluded[EntryRef{Site: m.Site, Upstream: m.Upstream}] = true
		}
	}
	kept := cat.Entries[:0]
	for _, entry := range cat.Entries {
		if entry.Present || entry.Pending || excluded[EntryRef{Site: entry.Site, Upstream: entry.Upstream}] {
			kept = append(kept, entry)
		}
	}
	cat.Entries = kept
}
