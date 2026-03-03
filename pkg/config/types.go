package config

// ResourceMap is the top-level configuration structure parsed from the
// resource map YAML file. It defines the organizational hierarchy, Jira
// settings, sprint calendar, and scoring parameters.
type ResourceMap struct {
	Jira                JiraConfig     `yaml:"jira"`
	SprintReferenceDate string         `yaml:"sprint_reference_date"`
	SprintDurationDays  int            `yaml:"sprint_duration_days"`
	Organizations       []Organization `yaml:"organizations"`
}

// JiraConfig holds Jira-specific settings that apply to all teams.
type JiraConfig struct {
	ScoredIssueTypes   []string `yaml:"scored_issue_types"`
	ActivityTypeField  string   `yaml:"activity_type_field"`
	RequestDelayMs     int      `yaml:"request_delay_ms"`
	InProgressStatuses []string `yaml:"in_progress_statuses"`
}

// Organization is the top level of the hierarchy.
type Organization struct {
	Name       string   `yaml:"name"`
	Identifier string   `yaml:"identifier"`
	Pillars    []Pillar `yaml:"pillars"`
}

// Pillar is the middle level of the hierarchy, nested within an Organization.
type Pillar struct {
	Name       string `yaml:"name"`
	Identifier string `yaml:"identifier"`
	Teams      []Team `yaml:"teams"`
}

// Team is the base scoring unit, nested within a Pillar.
type Team struct {
	Name       string    `yaml:"name"`
	Identifier string    `yaml:"identifier"`
	Ownership  Ownership `yaml:"ownership"`
}

// Ownership defines how a team claims ownership of Jira issues.
type Ownership struct {
	Method         string   `yaml:"method"`           // "component", "team_field", "jql", "sprint_board"
	Project        string   `yaml:"project"`          // required for component, team_field, and sprint_board
	Components     []string `yaml:"components"`       // required for component method
	TeamFieldValue string   `yaml:"team_field_value"` // required for team_field method
	JQL            string   `yaml:"jql"`              // required for jql method
	Boards         []int    `yaml:"boards"`           // required for sprint_board method
}

// EntityLevel represents the level of an entity in the hierarchy.
type EntityLevel string

const (
	LevelOrganization EntityLevel = "organization"
	LevelPillar       EntityLevel = "pillar"
	LevelTeam         EntityLevel = "team"
)

// ResolvedEntity represents the result of resolving an identifier, including
// its position in the hierarchy.
type ResolvedEntity struct {
	Level        EntityLevel
	Organization Organization
	Pillar       Pillar // zero value if Level == LevelOrganization
	Team         Team   // zero value if Level != LevelTeam
	Path         string // fully qualified path, e.g. "hcm/rosa/aurora"
}

// Teams returns the teams in scope for this entity.
// Organization: all teams across all pillars.
// Pillar: all teams in the pillar.
// Team: single-element slice containing the team.
func (e *ResolvedEntity) Teams() []Team {
	switch e.Level {
	case LevelOrganization:
		var teams []Team
		for _, p := range e.Organization.Pillars {
			teams = append(teams, p.Teams...)
		}
		return teams
	case LevelPillar:
		return e.Pillar.Teams
	case LevelTeam:
		return []Team{e.Team}
	default:
		return nil
	}
}
