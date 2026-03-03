package handlers

import (
	"context"
	_ "embed"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"net/url"
	"time"

	"github.com/tiwillia/sankey-scorecard/pkg/config"
	"github.com/tiwillia/sankey-scorecard/pkg/reconciler"
	"github.com/tiwillia/sankey-scorecard/pkg/scorecard"
)

//go:embed openapi.yaml
var openAPISpec []byte

type apiHandler struct {
	cfg        *config.ResourceMap
	store      reconciler.DataStore
	reconciler *reconciler.Reconciler
}

type errorResponse struct {
	Error   string `json:"error"`
	Message string `json:"message"`
}

func writeJSON(w http.ResponseWriter, status int, v interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(v) //nolint:errcheck
}

func writeError(w http.ResponseWriter, status int, code string, message string) {
	writeJSON(w, status, errorResponse{Error: code, Message: message})
}

func (h *apiHandler) handleHealthz(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (h *apiHandler) handleGetOpenAPI(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	w.Write(openAPISpec) //nolint:errcheck
}

func (h *apiHandler) handleGetScorecard(w http.ResponseWriter, r *http.Request) {
	if !h.store.HasData() {
		writeError(w, http.StatusServiceUnavailable, "no_data", "No data available. Run 'refresh-data' first.")
		return
	}

	query := r.URL.Query()
	orgParam := query.Get("org")
	pillarParam := query.Get("pillar")
	teamParam := query.Get("team")

	// Parse filter options
	filter, err := parseFilterOptions(query)
	if err != nil {
		writeError(w, http.StatusBadRequest, "bad_request", err.Error())
		return
	}

	// Compute full scorecard
	fs := scorecard.ComputeFullScorecard(h.cfg, h.store, filter)

	// If no filters, return full scorecard
	if orgParam == "" && pillarParam == "" && teamParam == "" {
		writeJSON(w, http.StatusOK, fs)
		return
	}

	// Filter by parameters
	filtered := h.filterScorecard(fs, orgParam, pillarParam, teamParam)
	if filtered == nil {
		writeError(w, http.StatusNotFound, "not_found", "Specified entity not found")
		return
	}

	writeJSON(w, http.StatusOK, filtered)
}

func (h *apiHandler) filterScorecard(fs scorecard.FullScorecard, orgParam, pillarParam, teamParam string) *scorecard.FullScorecard {
	var filteredOrgs []scorecard.OrganizationScore

	for _, org := range fs.Organizations {
		if orgParam != "" && org.Identifier != orgParam {
			continue
		}

		var filteredPillars []scorecard.PillarScore
		for _, pillar := range org.Pillars {
			if pillarParam != "" && pillar.Identifier != pillarParam {
				continue
			}

			if teamParam != "" {
				var filteredTeams []scorecard.TeamScore
				for _, team := range pillar.Teams {
					if team.Identifier == teamParam {
						filteredTeams = append(filteredTeams, team)
					}
				}
				if len(filteredTeams) > 0 {
					p := pillar
					p.Teams = filteredTeams
					filteredPillars = append(filteredPillars, p)
				}
			} else {
				filteredPillars = append(filteredPillars, pillar)
			}
		}

		if len(filteredPillars) > 0 || (pillarParam == "" && teamParam == "") {
			o := org
			o.Pillars = filteredPillars
			filteredOrgs = append(filteredOrgs, o)
		}
	}

	if len(filteredOrgs) == 0 {
		return nil
	}

	result := fs
	result.Organizations = filteredOrgs
	return &result
}

func (h *apiHandler) handleRefreshData(w http.ResponseWriter, r *http.Request) {
	query := r.URL.Query()

	// Parse optional mode query parameter
	mode := query.Get("mode")
	if mode != "" && mode != "replace" && mode != "upsert" {
		writeError(w, http.StatusBadRequest, "bad_request", "invalid mode: must be \"replace\" or \"upsert\"")
		return
	}

	// Parse optional scope parameters (reuse the same parsing as scorecard filters)
	scopeFilter, err := parseFilterOptions(query)
	if err != nil {
		writeError(w, http.StatusBadRequest, "bad_request", err.Error())
		return
	}

	// Build RefreshScope if any scope param is set
	var scope *reconciler.RefreshScope
	if scopeFilter.StartDate != nil || scopeFilter.EndDate != nil || scopeFilter.IssueStatus != "" {
		scope = &reconciler.RefreshScope{
			StartDate: scopeFilter.StartDate,
			EndDate:   scopeFilter.EndDate,
			Status:    scopeFilter.IssueStatus,
		}
	}

	// Reject mode=replace with scope params -- scoped refresh must upsert
	if mode == "replace" && scope != nil {
		writeError(w, http.StatusBadRequest, "bad_request", "mode \"replace\" cannot be combined with scope parameters (start_date, end_date, status)")
		return
	}

	// StartRefresh atomically checks for a running refresh and transitions to "running"
	if !h.store.StartRefresh() {
		writeError(w, http.StatusConflict, "conflict", "A refresh is already in progress")
		return
	}

	// Apply refresh mode if specified
	if mode != "" {
		h.reconciler.SetRefreshMode(reconciler.RefreshMode(mode))
	}

	// Apply refresh scope if specified
	h.reconciler.SetRefreshScope(scope)

	// Run refresh asynchronously - state is already "running"
	go func() {
		if err := h.reconciler.ExecuteRefresh(context.Background()); err != nil {
			slog.Error("async refresh failed", "error", err)
		}
	}()

	writeJSON(w, http.StatusAccepted, h.store.GetState())
}

func (h *apiHandler) handleGetRefreshStatus(w http.ResponseWriter, r *http.Request) {
	state := h.store.GetState()
	writeJSON(w, http.StatusOK, state)
}

func parseFilterOptions(query url.Values) (scorecard.FilterOptions, error) {
	var filter scorecard.FilterOptions

	if s := query.Get("start_date"); s != "" {
		t, err := time.Parse("2006-01-02", s)
		if err != nil {
			return filter, fmt.Errorf("invalid start_date %q: must be YYYY-MM-DD", s)
		}
		filter.StartDate = &t
	}

	if s := query.Get("end_date"); s != "" {
		t, err := time.Parse("2006-01-02", s)
		if err != nil {
			return filter, fmt.Errorf("invalid end_date %q: must be YYYY-MM-DD", s)
		}
		filter.EndDate = &t
	}

	if s := query.Get("status"); s != "" {
		if s != "closed" && s != "in_progress" {
			return filter, fmt.Errorf("invalid status %q: must be \"closed\" or \"in_progress\"", s)
		}
		filter.IssueStatus = s
	}

	return filter, nil
}
