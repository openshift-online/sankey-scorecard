package config

import (
	"fmt"
	"regexp"
	"strings"
	"time"
)

var identifierRegex = regexp.MustCompile(`^[a-z0-9-]+$`)

var reservedWords = map[string]bool{
	"serve":        true,
	"refresh-data": true,
	"version":      true,
	"help":         true,
}

// Validate checks a ResourceMap for all validation rules and returns a
// descriptive error listing all failures (not just the first one).
func Validate(rm *ResourceMap) error {
	var errs []string

	// Validate sprint_duration_days > 0
	if rm.SprintDurationDays <= 0 {
		errs = append(errs, "sprint_duration_days must be greater than 0")
	}

	// Validate sprint_reference_date parses as YYYY-MM-DD
	if rm.SprintReferenceDate == "" {
		errs = append(errs, "sprint_reference_date is required")
	} else if _, err := time.Parse("2006-01-02", rm.SprintReferenceDate); err != nil {
		errs = append(errs, fmt.Sprintf("sprint_reference_date %q is not a valid YYYY-MM-DD date", rm.SprintReferenceDate))
	}

	// Validate in_progress_statuses is non-empty
	if len(rm.Jira.InProgressStatuses) == 0 {
		errs = append(errs, "jira.in_progress_statuses must be non-empty")
	}

	// Collect all identifiers and check for uniqueness, format, and reserved words
	seen := make(map[string]string) // identifier -> description of where it was defined
	for _, org := range rm.Organizations {
		errs = append(errs, validateIdentifier(org.Identifier, "organization "+org.Name, seen)...)

		for _, pillar := range org.Pillars {
			errs = append(errs, validateIdentifier(pillar.Identifier, fmt.Sprintf("pillar %s in org %s", pillar.Name, org.Name), seen)...)

			for _, team := range pillar.Teams {
				errs = append(errs, validateIdentifier(team.Identifier, fmt.Sprintf("team %s in pillar %s", team.Name, pillar.Name), seen)...)
				errs = append(errs, validateOwnership(team)...)
			}
		}
	}

	if len(errs) > 0 {
		return fmt.Errorf("resource map validation failed:\n  - %s", strings.Join(errs, "\n  - "))
	}
	return nil
}

func validateIdentifier(id, context string, seen map[string]string) []string {
	var errs []string

	if id == "" {
		errs = append(errs, fmt.Sprintf("%s: identifier is required", context))
		return errs
	}

	if !identifierRegex.MatchString(id) {
		errs = append(errs, fmt.Sprintf("%s: identifier %q must be lowercase alphanumeric with hyphens only", context, id))
	}

	if reservedWords[id] {
		errs = append(errs, fmt.Sprintf("%s: identifier %q is a reserved word", context, id))
	}

	if prev, exists := seen[id]; exists {
		errs = append(errs, fmt.Sprintf("%s: identifier %q conflicts with %s", context, id, prev))
	} else {
		seen[id] = context
	}

	return errs
}

func validateOwnership(team Team) []string {
	var errs []string
	o := team.Ownership
	ctx := fmt.Sprintf("team %s (%s)", team.Name, team.Identifier)

	switch o.Method {
	case "component":
		if o.Project == "" {
			errs = append(errs, fmt.Sprintf("%s: ownership method 'component' requires 'project'", ctx))
		}
		if len(o.Components) == 0 {
			errs = append(errs, fmt.Sprintf("%s: ownership method 'component' requires non-empty 'components'", ctx))
		}
	case "team_field":
		if o.Project == "" {
			errs = append(errs, fmt.Sprintf("%s: ownership method 'team_field' requires 'project'", ctx))
		}
		if o.TeamFieldValue == "" {
			errs = append(errs, fmt.Sprintf("%s: ownership method 'team_field' requires 'team_field_value'", ctx))
		}
	case "jql":
		if o.JQL == "" {
			errs = append(errs, fmt.Sprintf("%s: ownership method 'jql' requires non-empty 'jql'", ctx))
		}
	case "sprint_board":
		if o.Project == "" {
			errs = append(errs, fmt.Sprintf("%s: ownership method 'sprint_board' requires 'project'", ctx))
		}
		if len(o.Boards) == 0 {
			errs = append(errs, fmt.Sprintf("%s: ownership method 'sprint_board' requires non-empty 'boards'", ctx))
		}
	default:
		errs = append(errs, fmt.Sprintf("%s: ownership method %q is not one of: component, team_field, jql, sprint_board", ctx, o.Method))
	}

	return errs
}
