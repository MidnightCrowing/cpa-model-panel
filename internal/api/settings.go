package api

import (
	"encoding/json"
	"net/http"

	"github.com/local/cpa-model-panel/internal/catalog"
)

// handleSettings exposes the whole pipeline configuration as one object.
//
// A PUT recomputes the view straight from the cached catalog and returns it,
// so changing a prefix, a whitelist or a protocol regex updates suggestions,
// exclusions and protocol tags immediately — without touching CPA. Nothing is
// written upstream until the user saves.
//
// Deliberately built on loadBase rather than load: the settings currently
// stored may be the very thing that breaks the pipeline, and this endpoint has
// to keep working so they can be repaired.
func (s *Server) handleSettings(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		settings, err := s.Store.Settings()
		if err != nil {
			writeErr(w, http.StatusInternalServerError, err.Error())
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"settings": settings})

	case http.MethodPut:
		var settings catalog.Settings
		if err := json.NewDecoder(r.Body).Decode(&settings); err != nil {
			writeErr(w, http.StatusBadRequest, "invalid body")
			return
		}

		s.mu.Lock()
		defer s.mu.Unlock()

		b, err := s.loadBase()
		if err != nil {
			writeErr(w, http.StatusBadGateway, err.Error())
			return
		}

		// Running the pipeline is the validation: a pattern that does not
		// compile is rejected here instead of being stored and hit on every
		// later load.
		view, err := s.compute(b, settings)
		if err != nil {
			writeErr(w, http.StatusBadRequest, err.Error())
			return
		}

		if err := s.Store.SetCleanRules(settings.CleaningRules()); err != nil {
			writeErr(w, http.StatusInternalServerError, err.Error())
			return
		}
		if err := s.Store.SetWhitelist(settings.Whitelist); err != nil {
			writeErr(w, http.StatusInternalServerError, err.Error())
			return
		}
		if err := s.Store.SetVersionFilter(settings.Version); err != nil {
			writeErr(w, http.StatusInternalServerError, err.Error())
			return
		}
		if err := s.Store.SetProtocolRegex(settings.Protocol); err != nil {
			writeErr(w, http.StatusInternalServerError, err.Error())
			return
		}

		writeJSON(w, http.StatusOK, map[string]any{"ok": true, "view": view})

	default:
		writeErr(w, http.StatusMethodNotAllowed, "method not allowed")
	}
}
