package catalog

import (
	"encoding/json"
	"testing"

	"github.com/local/cpa-model-panel/internal/clean"
	"github.com/local/cpa-model-panel/internal/cpa"
)

// fixture mirrors the real CPA shape: openai entries have names and
// api-key-entries, codex/claude entries have no name, a flat api-key and an
// auth-index the panel must not lose.
func fixture() *cpa.Snapshot {
	raw := map[string][]map[string]any{
		"openai": {
			{
				"name":     "示例站点 / free",
				"base-url": "https://ai.example.com/v1",
				"api-key-entries": []any{
					map[string]any{"api-key": "key-free"},
				},
				"models": []any{
					map[string]any{"name": "deepseek-ai/DeepSeek-V3", "alias": "deepseek-v3"},
					map[string]any{"name": "[free]glm-4.5", "alias": "glm-4.5", "max-context-length": float64(131072)},
				},
			},
			{
				"name":     "示例站点 / office",
				"base-url": "https://ai.example.com/v1/office",
				"api-key-entries": []any{
					map[string]any{"api-key": "key-office"},
				},
				"models": []any{
					map[string]any{"name": "qwen-max", "alias": "qwen-max"},
				},
			},
		},
		"codex": {
			{
				"base-url":   "https://ai.example.com/v1/office",
				"api-key":    "key-office",
				"auth-index": float64(3),
				"models": []any{
					map[string]any{"name": "openai/gpt-5.4", "alias": "gpt-5.4"},
					map[string]any{"name": "anthropic/claude-opus-4.5", "alias": "claude-opus-4.5"},
				},
			},
		},
		"claude": {
			{
				"base-url":   "https://ai.example.com",
				"api-key":    "key-free",
				"auth-index": float64(1),
				"models": []any{
					map[string]any{"name": "[free]claude-sonnet-5", "alias": "claude-sonnet-5"},
				},
			},
		},
	}

	snap := &cpa.Snapshot{Channels: map[cpa.Channel][]cpa.Provider{}}
	for key, entries := range raw {
		providers, err := cpa.ProvidersFromPayload(entries)
		if err != nil {
			panic(err)
		}
		snap.Channels[cpa.Channel(key)] = providers
	}
	return snap
}

func defaultSettings() Settings {
	return Settings{
		Prefixes: clean.DefaultPrefixes,
		Suffixes: clean.DefaultSuffixes,
		Protocol: clean.DefaultProtocolConfig(),
	}
}

func compute(t *testing.T, cat *Catalog, settings Settings, exclusions, disabled RefSet) View {
	t.Helper()
	view, err := Compute(Inputs{Catalog: cat, Settings: settings, Exclusions: exclusions, Disabled: disabled})
	if err != nil {
		t.Fatalf("Compute: %v", err)
	}
	return view
}

func payloadJSON(t *testing.T, providers []cpa.Provider) string {
	t.Helper()
	encoded, err := json.Marshal(cpa.ChannelPayload(providers))
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	return string(encoded)
}

// The single most important guarantee: saving without edits must reproduce
// CPA's configuration exactly. The old implementation re-routed models between
// channels here and silently dropped any whose target list lacked the site.
func TestWritebackWithoutEditsIsIdentity(t *testing.T) {
	snap := fixture()
	cat := Reconcile(nil, snap)
	view := compute(t, cat, defaultSettings(), RefSet{}, RefSet{})

	write := BuildWrite(cat, view, snap, nil)

	for _, ch := range cpa.AllChannels {
		before := payloadJSON(t, snap.Providers(ch))
		after := payloadJSON(t, write.Channels[ch])
		if before != after {
			t.Fatalf("channel %s changed on a no-op save:\n before=%s\n after =%s", ch, before, after)
		}
		if write.Changed[ch] {
			t.Fatalf("channel %s reported as changed on a no-op save", ch)
		}
	}
}

func TestSiteResolution(t *testing.T) {
	sites := BuildSites(fixture())
	if len(sites) != 2 {
		t.Fatalf("got %d sites, want 2: %+v", len(sites), sites)
	}

	byName := map[string]Site{}
	for _, site := range sites {
		byName[site.Name] = site
	}

	free, ok := byName["示例站点 / free"]
	if !ok {
		t.Fatalf("free site missing: %+v", sites)
	}
	// claude entry shares the host but has no /v1 path — it must attach by key.
	if !free.HasChannel(cpa.ChannelClaude) {
		t.Fatalf("claude entry did not attach to the free site by api key: %+v", free)
	}
	if free.HasChannel(cpa.ChannelCodex) {
		t.Fatalf("codex entry wrongly attached to the free site")
	}

	office := byName["示例站点 / office"]
	if !office.HasChannel(cpa.ChannelCodex) {
		t.Fatalf("codex entry did not attach to the office site by base-url: %+v", office)
	}
}

func TestRenameOnlyTouchesItsTargets(t *testing.T) {
	snap := fixture()
	cat := Reconcile(nil, snap)

	// Two sites both serve models; renaming one must not touch the other.
	ops := []Op{{
		Type:    OpRename,
		To:      "glm-4.5-air",
		Targets: []EntryRef{{Site: "示例站点 / free", Upstream: "[free]glm-4.5"}},
	}}
	if _, err := ApplyOps(cat, RefSet{}, RefSet{}, RefSet{}, ops); err != nil {
		t.Fatalf("ApplyOps: %v", err)
	}

	view := compute(t, cat, defaultSettings(), RefSet{}, RefSet{})
	for _, m := range view.Models {
		switch m.Upstream {
		case "[free]glm-4.5":
			if m.Alias != "glm-4.5-air" {
				t.Fatalf("target alias = %q, want glm-4.5-air", m.Alias)
			}
		case "qwen-max":
			if m.Alias != "qwen-max" {
				t.Fatalf("unrelated model was renamed to %q", m.Alias)
			}
		}
	}
}

func TestRenameToUpstreamNameClearsAlias(t *testing.T) {
	snap := fixture()
	cat := Reconcile(nil, snap)
	ops := []Op{{
		Type:    OpRename,
		To:      "qwen-max",
		Targets: []EntryRef{{Site: "示例站点 / office", Upstream: "qwen-max"}},
	}}
	if _, err := ApplyOps(cat, RefSet{}, RefSet{}, RefSet{}, ops); err != nil {
		t.Fatalf("ApplyOps: %v", err)
	}
	view := compute(t, cat, defaultSettings(), RefSet{}, RefSet{})
	write := BuildWrite(cat, view, snap, nil)

	encoded := payloadJSON(t, write.Channels[cpa.ChannelOpenAI])
	if contains(encoded, `"alias":"qwen-max"`) {
		t.Fatalf("redundant alias was written: %s", encoded)
	}
}

func TestWhitelistMatchesOriginalNameOnly(t *testing.T) {
	snap := fixture()
	cat := Reconcile(nil, snap)
	settings := defaultSettings()
	settings.Whitelist = `^\[free\]`

	view := compute(t, cat, settings, RefSet{}, RefSet{})
	for _, m := range view.Models {
		wantExcluded := m.Upstream[0] != '['
		if (m.Excluded == ExcludedWhitelist) != wantExcluded {
			t.Fatalf("%s: excluded=%q, want whitelist-excluded=%v (alias %q)", m.Upstream, m.Excluded, wantExcluded, m.Alias)
		}
	}
}

func TestExcludedModelsAreNotWrittenButSurviveInCatalog(t *testing.T) {
	snap := fixture()
	cat := Reconcile(nil, snap)
	settings := defaultSettings()
	settings.Whitelist = `^\[free\]`

	view := compute(t, cat, settings, RefSet{}, RefSet{})
	write := BuildWrite(cat, view, snap, nil)

	encoded := payloadJSON(t, write.Channels[cpa.ChannelOpenAI])
	if contains(encoded, "DeepSeek-V3") {
		t.Fatalf("filtered model still written: %s", encoded)
	}
	if !contains(encoded, "[free]glm-4.5") {
		t.Fatalf("whitelisted model missing: %s", encoded)
	}

	Prune(cat, view)
	found := false
	for _, entry := range cat.Entries {
		if entry.Upstream == "deepseek-ai/DeepSeek-V3" {
			found = true
		}
	}
	if !found {
		t.Fatal("excluded entry was pruned; relaxing the whitelist could not restore it")
	}
}

// A model that disappears from CPA without any filter explaining it was
// deleted elsewhere and must not be resurrected.
func TestPruneDropsUnexplainedAbsentEntries(t *testing.T) {
	snap := fixture()
	cat := Reconcile(nil, snap)
	view := compute(t, cat, defaultSettings(), RefSet{}, RefSet{})
	Prune(cat, view)

	// Drop a model from CPA and reconcile against the cache.
	openai := snap.Channels[cpa.ChannelOpenAI]
	openai[0].Models = openai[0].Models[:1]

	next := Reconcile(cat, snap)
	nextView := compute(t, next, defaultSettings(), RefSet{}, RefSet{})
	Prune(next, nextView)

	for _, entry := range next.Entries {
		if entry.Upstream == "[free]glm-4.5" {
			t.Fatal("externally deleted model was kept and would be re-added on save")
		}
	}
}

func TestManualExclusionKeepsEntryAndRestoreBringsItBack(t *testing.T) {
	snap := fixture()
	cat := Reconcile(nil, snap)
	ref := EntryRef{Site: "示例站点 / office", Upstream: "qwen-max"}

	result, err := ApplyOps(cat, RefSet{}, RefSet{}, RefSet{}, []Op{{Type: OpExclude, Targets: []EntryRef{ref}}})
	if err != nil {
		t.Fatalf("ApplyOps: %v", err)
	}
	view := compute(t, cat, defaultSettings(), result.Exclusions, RefSet{})
	write := BuildWrite(cat, view, snap, nil)
	if contains(payloadJSON(t, write.Channels[cpa.ChannelOpenAI]), "qwen-max") {
		t.Fatal("manually excluded model was still written to CPA")
	}
	Prune(cat, view)

	restored, err := ApplyOps(cat, result.Exclusions, RefSet{}, RefSet{}, []Op{{Type: OpInclude, Targets: []EntryRef{ref}}})
	if err != nil {
		t.Fatalf("ApplyOps restore: %v", err)
	}
	view = compute(t, cat, defaultSettings(), restored.Exclusions, RefSet{})
	write = BuildWrite(cat, view, snap, nil)
	if !contains(payloadJSON(t, write.Channels[cpa.ChannelOpenAI]), "qwen-max") {
		t.Fatal("restored model did not come back")
	}
}

// Disabling at one site must not touch the same model at another site, and
// must not remove it from the catalog.
func TestSiteDisableIsScopedAndReversible(t *testing.T) {
	snap := fixture()
	cat := Reconcile(nil, snap)
	ref := EntryRef{Site: "示例站点 / office", Upstream: "openai/gpt-5.4"}

	result, err := ApplyOps(cat, RefSet{}, RefSet{}, RefSet{}, []Op{{Type: OpSetDisabled, Disabled: true, Targets: []EntryRef{ref}}})
	if err != nil {
		t.Fatalf("ApplyOps: %v", err)
	}
	view := compute(t, cat, defaultSettings(), RefSet{}, result.Disabled)
	write := BuildWrite(cat, view, snap, nil)

	if contains(payloadJSON(t, write.Channels[cpa.ChannelCodex]), "gpt-5.4") {
		t.Fatal("disabled model was still written")
	}
	if !contains(payloadJSON(t, write.Channels[cpa.ChannelCodex]), "claude-opus-4.5") {
		t.Fatal("sibling model at the same provider was dropped")
	}
}

func TestDiscoveredModelsLandInTheProtocolChannel(t *testing.T) {
	snap := fixture()
	cat := Reconcile(nil, snap)
	matcher, err := clean.NewProtocolMatcher(clean.DefaultProtocolConfig())
	if err != nil {
		t.Fatalf("matcher: %v", err)
	}

	rules := clean.NewRules(clean.DefaultPrefixes, clean.DefaultSuffixes)
	added := MergeDiscovered(cat, "示例站点 / office", []string{"gpt-6", "claude-opus-9", "llama-4"}, matcher, rules)
	if added != 3 {
		t.Fatalf("added = %d, want 3", added)
	}

	want := map[string]cpa.Channel{
		"gpt-6":         cpa.ChannelCodex,  // site has a codex entry
		"claude-opus-9": cpa.ChannelOpenAI, // site has no claude entry → fallback
		"llama-4":       cpa.ChannelOpenAI,
	}
	for _, entry := range cat.Entries {
		expected, ok := want[entry.Upstream]
		if !ok {
			continue
		}
		if len(entry.Occurrences) != 1 || entry.Occurrences[0].Channel != expected {
			t.Fatalf("%s landed in %+v, want %s", entry.Upstream, entry.Occurrences, expected)
		}
		if !entry.Pending {
			t.Fatalf("%s should be pending", entry.Upstream)
		}
	}

	view := compute(t, cat, defaultSettings(), RefSet{}, RefSet{})
	write := BuildWrite(cat, view, snap, nil)
	if !contains(payloadJSON(t, write.Channels[cpa.ChannelCodex]), "gpt-6") {
		t.Fatal("pending codex model was not written")
	}
	if !contains(payloadJSON(t, write.Channels[cpa.ChannelOpenAI]), "llama-4") {
		t.Fatal("pending openai model was not written")
	}
}

func TestPriorityIsWrittenBack(t *testing.T) {
	snap := fixture()
	cat := Reconcile(nil, snap)
	result, err := ApplyOps(cat, RefSet{}, RefSet{}, RefSet{}, []Op{{Type: OpSetPriority, Site: "示例站点 / free", Priority: 7}})
	if err != nil {
		t.Fatalf("ApplyOps: %v", err)
	}
	view := compute(t, cat, defaultSettings(), RefSet{}, RefSet{})
	write := BuildWrite(cat, view, snap, result.Priorities)

	if !contains(payloadJSON(t, write.Channels[cpa.ChannelOpenAI]), `"priority":7`) {
		t.Fatalf("priority was not written: %s", payloadJSON(t, write.Channels[cpa.ChannelOpenAI]))
	}
	if !write.Changed[cpa.ChannelOpenAI] {
		t.Fatal("priority change was not detected")
	}
}

func TestFingerprintTracksExternalEdits(t *testing.T) {
	snap := fixture()
	first, err := Fingerprint(snap)
	if err != nil {
		t.Fatalf("Fingerprint: %v", err)
	}
	again, _ := Fingerprint(fixture())
	if first != again {
		t.Fatal("fingerprint is not stable across identical snapshots")
	}

	snap.Channels[cpa.ChannelClaude][0].Models[0].Alias = "changed"
	changed, _ := Fingerprint(snap)
	if changed == first {
		t.Fatal("fingerprint did not change after an external edit")
	}
}

func TestUnknownProvidersPassThroughUntouched(t *testing.T) {
	snap := fixture()
	cat := Reconcile(nil, snap)
	view := compute(t, cat, defaultSettings(), RefSet{}, RefSet{})

	// A provider CPA gained after the catalog was built.
	extra, err := cpa.ProvidersFromPayload([]map[string]any{{
		"name":     "brand new",
		"base-url": "https://new.example.com/v1",
		"models":   []any{map[string]any{"name": "mystery-1"}},
	}})
	if err != nil {
		t.Fatalf("payload: %v", err)
	}
	snap.Channels[cpa.ChannelOpenAI] = append(snap.Channels[cpa.ChannelOpenAI], extra...)

	write := BuildWrite(cat, view, snap, nil)
	if !contains(payloadJSON(t, write.Channels[cpa.ChannelOpenAI]), "mystery-1") {
		t.Fatal("unknown provider's models were wiped")
	}
}

func contains(haystack, needle string) bool {
	return len(haystack) >= len(needle) && indexOf(haystack, needle) >= 0
}

func indexOf(haystack, needle string) int {
	for i := 0; i+len(needle) <= len(haystack); i++ {
		if haystack[i:i+len(needle)] == needle {
			return i
		}
	}
	return -1
}

// Protocol matching runs on the cleaned name, so a vendor prefix or a `[free]`
// marker does not have to be spelled out in every protocol regex.
func TestProtocolMatchesTheCleanedName(t *testing.T) {
	snap := fixture()
	cat := Reconcile(nil, snap)

	settings := defaultSettings()
	// Anchored on the clean form only — it would never match the raw
	// "anthropic/claude-opus-4.5" or "[free]claude-sonnet-5".
	settings.Protocol = clean.ProtocolConfig{
		CodexRegex:  `^gpt-`,
		ClaudeRegex: `^claude-`,
	}

	view := compute(t, cat, settings, RefSet{}, RefSet{})
	want := map[string]string{
		"openai/gpt-5.4":            clean.ProtocolCodex,
		"anthropic/claude-opus-4.5": clean.ProtocolClaude,
		"[free]claude-sonnet-5":     clean.ProtocolClaude,
		"qwen-max":                  clean.ProtocolOpenAI,
	}
	for _, model := range view.Models {
		expected, ok := want[model.Upstream]
		if !ok {
			continue
		}
		if model.Protocol != expected {
			t.Errorf("%s (canonical %q) tagged %q, want %q", model.Upstream, model.Canonical, model.Protocol, expected)
		}
	}
}
