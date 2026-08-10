package api

import (
	"net/http"
	"strconv"
	"strings"

	"github.com/local/cpa-model-panel/internal/catalog"
	"github.com/local/cpa-model-panel/internal/cpa"
)

func (s *Server) handleSnapshots(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeErr(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	list, err := s.Store.ListSnapshots(s.Retain)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"snapshots": list})
}

// handleSnapshotAction serves POST /api/snapshots/{id}/rollback.
func (s *Server) handleSnapshotAction(w http.ResponseWriter, r *http.Request) {
	parts := strings.Split(strings.Trim(strings.TrimPrefix(r.URL.Path, "/api/snapshots/"), "/"), "/")
	if len(parts) != 2 || parts[1] != "rollback" || r.Method != http.MethodPost {
		writeErr(w, http.StatusNotFound, "not found")
		return
	}
	id, err := strconv.ParseInt(parts[0], 10, 64)
	if err != nil {
		writeErr(w, http.StatusBadRequest, "bad id")
		return
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	var payload channelSnapshot
	if err := s.Store.SnapshotPayload(id, &payload); err != nil {
		writeErr(w, http.StatusNotFound, "快照不存在")
		return
	}

	current, err := s.CPA.Snapshot()
	if err != nil {
		writeErr(w, http.StatusBadGateway, err.Error())
		return
	}
	fingerprint, err := catalog.Fingerprint(current)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	if _, err := s.Store.AddSnapshot(fingerprint, "pre-rollback", snapshotPayload(current)); err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	_ = s.Store.PruneSnapshots(s.Retain)

	for _, ch := range cpa.AllChannels {
		entries, ok := payload[string(ch)]
		if !ok {
			continue
		}
		providers, err := cpa.ProvidersFromPayload(entries)
		if err != nil {
			writeErr(w, http.StatusInternalServerError, err.Error())
			return
		}
		if err := s.CPA.PutChannel(ch, providers); err != nil {
			writeErr(w, http.StatusBadGateway, "回滚 "+string(ch)+" 失败: "+err.Error())
			return
		}
	}

	st, err := s.load()
	if err != nil {
		writeJSON(w, http.StatusOK, map[string]any{"ok": true})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "view": st.View})
}
