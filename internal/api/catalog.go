package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
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
	rules, err := clean.NewRules(st.Settings.CleaningRules())
	if err != nil {
		send(map[string]any{"type": "done", "error": err.Error()})
		return
	}

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

			var names []string
			var err error
			if strings.TrimSpace(site.APIKey) == "" {
				err = errNoKey{}
			} else {
				names, err = s.CPA.DiscoverModels(cpa.DiscoverTarget{
					BaseURL: site.BaseURL,
					Headers: site.Headers,
					APIKey:  site.APIKey,
				})
			}
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

	// A provider CPA holds with an empty api-key can never authenticate, so it
	// is swept up here rather than left for the user to find — its own UI does
	// not even show these.
	swept, sweepErr := s.sweepKeylessSites(st)
	if sweepErr != nil {
		send(map[string]any{"type": "done", "error": "清理无密钥站点失败: " + sweepErr.Error()})
		return
	}
	if len(swept) > 0 {
		reloaded, err := s.load()
		if err != nil {
			send(map[string]any{"type": "done", "error": err.Error()})
			return
		}
		st = reloaded
	}

	failures := make([]refreshFailure, 0)
	added := 0
	refreshed := 0
	for _, result := range results {
		if result.site.APIKey == "" {
			// Removed above; reporting it as a failure would be noise.
			continue
		}
		_ = s.Store.RecordProbe(result.site.ID, result.err)
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
		TempSites:  st.TempSites,
		Health:     st.Health,
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
		"swept":     swept,
		"view":      view,
	})
}

// sweepKeylessSites deletes every provider entry CPA holds without an API key.
//
// They cannot work — CPA has nothing to authenticate with — and CPA's own
// management UI hides them, so they accumulate invisibly and fail every probe.
// A snapshot is written first, as with any other removal.
func (s *Server) sweepKeylessSites(st *state) ([]string, error) {
	victims := make([]catalog.Site, 0)
	for _, site := range st.Catalog.Sites() {
		if strings.TrimSpace(site.APIKey) == "" {
			victims = append(victims, site)
		}
	}
	if len(victims) == 0 {
		return nil, nil
	}

	if _, err := s.Store.AddSnapshot(st.View.Fingerprint, "pre-keyless-sweep", snapshotPayload(st.Snapshot)); err != nil {
		return nil, err
	}
	_ = s.Store.PruneSnapshots(s.Retain)

	drop := make(map[cpa.Channel]map[int]bool, len(cpa.AllChannels))
	for _, ch := range cpa.AllChannels {
		drop[ch] = map[int]bool{}
	}
	names := make([]string, 0, len(victims))
	for _, site := range victims {
		names = append(names, site.Name)
		for ch, idx := range site.Providers {
			drop[ch][idx] = true
		}
	}

	for _, ch := range cpa.AllChannels {
		if len(drop[ch]) == 0 {
			continue
		}
		providers := st.Snapshot.Providers(ch)
		next := make([]cpa.Provider, 0, len(providers))
		for i, provider := range providers {
			if !drop[ch][i] {
				next = append(next, provider)
			}
		}
		if err := s.CPA.PutChannel(ch, next); err != nil {
			return nil, err
		}
	}
	for _, site := range victims {
		_ = s.Store.ForgetSite(site.ID)
	}
	return names, nil
}
