# Sankey Scorecard

Sankey Scorecard intends to review Jira ticketing data to assign a score to teams on their accurate utilization of
sankey in their planning and execution processes.

## Concepts

### Organizational Structure

The structure of teams is divided into:
- top-level organizations (ex: Hybrid Platfroms, Hybrid Cloud Management)
- mid-level Pillars (ex: ROSA, Fleet Engineering)
- Teams (ex: Aurora, Coffee)

Identifiers for all organizations, pillars, and teams are expected to be unique.

### Jira

Jira is the ticketing system used to track work in "issues".

#### Issue Ownership by Team

Teams track which issues belong to them differently for each team. The known ways are:
- A team owns all of the issues in $project with components in the list $componentList
- A team owns all of the issues in $project with the `Team` custom field set to the value $jiraTeamName
  - Note that the value of Team name can differ from the actual team name
- A team owns all of the issues resulting from a custom JQL query: $jqlQuery

#### Relevant Issue Details

**Issue Type**: Supported types are: Epic, Bug, Story, Task, Initiative, Feature
**Issue Team**: Custom field defining team ownership, only used by some teams
**Issue Component**: List of project-specific components the issue applies to
**Issue Acitivity Type**: Custom field primarily used for sankey adherence evaluation
**Issue Status**: Open, Closed, In Progress, etc.

#### Data Reconciliation

Jira data should be gathered and reconciled only when a refresh is initiated. Data will not be reconciled automatically.

Jira data that is gathered is stored in short-term storage. Jira data is always overwritten on refresh, historical jira
data will not be maintained.

## Components

### API
Simple JSON rest API supporting the following routes:

GET /api/
  Returns openAPI Spec
GET /api/scorecard
  Returns the full scorecard for all organizations, pillars, and teams
GET /api/scorecard/$orgName
  Returns the scorecard for a single organization
GET /api/scorecard/$orgName/$pillarName
  Returns the scorecard for a pillar within an organization
GET /api/scorecard/$orgName/$pillarName/$teamName
  Returns the scorecard for a single team
POST /api/refresh_data
  Initiates an asynchronous reconciliation task

### Jira Reconciler

### CLI
Basic Golang CLI using viper / cobra supporting the following interface:

```
$ sankey-scorecard $organizationalIdentifier
  # Returns the scorecard in human-readable format
  # Supports -o / --output with values json, yaml, plain (default: plain)
$ sankey-scorecard refresh-data
  # Reconciles jira data
$ sankey-scorecard serve
  # Starts the API server synchronously
$ sankey-scorecard help
  # Help output
```

All sub-commands support `--help` option.

The `$organizationalIDentifier` represents either an organization, pillar, or team.

The CLI uses the same packages as the API. The CLI is a thin layer in `cmd/` and should contain minimal unique logic.

### Tests
The repository will contain an integration test suite under tests/ using ginkgo.

Coverage must be reported on every test run as part of the test suite. The test suite is considered a failure if
coverage is lower than 80%.

A Jira mock server is created. Data for the jira mock server to exposed is stored in easily maintained YAML files,
similar to the example in https://github.com/andygrunwald/go-jira/tree/main/testing/mock-data

The CLI will not be tested, only the API and the various packages.

### Web UI
Future enhancement, do not implement. The API is exposed under /api to support eventually hosting a WebUI from the same
binary as the API.

## Technical requirements
- MUST use an official jira SDK to gather jira data
- MUST support use of a jira PAT (Personal Access Token) to grant access to data
- MUST support both Jira Cloud and a hosted jira instance
- MUST be writtin in Golang

### API Server Configuration
At minimum, we must support:
- Jira URL
- Jira PAT access credential
- Jira Resource Map

#### Jira Resource Map

A map must exist in YAML format defining the organization, pillars, teams, and how each of the teams defines the issues
it owns. This is maintained in the config/ directory.

### Code Organization

Primary code artifacts divided into golang packages in pkg/

API Handlers are defined in pkg/handlers.

Configuration handling in pkg/config

CLI interface defined in cmd/, including flag parsing into configuration

Makefile exists defining targets:
- `build`: Uses go build to build the binary in the root dir
- `install`: Uses go install to install the tool
- `test`: Run unit tests only
- `test-integration`: Run integration tests only
- `lint`: Run linter
- `test-all`: Run all tests, inluding linter

### Code Linting
A linter must be implemented. Default configuration is acceptable, research and follow industry-wide golang best
practices.

## References
See @tmp/references for additional context

Potential Jira SDK choices:
- https://github.com/ctreminiom/go-atlassian
- https://github.com/andygrunwald/go-jira
