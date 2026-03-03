//go:build integration

package tests_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	jira "github.com/andygrunwald/go-jira/v2/onpremise"

	"github.com/tiwillia/sankey-scorecard/pkg/config"
	"github.com/tiwillia/sankey-scorecard/pkg/handlers"
	"github.com/tiwillia/sankey-scorecard/pkg/reconciler"
	"github.com/tiwillia/sankey-scorecard/pkg/scorecard"
)

// rosaAuroraIssues returns 10 fully categorized issues for rosa-aurora.
// Mix of statuses: 7 Closed + 2 In Progress + 1 Open
func rosaAuroraIssues() []mockJiraIssue {
	return []mockJiraIssue{
		makeMockIssue("SREP-1", "SREP", "Story", "Associate Wellness & Development", "Closed", nil),
		makeMockIssue("SREP-2", "SREP", "Story", "Incidents & Escalations", "Closed", nil),
		makeMockIssue("SREP-3", "SREP", "Bug", "Security & Compliance", "Closed", nil),
		makeMockIssue("SREP-4", "SREP", "Story", "Tech Debt", "In Progress", nil),
		makeMockIssue("SREP-5", "SREP", "Story", "Defect", "Closed", nil),
		makeMockIssue("SREP-6", "SREP", "Task", "QE Activities", "Open", nil),
		makeMockIssue("SREP-7", "SREP", "Story", "Future Sustainability", "Closed", nil),
		makeMockIssue("SREP-8", "SREP", "Story", "Future Sustainability", "Closed", nil),
		makeMockIssue("SREP-9", "SREP", "Story", "Product / Portfolio Work", "Closed", nil),
		makeMockIssue("SREP-10", "SREP", "Story", "New Feature", "In Progress", nil),
	}
}

var _ = Describe("API Server Integration", func() {
	var (
		cfg        *config.ResourceMap
		store      *reconciler.ReconciliationStore
		mockJira   *mockJiraServer
		jiraClient *jira.Client
		rec        *reconciler.Reconciler
		server     *httptest.Server
		client     *http.Client
	)

	BeforeEach(func() {
		var err error
		cfg, err = config.LoadFromFile("testdata/api-resource-map.yaml")
		Expect(err).NotTo(HaveOccurred())

		store = reconciler.NewReconciliationStore()
		mockJira = newMockJiraServer()

		// Populate rosa-aurora issues keyed by team field value
		mockJira.SetIssues("5695", rosaAuroraIssues())

		// Create a real go-jira client pointing at the mock server
		tp := &jira.PATAuthTransport{Token: "test-token"}
		httpClient := &http.Client{Transport: tp}
		jiraClient, err = jira.NewClient(mockJira.URL(), httpClient)
		Expect(err).NotTo(HaveOccurred())

		rec = reconciler.NewReconciler(jiraClient.Issue, cfg, store, "customfield_123", nil)

		// Start a live HTTP server
		router := handlers.NewRouter(cfg, store, rec)
		server = httptest.NewServer(router)
		client = server.Client()
	})

	AfterEach(func() {
		server.Close()
		mockJira.Close()
	})

	It("returns 503 before refresh, then scorecard after refresh", func() {
		// 1. GET scorecard before any data exists → 503
		resp, err := client.Get(server.URL + "/api/scorecard?team=rosa-aurora")
		Expect(err).NotTo(HaveOccurred())
		resp.Body.Close()
		Expect(resp.StatusCode).To(Equal(http.StatusServiceUnavailable))

		// 2. POST refresh → 202
		resp, err = client.Post(server.URL+"/api/refresh_data", "", nil)
		Expect(err).NotTo(HaveOccurred())
		resp.Body.Close()
		Expect(resp.StatusCode).To(Equal(http.StatusAccepted))

		// 3. Poll refresh_status until completed
		Eventually(func() string {
			resp, err := client.Get(server.URL + "/api/refresh_status")
			if err != nil {
				return ""
			}
			defer resp.Body.Close()
			var state map[string]interface{}
			json.NewDecoder(resp.Body).Decode(&state) //nolint:errcheck
			if s, ok := state["status"].(string); ok {
				return s
			}
			return ""
		}, 10*time.Second, 100*time.Millisecond).Should(Equal("completed"))

		// 4. GET scorecard → 200
		resp, err = client.Get(server.URL + "/api/scorecard?team=rosa-aurora")
		Expect(err).NotTo(HaveOccurred())
		defer resp.Body.Close()
		Expect(resp.StatusCode).To(Equal(http.StatusOK))

		// 5. Decode the JSON response
		var fs scorecard.FullScorecard
		Expect(json.NewDecoder(resp.Body).Decode(&fs)).To(Succeed())

		// 6. Assert structure: one org (hcm) with one pillar (rosa) with one team (rosa-aurora)
		Expect(fs.Organizations).To(HaveLen(1))
		org := fs.Organizations[0]
		Expect(org.Identifier).To(Equal("hcm"))

		Expect(org.Pillars).To(HaveLen(1))
		pillar := org.Pillars[0]
		Expect(pillar.Identifier).To(Equal("rosa"))

		Expect(pillar.Teams).To(HaveLen(1))
		team := pillar.Teams[0]
		Expect(team.Identifier).To(Equal("rosa-aurora"))

		// 7. Assert rosa-aurora has a non-nil score, non-zero issue count, and valid grade
		Expect(team.Score.Total).NotTo(BeNil())
		Expect(team.Score.IssueCount).To(BeNumerically(">", 0))
		Expect(team.Score.Grade).To(BeElementOf("A", "B", "C", "D", "F"))
	})
})
