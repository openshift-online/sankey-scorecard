# CLI Specification

## Overview

The `sankey-scorecard` CLI is built with Cobra/Viper and serves as a thin layer over the
same packages used by the API. The CLI operates in two modes:

1. **Direct mode** - Fetches Jira data and computes scores locally (no running server needed)
2. **Server mode** - Starts the HTTP API server

All subcommands support `--help`. Global flags are inherited by all subcommands.

## Command Structure

```
sankey-scorecard                        Root command (shows help with no args)
sankey-scorecard <identifier>           Show scorecard for an org, pillar, or team
sankey-scorecard refresh-data           Fetch Jira data and compute scores
sankey-scorecard serve                  Start the API server
sankey-scorecard version                Print version information
sankey-scorecard help [command]         Help for any command
```

The `<identifier>` argument is the name of an organization, pillar, or team. All
identifiers are expected to be globally unique, so a single name is sufficient
(e.g., `sankey-scorecard aurora`).

If an identifier is ambiguous (matches entities at different levels or multiple
entities of the same type), the CLI exits with code 1 and prints a message listing
the conflicts and instructing the user to disambiguate using a slash-delimited path:

| Segments | Format | Level | Example |
|----------|--------|-------|---------|
| 1 | `{name}` | Unique identifier | `aurora` |
| 2 | `{pillar}/{team}` | Disambiguate team | `rosa/aurora` |
| 3 | `{org}/{pillar}/{team}` | Fully qualified | `hcm/rosa/aurora` |

---

## Help Output

### Root Command

```
$ sankey-scorecard --help
Evaluate teams on their Sankey planning framework adherence by analyzing
Jira issue data and producing scorecard reports.

When called with an identifier, displays the scorecard for that entity.
Identifiers are globally unique names. If ambiguous, use a slash-delimited
path to disambiguate (e.g., rosa/aurora or hcm/rosa/aurora).

Usage:
  sankey-scorecard [identifier] [flags]
  sankey-scorecard [command]

Available Commands:
  refresh-data  Reconcile Jira data and compute scores
  serve         Start the API server
  version       Print version information
  help          Help about any command

Global Flags:
  -c, --config string   Path to resource map config file (default: embedded)
  -h, --help            Help for sankey-scorecard

Use "sankey-scorecard [command] --help" for more information about a command.
```

### Scorecard Lookup (root command with identifier)

```
$ sankey-scorecard <identifier> --help
Display the scorecard for an organization, pillar, or team.

The identifier is the name of the entity. All identifiers are expected to
be globally unique, so a single name is sufficient. If the identifier is
ambiguous, use a slash-delimited path to disambiguate:
  {pillar}/{team}            e.g., rosa/aurora
  {org}/{pillar}/{team}      e.g., hcm/rosa/aurora

The scorecard is computed from previously reconciled Jira data. Data must be
loaded first via the refresh-data command. If no data has been loaded, the
command exits with code 3.

The scoring window covers the current sprint and the previous sprint,
calculated from the configured reference date and sprint duration
(default: 3-week sprints starting from 2026-02-11).

Usage:
  sankey-scorecard <identifier> [flags]

Flags:
  -o, --output string                Output format: plaintext, json, yaml (default "plaintext")
  -h, --help                         Help for this command

Global Flags:
  -c, --config string   Path to resource map config file (default: embedded)

Examples:
  # Show scorecard for a team by name
  sankey-scorecard aurora

  # Show scorecard for a team in JSON
  sankey-scorecard aurora -o json

  # Disambiguate if "aurora" matches multiple entities
  sankey-scorecard rosa/aurora

  # Show scorecard for an organization
  sankey-scorecard hcm

  # Use a custom config
  sankey-scorecard hcm --config ./my-config.yaml
```

### refresh-data

```
$ sankey-scorecard refresh-data --help
Fetch current Jira issue data for all configured teams and compute scores.

Runs synchronously, printing progress to stdout. The scoring window covers
the current sprint and the previous sprint, calculated from the configured
reference date and sprint duration (default: 3-week sprints).

Fetched data is stored in memory and available for subsequent scorecard
lookups.

This command validates connectivity and data quality without rendering
a scorecard.

Usage:
  sankey-scorecard refresh-data [flags]

Flags:
      --jira-url string              Jira instance URL (required)
      --jira-pat string              Jira Personal Access Token (required)
      --activity-type-field string   Jira custom field ID for Activity Type
                                      (required; e.g., customfield_12319440)
      --since string                 Override sprint calendar; include issues updated since
                                      this date (YYYY-MM-DD). When set, the sprint calendar
                                      is ignored and a single period is used.
  -h, --help                         Help for refresh-data

Global Flags:
  -c, --config string   Path to resource map config file (default: embedded)

Examples:
  # Refresh with a custom time scope (overrides sprint calendar)
  sankey-scorecard refresh-data \
    --jira-url https://issues.redhat.com \
    --jira-pat $MY_TOKEN \
    --activity-type-field customfield_12319440 \
    --since 2026-01-01
```

### serve

```
$ sankey-scorecard serve --help
Start the HTTP API server for serving scorecard data.

The server exposes REST endpoints for querying scorecards and initiating
data refreshes. No data is loaded on startup; a refresh must be initiated
via POST /api/refresh_data before scorecard endpoints return data.

Usage:
  sankey-scorecard serve [flags]

Flags:
      --bind-address string          Server listen address (default ":8080")
      --jira-url string              Jira instance URL (required)
      --jira-pat string              Jira Personal Access Token (required)
      --activity-type-field string   Jira custom field ID for Activity Type
                                      (required; e.g., customfield_12319440)
  -h, --help                         Help for serve

Global Flags:
  -c, --config string   Path to resource map config file (default: embedded)

Examples:
  # Start server with default settings
  sankey-scorecard serve --jira-url https://issues.redhat.com \
    --activity-type-field customfield_12319440

  # Start on a custom port
  sankey-scorecard serve --bind-address :9090 --jira-url https://issues.redhat.com
```

### version

```
$ sankey-scorecard version --help
Print the version, build date, and commit hash of the sankey-scorecard binary.

Usage:
  sankey-scorecard version [flags]

Flags:
  -h, --help   Help for version
```

### version output

```
$ sankey-scorecard version
sankey-scorecard v0.1.0 (commit: abc1234, built: 2026-02-07T10:00:00Z)
```

---

## Exit Codes

| Code | Meaning |
|------|---------|
| 0 | Success |
| 1 | General error (bad config, Jira auth failure, etc.) |
| 2 | Identifier not found |
| 3 | No data available (refresh-data has not been run) |

---

## Output Formats

### plaintext (default)

Human-readable table format. See scorecard-data-model.md for examples.

### json

JSON output matching the API response schemas defined in openapi.yaml.

### yaml

YAML serialization of the same structures as JSON.

---

## Implementation Notes

- The CLI is a thin layer in `cmd/`. It parses flags, loads configuration, and delegates
  to packages in `pkg/`.
- The resource map YAML is embedded in the binary at build time using Go's `embed` package.
  The embedded map is used by default. When `-c` / `--config` is provided,
  the external file overrides the embedded resource map entirely.
- The positional argument is resolved by searching all organizations, pillars, and teams
  for a matching identifier. If the argument contains `/`, it is split and used to narrow
  the search hierarchically. If a bare name matches multiple entities, the CLI exits with
  code 1 and lists the conflicts with disambiguation instructions.
- `refresh-data` in CLI mode runs synchronously (blocks until complete). In server mode
  (`POST /api/refresh_data`), it runs asynchronously.
- The scorecard lookup reads from a previously populated in-memory store. If no prior
  refresh has been done in the same process, it exits with code 3.
