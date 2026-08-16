package api

import (
	"encoding/json"
	"net/http"
	"testing"
	"time"

	"github.com/local/cpa-model-panel/internal/catalog"
	"github.com/local/cpa-model-panel/internal/store"
)

func TestAutoSyncConfigEndpointPersistsAndValidates(t *testing.T) {
	fake := newFakeCPA(t)
	server := newTestServer(t, fake)

	res := call(t, server, http.MethodPut, "/api/auto-sync", `{
		"enabled":true,"interval_minutes":90,"jitter_minutes":15
	}`)
	if res.Code != http.StatusOK {
		t.Fatalf("PUT auto sync = %d: %s", res.Code, res.Body.String())
	}

	res = call(t, server, http.MethodGet, "/api/auto-sync", "")
	if res.Code != http.StatusOK {
		t.Fatalf("GET auto sync = %d: %s", res.Code, res.Body.String())
	}
	var payload autoSyncPayload
	if err := json.Unmarshal(res.Body.Bytes(), &payload); err != nil {
		t.Fatal(err)
	}
	if !payload.Config.Enabled || payload.Config.IntervalMinutes != 90 || payload.Config.JitterMinutes != 15 {
		t.Fatalf("config = %+v", payload.Config)
	}

	res = call(t, server, http.MethodPut, "/api/auto-sync", `{
		"enabled":true,"interval_minutes":10,"jitter_minutes":10
	}`)
	if res.Code != http.StatusBadRequest {
		t.Fatalf("invalid jitter = %d, want 400: %s", res.Code, res.Body.String())
	}
}

func TestAutoSyncRefreshesAppliesEverySuggestionAndWritesLog(t *testing.T) {
	fake := newFakeCPA(t)
	server := newTestServer(t, fake)
	fake.mu.Lock()
	fake.lists["openai-compatibility"] = append(fake.lists["openai-compatibility"], map[string]any{
		"name": "invisible-keyless", "base-url": "https://keyless.example.com/v1", "models": []any{map[string]any{"name": "keep-me"}},
	})
	fake.mu.Unlock()

	// Seed the cache exactly as a normal catalog read does.
	if res := call(t, server, http.MethodGet, "/api/catalog", ""); res.Code != http.StatusOK {
		t.Fatalf("catalog = %d: %s", res.Code, res.Body.String())
	}
	beforeDisabled, err := server.Store.Disabled()
	if err != nil {
		t.Fatal(err)
	}
	beforeDisabled[catalog.EntryRef{Site: "site-a", Upstream: "old-model"}] = true
	if err := server.Store.SetDisabled(beforeDisabled); err != nil {
		t.Fatal(err)
	}

	entry := server.runAutoSyncOnce()
	if entry.Status != "success" || entry.Error != "" {
		t.Fatalf("auto sync = %+v", entry)
	}
	if entry.Refreshed != 1 || entry.Added != 1 || entry.Suggested != 1 || entry.Renamed != 1 {
		t.Fatalf("unexpected summary: %+v", entry)
	}
	if entry.Snapshot == 0 {
		t.Fatalf("automatic write did not record its snapshot: %+v", entry)
	}
	if entry.ID == 0 {
		t.Fatal("run log was not persisted")
	}

	fake.mu.Lock()
	var alias any
	for _, provider := range fake.lists["openai-compatibility"] {
		models, _ := provider["models"].([]any)
		for _, raw := range models {
			model := raw.(map[string]any)
			if model["name"] == "vendor/brand-new-model" {
				alias = model["alias"]
			}
		}
	}
	fake.mu.Unlock()
	if alias != "brand-new-model" {
		t.Fatalf("new model alias = %#v, want brand-new-model", alias)
	}
	fake.mu.Lock()
	keylessKept := false
	for _, provider := range fake.lists["openai-compatibility"] {
		if provider["name"] == "invisible-keyless" {
			keylessKept = true
		}
	}
	fake.mu.Unlock()
	if !keylessKept {
		t.Fatal("automatic refresh swept a keyless provider")
	}

	afterDisabled, err := server.Store.Disabled()
	if err != nil {
		t.Fatal(err)
	}
	if len(beforeDisabled) != len(afterDisabled) {
		t.Fatalf("auto sync changed site-disabled state: before=%v after=%v", beforeDisabled, afterDisabled)
	}
	for ref := range beforeDisabled {
		if !afterDisabled[ref] {
			t.Fatalf("auto sync removed site-disabled state for %v", ref)
		}
	}
	logs, err := server.Store.ListAutoSyncLogs(20)
	if err != nil {
		t.Fatal(err)
	}
	if len(logs) != 1 || logs[0].ID != entry.ID || logs[0].Status != "success" {
		t.Fatalf("logs = %+v", logs)
	}
}

func TestSuggestionOpsUsePreciseTargets(t *testing.T) {
	view := catalog.View{Models: []catalog.ModelView{
		{Site: "a", Upstream: "vendor/one", Suggested: "one", Disabled: true},
		{Site: "b", Upstream: "vendor/one", Suggested: "one", Excluded: catalog.ExcludedWhitelist},
		{Site: "c", Upstream: "already-clean"},
	}}
	ops, count := suggestionOps(view)
	if count != 2 || len(ops) != 1 {
		t.Fatalf("count=%d ops=%+v", count, ops)
	}
	if ops[0].Type != catalog.OpRename || ops[0].To != "one" || len(ops[0].Targets) != 2 {
		t.Fatalf("op = %+v", ops[0])
	}
}

func TestRandomizedAutoSyncDelayStaysWithinConfiguredError(t *testing.T) {
	cfg := store.AutoSyncConfig{IntervalMinutes: 60, JitterMinutes: 7}
	minimum := 53 * time.Minute
	maximum := 67 * time.Minute
	for i := 0; i < 100; i++ {
		delay := randomizedAutoSyncDelay(cfg)
		if delay < minimum || delay > maximum {
			t.Fatalf("delay %s outside [%s, %s]", delay, minimum, maximum)
		}
	}
	if got := randomizedAutoSyncDelay(store.AutoSyncConfig{IntervalMinutes: 12}); got != 12*time.Minute {
		t.Fatalf("zero-jitter delay = %s", got)
	}
}
