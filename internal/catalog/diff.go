package catalog

import (
	"sort"

	"github.com/local/cpa-model-panel/internal/cpa"
)

// ChannelDiff is what a save would change in one CPA list.
type ChannelDiff struct {
	Channel string   `json:"channel"`
	Added   []string `json:"added"`
	Removed []string `json:"removed"`
	Renamed []string `json:"renamed"`
	// Providers added because a site had no entry in this list.
	NewProviders []string `json:"new_providers"`
}

// Diff compares what CPA holds with what a write would put there, per
// provider, so a preview can name every model that moves rather than only
// counting them.
func Diff(snap *cpa.Snapshot, write WriteResult) []ChannelDiff {
	out := make([]ChannelDiff, 0, len(cpa.AllChannels))
	for _, ch := range cpa.AllChannels {
		before := snap.Providers(ch)
		after := write.Channels[ch]
		diff := ChannelDiff{Channel: string(ch), Added: []string{}, Removed: []string{}, Renamed: []string{}, NewProviders: []string{}}

		for i, provider := range after {
			label := providerLabel(provider)
			if i >= len(before) {
				diff.NewProviders = append(diff.NewProviders, label)
				for _, model := range provider.Models {
					diff.Added = append(diff.Added, label+": "+model.Name)
				}
				continue
			}
			was := map[string]string{}
			for _, model := range before[i].Models {
				was[model.Name] = model.Alias
			}
			now := map[string]string{}
			for _, model := range provider.Models {
				now[model.Name] = model.Alias
			}
			for name, alias := range now {
				previous, existed := was[name]
				if !existed {
					diff.Added = append(diff.Added, label+": "+name)
				} else if previous != alias {
					diff.Renamed = append(diff.Renamed, label+": "+name+" ("+orDash(previous)+" → "+orDash(alias)+")")
				}
			}
			for name := range was {
				if _, kept := now[name]; !kept {
					diff.Removed = append(diff.Removed, label+": "+name)
				}
			}
		}

		sort.Strings(diff.Added)
		sort.Strings(diff.Removed)
		sort.Strings(diff.Renamed)
		out = append(out, diff)
	}
	return out
}

func providerLabel(p cpa.Provider) string {
	if p.Name != "" {
		return p.Name
	}
	return p.BaseURL
}

func orDash(s string) string {
	if s == "" {
		return "—"
	}
	return s
}
