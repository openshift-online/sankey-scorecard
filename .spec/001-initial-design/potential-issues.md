# Potential Issues

Issues identified during the initial review of `initial-idea.md` and subsequent spec generation
in `.spec/`.

## Spec Gaps

### 1. No Scoring Model Was Defined

**ADDRESSED** - scoring model reviewed and updated in `.spec/scoring-model.md`

The initial idea describes producing a "scorecard" but never defines what the score measures,
how it is computed, or what constitutes good vs. bad Sankey adherence. This was the central
purpose of the tool and was entirely missing. A scoring model has been drafted in
`.spec/scoring-model.md`.

### 2. Jira Reconciler Section Is Empty

`initial-idea.md` line 62 has a `### Jira Reconciler` header with no content. This is the
component responsible for fetching, transforming, and storing Jira data. The reconciliation
data model has been drafted in `.spec/reconciliation-data-model.md`.

### 3. No Refresh Status Observability

**ADDRESSED**

`POST /api/refresh_data` initiates an async task but the original spec provides no way to check
its progress or outcome. A `GET /api/refresh_status` endpoint has been added to the OpenAPI spec
to address this.

### 4. Time Scoping Not Specified

**ADDRESSED** - sprint-based time scoping added to all specs

Without a time window, the scorecard evaluates all issues ever created in a project/component.
This dilutes the signal for current planning adherence. The Sankey guide discusses sprint-level
planning, implying a narrow evaluation window.

The scoring window is now aligned to a **calculated sprint calendar**. Sprint boundaries are
derived from a configured **reference date** (`sprint_reference_date`, default: `2026-02-11`)
and **sprint duration** (`sprint_duration_days`, default: `21`). The reconciler calculates the
current and previous sprint windows from these values -- no Jira Agile API queries required.
Scores are computed per-sprint with an aggregate across both periods.

Changes made:
- `scope_days` replaced with `sprint_reference_date` and `sprint_duration_days` in config
- Reconciliation flow calculates sprint boundaries from reference date
- Data model restructured around `ScoringPeriod` with calculated `TimeWindow`
- Scorecard responses include per-sprint score breakdowns
- API schemas updated with `PeriodScore` and `TimeWindow` types
- `--since` CLI flag overrides the sprint calendar for ad-hoc analysis

### 5. Activity Type Field ID Not Addressed

**ADDRESSED**

The Jira "Activity Type" is a custom field with an instance-specific ID (e.g.,
`customfield_12319440`). The initial idea references it by display name but does not address
how the tool discovers or configures the correct field ID. Since the field ID is the same
across the entire Jira instance, it is now a required runtime configuration provided via the
`--activity-type-field` CLI flag (not in
the resource map).

### 6. Activity Type Value Mapping Missing

**RESOLVED** - The Jira Activity Type field uses static, well-known values that do not
vary between deployments. The mapping from these values to the six Sankey categories is
hardcoded in the scoring package rather than maintained as configuration.

## Data Quality Issues

### 7. The `types: [main]` Problem

**DEFERRED** - will be addressed by creating a proper mapping before production deployment

Many teams in the org repository data (`teams-from-org.yaml`) use `types: [main]`. This
appears to be a reference to "the main Jira board/dashboard" rather than an issue type filter
or ownership rule. These teams have no actionable ownership definition and cannot be scored
until their entries are replaced with real project+component, team field, or JQL rules.

**Affected teams**: Most SRE teams and several others in the org data that only reference
dashboards.

### 8. Teams Without Queryable Jira Data

**DEFERRED** - will be addressed by creating a proper mapping before production deployment

Several team entries in the org repository only have dashboard URLs with no project or
component information. These teams are entirely unscorable. The tool should:
- Validate the resource map at startup
- Report which configured teams have no ownership rules
- Exclude them from scoring without failing the entire refresh

### 9. Overlapping Team Issue Ownership

**RESOLVED** - Overlap is allowed by design. Multiple teams can contribute to the same Jira
issue, and the issue will be included in both teams' scores regardless of the assignee field.

Multiple teams can match the same Jira issue. For example, two teams in the same project may
both claim a component, or a JQL query may overlap with a component-based ownership rule.

### 10. Multi-Component Syntax Ambiguity

**RESOLVED** - `/`-separated components are treated as OR (the issue has any one of the
listed components). JQL uses `component in (...)` syntax.

Some org data entries list components with `/` separators (e.g.,
`clusters-service / aro-hcp-clusters-service / aro-hcp-clusters-service-east`).

## Scoring Model Open Questions

### 11. Story Points vs. Issue Count

**DEFERRED** - Issue counts will be used as it is the best available metric right now.
Story point weighting may be revisited in the future.

The Sankey guide discusses "capacity allocation" which implies story points, not issue counts.
However, not all teams use story points consistently, and some don't use them at all.

The current scoring model uses issue counts for simplicity. Story point weighting could be
added as an optional enhancement, but would require:
- Fetching story point data from Jira (another custom field)
- Handling teams with mixed or missing story point data
- Deciding whether to fall back to issue counts when story points are absent

### 12. Issue Status Filtering

**RESOLVED** - All statuses will be included. Status filtering may be added as a future
enhancement but is not needed now.

The initial idea does not specify which issue statuses to include in scoring. All issues
within the time window are scored regardless of status. Rationale: Activity Type should be
set at issue creation, not closure.

### 13. Container Types in Scoring

**RESOLVED** - Container types (Epic, Initiative, Feature) are excluded by default. Filtering
on them may be added as a future enhancement.

The initial idea lists Epic, Initiative, and Feature as supported issue types. These are
planning containers, not sprint-level work items. The scoring model excludes them by default
since the Sankey framework evaluates sprint-level capacity allocation, not portfolio planning.

## Operational Concerns

### 14. Jira API Rate Limiting

**RESOLVED** - Less of a concern now that queries are time-boxed to the past 6 weeks maximum,
significantly reducing result volume. Sequential fetching with existing mitigations
(`request_delay_ms`, exponential backoff on 429s, per-team timeouts) should be sufficient.

### 15. Jira API Pagination

**RESOLVED** - Same as #14, the 6-week time-boxing significantly reduces result volume,
making pagination less of a concern.

### 16. Concurrent Refresh Requests

**DEFERRED** - Could be an issue in theory, but not expected to scale to a point where it
would actually be hit anytime soon.

Multiple `POST /api/refresh_data` calls could race. The spec rejects concurrent refreshes
with HTTP 409, but the implementation needs a proper mutex or atomic state check to prevent
TOCTOU races between checking "is a refresh running?" and starting one.

## CLI Design Issues

### 17. Reserved Word Collision

**ADDRESSED** - identifiers are globally unique; slash paths only needed for disambiguation

The original design used flat identifiers that could collide with subcommand names.
Identifiers are expected to be globally unique, so a bare name like `aurora` is
sufficient in the common case. Single-segment arguments are checked against subcommand
names first, but in practice identifiers like `hcm` or `aurora` are unlikely to collide.

Slash-delimited paths (e.g., `rosa/aurora`) are only needed when an identifier is
ambiguous. See issue #25 for disambiguation handling.

The API uses a single `GET /api/scorecard` endpoint with independent `org`, `pillar`,
and `team` query parameters, each taking only the entity's own name.

### 18. CLI `--no-refresh` Semantics

**RESOLVED** - The `--no-refresh` flag is not needed. Not refreshing data is the default
behavior. If cached data exists, use it. Data refresh is only triggered explicitly and
manually.

## initial-idea.md Typos and Inconsistencies

### 19. Typos

| Line | Error | Correction |
|------|-------|------------|
| 11 | "Platfroms" | "Platforms" |
| 34 | "Acitivity" | "Activity" |
| 104 | "writtin" | "written" |
| 133 | "inluding" | "including" |

### 20. Inconsistent Casing

Line 81 uses `$organizationalIDentifier` (capital D) while line 68 uses
`$organizationalIdentifier`. Should be consistent.

### 21. Jira SDK Choice Not Resolved

**RESOLVED** - `go-jira` (`github.com/andygrunwald/go-jira/v2`) is the selected SDK.
`go-atlassian` was eliminated because it only supports Jira Cloud and has no Server/Data
Center support. `go-jira` v2 provides dedicated `cloud` and `onpremise` packages with the
same API surface. Its custom field handling is weaker (raw `interface{}` via `Unknowns` map
rather than typed parsers), but this is manageable with a small internal helper for the
Activity Type field.

## Architectural Considerations

### 22. No API Versioning

**ADDRESSED** - No versioning will be used. Routes remain as `/api/scorecard`,
`/api/refresh_data`, etc.

### 23. No Authentication on the API

**DEFERRED** - No authentication for now. Will be revisited later if needed.

The API has no authentication mechanism. For internal/dev use this is acceptable, but if
the tool is deployed as a shared service, the scorecard data (team performance scores)
may be sensitive.

### 24. In-Memory Storage Volatility

**DEFERRED**

All reconciled data is stored in memory and lost on process restart. If a refresh takes
several minutes and the process crashes, all data is gone. Consider:
- Writing reconciled data to a local file as a fallback
- Auto-refreshing on startup if a config flag is set
- Neither of these is required for an MVP but should be considered for production use

### 25. Ambiguous Identifier Resolution

**ADDRESSED** - Handled via HTTP 409 in the API and error messaging in the CLI

All identifiers (organizations, pillars, teams) are expected to be globally unique per
`initial-idea.md`. The API and CLI allow users to query by a single identifier without
requiring the full hierarchical path (e.g., `?team=aurora` or `sankey-scorecard aurora`).

If a non-unique identifier is encountered at runtime, the implementation must:
- Return HTTP 409 (API) or exit code 1 (CLI) with a message listing the conflicting
  matches and instructing the user to also provide `pillar` and/or `org` to disambiguate.
- Example error: `"Ambiguous identifier 'aurora': matches teams in pillars 'rosa' and
  'fleet-eng'. Specify pillar (and org if needed) to disambiguate."`

This keeps the common case simple (one parameter) while gracefully handling edge cases
without requiring the caller to always provide the full hierarchy.
