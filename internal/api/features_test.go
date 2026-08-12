package api

import (
	"encoding/base64"
	"net/http"
	"strings"
	"testing"
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

// A provider CPA holds without credentials can never authenticate, so the
// panel drops it rather than showing a column that fails every probe. CPA's
// own management UI hides these too, which is how they accumulate unnoticed.
func TestKeylessSiteIsDropped(t *testing.T) {
	fake := newFakeCPA(t)
	fake.lists["codex-api-key"] = append(fake.lists["codex-api-key"], map[string]any{
		"base-url": "https://nokey.example.com/v1",
		"api-key":  "",
		"models":   []any{map[string]any{"name": "gpt-5.4"}},
	})
	server := newTestServer(t, fake)

	view := decodeView(t, call(t, server, http.MethodGet, "/api/catalog", "").Body.String())
	for _, site := range view.Sites {
		if strings.Contains(site.BaseURL, "nokey.example.com") {
			t.Fatalf("keyless entry became a site: %+v", site)
		}
	}

	if res := call(t, server, http.MethodPost, "/api/sites/host%3Anokey.example.com/refresh", ""); res.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404: %s", res.Code, res.Body.String())
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
