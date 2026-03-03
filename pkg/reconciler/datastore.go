package reconciler

// DataStore defines the interface for storing and retrieving reconciled
// team data. Both the in-memory ReconciliationStore and the PostgreSQL
// PGStore implement this interface.
type DataStore interface {
	GetTeamData(identifier string) (*TeamData, bool)
	GetAllTeamData() map[string]*TeamData
	HasData() bool
	SwapData(teams map[string]*TeamData, issueCount int)
	UpsertTeamData(teams map[string]*TeamData, issueCount int)
	StartRefresh() bool
	CompleteRefresh(issueCount int)
	FailRefresh(err error)
	GetState() ReconciliationState
}
