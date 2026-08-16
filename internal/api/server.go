package api

import (
	"context"
	"crypto/subtle"
	"encoding/json"
	"log"
	"net/http"
	"strings"
	"sync"

	"github.com/local/cpa-model-panel/internal/catalog"
	"github.com/local/cpa-model-panel/internal/cpa"
	"github.com/local/cpa-model-panel/internal/keeper"
	"github.com/local/cpa-model-panel/internal/store"
)

type Server struct {
	AdminToken string
	CPA        *cpa.Client
	Store      *store.Store
	Keeper     *keeper.Client
	Retain     int

	// mu serialises every read-modify-write cycle against CPA.
	mu sync.Mutex

	// The scheduler has its own small state lock. It never nests mu while held.
	autoMu      sync.Mutex
	autoStarted bool
	autoRunning bool
	autoNextRun string
	autoWake    chan struct{}
	autoCancel  context.CancelFunc
	autoDone    chan struct{}
}

func (s *Server) Routes(mux *http.ServeMux) {
	mux.HandleFunc("/api/health", s.handleHealth)
	mux.HandleFunc("/api/login", s.handleLogin)
	mux.Handle("/api/catalog", s.auth(http.HandlerFunc(s.handleCatalog)))
	mux.Handle("/api/catalog/refresh", s.auth(http.HandlerFunc(s.handleRefresh)))
	mux.Handle("/api/save", s.auth(http.HandlerFunc(s.handleSave)))
	mux.Handle("/api/settings", s.auth(http.HandlerFunc(s.handleSettings)))
	mux.Handle("/api/auto-sync", s.auth(http.HandlerFunc(s.handleAutoSync)))
	mux.Handle("/api/snapshots", s.auth(http.HandlerFunc(s.handleSnapshots)))
	mux.Handle("/api/snapshots/", s.auth(http.HandlerFunc(s.handleSnapshotAction)))
	mux.Handle("/api/stats", s.auth(http.HandlerFunc(s.handleStats)))
	mux.Handle("/api/eggs", s.auth(http.HandlerFunc(s.handleEggs)))
	mux.Handle("/api/sites/", s.auth(http.HandlerFunc(s.handleSiteAction)))
}

func (s *Server) auth(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !secureEqual(bearer(r), s.AdminToken) {
			writeErr(w, http.StatusUnauthorized, "unauthorized")
			return
		}
		next.ServeHTTP(w, r)
	})
}

func bearer(r *http.Request) string {
	h := r.Header.Get("Authorization")
	if strings.HasPrefix(strings.ToLower(h), "bearer ") {
		return strings.TrimSpace(h[7:])
	}
	return strings.TrimSpace(r.Header.Get("X-Admin-Token"))
}

func secureEqual(a, b string) bool {
	if len(a) != len(b) {
		return false
	}
	return subtle.ConstantTimeCompare([]byte(a), []byte(b)) == 1
}

func (s *Server) handleHealth(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "service": "cpa-model-panel"})
}

func (s *Server) handleLogin(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeErr(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	var body struct {
		Token string `json:"token"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid body")
		return
	}
	if !secureEqual(strings.TrimSpace(body.Token), s.AdminToken) {
		writeErr(w, http.StatusUnauthorized, "invalid token")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "token": s.AdminToken})
}

// base is one consistent read of CPA and of everything stored locally, with
// the filter pipeline not yet run.
//
// The split matters: settings are strict, so a pattern that stops compiling
// makes the pipeline fail. If the settings endpoint needed a successful
// pipeline first, a bad regex would lock the user out of the only page where
// it can be fixed.
type base struct {
	Snapshot    *cpa.Snapshot
	Catalog     *catalog.Catalog
	Fingerprint string
	Settings    catalog.Settings
	Exclusions  catalog.RefSet
	Disabled    catalog.RefSet
	Keeps       catalog.RefSet
	TempSites   map[string]string
	Health      map[string]catalog.SiteProbe
}

// state is a base plus the computed view.
type state struct {
	*base
	View catalog.View
}

func (s *Server) loadBase() (*base, error) {
	snapshot, err := s.CPA.Snapshot()
	if err != nil {
		return nil, err
	}
	cached, err := s.Store.LoadCatalog()
	if err != nil {
		return nil, err
	}
	cat := catalog.Reconcile(cached, snapshot)

	if err := s.Store.MigrateLegacyDisabled(cat.Sites()); err != nil {
		log.Printf("legacy migration: %v", err)
	}

	fingerprint, err := catalog.Fingerprint(snapshot)
	if err != nil {
		return nil, err
	}
	settings, err := s.Store.Settings()
	if err != nil {
		return nil, err
	}
	exclusions, err := s.Store.Exclusions()
	if err != nil {
		return nil, err
	}
	disabled, err := s.Store.Disabled()
	if err != nil {
		return nil, err
	}
	keeps, err := s.Store.Keeps()
	if err != nil {
		return nil, err
	}
	tempRows, err := s.Store.TempSites()
	if err != nil {
		return nil, err
	}
	temp := make(map[string]string, len(tempRows))
	for id, row := range tempRows {
		temp[id] = row.SourceURL
	}
	healthRows, err := s.Store.SiteHealth()
	if err != nil {
		return nil, err
	}
	health := make(map[string]catalog.SiteProbe, len(healthRows))
	for id, row := range healthRows {
		health[id] = catalog.SiteProbe{LastOKAt: row.LastOKAt, LastError: row.LastError, Failures: row.Failures}
	}

	return &base{
		Snapshot:    snapshot,
		Catalog:     cat,
		Fingerprint: fingerprint,
		Settings:    settings,
		Exclusions:  exclusions,
		Disabled:    disabled,
		Keeps:       keeps,
		TempSites:   temp,
		Health:      health,
	}, nil
}

// compute runs the pipeline over a base and persists the pruned cache.
func (s *Server) compute(b *base, settings catalog.Settings) (catalog.View, error) {
	view, err := catalog.Compute(catalog.Inputs{
		Catalog:    b.Catalog,
		Settings:   settings,
		Exclusions: b.Exclusions,
		Disabled:   b.Disabled,
		Keeps:      b.Keeps,
		TempSites:  b.TempSites,
		Health:     b.Health,
	})
	if err != nil {
		return catalog.View{}, err
	}
	view.Fingerprint = b.Fingerprint

	catalog.Prune(b.Catalog, view)
	if err := s.Store.SaveCatalog(b.Catalog); err != nil {
		return catalog.View{}, err
	}
	return view, nil
}

// load performs the full read pipeline: CPA snapshot → reconcile with the
// cached catalog → run filters → persist the pruned cache.
func (s *Server) load() (*state, error) {
	b, err := s.loadBase()
	if err != nil {
		return nil, err
	}
	view, err := s.compute(b, b.Settings)
	if err != nil {
		return nil, err
	}
	return &state{base: b, View: view}, nil
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	// Never let a proxy or the browser hand back an older view of CPA.
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func writeErr(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, map[string]any{"error": msg})
}
