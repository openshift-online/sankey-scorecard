# Reconciliation Data Model

## Overview

The reconciliation data model defines the internal representation of Jira data after it has
been fetched, filtered, and stored in short-term memory. This data is the input to the
scoring engine.

Key properties:
- Data is stored **in memory** (no persistent database)
- Data is **fully overwritten** on each refresh; no historical data is retained
- Data is **read-heavy, write-infrequent** (many scorecard reads, occasional refreshes)
- Concurrent access is managed via `sync.RWMutex`
- Issues are organized by **scoring period** (sprint or fallback time window)

## Sprint-Based Time Scoping

The reconciler determines the scoring time window using a **calculated sprint calendar**
rather than a fixed number of days. The tool evaluates the **current sprint** and the
**previous sprint**, producing two scoring periods.

### Sprint Calendar Calculation

Sprint boundaries are derived from two configuration values:

- **`sprint_reference_date`** -- A known sprint start date (default: `2026-02-11`).
  This is any date on which a sprint is known to have started. All sprint boundaries
  are calculated as multiples of `sprint_duration_days` from this anchor.
- **`sprint_duration_days`** -- The length of each sprint in days (default: `21`,
  i.e., 3 weeks).

Given these two values and the current date, the reconciler calculates which sprint
is active and which sprint preceded it:

```
days_since_reference = floor((now - reference_date) / sprint_duration_days)
current_sprint_start = reference_date + (days_since_reference * sprint_duration_days)
current_sprint_end   = current_sprint_start + sprint_duration_days
previous_sprint_start = current_sprint_start - sprint_duration_days
previous_sprint_end   = current_sprint_start
```

If the current date is before the reference date, the calculation works the same way
(negative offset), projecting sprint boundaries backwards.

**Example** with reference date `2026-02-11` and duration `21`:

| Date | Current Sprint | Previous Sprint |
|------|---------------|-----------------|
| 2026-02-07 | 2026-01-21 to 2026-02-11 | 2025-12-31 to 2026-01-21 |
| 2026-02-15 | 2026-02-11 to 2026-03-04 | 2026-01-21 to 2026-02-11 |
| 2026-03-10 | 2026-03-04 to 2026-03-25 | 2026-02-11 to 2026-03-04 |

## Data Structures

### ReconciliationStore

Top-level container holding all reconciled data. A single instance exists in the application.

```go
// ReconciliationStore holds all reconciled Jira data in memory.
// Protected by an RWMutex for concurrent read access during scoring
// and exclusive write access during refresh.
type ReconciliationStore struct {
    mu       sync.RWMutex
    state    ReconciliationState
    teams    map[string]*TeamData  // keyed by team identifier
}
```

### ReconciliationState

Tracks the status of the most recent (or in-progress) reconciliation.

```go
type ReconciliationState struct {
    Status      ReconciliationStatus
    StartedAt   *time.Time
    CompletedAt *time.Time
    Error       string
    IssueCount  int   // total issues across all teams after last successful refresh
}

type ReconciliationStatus string

const (
    ReconciliationIdle      ReconciliationStatus = "idle"
    ReconciliationRunning   ReconciliationStatus = "running"
    ReconciliationCompleted ReconciliationStatus = "completed"
    ReconciliationFailed    ReconciliationStatus = "failed"
)
```

**State transitions:**

```
idle ──> running ──> completed
                 └─> failed
```

After `completed` or `failed`, the next refresh transitions back to `running`.
The initial state on startup is `idle`.

### TeamData

Holds the reconciled issues for a single team, organized by scoring period (sprint or
fallback time window).

```go
// TeamData contains all reconciled issues for a team organized by scoring
// period. Each period corresponds to a sprint (when a sprint board is
// configured) or a fallback time window. Pre-computed aggregates are stored
// per-period for use by the scoring engine.
type TeamData struct {
    TeamIdentifier string
    Periods        []ScoringPeriod
    ReconciledAt   time.Time
}
```

### ScoringPeriod

Represents a single scoring window (a calculated sprint). Issues within this window are
scored independently, and the results are also aggregated across periods.

```go
// ScoringPeriod groups issues within a single sprint window for scoring.
// Sprint boundaries are calculated from the configured reference date and
// sprint duration -- no Jira sprint API queries are needed.
type ScoringPeriod struct {
    Window           TimeWindow           // sprint start/end dates
    Label            string               // display label, e.g., "2026-01-21 to 2026-02-11"
    Current          bool                 // true if this is the active sprint

    Issues           []Issue

    // Pre-computed aggregates (populated during reconciliation)
    TotalCount       int
    CategorizedCount int
    Distribution     ActivityDistribution
}
```

### TimeWindow

A time range used as the scoring boundary for a period.

```go
type TimeWindow struct {
    Since time.Time
    Until time.Time
}
```

`Since` and `Until` are derived from the calculated sprint boundaries.

### Issue

Represents a single reconciled Jira issue. Contains only the fields required for scoring
and display. Extraneous Jira fields are discarded during reconciliation.

```go
type Issue struct {
    // Identification
    Key     string // Jira issue key, e.g., "OCM-1234"
    Project string // Jira project key, e.g., "OCM"

    // Classification (used for scoring)
    IssueType    string   // e.g., "Story", "Bug", "Task"
    ActivityType string   // Custom field value; empty string if unset
    Status       string   // e.g., "Open", "In Progress", "Closed"
    Components   []string // List of component names on the issue

    // Metadata
    Summary     string
    UpdatedDate time.Time
    CreatedDate time.Time
}
```

**Field notes:**

| Field | Source | Notes |
|-------|--------|-------|
| `Key` | `issue.Key` | Always present |
| `Project` | `issue.Fields.Project.Key` | Always present |
| `IssueType` | `issue.Fields.Type.Name` | Always present |
| `ActivityType` | `issue.Fields[runtimeConfig.ActivityTypeField]` | Custom field ID provided at runtime via `--activity-type-field` flag; empty if unset |
| `Status` | `issue.Fields.Status.Name` | Always present |
| `Components` | `issue.Fields.Components[].Name` | May be empty list |
| `UpdatedDate` | `issue.Fields.Updated` | Always present |
| `CreatedDate` | `issue.Fields.Created` | Always present |

### ActivityDistribution

Pre-computed issue counts per Sankey category. Computed during reconciliation to avoid
repeated iteration during scoring and API responses.

```go
type ActivityDistribution struct {
    AssociateWellness    int
    IncidentsSupport     int
    SecurityCompliance   int
    QualityStability     int
    FutureSustainability int
    ProductPortfolio     int
    Uncategorized        int
}
```

## Reconciliation Flow

### 1. Initiation

A refresh is initiated via `POST /api/refresh_data` or `sankey-scorecard refresh-data`.
If a refresh is already running, the request is rejected with HTTP 409.

### 2. Sprint Calendar Calculation

The reconciler calculates two scoring periods from the configured `sprint_reference_date`
and `sprint_duration_days`:

1. Compute `days_offset = floor((now - reference_date) / sprint_duration_days)`
2. `current_sprint_start = reference_date + (days_offset * sprint_duration_days)`
3. `current_sprint_end = current_sprint_start + sprint_duration_days`
4. `previous_sprint_start = current_sprint_start - sprint_duration_days`
5. `previous_sprint_end = current_sprint_start`
6. Build two `ScoringPeriod` entries:
   - Previous sprint: `Window{previous_sprint_start, previous_sprint_end}`, `Current: false`
   - Current sprint: `Window{current_sprint_start, current_sprint_end}`, `Current: true`

These two periods apply uniformly to all teams. The calculation is performed once per
refresh, not per team.

### 3. Issue Fetching (per team, per period)

For each team, for each scoring period:

1. Build the JQL query by combining the team's ownership clause with a date filter
   derived from the period's `TimeWindow`:

   - **Component method:**
     ```
     project = {project} AND component in ({components}) AND
     issuetype in ({scored_types}) AND
     updated >= "{window.Since}" AND updated <= "{window.Until}"
     ```
   - **Team field method:**
     ```
     project = {project} AND "Team" = "{team_field_value}" AND
     issuetype in ({scored_types}) AND
     updated >= "{window.Since}" AND updated <= "{window.Until}"
     ```
   - **JQL method:**
     ```
     ({custom_jql}) AND issuetype in ({scored_types}) AND
     updated >= "{window.Since}" AND updated <= "{window.Until}"
     ```

   Date values in JQL use the format `"YYYY-MM-DD"`.

2. Execute the query against the Jira API with pagination (max 100 results per page)
3. For each returned issue, extract the fields listed in the Issue struct
4. Map the `ActivityType` value to a Sankey category using the hardcoded mapping
5. Compute the `ActivityDistribution` aggregates for the period

### 4. Storage

After all teams have been fetched:

1. Acquire write lock on `ReconciliationStore`
2. Replace the `teams` map entirely with new data
3. Update `ReconciliationState` to `completed` with timestamp and total issue count
4. Release write lock

If any team fails to fetch, the entire refresh fails. Partial updates are not applied.

### 5. Error Handling

- **Jira API errors**: Logged per-team. If any team fails, the refresh is marked `failed`
  with the error message. Previously reconciled data is preserved (not cleared).
- **Rate limiting**: Requests are throttled with a configurable delay between API calls
  (default: 100ms). If a 429 response is received, the reconciler backs off exponentially.
- **Timeout**: Each team fetch has a 60-second timeout. The overall refresh has a
  10-minute timeout.

## Concurrency Model

```
Readers (scorecard API handlers)     Writer (reconciliation goroutine)
         |                                      |
         |-- RLock()                             |
         |-- read teams map                      |
         |-- compute scores                      |
         |-- RUnlock()                           |
         |                                      |-- Lock()
         |                                      |-- replace teams map
         |                                      |-- update state
         |                                      |-- Unlock()
         |                                      |
```

Multiple readers can access the store concurrently. The writer blocks all readers only
during the final swap (not during the entire fetch process). The writer builds the new
data set in a local variable, then swaps it in under the lock.

## Package Location

```
pkg/reconciler/
    store.go       - ReconciliationStore, state management, locking
    reconciler.go  - Jira fetch logic, pagination, retry
    sprint.go      - Sprint calendar calculation from reference date
    types.go       - Issue, TeamData, ScoringPeriod, TimeWindow, ActivityDistribution
```
