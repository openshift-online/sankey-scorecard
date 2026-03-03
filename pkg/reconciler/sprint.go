package reconciler

import (
	"math"
	"time"
)

// CalculateSprintBoundaries computes the current and previous sprint time
// windows based on a reference date, sprint duration, and the current time.
//
// The algorithm:
//
//	days_since_reference = floor((now - reference_date) / sprint_duration_days)
//	current_sprint_start = reference_date + (days_since_reference * sprint_duration_days)
//	current_sprint_end   = current_sprint_start + sprint_duration_days
//	previous_sprint_start = current_sprint_start - sprint_duration_days
//	previous_sprint_end   = current_sprint_start
func CalculateSprintBoundaries(referenceDate time.Time, durationDays int, now time.Time) (current TimeWindow, previous TimeWindow) {
	daysSinceRef := now.Sub(referenceDate).Hours() / 24.0
	sprintOffset := int(math.Floor(daysSinceRef / float64(durationDays)))

	currentStart := referenceDate.AddDate(0, 0, sprintOffset*durationDays)
	currentEnd := currentStart.AddDate(0, 0, durationDays)
	previousStart := currentStart.AddDate(0, 0, -durationDays)
	previousEnd := currentStart

	current = TimeWindow{Since: currentStart, Until: currentEnd}
	previous = TimeWindow{Since: previousStart, Until: previousEnd}
	return current, previous
}
