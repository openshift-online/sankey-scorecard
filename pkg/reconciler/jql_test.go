package reconciler_test

import (
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/tiwillia/sankey-scorecard/pkg/config"
	"github.com/tiwillia/sankey-scorecard/pkg/reconciler"
)

var _ = Describe("JQL Builder", func() {
	scoredTypes := []string{"Story", "Bug", "Task"}
	window := reconciler.TimeWindow{
		Since: time.Date(2026, 2, 11, 0, 0, 0, 0, time.UTC),
		Until: time.Date(2026, 3, 4, 0, 0, 0, 0, time.UTC),
	}
	inProgressStatuses := []string{"In Progress", "Code Review", "Review"}

	Describe("BuildClosedJQL", func() {
		It("builds closed JQL for component ownership", func() {
			team := config.Team{
				Name:       "Test Team",
				Identifier: "test-team",
				Ownership: config.Ownership{
					Method:     "component",
					Project:    "ARO",
					Components: []string{"clusters-service", "aro-hcp-clusters-service"},
				},
			}
			jql := reconciler.BuildClosedJQL(team, scoredTypes, window, nil)
			Expect(jql).To(ContainSubstring("project = ARO"))
			Expect(jql).To(ContainSubstring("component in ("))
			Expect(jql).To(ContainSubstring("clusters-service"))
			Expect(jql).To(ContainSubstring("aro-hcp-clusters-service"))
			Expect(jql).To(ContainSubstring("issuetype in ("))
			Expect(jql).To(ContainSubstring("statusCategory = Done"))
			Expect(jql).To(ContainSubstring(`resolved >= "2026-02-11"`))
			Expect(jql).To(ContainSubstring(`resolved <= "2026-03-04"`))
			Expect(jql).NotTo(ContainSubstring("updated"))
		})

		It("builds closed JQL for team_field ownership", func() {
			team := config.Team{
				Name:       "Aurora",
				Identifier: "rosa-aurora",
				Ownership: config.Ownership{
					Method:         "team_field",
					Project:        "SREP",
					TeamFieldValue: "5695",
				},
			}
			jql := reconciler.BuildClosedJQL(team, scoredTypes, window, nil)
			Expect(jql).To(ContainSubstring("project = SREP"))
			Expect(jql).To(ContainSubstring("\"Team\" = 5695"))
			Expect(jql).To(ContainSubstring("statusCategory = Done"))
			Expect(jql).To(ContainSubstring("resolved >= "))
		})

		It("builds closed JQL for jql ownership", func() {
			team := config.Team{
				Name:       "Mgmt Apps",
				Identifier: "mgmt-applications",
				Ownership: config.Ownership{
					Method: "jql",
					JQL:    "project = ACM AND component in (\"Application Lifecycle\", \"Search\")",
				},
			}
			jql := reconciler.BuildClosedJQL(team, scoredTypes, window, nil)
			Expect(jql).To(ContainSubstring("(project = ACM AND component in (\"Application Lifecycle\", \"Search\"))"))
			Expect(jql).To(ContainSubstring("statusCategory = Done"))
		})

		It("builds closed JQL for sprint_board ownership with sprint IDs", func() {
			team := config.Team{
				Name:       "Coffee",
				Identifier: "rosa-coffee",
				Ownership: config.Ownership{
					Method:  "sprint_board",
					Project: "OCM",
					Boards:  []int{100},
				},
			}
			sprintIDs := []int{501, 502, 503}
			jql := reconciler.BuildClosedJQL(team, scoredTypes, window, sprintIDs)
			Expect(jql).To(ContainSubstring("project = OCM"))
			Expect(jql).To(ContainSubstring("sprint in (501, 502, 503)"))
			Expect(jql).To(ContainSubstring("statusCategory = Done"))
			Expect(jql).To(ContainSubstring(`resolved >= "2026-02-11"`))
		})
	})

	Describe("BuildInProgressJQL", func() {
		It("builds in-progress JQL for component ownership", func() {
			team := config.Team{
				Name:       "Test Team",
				Identifier: "test-team",
				Ownership: config.Ownership{
					Method:     "component",
					Project:    "ARO",
					Components: []string{"clusters-service"},
				},
			}
			jql := reconciler.BuildInProgressJQL(team, scoredTypes, inProgressStatuses, nil)
			Expect(jql).To(ContainSubstring("project = ARO"))
			Expect(jql).To(ContainSubstring("component in ("))
			Expect(jql).To(ContainSubstring("issuetype in ("))
			Expect(jql).To(ContainSubstring(`status in ("In Progress", "Code Review", "Review")`))
			Expect(jql).NotTo(ContainSubstring("resolved"))
			Expect(jql).NotTo(ContainSubstring("updated"))
		})

		It("builds in-progress JQL for sprint_board ownership", func() {
			team := config.Team{
				Name:       "Coffee",
				Identifier: "rosa-coffee",
				Ownership: config.Ownership{
					Method:  "sprint_board",
					Project: "OCM",
					Boards:  []int{100},
				},
			}
			sprintIDs := []int{501, 502}
			jql := reconciler.BuildInProgressJQL(team, scoredTypes, inProgressStatuses, sprintIDs)
			Expect(jql).To(ContainSubstring("project = OCM"))
			Expect(jql).To(ContainSubstring("sprint in (501, 502)"))
			Expect(jql).To(ContainSubstring("status in ("))
		})

		It("has no date filter", func() {
			team := config.Team{
				Name:       "Test",
				Identifier: "test",
				Ownership: config.Ownership{
					Method:         "team_field",
					Project:        "PROJ",
					TeamFieldValue: "TestTeam",
				},
			}
			jql := reconciler.BuildInProgressJQL(team, scoredTypes, inProgressStatuses, nil)
			Expect(jql).NotTo(ContainSubstring("resolved"))
			Expect(jql).NotTo(ContainSubstring("updated"))
			Expect(jql).NotTo(ContainSubstring(">="))
			Expect(jql).NotTo(ContainSubstring("<="))
		})
	})
})
