package db

import (
	"time"

	"github.com/lib/pq"

	"github.com/tiwillia/sankey-scorecard/pkg/reconciler"
)

// ReconciliationStateModel is the GORM model for the singleton reconciliation state row.
type ReconciliationStateModel struct {
	ID          uint       `gorm:"primaryKey;autoIncrement:false;default:1;check:id = 1"`
	Status      string     `gorm:"not null;default:'idle'"`
	StartedAt   *time.Time
	CompletedAt *time.Time
	Error       string
	IssueCount  int `gorm:"not null;default:0"`
}

func (ReconciliationStateModel) TableName() string {
	return "reconciliation_state"
}

// TeamModel is the GORM model for a reconciled team.
type TeamModel struct {
	Identifier   string               `gorm:"primaryKey"`
	ReconciledAt time.Time            `gorm:"not null"`
	Periods      []ScoringPeriodModel `gorm:"foreignKey:TeamIdentifier;references:Identifier;constraint:OnDelete:CASCADE"`
}

func (TeamModel) TableName() string {
	return "teams"
}

// ScoringPeriodModel is the GORM model for a scoring period within a team.
type ScoringPeriodModel struct {
	ID                     uint   `gorm:"primaryKey;autoIncrement"`
	TeamIdentifier         string `gorm:"not null;index;uniqueIndex:idx_team_label_type"`
	WindowSince            time.Time  `gorm:"not null"`
	WindowUntil            time.Time  `gorm:"not null"`
	Label                  string     `gorm:"not null;uniqueIndex:idx_team_label_type"`
	CurrentPeriod          bool       `gorm:"not null;default:false"`
	SetType                string     `gorm:"not null;uniqueIndex:idx_team_label_type"`
	TotalCount             int        `gorm:"not null;default:0"`
	CategorizedCount       int        `gorm:"not null;default:0"`
	DistAssociateWellness  int        `gorm:"not null;default:0"`
	DistIncidentsSupport   int        `gorm:"not null;default:0"`
	DistSecurityCompliance int        `gorm:"not null;default:0"`
	DistQualityStability   int        `gorm:"not null;default:0"`
	DistFutureSustain      int        `gorm:"not null;default:0"`
	DistProductPortfolio   int        `gorm:"not null;default:0"`
	DistUncategorized      int        `gorm:"not null;default:0"`
	Issues                 []IssueModel `gorm:"foreignKey:PeriodID;constraint:OnDelete:CASCADE"`
}

func (ScoringPeriodModel) TableName() string {
	return "scoring_periods"
}

// IssueModel is the GORM model for a single reconciled Jira issue.
type IssueModel struct {
	ID           uint           `gorm:"primaryKey;autoIncrement"`
	PeriodID     uint           `gorm:"not null;index"`
	IssueKey     string         `gorm:"not null"`
	Project      string         `gorm:"not null"`
	IssueType    string         `gorm:"not null"`
	ActivityType string
	Status       string         `gorm:"not null"`
	Components   pq.StringArray `gorm:"type:text[]"`
	Summary      string
	UpdatedDate  time.Time `gorm:"not null"`
	CreatedDate  time.Time `gorm:"not null"`
}

func (IssueModel) TableName() string {
	return "issues"
}

// toTeamData converts a TeamModel to a reconciler.TeamData.
func toTeamData(m *TeamModel) *reconciler.TeamData {
	td := &reconciler.TeamData{
		TeamIdentifier: m.Identifier,
		ReconciledAt:   m.ReconciledAt,
	}
	for _, pm := range m.Periods {
		td.Periods = append(td.Periods, toScoringPeriod(&pm))
	}
	return td
}

// toScoringPeriod converts a ScoringPeriodModel to a reconciler.ScoringPeriod.
func toScoringPeriod(m *ScoringPeriodModel) reconciler.ScoringPeriod {
	sp := reconciler.ScoringPeriod{
		Window: reconciler.TimeWindow{
			Since: m.WindowSince,
			Until: m.WindowUntil,
		},
		Label:            m.Label,
		Current:          m.CurrentPeriod,
		SetType:          reconciler.IssueSetType(m.SetType),
		TotalCount:       m.TotalCount,
		CategorizedCount: m.CategorizedCount,
		Distribution: reconciler.ActivityDistribution{
			AssociateWellness:    m.DistAssociateWellness,
			IncidentsSupport:    m.DistIncidentsSupport,
			SecurityCompliance:   m.DistSecurityCompliance,
			QualityStability:     m.DistQualityStability,
			FutureSustainability: m.DistFutureSustain,
			ProductPortfolio:     m.DistProductPortfolio,
			Uncategorized:        m.DistUncategorized,
		},
	}
	for _, im := range m.Issues {
		sp.Issues = append(sp.Issues, toIssue(&im))
	}
	return sp
}

// toIssue converts an IssueModel to a reconciler.Issue.
func toIssue(m *IssueModel) reconciler.Issue {
	return reconciler.Issue{
		Key:          m.IssueKey,
		Project:      m.Project,
		IssueType:    m.IssueType,
		ActivityType: m.ActivityType,
		Status:       m.Status,
		Components:   []string(m.Components),
		Summary:      m.Summary,
		UpdatedDate:  m.UpdatedDate,
		CreatedDate:  m.CreatedDate,
	}
}

// fromTeamData converts a reconciler.TeamData to a TeamModel.
func fromTeamData(td *reconciler.TeamData) *TeamModel {
	m := &TeamModel{
		Identifier:   td.TeamIdentifier,
		ReconciledAt: td.ReconciledAt,
	}
	for _, sp := range td.Periods {
		m.Periods = append(m.Periods, fromScoringPeriod(td.TeamIdentifier, &sp))
	}
	return m
}

// fromScoringPeriod converts a reconciler.ScoringPeriod to a ScoringPeriodModel.
func fromScoringPeriod(teamID string, sp *reconciler.ScoringPeriod) ScoringPeriodModel {
	pm := ScoringPeriodModel{
		TeamIdentifier:         teamID,
		WindowSince:            sp.Window.Since,
		WindowUntil:            sp.Window.Until,
		Label:                  sp.Label,
		CurrentPeriod:          sp.Current,
		SetType:                string(sp.SetType),
		TotalCount:             sp.TotalCount,
		CategorizedCount:       sp.CategorizedCount,
		DistAssociateWellness:  sp.Distribution.AssociateWellness,
		DistIncidentsSupport:   sp.Distribution.IncidentsSupport,
		DistSecurityCompliance: sp.Distribution.SecurityCompliance,
		DistQualityStability:   sp.Distribution.QualityStability,
		DistFutureSustain:      sp.Distribution.FutureSustainability,
		DistProductPortfolio:   sp.Distribution.ProductPortfolio,
		DistUncategorized:      sp.Distribution.Uncategorized,
	}
	for _, issue := range sp.Issues {
		pm.Issues = append(pm.Issues, fromIssue(&issue))
	}
	return pm
}

// fromIssue converts a reconciler.Issue to an IssueModel.
func fromIssue(issue *reconciler.Issue) IssueModel {
	return IssueModel{
		IssueKey:     issue.Key,
		Project:      issue.Project,
		IssueType:    issue.IssueType,
		ActivityType: issue.ActivityType,
		Status:       issue.Status,
		Components:   pq.StringArray(issue.Components),
		Summary:      issue.Summary,
		UpdatedDate:  issue.UpdatedDate,
		CreatedDate:  issue.CreatedDate,
	}
}
