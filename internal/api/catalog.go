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

type refreshProgress struct {
	Completed int    `json:"completed"`
	Total     int    `json:"total"`
	Site      string `json:"site"`
	OK        bool   `json:"ok"`
	Found     int    `json:"found,omitempty"`
	Error     string `json:"error,omitempty"`
}

type refreshResponse struct {
	OK        bool             `json:"ok"`
	Refreshed int              `json:"refreshed"`
	Added     int              `json:"added"`
	Dropped   int              `json:"dropped"`
	Failed    []refreshFailure `json:"failed"`
	Swept     []string         `json:"swept,omitempty"`
	View      catalog.View     `json:"view"`
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
	send := func(event any) {
		sendMu.Lock()
		defer sendMu.Unlock()
		encoded, _ := json.Marshal(event)
		fmt.Fprintf(w, "data: %s\n\n", encoded)
		flusher.Flush()
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	result, err := s.refreshLocked(
		func(total int) { send(map[string]any{"type": "start", "total": total}) },
		func(progress refreshProgress) {
			event := map[string]any{
				"type":      "progress",
				"completed": progress.Completed,
				"total":     progress.Total,
				"site":      progress.Site,
				"ok":        progress.OK,
			}
			if progress.Error != "" {
				event["error"] = progress.Error
			} else {
				event["found"] = progress.Found
			}
			send(event)
		},
		true,
	)
	if err != nil {
		send(map[string]any{"type": "done", "error": err.Error()})
		return
	}
	send(struct {
		Type string `json:"type"`
		refreshResponse
	}{Type: "done", refreshResponse: result})
}

// refreshLocked runs model discovery and updates the cached catalog. The
// caller owns s.mu. A scheduled run sets sweepKeyless=false because its scope
// is only model discovery/mapping; the interactive refresh retains its
// existing cleanup behaviour.
func (s *Server) refreshLocked(onStart func(int), onProgress func(refreshProgress), sweepKeyless bool) (refreshResponse, error) {
	st, err := s.load()
	if err != nil {
		return refreshResponse{}, err
	}

	matcher, err := clean.NewProtocolMatcher(st.Settings.Protocol)
	if err != nil {
		return refreshResponse{}, err
	}
	rules, err := clean.NewRules(st.Settings.CleaningRules())
	if err != nil {
		return refreshResponse{}, err
	}

	sites := st.Catalog.Sites()
	if onStart != nil {
		onStart(len(sites))
	}
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
			var probeErr error
			if strings.TrimSpace(site.APIKey) == "" {
				probeErr = errNoKey{}
			} else {
				names, probeErr = s.CPA.DiscoverModels(cpa.DiscoverTarget{
					BaseURL: site.BaseURL,
					Headers: site.Headers,
					APIKey:  site.APIKey,
				})
			}
			results[i] = discovery{site: site, names: names, err: probeErr}

			progressMu.Lock()
			completed++
			progress := refreshProgress{
				Completed: completed,
				Total:     len(sites),
				Site:      site.Name,
				OK:        probeErr == nil,
				Found:     len(names),
			}
			if probeErr != nil {
				progress.Error = probeErr.Error()
			}
			progressMu.Unlock()
			if onProgress != nil {
				onProgress(progress)
			}
		}(i, site)
	}
	wg.Wait()

	var swept []string
	if sweepKeyless {
		swept, err = s.sweepKeylessSites(st)
		if err != nil {
			return refreshResponse{}, fmt.Errorf("清理无密钥站点失败: %w", err)
		}
		if len(swept) > 0 {
			st, err = s.load()
			if err != nil {
				return refreshResponse{}, err
			}
		}
	}

	response := refreshResponse{OK: true, Failed: make([]refreshFailure, 0), Swept: swept}
	for _, result := range results {
		if result.site.APIKey == "" {
			continue
		}
		_ = s.Store.RecordProbe(result.site.ID, result.err)
		if result.err != nil {
			response.Failed = append(response.Failed, refreshFailure{
				Site: result.site.ID, Name: result.site.Name, Error: result.err.Error(),
			})
			continue
		}
		response.Refreshed++
		added, dropped := catalog.MergeDiscovered(st.Catalog, result.site.ID, result.names, matcher, rules)
		response.Added += added
		response.Dropped += dropped
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
		return refreshResponse{}, err
	}
	view.Fingerprint = st.View.Fingerprint
	catalog.Prune(st.Catalog, view)
	if err := s.Store.SaveCatalog(st.Catalog); err != nil {
		return refreshResponse{}, err
	}
	response.View = view
	return response, nil
}

// sweepKeylessSites deletes every provider entry CPA holds without an API key.
//
// They cannot work — CPA has nothing to authenticate with — and CPA's own
// management UI hides them, so they accumulate invisibly and fail every probe.
// A snapshot is written first, as with any other removal.
//
// This reads the raw CPA snapshot, not the site list: BuildSites drops keyless
// entries so they never reach the UI, which would leave this with nothing to
// find if it went through the catalog.
func (s *Server) sweepKeylessSites(st *state) ([]string, error) {
	drop := make(map[cpa.Channel]map[int]bool, len(cpa.AllChannels))
	names := make([]string, 0)
	found := 0
	for _, ch := range cpa.AllChannels {
		drop[ch] = map[int]bool{}
		for idx, provider := range st.Snapshot.Providers(ch) {
			if len(provider.APIKeys()) > 0 {
				continue
			}
			drop[ch][idx] = true
			found++
			label := strings.TrimSpace(provider.Name)
			if label == "" {
				label = provider.BaseURL
			}
			names = append(names, label)
		}
	}
	if found == 0 {
		return nil, nil
	}

	if _, err := s.Store.AddSnapshot(st.View.Fingerprint, "pre-keyless-sweep", snapshotPayload(st.Snapshot)); err != nil {
		return nil, err
	}
	_ = s.Store.PruneSnapshots(s.Retain)

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
	return names, nil
}
