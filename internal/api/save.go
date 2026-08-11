package api

import (
	"encoding/json"
	"net/http"

	"github.com/local/cpa-model-panel/internal/catalog"
	"github.com/local/cpa-model-panel/internal/cpa"
)

type saveRequest struct {
	Fingerprint string       `json:"fingerprint"`
	Ops         []catalog.Op `json:"ops"`
	Note        string       `json:"note"`
}

type saveResponse struct {
	OK       bool         `json:"ok"`
	View     catalog.View `json:"view"`
	Written  []string     `json:"written"`
	Renamed  int          `json:"renamed"`
	Kept     int          `json:"kept"`
	Removed  int          `json:"removed"`
	Restored int          `json:"restored"`
	Moved    int          `json:"moved"`
	Created  []string     `json:"created"`
	Skipped  int          `json:"skipped"`
	Snapshot int64        `json:"snapshot,omitempty"`
}

// handleSave is the panel's only path that writes to CPA.
func (s *Server) handleSave(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeErr(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	var req saveRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid body")
		return
	}
	if req.Fingerprint == "" {
		writeErr(w, http.StatusBadRequest, "fingerprint required")
		return
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	st, err := s.load()
	if err != nil {
		writeErr(w, http.StatusBadGateway, err.Error())
		return
	}
	if st.View.Fingerprint != req.Fingerprint {
		writeErr(w, http.StatusConflict, "CPA 配置已在面板之外发生变化，请刷新后重试")
		return
	}

	opResult, err := catalog.ApplyOps(st.Catalog, st.Exclusions, st.Disabled, st.Keeps, req.Ops)
	if err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}

	view, err := catalog.Compute(catalog.Inputs{
		Catalog:    st.Catalog,
		Settings:   st.Settings,
		Exclusions: opResult.Exclusions,
		Disabled:   opResult.Disabled,
		Keeps:      opResult.Keeps,
		TempSites:  st.TempSites,
		Health:     st.Health,
	})
	if err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}

	write := catalog.BuildWrite(st.Catalog, view, st.Snapshot, opResult.Priorities)

	// A preview runs everything except the writes, so the diff shown before a
	// big change is produced by the same code that will apply it.
	if r.URL.Query().Get("dry") == "1" {
		writeJSON(w, http.StatusOK, map[string]any{
			"ok":        true,
			"dry":       true,
			"diff":      catalog.Diff(st.Snapshot, write),
			"conflicts": view.Conflicts,
			"moved":     write.Moved,
			"created":   write.Created,
		})
		return
	}

	// Persist the edited catalog before touching CPA.
	//
	// Renames live on the catalog entry, and only models that actually reach
	// CPA get their new name back on the next read. Without this, renaming
	// something the rules currently exclude — or a model disabled at its site —
	// silently reverted: the save wrote nothing, the reload rebuilt from the
	// stale cache, and the edit was gone.
	catalog.Prune(st.Catalog, view)
	if err := s.Store.SaveCatalog(st.Catalog); err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}

	changed := make([]cpa.Channel, 0, len(cpa.AllChannels))
	for _, ch := range cpa.AllChannels {
		if write.Changed[ch] {
			changed = append(changed, ch)
		}
	}

	response := saveResponse{
		OK:       true,
		Renamed:  opResult.Renamed,
		Kept:     write.Kept,
		Removed:  write.Removed,
		Restored: write.Restored,
		Moved:    write.Moved,
		Created:  write.Created,
		Skipped:  opResult.Skipped,
	}

	if len(changed) > 0 {
		note := req.Note
		if note == "" {
			note = "pre-save"
		}
		id, err := s.Store.AddSnapshot(req.Fingerprint, note, snapshotPayload(st.Snapshot))
		if err != nil {
			writeErr(w, http.StatusInternalServerError, "写入快照失败: "+err.Error())
			return
		}
		_ = s.Store.PruneSnapshots(s.Retain)
		response.Snapshot = id

		for _, ch := range changed {
			if err := s.CPA.PutChannel(ch, write.Channels[ch]); err != nil {
				writeErr(w, http.StatusBadGateway, "写回 "+string(ch)+" 失败: "+err.Error())
				return
			}
			response.Written = append(response.Written, string(ch))
		}
	}

	if err := s.Store.SetExclusions(opResult.Exclusions); err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	if err := s.Store.SetDisabled(opResult.Disabled); err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	if err := s.Store.SetKeeps(opResult.Keeps); err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}

	fresh, err := s.load()
	if err != nil {
		response.View = view
		writeJSON(w, http.StatusOK, response)
		return
	}
	response.View = fresh.View
	writeJSON(w, http.StatusOK, response)
}

type channelSnapshot map[string][]map[string]any

func snapshotPayload(snap *cpa.Snapshot) channelSnapshot {
	out := make(channelSnapshot, len(cpa.AllChannels))
	for _, ch := range cpa.AllChannels {
		out[string(ch)] = cpa.ChannelPayload(snap.Providers(ch))
	}
	return out
}
