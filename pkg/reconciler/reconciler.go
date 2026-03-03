package reconciler

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	jira "github.com/andygrunwald/go-jira/v2/onpremise"

	"github.com/tiwillia/sankey-scorecard/pkg/config"
)

// JiraClient is the interface for Jira search operations, allowing mocking in tests.
type JiraClient interface {
	Search(ctx context.Context, jql string, opts *jira.SearchOptions) ([]jira.Issue, *jira.Response, error)
}

// RefreshMode controls how fetched data is applied to the store.
type RefreshMode string

const (
	// RefreshModeReplace replaces all data in the store (SwapData).
	RefreshModeReplace RefreshMode = "replace"
	// RefreshModeUpsert merges fetched teams into existing data (UpsertTeamData).
	RefreshModeUpsert RefreshMode = "upsert"
)

// RefreshScope limits a refresh to a subset of data. When set, only data
// matching the scope is fetched and merged into the store via upsert.
type RefreshScope struct {
	StartDate *time.Time
	EndDate   *time.Time
	Status    string // "closed", "in_progress", or "" (both)
}

// Reconciler fetches Jira data and populates the reconciliation store.
type Reconciler struct {
	client            JiraClient
	config            *config.ResourceMap
	store             DataStore
	activityTypeField string
	sinceOverride     *time.Time
	sprintFetcher     BoardSprintFetcher
	refreshMode       RefreshMode
	refreshScope      *RefreshScope
}

// NewReconciler creates a new Reconciler.
func NewReconciler(client JiraClient, cfg *config.ResourceMap, store DataStore, activityTypeField string, sprintFetcher BoardSprintFetcher) *Reconciler {
	if sprintFetcher == nil {
		sprintFetcher = &NoOpSprintFetcher{}
	}
	return &Reconciler{
		client:            client,
		config:            cfg,
		store:             store,
		activityTypeField: activityTypeField,
		sprintFetcher:     sprintFetcher,
	}
}

// SetRefreshMode sets how fetched data is applied to the store.
func (r *Reconciler) SetRefreshMode(mode RefreshMode) {
	r.refreshMode = mode
}

// SetRefreshScope sets a scope that limits the next refresh to a subset of data.
// When scoped, the reconciler forces upsert mode to preserve out-of-scope data.
func (r *Reconciler) SetRefreshScope(scope *RefreshScope) {
	r.refreshScope = scope
}

// SetSinceOverride sets a custom start date that overrides the sprint calendar.
// When set, a single scoring period is used from the given date to now.
func (r *Reconciler) SetSinceOverride(since time.Time) {
	r.sinceOverride = &since
}

// Refresh fetches Jira data for all configured teams and updates the store.
func (r *Reconciler) Refresh(ctx context.Context) error {
	var allTeams []config.Team
	for _, org := range r.config.Organizations {
		for _, pillar := range org.Pillars {
			allTeams = append(allTeams, pillar.Teams...)
		}
	}
	return r.RefreshTeams(ctx, allTeams)
}

// Cadence represents a time window within the sprint calendar for scoring.
type Cadence struct {
	Window  TimeWindow
	Label   string
	Current bool
}

// RefreshTeams fetches Jira data for the specified teams and updates the store.
// Manages the full refresh lifecycle: StartRefresh -> fetch -> CompleteRefresh/FailRefresh.
func (r *Reconciler) RefreshTeams(ctx context.Context, teams []config.Team) error {
	if !r.store.StartRefresh() {
		return fmt.Errorf("refresh is already running")
	}
	return r.executeRefresh(ctx, teams)
}

// ExecuteRefresh performs the refresh for all configured teams without calling
// StartRefresh. The caller must have already called store.StartRefresh().
// Handles CompleteRefresh/FailRefresh on completion.
func (r *Reconciler) ExecuteRefresh(ctx context.Context) error {
	var allTeams []config.Team
	for _, org := range r.config.Organizations {
		for _, pillar := range org.Pillars {
			allTeams = append(allTeams, pillar.Teams...)
		}
	}
	return r.executeRefresh(ctx, allTeams)
}

// executeRefresh is the internal implementation that fetches data and updates
// the store. Assumes StartRefresh has already been called.
func (r *Reconciler) executeRefresh(ctx context.Context, teams []config.Team) error {
	slog.Info("refresh started", "team_count", len(teams))
	start := time.Now()

	// Set overall timeout
	ctx, cancel := context.WithTimeout(ctx, 10*time.Minute)
	defer cancel()

	// Calculate cadences (time windows for closed issues)
	cadences := r.calculateCadences()

	// Scope-aware cadence override: when scope has date params, replace
	// cadences with a single cadence covering the specified range.
	if r.refreshScope != nil && (r.refreshScope.StartDate != nil || r.refreshScope.EndDate != nil) {
		now := time.Now()
		refDate, _ := time.Parse("2006-01-02", r.config.SprintReferenceDate)
		current, _ := CalculateSprintBoundaries(refDate, r.config.SprintDurationDays, now)

		scopeStart := current.Since // default: start of current sprint
		if r.refreshScope.StartDate != nil {
			scopeStart = *r.refreshScope.StartDate
		}
		scopeEnd := now
		if r.refreshScope.EndDate != nil {
			scopeEnd = *r.refreshScope.EndDate
		}

		cadences = []Cadence{
			{
				Window:  TimeWindow{Since: scopeStart, Until: scopeEnd},
				Label:   fmt.Sprintf("%s to %s", scopeStart.Format("2006-01-02"), scopeEnd.Format("2006-01-02")),
				Current: true,
			},
		}
	}

	// Determine which set types are being refreshed based on scope
	refreshClosed := true
	refreshInProgress := true
	if r.refreshScope != nil && r.refreshScope.Status != "" {
		refreshClosed = r.refreshScope.Status == "closed"
		refreshInProgress = r.refreshScope.Status == "in_progress"
	}

	// Fetch data for specified teams
	teamDataMap := make(map[string]*TeamData)
	totalIssues := 0

	for _, team := range teams {
		slog.Info("fetching team data", "team", team.Identifier, "method", team.Ownership.Method)
		teamData, err := r.fetchTeamDataScoped(ctx, team, cadences, refreshClosed, refreshInProgress)
		if err != nil {
			slog.Error("team fetch failed", "team", team.Identifier, "error", err)
			r.store.FailRefresh(fmt.Errorf("failed to fetch data for team %s: %w", team.Identifier, err))
			return err
		}

		// Period-level merge: preserve existing periods whose SetType was not refreshed
		if r.refreshScope != nil {
			r.mergeExistingPeriods(teamData, refreshClosed, refreshInProgress)
		}

		teamIssues := 0
		for _, p := range teamData.Periods {
			teamIssues += p.TotalCount
		}
		slog.Info("team fetch complete", "team", team.Identifier, "issues", teamIssues)
		teamDataMap[team.Identifier] = teamData
		totalIssues += teamIssues
	}

	// Apply data to store: scoped refresh forces upsert
	if r.refreshScope != nil || r.refreshMode == RefreshModeUpsert {
		r.store.UpsertTeamData(teamDataMap, totalIssues)
	} else {
		r.store.SwapData(teamDataMap, totalIssues)
	}
	r.store.CompleteRefresh(totalIssues)
	slog.Info("refresh complete", "total_issues", totalIssues, "teams", len(teamDataMap), "duration", time.Since(start).Round(time.Millisecond))
	return nil
}

// mergeExistingPeriods preserves periods from the store whose SetType was not
// refreshed. For example, if only closed data was refreshed, existing
// in-progress periods are carried forward.
func (r *Reconciler) mergeExistingPeriods(teamData *TeamData, refreshedClosed, refreshedInProgress bool) {
	existing, ok := r.store.GetTeamData(teamData.TeamIdentifier)
	if !ok {
		return
	}
	for _, p := range existing.Periods {
		if p.SetType == IssueSetClosed && !refreshedClosed {
			teamData.Periods = append(teamData.Periods, p)
		}
		if p.SetType == IssueSetInProgress && !refreshedInProgress {
			teamData.Periods = append(teamData.Periods, p)
		}
	}
}

func (r *Reconciler) calculateCadences() []Cadence {
	now := time.Now()

	if r.sinceOverride != nil {
		return []Cadence{
			{
				Window:  TimeWindow{Since: *r.sinceOverride, Until: now},
				Label:   fmt.Sprintf("%s to %s", r.sinceOverride.Format("2006-01-02"), now.Format("2006-01-02")),
				Current: true,
			},
		}
	}

	refDate, _ := time.Parse("2006-01-02", r.config.SprintReferenceDate)
	current, previous := CalculateSprintBoundaries(refDate, r.config.SprintDurationDays, now)

	return []Cadence{
		{
			Window:  previous,
			Label:   fmt.Sprintf("%s to %s", previous.Since.Format("2006-01-02"), previous.Until.Format("2006-01-02")),
			Current: false,
		},
		{
			Window:  current,
			Label:   fmt.Sprintf("%s to %s", current.Since.Format("2006-01-02"), current.Until.Format("2006-01-02")),
			Current: true,
		},
	}
}

func (r *Reconciler) fetchTeamDataScoped(ctx context.Context, team config.Team, cadences []Cadence, fetchClosed, fetchInProgress bool) (*TeamData, error) {
	// Per-team timeout
	ctx, cancel := context.WithTimeout(ctx, 60*time.Second)
	defer cancel()

	teamData := &TeamData{
		TeamIdentifier: team.Identifier,
		ReconciledAt:   time.Now(),
	}

	// For sprint_board teams, discover sprint IDs from the Agile API
	var sprintIDs []int
	if team.Ownership.Method == "sprint_board" {
		var err error
		sprintIDs, err = r.discoverSprintIDs(ctx, team.Ownership.Boards)
		if err != nil {
			return nil, fmt.Errorf("failed to discover sprints for team %s: %w", team.Identifier, err)
		}
	}

	// Fetch closed issues for each cadence (skip if scope excludes closed)
	if fetchClosed {
		for _, cadence := range cadences {
			period, err := r.fetchClosedPeriod(ctx, team, cadence, sprintIDs)
			if err != nil {
				return nil, err
			}
			teamData.Periods = append(teamData.Periods, *period)
		}
	}

	// Fetch in-progress issues (skip if scope excludes in-progress)
	if fetchInProgress {
		inProgressPeriod, err := r.fetchInProgressPeriod(ctx, team, sprintIDs)
		if err != nil {
			return nil, err
		}
		teamData.Periods = append(teamData.Periods, *inProgressPeriod)
	}

	return teamData, nil
}

func (r *Reconciler) discoverSprintIDs(ctx context.Context, boardIDs []int) ([]int, error) {
	var allIDs []int
	for _, boardID := range boardIDs {
		sprints, err := r.sprintFetcher.FetchBoardSprints(ctx, boardID)
		if err != nil {
			return nil, err
		}
		for _, s := range sprints {
			allIDs = append(allIDs, s.ID)
		}
	}
	return allIDs, nil
}

func (r *Reconciler) fetchClosedPeriod(ctx context.Context, team config.Team, cadence Cadence, sprintIDs []int) (*ScoringPeriod, error) {
	jql := BuildClosedJQL(team, r.config.Jira.ScoredIssueTypes, cadence.Window, sprintIDs)

	issues, err := r.fetchAllIssues(ctx, jql)
	if err != nil {
		return nil, fmt.Errorf("closed JQL query failed for team %s: %w", team.Identifier, err)
	}

	period := &ScoringPeriod{
		Window:  cadence.Window,
		Label:   cadence.Label,
		Current: cadence.Current,
		SetType: IssueSetClosed,
	}

	categorizeAndAdd(period, issues, r)
	return period, nil
}

func (r *Reconciler) fetchInProgressPeriod(ctx context.Context, team config.Team, sprintIDs []int) (*ScoringPeriod, error) {
	jql := BuildInProgressJQL(team, r.config.Jira.ScoredIssueTypes, r.config.Jira.InProgressStatuses, sprintIDs)

	issues, err := r.fetchAllIssues(ctx, jql)
	if err != nil {
		return nil, fmt.Errorf("in-progress JQL query failed for team %s: %w", team.Identifier, err)
	}

	now := time.Now()
	period := &ScoringPeriod{
		Window:  TimeWindow{Since: now, Until: now},
		Label:   "In Progress",
		Current: true,
		SetType: IssueSetInProgress,
	}

	categorizeAndAdd(period, issues, r)
	return period, nil
}

// categorizeAndAdd processes fetched Jira issues into a scoring period.
func categorizeAndAdd(period *ScoringPeriod, issues []jira.Issue, r *Reconciler) {
	for _, jiraIssue := range issues {
		issue := r.transformIssue(jiraIssue)
		period.Issues = append(period.Issues, issue)
		period.TotalCount++

		category := mapActivityTypeToCategory(issue.ActivityType)
		if category != "" {
			period.CategorizedCount++
			addToDistribution(&period.Distribution, category)
		} else {
			period.Distribution.Uncategorized++
		}
	}
}

func (r *Reconciler) fetchAllIssues(ctx context.Context, jql string) ([]jira.Issue, error) {
	var allIssues []jira.Issue
	startAt := 0
	maxResults := 100

	delay := time.Duration(r.config.Jira.RequestDelayMs) * time.Millisecond

	for {
		if startAt > 0 && delay > 0 {
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			case <-time.After(delay):
			}
		}

		issues, resp, err := r.client.Search(ctx, jql, &jira.SearchOptions{
			StartAt:    startAt,
			MaxResults: maxResults,
		})
		if err != nil {
			// Handle rate limiting with exponential backoff
			if resp != nil && resp.StatusCode == 429 {
				backoff := delay * 2
				if backoff < time.Second {
					backoff = time.Second
				}
				select {
				case <-ctx.Done():
					return nil, ctx.Err()
				case <-time.After(backoff):
				}
				continue
			}
			return nil, err
		}

		allIssues = append(allIssues, issues...)

		if len(allIssues) >= resp.Total {
			break
		}
		startAt += len(issues)
	}

	return allIssues, nil
}

func (r *Reconciler) transformIssue(jiraIssue jira.Issue) Issue {
	issue := Issue{
		Key: jiraIssue.Key,
	}

	if jiraIssue.Fields != nil {
		if jiraIssue.Fields.Project.Key != "" {
			issue.Project = jiraIssue.Fields.Project.Key
		}
		if jiraIssue.Fields.Type.Name != "" {
			issue.IssueType = jiraIssue.Fields.Type.Name
		}
		if jiraIssue.Fields.Status != nil {
			issue.Status = jiraIssue.Fields.Status.Name
		}
		issue.Summary = jiraIssue.Fields.Summary

		for _, c := range jiraIssue.Fields.Components {
			if c != nil {
				issue.Components = append(issue.Components, c.Name)
			}
		}

		issue.UpdatedDate = time.Time(jiraIssue.Fields.Updated)
		issue.CreatedDate = time.Time(jiraIssue.Fields.Created)

		issue.ActivityType = ExtractActivityType(jiraIssue.Fields.Unknowns, r.activityTypeField)
	}

	return issue
}

// mapActivityTypeToCategory maps a Jira Activity Type value to a Sankey category.
// Returns empty string for uncategorized/unknown values.
func mapActivityTypeToCategory(activityType string) string {
	// This mapping is hardcoded per the design document.
	// The scorecard package has the canonical category constants,
	// but we use string matching here to avoid circular imports.
	switch activityType {
	case "Associate Wellness & Development":
		return "Associate Wellness & Development"
	case "Incidents & Escalations", "Customer Support":
		return "Incidents & Support"
	case "Security & Compliance":
		return "Security & Compliance"
	case "Tech Debt", "Defect", "QE Activities", "Quality / Stability / Reliability":
		return "Quality / Stability / Reliability"
	case "Future Sustainability":
		return "Future Sustainability"
	case "Product / Portfolio Work", "New Feature", "Feature Enhancement":
		return "Product / Portfolio Work"
	default:
		return ""
	}
}

func addToDistribution(dist *ActivityDistribution, category string) {
	switch category {
	case "Associate Wellness & Development":
		dist.AssociateWellness++
	case "Incidents & Support":
		dist.IncidentsSupport++
	case "Security & Compliance":
		dist.SecurityCompliance++
	case "Quality / Stability / Reliability":
		dist.QualityStability++
	case "Future Sustainability":
		dist.FutureSustainability++
	case "Product / Portfolio Work":
		dist.ProductPortfolio++
	}
}
