package keeper

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestSiteOfStripsTheMaskedKey(t *testing.T) {
	cases := map[string]string{
		"CHY公益站 @ 1Om*********9mdv5B":          "CHY公益站",
		"maoyulin / kimi @ sk-*********L8VTKS": "maoyulin / kimi",
		"no-key-site":                          "no-key-site",
	}
	for in, want := range cases {
		if got := SiteOf(in); got != want {
			t.Errorf("SiteOf(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestStatsAggregatesBySiteAndModel(t *testing.T) {
	var logins int
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get(fetchHeader) != fetchHeaderOn {
			// Keeper serves its SPA to anything that does not look like its
			// own frontend, which is what made the API look absent.
			w.WriteHeader(http.StatusForbidden)
			_ = json.NewEncoder(w).Encode(map[string]string{"error": "fetch request required"})
			return
		}
		switch r.URL.Path {
		case apiPrefix + "/auth/login":
			logins++
			http.SetCookie(w, &http.Cookie{Name: "session", Value: "abc", Path: "/"})
			w.WriteHeader(http.StatusNoContent)
		case apiPrefix + "/usage/events":
			if _, err := r.Cookie("session"); err != nil {
				w.WriteHeader(http.StatusUnauthorized)
				return
			}
			_ = json.NewEncoder(w).Encode(eventsPage{
				TotalPages: 1,
				Events: []event{
					{Source: "站点甲 @ sk-***abc", Model: "gpt-5.4", Failed: false, LatencyMs: 100, Timestamp: "2026-08-11T10:00:00Z"},
					{Source: "站点甲 @ sk-***abc", Model: "gpt-5.4", Failed: false, LatencyMs: 300, Timestamp: "2026-08-11T11:00:00Z"},
					{Source: "站点甲 @ sk-***abc", Model: "gpt-5.4", Failed: true, Timestamp: "2026-08-11T12:00:00Z"},
					{Source: "站点乙 @ sk-***xyz", Model: "claude-opus-5", Failed: true, Timestamp: "2026-08-11T09:00:00Z"},
				},
			})
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	client := NewClient(server.URL, "pw")
	stats, err := client.Stats("24h")
	if err != nil {
		t.Fatalf("Stats: %v", err)
	}
	if stats.Events != 4 || len(stats.Cells) != 2 {
		t.Fatalf("events=%d cells=%d, want 4/2", stats.Events, len(stats.Cells))
	}
	for _, cell := range stats.Cells {
		switch cell.Site + "/" + cell.Model {
		case "站点甲/gpt-5.4":
			if cell.OK != 2 || cell.Failed != 1 {
				t.Errorf("ok/failed = %d/%d, want 2/1", cell.OK, cell.Failed)
			}
			if cell.LatencyMs != 200 {
				t.Errorf("latency = %d, want the mean of successes (200)", cell.LatencyMs)
			}
			if cell.LastAt != "2026-08-11T12:00:00Z" {
				t.Errorf("last_at = %q", cell.LastAt)
			}
		case "站点乙/claude-opus-5":
			if cell.OK != 0 || cell.Failed != 1 {
				t.Errorf("ok/failed = %d/%d, want 0/1", cell.OK, cell.Failed)
			}
		default:
			t.Errorf("unexpected cell %+v", cell)
		}
	}
	if logins != 1 {
		t.Errorf("logins = %d, want 1 reused session", logins)
	}
}
