package cpa

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestDiscoverModelsGoesThroughManagementAPICall(t *testing.T) {
	var server *httptest.Server
	server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v0/management/api-call" || r.Method != http.MethodPost {
			http.NotFound(w, r)
			return
		}
		var payload struct {
			Method string            `json:"method"`
			URL    string            `json:"url"`
			Header map[string]string `json:"header"`
		}
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			t.Fatalf("decode api-call payload: %v", err)
		}
		if payload.URL != server.URL+"/v1/models" {
			t.Fatalf("target = %q", payload.URL)
		}
		if payload.Header["Authorization"] != "Bearer site-key" {
			t.Fatalf("authorization = %q", payload.Header["Authorization"])
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"status_code": http.StatusOK,
			"body": map[string]any{"data": []map[string]string{
				{"id": "model-a"}, {"name": "model-b"}, {"id": "model-a"},
			}},
		})
	}))
	defer server.Close()

	client := NewClient(server.URL, "management-secret")
	names, err := client.DiscoverModels(DiscoverTarget{BaseURL: server.URL, APIKey: "site-key"})
	if err != nil {
		t.Fatalf("DiscoverModels: %v", err)
	}
	if len(names) != 2 || names[0] != "model-a" || names[1] != "model-b" {
		t.Fatalf("names = %#v", names)
	}
}

func TestDiscoverModelsSurfacesUpstreamFailure(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{"status_code": 401, "body": "unauthorized"})
	}))
	defer server.Close()

	client := NewClient(server.URL, "secret")
	if _, err := client.DiscoverModels(DiscoverTarget{BaseURL: server.URL + "/v1"}); err == nil {
		t.Fatal("expected an error for a 401 upstream")
	}
}

// Round-tripping must not invent or drop keys: codex/claude entries carry
// auth-index and a flat api-key that CPA needs back verbatim.
func TestProviderRoundTripPreservesUnknownFields(t *testing.T) {
	original := []map[string]any{{
		"base-url":   "https://example.com",
		"api-key":    "k",
		"proxy-url":  "socks5://127.0.0.1:1080",
		"auth-index": float64(2),
		"models": []any{
			map[string]any{"name": "m1", "alias": "a1", "max-context-length": float64(8192)},
			map[string]any{"name": "m2"},
		},
	}}
	providers, err := ProvidersFromPayload(original)
	if err != nil {
		t.Fatalf("ProvidersFromPayload: %v", err)
	}
	before, _ := json.Marshal(original)
	after, _ := json.Marshal(ChannelPayload(providers))
	if string(before) != string(after) {
		t.Fatalf("round trip changed payload:\n before=%s\n after =%s", before, after)
	}
}

func TestEmptyAliasIsRemovedNotWrittenBlank(t *testing.T) {
	m := Model{Name: "m", Alias: "", Raw: map[string]any{"name": "m", "alias": "old"}}
	if _, present := m.ToMap()["alias"]; present {
		t.Fatal("clearing an alias must delete the key")
	}
}
