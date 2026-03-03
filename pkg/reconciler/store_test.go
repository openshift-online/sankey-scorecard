package reconciler_test

import (
	"fmt"
	"sync"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/tiwillia/sankey-scorecard/pkg/reconciler"
)

var _ = Describe("ReconciliationStore", func() {
	var store *reconciler.ReconciliationStore

	BeforeEach(func() {
		store = reconciler.NewReconciliationStore()
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

	Describe("data operations", func() {
		It("swaps data atomically", func() {
			teams := map[string]*reconciler.TeamData{
				"team-a": {TeamIdentifier: "team-a", ReconciledAt: time.Now()},
				"team-b": {TeamIdentifier: "team-b", ReconciledAt: time.Now()},
			}
			store.SwapData(teams, 100)

			Expect(store.HasData()).To(BeTrue())
			allTeams := store.GetAllTeamData()
			Expect(allTeams).To(HaveLen(2))
		})

		It("retrieves specific team data", func() {
			teams := map[string]*reconciler.TeamData{
				"team-a": {TeamIdentifier: "team-a"},
			}
			store.SwapData(teams, 10)

			td, ok := store.GetTeamData("team-a")
			Expect(ok).To(BeTrue())
			Expect(td.TeamIdentifier).To(Equal("team-a"))

			_, ok = store.GetTeamData("nonexistent")
			Expect(ok).To(BeFalse())
		})

		It("overwrites previous data on swap", func() {
			teams1 := map[string]*reconciler.TeamData{
				"team-a": {TeamIdentifier: "team-a"},
			}
			store.SwapData(teams1, 10)

			teams2 := map[string]*reconciler.TeamData{
				"team-b": {TeamIdentifier: "team-b"},
			}
			store.SwapData(teams2, 20)

			_, ok := store.GetTeamData("team-a")
			Expect(ok).To(BeFalse())
			_, ok = store.GetTeamData("team-b")
			Expect(ok).To(BeTrue())
		})
	})

	Describe("UpsertTeamData", func() {
		It("merges new teams into existing map", func() {
			existing := map[string]*reconciler.TeamData{
				"team-a": {TeamIdentifier: "team-a", ReconciledAt: time.Now()},
			}
			store.SwapData(existing, 0)

			newTeams := map[string]*reconciler.TeamData{
				"team-b": {TeamIdentifier: "team-b", ReconciledAt: time.Now()},
			}
			store.UpsertTeamData(newTeams, 0)

			Expect(store.GetAllTeamData()).To(HaveLen(2))
			_, ok := store.GetTeamData("team-a")
			Expect(ok).To(BeTrue())
			_, ok = store.GetTeamData("team-b")
			Expect(ok).To(BeTrue())
		})

		It("replaces existing team's periods", func() {
			existing := map[string]*reconciler.TeamData{
				"team-a": {
					TeamIdentifier: "team-a",
					Periods: []reconciler.ScoringPeriod{
						{Label: "old period", TotalCount: 5},
					},
				},
			}
			store.SwapData(existing, 5)

			updated := map[string]*reconciler.TeamData{
				"team-a": {
					TeamIdentifier: "team-a",
					Periods: []reconciler.ScoringPeriod{
						{Label: "new period", TotalCount: 10},
					},
				},
			}
			store.UpsertTeamData(updated, 10)

			td, ok := store.GetTeamData("team-a")
			Expect(ok).To(BeTrue())
			Expect(td.Periods).To(HaveLen(1))
			Expect(td.Periods[0].Label).To(Equal("new period"))
		})

		It("preserves teams not in the input", func() {
			existing := map[string]*reconciler.TeamData{
				"team-a": {TeamIdentifier: "team-a", Periods: []reconciler.ScoringPeriod{{TotalCount: 5}}},
				"team-b": {TeamIdentifier: "team-b", Periods: []reconciler.ScoringPeriod{{TotalCount: 3}}},
			}
			store.SwapData(existing, 8)

			updated := map[string]*reconciler.TeamData{
				"team-a": {
					TeamIdentifier: "team-a",
					Periods:        []reconciler.ScoringPeriod{{TotalCount: 10}},
				},
			}
			store.UpsertTeamData(updated, 10)

			// team-b should still be present
			td, ok := store.GetTeamData("team-b")
			Expect(ok).To(BeTrue())
			Expect(td.Periods[0].TotalCount).To(Equal(3))
		})

		It("recounts total issues correctly", func() {
			existing := map[string]*reconciler.TeamData{
				"team-a": {TeamIdentifier: "team-a", Periods: []reconciler.ScoringPeriod{{TotalCount: 5}}},
				"team-b": {TeamIdentifier: "team-b", Periods: []reconciler.ScoringPeriod{{TotalCount: 3}}},
			}
			store.SwapData(existing, 8)

			updated := map[string]*reconciler.TeamData{
				"team-a": {
					TeamIdentifier: "team-a",
					Periods:        []reconciler.ScoringPeriod{{TotalCount: 10}},
				},
			}
			store.UpsertTeamData(updated, 10)

			// Total should be 10 (team-a) + 3 (team-b) = 13
			state := store.GetState()
			Expect(state.IssueCount).To(Equal(13))
		})
	})

	Describe("concurrent access", func() {
		It("supports concurrent reads", func() {
			teams := map[string]*reconciler.TeamData{
				"team-a": {TeamIdentifier: "team-a"},
			}
			store.SwapData(teams, 10)

			var wg sync.WaitGroup
			for i := 0; i < 100; i++ {
				wg.Add(1)
				go func() {
					defer wg.Done()
					_ = store.HasData()
					_ = store.GetState()
					_ = store.GetAllTeamData()
					_, _ = store.GetTeamData("team-a")
				}()
			}
			wg.Wait()
		})

		It("handles concurrent reads during write", func() {
			// Start with initial data
			teams := map[string]*reconciler.TeamData{
				"team-a": {TeamIdentifier: "team-a"},
			}
			store.SwapData(teams, 10)

			var wg sync.WaitGroup
			// Readers
			for i := 0; i < 50; i++ {
				wg.Add(1)
				go func() {
					defer wg.Done()
					for j := 0; j < 100; j++ {
						_ = store.HasData()
						_ = store.GetAllTeamData()
					}
				}()
			}
			// Writer
			wg.Add(1)
			go func() {
				defer wg.Done()
				newTeams := map[string]*reconciler.TeamData{
					"team-b": {TeamIdentifier: "team-b"},
				}
				store.SwapData(newTeams, 20)
			}()

			wg.Wait()
		})
	})
})
