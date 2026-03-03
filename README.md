# Sankey Scorecard

A CLI and API tool that evaluates engineering teams on their adherence to the Sankey planning framework by analyzing Jira issue data and producing scorecard reports.

## Overview

Sankey Scorecard fetches Jira issues for configured teams, measures how consistently teams categorize their work using the Activity Type custom field, and scores how well each team's work distribution aligns with target allocations across six Sankey categories.

Scorecards are produced at three levels of an organizational hierarchy: **Organization > Pillar > Team**. Scores range from 0-100 with letter grades (A-F).

### Scoring Model

Each team receives a composite score from two dimensions:

**Categorization Rate (60 points)** -- The percentage of scored issues that have the Activity Type field populated.

```
score = (categorized_count / total_count) * 60
```

**Distribution Alignment (40 points)** -- How closely the team's actual work distribution across six Sankey categories matches the target distribution. Deviations are penalized asymmetrically: over-allocating to high-priority categories incurs less penalty than over-allocating to low-priority ones.

The six categories, in priority order:

| Rank | Category | Target |
|------|----------|--------|
| 1 | Associate Wellness & Development | 12% |
| 2 | Incidents & Support | 12% |
| 3 | Security & Compliance | 12% |
| 4 | Quality / Stability / Reliability | 22% |
| 5 | Future Sustainability | 21% |
| 6 | Product / Portfolio Work | 21% |

**Grade Scale:**

| Grade | Score Range |
|-------|------------|
| A | 90-100 |
| B | 75-89 |
| C | 60-74 |
| D | 45-59 |
| F | 0-44 |
| - | nil (no data) |

Pillar and organization scores are issue-count-weighted averages of their children. Teams with no scored issues receive a nil score (grade "-") and are excluded from aggregation.

## Installation

```bash
make build
# Binary is produced at ./sankey-scorecard

make install
# Installs to $GOPATH/bin
```

Version, commit hash, and build date are injected via `-ldflags` at build time.

## Configuration

### Central Configuration File

The organizational hierarchy, team ownership rules, sprint calendar, and scoring parameters are all defined in a single YAML config file (`config/sankey-scorecard.yaml`). The binary loads it at runtime from one of these locations in priority order:

1. `--config` / `-c` flag
2. `RESOURCE_MAP_PATH` environment variable
3. `/etc/sankey-scorecard/sankey-scorecard.yaml` (default for container deployments)

See `config/sankey-scorecard.yaml.example` for a fully annotated template.

```yaml
sprint_reference_date: "2026-01-01"
sprint_duration_days: 21

jira:
  activity_type_field: customfield_XXXXXXXX
  scored_issue_types:
    - Story
    - Bug
    - Task
  request_delay_ms: 100

organizations:
  - name: Example Organization
    identifier: example-org
    pillars:
      - name: Example Pillar
        identifier: example-pillar
        teams:
          - name: Example Team B
            identifier: example-team-b
            ownership:
              method: team_field
              project: PROJ
              team_field_value: "TeamFieldValue"
```

Each team defines an ownership method for resolving its Jira issues:

| Method | Identifies issues by | Required fields |
|--------|---------------------|-----------------|
| `component` | Project + component list | `project`, `components` |
| `team_field` | Project + Team custom field value | `project`, `team_field_value` |
| `sprint_board` | Project + Jira Agile board sprints | `project`, `boards` |
| `jql` | Arbitrary JQL query | `jql` |

All identifiers must be globally unique, lowercase alphanumeric with hyphens, and not conflict with reserved words (`serve`, `refresh-data`, `version`, `help`).

### Sprint Calendar

Sprint boundaries are calculated from `sprint_reference_date` (a known sprint start date) and `sprint_duration_days`. The scorecard evaluates two periods: the current sprint and the previous sprint. No Jira sprint API calls are made.

### Runtime Flags

Sensitive and instance-specific values are provided at invocation time (not in the config file):

- `--jira-url` -- Jira instance URL
- `--jira-api-token` -- Jira API token for authentication (falls back to `JIRA_API_TOKEN` env var)
- `--activity-type-field` -- Override the Activity Type custom field ID from the config file (e.g., `customfield_XXXXXXXX`)

## Usage

### Refresh Data

Fetch Jira issue data for all configured teams. This must be run before querying scorecards.

```bash
sankey-scorecard refresh-data \
  --jira-url https://issues.redhat.com \
  --jira-api-token $JIRA_API_TOKEN
```

Optionally override the sprint calendar with a custom start date:

```bash
sankey-scorecard refresh-data \
  --jira-url https://issues.redhat.com \
  --jira-api-token $JIRA_API_TOKEN \
  --since 2026-01-01
```

### View Scorecards

After refreshing data, query scorecards by identifier:

```bash
# Show scorecard for a team
sankey-scorecard example-team-b

# Show scorecard for a pillar
sankey-scorecard example-pillar

# Show scorecard for an organization
sankey-scorecard example-org

# Output as JSON or YAML
sankey-scorecard example-team-b -o json
sankey-scorecard example-pillar -o yaml

# Disambiguate if an identifier matches multiple entities
sankey-scorecard example-pillar/example-team-b

# Use a custom config file
sankey-scorecard example-org --config ./my-config.yaml
```

Example plaintext output:

```
Example Pillar Scorecard
Generated: 2026-01-15 14:30 UTC | Issues: 523

Pillar Score: 71.0 (C)
  Categorization Rate:      43.2 / 60
  Distribution Alignment:   27.8 / 40

Teams:
  TEAM                  SCORE  GRADE  ISSUES  CAT.RATE  DIST.ALIGN
  example-team-a         82.5  B          89     50.4      32.1
  example-team-b         72.5  C          47     45.0      27.5
```

### Start the API Server

```bash
sankey-scorecard serve \
  --jira-url https://issues.redhat.com \
  --jira-api-token $JIRA_API_TOKEN

# Custom bind address
sankey-scorecard serve --bind-address :9090 \
  --jira-url https://issues.redhat.com \
  --jira-api-token $JIRA_API_TOKEN
```

The server starts with no data loaded. A refresh must be initiated via the API before scorecard endpoints return data.

### Exit Codes

| Code | Meaning |
|------|---------|
| 0 | Success |
| 1 | General error (bad config, Jira auth failure, etc.) |
| 2 | Identifier not found |
| 3 | No data available (refresh-data has not been run) |

## API

The REST API is served under the `/api` prefix. All responses are JSON.

| Method | Path | Description |
|--------|------|-------------|
| `GET` | `/healthz` | Health check (returns `{"status": "ok"}`) |
| `GET` | `/api/` | Returns the OpenAPI specification |
| `GET` | `/api/scorecard` | Returns scorecards, with optional `org`, `pillar`, `team`, `start_date`, `end_date`, `status` query params |
| `POST` | `/api/refresh_data` | Initiates async Jira data refresh (returns 202). Optional `mode`, `start_date`, `end_date`, `status` query params |
| `GET` | `/api/refresh_status` | Returns refresh status (idle/running/completed/failed) |

### Examples

```bash
# Get full scorecard
curl http://localhost:8080/api/scorecard

# Filter by team
curl http://localhost:8080/api/scorecard?team=example-team-b

# Filter by organization
curl http://localhost:8080/api/scorecard?org=example-org

# Filter scorecard by date range and issue status
curl http://localhost:8080/api/scorecard?start_date=2026-01-01&end_date=2026-02-28
curl http://localhost:8080/api/scorecard?status=closed

# Initiate a full data refresh
curl -X POST http://localhost:8080/api/refresh_data

# Scoped refresh: only closed issues in a date range
curl -X POST "http://localhost:8080/api/refresh_data?status=closed&start_date=2026-01-01&end_date=2026-02-28"

# Upsert mode: merge new data without replacing existing
curl -X POST http://localhost:8080/api/refresh_data?mode=upsert

# Check refresh status
curl http://localhost:8080/api/refresh_status
```

### Error Responses

Errors use a consistent structure:

```json
{
  "error": "not_found",
  "message": "Specified entity not found"
}
```

| Status | Meaning |
|--------|---------|
| 400 | Invalid parameters (bad date format, invalid status, or mode=replace with scope params) |
| 404 | Entity not found |
| 409 | Refresh already running, or ambiguous identifier |
| 503 | No data available (refresh has not been run) |

The full OpenAPI 3.0.3 specification is served at `GET /api/` and embedded in the binary.

## Container Image

The application is packaged as a container using a multi-stage build (Go builder + UBI 10 minimal runtime). The binary runs as non-root (`USER 1001`) and listens on port 8080.

```bash
# Build the container image
make build-image

# Run locally
podman run --rm -e JIRA_API_TOKEN=$JIRA_API_TOKEN \
  sankey-scorecard:$(git describe --tags --always) \
  serve --jira-url https://issues.redhat.com
```

## OpenShift Deployment

The application deploys to OpenShift using the internal image registry. Images are pushed via the registry's exposed route and referenced internally by the deployment.

### Prerequisites

- `oc` CLI logged into the target cluster
- A namespace for the deployment
- The internal image registry enabled with its default route exposed
- A secret containing Jira credentials:

```bash
oc create secret generic sankey-scorecard-jira \
  --from-literal=jira-url=https://issues.redhat.com \
  --from-literal=api-token=$JIRA_API_TOKEN \
  -n my-namespace
```

### Deploy

```bash
# Build and deploy (build-image must run first)
make build-image
NAMESPACE=my-namespace make deploy
```

The deploy script handles registry login, image push, manifest application, and rollout. On completion it prints the application route URL.

### Teardown

```bash
NAMESPACE=my-namespace make deploy-teardown
```

This removes the deployment, service, and route but preserves the Jira secret.
