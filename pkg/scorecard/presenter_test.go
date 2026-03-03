package scorecard_test

import (
	"encoding/json"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/tiwillia/sankey-scorecard/pkg/reconciler"
	"github.com/tiwillia/sankey-scorecard/pkg/scorecard"
)

var _ = Describe("Presenter", func() {
	Describe("ToJSON", func() {
		It("serializes nil scores as null", func() {
			s := scorecard.Score{
				Total:                 nil,
				Grade:                 "-",
				CategorizationRate:    nil,
				DistributionAlignment: nil,
				IssueCount:            0,
			}
			data, err := scorecard.ToJSON(s)
			Expect(err).NotTo(HaveOccurred())

			var parsed map[string]interface{}
			Expect(json.Unmarshal(data, &parsed)).To(Succeed())
			Expect(parsed["total"]).To(BeNil())
			Expect(parsed["categorization_rate"]).To(BeNil())
			Expect(parsed["distribution_alignment"]).To(BeNil())
			Expect(parsed["grade"]).To(Equal("-"))
		})

		It("serializes non-nil scores as numbers", func() {
			total := 72.5
			catRate := 45.0
			distAlign := 27.5
			s := scorecard.Score{
				Total:                 &total,
				Grade:                 "C",
				CategorizationRate:    &catRate,
				DistributionAlignment: &distAlign,
				IssueCount:            47,
			}
			data, err := scorecard.ToJSON(s)
			Expect(err).NotTo(HaveOccurred())

			var parsed map[string]interface{}
			Expect(json.Unmarshal(data, &parsed)).To(Succeed())
			Expect(parsed["total"]).To(BeNumerically("~", 72.5, 0.01))
			Expect(parsed["grade"]).To(Equal("C"))
		})
	})

	Describe("ToYAML", func() {
		It("serializes a score to YAML", func() {
			total := 72.5
			s := scorecard.Score{
				Total:      &total,
				Grade:      "C",
				IssueCount: 47,
			}
			data, err := scorecard.ToYAML(s)
			Expect(err).NotTo(HaveOccurred())
			Expect(string(data)).To(ContainSubstring("grade: C"))
		})
	})

	Describe("ToPlaintext", func() {
		It("formats a pillar scorecard", func() {
			total := 71.0
			catRate := 43.2
			distAlign := 27.8

			ps := scorecard.PillarScore{
				Name:       "ROSA",
				Identifier: "rosa",
				Path:       "hcm/rosa",
				Score: scorecard.Score{
					Total:                 &total,
					Grade:                 "C",
					CategorizationRate:    &catRate,
					DistributionAlignment: &distAlign,
					IssueCount:            523,
				},
				Teams: []scorecard.TeamScore{
					makeTeamScoreForPresenter("rosa-coffee", 82.5, "B", 89, 50.4, 32.1),
					makeTeamScoreForPresenter("rosa-aurora", 72.5, "C", 47, 45.0, 27.5),
				},
			}

			output := scorecard.ToPlaintext(ps)
			Expect(output).To(ContainSubstring("ROSA Scorecard"))
			Expect(output).To(ContainSubstring("Pillar Score: 71.0 (C)"))
			Expect(output).To(ContainSubstring("43.2 / 70"))
			Expect(output).To(ContainSubstring("27.8 / 30"))
			Expect(output).To(ContainSubstring("rosa-coffee"))
			Expect(output).To(ContainSubstring("rosa-aurora"))
		})

		It("formats a team scorecard with section-based layout", func() {
			total := 72.5
			catRate := 45.0
			distAlign := 27.5

			closedScore := 68.0
			closedGrade := "C"
			closedCat := 42.0
			closedDist := 26.0
			ipScore := 76.8
			ipGrade := "B"
			ipCat := 48.0
			ipDist := 28.8

			ts := scorecard.TeamScore{
				Name:       "Aurora",
				Identifier: "rosa-aurora",
				Path:       "hcm/rosa/rosa-aurora",
				Score: scorecard.Score{
					Total:                 &total,
					Grade:                 "C",
					CategorizationRate:    &catRate,
					DistributionAlignment: &distAlign,
					IssueCount:            47,
				},
				Distribution: reconciler.ActivityDistribution{
					AssociateWellness:    2,
					IncidentsSupport:     8,
					SecurityCompliance:   5,
					QualityStability:     12,
					FutureSustainability: 3,
					ProductPortfolio:     10,
					Uncategorized:        7,
				},
				Sections: []scorecard.SectionScore{
					{
						Type:  "closed",
						Label: "Past (Closed)",
						Score: scorecard.Score{
							Total:                 &closedScore,
							Grade:                 closedGrade,
							CategorizationRate:    &closedCat,
							DistributionAlignment: &closedDist,
							IssueCount:            22,
						},
						Periods: []scorecard.PeriodScore{
							makePeriodScore("2026-01-21 to 2026-02-11", false, 68.0, "C", 22, 42.0, 26.0),
						},
					},
					{
						Type:  "in_progress",
						Label: "Current (In Progress)",
						Score: scorecard.Score{
							Total:                 &ipScore,
							Grade:                 ipGrade,
							CategorizationRate:    &ipCat,
							DistributionAlignment: &ipDist,
							IssueCount:            25,
						},
						Periods: []scorecard.PeriodScore{
							makePeriodScore("In Progress", true, 76.8, "B", 25, 48.0, 28.8),
						},
					},
				},
				Periods: []scorecard.PeriodScore{
					makePeriodScore("2026-01-21 to 2026-02-11", false, 68.0, "C", 22, 42.0, 26.0),
					makePeriodScore("In Progress", true, 76.8, "B", 25, 48.0, 28.8),
				},
			}

			output := scorecard.ToPlaintext(ts)
			Expect(output).To(ContainSubstring("Aurora Scorecard"))
			Expect(output).To(ContainSubstring("Team Score: 72.5 (C)"))
			Expect(output).To(ContainSubstring("Past (Closed)"))
			Expect(output).To(ContainSubstring("Current (In Progress)"))
			Expect(output).To(ContainSubstring("* = current"))
			Expect(output).To(ContainSubstring("Activity Distribution"))
			Expect(output).To(ContainSubstring("target:"))
		})

		It("handles nil scores in plaintext", func() {
			ts := scorecard.TeamScore{
				Name:       "New Team",
				Identifier: "new-team",
				Score: scorecard.Score{
					Total: nil,
					Grade: "-",
				},
				Distribution: reconciler.ActivityDistribution{},
			}

			output := scorecard.ToPlaintext(ts)
			Expect(output).To(ContainSubstring("no data"))
		})
	})
})

func makeTeamScoreForPresenter(id string, total float64, grade string, issues int, catRate, distAlign float64) scorecard.TeamScore {
	return scorecard.TeamScore{
		Identifier: id,
		Score: scorecard.Score{
			Total:                 &total,
			Grade:                 grade,
			CategorizationRate:    &catRate,
			DistributionAlignment: &distAlign,
			IssueCount:            issues,
		},
	}
}

func makePeriodScore(label string, current bool, total float64, grade string, issues int, catRate, distAlign float64) scorecard.PeriodScore {
	return scorecard.PeriodScore{
		Label:   label,
		Current: current,
		Score: scorecard.Score{
			Total:                 &total,
			Grade:                 grade,
			CategorizationRate:    &catRate,
			DistributionAlignment: &distAlign,
			IssueCount:            issues,
		},
	}
}
