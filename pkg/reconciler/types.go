package reconciler

import (
	"time"
)

// IssueSetType identifies whether a scoring period contains closed or in-progress issues.
type IssueSetType string

const (
	IssueSetClosed     IssueSetType = "closed"
	IssueSetInProgress IssueSetType = "in_progress"
)

// Issue represents a single reconciled Jira issue containing only the
// fields required for scoring and display.
type Issue struct {
	Key          string    `json:"key"`
	Project      string    `json:"project"`
	IssueType    string    `json:"issue_type"`
	ActivityType string    `json:"activity_type"`
	Status       string    `json:"status"`
	Components   []string  `json:"components"`
	Summary      string    `json:"summary"`
	UpdatedDate  time.Time `json:"updated_date"`
	CreatedDate  time.Time `json:"created_date"`
}

// TeamData holds the reconciled issues for a single team, organized by
// scoring period.
type TeamData struct {
	TeamIdentifier string
	Periods        []ScoringPeriod
	ReconciledAt   time.Time
}

// ScoringPeriod groups issues within a single sprint window for scoring.
type ScoringPeriod struct {
	Window           TimeWindow
	Label            string
	Current          bool
	SetType          IssueSetType
	Issues           []Issue
	TotalCount       int
	CategorizedCount int
	Distribution     ActivityDistribution
}

// TimeWindow represents a time range used as the scoring boundary for a period.
type TimeWindow struct {
	Since time.Time `json:"since"`
	Until time.Time `json:"until"`
}

// ActivityDistribution holds pre-computed issue counts per Sankey category.
type ActivityDistribution struct {
	AssociateWellness    int `json:"associate_wellness"`
	IncidentsSupport     int `json:"incidents_support"`
	SecurityCompliance   int `json:"security_compliance"`
	QualityStability     int `json:"quality_stability"`
	FutureSustainability int `json:"future_sustainability"`
	ProductPortfolio     int `json:"product_portfolio"`
	Uncategorized        int `json:"uncategorized"`
}
