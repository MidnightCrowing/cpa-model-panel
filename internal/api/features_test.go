package api

import (
	"encoding/base64"
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"github.com/local/cpa-model-panel/internal/catalog"
)

// Shared keys usually arrive base64-wrapped, but not always.
func TestDecodeKeyUnwrapsBase64AndLeavesTherestAlone(t *testing.T) {
	plain := "sk-abcdefghijklmnopqrstuvwxyz"
	if got, decoded := DecodeKey(base64.StdEncoding.EncodeToString([]byte(plain))); got != plain || !decoded {
		t.Errorf("base64 key = %q, decoded=%v", got, decoded)
	}
	if got, decoded := DecodeKey(plain); got != plain || decoded {
		t.Errorf("plain key = %q, decoded=%v", got, decoded)
	}
	if got, _ := DecodeKey("  sk-with-spaces-around  "); got != "sk-with-spaces-around" {
		t.Errorf("trim failed: %q", got)
	}
}

// People paste whatever the post had: bare host, /v1, even the chat endpoint.
func TestNormalizeEggURL(t *testing.T) {
	cases := map[string]string{
		"https://x.example.com":                     "https://x.example.com/v1",
		"https://x.example.com/":                    "https://x.example.com/v1",
		"https://x.example.com/v1":                  "https://x.example.com/v1",
		"x.example.com/v1/":                         "https://x.example.com/v1",
		"https://x.example.com/v1/chat/completions": "https://x.example.com/v1",
		"https://x.example.com/v1/models":           "https://x.example.com/v1",
	}
	for in, want := range cases {
		got, err := normalizeEggURL(in)
		if err != nil || got != want {
			t.Errorf("normalizeEggURL(%q) = %q, %v; want %q", in, got, err, want)
		}
	}
	if _, err := normalizeEggURL(""); err == nil {
		t.Error("empty URL should fail")
	}
}

// Two upstream models at one site written under the same name is a real
// misconfiguration; the panel should say so rather than silently pick one.
func TestConflictsAreReported(t *testing.T) {
	fake := newFakeCPA(t)
	server := newTestServer(t, fake)

	view := decodeView(t, call(t, server, http.MethodGet, "/api/catalog", "").Body.String())
	body := `{"fingerprint":"` + view.Fingerprint + `","ops":[
		{"type":"rename","to":"same-name","targets":[
			{"site":"site-a","upstream":"deepseek-ai/DeepSeek-V3"},
			{"site":"site-a","upstream":"old-model"}
		]}
	]}`
	res := call(t, server, http.MethodPost, "/api/save?dry=1", body)
	if res.Code != http.StatusOK {
		t.Fatalf("preview = %d: %s", res.Code, res.Body.String())
	}
	var payload struct {
		Dry       bool                  `json:"dry"`
		Conflicts []catalog.Conflict    `json:"conflicts"`
		Diff      []catalog.ChannelDiff `json:"diff"`
	}
	if err := json.Unmarshal(res.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(payload.Conflicts) != 1 {
		t.Fatalf("conflicts = %+v, want 1", payload.Conflicts)
	}
	if len(payload.Conflicts[0].Upstreams) != 2 {
		t.Errorf("conflict should name both models: %+v", payload.Conflicts[0])
	}
	if len(fake.writes()) != 0 {
		t.Fatalf("a preview wrote to CPA: %v", fake.writes())
	}
}

// The preview must describe the same change the save would apply.
func TestPreviewNamesWhatWouldChange(t *testing.T) {
	fake := newFakeCPA(t)
	server := newTestServer(t, fake)

	view := decodeView(t, call(t, server, http.MethodGet, "/api/catalog", "").Body.String())
	body := `{"fingerprint":"` + view.Fingerprint + `","ops":[
		{"type":"exclude","targets":[{"site":"site-a","upstream":"old-model"}]}
	]}`
	res := call(t, server, http.MethodPost, "/api/save?dry=1", body)
	if res.Code != http.StatusOK {
		t.Fatalf("preview = %d: %s", res.Code, res.Body.String())
	}
	if !strings.Contains(res.Body.String(), "old-model") {
		t.Fatalf("preview did not name the model being removed: %s", res.Body.String())
	}
	if len(fake.writes()) != 0 {
		t.Fatal("preview wrote to CPA")
	}

	// The same request without dry=1 applies it.
	if res := call(t, server, http.MethodPost, "/api/save", body); res.Code != http.StatusOK {
		t.Fatalf("save = %d: %s", res.Code, res.Body.String())
	}
	if len(fake.writes()) == 0 {
		t.Fatal("save wrote nothing")
	}
}

// A site CPA holds without credentials cannot be probed; saying that beats
// relaying the upstream's "Invalid token".
func TestKeylessSiteIsNotProbed(t *testing.T) {
	fake := newFakeCPA(t)
	fake.lists["codex-api-key"] = append(fake.lists["codex-api-key"], map[string]any{
		"base-url": "https://nokey.example.com/v1",
		"api-key":  "",
		"models":   []any{map[string]any{"name": "gpt-5.4"}},
	})
	server := newTestServer(t, fake)

	res := call(t, server, http.MethodPost, "/api/sites/"+"host%3Anokey.example.com"+"/refresh", "")
	if res.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400: %s", res.Code, res.Body.String())
	}
	if !strings.Contains(res.Body.String(), "api-key") {
		t.Fatalf("error should explain the missing key: %s", res.Body.String())
	}
}

// Deleting a site clears it from every list it appears in.
func TestDeleteSiteRemovesItEverywhere(t *testing.T) {
	fake := newFakeCPA(t)
	server := newTestServer(t, fake)
	_ = call(t, server, http.MethodGet, "/api/catalog", "")

	res := call(t, server, http.MethodDelete, "/api/sites/site-a", "")
	if res.Code != http.StatusOK {
		t.Fatalf("delete = %d: %s", res.Code, res.Body.String())
	}

	fake.mu.Lock()
	defer fake.mu.Unlock()
	for name, list := range fake.lists {
		if len(list) != 0 {
			t.Errorf("%s still holds %d providers", name, len(list))
		}
	}
}
