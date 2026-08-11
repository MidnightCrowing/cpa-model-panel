package main

import (
	"io"
	"io/fs"
	"log"
	"net/http"
	"os"
	"strings"

	"github.com/local/cpa-model-panel/internal/api"
	"github.com/local/cpa-model-panel/internal/config"
	"github.com/local/cpa-model-panel/internal/cpa"
	"github.com/local/cpa-model-panel/internal/store"
	"github.com/local/cpa-model-panel/web"
)

func main() {
	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("config: %v", err)
	}
	st, err := store.Open(cfg.DataDir)
	if err != nil {
		log.Fatalf("store: %v", err)
	}
	defer st.Close()

	srv := &api.Server{
		AdminToken: cfg.AdminToken,
		CPA:        cpa.NewClient(cfg.CPABaseURL, cfg.CPAManagementSecret),
		Store:      st,
		Retain:     cfg.SnapshotRetain,
	}

	mux := http.NewServeMux()
	srv.Routes(mux)
	mux.Handle("/", spaHandler())

	log.Printf("cpa-model-panel listening on %s (cpa=%s data=%s)", cfg.Listen, cfg.CPABaseURL, cfg.DataDir)
	if err := http.ListenAndServe(cfg.Listen, mux); err != nil {
		log.Fatal(err)
	}
}

func spaHandler() http.Handler {
	sub, err := fs.Sub(web.Dist, "dist")
	if err != nil {
		// fallback: no frontend built yet
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Path == "/" {
				w.Header().Set("Content-Type", "text/plain; charset=utf-8")
				_, _ = io.WriteString(w, "cpa-model-panel API is up. Build web/ frontend into web/dist.\n")
				return
			}
			http.NotFound(w, r)
		})
	}
	fileServer := http.FileServer(http.FS(sub))
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path := r.URL.Path
		// Asset filenames are content-hashed and may be cached hard; the shell
		// that points at them must not be, or a deploy goes unnoticed.
		if path == "/" || !strings.Contains(path, "/assets/") {
			w.Header().Set("Cache-Control", "no-store")
		} else {
			w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
		}
		if path == "/" {
			fileServer.ServeHTTP(w, r)
			return
		}
		// API already registered higher; here only static
		if strings.HasPrefix(path, "/api/") {
			http.NotFound(w, r)
			return
		}
		// try exact file
		trimmed := strings.TrimPrefix(path, "/")
		if trimmed != "" {
			if f, err := sub.Open(trimmed); err == nil {
				_ = f.Close()
				fileServer.ServeHTTP(w, r)
				return
			}
		}
		// SPA fallback
		r2 := r.Clone(r.Context())
		r2.URL.Path = "/"
		fileServer.ServeHTTP(w, r2)
	})
}

// ensure os import used in future hooks
var _ = os.Stderr
