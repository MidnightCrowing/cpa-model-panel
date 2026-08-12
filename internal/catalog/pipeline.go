package catalog

import (
	"fmt"
	"regexp"
	"sort"
	"strings"

	"github.com/local/cpa-model-panel/internal/clean"
	"github.com/local/cpa-model-panel/internal/cpa"
)

// Exclusion reasons, in precedence order.
const (
	ExcludedManual    = "manual"
	ExcludedWhitelist = "whitelist"
	ExcludedVersion   = "version"
)

// Settings is everything the pipeline is configured with.
type Settings struct {
	Prefixes []string `json:"prefixes"`
	Suffixes []string `json:"suffixes"`
	// Protect stops suffix stripping for names that legitimately end in one
	// of them; Rewrites normalise what is left.
	Protect   string                    `json:"protect"`
	Rewrites  []clean.Rewrite           `json:"rewrites"`
	Whitelist string                    `json:"whitelist"`
	Version   clean.VersionFilterConfig `json:"version"`
	Protocol  clean.ProtocolConfig      `json:"protocol"`
	// RouteByProtocol makes CPA's three lists a projection of the panel: each
	// model is written to the list its protocol names, instead of staying in
	// whichever list it happens to sit in today.
	RouteByProtocol bool `json:"route_by_protocol"`
}

// CleaningRules is the cleaning half of the settings.
func (s Settings) CleaningRules() clean.RulesConfig {
	return clean.RulesConfig{
		Prefixes: s.Prefixes,
		Suffixes: s.Suffixes,
		Protect:  s.Protect,
		Rewrites: s.Rewrites,
	}
}

type Inputs struct {
	Catalog    *Catalog
	Settings   Settings
	Exclusions RefSet
	Disabled   RefSet
	// Keeps are per-model overrides that survive the whitelist and the version
	// filter — the escape hatch for the one model a rule gets wrong.
	Keeps RefSet
	// TempSites maps a site id to where its link was shared.
	TempSites map[string]string
	// Health is the outcome of each site's last model-list probe.
	Health map[string]SiteProbe
}

// SiteProbe mirrors the stored health record without importing the store.
type SiteProbe struct {
	LastOKAt  string
	LastError string
	Failures  int
}

// ModelView is one row as the UI sees it.
type ModelView struct {
	Site     string `json:"site"`
	Upstream string `json:"upstream"`
	Alias    string `json:"alias"`
	// Canonical is the name after cleaning: the remap when set, otherwise the
	// cleaned upstream name. Protocol matching runs against it.
	Canonical string   `json:"canonical"`
	Suggested string   `json:"suggested"`
	Protocol  string   `json:"protocol"`
	Channels  []string `json:"channels"`
	Excluded  string   `json:"excluded,omitempty"`
	Reason    string   `json:"reason,omitempty"`
	Kept      bool     `json:"kept,omitempty"`
	Disabled  bool     `json:"disabled,omitempty"`
	Pending   bool     `json:"pending,omitempty"`
	// Present reports whether CPA currently holds the model. False means the
	// next save would add it; true plus an exclusion means the next save would
	// remove it.
	Present bool `json:"present"`
	// Writable is false when the model cannot be written at all because the
	// site has no provider in any channel the model belongs to. Without this
	// such an entry would sit in to_add forever and the save button would
	// never go quiet.
	Writable bool `json:"writable"`
	// Target is the CPA list this model belongs in. With routing off it is
	// simply where it already is.
	Target string `json:"target"`
}

type SiteView struct {
	ID       string   `json:"id"`
	Name     string   `json:"name"`
	Priority int      `json:"priority"`
	Channels []string `json:"channels"`
	Active   int      `json:"active"`
	BaseURL  string   `json:"base_url"`
	// HasKey is false for entries CPA holds with api-key: "". Its own UI hides
	// those, so they are invisible there while still failing every probe.
	HasKey bool `json:"has_key"`
	// Temp marks a short-lived endpoint added from a shared link.
	Temp      bool   `json:"temp,omitempty"`
	SourceURL string `json:"source_url,omitempty"`
	// Health of the last model-list probe.
	LastOKAt  string `json:"last_ok_at,omitempty"`
	LastError string `json:"last_error,omitempty"`
	Failures  int    `json:"failures,omitempty"`
}

type Stats struct {
	Models      int            `json:"models"`
	Active      int            `json:"active"`
	Excluded    int            `json:"excluded"`
	Disabled    int            `json:"disabled"`
	Pending     int            `json:"pending"`
	ByExclusion map[string]int `json:"by_exclusion"`
	// ToAdd / ToRemove are the difference between the panel and CPA that a
	// save would apply even with an empty draft: models discovered by a
	// refresh, and models the rules exclude but CPA still has.
	ToAdd    int `json:"to_add"`
	ToRemove int `json:"to_remove"`
	// ToMove counts models sitting in a CPA list their protocol does not name.
	ToMove int `json:"to_move"`
}

type View struct {
	Fingerprint string      `json:"fingerprint"`
	FetchedAt   string      `json:"fetched_at"`
	Sites       []SiteView  `json:"sites"`
	Models      []ModelView `json:"models"`
	Stats       Stats       `json:"stats"`
	Settings    Settings    `json:"settings"`
}

// Compute runs the whole pipeline:
//
//	catalog → whitelist → version filter → manual deletions → protocol tag
//	        → prefix/suffix suggestion → site-level disable → view
//
// Filters only *label* entries. Nothing is dropped here, which is what lets
// the naming page show excluded models with a restore button and lets a
// relaxed rule bring models straight back.
func Compute(in Inputs) (View, error) {
	rules, err := clean.NewRules(in.Settings.CleaningRules())
	if err != nil {
		return View{}, fmt.Errorf("名称清洗规则无效: %w", err)
	}

	matcher, err := clean.NewProtocolMatcher(in.Settings.Protocol)
	if err != nil {
		return View{}, fmt.Errorf("协议标记正则无效: %w", err)
	}

	var whitelist *regexp.Regexp
	if pattern := strings.TrimSpace(in.Settings.Whitelist); pattern != "" {
		whitelist, err = regexp.Compile(pattern)
		if err != nil {
			return View{}, fmt.Errorf("模型白名单正则无效: %w", err)
		}
	}

	view := View{
		FetchedAt: in.Catalog.FetchedAt,
		Models:    make([]ModelView, 0, len(in.Catalog.Entries)),
		Settings:  in.Settings,
		Stats:     Stats{ByExclusion: map[string]int{}},
	}

	activeBySite := make(map[string]int)

	for i := range in.Catalog.Entries {
		entry := &in.Catalog.Entries[i]
		alias := entry.Alias()
		effective := alias
		if effective == "" {
			effective = entry.Upstream
		}

		// The canonical name is what the model is actually called after
		// cleaning: its remap when one is set, otherwise the cleaned upstream
		// name. Protocol matching runs against this rather than the raw
		// upstream name — vendor prefixes, `[free]` markers and `(xhigh)[1M]`
		// suffixes are exactly the noise that makes a protocol regex hard to
		// write.
		canonical := alias
		if canonical == "" {
			canonical = rules.Clean(entry.Upstream)
		}
		if canonical == "" {
			canonical = entry.Upstream
		}

		// A suggestion is only worth showing when the name in use is not
		// already clean; otherwise every tidy row would sprout a button that
		// makes it worse.
		suggested := rules.Clean(entry.Upstream)
		if suggested == effective || rules.Clean(effective) == effective {
			suggested = ""
		}

		model := ModelView{
			Site:      entry.Site,
			Upstream:  entry.Upstream,
			Alias:     alias,
			Canonical: canonical,
			Suggested: suggested,
			Protocol:  matcher.Classify(canonical),
			Channels:  entry.Channels(),
			Pending:   entry.Pending,
			Kept:      in.Keeps.Has(entry.Site, entry.Upstream),
			Disabled:  in.Disabled.Has(entry.Site, entry.Upstream),
			Present:   entry.Present,
			Writable:  entry.Present || in.Catalog.canWrite(entry),
		}
		model.Target = string(ChannelForProtocol(model.Protocol))
		if !in.Settings.RouteByProtocol {
			model.Target = ""
			if occ := entry.anyOccurrence(); occ != nil {
				model.Target = string(occ.Channel)
			}
		}

		switch {
		case in.Exclusions.Has(entry.Site, entry.Upstream):
			model.Excluded = ExcludedManual
		case model.Kept:
			// Explicitly kept: rules do not apply.
		case whitelist != nil && !whitelist.MatchString(entry.Upstream):
			// Whitelist matches the *original* upstream name only.
			model.Excluded = ExcludedWhitelist
		default:
			if verdict := clean.Evaluate(in.Settings.Version, entry.Upstream, alias, rules.Clean(entry.Upstream)); !verdict.Keep {
				model.Excluded = ExcludedVersion
				model.Reason = fmt.Sprintf("%s %.1f < %.1f", verdict.Series, verdict.Version, verdict.MinVersion)
			}
		}

		view.Stats.Models++
		if entry.Pending {
			view.Stats.Pending++
		}
		if hidden := model.Excluded != "" || model.Disabled; hidden != !entry.Present {
			if entry.Present {
				view.Stats.ToRemove++
			} else if model.Writable {
				view.Stats.ToAdd++
			}
		} else if in.Settings.RouteByProtocol && entry.Present && !hidden && misplaced(entry, model.Target) {
			view.Stats.ToMove++
		}
		if model.Excluded != "" {
			view.Stats.Excluded++
			view.Stats.ByExclusion[model.Excluded]++
		} else if model.Disabled {
			view.Stats.Disabled++
		} else {
			view.Stats.Active++
			activeBySite[entry.Site]++
		}

		view.Models = append(view.Models, model)
	}

	for _, site := range in.Catalog.Sites() {
		channels := make([]string, 0, len(site.Providers))
		for _, ch := range cpa.AllChannels {
			if site.HasChannel(ch) {
				channels = append(channels, string(ch))
			}
		}
		entry := SiteView{
			ID:       site.ID,
			Name:     site.Name,
			Priority: site.Priority,
			Channels: channels,
			Active:   activeBySite[site.ID],
			BaseURL:  site.BaseURL,
			HasKey:   strings.TrimSpace(site.APIKey) != "",
		}
		if temp, ok := in.TempSites[site.ID]; ok {
			entry.Temp = true
			entry.SourceURL = temp
		}
		if health, ok := in.Health[site.ID]; ok {
			entry.LastOKAt = health.LastOKAt
			entry.LastError = health.LastError
			entry.Failures = health.Failures
		}
		view.Sites = append(view.Sites, entry)
	}

	return view, nil
}

// misplaced reports whether the model occupies anything other than exactly
// the one list it belongs in.
func misplaced(entry *Entry, target string) bool {
	if len(entry.Occurrences) != 1 {
		return true
	}
	return string(entry.Occurrences[0].Channel) != target
}

func sortEntries(entries []Entry) {
	sort.SliceStable(entries, func(i, j int) bool {
		li, lj := strings.ToLower(entries[i].Upstream), strings.ToLower(entries[j].Upstream)
		if li != lj {
			return li < lj
		}
		return entries[i].Site < entries[j].Site
	})
}
