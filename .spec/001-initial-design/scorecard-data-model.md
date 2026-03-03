# Scorecard Data Model

## Overview

The scorecard data model defines the structures returned by the API and rendered by the CLI.
Scorecards are computed on-read from the reconciliation store -- they are not persisted
independently.

Scores are organized by **scoring period** (sprint or fallback time window). Each team has
per-period scores and an aggregate score computed from the combined issue set across all
periods.

## Data Structures

### FullScorecard

Response for `GET /api/scorecard`. Always returned as the top-level wrapper, whether
the request is unfiltered or filtered by `org`, `pillar`, or `team` query parameter.
When filtered, the `organizations` array contains only the matching entities.

```go
type FullScorecard struct {
    GeneratedAt      time.Time             `json:"generated_at"`
    SprintDurationDays int                 `json:"sprint_duration_days"`
    CurrentSprint    TimeWindow            `json:"current_sprint"`
    PreviousSprint   TimeWindow            `json:"previous_sprint"`
    Organizations    []OrganizationScore   `json:"organizations"`
}
```

| Field | Description |
|-------|-------------|
| `GeneratedAt` | Timestamp when the scorecard was computed (time of request) |
| `SprintDurationDays` | Configured sprint duration in days (default: 21) |
| `CurrentSprint` | Time window for the current (active) sprint |
| `PreviousSprint` | Time window for the previous sprint |
| `Organizations` | List of all organizations with their scores |

### OrganizationScore

Nested in `FullScorecard`. When filtering by `?org=hcm`, the `organizations` array
contains only the matching organization.

```go
type OrganizationScore struct {
    Name       string        `json:"name"`
    Identifier string        `json:"identifier"`
    Path       string        `json:"path"`
    Score      Score         `json:"score"`
    Pillars    []PillarScore `json:"pillars"`
}
```

### PillarScore

Nested in `OrganizationScore`. When filtering by `?pillar=hcm/rosa`, the response
contains only the matching organization and pillar.

```go
type PillarScore struct {
    Name       string      `json:"name"`
    Identifier string      `json:"identifier"`
    Path       string      `json:"path"`
    Score      Score       `json:"score"`
    Teams      []TeamScore `json:"teams"`
}
```

### TeamScore

Nested in `PillarScore`. When filtering by `?team=hcm/rosa/rosa-aurora`, the response
contains only the matching organization, pillar, and team.

Teams are the base scoring unit. The aggregate `score` and `distribution` are computed from
the combined issue set across all periods. The `periods` array provides per-sprint (or
per-window) breakdowns.

```go
type TeamScore struct {
    Name         string               `json:"name"`
    Identifier   string               `json:"identifier"`
    Path         string               `json:"path"`
    Score        Score                 `json:"score"`
    Distribution ActivityDistribution  `json:"distribution"`
    Periods      []PeriodScore        `json:"periods"`
}
```

### PeriodScore

A score for a single scoring period (sprint). Allows stakeholders to see trends across
sprints.

```go
type PeriodScore struct {
    Label        string               `json:"label"`
    Window       TimeWindow           `json:"window"`
    Current      bool                 `json:"current"`
    Score        Score                `json:"score"`
    Distribution ActivityDistribution `json:"distribution"`
}
```

### TimeWindow

The time boundaries for a scoring period. Calculated from the configured
`sprint_reference_date` and `sprint_duration_days`.

```go
type TimeWindow struct {
    Since time.Time `json:"since"`
    Until time.Time `json:"until"`
}
```

### Score

The scoring breakdown, present at every level (team, pillar, organization).

```go
type Score struct {
    Total                  *float64 `json:"total"`                   // 0-100, nil if no data
    Grade                  string   `json:"grade"`                   // A, B, C, D, F, or "-"
    CategorizationRate     *float64 `json:"categorization_rate"`     // 0-60, nil if no data
    DistributionAlignment  *float64 `json:"distribution_alignment"`  // 0-40, nil if no data
    IssueCount             int      `json:"issue_count"`             // total scored issues
}
```

**Nil scores:** When a team has zero scored issues, all score fields are `nil` and the grade
is `"-"`. This distinguishes "no data" from "scored 0." In JSON, nil fields serialize as
`null`.

### ActivityDistribution

Issue counts per Sankey category. Present only at the team level.

```go
type ActivityDistribution struct {
    AssociateWellness    int `json:"associate_wellness"`
    IncidentsSupport     int `json:"incidents_support"`
    SecurityCompliance   int `json:"security_compliance"`
    QualityStability     int `json:"quality_stability"`
    FutureSustainability int `json:"future_sustainability"`
    ProductPortfolio     int `json:"product_portfolio"`
    Uncategorized        int `json:"uncategorized"`
}
```

## JSON Response Examples

### Team Scorecard

```json
{
  "name": "Aurora",
  "identifier": "rosa-aurora",
  "path": "hcm/rosa/rosa-aurora",
  "score": {
    "total": 72.5,
    "grade": "C",
    "categorization_rate": 45.0,
    "distribution_alignment": 27.5,
    "issue_count": 47
  },
  "distribution": {
    "associate_wellness": 2,
    "incidents_support": 8,
    "security_compliance": 5,
    "quality_stability": 12,
    "future_sustainability": 3,
    "product_portfolio": 10,
    "uncategorized": 7
  },
  "periods": [
    {
      "label": "2026-01-21 to 2026-02-11",
      "window": {
        "since": "2026-01-21T00:00:00Z",
        "until": "2026-02-11T00:00:00Z"
      },
      "current": false,
      "score": {
        "total": 68.0,
        "grade": "C",
        "categorization_rate": 42.0,
        "distribution_alignment": 26.0,
        "issue_count": 22
      },
      "distribution": {
        "associate_wellness": 1,
        "incidents_support": 4,
        "security_compliance": 2,
        "quality_stability": 6,
        "future_sustainability": 1,
        "product_portfolio": 5,
        "uncategorized": 3
      }
    },
    {
      "label": "2026-02-11 to 2026-03-04",
      "window": {
        "since": "2026-02-11T00:00:00Z",
        "until": "2026-03-04T00:00:00Z"
      },
      "current": true,
      "score": {
        "total": 76.8,
        "grade": "B",
        "categorization_rate": 48.0,
        "distribution_alignment": 28.8,
        "issue_count": 25
      },
      "distribution": {
        "associate_wellness": 1,
        "incidents_support": 4,
        "security_compliance": 3,
        "quality_stability": 6,
        "future_sustainability": 2,
        "product_portfolio": 5,
        "uncategorized": 4
      }
    }
  ]
}
```

### Team with No Data

```json
{
  "name": "New Team",
  "identifier": "new-team",
  "path": "hcm/rosa/new-team",
  "score": {
    "total": null,
    "grade": "-",
    "categorization_rate": null,
    "distribution_alignment": null,
    "issue_count": 0
  },
  "distribution": {
    "associate_wellness": 0,
    "incidents_support": 0,
    "security_compliance": 0,
    "quality_stability": 0,
    "future_sustainability": 0,
    "product_portfolio": 0,
    "uncategorized": 0
  },
  "periods": []
}
```

### Full Scorecard (abbreviated)

```json
{
  "generated_at": "2026-02-15T14:30:00Z",
  "sprint_duration_days": 21,
  "current_sprint": {
    "since": "2026-02-11T00:00:00Z",
    "until": "2026-03-04T00:00:00Z"
  },
  "previous_sprint": {
    "since": "2026-01-21T00:00:00Z",
    "until": "2026-02-11T00:00:00Z"
  },
  "organizations": [
    {
      "name": "Hybrid Cloud Management",
      "identifier": "hcm",
      "path": "hcm",
      "score": {
        "total": 68.3,
        "grade": "C",
        "categorization_rate": 40.8,
        "distribution_alignment": 27.5,
        "issue_count": 1847
      },
      "pillars": [
        {
          "name": "ROSA",
          "identifier": "rosa",
          "path": "hcm/rosa",
          "score": {
            "total": 71.0,
            "grade": "C",
            "categorization_rate": 43.2,
            "distribution_alignment": 27.8,
            "issue_count": 523
          },
          "teams": ["..."]
        }
      ]
    }
  ]
}
```

### Refresh Status

```json
{
  "status": "completed",
  "started_at": "2026-02-07T14:28:00Z",
  "completed_at": "2026-02-07T14:29:45Z",
  "error": null,
  "issue_count": 1847
}
```

## CLI Plain-Text Output

The `plain` output format renders a human-readable table. Example for a pillar:

```
ROSA Scorecard
Generated: 2026-02-07 14:30 UTC | Issues: 523

Pillar Score: 71.0 (C)
  Categorization Rate:      43.2 / 60
  Distribution Alignment:   27.8 / 40

Teams:
  TEAM                  SCORE  GRADE  ISSUES  CAT.RATE  DIST.ALIGN
  rosa-coffee            82.5  B          89     50.4      32.1
  rosa-aurora            72.5  C          47     45.0      27.5
  rosa-hulk              65.0  C          53     39.0      26.0
  rosa-fedramp-core      58.3  D          31     33.6      24.7
```

Example for a team (showing per-sprint breakdown):

```
Aurora Scorecard
Generated: 2026-02-15 14:30 UTC | Issues: 47

Team Score: 72.5 (C)
  Categorization Rate:      45.0 / 60
  Distribution Alignment:   27.5 / 40

Sprint Breakdown:
  SPRINT                          SCORE  GRADE  ISSUES  CAT.RATE  DIST.ALIGN
  2026-01-21 to 2026-02-11         68.0  C          22     42.0      26.0
  2026-02-11 to 2026-03-04 *       76.8  B          25     48.0      28.8

  * = current sprint

Activity Distribution (aggregate):
  Associate Wellness:      2  ( 5.0%)  target: 12%
  Incidents & Support:     8  (20.0%)  target: 12%
  Security & Compliance:   5  (12.5%)  target: 12%
  Quality / Stability:    12  (30.0%)  target: 22%
  Future Sustainability:   3  ( 7.5%)  target: 21%
  Product / Portfolio:    10  (25.0%)  target: 21%
  Uncategorized:           7
```

## Error Responses

All error responses use a consistent structure:

```go
type ErrorResponse struct {
    Error   string `json:"error"`
    Message string `json:"message"`
}
```

| HTTP Status | Error | When |
|------------|-------|------|
| 400 | `bad_request` | Multiple filters provided, or malformed qualified identifier |
| 404 | `not_found` | Organization, pillar, or team identifier not found |
| 409 | `conflict` | Refresh already in progress |
| 503 | `no_data` | No data available (refresh has never been run) |

## Package Location

```
pkg/scorecard/
    scorecard.go   - Score computation, aggregation, grade assignment
    types.go       - FullScorecard, OrganizationScore, PillarScore, TeamScore, Score
    presenter.go   - JSON serialization, plain-text table formatting
```
