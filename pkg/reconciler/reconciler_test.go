package reconciler_test

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"testing"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	jira "github.com/andygrunwald/go-jira/v2/onpremise"

	"github.com/tiwillia/sankey-scorecard/pkg/config"
	"github.com/tiwillia/sankey-scorecard/pkg/reconciler"
)

func TestReconciler(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Reconciler Suite")
}

// mockJiraClient implements reconciler.JiraClient for testing.
type mockJiraClient struct {
	searchFunc func(ctx context.Context, jql string, opts *jira.SearchOptions) ([]jira.Issue, *jira.Response, error)
}

func (m *mockJiraClient) Search(ctx context.Context, jql string, opts *jira.SearchOptions) ([]jira.Issue, *jira.Response, error) {
	if m.searchFunc != nil {
		return m.searchFunc(ctx, jql, opts)
	}
	return nil, &jira.Response{Response: &http.Response{StatusCode: 200}, Total: 0}, nil
}

// mockSprintFetcher implements reconciler.BoardSprintFetcher for testing.
type mockSprintFetcher struct {
	sprints map[int][]reconciler.SprintInfo // boardID -> sprints
}

func (m *mockSprintFetcher) FetchBoardSprints(ctx context.Context, boardID int) ([]reconciler.SprintInfo, error) {
	if m.sprints != nil {
		return m.sprints[boardID], nil
	}
	return nil, nil
}

func makeTestConfig() *config.ResourceMap {
	return &config.ResourceMap{
		SprintReferenceDate: "2026-02-11",
		SprintDurationDays:  21,
		Jira: config.JiraConfig{
			ScoredIssueTypes:   []string{"Story", "Bug", "Task"},
			RequestDelayMs:     0, // no delay for tests
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

func makeJiraIssue(key, project, issueType, activityType string) jira.Issue {
	fields := &jira.IssueFields{
		Project: jira.Project{Key: project},
		Type:    jira.IssueType{Name: issueType},
		Status:  &jira.Status{Name: "Open"},
		Summary: "Test issue " + key,
	}
	if activityType != "" {
		fields.Unknowns = map[string]interface{}{
			"customfield_123": map[string]interface{}{
				"value": activityType,
			},
		}
	}
	return jira.Issue{
		Key:    key,
		Fields: fields,
	}
}

var _ = Describe("Reconciler", func() {
	var (
		store  *reconciler.ReconciliationStore
		client *mockJiraClient
		cfg    *config.ResourceMap
	)

	BeforeEach(func() {
		store = reconciler.NewReconciliationStore()
		client = &mockJiraClient{}
		cfg = makeTestConfig()
	})

	Describe("Refresh", func() {
		It("fetches and stores team data successfully", func() {
			client.searchFunc = func(ctx context.Context, jql string, opts *jira.SearchOptions) ([]jira.Issue, *jira.Response, error) {
				issues := []jira.Issue{
					makeJiraIssue("PROJ-1", "PROJ", "Story", "Tech Debt"),
					makeJiraIssue("PROJ-2", "PROJ", "Bug", "Security & Compliance"),
					makeJiraIssue("PROJ-3", "PROJ", "Task", ""),
				}
				return issues, &jira.Response{
					Response: &http.Response{StatusCode: 200},
					Total:    3,
				}, nil
			}

			r := reconciler.NewReconciler(client, cfg, store, "customfield_123", nil)
			err := r.Refresh(context.Background())
			Expect(err).NotTo(HaveOccurred())

			Expect(store.HasData()).To(BeTrue())
			state := store.GetState()
			Expect(state.Status).To(Equal(reconciler.ReconciliationCompleted))

			td, ok := store.GetTeamData("team-alpha")
			Expect(ok).To(BeTrue())
			// 2 closed cadences + 1 in-progress = 3 periods
			Expect(td.Periods).To(HaveLen(3))

			// Verify set types
			closedCount := 0
			inProgressCount := 0
			for _, p := range td.Periods {
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

		It("fails if already running", func() {
			store.StartRefresh()

			r := reconciler.NewReconciler(client, cfg, store, "customfield_123", nil)
			err := r.Refresh(context.Background())
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("already running"))
		})

		It("marks refresh as failed on Jira error", func() {
			client.searchFunc = func(ctx context.Context, jql string, opts *jira.SearchOptions) ([]jira.Issue, *jira.Response, error) {
				return nil, nil, fmt.Errorf("connection refused")
			}

			r := reconciler.NewReconciler(client, cfg, store, "customfield_123", nil)
			err := r.Refresh(context.Background())
			Expect(err).To(HaveOccurred())

			state := store.GetState()
			Expect(state.Status).To(Equal(reconciler.ReconciliationFailed))
		})

		It("handles pagination", func() {
			callCount := 0
			client.searchFunc = func(ctx context.Context, jql string, opts *jira.SearchOptions) ([]jira.Issue, *jira.Response, error) {
				callCount++
				if opts.StartAt == 0 {
					issues := []jira.Issue{
						makeJiraIssue("PROJ-1", "PROJ", "Story", "Tech Debt"),
					}
					return issues, &jira.Response{
						Response: &http.Response{StatusCode: 200},
						Total:    2,
					}, nil
				}
				issues := []jira.Issue{
					makeJiraIssue("PROJ-2", "PROJ", "Bug", ""),
				}
				return issues, &jira.Response{
					Response: &http.Response{StatusCode: 200},
					Total:    2,
				}, nil
			}

			r := reconciler.NewReconciler(client, cfg, store, "customfield_123", nil)
			err := r.Refresh(context.Background())
			Expect(err).NotTo(HaveOccurred())
			// Should have multiple calls due to pagination (for 3 periods)
			Expect(callCount).To(BeNumerically(">", 3))
		})

		It("correctly categorizes issues in distribution", func() {
			client.searchFunc = func(ctx context.Context, jql string, opts *jira.SearchOptions) ([]jira.Issue, *jira.Response, error) {
				issues := []jira.Issue{
					makeJiraIssue("PROJ-1", "PROJ", "Story", "Associate Wellness & Development"),
					makeJiraIssue("PROJ-2", "PROJ", "Story", "Incidents & Escalations"),
					makeJiraIssue("PROJ-3", "PROJ", "Story", "Security & Compliance"),
					makeJiraIssue("PROJ-4", "PROJ", "Story", "Tech Debt"),
					makeJiraIssue("PROJ-5", "PROJ", "Story", "Future Sustainability"),
					makeJiraIssue("PROJ-6", "PROJ", "Story", "New Feature"),
					makeJiraIssue("PROJ-7", "PROJ", "Story", ""),
				}
				return issues, &jira.Response{
					Response: &http.Response{StatusCode: 200},
					Total:    7,
				}, nil
			}

			r := reconciler.NewReconciler(client, cfg, store, "customfield_123", nil)
			err := r.Refresh(context.Background())
			Expect(err).NotTo(HaveOccurred())

			td, _ := store.GetTeamData("team-alpha")
			// Check distribution in one of the periods
			for _, p := range td.Periods {
				if p.TotalCount > 0 {
					Expect(p.Distribution.AssociateWellness).To(Equal(1))
					Expect(p.Distribution.IncidentsSupport).To(Equal(1))
					Expect(p.Distribution.SecurityCompliance).To(Equal(1))
					Expect(p.Distribution.QualityStability).To(Equal(1))
					Expect(p.Distribution.FutureSustainability).To(Equal(1))
					Expect(p.Distribution.ProductPortfolio).To(Equal(1))
					Expect(p.Distribution.Uncategorized).To(Equal(1))
					Expect(p.CategorizedCount).To(Equal(6))
					Expect(p.TotalCount).To(Equal(7))
				}
			}
		})

		It("preserves data on failed refresh", func() {
			// First successful refresh
			client.searchFunc = func(ctx context.Context, jql string, opts *jira.SearchOptions) ([]jira.Issue, *jira.Response, error) {
				return []jira.Issue{makeJiraIssue("PROJ-1", "PROJ", "Story", "Tech Debt")},
					&jira.Response{Response: &http.Response{StatusCode: 200}, Total: 1}, nil
			}
			r := reconciler.NewReconciler(client, cfg, store, "customfield_123", nil)
			Expect(r.Refresh(context.Background())).To(Succeed())
			Expect(store.HasData()).To(BeTrue())

			// Second refresh fails
			client.searchFunc = func(ctx context.Context, jql string, opts *jira.SearchOptions) ([]jira.Issue, *jira.Response, error) {
				return nil, nil, fmt.Errorf("network error")
			}
			r2 := reconciler.NewReconciler(client, cfg, store, "customfield_123", nil)
			err := r2.Refresh(context.Background())
			Expect(err).To(HaveOccurred())

			// Previous data should be preserved
			Expect(store.HasData()).To(BeTrue())
			_, ok := store.GetTeamData("team-alpha")
			Expect(ok).To(BeTrue())
		})
	})

	Describe("RefreshTeams", func() {
		It("fetches only the specified teams", func() {
			// Add a second team to config
			cfg.Organizations[0].Pillars[0].Teams = append(cfg.Organizations[0].Pillars[0].Teams, config.Team{
				Name:       "Team Beta",
				Identifier: "team-beta",
				Ownership: config.Ownership{
					Method:     "component",
					Project:    "PROJ",
					Components: []string{"comp-b"},
				},
			})

			queriedJQLs := []string{}
			client.searchFunc = func(ctx context.Context, jql string, opts *jira.SearchOptions) ([]jira.Issue, *jira.Response, error) {
				queriedJQLs = append(queriedJQLs, jql)
				return []jira.Issue{makeJiraIssue("PROJ-1", "PROJ", "Story", "Tech Debt")},
					&jira.Response{Response: &http.Response{StatusCode: 200}, Total: 1}, nil
			}

			r := reconciler.NewReconciler(client, cfg, store, "customfield_123", nil)
			// Only refresh team-alpha
			err := r.RefreshTeams(context.Background(), []config.Team{cfg.Organizations[0].Pillars[0].Teams[0]})
			Expect(err).NotTo(HaveOccurred())

			// team-alpha should be in store
			_, ok := store.GetTeamData("team-alpha")
			Expect(ok).To(BeTrue())

			// team-beta should NOT be in store
			_, ok = store.GetTeamData("team-beta")
			Expect(ok).To(BeFalse())
		})

		It("completes store lifecycle correctly", func() {
			client.searchFunc = func(ctx context.Context, jql string, opts *jira.SearchOptions) ([]jira.Issue, *jira.Response, error) {
				return []jira.Issue{makeJiraIssue("PROJ-1", "PROJ", "Story", "Tech Debt")},
					&jira.Response{Response: &http.Response{StatusCode: 200}, Total: 1}, nil
			}

			r := reconciler.NewReconciler(client, cfg, store, "customfield_123", nil)
			err := r.RefreshTeams(context.Background(), cfg.Organizations[0].Pillars[0].Teams)
			Expect(err).NotTo(HaveOccurred())

			Expect(store.HasData()).To(BeTrue())
			state := store.GetState()
			Expect(state.Status).To(Equal(reconciler.ReconciliationCompleted))
		})

		It("fails if already running", func() {
			store.StartRefresh()

			r := reconciler.NewReconciler(client, cfg, store, "customfield_123", nil)
			err := r.RefreshTeams(context.Background(), cfg.Organizations[0].Pillars[0].Teams)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("already running"))
		})
	})

	Describe("ExecuteRefresh", func() {
		It("works when StartRefresh has already been called", func() {
			client.searchFunc = func(ctx context.Context, jql string, opts *jira.SearchOptions) ([]jira.Issue, *jira.Response, error) {
				return []jira.Issue{makeJiraIssue("PROJ-1", "PROJ", "Story", "Tech Debt")},
					&jira.Response{Response: &http.Response{StatusCode: 200}, Total: 1}, nil
			}

			store.StartRefresh()
			Expect(store.GetState().Status).To(Equal(reconciler.ReconciliationRunning))

			r := reconciler.NewReconciler(client, cfg, store, "customfield_123", nil)
			err := r.ExecuteRefresh(context.Background())
			Expect(err).NotTo(HaveOccurred())

			state := store.GetState()
			Expect(state.Status).To(Equal(reconciler.ReconciliationCompleted))
			Expect(store.HasData()).To(BeTrue())
		})

		It("marks refresh as failed on error", func() {
			client.searchFunc = func(ctx context.Context, jql string, opts *jira.SearchOptions) ([]jira.Issue, *jira.Response, error) {
				return nil, nil, fmt.Errorf("connection refused")
			}

			store.StartRefresh()

			r := reconciler.NewReconciler(client, cfg, store, "customfield_123", nil)
			err := r.ExecuteRefresh(context.Background())
			Expect(err).To(HaveOccurred())

			state := store.GetState()
			Expect(state.Status).To(Equal(reconciler.ReconciliationFailed))
		})
	})

	Describe("sprint_board team", func() {
		It("discovers sprint IDs and includes them in JQL", func() {
			sprintBoardTeam := config.Team{
				Name:       "Coffee",
				Identifier: "coffee",
				Ownership: config.Ownership{
					Method:  "sprint_board",
					Project: "OCM",
					Boards:  []int{100},
				},
			}
			cfg.Organizations[0].Pillars[0].Teams = []config.Team{sprintBoardTeam}

			fetcher := &mockSprintFetcher{
				sprints: map[int][]reconciler.SprintInfo{
					100: {
						{ID: 501, Name: "Sprint 1", State: "closed"},
						{ID: 502, Name: "Sprint 2", State: "active"},
					},
				},
			}

			queriedJQLs := []string{}
			client.searchFunc = func(ctx context.Context, jql string, opts *jira.SearchOptions) ([]jira.Issue, *jira.Response, error) {
				queriedJQLs = append(queriedJQLs, jql)
				return nil, &jira.Response{Response: &http.Response{StatusCode: 200}, Total: 0}, nil
			}

			r := reconciler.NewReconciler(client, cfg, store, "customfield_123", fetcher)
			err := r.Refresh(context.Background())
			Expect(err).NotTo(HaveOccurred())

			// All JQL queries should contain sprint IDs
			for _, jql := range queriedJQLs {
				Expect(jql).To(ContainSubstring("sprint in (501, 502)"))
				Expect(jql).To(ContainSubstring("project = OCM"))
			}
		})
	})

	Describe("Closed vs In-Progress JQL differentiation", func() {
		It("uses statusCategory=Done for closed and status in() for in-progress", func() {
			closedJQLs := []string{}
			inProgressJQLs := []string{}

			client.searchFunc = func(ctx context.Context, jql string, opts *jira.SearchOptions) ([]jira.Issue, *jira.Response, error) {
				if strings.Contains(jql, "statusCategory = Done") {
					closedJQLs = append(closedJQLs, jql)
				}
				if strings.Contains(jql, "status in (") {
					inProgressJQLs = append(inProgressJQLs, jql)
				}
				return nil, &jira.Response{Response: &http.Response{StatusCode: 200}, Total: 0}, nil
			}

			r := reconciler.NewReconciler(client, cfg, store, "customfield_123", nil)
			err := r.Refresh(context.Background())
			Expect(err).NotTo(HaveOccurred())

			// Should have 2 closed queries (previous + current cadence) and 1 in-progress query
			Expect(closedJQLs).To(HaveLen(2))
			Expect(inProgressJQLs).To(HaveLen(1))

			// Closed JQL should use resolved dates, not updated
			for _, jql := range closedJQLs {
				Expect(jql).To(ContainSubstring("resolved >= "))
				Expect(jql).To(ContainSubstring("resolved <= "))
				Expect(jql).NotTo(ContainSubstring("updated"))
			}

			// In-progress JQL should have no date filter
			for _, jql := range inProgressJQLs {
				Expect(jql).NotTo(ContainSubstring("resolved"))
				Expect(jql).NotTo(ContainSubstring("updated"))
			}
		})
	})

	Describe("Scoped Refresh", func() {
		It("status scope 'closed' skips in-progress fetch", func() {
			closedJQLs := []string{}
			inProgressJQLs := []string{}

			client.searchFunc = func(ctx context.Context, jql string, opts *jira.SearchOptions) ([]jira.Issue, *jira.Response, error) {
				if strings.Contains(jql, "statusCategory = Done") {
					closedJQLs = append(closedJQLs, jql)
				}
				if strings.Contains(jql, "status in (") {
					inProgressJQLs = append(inProgressJQLs, jql)
				}
				return nil, &jira.Response{Response: &http.Response{StatusCode: 200}, Total: 0}, nil
			}

			r := reconciler.NewReconciler(client, cfg, store, "customfield_123", nil)
			r.SetRefreshScope(&reconciler.RefreshScope{Status: "closed"})
			err := r.Refresh(context.Background())
			Expect(err).NotTo(HaveOccurred())

			Expect(closedJQLs).To(HaveLen(2)) // previous + current cadence
			Expect(inProgressJQLs).To(BeEmpty())
		})

		It("status scope 'in_progress' skips closed fetch", func() {
			closedJQLs := []string{}
			inProgressJQLs := []string{}

			client.searchFunc = func(ctx context.Context, jql string, opts *jira.SearchOptions) ([]jira.Issue, *jira.Response, error) {
				if strings.Contains(jql, "statusCategory = Done") {
					closedJQLs = append(closedJQLs, jql)
				}
				if strings.Contains(jql, "status in (") {
					inProgressJQLs = append(inProgressJQLs, jql)
				}
				return nil, &jira.Response{Response: &http.Response{StatusCode: 200}, Total: 0}, nil
			}

			r := reconciler.NewReconciler(client, cfg, store, "customfield_123", nil)
			r.SetRefreshScope(&reconciler.RefreshScope{Status: "in_progress"})
			err := r.Refresh(context.Background())
			Expect(err).NotTo(HaveOccurred())

			Expect(closedJQLs).To(BeEmpty())
			Expect(inProgressJQLs).To(HaveLen(1))
		})

		It("date range overrides sprint calendar to a single cadence", func() {
			closedJQLs := []string{}

			client.searchFunc = func(ctx context.Context, jql string, opts *jira.SearchOptions) ([]jira.Issue, *jira.Response, error) {
				if strings.Contains(jql, "statusCategory = Done") {
					closedJQLs = append(closedJQLs, jql)
				}
				return nil, &jira.Response{Response: &http.Response{StatusCode: 200}, Total: 0}, nil
			}

			startDate := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
			endDate := time.Date(2026, 2, 28, 0, 0, 0, 0, time.UTC)
			r := reconciler.NewReconciler(client, cfg, store, "customfield_123", nil)
			r.SetRefreshScope(&reconciler.RefreshScope{
				StartDate: &startDate,
				EndDate:   &endDate,
			})
			err := r.Refresh(context.Background())
			Expect(err).NotTo(HaveOccurred())

			// Single cadence instead of the normal 2 (previous + current)
			Expect(closedJQLs).To(HaveLen(1))
			Expect(closedJQLs[0]).To(ContainSubstring("2026-01-01"))
			Expect(closedJQLs[0]).To(ContainSubstring("2026-02-28"))
		})

		It("period merge preserves out-of-scope periods", func() {
			// Pre-populate store with both closed and in-progress data
			existingTeams := map[string]*reconciler.TeamData{
				"team-alpha": {
					TeamIdentifier: "team-alpha",
					Periods: []reconciler.ScoringPeriod{
						{
							Label:      "Old Closed",
							SetType:    reconciler.IssueSetClosed,
							TotalCount: 5,
						},
						{
							Label:      "Old In Progress",
							SetType:    reconciler.IssueSetInProgress,
							TotalCount: 3,
						},
					},
				},
			}
			store.SwapData(existingTeams, 8)

			// Scoped refresh: only closed
			client.searchFunc = func(ctx context.Context, jql string, opts *jira.SearchOptions) ([]jira.Issue, *jira.Response, error) {
				return []jira.Issue{makeJiraIssue("PROJ-NEW", "PROJ", "Story", "Tech Debt")},
					&jira.Response{Response: &http.Response{StatusCode: 200}, Total: 1}, nil
			}

			r := reconciler.NewReconciler(client, cfg, store, "customfield_123", nil)
			r.SetRefreshScope(&reconciler.RefreshScope{Status: "closed"})
			err := r.Refresh(context.Background())
			Expect(err).NotTo(HaveOccurred())

			td, ok := store.GetTeamData("team-alpha")
			Expect(ok).To(BeTrue())

			// Should have new closed periods + preserved old in-progress period
			hasNewClosed := false
			hasOldInProgress := false
			for _, p := range td.Periods {
				if p.SetType == reconciler.IssueSetClosed && p.TotalCount > 0 {
					hasNewClosed = true
				}
				if p.SetType == reconciler.IssueSetInProgress && p.Label == "Old In Progress" {
					hasOldInProgress = true
				}
			}
			Expect(hasNewClosed).To(BeTrue(), "should have new closed periods")
			Expect(hasOldInProgress).To(BeTrue(), "should preserve old in-progress period")
		})

		It("period merge replaces in-scope periods", func() {
			// Pre-populate store with closed data
			existingTeams := map[string]*reconciler.TeamData{
				"team-alpha": {
					TeamIdentifier: "team-alpha",
					Periods: []reconciler.ScoringPeriod{
						{
							Label:      "Old Closed",
							SetType:    reconciler.IssueSetClosed,
							TotalCount: 5,
						},
					},
				},
			}
			store.SwapData(existingTeams, 5)

			// Scoped refresh: only closed (same scope)
			client.searchFunc = func(ctx context.Context, jql string, opts *jira.SearchOptions) ([]jira.Issue, *jira.Response, error) {
				return []jira.Issue{makeJiraIssue("PROJ-NEW", "PROJ", "Story", "Tech Debt")},
					&jira.Response{Response: &http.Response{StatusCode: 200}, Total: 1}, nil
			}

			r := reconciler.NewReconciler(client, cfg, store, "customfield_123", nil)
			r.SetRefreshScope(&reconciler.RefreshScope{Status: "closed"})
			err := r.Refresh(context.Background())
			Expect(err).NotTo(HaveOccurred())

			td, ok := store.GetTeamData("team-alpha")
			Expect(ok).To(BeTrue())

			// Old closed periods should be replaced, not duplicated
			closedCount := 0
			for _, p := range td.Periods {
				if p.SetType == reconciler.IssueSetClosed {
					closedCount++
					// None of the closed periods should be the "Old Closed" one
					Expect(p.Label).NotTo(Equal("Old Closed"))
				}
			}
			Expect(closedCount).To(BeNumerically(">", 0))
		})

		It("scoped refresh forces upsert to preserve other teams", func() {
			// Pre-populate store with team-beta data
			existingTeams := map[string]*reconciler.TeamData{
				"team-beta": {
					TeamIdentifier: "team-beta",
					Periods: []reconciler.ScoringPeriod{
						{
							Label:      "Beta Sprint",
							SetType:    reconciler.IssueSetClosed,
							TotalCount: 10,
						},
					},
				},
			}
			store.SwapData(existingTeams, 10)

			client.searchFunc = func(ctx context.Context, jql string, opts *jira.SearchOptions) ([]jira.Issue, *jira.Response, error) {
				return []jira.Issue{makeJiraIssue("PROJ-1", "PROJ", "Story", "Tech Debt")},
					&jira.Response{Response: &http.Response{StatusCode: 200}, Total: 1}, nil
			}

			r := reconciler.NewReconciler(client, cfg, store, "customfield_123", nil)
			r.SetRefreshScope(&reconciler.RefreshScope{Status: "closed"})
			err := r.Refresh(context.Background())
			Expect(err).NotTo(HaveOccurred())

			// team-alpha was refreshed
			_, ok := store.GetTeamData("team-alpha")
			Expect(ok).To(BeTrue())

			// team-beta should still be preserved (upsert, not swap)
			_, ok = store.GetTeamData("team-beta")
			Expect(ok).To(BeTrue(), "scoped refresh should preserve other teams via upsert")
		})
	})

	Describe("mapActivityTypeToCategory (via integration)", func() {
		DescribeTable("maps known activity types correctly",
			func(activityType, expectedCategory string) {
				client.searchFunc = func(ctx context.Context, jql string, opts *jira.SearchOptions) ([]jira.Issue, *jira.Response, error) {
					return []jira.Issue{makeJiraIssue("PROJ-1", "PROJ", "Story", activityType)},
						&jira.Response{Response: &http.Response{StatusCode: 200}, Total: 1}, nil
				}
				r := reconciler.NewReconciler(client, cfg, store, "customfield_123", nil)
				Expect(r.Refresh(context.Background())).To(Succeed())

				td, _ := store.GetTeamData("team-alpha")
				for _, p := range td.Periods {
					if p.TotalCount > 0 {
						switch expectedCategory {
						case "Associate Wellness & Development":
							Expect(p.Distribution.AssociateWellness).To(Equal(1))
						case "Incidents & Support":
							Expect(p.Distribution.IncidentsSupport).To(Equal(1))
						case "Security & Compliance":
							Expect(p.Distribution.SecurityCompliance).To(Equal(1))
						case "Quality / Stability / Reliability":
							Expect(p.Distribution.QualityStability).To(Equal(1))
						case "Future Sustainability":
							Expect(p.Distribution.FutureSustainability).To(Equal(1))
						case "Product / Portfolio Work":
							Expect(p.Distribution.ProductPortfolio).To(Equal(1))
						case "":
							Expect(p.Distribution.Uncategorized).To(Equal(1))
						}
					}
				}
			},
			Entry("Associate Wellness & Development", "Associate Wellness & Development", "Associate Wellness & Development"),
			Entry("Incidents & Escalations", "Incidents & Escalations", "Incidents & Support"),
			Entry("Customer Support", "Customer Support", "Incidents & Support"),
			Entry("Security & Compliance", "Security & Compliance", "Security & Compliance"),
			Entry("Tech Debt", "Tech Debt", "Quality / Stability / Reliability"),
			Entry("Defect", "Defect", "Quality / Stability / Reliability"),
			Entry("QE Activities", "QE Activities", "Quality / Stability / Reliability"),
			Entry("Quality / Stability / Reliability", "Quality / Stability / Reliability", "Quality / Stability / Reliability"),
			Entry("Future Sustainability", "Future Sustainability", "Future Sustainability"),
			Entry("Product / Portfolio Work", "Product / Portfolio Work", "Product / Portfolio Work"),
			Entry("New Feature", "New Feature", "Product / Portfolio Work"),
			Entry("Feature Enhancement", "Feature Enhancement", "Product / Portfolio Work"),
			Entry("Unknown value", "Something Unknown", ""),
			Entry("Empty string", "", ""),
		)
	})
})
