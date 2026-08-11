// Command dryrun proves, against a live CPA, that a save with no edits would
// not change anything. It only ever issues GETs.
//
//	CPA_BASE_URL=... CPA_MANAGEMENT_SECRET=... go run ./cmd/dryrun
package main

import (
	"encoding/json"
	"fmt"
	"log"
	"os"
	"strings"

	"github.com/local/cpa-model-panel/internal/catalog"
	"github.com/local/cpa-model-panel/internal/clean"
	"github.com/local/cpa-model-panel/internal/cpa"
)

func main() {
	base := strings.TrimRight(os.Getenv("CPA_BASE_URL"), "/")
	secret := os.Getenv("CPA_MANAGEMENT_SECRET")
	if base == "" || secret == "" {
		log.Fatal("CPA_BASE_URL and CPA_MANAGEMENT_SECRET are required")
	}

	client := cpa.NewClient(base, secret)
	snapshot, err := client.Snapshot()
	if err != nil {
		log.Fatalf("read CPA: %v", err)
	}

	cat := catalog.Reconcile(nil, snapshot)
	settings := catalog.Settings{
		Prefixes: clean.DefaultPrefixes,
		Suffixes: clean.DefaultSuffixes,
		Protocol: clean.DefaultProtocolConfig(),
	}
	// Optional: check what the *deployed* settings would do before shipping.
	if path := os.Getenv("SETTINGS_JSON"); path != "" {
		raw, err := os.ReadFile(path)
		if err != nil {
			log.Fatalf("read settings: %v", err)
		}
		if err := json.Unmarshal(raw, &settings); err != nil {
			log.Fatalf("parse settings: %v", err)
		}
	}
	view, err := catalog.Compute(catalog.Inputs{Catalog: cat, Settings: settings})
	if err != nil {
		log.Fatalf("pipeline: %v", err)
	}
	if view.Stats.Excluded > 0 {
		fmt.Printf("would remove %d models from CPA on the next save: %v\n", view.Stats.Excluded, view.Stats.ByExclusion)
	}

	fmt.Printf("sites=%d models=%d active=%d excluded=%d\n",
		len(view.Sites), view.Stats.Models, view.Stats.Active, view.Stats.Excluded)
	protocols := map[string]int{}
	for _, m := range view.Models {
		protocols[m.Protocol]++
	}
	fmt.Printf("protocol tags: %v\n", protocols)

	write := catalog.BuildWrite(cat, view, snapshot, nil)

	// Nothing may go missing: every model the panel considers active has to
	// appear exactly once in the configuration being written.
	written := map[string]int{}
	total := 0
	for _, ch := range cpa.AllChannels {
		for _, provider := range write.Channels[ch] {
			for _, model := range provider.Models {
				written[provider.BaseURL+"|"+model.Name]++
				total++
			}
		}
	}
	duplicates := 0
	for _, n := range written {
		if n > 1 {
			duplicates++
		}
	}
	fmt.Printf("active=%d written=%d unique=%d duplicated=%d moved=%d created=%v\n",
		view.Stats.Active, total, len(written), duplicates, write.Moved, write.Created)

	missing := 0
	for _, m := range view.Models {
		if m.Excluded != "" || m.Disabled {
			continue
		}
		found := false
		for _, ch := range cpa.AllChannels {
			for _, provider := range write.Channels[ch] {
				for _, model := range provider.Models {
					if model.Name == m.Upstream {
						found = true
					}
				}
			}
		}
		if !found {
			missing++
			if missing <= 5 {
				fmt.Println("  MISSING", m.Site, "/", m.Upstream)
			}
		}
	}
	fmt.Println("models that would disappear:", missing)

	identical := true
	for _, ch := range cpa.AllChannels {
		before, _ := json.Marshal(cpa.ChannelPayload(snapshot.Providers(ch)))
		after, _ := json.Marshal(cpa.ChannelPayload(write.Channels[ch]))
		status := "IDENTICAL"
		if string(before) != string(after) {
			status = "DIFFERENT"
			identical = false
		}
		fmt.Printf("%-7s providers=%-3d bytes %d -> %d  %s\n",
			ch, len(write.Channels[ch]), len(before), len(after), status)
		if status == "DIFFERENT" {
			reportDiff(ch, snapshot.Providers(ch), write.Channels[ch])
		}
	}

	if !identical {
		os.Exit(1)
	}
	fmt.Println("\nno-op save would write nothing — CPA configuration is reproduced exactly")
}

func reportDiff(ch cpa.Channel, before, after []cpa.Provider) {
	for i := range before {
		if i >= len(after) {
			fmt.Printf("  %s: provider %q disappeared\n", ch, before[i].Name)
			continue
		}
		lost := missing(before[i].Models, after[i].Models)
		gained := missing(after[i].Models, before[i].Models)
		if len(lost) == 0 && len(gained) == 0 {
			continue
		}
		fmt.Printf("  %s / %s: -%d +%d\n", ch, label(before[i]), len(lost), len(gained))
		for _, name := range lost {
			fmt.Printf("      - %s\n", name)
		}
		for _, name := range gained {
			fmt.Printf("      + %s\n", name)
		}
	}
}

func label(p cpa.Provider) string {
	if p.Name != "" {
		return p.Name
	}
	return p.BaseURL
}

func missing(from, in []cpa.Model) []string {
	present := make(map[string]struct{}, len(in))
	for _, m := range in {
		present[m.Name] = struct{}{}
	}
	out := []string{}
	for _, m := range from {
		if _, ok := present[m.Name]; !ok {
			out = append(out, m.Name)
		}
	}
	return out
}
