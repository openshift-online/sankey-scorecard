package scorecard

import (
	"time"

	"github.com/tiwillia/sankey-scorecard/pkg/reconciler"
)

// FilterOptions controls which scoring periods are included in scorecard computation.
type FilterOptions struct {
	StartDate   *time.Time
	EndDate     *time.Time
	IssueStatus string // "closed", "in_progress", or "" (both)
}

// ActiveFilters echoes the applied filter parameters in the API response.
type ActiveFilters struct {
	StartDate   *string `json:"start_date,omitempty"`
	EndDate     *string `json:"end_date,omitempty"`
	IssueStatus *string `json:"issue_status,omitempty"`
}

// FullScorecard is the top-level response for scorecard queries.
type FullScorecard struct {
	GeneratedAt        time.Time              `json:"generated_at"`
	SprintDurationDays int                    `json:"sprint_duration_days"`
	CurrentSprint      reconciler.TimeWindow  `json:"current_sprint"`
	PreviousSprint     reconciler.TimeWindow  `json:"previous_sprint"`
	ActiveFilters      *ActiveFilters         `json:"active_filters,omitempty"`
	Organizations      []OrganizationScore    `json:"organizations"`
}

// OrganizationScore is the scorecard for a single organization.
type OrganizationScore struct {
	Name       string        `json:"name"`
	Identifier string        `json:"identifier"`
	Path       string        `json:"path"`
	Score      Score         `json:"score"`
	Pillars    []PillarScore `json:"pillars"`
}

// PillarScore is the scorecard for a single pillar within an organization.
type PillarScore struct {
	Name       string      `json:"name"`
	Identifier string      `json:"identifier"`
	Path       string      `json:"path"`
	Score      Score       `json:"score"`
	Teams      []TeamScore `json:"teams"`
}

// TeamScore is the scorecard for a single team.
type TeamScore struct {
	Name         string                         `json:"name"`
	Identifier   string                         `json:"identifier"`
	Path         string                         `json:"path"`
	Score        Score                          `json:"score"`
	Distribution reconciler.ActivityDistribution `json:"distribution"`
	Sections     []SectionScore                 `json:"sections"`
	Periods      []PeriodScore                  `json:"periods"`
}

// SectionScore groups period scores by issue set type (closed or in-progress).
type SectionScore struct {
	Type         string                         `json:"type"`
	Label        string                         `json:"label"`
	Score        Score                          `json:"score"`
	Distribution reconciler.ActivityDistribution `json:"distribution"`
	Periods      []PeriodScore                  `json:"periods"`
}

// PeriodScore is the score for a single scoring period (sprint).
type PeriodScore struct {
	Label        string                         `json:"label"`
	Window       reconciler.TimeWindow          `json:"window"`
	Current      bool                           `json:"current"`
	SetType      string                         `json:"set_type"`
	Score        Score                          `json:"score"`
	Distribution reconciler.ActivityDistribution `json:"distribution"`
}

// Score is the scoring breakdown, present at every level.
// Nullable fields use *float64 to distinguish nil (no data) from 0.
type Score struct {
	Total                 *float64 `json:"total"`
	Grade                 string   `json:"grade"`
	CategorizationRate    *float64 `json:"categorization_rate"`
	DistributionAlignment *float64 `json:"distribution_alignment"`
	IssueCount            int      `json:"issue_count"`
}
