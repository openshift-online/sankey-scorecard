# Scoring Model

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

## Time Scope

Issues are evaluated within scoring periods aligned to a **calculated sprint calendar**.
The reconciler determines the **current sprint** and the **previous sprint** from a
configured reference date, producing two scoring periods. Scores are computed per-period
and aggregated across periods.

Sprint boundaries are calculated from two configuration values:

- **`sprint_reference_date`** -- A known sprint start date (default: `2026-02-11`).
- **`sprint_duration_days`** -- Sprint length in days (default: `21`, i.e., 3 weeks).

All sprint boundaries are computed as multiples of `sprint_duration_days` from the
reference date. No Jira API calls are needed for sprint discovery.

### Per-Sprint Scoring

Each scoring period receives its own independent score. This allows stakeholders to see
trends across sprints -- whether a team's Sankey adherence is improving or declining.

The **aggregate score** for a team is computed from the combined issue set across both
scoring periods. This is the headline number shown at the team, pillar, and organization
level. Per-period scores are available in the detailed team view.

### CLI Override

The `--since` CLI flag on `refresh-data` overrides sprint-based scoping entirely. When
specified, the tool ignores the sprint calendar and uses the provided date as the start of
a single scoring period ending at the current date. This is useful for ad-hoc analysis of
a specific time range.

## Sankey Categories

The six Sankey activity type categories are listed below in priority order. The ordering is
intentional: categories at the top of the Sankey diagram are considered more critical than
those at the bottom. This ordering affects how distribution deviations are penalized (see
Distribution Alignment).

| Priority Rank | Category |
|---------------|----------|
| 1 (highest) | Associate Wellness & Development |
| 2 | Incidents & Support |
| 3 | Security & Compliance |
| 4 | Quality / Stability / Reliability |
| 5 | Future Sustainability |
| 6 (lowest) | Product / Portfolio Work |

The priority ranking is configurable via the `category_priority` field in the resource map.

The mapping from Jira Activity Type field values to these six categories is hardcoded in
the scoring package. The Jira Activity Type field uses static, well-known values that do
not vary between deployments (e.g., "Tech Debt" maps to Quality / Stability / Reliability).

## Dimensions

### 1. Categorization Rate (60 points)

Measures the percentage of scored issues that have the Activity Type custom field populated.
This is the fundamental adoption metric: a team that does not categorize its work has not
implemented the Sankey framework. Adoption is the primary goal, so this dimension carries the
majority of the score weight.

**Formula:**

```
categorized_count  = count of issues where Activity Type is set
total_count        = count of all scored issues
categorization_pct = categorized_count / total_count

score = categorization_pct * 60
```

**Rationale:** The Sankey framework requires teams to categorize their work by Activity Type.
A team that does not categorize its work cannot demonstrate intentional planning. The Sankey
usage guide explicitly flags >=10% uncategorized work as a concern requiring management
attention.

**Edge case:** If `total_count` is 0, the team receives a score of `nil` (No Data) rather
than 0. A nil score indicates missing data, not poor adherence.

### 2. Distribution Alignment (40 points)

Measures how well the team's actual work distribution across the six Sankey categories matches
the target distribution defined by the Sankey diagram. Only categorized issues are evaluated
(uncategorized issues are addressed by Dimension 1).

The Sankey framework is fundamentally about work type distribution, not a binary
proactive/reactive split. A team that categorizes all its work but concentrates entirely in
one category has not implemented the Sankey's distribution guidance.

#### Target Distribution

The target distribution represents the ideal percentage of work in each category. This is
configured at the organization level via the `target_distribution` field in the resource map.
There are no pillar or team-level overrides; the target is uniform across the organization.

**Default target distribution:**

| Category | Target % |
|----------|----------|
| Associate Wellness & Development | 12% |
| Incidents & Support | 12% |
| Security & Compliance | 12% |
| Quality / Stability / Reliability | 22% |
| Future Sustainability | 21% |
| Product / Portfolio Work | 21% |

Target percentages must sum to 100%.

#### Priority-Weighted Deviation

For each category, the deviation from target is computed and weighted based on the category's
priority rank. The weighting is asymmetric:

- **Over-allocation in high-priority categories** (spending more than target on top-of-Sankey
  work) is penalized less, since over-investing in the most critical categories is a less
  concerning deviation.
- **Over-allocation in low-priority categories** (spending more than target on bottom-of-Sankey
  work) is penalized more, since it indicates the team is not following the Sankey's
  distribution guidance where it matters most.
- **Under-allocation in high-priority categories** is penalized more, since neglecting critical
  categories is a significant gap.
- **Under-allocation in low-priority categories** is penalized less.

**Penalty weights by priority rank:**

| Priority Rank | Over-Allocation Weight | Under-Allocation Weight |
|---------------|----------------------|------------------------|
| 1 (highest) | 0.5 | 1.5 |
| 2 | 0.7 | 1.3 |
| 3 | 0.9 | 1.1 |
| 4 | 1.1 | 0.9 |
| 5 | 1.3 | 0.7 |
| 6 (lowest) | 1.5 | 0.5 |

These weights are derived from the formula:

```
over_weight(rank)  = 0.5 + (rank - 1) * 0.2
under_weight(rank) = 1.5 - (rank - 1) * 0.2
```

The base weight range (0.5 to 1.5) is configurable via `priority_weight_range` in the
resource map. The default range of `[0.5, 1.5]` means the highest-priority category receives
half the penalty for over-allocation and 1.5x the penalty for under-allocation relative to a
neutral weight of 1.0.

#### Formula

```
For each category i (priority rank 1 to 6):
  actual_pct[i] = issues_in_category[i] / categorized_count
  target_pct[i] = configured target percentage for category i
  deviation[i]  = actual_pct[i] - target_pct[i]

  if deviation[i] > 0:   (over-allocated)
    penalty[i] = deviation[i] * over_weight[i]
  else:                   (under-allocated)
    penalty[i] = |deviation[i]| * under_weight[i]

total_penalty = sum(penalty[i]) for all categories

alignment_pct = max(0, 1 - total_penalty)
score = alignment_pct * 40
```

**Normalization note:** The sum of absolute deviations across all categories ranges from 0
(perfect alignment) to a theoretical maximum of 2.0 (all work in one category with no overlap
with the target). The priority weights shift this range slightly. In practice, `total_penalty`
values above 1.0 represent severely misaligned distributions and correctly yield a score of 0.

**Edge case:** If `categorized_count` is 0, the team scores 0 for this dimension.

**Category coverage:** A category with 0% actual allocation against a non-zero target produces
a deviation penalty, with higher-priority missing categories penalized more heavily. There is
no separate breadth dimension; coverage gaps are naturally captured here.

#### Worked Examples

All examples use the default target distribution: [12%, 12%, 12%, 22%, 21%, 21%].

**Example 1: Well-aligned team**

Actual: [14%, 11%, 11%, 21%, 22%, 21%]

| Category | Rank | Deviation | Direction | Weight | Penalty |
|----------|------|-----------|-----------|--------|---------|
| AWD | 1 | +2% | over | 0.5 | 0.010 |
| I&S | 2 | -1% | under | 1.3 | 0.013 |
| S&C | 3 | -1% | under | 1.1 | 0.011 |
| Q/S/R | 4 | -1% | under | 0.9 | 0.009 |
| FS | 5 | +1% | over | 1.3 | 0.013 |
| P/PW | 6 | 0% | -- | -- | 0.000 |

total_penalty = 0.056
alignment_pct = 1 - 0.056 = 0.944
score = 0.944 * 40 = **37.8 / 40**

**Example 2: Moderate misalignment, skewed toward bottom category**

Actual: [5%, 5%, 5%, 10%, 25%, 50%]

| Category | Rank | Deviation | Direction | Weight | Penalty |
|----------|------|-----------|-----------|--------|---------|
| AWD | 1 | -7% | under | 1.5 | 0.105 |
| I&S | 2 | -7% | under | 1.3 | 0.091 |
| S&C | 3 | -7% | under | 1.1 | 0.077 |
| Q/S/R | 4 | -12% | under | 0.9 | 0.108 |
| FS | 5 | +4% | over | 1.3 | 0.052 |
| P/PW | 6 | +29% | over | 1.5 | 0.435 |

total_penalty = 0.868
alignment_pct = 1 - 0.868 = 0.132
score = 0.132 * 40 = **5.3 / 40**

Note: the heavy over-allocation in the lowest-priority category (Product / Portfolio Work)
drives the penalty. The same degree of over-allocation in a top-priority category would
produce a much lower penalty.

**Example 3: Same total deviation, skewed toward top category**

Actual: [50%, 25%, 10%, 5%, 5%, 5%]

| Category | Rank | Deviation | Direction | Weight | Penalty |
|----------|------|-----------|-----------|--------|---------|
| AWD | 1 | +38% | over | 0.5 | 0.190 |
| I&S | 2 | +13% | over | 0.7 | 0.091 |
| S&C | 3 | -2% | under | 1.1 | 0.022 |
| Q/S/R | 4 | -17% | under | 0.9 | 0.153 |
| FS | 5 | -16% | under | 0.7 | 0.112 |
| P/PW | 6 | -16% | under | 0.5 | 0.080 |

total_penalty = 0.648
alignment_pct = 1 - 0.648 = 0.352
score = 0.352 * 40 = **14.1 / 40**

This is still a poor score due to extreme concentration, but it scores higher than Example 2
despite having larger absolute deviations. The asymmetric weighting reflects that
over-investing in the most critical category is a less harmful deviation than over-investing
in the least critical one.

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

## Configuration Dependencies

The scoring model requires the following configuration in the resource map:

1. **`--activity-type-field`** (runtime) - The Jira custom field ID for Activity Type
   (e.g., `customfield_12345`). This varies between Jira instances but is the same
   across the whole instance. Provided at runtime via the `--activity-type-field` CLI flag,
   not in the resource map.
2. **`jira.scored_issue_types`** - List of issue types to include in scoring.
3. **`sprint_reference_date`** - A known sprint start date used to calculate sprint
   boundaries (default: `2026-02-11`). All sprint windows are derived as multiples of
   `sprint_duration_days` from this date.
4. **`sprint_duration_days`** - Length of each sprint in days (default: `21`).
5. **`target_distribution`** - Target percentage for each of the six Sankey categories.
   Must sum to 100%. Default: top 3 categories at 12%; Q/S/R at 22%; FS and P/PW at 21%.
6. **`category_priority`** - Ordered list of categories from highest to lowest priority.
   Defaults to the Sankey diagram's top-to-bottom visual ordering.
7. **`priority_weight_range`** - Two-element array `[min, max]` defining the penalty weight
   range for priority-based asymmetric scoring. Default: `[0.5, 1.5]`.

## Resolved Design Decisions

1. **Story Points vs Issue Count**: The scoring model uses issue counts. Story point
   weighting may be revisited in the future but is deferred for now as issue counts are
   the best available metric across all teams.

2. **Status Filtering**: All issue statuses are included in scoring. Activity Type should
   be set at issue creation, not closure, so status is not a relevant filter. Status
   filtering may be added as a future enhancement but is not needed now.
