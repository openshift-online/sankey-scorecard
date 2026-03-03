//go:build integration

package db_test

import (
	"context"
	"fmt"
	"testing"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/modules/postgres"
	"github.com/testcontainers/testcontainers-go/wait"

	"github.com/tiwillia/sankey-scorecard/pkg/db"
	"github.com/tiwillia/sankey-scorecard/pkg/reconciler"
)

func TestPGStore(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "PGStore Suite")
}

var (
	pgContainer *postgres.PostgresContainer
	dsn         string
)

var _ = BeforeSuite(func() {
	ctx := context.Background()
	var err error
	pgContainer, err = postgres.Run(ctx, "postgres:16-alpine",
		postgres.WithDatabase("testdb"),
		postgres.WithUsername("test"),
		postgres.WithPassword("test"),
		testcontainers.WithWaitStrategy(
			wait.ForLog("database system is ready to accept connections").
				WithOccurrence(2).
				WithStartupTimeout(30*time.Second),
		),
	)
	Expect(err).NotTo(HaveOccurred())

	dsn, err = pgContainer.ConnectionString(ctx, "sslmode=disable")
	Expect(err).NotTo(HaveOccurred())
})

var _ = AfterSuite(func() {
	if pgContainer != nil {
		pgContainer.Terminate(context.Background())
	}
})

func makeTestTeamData(identifier string, periodCount int) *reconciler.TeamData {
	td := &reconciler.TeamData{
		TeamIdentifier: identifier,
		ReconciledAt:   time.Now().Truncate(time.Microsecond),
	}
	for i := 0; i < periodCount; i++ {
		td.Periods = append(td.Periods, reconciler.ScoringPeriod{
			Window: reconciler.TimeWindow{
				Since: time.Date(2026, 1, 1+i*21, 0, 0, 0, 0, time.UTC),
				Until: time.Date(2026, 1, 22+i*21, 0, 0, 0, 0, time.UTC),
			},
			Label:            fmt.Sprintf("Sprint %d", i+1),
			Current:          i == periodCount-1,
			SetType:          reconciler.IssueSetClosed,
			TotalCount:       5,
			CategorizedCount: 4,
			Distribution: reconciler.ActivityDistribution{
				IncidentsSupport:     1,
				SecurityCompliance:   1,
				QualityStability:     1,
				FutureSustainability: 1,
				Uncategorized:        1,
			},
			Issues: []reconciler.Issue{
				{
					Key:          fmt.Sprintf("%s-%d", identifier, i*5+1),
					Project:      "PROJ",
					IssueType:    "Story",
					ActivityType: "Tech Debt",
					Status:       "Closed",
					Components:   []string{"comp-a"},
					Summary:      "Test issue",
					UpdatedDate:  time.Date(2026, 1, 15, 0, 0, 0, 0, time.UTC),
					CreatedDate:  time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC),
				},
			},
		})
	}
	return td
}

var _ = Describe("PGStore", func() {
	var store *db.PGStore

	BeforeEach(func() {
		var err error
		store, err = db.NewPGStore(dsn)
		Expect(err).NotTo(HaveOccurred())

		// Clean all data between tests
		teams := store.GetAllTeamData()
		if len(teams) > 0 {
			store.SwapData(map[string]*reconciler.TeamData{}, 0)
		}
	})

	AfterEach(func() {
		if store != nil {
			store.Close()
		}
	})

	Describe("initial state", func() {
		It("starts with idle status", func() {
			state := store.GetState()
			Expect(state.Status).To(Equal(reconciler.ReconciliationIdle))
		})

		It("starts with no data", func() {
			Expect(store.HasData()).To(BeFalse())
		})

		It("returns empty team data", func() {
			teams := store.GetAllTeamData()
			Expect(teams).To(BeEmpty())
		})
	})

	Describe("state transitions", func() {
		It("transitions from idle to running", func() {
			ok := store.StartRefresh()
			Expect(ok).To(BeTrue())
			state := store.GetState()
			Expect(state.Status).To(Equal(reconciler.ReconciliationRunning))
			Expect(state.StartedAt).NotTo(BeNil())
		})

		It("rejects concurrent refresh", func() {
			ok := store.StartRefresh()
			Expect(ok).To(BeTrue())
			ok = store.StartRefresh()
			Expect(ok).To(BeFalse())
		})

		It("transitions from running to completed", func() {
			store.StartRefresh()
			store.CompleteRefresh(42)
			state := store.GetState()
			Expect(state.Status).To(Equal(reconciler.ReconciliationCompleted))
			Expect(state.CompletedAt).NotTo(BeNil())
			Expect(state.IssueCount).To(Equal(42))
		})

		It("transitions from running to failed", func() {
			store.StartRefresh()
			store.FailRefresh(fmt.Errorf("test error"))
			state := store.GetState()
			Expect(state.Status).To(Equal(reconciler.ReconciliationFailed))
			Expect(state.Error).To(Equal("test error"))
		})

		It("transitions from completed to running", func() {
			store.StartRefresh()
			store.CompleteRefresh(10)
			ok := store.StartRefresh()
			Expect(ok).To(BeTrue())
			Expect(store.GetState().Status).To(Equal(reconciler.ReconciliationRunning))
		})

		It("transitions from failed to running", func() {
			store.StartRefresh()
			store.FailRefresh(fmt.Errorf("error"))
			ok := store.StartRefresh()
			Expect(ok).To(BeTrue())
			Expect(store.GetState().Status).To(Equal(reconciler.ReconciliationRunning))
		})
	})

	Describe("SwapData", func() {
		It("stores and retrieves team data", func() {
			teams := map[string]*reconciler.TeamData{
				"team-a": makeTestTeamData("team-a", 2),
				"team-b": makeTestTeamData("team-b", 1),
			}
			store.SwapData(teams, 15)

			Expect(store.HasData()).To(BeTrue())
			allTeams := store.GetAllTeamData()
			Expect(allTeams).To(HaveLen(2))
		})

		It("retrieves specific team data with issues", func() {
			teams := map[string]*reconciler.TeamData{
				"team-a": makeTestTeamData("team-a", 1),
			}
			store.SwapData(teams, 5)

			td, ok := store.GetTeamData("team-a")
			Expect(ok).To(BeTrue())
			Expect(td.TeamIdentifier).To(Equal("team-a"))
			Expect(td.Periods).To(HaveLen(1))
			Expect(td.Periods[0].Issues).To(HaveLen(1))
			Expect(td.Periods[0].Issues[0].Key).To(Equal("team-a-1"))
		})

		It("returns false for nonexistent team", func() {
			_, ok := store.GetTeamData("nonexistent")
			Expect(ok).To(BeFalse())
		})

		It("overwrites previous data on swap", func() {
			teams1 := map[string]*reconciler.TeamData{
				"team-a": makeTestTeamData("team-a", 1),
			}
			store.SwapData(teams1, 5)

			teams2 := map[string]*reconciler.TeamData{
				"team-b": makeTestTeamData("team-b", 1),
			}
			store.SwapData(teams2, 5)

			_, ok := store.GetTeamData("team-a")
			Expect(ok).To(BeFalse())
			_, ok = store.GetTeamData("team-b")
			Expect(ok).To(BeTrue())
		})
	})

	Describe("UpsertTeamData", func() {
		It("merges new teams into existing data", func() {
			existing := map[string]*reconciler.TeamData{
				"team-a": makeTestTeamData("team-a", 1),
			}
			store.SwapData(existing, 5)

			newTeams := map[string]*reconciler.TeamData{
				"team-b": makeTestTeamData("team-b", 1),
			}
			store.UpsertTeamData(newTeams, 5)

			Expect(store.GetAllTeamData()).To(HaveLen(2))
			_, ok := store.GetTeamData("team-a")
			Expect(ok).To(BeTrue())
			_, ok = store.GetTeamData("team-b")
			Expect(ok).To(BeTrue())
		})

		It("replaces existing team's periods", func() {
			existing := map[string]*reconciler.TeamData{
				"team-a": makeTestTeamData("team-a", 2),
			}
			store.SwapData(existing, 10)

			updated := map[string]*reconciler.TeamData{
				"team-a": makeTestTeamData("team-a", 1),
			}
			store.UpsertTeamData(updated, 5)

			td, ok := store.GetTeamData("team-a")
			Expect(ok).To(BeTrue())
			Expect(td.Periods).To(HaveLen(1))
		})

		It("preserves teams not in the input", func() {
			existing := map[string]*reconciler.TeamData{
				"team-a": makeTestTeamData("team-a", 1),
				"team-b": makeTestTeamData("team-b", 1),
			}
			store.SwapData(existing, 10)

			updated := map[string]*reconciler.TeamData{
				"team-a": makeTestTeamData("team-a", 1),
			}
			store.UpsertTeamData(updated, 5)

			_, ok := store.GetTeamData("team-b")
			Expect(ok).To(BeTrue())
		})
	})

	Describe("data persistence", func() {
		It("data survives close and reopen", func() {
			teams := map[string]*reconciler.TeamData{
				"team-a": makeTestTeamData("team-a", 1),
			}
			store.SwapData(teams, 5)
			store.StartRefresh()
			store.CompleteRefresh(5)
			store.Close()

			// Reopen
			store2, err := db.NewPGStore(dsn)
			Expect(err).NotTo(HaveOccurred())
			defer store2.Close()

			Expect(store2.HasData()).To(BeTrue())
			td, ok := store2.GetTeamData("team-a")
			Expect(ok).To(BeTrue())
			Expect(td.Periods).To(HaveLen(1))
			Expect(td.Periods[0].Issues).To(HaveLen(1))

			state := store2.GetState()
			Expect(state.Status).To(Equal(reconciler.ReconciliationCompleted))
			Expect(state.IssueCount).To(Equal(5))

			// Set store to reopened store for AfterEach cleanup
			store = store2
		})
	})

	Describe("AutoMigrate idempotency", func() {
		It("opening a second store against the same DB succeeds", func() {
			store2, err := db.NewPGStore(dsn)
			Expect(err).NotTo(HaveOccurred())
			store2.Close()
		})
	})
})
