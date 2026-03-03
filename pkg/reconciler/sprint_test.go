package reconciler_test

import (
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/tiwillia/sankey-scorecard/pkg/reconciler"
)

var _ = Describe("Sprint Calendar", func() {
	refDate := time.Date(2026, 2, 11, 0, 0, 0, 0, time.UTC)
	duration := 21

	DescribeTable("calculates correct sprint boundaries",
		func(nowStr string, expectedCurrentStart, expectedCurrentEnd, expectedPrevStart, expectedPrevEnd string) {
			now, err := time.Parse("2006-01-02", nowStr)
			Expect(err).NotTo(HaveOccurred())

			current, previous := reconciler.CalculateSprintBoundaries(refDate, duration, now)

			Expect(current.Since.Format("2006-01-02")).To(Equal(expectedCurrentStart))
			Expect(current.Until.Format("2006-01-02")).To(Equal(expectedCurrentEnd))
			Expect(previous.Since.Format("2006-01-02")).To(Equal(expectedPrevStart))
			Expect(previous.Until.Format("2006-01-02")).To(Equal(expectedPrevEnd))
		},
		// Examples from the spec
		Entry("2026-02-07 (before reference date)",
			"2026-02-07",
			"2026-01-21", "2026-02-11",
			"2025-12-31", "2026-01-21"),
		Entry("2026-02-15 (after reference date)",
			"2026-02-15",
			"2026-02-11", "2026-03-04",
			"2026-01-21", "2026-02-11"),
		Entry("2026-03-10 (two sprints after reference)",
			"2026-03-10",
			"2026-03-04", "2026-03-25",
			"2026-02-11", "2026-03-04"),
	)

	It("handles now exactly on reference date", func() {
		current, previous := reconciler.CalculateSprintBoundaries(refDate, duration, refDate)
		Expect(current.Since.Format("2006-01-02")).To(Equal("2026-02-11"))
		Expect(current.Until.Format("2006-01-02")).To(Equal("2026-03-04"))
		Expect(previous.Since.Format("2006-01-02")).To(Equal("2026-01-21"))
		Expect(previous.Until.Format("2006-01-02")).To(Equal("2026-02-11"))
	})

	It("handles now before reference date (negative offset)", func() {
		earlyDate := time.Date(2025, 12, 15, 0, 0, 0, 0, time.UTC)
		current, previous := reconciler.CalculateSprintBoundaries(refDate, duration, earlyDate)
		// Should project backwards
		Expect(current.Since.Before(refDate)).To(BeTrue())
		Expect(previous.Until.Equal(current.Since)).To(BeTrue())
		// Sprint duration should be maintained
		diff := current.Until.Sub(current.Since)
		Expect(int(diff.Hours()/24)).To(Equal(duration))
	})

	It("handles now exactly on sprint boundary end", func() {
		boundaryDate := time.Date(2026, 3, 4, 0, 0, 0, 0, time.UTC)
		current, previous := reconciler.CalculateSprintBoundaries(refDate, duration, boundaryDate)
		Expect(current.Since.Format("2006-01-02")).To(Equal("2026-03-04"))
		Expect(previous.Until.Format("2006-01-02")).To(Equal("2026-03-04"))
	})
})
