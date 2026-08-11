package catalog

import (
	"github.com/local/cpa-model-panel/internal/cpa"
)

// Occurrence is one physical copy of a model inside a CPA channel list.
//
// A model can legitimately exist in more than one channel (the same site can
// serve it over openai-compatibility and codex-api-key); each copy is tracked
// separately so writeback puts every copy back exactly where it came from.
type Occurrence struct {
	Channel cpa.Channel    `json:"channel"`
	Alias   string         `json:"alias,omitempty"`
	Raw     map[string]any `json:"raw,omitempty"`

	// Live position inside the current CPA snapshot. Not persisted: it is
	// recomputed on every load because CPA may be edited elsewhere.
	providerIdx int
	modelIdx    int
}

// Entry is one (site, upstream model name) pair — the panel's unit of work.
type Entry struct {
	Site     string `json:"site"`
	Upstream string `json:"upstream"`

	// Present reports whether the model currently exists in CPA. Entries that
	// the panel itself filtered out stay in the cache with Present=false so
	// relaxing a filter brings them back instead of losing them forever.
	Present bool `json:"present"`

	// Pending marks a model discovered from a site's own /v1/models that has
	// never been written to CPA yet.
	Pending bool `json:"pending,omitempty"`

	// Withheld records that the panel's own last write left this model out.
	// It is what separates "we removed it and can put it back" from "someone
	// deleted it in CPA and it must stay gone" — the two look identical
	// otherwise, and Prune has to keep the first and drop the second.
	Withheld bool `json:"withheld,omitempty"`

	Occurrences []Occurrence `json:"occurrences"`
}

func (e *Entry) occurrence(ch cpa.Channel) *Occurrence {
	for i := range e.Occurrences {
		if e.Occurrences[i].Channel == ch {
			return &e.Occurrences[i]
		}
	}
	return nil
}

// Alias returns the effective remapped name, preferring the openai channel so
// a site with copies in several channels shows one stable name.
func (e *Entry) Alias() string {
	for _, ch := range cpa.AllChannels {
		if occ := e.occurrence(ch); occ != nil && occ.Alias != "" {
			return occ.Alias
		}
	}
	return ""
}

// anyOccurrence returns a copy the write can take field values from when the
// model is moving to a channel it is not in yet.
func (e *Entry) anyOccurrence() *Occurrence {
	for _, ch := range cpa.AllChannels {
		if occ := e.occurrence(ch); occ != nil {
			return occ
		}
	}
	return nil
}

func (e *Entry) Channels() []string {
	out := make([]string, 0, len(e.Occurrences))
	for _, ch := range cpa.AllChannels {
		if e.occurrence(ch) != nil {
			out = append(out, string(ch))
		}
	}
	return out
}

// Site is a logical站点: one CPA account, possibly configured in several
// channel lists under different (or missing) names.
type Site struct {
	ID       string
	Name     string
	Priority int
	BaseURL  string
	Headers  map[string]string
	APIKey   string

	// Providers maps a channel to the provider index inside the snapshot.
	Providers map[cpa.Channel]int
}

func (s Site) HasChannel(ch cpa.Channel) bool {
	_, ok := s.Providers[ch]
	return ok
}

// Catalog is the panel's full picture of every model it has ever seen.
type Catalog struct {
	FetchedAt string  `json:"fetched_at"`
	Entries   []Entry `json:"entries"`

	sites   []Site
	siteMap map[string]*Site
}

func (c *Catalog) Sites() []Site { return c.sites }

func (c *Catalog) Site(id string) *Site {
	if c.siteMap == nil {
		return nil
	}
	return c.siteMap[id]
}

func (c *Catalog) setSites(sites []Site) {
	c.sites = sites
	c.siteMap = make(map[string]*Site, len(sites))
	for i := range c.sites {
		c.siteMap[c.sites[i].ID] = &c.sites[i]
	}
}

// canWrite reports whether the entry has somewhere to go: its site must still
// have a provider in one of the channels the model belongs to.
func (c *Catalog) canWrite(entry *Entry) bool {
	site := c.Site(entry.Site)
	if site == nil {
		return false
	}
	for _, occ := range entry.Occurrences {
		if site.HasChannel(occ.Channel) {
			return true
		}
	}
	return false
}

// EntryRef addresses one entry from the UI.
type EntryRef struct {
	Site     string `json:"site"`
	Upstream string `json:"upstream"`
}

type RefSet map[EntryRef]bool

func (s RefSet) Has(site, upstream string) bool {
	return s[EntryRef{Site: site, Upstream: upstream}]
}

func (c *Catalog) entryIndex() map[EntryRef]int {
	out := make(map[EntryRef]int, len(c.Entries))
	for i, e := range c.Entries {
		out[EntryRef{Site: e.Site, Upstream: e.Upstream}] = i
	}
	return out
}
