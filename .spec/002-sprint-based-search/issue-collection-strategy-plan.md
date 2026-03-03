# Implementation Plan: Sprint-Board Ownership & Status-Based Issue Collection

## Context

The scorecard currently scopes issues using `updated >= / <=` date filtering, which is unreliable -- issues can be updated long after completion or before work starts. Additionally, teams like Coffee and Focaccia define ownership by sprint membership, which isn't supported by the existing ownership methods (`component`, `team_field`, `jql`).

This plan implements two changes from `.spec/002-sprint-based-search/collection-strategy.md`:
1. **New `sprint_board` ownership method** for teams that define ownership via board sprints
2. **Status-based issue collection** replacing `updated` filtering with two scored sections: closed-in-cadence issues and currently-in-progress issues

## Important Design Note: Sprint-Board Board Scoping

Jira's JQL functions `openSprints()` and `closedSprints()` do **not** accept board ID parameters on Jira Server/Data Center. This means pure JQL cannot scope sprints to a specific board. To correctly differentiate teams that share a project (Coffee vs Focaccia in OCM), we need to discover sprint IDs from the Agile API (`/rest/agile/1.0/board/{boardId}/sprint`), then use those IDs in JQL (`sprint in (id1, id2, ...)`).

This is implemented as a minimal HTTP call per board during reconciliation -- not a full Agile API client.

---

## Changes by File

### 1. Config Types — `pkg/config/types.go`

- Add `Boards []int` field to `Ownership` struct (for `sprint_board` method)
- Add `InProgressStatuses []string` field to `JiraConfig` struct (global list of in-progress status names)

### 2. Config Validation — `pkg/config/validation.go`

- Add `case "sprint_board"` to ownership validation: requires non-empty `Project` and non-empty `Boards`
- Update the `default` case error message to include `sprint_board` in the valid methods list
- Add validation that `InProgressStatuses` is non-empty

### 3. Resource Map — `config/resource-map.yaml`

- Add `in_progress_statuses` list under `jira:` (values: `In Progress`, `Code Review`, `Review`)
- Update `rosa-coffee` and `rosa-foccacia` to use `method: sprint_board` with board IDs (board IDs TBD -- will need to be discovered from Jira)

### 4. Reconciler Types — `pkg/reconciler/types.go`

- Add `IssueSetType` string type with constants `IssueSetClosed` and `IssueSetInProgress`
- Add `SetType IssueSetType` field to `ScoringPeriod` struct

### 5. JQL Builder — `pkg/reconciler/jql.go`

Replace `BuildJQL` with two functions:

- **`BuildClosedJQL(team, scoredIssueTypes, window, sprintIDs)`**: ownership clause + type clause + `statusCategory = Done AND resolved >= "date" AND resolved <= "date"`. For `sprint_board` method, adds `sprint in (id1, id2, ...)` where IDs come from Agile API discovery.
- **`BuildInProgressJQL(team, scoredIssueTypes, inProgressStatuses, sprintIDs)`**: ownership clause + type clause + `status in ("In Progress", "Code Review", ...)`
- Add `case "sprint_board"` to `buildOwnershipClause()`: `project = X AND sprint in (id1, id2, ...)`

The `sprintIDs` parameter is only populated for `sprint_board` teams; for other methods it's nil and ownership clause works as before.

### 6. Sprint Discovery — `pkg/reconciler/sprints.go` (new file)

Minimal Agile API integration:

- Define `SprintInfo` struct: `ID int`, `Name string`, `State string` (active/closed/future)
- Define `BoardSprintFetcher` interface: `FetchBoardSprints(ctx, boardID) ([]SprintInfo, error)`
- Implement `AgileSprintFetcher` using a raw HTTP client (same base URL and auth as the Jira client)
- `FetchBoardSprints` calls `GET /rest/agile/1.0/board/{boardId}/sprint?state=active,closed` and parses the response
- Filter returned sprints to only active + recently closed (within the scoring window)

### 7. Reconciler Pipeline — `pkg/reconciler/reconciler.go`

- Add `sprintFetcher BoardSprintFetcher` field to `Reconciler` struct
- Update `NewReconciler` to accept the sprint fetcher
- Replace `calculatePeriods()` with `calculateCadences()` returning a `[]Cadence` type (simple struct: `Window`, `Label`, `Current`)
- Restructure `fetchTeamData()`:
  1. If team uses `sprint_board`, call `sprintFetcher.FetchBoardSprints()` for each board to get sprint IDs
  2. For each cadence: call `fetchClosedPeriod()` with sprint IDs
  3. Then call `fetchInProgressPeriod()` with sprint IDs (single set, no time window)
- Extract issue categorization logic from `fetchPeriod()` into shared helper `categorizeAndAdd()`
- Remove old `fetchPeriod()` and `calculatePeriods()`
- `--since` override: produces one cadence (since→now) for closed + one in-progress set

### 8. Scorecard Types — `pkg/scorecard/types.go`

- Add `SectionScore` struct: `Type string`, `Label string`, `Score`, `Distribution`, `Periods []PeriodScore`
- Add `Sections []SectionScore` field to `TeamScore` struct (alongside existing `Periods` for backward compat)
- Add `SetType string` field to `PeriodScore`

### 9. Scorecard Computation — `pkg/scorecard/scorecard.go`

- Update `ComputeTeamScore`: partition `teamData.Periods` by `SetType`, compute a `SectionScore` for each set type ("closed" → "Past (Closed)", "in_progress" → "Current (In Progress)"), and compute overall aggregate across all periods
- Add `computeSectionScore()` helper
- Update `computePeriodScore()` to include `SetType`

### 10. Presenter — `pkg/scorecard/presenter.go`

- Update `formatTeam()` to display sections instead of flat sprint breakdown:
  - Show section headers ("Past (Closed)", "Current (In Progress)")
  - Show per-section score summary
  - Show cadence rows within each section
  - Replace "Sprint Breakdown" header with section-based layout
  - Change `* = current sprint` to `* = current`

### 11. Scoring Strategy Doc — `scoring-strategy.md`

- Update "Time Scope" section to describe the two-set collection strategy
- Document the new `sprint_board` ownership method
- Document `in_progress_statuses` configuration
- Add changelog entry

---

## Test Changes

### Unit Tests

**`pkg/config/validation_test.go`**:
- Add test cases for `sprint_board` validation (missing project, missing boards, valid)
- Add test cases for `in_progress_statuses` validation
- Update `makeValidResourceMap` helper to include `InProgressStatuses`
- Update existing "rejects unknown ownership method" test expectation

**`pkg/reconciler/jql_test.go`**:
- Replace `BuildJQL` tests with `BuildClosedJQL` and `BuildInProgressJQL` tests
- Add `sprint_board` ownership clause tests
- Assert closed JQL contains `statusCategory = Done AND resolved >=` (not `updated >=`)
- Assert in-progress JQL contains `status in (...)` with no date filter

**`pkg/reconciler/sprints_test.go`** (new):
- Test `FetchBoardSprints` with mock HTTP server returning sprint list
- Test filtering by sprint state

**`pkg/reconciler/reconciler_test.go`**:
- Update `makeTestConfig` to include `InProgressStatuses`
- Update period count expectations: 3 periods per team (2 closed + 1 in-progress) instead of 2
- Mock `searchFunc` should dispatch differently for closed vs in-progress JQL
- Add test for `sprint_board` team
- Add a nil/no-op `BoardSprintFetcher` mock for tests that don't use sprint_board

**`pkg/scorecard/scorecard_test.go`**:
- Update `makeTeamData` to include `SetType: IssueSetClosed`
- Add tests verifying sections are correctly separated and independently scored
- Add tests verifying aggregate score combines both sections

**`pkg/scorecard/presenter_test.go`**:
- Update plaintext output assertions to match new section-based layout

### Integration Tests

**`tests/testdata/resource-map.yaml`**: Add `in_progress_statuses` field

**`tests/mock_jira_test.go`**: Update mock server to handle both closed JQL (`statusCategory = Done`) and in-progress JQL (`status in`), plus optionally sprint-based queries

**`tests/integration_test.go`**: Update period count assertions, verify two sections in output

---

## Implementation Order

1. **Config layer** (types.go, validation.go, resource-map.yaml) — additive, validation tests first
2. **Reconciler types** (types.go — IssueSetType, Cadence) — additive
3. **Sprint discovery** (sprints.go — new file, minimal Agile API) — independent
4. **JQL builder** (jql.go — replace BuildJQL, add sprint_board clause) — update JQL tests
5. **Reconciler pipeline** (reconciler.go — two-set fetching) — update reconciler tests
6. **Scorecard types + computation** (types.go, scorecard.go — sections) — update scorecard tests
7. **Presenter** (presenter.go — section layout) — update presenter tests
8. **Integration tests** (mock server + assertions)
9. **Documentation** (scoring-strategy.md)

---

## Verification

1. `make test` — all unit tests pass
2. `make test-integration` — integration tests pass with updated mock server
3. `make lint` — no lint errors
4. Manual run: `go run . refresh-data --jira-url=... --jira-pat=... && go run . <team-identifier>` — verify two sections appear in output, closed issues have resolved dates within cadence, in-progress issues reflect current status
5. Verify JSON output includes `sections` array with `type: "closed"` and `type: "in_progress"`
