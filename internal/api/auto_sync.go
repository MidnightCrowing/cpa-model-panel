package api

import (
	"context"
	"crypto/rand"
	"encoding/json"
	"log"
	"math/big"
	"net/http"
	"sort"
	"strings"
	"time"

	"github.com/local/cpa-model-panel/internal/catalog"
	"github.com/local/cpa-model-panel/internal/store"
)

const autoSyncLogRetain = 100

type autoSyncStateView struct {
	Running   bool   `json:"running"`
	NextRunAt string `json:"next_run_at,omitempty"`
}

type autoSyncPayload struct {
	Config store.AutoSyncConfig `json:"config"`
	State  autoSyncStateView    `json:"state"`
	Logs   []store.AutoSyncLog  `json:"logs"`
}

func (s *Server) handleAutoSync(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		payload, err := s.autoSyncPayload()
		if err != nil {
			writeErr(w, http.StatusInternalServerError, err.Error())
			return
		}
		writeJSON(w, http.StatusOK, payload)

	case http.MethodPut:
		var cfg store.AutoSyncConfig
		if err := json.NewDecoder(r.Body).Decode(&cfg); err != nil {
			writeErr(w, http.StatusBadRequest, "invalid body")
			return
		}
		if err := cfg.Validate(); err != nil {
			writeErr(w, http.StatusBadRequest, err.Error())
			return
		}
		if err := s.Store.SetAutoSyncConfig(cfg); err != nil {
			writeErr(w, http.StatusInternalServerError, err.Error())
			return
		}

		// The old next-run timestamp is no longer accurate after any edit. The
		// loop will publish a newly randomized one as soon as it wakes.
		s.autoMu.Lock()
		if !s.autoRunning {
			s.autoNextRun = ""
		}
		s.autoMu.Unlock()
		s.wakeAutoSync()

		payload, err := s.autoSyncPayload()
		if err != nil {
			writeErr(w, http.StatusInternalServerError, err.Error())
			return
		}
		writeJSON(w, http.StatusOK, payload)

	default:
		writeErr(w, http.StatusMethodNotAllowed, "method not allowed")
	}
}

func (s *Server) autoSyncPayload() (autoSyncPayload, error) {
	cfg, err := s.Store.AutoSyncConfig()
	if err != nil {
		return autoSyncPayload{}, err
	}
	logs, err := s.Store.ListAutoSyncLogs(20)
	if err != nil {
		return autoSyncPayload{}, err
	}
	s.autoMu.Lock()
	state := autoSyncStateView{Running: s.autoRunning, NextRunAt: s.autoNextRun}
	s.autoMu.Unlock()
	return autoSyncPayload{Config: cfg, State: state, Logs: logs}, nil
}

// StartAutoSync starts the one in-process scheduler. A disabled configuration
// consumes no timer and waits only for a settings change or shutdown.
func (s *Server) StartAutoSync() {
	s.autoMu.Lock()
	if s.autoStarted {
		s.autoMu.Unlock()
		return
	}
	ctx, cancel := context.WithCancel(context.Background())
	s.autoStarted = true
	s.autoWake = make(chan struct{}, 1)
	s.autoCancel = cancel
	s.autoDone = make(chan struct{})
	done := s.autoDone
	s.autoMu.Unlock()

	go func() {
		defer close(done)
		s.autoSyncLoop(ctx)
	}()
}

// StopAutoSync waits for an in-flight run so the Store can be closed safely.
func (s *Server) StopAutoSync() {
	s.autoMu.Lock()
	if !s.autoStarted {
		s.autoMu.Unlock()
		return
	}
	cancel := s.autoCancel
	done := s.autoDone
	s.autoStarted = false
	s.autoMu.Unlock()
	cancel()
	<-done

	s.autoMu.Lock()
	s.autoRunning = false
	s.autoNextRun = ""
	s.autoCancel = nil
	s.autoDone = nil
	s.autoWake = nil
	s.autoMu.Unlock()
}

func (s *Server) wakeAutoSync() {
	s.autoMu.Lock()
	wake := s.autoWake
	started := s.autoStarted
	s.autoMu.Unlock()
	if !started || wake == nil {
		return
	}
	select {
	case wake <- struct{}{}:
	default:
	}
}

func (s *Server) autoSyncLoop(ctx context.Context) {
	for {
		cfg, err := s.Store.AutoSyncConfig()
		if err != nil {
			log.Printf("auto sync config: %v", err)
			s.setAutoSyncState(false, "")
			if !s.waitAutoSync(ctx, time.Minute) {
				return
			}
			continue
		}
		if !cfg.Enabled {
			s.setAutoSyncState(false, "")
			if !s.waitAutoSync(ctx, 0) {
				return
			}
			continue
		}

		delay := randomizedAutoSyncDelay(cfg)
		next := time.Now().UTC().Add(delay).Format(time.RFC3339)
		s.setAutoSyncState(false, next)
		if !s.waitAutoSync(ctx, delay) {
			return
		}

		// A settings edit also wakes the loop. Only run if the scheduled time
		// really arrived; otherwise reload the edited configuration immediately.
		s.autoMu.Lock()
		stillScheduled := s.autoNextRun == next
		s.autoMu.Unlock()
		if !stillScheduled {
			continue
		}

		s.setAutoSyncState(true, "")
		entry := s.runAutoSyncOnce()
		if entry.Error != "" {
			log.Printf("auto sync failed: %s", entry.Error)
		}
		s.setAutoSyncState(false, "")
	}
}

// waitAutoSync returns true both when a timer fires and when settings wake the
// loop. The caller distinguishes them via the published next-run timestamp.
// A zero delay means wait indefinitely for a settings change.
func (s *Server) waitAutoSync(ctx context.Context, delay time.Duration) bool {
	s.autoMu.Lock()
	wake := s.autoWake
	s.autoMu.Unlock()
	if delay <= 0 {
		select {
		case <-ctx.Done():
			return false
		case <-wake:
			return true
		}
	}
	timer := time.NewTimer(delay)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return false
	case <-wake:
		s.setAutoSyncState(false, "")
		return true
	case <-timer.C:
		return true
	}
}

func (s *Server) setAutoSyncState(running bool, next string) {
	s.autoMu.Lock()
	s.autoRunning = running
	s.autoNextRun = next
	s.autoMu.Unlock()
}

func randomizedAutoSyncDelay(cfg store.AutoSyncConfig) time.Duration {
	baseSeconds := int64(cfg.IntervalMinutes) * 60
	jitterSeconds := int64(cfg.JitterMinutes) * 60
	if jitterSeconds == 0 {
		return time.Duration(baseSeconds) * time.Second
	}
	span := jitterSeconds*2 + 1
	sampled, err := rand.Int(rand.Reader, big.NewInt(span))
	if err != nil {
		return time.Duration(baseSeconds) * time.Second
	}
	seconds := baseSeconds - jitterSeconds + sampled.Int64()
	return time.Duration(seconds) * time.Second
}

// runAutoSyncOnce performs exactly the requested chain under the same CPA
// mutex as interactive actions: refresh models, accept every naming
// suggestion, then save. It never creates a site enable/disable operation.
func (s *Server) runAutoSyncOnce() store.AutoSyncLog {
	entry := store.AutoSyncLog{StartedAt: time.Now().UTC().Format(time.RFC3339), Status: "success"}

	s.mu.Lock()
	refresh, err := s.refreshLocked(nil, nil, false)
	if err == nil {
		entry.Refreshed = refresh.Refreshed
		entry.Added = refresh.Added
		entry.Dropped = refresh.Dropped
		entry.Failed = len(refresh.Failed)
		for _, failure := range refresh.Failed {
			entry.Failures = append(entry.Failures, failure.Name+"："+failure.Error)
		}

		ops, suggestions := suggestionOps(refresh.View)
		entry.Suggested = suggestions
		var saved saveResponse
		saved, _, err = s.saveLocked(saveRequest{
			Fingerprint: refresh.View.Fingerprint,
			Ops:         ops,
			Note:        "auto-model-sync",
		}, false)
		if err == nil {
			entry.Renamed = saved.Renamed
			entry.Removed = saved.Removed
			entry.Restored = saved.Restored
			entry.Moved = saved.Moved
			entry.Written = saved.Written
			entry.Snapshot = saved.Snapshot
		}
	}
	s.mu.Unlock()

	if err != nil {
		entry.Status = "error"
		entry.Error = err.Error()
	} else if entry.Failed > 0 {
		entry.Status = "partial"
	}
	entry.FinishedAt = time.Now().UTC().Format(time.RFC3339)
	if id, logErr := s.Store.AddAutoSyncLog(entry); logErr != nil {
		log.Printf("write auto sync log: %v", logErr)
	} else {
		entry.ID = id
		_ = s.Store.PruneAutoSyncLogs(autoSyncLogRetain)
	}
	return entry
}

// suggestionOps groups equal suggested names into the same precise-target
// rename operation, matching the UI's draft representation.
func suggestionOps(view catalog.View) ([]catalog.Op, int) {
	byName := make(map[string][]catalog.EntryRef)
	count := 0
	for _, model := range view.Models {
		name := strings.TrimSpace(model.Suggested)
		if name == "" {
			continue
		}
		byName[name] = append(byName[name], catalog.EntryRef{Site: model.Site, Upstream: model.Upstream})
		count++
	}
	names := make([]string, 0, len(byName))
	for name := range byName {
		names = append(names, name)
	}
	sort.Strings(names)
	ops := make([]catalog.Op, 0, len(names))
	for _, name := range names {
		ops = append(ops, catalog.Op{Type: catalog.OpRename, Targets: byName[name], To: name})
	}
	return ops, count
}
