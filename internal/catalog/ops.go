package catalog

import (
	"fmt"
	"strings"
)

// Operation types sent by the UI. Every model-level op carries an explicit
// target list, so an edit made on one row touches exactly the (site, model)
// pairs of that row and nothing else.
const (
	OpRename      = "rename"
	OpExclude     = "exclude"
	OpInclude     = "include"
	OpKeep        = "keep"
	OpUnkeep      = "unkeep"
	OpSetDisabled = "set_disabled"
	OpSetPriority = "set_priority"
)

type Op struct {
	Type     string     `json:"type"`
	Targets  []EntryRef `json:"targets,omitempty"`
	To       string     `json:"to,omitempty"`
	Disabled bool       `json:"disabled,omitempty"`
	Site     string     `json:"site,omitempty"`
	Priority int        `json:"priority,omitempty"`
}

type OpResult struct {
	Exclusions RefSet
	Disabled   RefSet
	Keeps      RefSet
	Priorities map[string]int
	Renamed    int
	Skipped    int
}

// ApplyOps folds the draft into the catalog and returns the persistent state
// the store must be updated to. The catalog's aliases are mutated in place.
func ApplyOps(cat *Catalog, exclusions, disabled, keeps RefSet, ops []Op) (OpResult, error) {
	result := OpResult{
		Exclusions: copyRefSet(exclusions),
		Disabled:   copyRefSet(disabled),
		Keeps:      copyRefSet(keeps),
		Priorities: map[string]int{},
	}

	index := cat.entryIndex()

	for _, op := range ops {
		switch op.Type {
		case OpRename:
			alias := strings.TrimSpace(op.To)
			for _, ref := range op.Targets {
				position, ok := index[ref]
				if !ok {
					result.Skipped++
					continue
				}
				entry := &cat.Entries[position]
				next := alias
				if next == entry.Upstream {
					// An alias equal to the upstream name is a no-op remap;
					// clear it so CPA's config stays minimal.
					next = ""
				}
				for i := range entry.Occurrences {
					entry.Occurrences[i].Alias = next
				}
				result.Renamed++
			}

		case OpExclude, OpInclude:
			for _, ref := range op.Targets {
				if _, ok := index[ref]; !ok {
					result.Skipped++
					continue
				}
				if op.Type == OpExclude {
					result.Exclusions[ref] = true
				} else {
					delete(result.Exclusions, ref)
				}
			}

		case OpKeep, OpUnkeep:
			for _, ref := range op.Targets {
				if _, ok := index[ref]; !ok {
					result.Skipped++
					continue
				}
				if op.Type == OpKeep {
					result.Keeps[ref] = true
					delete(result.Exclusions, ref)
				} else {
					delete(result.Keeps, ref)
				}
			}

		case OpSetDisabled:
			for _, ref := range op.Targets {
				if _, ok := index[ref]; !ok {
					result.Skipped++
					continue
				}
				if op.Disabled {
					result.Disabled[ref] = true
				} else {
					delete(result.Disabled, ref)
				}
			}

		case OpSetPriority:
			site := strings.TrimSpace(op.Site)
			if site == "" {
				return result, fmt.Errorf("set_priority 缺少站点")
			}
			if cat.Site(site) == nil {
				result.Skipped++
				continue
			}
			result.Priorities[site] = op.Priority

		default:
			return result, fmt.Errorf("未知操作类型 %q", op.Type)
		}
	}

	return result, nil
}

func copyRefSet(in RefSet) RefSet {
	out := make(RefSet, len(in))
	for k, v := range in {
		if v {
			out[k] = true
		}
	}
	return out
}
