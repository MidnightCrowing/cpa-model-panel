package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/local/cpa-model-panel/internal/catalog"
	"github.com/local/cpa-model-panel/internal/clean"
	"github.com/local/cpa-model-panel/internal/cpa"
	"github.com/local/cpa-model-panel/internal/store"
)

// fakeCPA is a management API stand-in that records every write.
type fakeCPA struct {
	mu      sync.Mutex
	lists   map[string][]map[string]any
	puts    []string
	server  *httptest.Server
	putBody map[string][]map[string]any
}

func newFakeCPA(t *testing.T) *fakeCPA {
	t.Helper()
	fake := &fakeCPA{
		lists: map[string][]map[string]any{
			"openai-compatibility": {{
				"name":            "site-a",
				"base-url":        "https://a.example.com/v1",
				"api-key-entries": []any{map[string]any{"api-key": "key-a"}},
				"models": []any{
					map[string]any{"name": "deepseek-ai/DeepSeek-V3", "alias": "deepseek-v3"},
					map[string]any{"name": "old-model"},
				},
			}},
			"codex-api-key": {{
				"base-url":   "https://a.example.com/v1",
				"api-key":    "key-a",
				"auth-index": float64(4),
				"models": []any{
					map[string]any{"name": "openai/gpt-5.4", "alias": "gpt-5.4"},
				},
			}},
			"claude-api-key": {},
		},
		putBody: map[string][]map[string]any{},
	}

	fake.server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		name := strings.TrimPrefix(r.URL.Path, "/v0/management/")
		fake.mu.Lock()
		defer fake.mu.Unlock()
		switch r.Method {
		case http.MethodGet:
			_ = json.NewEncoder(w).Encode(map[string]any{name: fake.lists[name]})
		case http.MethodPut:
			var body []map[string]any
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				t.Errorf("decode PUT %s: %v", name, err)
			}
			fake.lists[name] = body
			fake.putBody[name] = body
			fake.puts = append(fake.puts, name)
			w.WriteHeader(http.StatusOK)
		default:
			w.WriteHeader(http.StatusMethodNotAllowed)
		}
	}))
	t.Cleanup(fake.server.Close)
	return fake
}

func (f *fakeCPA) writes() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]string(nil), f.puts...)
}

func newTestServer(t *testing.T, fake *fakeCPA) *Server {
	t.Helper()
	st, err := store.Open(t.TempDir())
	if err != nil {
		t.Fatalf("store.Open: %v", err)
	}
	t.Cleanup(func() { _ = st.Close() })
	return &Server{
		AdminToken: "token",
		CPA:        cpa.NewClient(fake.server.URL, "secret"),
		Store:      st,
		Retain:     5,
	}
}

func call(t *testing.T, server *Server, method, path, body string) *httptest.ResponseRecorder {
	t.Helper()
	mux := http.NewServeMux()
	server.Routes(mux)
	var reader *strings.Reader
	if body == "" {
		reader = strings.NewReader("")
	} else {
		reader = strings.NewReader(body)
	}
	req := httptest.NewRequest(method, path, reader)
	req.Header.Set("Authorization", "Bearer token")
	res := httptest.NewRecorder()
	mux.ServeHTTP(res, req)
	return res
}

func decodeView(t *testing.T, body string) catalog.View {
	t.Helper()
	var view catalog.View
	if err := json.Unmarshal([]byte(body), &view); err != nil {
		t.Fatalf("decode view: %v (%s)", err, body)
	}
	return view
}

func TestCatalogRequiresAuth(t *testing.T) {
	fake := newFakeCPA(t)
	server := newTestServer(t, fake)
	mux := http.NewServeMux()
	server.Routes(mux)

	req := httptest.NewRequest(http.MethodGet, "/api/catalog", nil)
	res := httptest.NewRecorder()
	mux.ServeHTTP(res, req)
	if res.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401", res.Code)
	}
}

func TestCatalogReportsEveryChannel(t *testing.T) {
	fake := newFakeCPA(t)
	server := newTestServer(t, fake)

	res := call(t, server, http.MethodGet, "/api/catalog", "")
	if res.Code != http.StatusOK {
		t.Fatalf("status = %d: %s", res.Code, res.Body.String())
	}
	view := decodeView(t, res.Body.String())

	if len(view.Sites) != 1 {
		t.Fatalf("sites = %d, want 1 (codex entry must attach to the openai site)", len(view.Sites))
	}
	if view.Stats.Models != 3 {
		t.Fatalf("models = %d, want 3", view.Stats.Models)
	}
	if len(fake.writes()) != 0 {
		t.Fatalf("reading the catalog wrote to CPA: %v", fake.writes())
	}
}

// The regression that matters most: an empty draft must not touch CPA.
func TestSaveWithoutChangesWritesNothing(t *testing.T) {
	fake := newFakeCPA(t)
	server := newTestServer(t, fake)

	view := decodeView(t, call(t, server, http.MethodGet, "/api/catalog", "").Body.String())
	body := `{"fingerprint":"` + view.Fingerprint + `","ops":[]}`
	res := call(t, server, http.MethodPost, "/api/save", body)
	if res.Code != http.StatusOK {
		t.Fatalf("status = %d: %s", res.Code, res.Body.String())
	}
	if writes := fake.writes(); len(writes) != 0 {
		t.Fatalf("no-op save wrote %v", writes)
	}
}

func TestSaveOnlyWritesTheAffectedChannel(t *testing.T) {
	fake := newFakeCPA(t)
	server := newTestServer(t, fake)

	view := decodeView(t, call(t, server, http.MethodGet, "/api/catalog", "").Body.String())
	body := `{"fingerprint":"` + view.Fingerprint + `","ops":[
		{"type":"rename","to":"gpt-5.4-turbo","targets":[{"site":"site-a","upstream":"openai/gpt-5.4"}]}
	]}`
	res := call(t, server, http.MethodPost, "/api/save", body)
	if res.Code != http.StatusOK {
		t.Fatalf("status = %d: %s", res.Code, res.Body.String())
	}

	writes := fake.writes()
	if len(writes) != 1 || writes[0] != "codex-api-key" {
		t.Fatalf("writes = %v, want only codex-api-key", writes)
	}

	fake.mu.Lock()
	defer fake.mu.Unlock()
	models := fake.putBody["codex-api-key"][0]["models"].([]any)
	first := models[0].(map[string]any)
	if first["alias"] != "gpt-5.4-turbo" {
		t.Fatalf("alias = %v, want gpt-5.4-turbo", first["alias"])
	}
	if fake.putBody["codex-api-key"][0]["auth-index"] != float64(4) {
		t.Fatalf("auth-index was lost: %#v", fake.putBody["codex-api-key"][0])
	}
}

func TestSaveRejectsStaleFingerprint(t *testing.T) {
	fake := newFakeCPA(t)
	server := newTestServer(t, fake)

	body := `{"fingerprint":"stale","ops":[{"type":"exclude","targets":[{"site":"site-a","upstream":"old-model"}]}]}`
	res := call(t, server, http.MethodPost, "/api/save", body)
	if res.Code != http.StatusConflict {
		t.Fatalf("status = %d, want 409", res.Code)
	}
	if len(fake.writes()) != 0 {
		t.Fatalf("a rejected save still wrote: %v", fake.writes())
	}
}

// A settings change must never reach CPA, and an invalid regex must be
// rejected instead of silently ignored.
func TestSettingsAreLocalAndValidated(t *testing.T) {
	fake := newFakeCPA(t)
	server := newTestServer(t, fake)
	_ = call(t, server, http.MethodGet, "/api/catalog", "")

	ok := call(t, server, http.MethodPut, "/api/settings", `{"prefixes":["deepseek-ai/"],"suffixes":[],"whitelist":"^deepseek","version":{"enabled":false},"protocol":{"codex_regex":"(?i)gpt","claude_regex":"(?i)claude"}}`)
	if ok.Code != http.StatusOK {
		t.Fatalf("status = %d: %s", ok.Code, ok.Body.String())
	}
	var payload struct {
		View catalog.View `json:"view"`
	}
	if err := json.Unmarshal(ok.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if payload.View.Stats.Excluded != 2 {
		t.Fatalf("excluded = %d, want 2 (whitelist keeps only deepseek)", payload.View.Stats.Excluded)
	}
	if len(fake.writes()) != 0 {
		t.Fatalf("changing settings wrote to CPA: %v", fake.writes())
	}

	bad := call(t, server, http.MethodPut, "/api/settings", `{"whitelist":"(","protocol":{"codex_regex":"(?i)gpt","claude_regex":"(?i)claude"}}`)
	if bad.Code != http.StatusBadRequest {
		t.Fatalf("invalid regex status = %d, want 400", bad.Code)
	}
}

// A stored pattern that no longer compiles must surface as an error. The old
// panel silently fell back to its built-in default, so a broken regex looked
// like it was working for weeks.
func TestInvalidStoredProtocolRegexIsReported(t *testing.T) {
	fake := newFakeCPA(t)
	server := newTestServer(t, fake)
	if err := server.Store.SetProtocolRegex(clean.ProtocolConfig{
		CodexRegex:  `(?i)^(?!.*mini).*gpt.*`,
		ClaudeRegex: `(?i)claude`,
	}); err != nil {
		t.Fatalf("SetProtocolRegex: %v", err)
	}

	res := call(t, server, http.MethodGet, "/api/catalog", "")
	if res.Code == http.StatusOK {
		t.Fatal("a pattern RE2 cannot compile must not be silently ignored")
	}
	if !strings.Contains(res.Body.String(), "协议标记正则无效") {
		t.Fatalf("error should name the offending setting: %s", res.Body.String())
	}
}

// Clearing a pattern means "tag nothing", not "restore the built-in default".
func TestEmptyProtocolRegexTagsNothing(t *testing.T) {
	fake := newFakeCPA(t)
	server := newTestServer(t, fake)
	_ = call(t, server, http.MethodGet, "/api/catalog", "")

	res := call(t, server, http.MethodPut, "/api/settings", `{"prefixes":[],"suffixes":[],"whitelist":"","version":{"enabled":false},"protocol":{"codex_regex":"","claude_regex":""}}`)
	if res.Code != http.StatusOK {
		t.Fatalf("status = %d: %s", res.Code, res.Body.String())
	}
	var payload struct {
		View catalog.View `json:"view"`
	}
	if err := json.Unmarshal(res.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode: %v", err)
	}
	for _, model := range payload.View.Models {
		if model.Protocol != "openai" {
			t.Fatalf("%s tagged %q with no patterns configured", model.Upstream, model.Protocol)
		}
	}
}

// The settings page has to survive a stored pattern that no longer compiles:
// it is the only place the user can repair it.
func TestSettingsStayEditableWhenTheStoredRegexIsBroken(t *testing.T) {
	fake := newFakeCPA(t)
	server := newTestServer(t, fake)
	if err := server.Store.SetProtocolRegex(clean.ProtocolConfig{
		CodexRegex:  `(?i)^(?!.*mini).*gpt.*`,
		ClaudeRegex: `(?i)claude`,
	}); err != nil {
		t.Fatalf("SetProtocolRegex: %v", err)
	}

	if res := call(t, server, http.MethodGet, "/api/settings", ""); res.Code != http.StatusOK {
		t.Fatalf("GET settings = %d, want 200", res.Code)
	}

	fixed := `{"prefixes":[],"suffixes":[],"whitelist":"","version":{"enabled":false},"protocol":{"codex_regex":"(?i)^gpt-[0-9.]+$","claude_regex":"(?i)claude"}}`
	res := call(t, server, http.MethodPut, "/api/settings", fixed)
	if res.Code != http.StatusOK {
		t.Fatalf("PUT settings = %d: %s", res.Code, res.Body.String())
	}

	if res := call(t, server, http.MethodGet, "/api/catalog", ""); res.Code != http.StatusOK {
		t.Fatalf("catalog still broken after the fix: %d %s", res.Code, res.Body.String())
	}
	if len(fake.writes()) != 0 {
		t.Fatalf("repairing settings wrote to CPA: %v", fake.writes())
	}
}
