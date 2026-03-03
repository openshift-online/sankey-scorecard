package config_test

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/tiwillia/sankey-scorecard/pkg/config"
)

func makeValidResourceMap() *config.ResourceMap {
	return &config.ResourceMap{
		SprintReferenceDate: "2026-02-11",
		SprintDurationDays:  21,
		Jira: config.JiraConfig{
			ScoredIssueTypes:   []string{"Story", "Bug", "Task"},
			RequestDelayMs:     100,
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
									Method:  "component",
									Project: "PROJ",
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

var _ = Describe("Validation", func() {

	Describe("sprint_duration_days", func() {
		It("rejects zero value", func() {
			rm := makeValidResourceMap()
			rm.SprintDurationDays = 0
			err := config.Validate(rm)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("sprint_duration_days must be greater than 0"))
		})

		It("rejects negative value", func() {
			rm := makeValidResourceMap()
			rm.SprintDurationDays = -1
			err := config.Validate(rm)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("sprint_duration_days must be greater than 0"))
		})

		It("accepts positive value", func() {
			rm := makeValidResourceMap()
			rm.SprintDurationDays = 14
			err := config.Validate(rm)
			Expect(err).NotTo(HaveOccurred())
		})
	})

	Describe("sprint_reference_date", func() {
		It("rejects empty string", func() {
			rm := makeValidResourceMap()
			rm.SprintReferenceDate = ""
			err := config.Validate(rm)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("sprint_reference_date is required"))
		})

		It("rejects invalid date format", func() {
			rm := makeValidResourceMap()
			rm.SprintReferenceDate = "02-11-2026"
			err := config.Validate(rm)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("not a valid YYYY-MM-DD date"))
		})

		It("accepts valid YYYY-MM-DD date", func() {
			rm := makeValidResourceMap()
			rm.SprintReferenceDate = "2026-01-01"
			err := config.Validate(rm)
			Expect(err).NotTo(HaveOccurred())
		})
	})

	Describe("identifier format", func() {
		It("rejects identifiers with uppercase letters", func() {
			rm := makeValidResourceMap()
			rm.Organizations[0].Identifier = "TestOrg"
			err := config.Validate(rm)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("lowercase alphanumeric with hyphens only"))
		})

		It("rejects identifiers with underscores", func() {
			rm := makeValidResourceMap()
			rm.Organizations[0].Identifier = "test_org"
			err := config.Validate(rm)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("lowercase alphanumeric with hyphens only"))
		})

		It("rejects identifiers with spaces", func() {
			rm := makeValidResourceMap()
			rm.Organizations[0].Identifier = "test org"
			err := config.Validate(rm)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("lowercase alphanumeric with hyphens only"))
		})

		It("accepts valid identifiers", func() {
			rm := makeValidResourceMap()
			rm.Organizations[0].Identifier = "my-org-123"
			err := config.Validate(rm)
			Expect(err).NotTo(HaveOccurred())
		})
	})

	Describe("identifier uniqueness", func() {
		It("rejects duplicate identifiers across levels", func() {
			rm := makeValidResourceMap()
			rm.Organizations[0].Pillars[0].Identifier = "test-org" // same as org
			err := config.Validate(rm)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("conflicts with"))
		})

		It("rejects duplicate team identifiers", func() {
			rm := makeValidResourceMap()
			rm.Organizations[0].Pillars[0].Teams = append(rm.Organizations[0].Pillars[0].Teams,
				config.Team{
					Name:       "Team Alpha Duplicate",
					Identifier: "team-alpha",
					Ownership: config.Ownership{
						Method:  "component",
						Project: "PROJ",
						Components: []string{"comp-b"},
					},
				},
			)
			err := config.Validate(rm)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("conflicts with"))
		})
	})

	Describe("reserved words", func() {
		It("rejects 'serve' as identifier", func() {
			rm := makeValidResourceMap()
			rm.Organizations[0].Identifier = "serve"
			err := config.Validate(rm)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("reserved word"))
		})

		It("rejects 'refresh-data' as identifier", func() {
			rm := makeValidResourceMap()
			rm.Organizations[0].Identifier = "refresh-data"
			err := config.Validate(rm)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("reserved word"))
		})

		It("rejects 'version' as identifier", func() {
			rm := makeValidResourceMap()
			rm.Organizations[0].Identifier = "version"
			err := config.Validate(rm)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("reserved word"))
		})

		It("rejects 'help' as identifier", func() {
			rm := makeValidResourceMap()
			rm.Organizations[0].Identifier = "help"
			err := config.Validate(rm)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("reserved word"))
		})
	})

	Describe("ownership method validation", func() {
		It("rejects unknown ownership method", func() {
			rm := makeValidResourceMap()
			rm.Organizations[0].Pillars[0].Teams[0].Ownership.Method = "unknown"
			err := config.Validate(rm)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("not one of"))
		})

		It("rejects component method without project", func() {
			rm := makeValidResourceMap()
			rm.Organizations[0].Pillars[0].Teams[0].Ownership.Project = ""
			err := config.Validate(rm)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("requires 'project'"))
		})

		It("rejects component method without components", func() {
			rm := makeValidResourceMap()
			rm.Organizations[0].Pillars[0].Teams[0].Ownership.Components = nil
			err := config.Validate(rm)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("requires non-empty 'components'"))
		})

		It("rejects team_field method without project", func() {
			rm := makeValidResourceMap()
			rm.Organizations[0].Pillars[0].Teams[0].Ownership = config.Ownership{
				Method:         "team_field",
				TeamFieldValue: "Alpha",
			}
			err := config.Validate(rm)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("requires 'project'"))
		})

		It("rejects team_field method without team_field_value", func() {
			rm := makeValidResourceMap()
			rm.Organizations[0].Pillars[0].Teams[0].Ownership = config.Ownership{
				Method:  "team_field",
				Project: "PROJ",
			}
			err := config.Validate(rm)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("requires 'team_field_value'"))
		})

		It("rejects jql method without jql", func() {
			rm := makeValidResourceMap()
			rm.Organizations[0].Pillars[0].Teams[0].Ownership = config.Ownership{
				Method: "jql",
			}
			err := config.Validate(rm)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("requires non-empty 'jql'"))
		})

		It("accepts valid component ownership", func() {
			rm := makeValidResourceMap()
			err := config.Validate(rm)
			Expect(err).NotTo(HaveOccurred())
		})

		It("accepts valid team_field ownership", func() {
			rm := makeValidResourceMap()
			rm.Organizations[0].Pillars[0].Teams[0].Ownership = config.Ownership{
				Method:         "team_field",
				Project:        "OCM",
				TeamFieldValue: "Alpha",
			}
			err := config.Validate(rm)
			Expect(err).NotTo(HaveOccurred())
		})

		It("accepts valid jql ownership", func() {
			rm := makeValidResourceMap()
			rm.Organizations[0].Pillars[0].Teams[0].Ownership = config.Ownership{
				Method: "jql",
				JQL:    "project = ACM",
			}
			err := config.Validate(rm)
			Expect(err).NotTo(HaveOccurred())
		})

		It("rejects sprint_board method without project", func() {
			rm := makeValidResourceMap()
			rm.Organizations[0].Pillars[0].Teams[0].Ownership = config.Ownership{
				Method: "sprint_board",
				Boards: []int{123},
			}
			err := config.Validate(rm)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("requires 'project'"))
		})

		It("rejects sprint_board method without boards", func() {
			rm := makeValidResourceMap()
			rm.Organizations[0].Pillars[0].Teams[0].Ownership = config.Ownership{
				Method:  "sprint_board",
				Project: "OCM",
			}
			err := config.Validate(rm)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("requires non-empty 'boards'"))
		})

		It("accepts valid sprint_board ownership", func() {
			rm := makeValidResourceMap()
			rm.Organizations[0].Pillars[0].Teams[0].Ownership = config.Ownership{
				Method:  "sprint_board",
				Project: "OCM",
				Boards:  []int{100, 200},
			}
			err := config.Validate(rm)
			Expect(err).NotTo(HaveOccurred())
		})
	})

	Describe("in_progress_statuses validation", func() {
		It("rejects empty in_progress_statuses", func() {
			rm := makeValidResourceMap()
			rm.Jira.InProgressStatuses = nil
			err := config.Validate(rm)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("in_progress_statuses must be non-empty"))
		})

		It("accepts non-empty in_progress_statuses", func() {
			rm := makeValidResourceMap()
			rm.Jira.InProgressStatuses = []string{"In Progress"}
			err := config.Validate(rm)
			Expect(err).NotTo(HaveOccurred())
		})
	})

	Describe("multiple errors", func() {
		It("reports all validation failures", func() {
			rm := &config.ResourceMap{
				SprintReferenceDate: "",
				SprintDurationDays:  0,
				Organizations: []config.Organization{
					{
						Name:       "Bad Org",
						Identifier: "Bad_Org",
						Pillars: []config.Pillar{
							{
								Name:       "Pillar",
								Identifier: "serve",
								Teams: []config.Team{
									{
										Name:       "Team",
										Identifier: "team",
										Ownership: config.Ownership{
											Method: "unknown",
										},
									},
								},
							},
						},
					},
				},
			}
			err := config.Validate(rm)
			Expect(err).To(HaveOccurred())
			errStr := err.Error()
			Expect(errStr).To(ContainSubstring("sprint_duration_days must be greater than 0"))
			Expect(errStr).To(ContainSubstring("sprint_reference_date is required"))
			Expect(errStr).To(ContainSubstring("lowercase alphanumeric with hyphens only"))
			Expect(errStr).To(ContainSubstring("reserved word"))
			Expect(errStr).To(ContainSubstring("not one of"))
			Expect(errStr).To(ContainSubstring("in_progress_statuses must be non-empty"))
		})
	})
})
