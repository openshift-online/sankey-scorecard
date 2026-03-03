//go:build integration

package tests_test

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
	"github.com/tiwillia/sankey-scorecard/pkg/scorecard"
)

func TestIntegration(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Integration Suite")
}

func loadTestConfig() *config.ResourceMap {
	cfg, err := config.LoadFromFile("testdata/resource-map.yaml")
	Expect(err).NotTo(HaveOccurred())
	return cfg
}

// teamAlphaIssues returns 10 fully categorized issues for team-alpha.
// Mix of statuses: 7 Closed + 2 In Progress + 1 Open
func teamAlphaIssues() []mockJiraIssue {
	return []mockJiraIssue{
		makeMockIssue("INTEG-1", "INTEG", "Story", "Associate Wellness & Development", "Closed", []string{"comp-alpha"}),
		makeMockIssue("INTEG-2", "INTEG", "Story", "Incidents & Escalations", "Closed", []string{"comp-alpha"}),
		makeMockIssue("INTEG-3", "INTEG", "Bug", "Security & Compliance", "Closed", []string{"comp-alpha"}),
		makeMockIssue("INTEG-4", "INTEG", "Story", "Tech Debt", "In Progress", []string{"comp-alpha"}),
		makeMockIssue("INTEG-5", "INTEG", "Story", "Defect", "Closed", []string{"comp-alpha"}),
		makeMockIssue("INTEG-6", "INTEG", "Task", "QE Activities", "Open", []string{"comp-alpha"}),
		makeMockIssue("INTEG-7", "INTEG", "Story", "Future Sustainability", "Closed", []string{"comp-alpha"}),
		makeMockIssue("INTEG-8", "INTEG", "Story", "Future Sustainability", "Closed", []string{"comp-alpha"}),
		makeMockIssue("INTEG-9", "INTEG", "Story", "Product / Portfolio Work", "Closed", []string{"comp-alpha"}),
		makeMockIssue("INTEG-10", "INTEG", "Story", "New Feature", "In Progress", []string{"comp-alpha"}),
	}
}

// teamBetaIssues returns 10 issues, 5 categorized, for team-beta.
// Mix of statuses: 4 Closed + 2 In Progress + 4 Open
func teamBetaIssues() []mockJiraIssue {
	return []mockJiraIssue{
		makeMockIssue("INTEG-11", "INTEG", "Story", "Tech Debt", "Closed", []string{"comp-beta"}),
		makeMockIssue("INTEG-12", "INTEG", "Story", "Defect", "Closed", []string{"comp-beta"}),
		makeMockIssue("INTEG-13", "INTEG", "Story", "New Feature", "In Progress", []string{"comp-beta"}),
		makeMockIssue("INTEG-14", "INTEG", "Story", "Feature Enhancement", "Closed", []string{"comp-beta"}),
		makeMockIssue("INTEG-15", "INTEG", "Story", "Product / Portfolio Work", "Open", []string{"comp-beta"}),
		makeMockIssue("INTEG-16", "INTEG", "Story", "", "Open", []string{"comp-beta"}),
		makeMockIssue("INTEG-17", "INTEG", "Bug", "", "Closed", []string{"comp-beta"}),
		makeMockIssue("INTEG-18", "INTEG", "Task", "", "Open", []string{"comp-beta"}),
		makeMockIssue("INTEG-19", "INTEG", "Story", "", "In Progress", []string{"comp-beta"}),
		makeMockIssue("INTEG-20", "INTEG", "Story", "", "Open", []string{"comp-beta"}),
	}
}

var _ = Describe("Integration Tests", func() {
	var (
		cfg        *config.ResourceMap
		store      *reconciler.ReconciliationStore
		mockJira   *mockJiraServer
		jiraClient *jira.Client
		rec        *reconciler.Reconciler
	)

	BeforeEach(func() {
		cfg = loadTestConfig()
		store = reconciler.NewReconciliationStore()
		mockJira = newMockJiraServer()

		// Set up mock data
		mockJira.SetIssues("comp-alpha", teamAlphaIssues())
		mockJira.SetIssues("comp-beta", teamBetaIssues())
		// Team Gamma: 0 issues (empty component)

		// Create a real go-jira client pointing at the mock server
		tp := &jira.PATAuthTransport{Token: "test-token"}
		httpClient := &http.Client{Transport: tp}
		var err error
		jiraClient, err = jira.NewClient(mockJira.URL(), httpClient)
		Expect(err).NotTo(HaveOccurred())

		rec = reconciler.NewReconciler(jiraClient.Issue, cfg, store, "customfield_123", nil)
	})

	AfterEach(func() {
		mockJira.Close()
	})

	Describe("Full reconciliation pipeline", func() {
		It("fetches and stores data for all teams", func() {
			err := rec.Refresh(context.Background())
			Expect(err).NotTo(HaveOccurred())

			Expect(store.HasData()).To(BeTrue())
			state := store.GetState()
			Expect(state.Status).To(Equal(reconciler.ReconciliationCompleted))

			// Team Alpha should have data: 2 closed + 1 in-progress = 3 periods
			tdAlpha, ok := store.GetTeamData("team-alpha")
			Expect(ok).To(BeTrue())
			Expect(tdAlpha.Periods).To(HaveLen(3))

			// Team Beta should have data: 3 periods
			tdBeta, ok := store.GetTeamData("team-beta")
			Expect(ok).To(BeTrue())
			Expect(tdBeta.Periods).To(HaveLen(3))

			// Team Gamma should have data (empty periods)
			tdGamma, ok := store.GetTeamData("team-gamma")
			Expect(ok).To(BeTrue())
			Expect(tdGamma.Periods).To(HaveLen(3))
		})

		It("separates closed and in-progress issue sets", func() {
			err := rec.Refresh(context.Background())
			Expect(err).NotTo(HaveOccurred())

			tdAlpha, _ := store.GetTeamData("team-alpha")

			closedCount := 0
			inProgressCount := 0
			for _, p := range tdAlpha.Periods {
				if p.SetType == reconciler.IssueSetClosed {
					closedCount++
				}
				if p.SetType == reconciler.IssueSetInProgress {
					inProgressCount++
				}
			}
			Expect(closedCount).To(Equal(2))
			Expect(inProgressCount).To(Equal(1))
		})

		It("correctly counts issues per set type for team-alpha", func() {
			err := rec.Refresh(context.Background())
			Expect(err).NotTo(HaveOccurred())

			tdAlpha, _ := store.GetTeamData("team-alpha")

			// Closed periods should only have closed issues (7 closed in alpha)
			for _, p := range tdAlpha.Periods {
				if p.SetType == reconciler.IssueSetClosed && p.TotalCount > 0 {
					Expect(p.TotalCount).To(Equal(7))
				}
			}

			// In-progress period should only have in-progress issues (2 in alpha)
			for _, p := range tdAlpha.Periods {
				if p.SetType == reconciler.IssueSetInProgress {
					Expect(p.TotalCount).To(Equal(2))
				}
			}
		})

		It("correctly computes activity distribution for team-alpha closed issues", func() {
			err := rec.Refresh(context.Background())
			Expect(err).NotTo(HaveOccurred())

			tdAlpha, _ := store.GetTeamData("team-alpha")
			for _, p := range tdAlpha.Periods {
				if p.SetType == reconciler.IssueSetClosed && p.TotalCount > 0 {
					// All 7 closed issues should be categorized
					Expect(p.CategorizedCount).To(Equal(p.TotalCount))
					Expect(p.Distribution.Uncategorized).To(Equal(0))
				}
			}
		})

		It("correctly computes team-beta issue counts by set", func() {
			err := rec.Refresh(context.Background())
			Expect(err).NotTo(HaveOccurred())

			tdBeta, _ := store.GetTeamData("team-beta")
			for _, p := range tdBeta.Periods {
				if p.SetType == reconciler.IssueSetClosed && p.TotalCount > 0 {
					// 4 closed issues in beta (INTEG-11,12,14,17)
					Expect(p.TotalCount).To(Equal(4))
				}
				if p.SetType == reconciler.IssueSetInProgress {
					// 2 in-progress issues in beta (INTEG-13,19)
					Expect(p.TotalCount).To(Equal(2))
				}
			}
		})

		It("correctly computes issue counts for team-gamma (empty)", func() {
			err := rec.Refresh(context.Background())
			Expect(err).NotTo(HaveOccurred())

			tdGamma, _ := store.GetTeamData("team-gamma")
			totalGamma := 0
			for _, p := range tdGamma.Periods {
				totalGamma += p.TotalCount
			}
			Expect(totalGamma).To(Equal(0))
		})
	})

	Describe("Scorecard computation after reconciliation", func() {
		BeforeEach(func() {
			err := rec.Refresh(context.Background())
			Expect(err).NotTo(HaveOccurred())
		})

		It("computes team-alpha score with sections", func() {
			fs := scorecard.ComputeFullScorecard(cfg, store, scorecard.FilterOptions{})

			var teamAlpha scorecard.TeamScore
			for _, org := range fs.Organizations {
				for _, p := range org.Pillars {
					for _, t := range p.Teams {
						if t.Identifier == "team-alpha" {
							teamAlpha = t
						}
					}
				}
			}

			Expect(teamAlpha.Score.Total).NotTo(BeNil())
			Expect(teamAlpha.Score.Grade).To(BeElementOf("A", "B"))

			// Should have two sections: closed and in-progress
			Expect(teamAlpha.Sections).To(HaveLen(2))
			Expect(teamAlpha.Sections[0].Type).To(Equal("closed"))
			Expect(teamAlpha.Sections[1].Type).To(Equal("in_progress"))
		})

		It("computes team-gamma with nil score (0 issues)", func() {
			fs := scorecard.ComputeFullScorecard(cfg, store, scorecard.FilterOptions{})

			var teamGamma scorecard.TeamScore
			for _, org := range fs.Organizations {
				for _, p := range org.Pillars {
					for _, t := range p.Teams {
						if t.Identifier == "team-gamma" {
							teamGamma = t
						}
					}
				}
			}

			Expect(teamGamma.Score.Total).To(BeNil())
			Expect(teamGamma.Score.Grade).To(Equal("-"))
			Expect(teamGamma.Score.IssueCount).To(Equal(0))
		})

		It("computes pillar-one score as weighted average of alpha and beta", func() {
			fs := scorecard.ComputeFullScorecard(cfg, store, scorecard.FilterOptions{})

			var pillarOne scorecard.PillarScore
			for _, org := range fs.Organizations {
				for _, p := range org.Pillars {
					if p.Identifier == "pillar-one" {
						pillarOne = p
					}
				}
			}

			Expect(pillarOne.Score.Total).NotTo(BeNil())
			var alphaScore, betaScore float64
			for _, t := range pillarOne.Teams {
				if t.Identifier == "team-alpha" && t.Score.Total != nil {
					alphaScore = *t.Score.Total
				}
				if t.Identifier == "team-beta" && t.Score.Total != nil {
					betaScore = *t.Score.Total
				}
			}
			Expect(*pillarOne.Score.Total).To(BeNumerically(">=", betaScore))
			Expect(*pillarOne.Score.Total).To(BeNumerically("<=", alphaScore))
		})

		It("computes organization score aggregating both pillars", func() {
			fs := scorecard.ComputeFullScorecard(cfg, store, scorecard.FilterOptions{})

			Expect(fs.Organizations).To(HaveLen(1))
			org := fs.Organizations[0]
			Expect(org.Identifier).To(Equal("integ-org"))

			Expect(org.Score.Total).NotTo(BeNil())
			Expect(org.Score.IssueCount).To(BeNumerically(">", 0))
		})
	})

	Describe("API endpoint integration", func() {
		var router http.Handler

		BeforeEach(func() {
			router = handlers.NewRouter(cfg, store, rec)
		})

		It("returns 503 before any refresh", func() {
			req := httptest.NewRequest("GET", "/api/scorecard", nil)
			w := httptest.NewRecorder()
			router.ServeHTTP(w, req)

			Expect(w.Code).To(Equal(http.StatusServiceUnavailable))
		})

		It("returns 202 when initiating refresh", func() {
			req := httptest.NewRequest("POST", "/api/refresh_data", nil)
			w := httptest.NewRecorder()
			router.ServeHTTP(w, req)

			Expect(w.Code).To(Equal(http.StatusAccepted))
		})

		It("returns full scorecard after refresh completes", func() {
			err := rec.Refresh(context.Background())
			Expect(err).NotTo(HaveOccurred())

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
			err := rec.Refresh(context.Background())
			Expect(err).NotTo(HaveOccurred())

			req := httptest.NewRequest("GET", "/api/scorecard?team=team-alpha", nil)
			w := httptest.NewRecorder()
			router.ServeHTTP(w, req)

			Expect(w.Code).To(Equal(http.StatusOK))
		})

		It("returns 404 for non-existent team", func() {
			err := rec.Refresh(context.Background())
			Expect(err).NotTo(HaveOccurred())

			req := httptest.NewRequest("GET", "/api/scorecard?team=nonexistent", nil)
			w := httptest.NewRecorder()
			router.ServeHTTP(w, req)

			Expect(w.Code).To(Equal(http.StatusNotFound))
		})

		It("returns refresh status", func() {
			req := httptest.NewRequest("GET", "/api/refresh_status", nil)
			w := httptest.NewRecorder()
			router.ServeHTTP(w, req)

			Expect(w.Code).To(Equal(http.StatusOK))
			var state map[string]interface{}
			Expect(json.Unmarshal(w.Body.Bytes(), &state)).To(Succeed())
			Expect(state["status"]).To(Equal("idle"))
		})

		It("returns completed status after refresh", func() {
			err := rec.Refresh(context.Background())
			Expect(err).NotTo(HaveOccurred())

			req := httptest.NewRequest("GET", "/api/refresh_status", nil)
			w := httptest.NewRecorder()
			router.ServeHTTP(w, req)

			Expect(w.Code).To(Equal(http.StatusOK))
			var state map[string]interface{}
			Expect(json.Unmarshal(w.Body.Bytes(), &state)).To(Succeed())
			Expect(state["status"]).To(Equal("completed"))
		})
	})

	Describe("Refresh error handling", func() {
		It("marks refresh as failed when Jira returns errors", func() {
			mockJira.SetError("comp-beta")

			err := rec.Refresh(context.Background())
			Expect(err).To(HaveOccurred())

			state := store.GetState()
			Expect(state.Status).To(Equal(reconciler.ReconciliationFailed))
		})

		It("preserves previous data after failed refresh", func() {
			// First successful refresh
			err := rec.Refresh(context.Background())
			Expect(err).NotTo(HaveOccurred())
			Expect(store.HasData()).To(BeTrue())

			// Corrupt the mock to fail for team-alpha
			mockJira.SetError("comp-alpha")

			// Second refresh should fail
			rec2 := reconciler.NewReconciler(jiraClient.Issue, cfg, store, "customfield_123", nil)
			err = rec2.Refresh(context.Background())
			Expect(err).To(HaveOccurred())

			// Previous data should still be available
			Expect(store.HasData()).To(BeTrue())
			_, ok := store.GetTeamData("team-alpha")
			Expect(ok).To(BeTrue())
		})
	})

	Describe("Concurrent refresh rejection", func() {
		It("rejects second refresh while first is running", func() {
			store.StartRefresh()

			router := handlers.NewRouter(cfg, store, rec)
			req := httptest.NewRequest("POST", "/api/refresh_data", nil)
			w := httptest.NewRecorder()
			router.ServeHTTP(w, req)

			Expect(w.Code).To(Equal(http.StatusConflict))
			var errResp map[string]string
			Expect(json.Unmarshal(w.Body.Bytes(), &errResp)).To(Succeed())
			Expect(errResp["error"]).To(Equal("conflict"))
		})

		It("rejects second reconciler refresh while first is running", func() {
			store.StartRefresh()

			rec2 := reconciler.NewReconciler(jiraClient.Issue, cfg, store, "customfield_123", nil)
			err := rec2.Refresh(context.Background())
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("already running"))
		})
	})

	Describe("Async refresh via API", func() {
		It("runs refresh asynchronously and completes", func() {
			router := handlers.NewRouter(cfg, store, rec)

			req := httptest.NewRequest("POST", "/api/refresh_data", nil)
			w := httptest.NewRecorder()
			router.ServeHTTP(w, req)
			Expect(w.Code).To(Equal(http.StatusAccepted))

			Eventually(func() string {
				req := httptest.NewRequest("GET", "/api/refresh_status", nil)
				w := httptest.NewRecorder()
				router.ServeHTTP(w, req)
				var state map[string]interface{}
				json.Unmarshal(w.Body.Bytes(), &state) //nolint:errcheck
				if s, ok := state["status"].(string); ok {
					return s
				}
				return ""
			}, 10*time.Second, 100*time.Millisecond).Should(Equal("completed"))

			req = httptest.NewRequest("GET", "/api/scorecard", nil)
			w = httptest.NewRecorder()
			router.ServeHTTP(w, req)
			Expect(w.Code).To(Equal(http.StatusOK))
		})
	})
})
