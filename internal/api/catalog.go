package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"sync"
	"time"

	"github.com/local/cpa-model-panel/internal/catalog"
	"github.com/local/cpa-model-panel/internal/clean"
	"github.com/local/cpa-model-panel/internal/cpa"
)

func (s *Server) handleCatalog(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeErr(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()

	st, err := s.load()
	if err != nil {
		writeErr(w, http.StatusBadGateway, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, st.View)
}

type refreshFailure struct {
	Site  string `json:"site"`
	Name  string `json:"name"`
	Error string `json:"error"`
}

// handleRefresh asks CPA to fetch every site's own model list and folds the
// result into the cached catalog.
//
// It deliberately does not write to CPA: discovered models show up as pending
// rows and only reach CPA when the user saves. A site that fails to answer
// simply keeps whatever the catalog already knows about it.
func (s *Server) handleRefresh(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeErr(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	flusher, ok := w.(http.Flusher)
	if !ok {
		writeErr(w, http.StatusInternalServerError, "streaming unsupported")
		return
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("X-Accel-Buffering", "no")

	var sendMu sync.Mutex
	send := func(event map[string]any) {
		sendMu.Lock()
		defer sendMu.Unlock()
		encoded, _ := json.Marshal(event)
		fmt.Fprintf(w, "data: %s\n\n", encoded)
		flusher.Flush()
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	st, err := s.load()
	if err != nil {
		send(map[string]any{"type": "done", "error": err.Error()})
		return
	}

	matcher, err := clean.NewProtocolMatcher(st.Settings.Protocol)
	if err != nil {
		send(map[string]any{"type": "done", "error": err.Error()})
		return
	}
	rules := clean.NewRules(st.Settings.Prefixes, st.Settings.Suffixes)

	sites := st.Catalog.Sites()
	send(map[string]any{"type": "start", "total": len(sites)})

	type discovery struct {
		site  catalog.Site
		names []string
		err   error
	}
	results := make([]discovery, len(sites))

	var wg sync.WaitGroup
	semaphore := make(chan struct{}, 8)
	var progressMu sync.Mutex
	completed := 0

	for i, site := range sites {
		wg.Add(1)
		go func(i int, site catalog.Site) {
			defer wg.Done()
			semaphore <- struct{}{}
			defer func() { <-semaphore }()

			names, err := s.CPA.DiscoverModels(cpa.DiscoverTarget{
				BaseURL: site.BaseURL,
				Headers: site.Headers,
				APIKey:  site.APIKey,
			})
			results[i] = discovery{site: site, names: names, err: err}

			progressMu.Lock()
			completed++
			done := completed
			progressMu.Unlock()

			event := map[string]any{
				"type":      "progress",
				"completed": done,
				"total":     len(sites),
				"site":      site.Name,
				"ok":        err == nil,
			}
			if err != nil {
				event["error"] = err.Error()
			} else {
				event["found"] = len(names)
			}
			send(event)
		}(i, site)
	}
	wg.Wait()

	failures := make([]refreshFailure, 0)
	added := 0
	refreshed := 0
	for _, result := range results {
		if result.err != nil {
			failures = append(failures, refreshFailure{
				Site:  result.site.ID,
				Name:  result.site.Name,
				Error: result.err.Error(),
			})
			continue
		}
		refreshed++
		added += catalog.MergeDiscovered(st.Catalog, result.site.ID, result.names, matcher, rules)
	}

	st.Catalog.FetchedAt = time.Now().UTC().Format(time.RFC3339)

	view, err := catalog.Compute(catalog.Inputs{
		Catalog:    st.Catalog,
		Settings:   st.Settings,
		Exclusions: st.Exclusions,
		Disabled:   st.Disabled,
		Keeps:      st.Keeps,
	})
	if err != nil {
		send(map[string]any{"type": "done", "error": err.Error()})
		return
	}
	view.Fingerprint = st.View.Fingerprint

	catalog.Prune(st.Catalog, view)
	if err := s.Store.SaveCatalog(st.Catalog); err != nil {
		send(map[string]any{"type": "done", "error": err.Error()})
		return
	}

	send(map[string]any{
		"type":      "done",
		"ok":        true,
		"refreshed": refreshed,
		"added":     added,
		"failed":    failures,
		"view":      view,
	})
}
