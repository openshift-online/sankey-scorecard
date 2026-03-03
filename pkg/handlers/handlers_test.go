package handlers_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	jira "github.com/andygrunwald/go-jira/v2/onpremise"

	"github.com/tiwillia/sankey-scorecard/pkg/config"
	"github.com/tiwillia/sankey-scorecard/pkg/handlers"
	"github.com/tiwillia/sankey-scorecard/pkg/reconciler"
)

func TestHandlers(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Handlers Suite")
}

type mockJiraClient struct {
	searchFunc func(ctx context.Context, jql string, opts *jira.SearchOptions) ([]jira.Issue, *jira.Response, error)
}

func (m *mockJiraClient) Search(ctx context.Context, jql string, opts *jira.SearchOptions) ([]jira.Issue, *jira.Response, error) {
	if m.searchFunc != nil {
		return m.searchFunc(ctx, jql, opts)
	}
	return nil, &jira.Response{Response: &http.Response{StatusCode: 200}, Total: 0}, nil
}

func makeTestConfig() *config.ResourceMap {
	return &config.ResourceMap{
		SprintReferenceDate: "2026-02-11",
		SprintDurationDays:  21,
		Jira: config.JiraConfig{
			ScoredIssueTypes:   []string{"Story", "Bug", "Task"},
			RequestDelayMs:     0,
			InProgressStatuses: []string{"In Progress", "Code Review", "Review"},
		},
		Organizations: []config.Organization{
			{
				Name:       "Test Org",
				Identifier: "test-org",
				Pillars: []config.Pillar{
					{
						Name:       "Test Pillar",
						Identifier: "test-pillar",
						Teams: []config.Team{
							{
								Name:       "Team Alpha",
								Identifier: "team-alpha",
								Ownership: config.Ownership{
									Method:     "component",
									Project:    "PROJ",
									Components: []string{"comp-a"},
								},
							},
						},
					},
				},
			},
		},
	}
}

func populateStore(store *reconciler.ReconciliationStore) {
	populateStoreWithPeriods(store)
}

func populateStoreWithPeriods(store *reconciler.ReconciliationStore) {
	teams := map[string]*reconciler.TeamData{
		"team-alpha": {
			TeamIdentifier: "team-alpha",
			ReconciledAt:   time.Now(),
			Periods: []reconciler.ScoringPeriod{
				{
					Label:            "Jan Sprint",
					SetType:          reconciler.IssueSetClosed,
					Window:           reconciler.TimeWindow{Since: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC), Until: time.Date(2026, 1, 21, 0, 0, 0, 0, time.UTC)},
					TotalCount:       5,
					CategorizedCount: 4,
					Distribution: reconciler.ActivityDistribution{
						IncidentsSupport:     1,
						SecurityCompliance:   1,
						QualityStability:     1,
						FutureSustainability: 1,
						Uncategorized:        1,
					},
				},
				{
					Label:            "Feb Sprint",
					SetType:          reconciler.IssueSetClosed,
					Window:           reconciler.TimeWindow{Since: time.Date(2026, 2, 1, 0, 0, 0, 0, time.UTC), Until: time.Date(2026, 2, 21, 0, 0, 0, 0, time.UTC)},
					TotalCount:       5,
					CategorizedCount: 4,
					Distribution: reconciler.ActivityDistribution{
						AssociateWellness:    1,
						QualityStability:     1,
						FutureSustainability: 1,
						ProductPortfolio:     1,
						Uncategorized:        1,
					},
				},
				{
					Label:            "In Progress",
					SetType:          reconciler.IssueSetInProgress,
					Current:          true,
					Window:           reconciler.TimeWindow{Since: time.Date(2026, 2, 11, 0, 0, 0, 0, time.UTC), Until: time.Date(2026, 3, 4, 0, 0, 0, 0, time.UTC)},
					TotalCount:       3,
					CategorizedCount: 3,
					Distribution: reconciler.ActivityDistribution{
						IncidentsSupport: 1,
						QualityStability: 1,
						ProductPortfolio: 1,
					},
				},
			},
		},
	}
	store.SwapData(teams, 13)
}

var _ = Describe("API Handlers", func() {
	var (
		cfg    *config.ResourceMap
		store  *reconciler.ReconciliationStore
		client *mockJiraClient
		rec    *reconciler.Reconciler
		router http.Handler
	)

	BeforeEach(func() {
		cfg = makeTestConfig()
		store = reconciler.NewReconciliationStore()
		client = &mockJiraClient{}
		rec = reconciler.NewReconciler(client, cfg, store, "customfield_123", nil)
		router = handlers.NewRouter(cfg, store, rec)
	})

	Describe("GET /healthz", func() {
		It("returns 200 OK", func() {
			req := httptest.NewRequest("GET", "/healthz", nil)
			w := httptest.NewRecorder()
			router.ServeHTTP(w, req)

			Expect(w.Code).To(Equal(http.StatusOK))
			Expect(w.Header().Get("Content-Type")).To(Equal("application/json"))

			var resp map[string]string
			Expect(json.Unmarshal(w.Body.Bytes(), &resp)).To(Succeed())
			Expect(resp["status"]).To(Equal("ok"))
		})
	})

	Describe("GET /api/", func() {
		It("returns the OpenAPI spec", func() {
			req := httptest.NewRequest("GET", "/api/", nil)
			w := httptest.NewRecorder()
			router.ServeHTTP(w, req)

			Expect(w.Code).To(Equal(http.StatusOK))
			Expect(w.Header().Get("Content-Type")).To(Equal("application/json"))
			Expect(w.Body.String()).To(ContainSubstring("openapi"))
		})
	})

	Describe("GET /api/scorecard", func() {
		It("returns 503 when no data is available", func() {
			req := httptest.NewRequest("GET", "/api/scorecard", nil)
			w := httptest.NewRecorder()
			router.ServeHTTP(w, req)

			Expect(w.Code).To(Equal(http.StatusServiceUnavailable))
			var errResp map[string]string
			Expect(json.Unmarshal(w.Body.Bytes(), &errResp)).To(Succeed())
			Expect(errResp["error"]).To(Equal("no_data"))
		})

		It("returns 200 with full scorecard when data is available", func() {
			populateStore(store)

			req := httptest.NewRequest("GET", "/api/scorecard", nil)
			w := httptest.NewRecorder()
			router.ServeHTTP(w, req)

			Expect(w.Code).To(Equal(http.StatusOK))
			Expect(w.Header().Get("Content-Type")).To(Equal("application/json"))

			var result map[string]interface{}
			Expect(json.Unmarshal(w.Body.Bytes(), &result)).To(Succeed())
			Expect(result["organizations"]).NotTo(BeNil())
		})

		It("returns filtered results for team parameter", func() {
			populateStore(store)

			req := httptest.NewRequest("GET", "/api/scorecard?team=team-alpha", nil)
			w := httptest.NewRecorder()
			router.ServeHTTP(w, req)

			Expect(w.Code).To(Equal(http.StatusOK))
		})

		It("returns 404 for non-existent team", func() {
			populateStore(store)

			req := httptest.NewRequest("GET", "/api/scorecard?team=nonexistent", nil)
			w := httptest.NewRecorder()
			router.ServeHTTP(w, req)

			Expect(w.Code).To(Equal(http.StatusNotFound))
		})

		It("returns filtered results for org parameter", func() {
			populateStore(store)

			req := httptest.NewRequest("GET", "/api/scorecard?org=test-org", nil)
			w := httptest.NewRecorder()
			router.ServeHTTP(w, req)

			Expect(w.Code).To(Equal(http.StatusOK))
		})

		It("returns 404 for non-existent org", func() {
			populateStore(store)

			req := httptest.NewRequest("GET", "/api/scorecard?org=nonexistent", nil)
			w := httptest.NewRecorder()
			router.ServeHTTP(w, req)

			Expect(w.Code).To(Equal(http.StatusNotFound))
		})

		It("returns 400 for invalid start_date", func() {
			populateStore(store)

			req := httptest.NewRequest("GET", "/api/scorecard?start_date=not-a-date", nil)
			w := httptest.NewRecorder()
			router.ServeHTTP(w, req)

			Expect(w.Code).To(Equal(http.StatusBadRequest))
			var errResp map[string]string
			Expect(json.Unmarshal(w.Body.Bytes(), &errResp)).To(Succeed())
			Expect(errResp["error"]).To(Equal("bad_request"))
			Expect(errResp["message"]).To(ContainSubstring("start_date"))
		})

		It("returns 400 for invalid end_date", func() {
			populateStore(store)

			req := httptest.NewRequest("GET", "/api/scorecard?end_date=12/31/2026", nil)
			w := httptest.NewRecorder()
			router.ServeHTTP(w, req)

			Expect(w.Code).To(Equal(http.StatusBadRequest))
			var errResp map[string]string
			Expect(json.Unmarshal(w.Body.Bytes(), &errResp)).To(Succeed())
			Expect(errResp["message"]).To(ContainSubstring("end_date"))
		})

		It("returns 400 for invalid status value", func() {
			populateStore(store)

			req := httptest.NewRequest("GET", "/api/scorecard?status=invalid", nil)
			w := httptest.NewRecorder()
			router.ServeHTTP(w, req)

			Expect(w.Code).To(Equal(http.StatusBadRequest))
			var errResp map[string]string
			Expect(json.Unmarshal(w.Body.Bytes(), &errResp)).To(Succeed())
			Expect(errResp["message"]).To(ContainSubstring("status"))
		})

		It("filters by status=closed returns only closed period data", func() {
			populateStore(store)

			req := httptest.NewRequest("GET", "/api/scorecard?status=closed", nil)
			w := httptest.NewRecorder()
			router.ServeHTTP(w, req)

			Expect(w.Code).To(Equal(http.StatusOK))
			var result map[string]interface{}
			Expect(json.Unmarshal(w.Body.Bytes(), &result)).To(Succeed())

			// Verify active_filters is set
			filters, ok := result["active_filters"].(map[string]interface{})
			Expect(ok).To(BeTrue())
			Expect(filters["issue_status"]).To(Equal("closed"))

			// Verify team periods only contain closed
			orgs := result["organizations"].([]interface{})
			org := orgs[0].(map[string]interface{})
			pillars := org["pillars"].([]interface{})
			pillar := pillars[0].(map[string]interface{})
			teams := pillar["teams"].([]interface{})
			team := teams[0].(map[string]interface{})
			periods := team["periods"].([]interface{})
			for _, p := range periods {
				period := p.(map[string]interface{})
				Expect(period["set_type"]).To(Equal("closed"))
			}
		})

		It("filters by date range returns matching periods", func() {
			populateStore(store)

			req := httptest.NewRequest("GET", "/api/scorecard?start_date=2026-02-01&end_date=2026-02-28", nil)
			w := httptest.NewRecorder()
			router.ServeHTTP(w, req)

			Expect(w.Code).To(Equal(http.StatusOK))
			var result map[string]interface{}
			Expect(json.Unmarshal(w.Body.Bytes(), &result)).To(Succeed())

			filters, ok := result["active_filters"].(map[string]interface{})
			Expect(ok).To(BeTrue())
			Expect(filters["start_date"]).To(Equal("2026-02-01"))
			Expect(filters["end_date"]).To(Equal("2026-02-28"))
		})

		It("no filter params returns same results as before", func() {
			populateStore(store)

			req := httptest.NewRequest("GET", "/api/scorecard", nil)
			w := httptest.NewRecorder()
			router.ServeHTTP(w, req)

			Expect(w.Code).To(Equal(http.StatusOK))
			var result map[string]interface{}
			Expect(json.Unmarshal(w.Body.Bytes(), &result)).To(Succeed())
			// No active_filters when no filters are set
			Expect(result["active_filters"]).To(BeNil())
		})
	})

	Describe("POST /api/refresh_data", func() {
		It("returns 202 with running status when refresh is initiated", func() {
			client.searchFunc = func(ctx context.Context, jql string, opts *jira.SearchOptions) ([]jira.Issue, *jira.Response, error) {
				return nil, &jira.Response{Response: &http.Response{StatusCode: 200}, Total: 0}, nil
			}

			req := httptest.NewRequest("POST", "/api/refresh_data", nil)
			w := httptest.NewRecorder()
			router.ServeHTTP(w, req)

			Expect(w.Code).To(Equal(http.StatusAccepted))
			var state map[string]interface{}
			Expect(json.Unmarshal(w.Body.Bytes(), &state)).To(Succeed())
			Expect(state["status"]).To(Equal("running"))
		})

		It("returns 409 when refresh is already running", func() {
			store.StartRefresh()

			req := httptest.NewRequest("POST", "/api/refresh_data", nil)
			w := httptest.NewRecorder()
			router.ServeHTTP(w, req)

			Expect(w.Code).To(Equal(http.StatusConflict))
			var errResp map[string]string
			Expect(json.Unmarshal(w.Body.Bytes(), &errResp)).To(Succeed())
			Expect(errResp["error"]).To(Equal("conflict"))
		})

		It("returns 202 with valid scope params", func() {
			client.searchFunc = func(ctx context.Context, jql string, opts *jira.SearchOptions) ([]jira.Issue, *jira.Response, error) {
				return nil, &jira.Response{Response: &http.Response{StatusCode: 200}, Total: 0}, nil
			}

			req := httptest.NewRequest("POST", "/api/refresh_data?status=closed&start_date=2026-01-01&end_date=2026-02-28", nil)
			w := httptest.NewRecorder()
			router.ServeHTTP(w, req)

			Expect(w.Code).To(Equal(http.StatusAccepted))
		})

		It("returns 400 when mode=replace is combined with scope params", func() {
			req := httptest.NewRequest("POST", "/api/refresh_data?mode=replace&status=closed", nil)
			w := httptest.NewRecorder()
			router.ServeHTTP(w, req)

			Expect(w.Code).To(Equal(http.StatusBadRequest))
			var errResp map[string]string
			Expect(json.Unmarshal(w.Body.Bytes(), &errResp)).To(Succeed())
			Expect(errResp["error"]).To(Equal("bad_request"))
			Expect(errResp["message"]).To(ContainSubstring("replace"))
		})

		It("returns 400 for invalid start_date", func() {
			req := httptest.NewRequest("POST", "/api/refresh_data?start_date=not-a-date", nil)
			w := httptest.NewRecorder()
			router.ServeHTTP(w, req)

			Expect(w.Code).To(Equal(http.StatusBadRequest))
			var errResp map[string]string
			Expect(json.Unmarshal(w.Body.Bytes(), &errResp)).To(Succeed())
			Expect(errResp["message"]).To(ContainSubstring("start_date"))
		})

		It("returns 400 for invalid status value", func() {
			req := httptest.NewRequest("POST", "/api/refresh_data?status=invalid", nil)
			w := httptest.NewRecorder()
			router.ServeHTTP(w, req)

			Expect(w.Code).To(Equal(http.StatusBadRequest))
			var errResp map[string]string
			Expect(json.Unmarshal(w.Body.Bytes(), &errResp)).To(Succeed())
			Expect(errResp["message"]).To(ContainSubstring("status"))
		})

		It("returns 202 with mode=upsert and scope params", func() {
			client.searchFunc = func(ctx context.Context, jql string, opts *jira.SearchOptions) ([]jira.Issue, *jira.Response, error) {
				return nil, &jira.Response{Response: &http.Response{StatusCode: 200}, Total: 0}, nil
			}

			req := httptest.NewRequest("POST", "/api/refresh_data?mode=upsert&status=closed", nil)
			w := httptest.NewRecorder()
			router.ServeHTTP(w, req)

			Expect(w.Code).To(Equal(http.StatusAccepted))
		})

		It("returns 202 with scope but no explicit mode", func() {
			client.searchFunc = func(ctx context.Context, jql string, opts *jira.SearchOptions) ([]jira.Issue, *jira.Response, error) {
				return nil, &jira.Response{Response: &http.Response{StatusCode: 200}, Total: 0}, nil
			}

			req := httptest.NewRequest("POST", "/api/refresh_data?status=in_progress", nil)
			w := httptest.NewRecorder()
			router.ServeHTTP(w, req)

			Expect(w.Code).To(Equal(http.StatusAccepted))
		})
	})

	Describe("GET /api/refresh_status", func() {
		It("returns idle status initially", func() {
			req := httptest.NewRequest("GET", "/api/refresh_status", nil)
			w := httptest.NewRecorder()
			router.ServeHTTP(w, req)

			Expect(w.Code).To(Equal(http.StatusOK))
			var state map[string]interface{}
			Expect(json.Unmarshal(w.Body.Bytes(), &state)).To(Succeed())
			Expect(state["status"]).To(Equal("idle"))
		})

		It("returns running status after refresh start", func() {
			store.StartRefresh()

			req := httptest.NewRequest("GET", "/api/refresh_status", nil)
			w := httptest.NewRecorder()
			router.ServeHTTP(w, req)

			Expect(w.Code).To(Equal(http.StatusOK))
			var state map[string]interface{}
			Expect(json.Unmarshal(w.Body.Bytes(), &state)).To(Succeed())
			Expect(state["status"]).To(Equal("running"))
		})

		It("returns completed status after successful refresh", func() {
			store.StartRefresh()
			store.CompleteRefresh(42)

			req := httptest.NewRequest("GET", "/api/refresh_status", nil)
			w := httptest.NewRecorder()
			router.ServeHTTP(w, req)

			Expect(w.Code).To(Equal(http.StatusOK))
			var state map[string]interface{}
			Expect(json.Unmarshal(w.Body.Bytes(), &state)).To(Succeed())
			Expect(state["status"]).To(Equal("completed"))
			Expect(state["issue_count"]).To(BeNumerically("==", 42))
		})
	})
})
