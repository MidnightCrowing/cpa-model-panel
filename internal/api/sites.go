package api

import (
	"net/http"
	"net/url"
	"strings"

	"github.com/local/cpa-model-panel/internal/catalog"
	"github.com/local/cpa-model-panel/internal/clean"
	"github.com/local/cpa-model-panel/internal/cpa"
)

// ErrNoKey is what a probe reports for a site CPA holds without credentials.
//
// Several entries in the live configuration have `api-key: ""`. CPA's own
// management UI hides them, so they are invisible there while still producing
// a 401 on every refresh. Saying so plainly beats relaying the upstream's
// "Invalid token".
const noKeyReason = "站点没有配置 API Key（CPA 配置里 api-key 为空），已跳过探测"

func (s *Server) handleSiteAction(w http.ResponseWriter, r *http.Request) {
	path := strings.Trim(strings.TrimPrefix(r.URL.Path, "/api/sites/"), "/")
	parts := strings.Split(path, "/")
	siteID, err := url.PathUnescape(parts[0])
	if err != nil || siteID == "" {
		writeErr(w, http.StatusBadRequest, "站点名无法解析")
		return
	}

	switch {
	case len(parts) == 2 && parts[1] == "refresh" && r.Method == http.MethodPost:
		s.refreshSite(w, siteID)
	case len(parts) == 1 && r.Method == http.MethodDelete:
		s.deleteSite(w, siteID)
	default:
		writeErr(w, http.StatusNotFound, "not found")
	}
}

// refreshSite re-reads one site's model list instead of all forty, which is
// what you want after fixing a single key.
func (s *Server) refreshSite(w http.ResponseWriter, siteID string) {
	s.mu.Lock()
	defer s.mu.Unlock()

	st, err := s.load()
	if err != nil {
		writeErr(w, http.StatusBadGateway, err.Error())
		return
	}
	site := st.Catalog.Site(siteID)
	if site == nil {
		writeErr(w, http.StatusNotFound, "站点不存在："+siteID)
		return
	}
	if strings.TrimSpace(site.APIKey) == "" {
		_ = s.Store.RecordProbe(siteID, errNoKey{})
		writeErr(w, http.StatusBadRequest, noKeyReason)
		return
	}

	matcher, err := clean.NewProtocolMatcher(st.Settings.Protocol)
	if err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	rules, err := clean.NewRules(st.Settings.CleaningRules())
	if err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}

	names, probeErr := s.CPA.DiscoverModels(cpa.DiscoverTarget{
		BaseURL: site.BaseURL,
		Headers: site.Headers,
		APIKey:  site.APIKey,
	})
	_ = s.Store.RecordProbe(siteID, probeErr)
	if probeErr != nil {
		writeErr(w, http.StatusBadGateway, probeErr.Error())
		return
	}

	added := catalog.MergeDiscovered(st.Catalog, siteID, names, matcher, rules)
	view, err := s.compute(st.base, st.Settings)
	if err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"ok": true, "site": siteID, "found": len(names), "added": added, "view": view,
	})
}

// deleteSite removes a site from every channel that holds it. Used both to
// throw away an expired 鸡蛋 and to clear out the keyless, modelless entries
// that accumulate in codex-api-key.
func (s *Server) deleteSite(w http.ResponseWriter, siteID string) {
	s.mu.Lock()
	defer s.mu.Unlock()

	st, err := s.load()
	if err != nil {
		writeErr(w, http.StatusBadGateway, err.Error())
		return
	}
	site := st.Catalog.Site(siteID)
	if site == nil {
		writeErr(w, http.StatusNotFound, "站点不存在："+siteID)
		return
	}

	if _, err := s.Store.AddSnapshot(st.View.Fingerprint, "pre-site-delete", snapshotPayload(st.Snapshot)); err != nil {
		writeErr(w, http.StatusInternalServerError, "写入快照失败: "+err.Error())
		return
	}
	_ = s.Store.PruneSnapshots(s.Retain)

	removed := make([]string, 0, len(site.Providers))
	for _, ch := range cpa.AllChannels {
		idx, ok := site.Providers[ch]
		if !ok {
			continue
		}
		providers := st.Snapshot.Providers(ch)
		next := make([]cpa.Provider, 0, len(providers))
		for i, provider := range providers {
			if i != idx {
				next = append(next, provider)
			}
		}
		if err := s.CPA.PutChannel(ch, next); err != nil {
			writeErr(w, http.StatusBadGateway, "从 "+string(ch)+" 删除失败: "+err.Error())
			return
		}
		removed = append(removed, string(ch))
	}
	if err := s.Store.ForgetSite(siteID); err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}

	fresh, err := s.load()
	if err != nil {
		writeErr(w, http.StatusBadGateway, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "removed": removed, "view": fresh.View})
}

type errNoKey struct{}

func (errNoKey) Error() string { return noKeyReason }
