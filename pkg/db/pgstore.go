package db

import (
	"sync"
	"time"

	"gorm.io/driver/postgres"
	"gorm.io/gorm"

	"github.com/tiwillia/sankey-scorecard/pkg/reconciler"
)

// Compile-time assertion that PGStore implements DataStore.
var _ reconciler.DataStore = (*PGStore)(nil)

// PGStore implements reconciler.DataStore backed by PostgreSQL via GORM.
type PGStore struct {
	db *gorm.DB
	mu sync.RWMutex // protects in-process state transitions
}

// NewPGStore connects to PostgreSQL, runs AutoMigrate, and returns a ready store.
func NewPGStore(dsn string) (*PGStore, error) {
	db, err := gorm.Open(postgres.Open(dsn), &gorm.Config{})
	if err != nil {
		return nil, err
	}

	if err := db.AutoMigrate(
		&ReconciliationStateModel{},
		&TeamModel{},
		&ScoringPeriodModel{},
		&IssueModel{},
	); err != nil {
		return nil, err
	}

	// Ensure the singleton reconciliation state row exists.
	db.FirstOrCreate(&ReconciliationStateModel{}, ReconciliationStateModel{ID: 1})

	return &PGStore{db: db}, nil
}

// Close closes the underlying database connection.
func (s *PGStore) Close() error {
	sqlDB, err := s.db.DB()
	if err != nil {
		return err
	}
	return sqlDB.Close()
}

// GetTeamData returns the reconciled data for a specific team.
func (s *PGStore) GetTeamData(identifier string) (*reconciler.TeamData, bool) {
	var team TeamModel
	result := s.db.Preload("Periods.Issues").First(&team, "identifier = ?", identifier)
	if result.Error != nil {
		return nil, false
	}
	return toTeamData(&team), true
}

// GetAllTeamData returns all reconciled team data.
func (s *PGStore) GetAllTeamData() map[string]*reconciler.TeamData {
	var teams []TeamModel
	s.db.Preload("Periods.Issues").Find(&teams)

	result := make(map[string]*reconciler.TeamData, len(teams))
	for i := range teams {
		td := toTeamData(&teams[i])
		result[td.TeamIdentifier] = td
	}
	return result
}

// HasData returns true if the store contains any reconciled team data.
func (s *PGStore) HasData() bool {
	var count int64
	s.db.Model(&TeamModel{}).Count(&count)
	return count > 0
}

// SwapData replaces all team data in a single transaction.
func (s *PGStore) SwapData(teams map[string]*reconciler.TeamData, issueCount int) {
	s.db.Transaction(func(tx *gorm.DB) error {
		// Delete all existing teams (cascades to periods and issues).
		tx.Where("1 = 1").Delete(&TeamModel{})

		for _, td := range teams {
			m := fromTeamData(td)
			tx.Create(m)
		}

		tx.Model(&ReconciliationStateModel{}).Where("id = 1").Update("issue_count", issueCount)
		return nil
	})
}

// UpsertTeamData merges incoming teams into the existing data. Teams already
// present have their periods replaced; teams not in the input are preserved.
func (s *PGStore) UpsertTeamData(teams map[string]*reconciler.TeamData, issueCount int) {
	s.db.Transaction(func(tx *gorm.DB) error {
		for _, td := range teams {
			// Delete existing periods for this team (cascades to issues).
			tx.Where("team_identifier = ?", td.TeamIdentifier).Delete(&ScoringPeriodModel{})

			m := fromTeamData(td)
			// Upsert the team row and create new periods.
			tx.Save(m)
		}

		// Recount total issues across all teams.
		var total int64
		tx.Model(&ScoringPeriodModel{}).Select("COALESCE(SUM(total_count), 0)").Scan(&total)
		tx.Model(&ReconciliationStateModel{}).Where("id = 1").Update("issue_count", total)
		return nil
	})
}

// StartRefresh transitions the state to running. Returns false if already running.
func (s *PGStore) StartRefresh() bool {
	s.mu.Lock()
	defer s.mu.Unlock()

	now := time.Now()
	result := s.db.Model(&ReconciliationStateModel{}).
		Where("id = 1 AND status != 'running'").
		Updates(map[string]interface{}{
			"status":       string(reconciler.ReconciliationRunning),
			"started_at":   now,
			"completed_at": nil,
			"error":        "",
		})
	return result.RowsAffected > 0
}

// CompleteRefresh transitions the state from running to completed.
func (s *PGStore) CompleteRefresh(issueCount int) {
	now := time.Now()
	s.db.Model(&ReconciliationStateModel{}).Where("id = 1").
		Updates(map[string]interface{}{
			"status":       string(reconciler.ReconciliationCompleted),
			"completed_at": now,
			"issue_count":  issueCount,
			"error":        "",
		})
}

// FailRefresh transitions the state from running to failed.
func (s *PGStore) FailRefresh(err error) {
	now := time.Now()
	s.db.Model(&ReconciliationStateModel{}).Where("id = 1").
		Updates(map[string]interface{}{
			"status":       string(reconciler.ReconciliationFailed),
			"completed_at": now,
			"error":        err.Error(),
		})
}

// GetState returns the current reconciliation state.
func (s *PGStore) GetState() reconciler.ReconciliationState {
	var model ReconciliationStateModel
	s.db.First(&model, 1)
	return reconciler.ReconciliationState{
		Status:      reconciler.ReconciliationStatus(model.Status),
		StartedAt:   model.StartedAt,
		CompletedAt: model.CompletedAt,
		Error:       model.Error,
		IssueCount:  model.IssueCount,
	}
}
