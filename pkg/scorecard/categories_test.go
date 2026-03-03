package scorecard_test

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/tiwillia/sankey-scorecard/pkg/scorecard"
)

var _ = Describe("MapActivityType", func() {
	DescribeTable("maps known values correctly",
		func(input, expected string) {
			result := scorecard.MapActivityType(input)
			Expect(result).To(Equal(expected))
		},
		Entry("Associate Wellness & Development", "Associate Wellness & Development", scorecard.CategoryAssociateWellness),
		Entry("Incidents & Escalations", "Incidents & Escalations", scorecard.CategoryIncidentsSupport),
		Entry("Customer Support", "Customer Support", scorecard.CategoryIncidentsSupport),
		Entry("Security & Compliance", "Security & Compliance", scorecard.CategorySecurityCompliance),
		Entry("Tech Debt", "Tech Debt", scorecard.CategoryQualityStability),
		Entry("Defect", "Defect", scorecard.CategoryQualityStability),
		Entry("QE Activities", "QE Activities", scorecard.CategoryQualityStability),
		Entry("Quality / Stability / Reliability", "Quality / Stability / Reliability", scorecard.CategoryQualityStability),
		Entry("Future Sustainability", "Future Sustainability", scorecard.CategoryFutureSustainability),
		Entry("Product / Portfolio Work", "Product / Portfolio Work", scorecard.CategoryProductPortfolio),
		Entry("New Feature", "New Feature", scorecard.CategoryProductPortfolio),
		Entry("Feature Enhancement", "Feature Enhancement", scorecard.CategoryProductPortfolio),
	)

	It("returns empty string for unknown values", func() {
		Expect(scorecard.MapActivityType("Unknown Category")).To(BeEmpty())
	})

	It("returns empty string for empty string", func() {
		Expect(scorecard.MapActivityType("")).To(BeEmpty())
	})
})
