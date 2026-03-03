# Contributing

## Prerequisites

- Go 1.22+
- [golangci-lint](https://golangci-lint.run/welcome/install/)

## Project Structure

```
sankey-scorecard/
  cmd/                          CLI commands (thin layer over pkg/)
  config/
    sankey-scorecard.yaml       Central config file (gitignored; copy from sankey-scorecard.yaml.example)
    sankey-scorecard.yaml.example  Annotated template for the config file
  pkg/
    config/                     Configuration parsing, validation, identifier resolution
    handlers/                   HTTP API handlers and router
    reconciler/
      store.go                  In-memory data store with RWMutex
      reconciler.go             Jira fetch logic, pagination, rate limiting
      sprint.go                 Sprint calendar calculation
      types.go                  Issue, TeamData, ScoringPeriod, ActivityDistribution
    scorecard/
      scorecard.go              Score computation, aggregation, grade assignment
      categories.go             Activity Type mapping, target distribution
      types.go                  FullScorecard, OrganizationScore, PillarScore, TeamScore, Score
      presenter.go              JSON, YAML, plaintext output formatting
  tests/                        Integration tests (Ginkgo, build tag: integration)
  .spec/                        Design specifications (authoritative)
```

## Makefile Targets

| Target | Description |
|--------|-------------|
| `make build` | Build binary to `./sankey-scorecard` |
| `make install` | Install to `$GOPATH/bin` |
| `make test` | Run unit tests with coverage |
| `make test-integration` | Run integration tests |
| `make lint` | Run golangci-lint |
| `make test-all` | Run lint, unit tests, and integration tests |

## Running Tests

```bash
# Unit tests only
make test

# Integration tests only
make test-integration

# Everything (lint + unit + integration)
make test-all
```

Unit tests use [Ginkgo](https://onsi.github.io/ginkgo/) and [Gomega](https://onsi.github.io/gomega/). Integration tests use the `//go:build integration` build tag and run via `make test-integration`.

## Coverage

Coverage is reported on every test run. All packages in `pkg/` must maintain **>80% coverage**:

| Package | Coverage |
|---------|----------|
| `pkg/config` | 93.1% |
| `pkg/handlers` | 98.5% |
| `pkg/reconciler` | 93.2% |
| `pkg/scorecard` | 87.4% |

## Architecture Notes

- The `cmd/` layer is intentionally thin and not directly tested. All logic lives in `pkg/` packages.
- The config file (`config/sankey-scorecard.yaml`) is loaded at runtime, not embedded. The binary searches for it via `--config` flag, `RESOURCE_MAP_PATH` env var, or `/etc/sankey-scorecard/sankey-scorecard.yaml`.
- The Activity Type to Sankey category mapping is hardcoded in `pkg/scorecard/categories.go`, not in configuration.
- `pkg/reconciler` defines a `JiraClient` interface to allow mocking in tests. The real implementation uses `github.com/andygrunwald/go-jira/v2/onpremise`.
- Score fields use `*float64` (pointer) to distinguish nil (no data) from zero. This affects JSON serialization (`null` vs `0`).
- The reconciliation store uses `sync.RWMutex` for concurrent read access during scoring and exclusive write access during refresh. Data is built locally and swapped atomically.

## Jira Team Field (team_field ownership)

The Jira `"Team"` custom field (`customfield_12313240`) uses the Atlassian Teams plugin (`com.atlassian.teams:rm-teams-custom-field-team`), **not** a standard select list. This has a critical implication for JQL queries:

- **Numeric IDs required**: JQL queries must use the team's numeric ID, not its display name. String comparisons silently return 0 results without error.
- **Correct**: `"Team" = 5695`
- **Wrong**: `"Team" = "SREP Networking/Infrastructure"` (returns 0 results, no error)

When adding a new team that uses the `team_field` ownership method in `config/sankey-scorecard.yaml`, you must look up the numeric team ID. To find it, fetch any issue known to belong to that team and inspect the raw JSON:

```bash
jira issue view <ISSUE-KEY> --raw | python3 -c "
import sys, json
d = json.load(sys.stdin)
val = d.get('fields', {}).get('customfield_12313240')
print(val)  # {'id': 5695, 'name': 'SREP Networking/Infrastructure'}
"
```

Use the `id` value (e.g., `5695`) as the `team_field_value` in the resource map, with a comment noting the human-readable name:

```yaml
ownership:
  method: team_field
  project: SREP
  team_field_value: "5695"  # SREP Networking/Infrastructure
```

## Modifying the Scoring Strategy

The scoring strategy is defined in `scoring-strategy.md` at the repository root. This file is the authoritative specification for how teams are scored on Sankey framework adherence.

To change the scoring strategy:

1. Edit `scoring-strategy.md` with your desired changes (new targets, weight adjustments, category changes, etc.).
2. Ask Claude to update the implementation to match the spec:
   ```
   Update the scoring strategy implementation to match scoring-strategy.md
   ```
3. Claude will update the relevant code in `pkg/scorecard/`, adjust tests, and ensure everything aligns with the spec.

Do not modify the scoring implementation code directly. The markdown spec is the source of truth -- update it first, then let the implementation follow.

## Linting

The project uses [golangci-lint](https://golangci-lint.run/) with default settings. Run `make lint` before submitting changes.
