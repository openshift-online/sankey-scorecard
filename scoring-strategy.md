# Scoring Strategy

> **This is the authoritative scoring strategy specification.** All future scoring strategy
> changes should be documented here. This file is the living spec; `.spec/` contains the
> original design documents and should not be updated.

## Overview

The Sankey Scorecard evaluates teams on two dimensions: (1) whether they have adopted the
Sankey framework by categorizing their work, and (2) how well their work distribution across
activity type categories aligns with the target distribution defined by the Sankey diagram.
Each team receives a composite score from 0 to 100. Pillar and organization scores are
aggregated from team scores.

## Scored Issue Types

By default, the following Jira issue types are included in scoring:

- Story
- Bug
- Task

Container types (Epic, Initiative, Feature) are **excluded** from scoring by default. These
represent planning containers rather than sprint-level capacity allocation, which is what the
Sankey framework evaluates.

Scored issue types are configurable via the `scored_issue_types` field in the resource map.

## Time Scope & Issue Collection

Issues are collected in two distinct sets, each scored independently:

1. **Closed Issues** -- Issues resolved within each scoring cadence. These represent
   completed work and are scoped by cadence time windows.
2. **In-Progress Issues** -- Issues currently in an in-progress-like status (configurable).
   These represent work being actively done and are collected as a single set with no
   time window filter.

This two-set approach replaces the previous `updated` date filtering, which was unreliable
because issues can be updated long after completion or before work starts.

### Scoring Cadences

Cadence boundaries are calculated from two configuration values:

- **`sprint_reference_date`** -- A known cadence start date (default: `2026-02-11`).
- **`sprint_duration_days`** -- Cadence length in days (default: `21`, i.e., 3 weeks).

The reconciler computes the **current cadence** and **previous cadence** from the reference
date, producing two closed scoring periods plus one in-progress period (three total).

All cadence boundaries are computed as multiples of `sprint_duration_days` from the
reference date. No Jira API calls are needed for cadence discovery.

### Closed Issue JQL

Closed issues use `statusCategory = Done` combined with `resolved` date filtering:

```
<ownership clause> AND issuetype in (...) AND statusCategory = Done
  AND resolved >= "<cadence_start>" AND resolved <= "<cadence_end>"
```

### In-Progress Issue JQL

In-progress issues use the configured `in_progress_statuses` list with no date filter:

```
<ownership clause> AND issuetype in (...) AND status in ("<status1>", "<status2>", ...)
```

The `in_progress_statuses` list is configured in the resource map under `jira:`:

```yaml
jira:
  in_progress_statuses:
    - In Progress
    - Code Review
    - Review
```

### Scorecard Sections

Each team scorecard is divided into two sections:

- **Past (Closed)** -- Contains per-cadence scores for closed issues. Each cadence gets
  its own score. The section score is the aggregate across cadences.
- **Current (In Progress)** -- Contains a single score for currently in-progress issues.

The **overall team score** aggregates across all periods in both sections (closed + in-progress).

### Per-Cadence Scoring

Each scoring period receives its own independent score. This allows stakeholders to see
trends across cadences -- whether a team's Sankey adherence is improving or declining.

The **aggregate score** for a team is computed from the combined issue set across all
scoring periods (closed + in-progress). This is the headline number shown at the team,
pillar, and organization level.

### CLI Override

The `--since` CLI flag on `refresh-data` overrides cadence-based scoping entirely. When
specified, the tool ignores the cadence calendar and uses the provided date as the start of
a single closed scoring period ending at the current date, plus one in-progress period.
This is useful for ad-hoc analysis of a specific time range.

## Ownership Methods

Teams define ownership of Jira issues using one of four methods:

### `component`

Ownership is defined by project + component combinations:

```yaml
ownership:
  method: component
  project: ARO
  components:
    - clusters-service
```

### `team_field`

Ownership is defined by the "Team" custom field value on issues in a project:

```yaml
ownership:
  method: team_field
  project: SREP
  team_field_value: "5695"
```

### `jql`

Ownership is defined by an arbitrary JQL query:

```yaml
ownership:
  method: jql
  jql: "project = ACM AND component in (\"Search\")"
```

### `sprint_board`

Ownership is defined by sprint membership on specific Jira boards. This method is used
for teams that define ownership as "issues pulled into my sprint" -- particularly when
multiple teams share a project and components (e.g., Coffee and Focaccia in OCM).

```yaml
ownership:
  method: sprint_board
  project: OCM
  boards:
    - 7488
```

Since Jira Server/Data Center's `openSprints()` and `closedSprints()` JQL functions do
not accept board ID parameters, the scorecard discovers sprint IDs from the Agile REST API
(`GET /rest/agile/1.0/board/{boardId}/sprint?state=active,closed`) and uses those IDs in
JQL via `sprint in (id1, id2, ...)`.

Board IDs must be discovered from Jira and configured in the resource map.

## Sankey Categories

There are six Sankey activity type categories. Issues are still categorized into all six
for categorization rate scoring (Dimension 1). However, **Associate Wellness & Development
is excluded from distribution alignment scoring** (Dimension 2) because this category is
not actively used in practice. Teams with 0% in this category are not penalized, and teams
with >0% do not receive a benefit.

The five **scored categories** are listed below in priority order:

| Priority Rank | Category |
|---------------|----------|
| 1 (highest) | Incidents & Support |
| 2 | Security & Compliance |
| 3 | Quality / Stability / Reliability |
| 4 | Future Sustainability |
| 5 (lowest) | Product / Portfolio Work |

The **excluded category** (still used for categorization rate):

| Category | Status |
|----------|--------|
| Associate Wellness & Development | Excluded from distribution alignment |

The mapping from Jira Activity Type field values to these categories is hardcoded in
the scoring package. The Jira Activity Type field uses static, well-known values that do
not vary between deployments (e.g., "Tech Debt" maps to Quality / Stability / Reliability).

## Dimensions

### 1. Categorization Rate (70 points)

Measures the percentage of scored issues that have the Activity Type custom field populated.
This is the fundamental adoption metric: a team that does not categorize its work has not
implemented the Sankey framework. Adoption is the primary goal, so this dimension carries the
majority of the score weight.

All six categories (including Associate Wellness) count toward categorization rate.

**Formula:**

```
categorized_count  = count of issues where Activity Type is set
total_count        = count of all scored issues
categorization_pct = categorized_count / total_count

score = categorization_pct * 70
```

**Edge case:** If `total_count` is 0, the team receives a score of `nil` (No Data) rather
than 0. A nil score indicates missing data, not poor adherence.

### 2. Distribution Alignment (30 points)

Measures how well the team's actual work distribution across the five scored categories
matches the target distribution. Associate Wellness issues are excluded from both the
numerator and denominator of this calculation. Only categorized, non-AW issues are evaluated.

#### Target Distribution

The target distribution represents the ideal percentage of work in each scored category.
Associate Wellness's original 12% target has been redistributed proportionally among the
remaining five categories (each original target divided by 0.88).

**Target distribution:**

| Category | Original Target | Scored Target |
|----------|----------------|---------------|
| Associate Wellness & Development | 12% | _(excluded)_ |
| Incidents & Support | 12% | ~13.6% (12/88) |
| Security & Compliance | 12% | ~13.6% (12/88) |
| Quality / Stability / Reliability | 22% | 25.0% (22/88) |
| Future Sustainability | 21% | ~23.9% (21/88) |
| Product / Portfolio Work | 21% | ~23.9% (21/88) |

Scored target percentages sum to 100%.

#### Priority-Weighted Deviation

For each scored category, the deviation from target is computed and weighted based on the
category's priority rank. The weighting is asymmetric:

- **Over-allocation in high-priority categories** is penalized less.
- **Over-allocation in low-priority categories** is penalized more.
- **Under-allocation in high-priority categories** is penalized more.
- **Under-allocation in low-priority categories** is penalized less.

**Penalty weights by priority rank (5 scored categories):**

| Priority Rank | Over-Allocation Weight | Under-Allocation Weight |
|---------------|----------------------|------------------------|
| 1 (highest) | 0.50 | 1.50 |
| 2 | 0.75 | 1.25 |
| 3 | 1.00 | 1.00 |
| 4 | 1.25 | 0.75 |
| 5 (lowest) | 1.50 | 0.50 |

These weights are derived from the formula:

```
over_weight(rank)  = 0.5 + (rank - 1) * 0.25
under_weight(rank) = 1.5 - (rank - 1) * 0.25
```

The step size is 0.25 (rather than 0.2) to span the full [0.5, 1.5] range across 5
categories instead of 6.

#### Formula

```
scored_count = categorized_count - associate_wellness_count

For each scored category i (priority rank 1 to 5):
  actual_pct[i] = issues_in_category[i] / scored_count
  target_pct[i] = configured target percentage for category i
  deviation[i]  = actual_pct[i] - target_pct[i]

  if deviation[i] > 0:   (over-allocated)
    penalty[i] = deviation[i] * over_weight[i]
  else:                   (under-allocated)
    penalty[i] = |deviation[i]| * under_weight[i]

total_penalty = sum(penalty[i]) for all scored categories

alignment_pct = max(0, 1 - total_penalty)
score = alignment_pct * 30
```

**Edge case:** If `scored_count` is 0 (all categorized issues are AW, or no categorized
issues at all), the team scores 0 for this dimension.

#### Worked Examples

All examples use the default target distribution. 100 total categorized issues.

**Example 1: Well-aligned team**

Actual: [14 AW, 11 I&S, 11 S&C, 21 Q/S/R, 22 FS, 21 P/PW]
scored_count = 100 - 14 = 86

| Category | Rank | Actual | Target | Deviation | Direction | Weight | Penalty |
|----------|------|--------|--------|-----------|-----------|--------|---------|
| I&S | 1 | 12.8% | 13.6% | -0.8% | under | 1.50 | 0.013 |
| S&C | 2 | 12.8% | 13.6% | -0.8% | under | 1.25 | 0.011 |
| Q/S/R | 3 | 24.4% | 25.0% | -0.6% | under | 1.00 | 0.006 |
| FS | 4 | 25.6% | 23.9% | +1.7% | over | 1.25 | 0.021 |
| P/PW | 5 | 24.4% | 23.9% | +0.6% | over | 1.50 | 0.008 |

total_penalty = 0.059
alignment_pct = 1 - 0.059 = 0.941
score = 0.941 * 30 = **28.2 / 30**

**Example 2: Skewed toward bottom category**

Actual: [5 AW, 5 I&S, 5 S&C, 10 Q/S/R, 25 FS, 50 P/PW]
scored_count = 100 - 5 = 95

| Category | Rank | Actual | Target | Deviation | Direction | Weight | Penalty |
|----------|------|--------|--------|-----------|-----------|--------|---------|
| I&S | 1 | 5.3% | 13.6% | -8.4% | under | 1.50 | 0.126 |
| S&C | 2 | 5.3% | 13.6% | -8.4% | under | 1.25 | 0.105 |
| Q/S/R | 3 | 10.5% | 25.0% | -14.5% | under | 1.00 | 0.145 |
| FS | 4 | 26.3% | 23.9% | +2.5% | over | 1.25 | 0.031 |
| P/PW | 5 | 52.6% | 23.9% | +28.8% | over | 1.50 | 0.432 |

total_penalty = 0.837
alignment_pct = 1 - 0.837 = 0.163
score = 0.163 * 30 = **4.9 / 30**

**Example 3: Skewed toward top (AW-heavy, excluded)**

Actual: [50 AW, 25 I&S, 10 S&C, 5 Q/S/R, 5 FS, 5 P/PW]
scored_count = 100 - 50 = 50

| Category | Rank | Actual | Target | Deviation | Direction | Weight | Penalty |
|----------|------|--------|--------|-----------|-----------|--------|---------|
| I&S | 1 | 50.0% | 13.6% | +36.4% | over | 0.50 | 0.182 |
| S&C | 2 | 20.0% | 13.6% | +6.4% | over | 0.75 | 0.048 |
| Q/S/R | 3 | 10.0% | 25.0% | -15.0% | under | 1.00 | 0.150 |
| FS | 4 | 10.0% | 23.9% | -13.9% | under | 0.75 | 0.104 |
| P/PW | 5 | 10.0% | 23.9% | -13.9% | under | 0.50 | 0.069 |

total_penalty = 0.553
alignment_pct = 1 - 0.553 = 0.447
score = 0.447 * 30 = **13.4 / 30**

Note: In this example, the 50 AW issues are entirely excluded from distribution alignment.
The score reflects only how the remaining 50 issues are distributed among the 5 scored
categories. The team's categorization rate still benefits from all 100 issues being
categorized.

## Composite Score

```
total = categorization_rate_score + distribution_alignment_score
```

Range: 0-100 (or nil if the team has no scored issues).

## Grade Scale

| Grade | Score Range | Description |
|-------|------------|-------------|
| A | 90 - 100 | Excellent Sankey adherence |
| B | 75 - 89 | Good adherence with minor gaps |
| C | 60 - 74 | Moderate adherence, improvement needed |
| D | 45 - 59 | Poor adherence, significant gaps |
| F | 0 - 44 | Minimal Sankey adoption |
| - | nil | No data (team has zero scored issues) |

## Aggregation

### Team Level

Teams are the base scoring unit. All dimensions are computed directly from the team's
reconciled issues.

### Pillar Level

Pillar scores are a weighted average of team scores, weighted by each team's scored issue
count. Teams with nil scores (no data) are excluded from aggregation.

```
pillar_score = sum(team_score * team_issue_count) / sum(team_issue_count)
```

If all teams in a pillar have nil scores, the pillar score is also nil.

### Organization Level

Organization scores are a weighted average of pillar scores, weighted by each pillar's
total issue count (sum of its teams' issue counts).

```
org_score = sum(pillar_score * pillar_issue_count) / sum(pillar_issue_count)
```

## Change Log

- **2026-02-19**: Changed scoring weight split from 60/40 to 70/30. Categorization Rate
  now worth 70 points (was 60); Distribution Alignment now worth 30 points (was 40).
  This increases the emphasis on adoption (categorizing work) relative to distribution
  alignment.

- **2026-02-09**: Replaced `updated` date filtering with two-set issue collection:
  closed issues (resolved within cadence) and in-progress issues (current status).
  Added `sprint_board` ownership method for teams that define ownership via board
  sprints (uses Agile API for sprint discovery). Added `in_progress_statuses`
  configuration. Scorecard output now shows sections (Past/Current) instead of flat
  sprint breakdown. Renamed "sprint" terminology to "cadence" in output.

- **2026-02-08**: Excluded Associate Wellness & Development from distribution alignment
  scoring. AW issues still count toward categorization rate but no longer affect
  distribution alignment. Target distribution renormalized across 5 categories. Penalty
  weight step changed from 0.2 to 0.25 for 5 categories.
