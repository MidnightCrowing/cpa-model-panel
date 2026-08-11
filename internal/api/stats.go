package api

import (
	"net/http"
	"strings"

	"github.com/local/cpa-model-panel/internal/keeper"
)

// handleStats reports per-(site, model) success and failure counts from
// Keeper, so a matrix cell can say whether that model actually works there
// rather than only whether it is switched on.
func (s *Server) handleStats(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeErr(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	if !s.Keeper.Configured() {
		writeJSON(w, http.StatusOK, map[string]any{
			"configured": false,
			"reason":     "未配置 KEEPER_URL / KEEPER_PASSWORD",
		})
		return
	}

	window := strings.TrimSpace(r.URL.Query().Get("range"))
	switch window {
	case "", "24h", "today", "7d", "30d":
	default:
		writeErr(w, http.StatusBadRequest, "不支持的时间范围")
		return
	}

	stats, err := s.Keeper.Stats(window)
	if err != nil {
		writeErr(w, http.StatusBadGateway, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, statsResponse{Configured: true, Stats: stats})
}

type statsResponse struct {
	Configured bool `json:"configured"`
	keeper.Stats
}
