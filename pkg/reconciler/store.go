package reconciler

import (
	"sync"
	"time"
)

// ReconciliationStatus represents the state of a reconciliation operation.
type ReconciliationStatus string

const (
	ReconciliationIdle      ReconciliationStatus = "idle"
	ReconciliationRunning   ReconciliationStatus = "running"
	ReconciliationCompleted ReconciliationStatus = "completed"
	ReconciliationFailed    ReconciliationStatus = "failed"
)

// ReconciliationState tracks the status of the most recent reconciliation.
type ReconciliationState struct {
	Status      ReconciliationStatus `json:"status"`
	StartedAt   *time.Time           `json:"started_at"`
	CompletedAt *time.Time           `json:"completed_at"`
	Error       string               `json:"error,omitempty"`
	IssueCount  int                  `json:"issue_count"`
}

// Compile-time assertion that ReconciliationStore implements DataStore.
var _ DataStore = (*ReconciliationStore)(nil)

// ReconciliationStore holds all reconciled Jira data in memory.
// Protected by an RWMutex for concurrent read access during scoring
// and exclusive write access during refresh.
type ReconciliationStore struct {
	mu    sync.RWMutex
	state ReconciliationState
	teams map[string]*TeamData
}

// NewReconciliationStore creates a new store with idle state.
func NewReconciliationStore() *ReconciliationStore {
	return &ReconciliationStore{
		state: ReconciliationState{
			Status: ReconciliationIdle,
		},
		teams: make(map[string]*TeamData),
	}
}

// GetTeamData returns the reconciled data for a specific team.
func (s *ReconciliationStore) GetTeamData(identifier string) (*TeamData, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	td, ok := s.teams[identifier]
	return td, ok
}

// GetAllTeamData returns a copy of the teams map.
func (s *ReconciliationStore) GetAllTeamData() map[string]*TeamData {
	s.mu.RLock()
	defer s.mu.RUnlock()
	result := make(map[string]*TeamData, len(s.teams))
	for k, v := range s.teams {
		result[k] = v
	}
	return result
}

// GetState returns the current reconciliation state.
func (s *ReconciliationStore) GetState() ReconciliationState {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.state
}

// HasData returns true if the store contains any reconciled team data.
func (s *ReconciliationStore) HasData() bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return len(s.teams) > 0
}

// SwapData replaces the entire teams map atomically under a write lock.
func (s *ReconciliationStore) SwapData(teams map[string]*TeamData, issueCount int) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.teams = teams
	s.state.IssueCount = issueCount
}

// UpsertTeamData merges incoming teams into the existing map. Teams already
// present are replaced; teams not in the input are preserved. The total issue
// count is recalculated across all teams.
func (s *ReconciliationStore) UpsertTeamData(teams map[string]*TeamData, issueCount int) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for k, v := range teams {
		s.teams[k] = v
	}
	total := 0
	for _, td := range s.teams {
		for _, p := range td.Periods {
			total += p.TotalCount
		}
	}
	s.state.IssueCount = total
}

// StartRefresh transitions the state to running. Returns false if already running.
func (s *ReconciliationStore) StartRefresh() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.state.Status == ReconciliationRunning {
		return false
	}
	now := time.Now()
	s.state.Status = ReconciliationRunning
	s.state.StartedAt = &now
	s.state.CompletedAt = nil
	s.state.Error = ""
	return true
}

// CompleteRefresh transitions the state from running to completed.
func (s *ReconciliationStore) CompleteRefresh(issueCount int) {
	s.mu.Lock()
	defer s.mu.Unlock()
	now := time.Now()
	s.state.Status = ReconciliationCompleted
	s.state.CompletedAt = &now
	s.state.IssueCount = issueCount
	s.state.Error = ""
}

// FailRefresh transitions the state from running to failed, preserving
// previously reconciled data.
func (s *ReconciliationStore) FailRefresh(err error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	now := time.Now()
	s.state.Status = ReconciliationFailed
	s.state.CompletedAt = &now
	s.state.Error = err.Error()
}
