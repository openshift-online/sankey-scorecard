package config_test

import (
	"os"
	"path/filepath"
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/tiwillia/sankey-scorecard/pkg/config"
)

func TestConfig(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Config Suite")
}

var validYAML = `
jira:
  scored_issue_types:
    - Story
    - Bug
    - Task
  request_delay_ms: 100
  in_progress_statuses:
    - In Progress
    - Code Review
    - Review

sprint_reference_date: "2026-02-11"
sprint_duration_days: 21

organizations:
  - name: Test Org
    identifier: test-org
    pillars:
      - name: Test Pillar
        identifier: test-pillar
        teams:
          - name: Team Alpha
            identifier: team-alpha
            ownership:
              method: component
              project: PROJ
              components:
                - comp-a
          - name: Team Beta
            identifier: team-beta
            ownership:
              method: team_field
              project: OCM
              team_field_value: Beta
      - name: Another Pillar
        identifier: another-pillar
        teams:
          - name: Team Gamma
            identifier: team-gamma
            ownership:
              method: jql
              jql: "project = ACM AND component in (Search)"
`

var _ = Describe("Config Loading", func() {

	Describe("LoadFromBytes", func() {
		It("parses a valid resource map YAML", func() {
			rm, err := config.LoadFromBytes([]byte(validYAML))
			Expect(err).NotTo(HaveOccurred())
			Expect(rm).NotTo(BeNil())
			Expect(rm.SprintDurationDays).To(Equal(21))
			Expect(rm.SprintReferenceDate).To(Equal("2026-02-11"))
			Expect(rm.Jira.ScoredIssueTypes).To(ConsistOf("Story", "Bug", "Task"))
			Expect(rm.Jira.RequestDelayMs).To(Equal(100))
			Expect(rm.Organizations).To(HaveLen(1))
			Expect(rm.Organizations[0].Pillars).To(HaveLen(2))
			Expect(rm.Organizations[0].Pillars[0].Teams).To(HaveLen(2))
		})

		It("returns error for invalid YAML", func() {
			_, err := config.LoadFromBytes([]byte(":::invalid"))
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("failed to parse resource map YAML"))
		})

		It("returns error for missing required fields", func() {
			yaml := `
sprint_reference_date: "2026-02-11"
sprint_duration_days: 0
organizations:
  - name: Org
    identifier: org
    pillars:
      - name: Pillar
        identifier: pillar
        teams:
          - name: Team
            identifier: team
            ownership:
              method: unknown
`
			_, err := config.LoadFromBytes([]byte(yaml))
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("sprint_duration_days must be greater than 0"))
			Expect(err.Error()).To(ContainSubstring("not one of"))
		})
	})

	Describe("LoadFromFile", func() {
		It("loads a valid file", func() {
			tmpDir := GinkgoT().TempDir()
			path := filepath.Join(tmpDir, "config.yaml")
			Expect(os.WriteFile(path, []byte(validYAML), 0644)).To(Succeed())

			rm, err := config.LoadFromFile(path)
			Expect(err).NotTo(HaveOccurred())
			Expect(rm).NotTo(BeNil())
			Expect(rm.Organizations).To(HaveLen(1))
		})

		It("returns error for non-existent file", func() {
			_, err := config.LoadFromFile("/nonexistent/path.yaml")
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("failed to read resource map file"))
		})
	})

	Describe("LoadFromBytes (inline YAML)", func() {
		It("loads a resource map from inline YAML bytes", func() {
			rm, err := config.LoadFromBytes([]byte(validYAML))
			Expect(err).NotTo(HaveOccurred())
			Expect(rm).NotTo(BeNil())
			Expect(rm.Organizations).NotTo(BeEmpty())
		})
	})
})
