package reconciler

import (
	"fmt"
	"strings"

	"github.com/tiwillia/sankey-scorecard/pkg/config"
)

// BuildClosedJQL constructs a JQL query for issues that were resolved (closed)
// within the given time window. For sprint_board teams, sprintIDs scopes the
// query to specific sprints discovered from the Agile API.
func BuildClosedJQL(team config.Team, scoredIssueTypes []string, window TimeWindow, sprintIDs []int) string {
	ownershipClause := buildOwnershipClause(team.Ownership, sprintIDs)
	typeClause := buildIssueTypeClause(scoredIssueTypes)
	since := window.Since.Format("2006-01-02")
	until := window.Until.Format("2006-01-02")

	return fmt.Sprintf("%s AND %s AND statusCategory = Done AND resolved >= \"%s\" AND resolved <= \"%s\"",
		ownershipClause, typeClause, since, until)
}

// BuildInProgressJQL constructs a JQL query for issues currently in an
// in-progress-like status. No time window is applied. For sprint_board teams,
// sprintIDs scopes the query to specific sprints.
func BuildInProgressJQL(team config.Team, scoredIssueTypes []string, inProgressStatuses []string, sprintIDs []int) string {
	ownershipClause := buildOwnershipClause(team.Ownership, sprintIDs)
	typeClause := buildIssueTypeClause(scoredIssueTypes)
	statusClause := buildStatusClause(inProgressStatuses)

	return fmt.Sprintf("%s AND %s AND %s",
		ownershipClause, typeClause, statusClause)
}

func buildOwnershipClause(o config.Ownership, sprintIDs []int) string {
	switch o.Method {
	case "component":
		components := make([]string, len(o.Components))
		for i, c := range o.Components {
			components[i] = fmt.Sprintf("%q", c)
		}
		return fmt.Sprintf("project = %s AND component in (%s)",
			o.Project, strings.Join(components, ", "))
	case "team_field":
		return fmt.Sprintf("project = %s AND \"Team\" = %s",
			o.Project, o.TeamFieldValue)
	case "jql":
		return fmt.Sprintf("(%s)", o.JQL)
	case "sprint_board":
		sprintClause := buildSprintClause(sprintIDs)
		return fmt.Sprintf("project = %s AND %s", o.Project, sprintClause)
	default:
		return ""
	}
}

func buildIssueTypeClause(types []string) string {
	quoted := make([]string, len(types))
	for i, t := range types {
		quoted[i] = fmt.Sprintf("%q", t)
	}
	return fmt.Sprintf("issuetype in (%s)", strings.Join(quoted, ", "))
}

func buildStatusClause(statuses []string) string {
	quoted := make([]string, len(statuses))
	for i, s := range statuses {
		quoted[i] = fmt.Sprintf("%q", s)
	}
	return fmt.Sprintf("status in (%s)", strings.Join(quoted, ", "))
}

func buildSprintClause(sprintIDs []int) string {
	if len(sprintIDs) == 0 {
		// No sprints found; return a clause that matches nothing
		return "sprint in ()"
	}
	ids := make([]string, len(sprintIDs))
	for i, id := range sprintIDs {
		ids[i] = fmt.Sprintf("%d", id)
	}
	return fmt.Sprintf("sprint in (%s)", strings.Join(ids, ", "))
}
