# Sankey Scorecard - Implementation Plan

This document is an implementation plan for Claude Opus 4.6 sessions to follow when building
the Sankey Scorecard project. Each phase must be completed in order. Mark checkboxes as tasks
are completed. When a phase is fully complete, update the phase header with `COMPLETE`.

## Instructions for Implementing Agents

1. **Read the design document first**: Start every session by reading `DESIGN.md` and this plan.
2. **Spec files are authoritative**: When this plan references `@.spec/filename`, read that
   file for the full specification. Do not invent behavior not described in the specs.
3. **Mark progress**: After completing each task, update the checkbox in this file from
   `[ ]` to `[x]`. After completing all tasks in a phase, append `COMPLETE` to the phase header.
4. **Test continuously**: Run `make test` after each package is implemented. Run `make lint`
   before marking any phase complete. Fix all issues before proceeding.
5. **Do not skip ahead**: Phases have dependencies. Complete them in order.
6. **Follow the resource map example exactly**: The YAML structure in
   `@.spec/resource-map-example.yaml` is the canonical configuration format.
7. **Follow the user's CLAUDE.md rules**: Use `podman` not docker, never `git add -A`,
   always include remote and branch names when pushing, never remove failing tests without
   asking.
8. **Embed the resource map**: The file `config/resource-map.yaml` is embedded in the binary
   using Go's `embed` package. The `--config` flag overrides it entirely.
9. **Activity Type mapping is hardcoded**: The mapping from Jira Activity Type field values
   to the six Sankey categories lives in the scoring package as a Go map, not in configuration.

---

## Phase 1: Project Scaffolding - COMPLETE

**Goal**: Establish the Go module, directory structure, Makefile, linter, and dependency
management. No application logic yet.

- [x] Initialize Go module: `go mod init github.com/tiwillia/sankey-scorecard`
- [x] Create the directory structure:
  ```
  sankey-scorecard/
    cmd/
      root.go
      refresh_data.go
      serve.go
      version.go
    config/
      resource-map.yaml       (copy from @.spec/resource-map-example.yaml)
    pkg/
      config/
      handlers/
      reconciler/
      scorecard/
    tests/
    Makefile
  ```
- [x] Create `Makefile` with these targets:
  - `build`: `go build -ldflags "..." -o sankey-scorecard .`
  - `install`: `go install -ldflags "..." .`
  - `test`: `go test ./pkg/... -coverprofile=coverage.out -covermode=atomic`
  - `test-integration`: `go test ./tests/... -tags=integration -coverprofile=coverage.out -covermode=atomic`
  - `lint`: Run `golangci-lint run ./...`
  - `test-all`: Run lint, test, and test-integration sequentially
  - The `-ldflags` should inject `Version`, `Commit`, and `BuildDate` variables into `cmd/version.go`
- [x] Add `golangci-lint` configuration (`.golangci.yml`) with default settings
- [x] Add core dependencies to `go.mod`:
  - `github.com/spf13/cobra` (CLI framework)
  - `github.com/spf13/viper` (configuration)
  - `github.com/andygrunwald/go-jira/v2` (Jira SDK)
  - `github.com/onsi/ginkgo/v2` (test framework)
  - `github.com/onsi/gomega` (test matchers)
  - `gopkg.in/yaml.v3` (YAML parsing)
- [x] Create a minimal `main.go` that calls `cmd.Execute()`
- [x] Create stub `cmd/root.go` with Cobra root command (no logic, just the command definition
  with the help text from @.spec/cli.md "Root Command" section)
- [x] Verify `make build` succeeds
- [x] Verify `make lint` passes

**Verification**: `make build && make lint` both exit 0.

---

## Phase 2: Configuration Package (`pkg/config`) - COMPLETE

**Goal**: Parse and validate the resource map YAML. This is a dependency for all other packages.

Read @.spec/resource-map-example.yaml for the full YAML structure before implementing.

### Types (`pkg/config/types.go`)

- [x] Define Go structs that map to the resource map YAML:
  ```go
  type ResourceMap struct {
      Jira                JiraConfig       `yaml:"jira"`
      SprintReferenceDate string           `yaml:"sprint_reference_date"`
      SprintDurationDays  int              `yaml:"sprint_duration_days"`
      Organizations       []Organization   `yaml:"organizations"`
  }

  type JiraConfig struct {
      ScoredIssueTypes []string `yaml:"scored_issue_types"`
      RequestDelayMs   int      `yaml:"request_delay_ms"`
  }

  type Organization struct {
      Name       string   `yaml:"name"`
      Identifier string   `yaml:"identifier"`
      Pillars    []Pillar `yaml:"pillars"`
  }

  type Pillar struct {
      Name       string `yaml:"name"`
      Identifier string `yaml:"identifier"`
      Teams      []Team `yaml:"teams"`
  }

  type Team struct {
      Name       string    `yaml:"name"`
      Identifier string    `yaml:"identifier"`
      Ownership  Ownership `yaml:"ownership"`
  }

  type Ownership struct {
      Method         string   `yaml:"method"`          // "component", "team_field", "jql"
      Project        string   `yaml:"project"`         // required for component and team_field
      Components     []string `yaml:"components"`      // required for component method
      TeamFieldValue string   `yaml:"team_field_value"` // required for team_field method
      JQL            string   `yaml:"jql"`             // required for jql method
  }
  ```

### Loader (`pkg/config/config.go`)

- [x] Implement `LoadFromBytes(data []byte) (*ResourceMap, error)` -- parses YAML bytes
- [x] Implement `LoadFromFile(path string) (*ResourceMap, error)` -- reads file, calls LoadFromBytes
- [x] Implement `LoadEmbedded() (*ResourceMap, error)` -- loads from embedded resource map

### Validation (`pkg/config/validation.go`)

- [x] Validate all identifiers are globally unique (across orgs, pillars, and teams)
- [x] Validate identifiers are lowercase alphanumeric with hyphens only (regex: `^[a-z0-9-]+$`)
- [x] Validate no identifier collides with reserved words: `serve`, `refresh-data`, `version`, `help`
- [x] Validate each team's ownership method is one of: `component`, `team_field`, `jql`
- [x] Validate required fields per ownership method:
  - `component`: `project` and `components` (non-empty) required
  - `team_field`: `project` and `team_field_value` required
  - `jql`: `jql` (non-empty) required
- [x] Validate `sprint_duration_days` > 0
- [x] Validate `sprint_reference_date` parses as YYYY-MM-DD
- [x] Return a descriptive error listing all validation failures (not just the first one)

### Identifier Resolution (`pkg/config/resolver.go`)

- [x] Implement `Resolve(identifier string) (entity, level, error)` that:
  1. If identifier contains `/`, split and walk the hierarchy
  2. If bare name, search all orgs, pillars, and teams for a match
  3. If exactly one match, return it with its level (org/pillar/team) and full path
  4. If multiple matches, return an error listing the conflicts with disambiguation instructions
  5. If no match, return a "not found" error
- [x] The resolver must return enough context for the caller to know the entity type and its
  position in the hierarchy (org name, pillar name, team name as applicable)

### Embed Setup (`config/embed.go`)

- [x] Create `config/embed.go` that uses `//go:embed resource-map.yaml` to provide
  the embedded resource map bytes. Export a function or variable that `pkg/config` can use.

### Tests

- [x] Write unit tests for YAML parsing (valid config, missing fields, invalid YAML)
- [x] Write unit tests for all validation rules (each rule should have positive and negative cases)
- [x] Write unit tests for identifier resolution (unique match, ambiguous, not found, slash paths)
- [x] Verify `make test` passes with >80% coverage on `pkg/config` (93.1%)

**Verification**: `make test && make lint` both pass.

---

## Phase 3: Reconciler Package (`pkg/reconciler`) - COMPLETE

**Goal**: Implement Jira data fetching, sprint calendar calculation, and in-memory storage.

Read @.spec/reconciliation-data-model.md for the full data model and reconciliation flow.

### Types (`pkg/reconciler/types.go`)

- [x] Implement all types exactly as defined in @.spec/reconciliation-data-model.md:
  - `Issue`
  - `TeamData`
  - `ScoringPeriod`
  - `TimeWindow`
  - `ActivityDistribution`

### Sprint Calendar (`pkg/reconciler/sprint.go`)

- [x] Implement `CalculateSprintBoundaries(referenceDate time.Time, durationDays int, now time.Time) (current TimeWindow, previous TimeWindow)`
- [x] The algorithm from @.spec/reconciliation-data-model.md "Sprint Calendar Calculation"
- [x] Handle the case where `now` is before the reference date (negative offset)
- [x] Write unit tests verifying the examples from @.spec/reconciliation-data-model.md

### Store (`pkg/reconciler/store.go`)

- [x] Implement `ReconciliationStore` with `sync.RWMutex` as defined in @.spec/reconciliation-data-model.md
- [x] Implement `ReconciliationState` with status enum (`idle`, `running`, `completed`, `failed`)
- [x] Implement read methods (all acquire RLock)
- [x] Implement write method (acquires full Lock)
- [x] Implement state transition methods

### JQL Builder (`pkg/reconciler/jql.go`)

- [x] Implement JQL query construction for each ownership method
- [x] Date format in JQL: `"YYYY-MM-DD"`
- [x] All 3 ownership method templates implemented

### Reconciler (`pkg/reconciler/reconciler.go`)

- [x] Define a `JiraClient` interface to allow mocking (using onpremise.Search method)
- [x] Implement `Reconciler` struct
- [x] Implement `Refresh(ctx context.Context) error` with full pipeline
- [x] Implement rate limiting: configurable delay between API calls (from `request_delay_ms`)
- [x] Implement exponential backoff on HTTP 429 responses
- [x] Implement per-team timeout (60 seconds) and overall refresh timeout (10 minutes)

### Activity Type Extraction Helper

- [x] Implement `ExtractActivityType(unknowns map[string]interface{}, fieldID string) string`

### Tests

- [x] Unit tests for sprint calendar calculation (3 spec examples + edge cases)
- [x] Unit tests for JQL builder (all 3 ownership methods)
- [x] Unit tests for store concurrency (concurrent reads, write-blocks-reads, state transitions)
- [x] Unit tests for Activity Type extraction (present, missing, nil, unexpected type)
- [x] Verify `make test` passes with >80% coverage on `pkg/reconciler` (93.2%)

**Verification**: `make test && make lint` both pass.

---

## Phase 4: Scoring Package (`pkg/scorecard`) - COMPLETE

**Goal**: Implement score computation, aggregation, grade assignment, and output formatting.

Read @.spec/scoring-model.md for the full scoring algorithm and worked examples.
Read @.spec/scorecard-data-model.md for output structures and JSON/plaintext formats.

### Activity Type Mapping (`pkg/scorecard/categories.go`)

- [x] Define the six Sankey categories as constants:
  ```go
  const (
      CategoryAssociateWellness    = "Associate Wellness & Development"
      CategoryIncidentsSupport     = "Incidents & Support"
      CategorySecurityCompliance   = "Security & Compliance"
      CategoryQualityStability     = "Quality / Stability / Reliability"
      CategoryFutureSustainability = "Future Sustainability"
      CategoryProductPortfolio     = "Product / Portfolio Work"
  )
  ```
- [x] Implement the hardcoded mapping from Jira Activity Type field values to Sankey categories.
  Research the known Jira Activity Type values from @tmp/references/sankey-usage-guide.md
  and map them. At minimum handle these values:
  - "Associate Wellness & Development" -> CategoryAssociateWellness
  - "Incidents & Escalations" / "Customer Support" -> CategoryIncidentsSupport
  - "Security & Compliance" -> CategorySecurityCompliance
  - "Tech Debt" / "Defect" / "QE Activities" / "Quality / Stability / Reliability" -> CategoryQualityStability
  - "Future Sustainability" -> CategoryFutureSustainability
  - "Product / Portfolio Work" / "New Feature" / "Feature Enhancement" -> CategoryProductPortfolio
  - If the Activity Type value is empty or unrecognized, it is "uncategorized"
- [x] Implement `MapActivityType(activityType string) string` returning the category name
  or empty string for uncategorized

### Types (`pkg/scorecard/types.go`)

- [x] Implement all types exactly as defined in @.spec/scorecard-data-model.md:
  - `FullScorecard`
  - `OrganizationScore`
  - `PillarScore`
  - `TeamScore`
  - `PeriodScore`
  - `Score` (with `*float64` for nullable fields)
  - Reuse `TimeWindow` and `ActivityDistribution` from `pkg/reconciler` (import, do not duplicate)

### Scorer (`pkg/scorecard/scorecard.go`)

- [x] Implement `ComputeTeamScore(teamData *reconciler.TeamData) TeamScore`:
  1. For each `ScoringPeriod`, compute `PeriodScore`:
     - Categorization Rate: `(categorized_count / total_count) * 60`
     - Distribution Alignment: Use the priority-weighted deviation formula from @.spec/scoring-model.md
     - Total: sum of both dimensions
     - Grade: map total to letter grade
  2. Compute aggregate score from combined issue set across all periods
  3. If total_count == 0, all score fields are nil, grade is "-"

- [x] Implement the distribution alignment algorithm exactly as specified in @.spec/scoring-model.md:
  ```
  For each category i (priority rank 1 to 6):
    actual_pct[i] = issues_in_category[i] / categorized_count
    target_pct[i] = configured target percentage for category i
    deviation[i]  = actual_pct[i] - target_pct[i]

    if deviation[i] > 0 (over-allocated):
      penalty[i] = deviation[i] * over_weight[i]
    else (under-allocated):
      penalty[i] = |deviation[i]| * under_weight[i]

  total_penalty = sum(penalty[i])
  alignment_pct = max(0, 1 - total_penalty)
  score = alignment_pct * 40
  ```

- [x] Implement penalty weight calculation:
  ```
  over_weight(rank)  = 0.5 + (rank - 1) * 0.2
  under_weight(rank) = 1.5 - (rank - 1) * 0.2
  ```
  Default range [0.5, 1.5] per @.spec/scoring-model.md.

- [x] Implement grade assignment:
  | Grade | Score Range |
  |-------|-----------|
  | A | 90-100 |
  | B | 75-89 |
  | C | 60-74 |
  | D | 45-59 |
  | F | 0-44 |
  | - | nil |

- [x] Implement `ComputePillarScore(pillar config.Pillar, teamScores []TeamScore) PillarScore`:
  Issue-count-weighted average. Exclude nil-scored teams.

- [x] Implement `ComputeOrgScore(org config.Organization, pillarScores []PillarScore) OrganizationScore`:
  Issue-count-weighted average of pillars.

- [x] Implement `ComputeFullScorecard(cfg *config.ResourceMap, store *reconciler.ReconciliationStore) FullScorecard`:
  Orchestrates the full computation.

### Presenter (`pkg/scorecard/presenter.go`)

- [x] Implement `ToJSON(scorecard interface{}) ([]byte, error)` -- JSON marshaling with null handling
- [x] Implement `ToYAML(scorecard interface{}) ([]byte, error)` -- YAML marshaling
- [x] Implement `ToPlaintext(scorecard interface{}) string` -- human-readable table format
  - See @.spec/scorecard-data-model.md "CLI Plain-Text Output" for exact format examples
  - Pillar view: table with TEAM, SCORE, GRADE, ISSUES, CAT.RATE, DIST.ALIGN columns
  - Team view: includes sprint breakdown table and activity distribution with target comparisons
  - Organization view: table with PILLAR, SCORE, GRADE, ISSUES, CAT.RATE, DIST.ALIGN columns
  - The presenter must detect the entity level being rendered and format accordingly

### Tests

- [x] Unit tests for the scoring algorithm using the three worked examples from @.spec/scoring-model.md:
  - Example 1 (well-aligned): expected distribution alignment = 37.8 / 40
  - Example 2 (skewed bottom): expected distribution alignment = 5.3 / 40
  - Example 3 (skewed top): expected distribution alignment = 14.1 / 40
- [x] Unit tests for categorization rate (100% categorized, 50% categorized, 0 issues -> nil)
- [x] Unit tests for grade assignment at each boundary (44->F, 45->D, 59->D, 60->C, 74->C, 75->B, 89->B, 90->A)
- [x] Unit tests for aggregation (weighted average, nil exclusion)
- [x] Unit tests for the Activity Type mapping (known values, unknown values, empty string)
- [x] Unit tests for plaintext formatter output structure
- [x] Verify `make test` passes with >80% coverage on `pkg/scorecard` (87.4%)

**Verification**: `make test && make lint` both pass.

---

## Phase 5: API Handlers (`pkg/handlers`) - COMPLETE

**Goal**: Implement the REST API endpoints.

Read @.spec/openapi.yaml for the full API specification including all request/response schemas.

### Router Setup (`pkg/handlers/router.go`)

- [x] Implement `NewRouter(cfg *config.ResourceMap, store *reconciler.ReconciliationStore, reconciler *reconciler.Reconciler) http.Handler`
- [x] Register routes under `/api` prefix:
  - `GET /api/` -> OpenAPI spec handler
  - `GET /api/scorecard` -> Scorecard handler
  - `POST /api/refresh_data` -> Refresh handler
  - `GET /api/refresh_status` -> Refresh status handler
- [x] Set `Content-Type: application/json` on all responses
- [x] Use Go's standard `net/http` (no external router framework needed for 4 routes)

### Handlers (`pkg/handlers/handlers.go`)

- [x] `HandleGetOpenAPI` -- returns the OpenAPI spec (embed @.spec/openapi.yaml in the binary)
- [x] `HandleGetScorecard`:
  1. Check `store.HasData()` -- return 503 if no data
  2. Parse query params: `org`, `pillar`, `team`
  3. Use `config.Resolve()` logic to find matching entities
  4. If ambiguous identifier: return 409 with disambiguation message
  5. If not found: return 404
  6. Compute scorecard and return 200
- [x] `HandleRefreshData`:
  1. Call `reconciler.Refresh()` in a goroutine (async)
  2. Return 202 with current `RefreshStatus`
  3. If already running: return 409
- [x] `HandleGetRefreshStatus`:
  1. Return 200 with current `ReconciliationState` mapped to `RefreshStatus` JSON

### Error Response Helper

- [x] Implement `writeError(w http.ResponseWriter, status int, code string, message string)`
  using the error schema from @.spec/openapi.yaml: `{"error": "...", "message": "..."}`

### Tests

- [x] Unit tests for each handler using `httptest.NewRecorder()`:
  - Scorecard: no data (503), valid request (200), not found (404), ambiguous (409)
  - Refresh: initiate (202), already running (409)
  - Refresh status: idle, running, completed, failed states
- [x] Verify `make test` passes with >80% coverage on `pkg/handlers` (98.5%)

**Verification**: `make test && make lint` both pass.

---

## Phase 6: CLI (`cmd/`) - COMPLETE

**Goal**: Implement the Cobra/Viper CLI as a thin layer over the packages.

Read @.spec/cli.md for full command structure, help text, flags, and exit codes.

### Root Command (`cmd/root.go`)

- [x] Define the root command with:
  - Description text from @.spec/cli.md "Root Command" help output
  - Global flag: `-c, --config` (path to resource map YAML, default: embedded)
  - The root command's `RunE` handles the `<identifier>` positional argument
- [x] Load configuration: if `--config` is provided, use `config.LoadFromFile()`; otherwise
  use `config.LoadEmbedded()`
- [x] When called with a positional argument:
  1. Resolve the identifier using `config.Resolve()`
  2. If no data in store: exit code 3 with message "No data available. Run 'refresh-data' first."
  3. If not found: exit code 2
  4. If ambiguous: exit code 1 with disambiguation instructions
  5. Compute scorecard, format per `-o` flag, print to stdout
- [x] Flag: `-o, --output` (string, default "plaintext", values: "plaintext", "json", "yaml")

### refresh-data Command (`cmd/refresh_data.go`)

- [x] Define the `refresh-data` subcommand with:
  - Description from @.spec/cli.md
  - Flags: `--jira-url` (required), `--jira-pat` (required), `--activity-type-field` (required), `--since` (optional)
- [x] Implementation:
  1. Validate required flags
  2. Create Jira client using `go-jira` SDK with PAT authentication
  3. Create reconciler
  4. Call `reconciler.Refresh()` synchronously (blocking)
  5. Print progress to stdout (team count, issue count, duration)
  6. Exit 0 on success, 1 on failure

### serve Command (`cmd/serve.go`)

- [x] Define the `serve` subcommand with:
  - Description from @.spec/cli.md
  - Flags: `--bind-address` (default ":8080"), `--jira-url` (required), `--jira-pat` (required), `--activity-type-field` (required)
- [x] Implementation:
  1. Create Jira client, reconciler, store
  2. Create router with handlers
  3. Start HTTP server on bind address
  4. Log "Listening on :8080" to stdout
  5. Block until signal (SIGINT/SIGTERM) for graceful shutdown

### version Command (`cmd/version.go`)

- [x] Define the `version` subcommand
- [x] Print: `sankey-scorecard {version} (commit: {commit}, built: {date})`
- [x] Version, Commit, BuildDate injected via `-ldflags` at build time

### Exit Codes

- [x] Ensure all exit codes match @.spec/cli.md:
  - 0: Success
  - 1: General error
  - 2: Identifier not found
  - 3: No data available

### Tests

- [x] The CLI layer (`cmd/`) is NOT tested directly (per design doc). All logic is tested
  through the package-level tests.

**Verification**: `make build && ./sankey-scorecard --help` shows expected output.
`./sankey-scorecard version` prints version info.

---

## Phase 7: Integration Tests (`tests/`) - COMPLETE

**Goal**: End-to-end tests covering the full pipeline: mock Jira -> reconciler -> scorer -> API.

Read the testing strategy in `DESIGN.md` and @.spec/reconciliation-data-model.md.

### Mock Jira Server (`tests/mock_jira.go`)

- [x] Implement a mock HTTP server that serves Jira search API responses
- [x] Mock data is maintained in YAML files under `tests/testdata/`, following the pattern
  used by [go-jira/testing/mock-data](https://github.com/andygrunwald/go-jira/tree/main/testing/mock-data)
- [x] The mock server should:
  - Accept JQL queries and return matching issues based on project/component filters
  - Support pagination (return `maxResults`, `startAt`, `total` in response)
  - Include custom field data (Activity Type) in issue responses
  - Support rate limiting simulation (return 429 on demand)

### Test Data (`tests/testdata/`)

- [x] Create YAML fixtures for at least 3 teams across 2 pillars:
  - Team A: 100% categorized, well-aligned distribution -> expect high score (A/B)
  - Team B: 50% categorized, moderate alignment -> expect mid score (C/D)
  - Team C: 0 issues -> expect nil score ("-")
- [x] Each fixture should contain issues spanning both current and previous sprint windows
- [x] Include a mix of issue types (Story, Bug, Task) and Activity Type values covering all 6 categories

### Test Resource Map (`tests/testdata/resource-map.yaml`)

- [x] Create a test-specific resource map that defines the 3 test teams organized into pillars
  and an organization, with ownership rules matching the mock data

### Integration Test Suite (`tests/integration_test.go`)

- [x] Use Ginkgo test framework with `//go:build integration` build tag
- [x] Test: Full reconciliation pipeline
  1. Start mock Jira server
  2. Load test resource map
  3. Create reconciler with mock Jira client
  4. Run refresh
  5. Verify store contains expected team data
  6. Verify issue counts per team per period
  7. Verify Activity Distribution computation
- [x] Test: Scorecard computation after reconciliation
  1. Compute scorecard from reconciled data
  2. Verify Team A scores in expected range
  3. Verify Team B scores in expected range
  4. Verify Team C has nil score
  5. Verify pillar score is issue-count-weighted average of teams A and B
  6. Verify organization score aggregates correctly
- [x] Test: API endpoint integration
  1. Start mock Jira server + API server
  2. POST /api/refresh_data -> 202
  3. Poll GET /api/refresh_status until completed
  4. GET /api/scorecard -> 200 with full scorecard
  5. GET /api/scorecard?team=team-a -> 200 with filtered result
  6. GET /api/scorecard?team=nonexistent -> 404
  7. GET /api/scorecard before refresh -> 503
- [x] Test: Refresh error handling
  1. Configure mock Jira to return errors for one team
  2. Run refresh
  3. Verify refresh status is "failed"
  4. Verify previously reconciled data is preserved (not cleared)
- [x] Test: Concurrent refresh rejection
  1. Start a refresh
  2. Attempt a second refresh while first is running
  3. Verify second attempt returns 409
- [x] Test: Ambiguous identifier resolution (covered by unit tests in pkg/config)
  1. Configure resource map with duplicate identifiers (if applicable)
  2. Query API with ambiguous identifier
  3. Verify 409 response with disambiguation instructions

### Coverage

- [x] Verify `make test-integration` passes
- [x] Verify combined coverage (unit + integration) exceeds 80%
- [x] Verify `make test-all` passes (lint + unit tests + integration tests)

**Verification**: `make test-all` exits 0 with coverage >= 80%.

---

## Phase 8: Polish and Final Validation - COMPLETE

**Goal**: Embed resources, wire everything together, verify end-to-end.

### Embed Resources

- [x] Verify `config/resource-map.yaml` is embedded correctly via `//go:embed`
- [x] Verify the OpenAPI spec is embedded and served by `GET /api/`
- [x] Verify `--config` flag overrides the embedded resource map entirely

### Build Verification

- [x] `make build` produces a working binary
- [x] `./sankey-scorecard --help` matches the expected output from @.spec/cli.md
- [x] `./sankey-scorecard refresh-data --help` matches expected output
- [x] `./sankey-scorecard serve --help` matches expected output
- [x] `./sankey-scorecard version` prints version with ldflags-injected values
- [x] `./sankey-scorecard nonexistent-name` exits with code 3 (no data check precedes identifier resolution)
- [x] `./sankey-scorecard aurora` (without prior refresh) exits with code 3

### Makefile Verification

- [x] `make build` works
- [x] `make install` works
- [x] `make test` works
- [x] `make test-integration` works
- [x] `make lint` works
- [x] `make test-all` works

### Final Cleanup

- [x] Remove any TODO comments left during implementation
- [x] Verify no hardcoded test paths or developer-specific values
- [x] Run `go mod tidy` to clean up dependencies
- [x] Verify `make test-all` passes one final time

**Verification**: All make targets pass. Binary runs correctly.

---

## Appendix: Key Design Decisions to Remember

These decisions are documented across the spec files. They are repeated here to prevent
common implementation mistakes:

1. **Nil vs Zero**: A team with 0 scored issues gets `nil` scores and grade `"-"`, NOT
   zero scores. This is critical for correct JSON serialization (`null` vs `0`).

2. **Atomic store swap**: The reconciler must build all team data in a local variable first,
   then swap the entire store under a write lock. Partial updates are never applied.

3. **Weighted average aggregation**: Pillar and org scores are weighted by issue count,
   not simple averages. Teams with nil scores are excluded.

4. **Asymmetric penalties**: Over-allocating to high-priority categories is penalized LESS
   than over-allocating to low-priority categories. The penalty weights are NOT symmetric.

5. **CLI refresh is synchronous**: `refresh-data` blocks until complete. API refresh is
   asynchronous (returns 202 immediately).

6. **Single identifier resolution**: Both CLI and API accept a single identifier. The full
   path is only needed for disambiguation.

7. **`go-jira` v2 custom fields**: Activity Type is read from `issue.Fields.Unknowns[fieldID]`.
   The value is a `map[string]interface{}` with a `"value"` key. Handle this carefully.

8. **Scored issue types**: Only Story, Bug, Task by default. Epic, Initiative, Feature are
   excluded as planning containers.

9. **Sprint calendar**: Calculated from a reference date, not fetched from Jira. No Jira
   sprint API calls are made.

10. **Rate limiting**: Default 100ms delay between Jira API calls. Exponential backoff on
    429 responses. Per-team timeout 60s, overall timeout 10min.
