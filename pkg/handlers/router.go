package handlers

import (
	"net/http"

	"github.com/tiwillia/sankey-scorecard/frontend"
	"github.com/tiwillia/sankey-scorecard/pkg/config"
	"github.com/tiwillia/sankey-scorecard/pkg/reconciler"
)

// NewRouter creates the HTTP handler with all API routes registered.
func NewRouter(cfg *config.ResourceMap, store reconciler.DataStore, rec *reconciler.Reconciler) http.Handler {
	mux := http.NewServeMux()

	h := &apiHandler{
		cfg:        cfg,
		store:      store,
		reconciler: rec,
	}

	mux.Handle("GET /", http.FileServerFS(frontend.Content))
	mux.HandleFunc("GET /healthz", h.handleHealthz)
	mux.HandleFunc("GET /api/", h.handleGetOpenAPI)
	mux.HandleFunc("GET /api/scorecard", h.handleGetScorecard)
	mux.HandleFunc("POST /api/refresh_data", h.handleRefreshData)
	mux.HandleFunc("GET /api/refresh_status", h.handleGetRefreshStatus)

	return mux
}
