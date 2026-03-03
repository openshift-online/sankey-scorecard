package scorecard

import (
	"math"
	"time"

	"github.com/tiwillia/sankey-scorecard/pkg/config"
	"github.com/tiwillia/sankey-scorecard/pkg/reconciler"
)

// ComputeTeamScore computes the full score for a team from its reconciled data.
func ComputeTeamScore(team config.Team, org config.Organization, pillar config.Pillar, teamData *reconciler.TeamData) TeamScore {
	ts := TeamScore{
		Name:       team.Name,
		Identifier: team.Identifier,
		Path:       org.Identifier + "/" + pillar.Identifier + "/" + team.Identifier,
	}

	// Partition periods by set type
	var closedPeriods, inProgressPeriods []reconciler.ScoringPeriod
	for _, period := range teamData.Periods {
		switch period.SetType {
		case reconciler.IssueSetInProgress:
			inProgressPeriods = append(inProgressPeriods, period)
		default:
			closedPeriods = append(closedPeriods, period)
		}
	}

	// Compute per-period scores and build flat periods list
	var totalIssues int
	var totalCategorized int
	aggDist := reconciler.ActivityDistribution{}

	for _, period := range teamData.Periods {
		ps := computePeriodScore(period)
		ts.Periods = append(ts.Periods, ps)
		totalIssues += period.TotalCount
		totalCategorized += period.CategorizedCount
		mergeDistribution(&aggDist, &period.Distribution)
	}

	// Build sections
	if len(closedPeriods) > 0 {
		ts.Sections = append(ts.Sections, computeSectionScore("closed", "Past (Closed)", closedPeriods))
	}
	if len(inProgressPeriods) > 0 {
		ts.Sections = append(ts.Sections, computeSectionScore("in_progress", "Current (In Progress)", inProgressPeriods))
	}

	ts.Distribution = aggDist
	ts.Score = computeScore(totalIssues, totalCategorized, aggDist)

	return ts
}

// ComputePillarScore computes the pillar score as a weighted average of team scores.
func ComputePillarScore(pillar config.Pillar, org config.Organization, teamScores []TeamScore) PillarScore {
	ps := PillarScore{
		Name:       pillar.Name,
		Identifier: pillar.Identifier,
		Path:       org.Identifier + "/" + pillar.Identifier,
		Teams:      teamScores,
	}
	ps.Score = computeWeightedAverage(teamScores)
	return ps
}

// ComputeOrgScore computes the org score as a weighted average of pillar scores.
func ComputeOrgScore(org config.Organization, pillarScores []PillarScore) OrganizationScore {
	os := OrganizationScore{
		Name:       org.Name,
		Identifier: org.Identifier,
		Path:       org.Identifier,
		Pillars:    pillarScores,
	}

	// Weighted average of pillars
	var weightedTotal float64
	var weightedCatRate float64
	var weightedDistAlign float64
	var totalWeight int

	for _, p := range pillarScores {
		if p.Score.Total == nil {
			continue
		}
		weight := p.Score.IssueCount
		weightedTotal += *p.Score.Total * float64(weight)
		weightedCatRate += *p.Score.CategorizationRate * float64(weight)
		weightedDistAlign += *p.Score.DistributionAlignment * float64(weight)
		totalWeight += weight
	}

	if totalWeight > 0 {
		total := weightedTotal / float64(totalWeight)
		catRate := weightedCatRate / float64(totalWeight)
		distAlign := weightedDistAlign / float64(totalWeight)
		os.Score = Score{
			Total:                 &total,
			Grade:                 assignGrade(&total),
			CategorizationRate:    &catRate,
			DistributionAlignment: &distAlign,
			IssueCount:            totalWeight,
		}
	} else {
		os.Score = nilScore()
	}

	return os
}

// ComputeFullScorecard orchestrates the full scorecard computation.
// The filter parameter controls which scoring periods are included.
func ComputeFullScorecard(cfg *config.ResourceMap, store reconciler.DataStore, filter FilterOptions) FullScorecard {
	refDate, _ := time.Parse("2006-01-02", cfg.SprintReferenceDate)
	current, previous := reconciler.CalculateSprintBoundaries(refDate, cfg.SprintDurationDays, time.Now())

	fs := FullScorecard{
		GeneratedAt:        time.Now(),
		SprintDurationDays: cfg.SprintDurationDays,
		CurrentSprint:      current,
		PreviousSprint:     previous,
		ActiveFilters:      buildActiveFilters(filter),
	}

	allTeamData := store.GetAllTeamData()

	for _, org := range cfg.Organizations {
		var pillarScores []PillarScore
		for _, pillar := range org.Pillars {
			var teamScores []TeamScore
			for _, team := range pillar.Teams {
				td, ok := allTeamData[team.Identifier]
				if !ok {
					td = &reconciler.TeamData{TeamIdentifier: team.Identifier}
				}
				// Apply period filters without mutating the store
				filtered := &reconciler.TeamData{
					TeamIdentifier: td.TeamIdentifier,
					Periods:        filterPeriods(td.Periods, filter),
					ReconciledAt:   td.ReconciledAt,
				}
				ts := ComputeTeamScore(team, org, pillar, filtered)
				teamScores = append(teamScores, ts)
			}
			ps := ComputePillarScore(pillar, org, teamScores)
			pillarScores = append(pillarScores, ps)
		}
		orgScore := ComputeOrgScore(org, pillarScores)
		fs.Organizations = append(fs.Organizations, orgScore)
	}

	return fs
}

// filterPeriods returns a new slice of periods matching the given filter criteria.
// Date range filtering applies only to closed periods; in-progress periods are
// included/excluded solely by the IssueStatus filter.
func filterPeriods(periods []reconciler.ScoringPeriod, filter FilterOptions) []reconciler.ScoringPeriod {
	if filter.StartDate == nil && filter.EndDate == nil && filter.IssueStatus == "" {
		return periods
	}

	var result []reconciler.ScoringPeriod
	for _, p := range periods {
		// Status filter
		if filter.IssueStatus != "" && string(p.SetType) != filter.IssueStatus {
			continue
		}

		// Date range filter (only applies to closed periods)
		if p.SetType == reconciler.IssueSetClosed {
			if filter.StartDate != nil && p.Window.Until.Before(*filter.StartDate) {
				continue
			}
			if filter.EndDate != nil && p.Window.Since.After(*filter.EndDate) {
				continue
			}
		}

		result = append(result, p)
	}
	return result
}

// buildActiveFilters constructs the ActiveFilters response field from filter options.
// Returns nil if no filters are set.
func buildActiveFilters(filter FilterOptions) *ActiveFilters {
	if filter.StartDate == nil && filter.EndDate == nil && filter.IssueStatus == "" {
		return nil
	}
	af := &ActiveFilters{}
	if filter.StartDate != nil {
		s := filter.StartDate.Format("2006-01-02")
		af.StartDate = &s
	}
	if filter.EndDate != nil {
		s := filter.EndDate.Format("2006-01-02")
		af.EndDate = &s
	}
	if filter.IssueStatus != "" {
		af.IssueStatus = &filter.IssueStatus
	}
	return af
}

func computeSectionScore(setType, label string, periods []reconciler.ScoringPeriod) SectionScore {
	section := SectionScore{
		Type:  setType,
		Label: label,
	}

	var totalIssues, totalCategorized int
	aggDist := reconciler.ActivityDistribution{}

	for _, period := range periods {
		ps := computePeriodScore(period)
		section.Periods = append(section.Periods, ps)
		totalIssues += period.TotalCount
		totalCategorized += period.CategorizedCount
		mergeDistribution(&aggDist, &period.Distribution)
	}

	section.Distribution = aggDist
	section.Score = computeScore(totalIssues, totalCategorized, aggDist)
	return section
}

func computePeriodScore(period reconciler.ScoringPeriod) PeriodScore {
	return PeriodScore{
		Label:        period.Label,
		Window:       period.Window,
		Current:      period.Current,
		SetType:      string(period.SetType),
		Score:        computeScore(period.TotalCount, period.CategorizedCount, period.Distribution),
		Distribution: period.Distribution,
	}
}

func computeScore(totalCount, categorizedCount int, dist reconciler.ActivityDistribution) Score {
	if totalCount == 0 {
		return nilScore()
	}

	catRate := (float64(categorizedCount) / float64(totalCount)) * 70.0
	distAlign := computeDistributionAlignment(categorizedCount, dist)

	total := catRate + distAlign
	return Score{
		Total:                 &total,
		Grade:                 assignGrade(&total),
		CategorizationRate:    &catRate,
		DistributionAlignment: &distAlign,
		IssueCount:            totalCount,
	}
}

func computeDistributionAlignment(categorizedCount int, dist reconciler.ActivityDistribution) float64 {
	// Exclude Associate Wellness issues from the distribution alignment denominator.
	// AW issues are still counted in categorization rate (dimension 1) but do not
	// affect distribution alignment scoring.
	scoredCount := categorizedCount - dist.AssociateWellness
	if scoredCount <= 0 {
		return 0
	}

	catCounts := map[string]int{
		CategoryIncidentsSupport:     dist.IncidentsSupport,
		CategorySecurityCompliance:   dist.SecurityCompliance,
		CategoryQualityStability:     dist.QualityStability,
		CategoryFutureSustainability: dist.FutureSustainability,
		CategoryProductPortfolio:     dist.ProductPortfolio,
	}

	var totalPenalty float64
	for rank, category := range ScoredCategoriesInOrder {
		actualPct := float64(catCounts[category]) / float64(scoredCount)
		targetPct := DefaultTargetDistribution[category]
		deviation := actualPct - targetPct

		overWeight := overAllocationWeight(rank + 1)
		underWeight := underAllocationWeight(rank + 1)

		if deviation > 0 {
			totalPenalty += deviation * overWeight
		} else {
			totalPenalty += math.Abs(deviation) * underWeight
		}
	}

	alignmentPct := math.Max(0, 1-totalPenalty)
	return alignmentPct * 30.0
}

// overAllocationWeight returns the penalty weight for over-allocation at a given priority rank.
// Formula: over_weight(rank) = 0.5 + (rank - 1) * 0.25
// Step size is 0.25 to span the full [0.5, 1.5] range across 5 scored categories.
func overAllocationWeight(rank int) float64 {
	return 0.5 + float64(rank-1)*0.25
}

// underAllocationWeight returns the penalty weight for under-allocation at a given priority rank.
// Formula: under_weight(rank) = 1.5 - (rank - 1) * 0.25
func underAllocationWeight(rank int) float64 {
	return 1.5 - float64(rank-1)*0.25
}

func assignGrade(total *float64) string {
	if total == nil {
		return "-"
	}
	switch {
	case *total >= 90:
		return "A"
	case *total >= 75:
		return "B"
	case *total >= 60:
		return "C"
	case *total >= 45:
		return "D"
	default:
		return "F"
	}
}

func nilScore() Score {
	return Score{
		Total:                 nil,
		Grade:                 "-",
		CategorizationRate:    nil,
		DistributionAlignment: nil,
		IssueCount:            0,
	}
}

func computeWeightedAverage(teamScores []TeamScore) Score {
	var weightedTotal float64
	var weightedCatRate float64
	var weightedDistAlign float64
	var totalWeight int

	for _, ts := range teamScores {
		if ts.Score.Total == nil {
			continue
		}
		weight := ts.Score.IssueCount
		weightedTotal += *ts.Score.Total * float64(weight)
		weightedCatRate += *ts.Score.CategorizationRate * float64(weight)
		weightedDistAlign += *ts.Score.DistributionAlignment * float64(weight)
		totalWeight += weight
	}

	if totalWeight == 0 {
		return nilScore()
	}

	total := weightedTotal / float64(totalWeight)
	catRate := weightedCatRate / float64(totalWeight)
	distAlign := weightedDistAlign / float64(totalWeight)

	return Score{
		Total:                 &total,
		Grade:                 assignGrade(&total),
		CategorizationRate:    &catRate,
		DistributionAlignment: &distAlign,
		IssueCount:            totalWeight,
	}
}

func mergeDistribution(dst, src *reconciler.ActivityDistribution) {
	dst.AssociateWellness += src.AssociateWellness
	dst.IncidentsSupport += src.IncidentsSupport
	dst.SecurityCompliance += src.SecurityCompliance
	dst.QualityStability += src.QualityStability
	dst.FutureSustainability += src.FutureSustainability
	dst.ProductPortfolio += src.ProductPortfolio
	dst.Uncategorized += src.Uncategorized
}
