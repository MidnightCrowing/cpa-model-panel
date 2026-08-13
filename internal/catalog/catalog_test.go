package catalog

import (
	"encoding/json"
	"sort"
	"strings"
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

	rules, err := clean.NewRules(clean.RulesConfig{})
	if err != nil {
		t.Fatalf("NewRules: %v", err)
	}
	// qwen-max is what the site already serves; listing it keeps this test
	// about where *new* models land.
	added := MergeDiscovered(cat, "示例站点 / office", []string{"qwen-max", "gpt-6", "claude-opus-9", "llama-4"}, matcher, rules)
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

// Each channel carries its own priority, so an edit names the channel it
// applies to and leaves the site's other channels alone.
func TestPriorityIsWrittenBack(t *testing.T) {
	snap := fixture()
	cat := Reconcile(nil, snap)
	result, err := ApplyOps(cat, RefSet{}, RefSet{}, RefSet{}, []Op{{
		Type:     OpSetPriority,
		Site:     "示例站点 / free",
		Channel:  string(cpa.ChannelOpenAI),
		Priority: 7,
	}})
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
	// The codex list must not inherit the openai edit.
	if contains(payloadJSON(t, write.Channels[cpa.ChannelCodex]), `"priority":7`) {
		t.Fatal("openai priority leaked into codex-api-key")
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

// With routing on, each CPA list holds exactly the models whose protocol names
// it — no more, no less. The fixture starts misfiled on purpose: the codex
// entry carries a claude model, which is the shape the live configuration had.
func TestRoutingProjectsEachListExactly(t *testing.T) {
	snap := fixture()
	cat := Reconcile(nil, snap)
	settings := defaultSettings()
	settings.RouteByProtocol = true

	view := compute(t, cat, settings, RefSet{}, RefSet{})
	write := BuildWrite(cat, view, snap, nil)

	got := map[cpa.Channel][]string{}
	for _, ch := range cpa.AllChannels {
		for _, provider := range write.Channels[ch] {
			for _, model := range provider.Models {
				got[ch] = append(got[ch], model.Name)
			}
		}
		sort.Strings(got[ch])
	}

	want := map[cpa.Channel][]string{
		cpa.ChannelOpenAI: {"[free]glm-4.5", "deepseek-ai/DeepSeek-V3", "qwen-max"},
		cpa.ChannelCodex:  {"openai/gpt-5.4"},
		cpa.ChannelClaude: {"[free]claude-sonnet-5", "anthropic/claude-opus-4.5"},
	}
	for _, ch := range cpa.AllChannels {
		if strings.Join(got[ch], ",") != strings.Join(want[ch], ",") {
			t.Errorf("%s holds %v, want %v", ch, got[ch], want[ch])
		}
	}
}

// A model routed to a channel the site has no entry in must get one created
// from the site's own credentials — dropping it is what the original
// implementation did.
func TestRoutingCreatesAMissingProviderInsteadOfDropping(t *testing.T) {
	snap := fixture()
	cat := Reconcile(nil, snap)
	settings := defaultSettings()
	settings.RouteByProtocol = true

	// The "free" site has openai and claude entries but no codex one.
	matcher, err := clean.NewProtocolMatcher(clean.DefaultProtocolConfig())
	if err != nil {
		t.Fatalf("matcher: %v", err)
	}
	rules, err := clean.NewRules(clean.RulesConfig{})
	if err != nil {
		t.Fatalf("rules: %v", err)
	}
	MergeDiscovered(cat, "示例站点 / free", []string{"gpt-9-turbo"}, matcher, rules)

	view := compute(t, cat, settings, RefSet{}, RefSet{})
	write := BuildWrite(cat, view, snap, nil)

	found := false
	for _, provider := range write.Channels[cpa.ChannelCodex] {
		for _, model := range provider.Models {
			if model.Name == "gpt-9-turbo" {
				found = true
				if provider.BaseURL != "https://ai.example.com/v1" {
					t.Errorf("created provider base-url = %q", provider.BaseURL)
				}
				if len(provider.APIKeys()) == 0 || provider.APIKeys()[0] != "key-free" {
					t.Errorf("created provider did not inherit the site key: %#v", provider.APIKeyEntries)
				}
			}
		}
	}
	if !found {
		t.Fatal("model routed to a channel the site lacks was dropped")
	}
	if len(write.Created) == 0 {
		t.Error("provider creation was not reported")
	}
}

// claude-api-key addresses the bare origin while the other two take /v1.
func TestCreatedProviderUsesTheChannelsUrlShape(t *testing.T) {
	cases := map[cpa.Channel]string{
		cpa.ChannelOpenAI: "https://x.example.com/v1",
		cpa.ChannelCodex:  "https://x.example.com/v1",
		cpa.ChannelClaude: "https://x.example.com",
	}
	for ch, want := range cases {
		if got := channelBaseURL("https://x.example.com/v1", ch); got != want {
			t.Errorf("channelBaseURL(/v1, %s) = %q, want %q", ch, got, want)
		}
		if got := channelBaseURL("https://x.example.com", ch); got != want {
			t.Errorf("channelBaseURL(bare, %s) = %q, want %q", ch, got, want)
		}
	}
}

// Routing off keeps the byte-for-byte guarantee.
func TestRoutingOffStillReproducesCpaExactly(t *testing.T) {
	snap := fixture()
	cat := Reconcile(nil, snap)
	view := compute(t, cat, defaultSettings(), RefSet{}, RefSet{})
	write := BuildWrite(cat, view, snap, nil)
	for _, ch := range cpa.AllChannels {
		if write.Changed[ch] {
			t.Errorf("channel %s changed with routing off", ch)
		}
	}
}

// Each channel of a site stores its own priority independently.
func TestSitePriorityComesFromTheOpenAiEntry(t *testing.T) {
	snap := fixture()
	// The codex entry carries a priority; the openai one does not.
	snap.Channels[cpa.ChannelCodex][0].Priority = 7
	snap.Channels[cpa.ChannelCodex][0].Raw["priority"] = float64(7)

	for _, site := range BuildSites(snap) {
		if site.Name == "示例站点 / office" {
			if site.Priorities[cpa.ChannelOpenAI] != 0 {
				t.Fatalf("openai priority = %d, want 0", site.Priorities[cpa.ChannelOpenAI])
			}
			if site.Priorities[cpa.ChannelCodex] != 7 {
				t.Fatalf("codex priority = %d, want 7", site.Priorities[cpa.ChannelCodex])
			}
		}
	}
}

// A site configured only in codex-api-key has no openai entry to read, so its
// own priority is the only answer.
func TestCodexOnlySiteKeepsItsOwnPriority(t *testing.T) {
	providers, err := cpa.ProvidersFromPayload([]map[string]any{{
		"base-url": "https://codex-only.example.com/v1",
		"api-key":  "k",
		"priority": float64(9),
		"models":   []any{map[string]any{"name": "gpt-5.4"}},
	}})
	if err != nil {
		t.Fatalf("payload: %v", err)
	}
	snap := &cpa.Snapshot{Channels: map[cpa.Channel][]cpa.Provider{cpa.ChannelCodex: providers}}

	sites := BuildSites(snap)
	if len(sites) != 1 {
		t.Fatalf("sites = %d, want 1", len(sites))
	}
	if sites[0].Priorities[cpa.ChannelCodex] != 9 {
		t.Fatalf("priority = %d, want 9", sites[0].Priorities[cpa.ChannelCodex])
	}
}

// One host commonly serves several groups, each with its own api-key. Matching
// on the base-url alone merged them into one column, which put two different
// accounts behind a single toggle.
func TestSameURLDifferentKeyIsADifferentSite(t *testing.T) {
	snap := fixture()
	extra, err := cpa.ProvidersFromPayload([]map[string]any{{
		// The free site's exact url, but another group's key.
		"base-url": "https://ai.example.com/v1",
		"api-key":  "key-other-group",
		"models":   []any{map[string]any{"name": "gpt-5.4"}},
	}})
	if err != nil {
		t.Fatalf("payload: %v", err)
	}
	snap.Channels[cpa.ChannelCodex] = append(snap.Channels[cpa.ChannelCodex], extra...)

	sites := BuildSites(snap)
	for _, site := range sites {
		if site.Name == "示例站点 / free" && site.APIKey == "key-free" {
			if idx, ok := site.Providers[cpa.ChannelCodex]; ok && idx == 1 {
				t.Fatal("another group's codex entry was merged into 示例站点 / free")
			}
		}
	}
	if len(sites) != 3 {
		t.Fatalf("sites = %d, want 3 (the extra group is its own)", len(sites))
	}
}

// A shared key is the one thing that does prove two entries are one account:
// the claude entry sits on a different url shape and still belongs to free.
func TestSharedKeyMergesAcrossChannels(t *testing.T) {
	for _, site := range BuildSites(fixture()) {
		if site.Name != "示例站点 / free" {
			continue
		}
		if !site.HasChannel(cpa.ChannelClaude) {
			t.Fatal("the claude entry sharing key-free was not attached")
		}
	}
}

// "<站点> / <分组>" is how the sites are named in CPA; the matrix shows the
// site large and the group as a subtitle instead of repeating the whole string.
func TestNameSplitsIntoSiteAndGroup(t *testing.T) {
	found := false
	for _, site := range BuildSites(fixture()) {
		if site.Name != "示例站点 / free" {
			continue
		}
		found = true
		if site.Label != "示例站点" || site.Group != "free" {
			t.Fatalf("label/group = %q / %q, want 示例站点 / free", site.Label, site.Group)
		}
	}
	if !found {
		t.Fatal("示例站点 / free is missing")
	}
}

func discoveryTools(t *testing.T) (*clean.ProtocolMatcher, clean.Rules) {
	t.Helper()
	matcher, err := clean.NewProtocolMatcher(clean.DefaultProtocolConfig())
	if err != nil {
		t.Fatalf("matcher: %v", err)
	}
	rules, err := clean.NewRules(clean.RulesConfig{})
	if err != nil {
		t.Fatalf("NewRules: %v", err)
	}
	return matcher, rules
}

func modelIn(view View, site, upstream string) (ModelView, bool) {
	for _, m := range view.Models {
		if m.Site == site && m.Upstream == upstream {
			return m, true
		}
	}
	return ModelView{}, false
}

// A model the site stopped serving has to leave CPA too: CPA would keep
// routing to it and take a 404 from the upstream.
func TestModelGoneUpstreamLeavesCPA(t *testing.T) {
	snap := fixture()
	cat := Reconcile(nil, snap)
	matcher, rules := discoveryTools(t)

	// office serves qwen-max today; this probe no longer lists it.
	MergeDiscovered(cat, "示例站点 / office", []string{"gpt-6"}, matcher, rules)

	view := compute(t, cat, defaultSettings(), RefSet{}, RefSet{})
	model, ok := modelIn(view, "示例站点 / office", "qwen-max")
	if !ok {
		t.Fatal("qwen-max vanished from the view instead of being marked")
	}
	if model.Excluded != ExcludedGone {
		t.Fatalf("excluded = %q, want %q", model.Excluded, ExcludedGone)
	}
	if view.Stats.ToRemove == 0 {
		t.Fatal("the save should be reported as having something to remove")
	}

	write := BuildWrite(cat, view, snap, nil)
	if contains(payloadJSON(t, write.Channels[cpa.ChannelOpenAI]), "qwen-max") {
		t.Fatal("a model the site no longer serves was written back to CPA")
	}
}

// Marking is reversible, like every other filter: the catalog copy survives a
// prune, so a model the site serves again comes straight back.
func TestGoneModelComesBackWhenServedAgain(t *testing.T) {
	cat := Reconcile(nil, fixture())
	matcher, rules := discoveryTools(t)

	MergeDiscovered(cat, "示例站点 / office", []string{"gpt-6"}, matcher, rules)
	Prune(cat, compute(t, cat, defaultSettings(), RefSet{}, RefSet{}))

	MergeDiscovered(cat, "示例站点 / office", []string{"gpt-6", "qwen-max"}, matcher, rules)
	model, ok := modelIn(compute(t, cat, defaultSettings(), RefSet{}, RefSet{}), "示例站点 / office", "qwen-max")
	if !ok {
		t.Fatal("qwen-max was pruned away and could not come back")
	}
	if model.Excluded != "" {
		t.Fatalf("still excluded as %q after the site served it again", model.Excluded)
	}
}

// An empty answer is far more likely to be a broken endpoint than a site that
// genuinely serves nothing, so it must not empty the site.
func TestEmptyDiscoveryMarksNothingGone(t *testing.T) {
	cat := Reconcile(nil, fixture())
	matcher, rules := discoveryTools(t)

	MergeDiscovered(cat, "示例站点 / office", nil, matcher, rules)

	model, ok := modelIn(compute(t, cat, defaultSettings(), RefSet{}, RefSet{}), "示例站点 / office", "qwen-max")
	if !ok {
		t.Fatal("qwen-max disappeared")
	}
	if model.Excluded != "" {
		t.Fatalf("an empty probe excluded it as %q", model.Excluded)
	}
}

// One site's probe says nothing about any other site's models.
func TestDiscoveryOnlyTouchesItsOwnSite(t *testing.T) {
	cat := Reconcile(nil, fixture())
	matcher, rules := discoveryTools(t)

	MergeDiscovered(cat, "示例站点 / office", []string{"gpt-6"}, matcher, rules)

	view := compute(t, cat, defaultSettings(), RefSet{}, RefSet{})
	for _, upstream := range []string{"deepseek-ai/DeepSeek-V3", "[free]glm-4.5"} {
		model, ok := modelIn(view, "示例站点 / free", upstream)
		if !ok {
			t.Fatalf("%s disappeared", upstream)
		}
		if model.Excluded == ExcludedGone {
			t.Fatalf("%s belongs to another site and was marked gone", upstream)
		}
	}
}

// An explicit keep outranks it, same as it does the whitelist and version
// rules — the site's list is not always the whole truth.
func TestKeepOverridesGone(t *testing.T) {
	cat := Reconcile(nil, fixture())
	matcher, rules := discoveryTools(t)

	MergeDiscovered(cat, "示例站点 / office", []string{"gpt-6"}, matcher, rules)

	keeps := RefSet{{Site: "示例站点 / office", Upstream: "qwen-max"}: true}
	view, err := Compute(Inputs{Catalog: cat, Settings: defaultSettings(), Keeps: keeps})
	if err != nil {
		t.Fatalf("Compute: %v", err)
	}
	model, ok := modelIn(view, "示例站点 / office", "qwen-max")
	if !ok {
		t.Fatal("qwen-max disappeared")
	}
	if model.Excluded != "" {
		t.Fatalf("kept model was still excluded as %q", model.Excluded)
	}
}
