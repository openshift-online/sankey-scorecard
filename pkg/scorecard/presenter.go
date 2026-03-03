package scorecard

import (
	"encoding/json"
	"fmt"
	"strings"

	"gopkg.in/yaml.v3"
)

// ToJSON serializes a scorecard to JSON with null handling for nil score fields.
func ToJSON(v interface{}) ([]byte, error) {
	return json.MarshalIndent(v, "", "  ")
}

// ToYAML serializes a scorecard to YAML.
func ToYAML(v interface{}) ([]byte, error) {
	return yaml.Marshal(v)
}

// ToPlaintext formats a scorecard entity as a human-readable table.
// The entity can be a FullScorecard, OrganizationScore, PillarScore, or TeamScore.
func ToPlaintext(v interface{}) string {
	switch s := v.(type) {
	case FullScorecard:
		return formatFullScorecard(s)
	case OrganizationScore:
		return formatOrganization(s)
	case PillarScore:
		return formatPillar(s)
	case TeamScore:
		return formatTeam(s)
	default:
		return fmt.Sprintf("%+v", v)
	}
}

func formatFullScorecard(fs FullScorecard) string {
	var b strings.Builder
	b.WriteString(fmt.Sprintf("Full Scorecard\nGenerated: %s\n\n",
		fs.GeneratedAt.Format("2006-01-02 15:04 UTC")))

	for _, org := range fs.Organizations {
		b.WriteString(formatOrganization(org))
		b.WriteString("\n")
	}
	return b.String()
}

func formatOrganization(org OrganizationScore) string {
	var b strings.Builder
	b.WriteString(fmt.Sprintf("%s Scorecard\n", org.Name))
	b.WriteString(fmt.Sprintf("Generated: %s | Issues: %d\n\n",
		"", org.Score.IssueCount))

	b.WriteString(formatScoreSummary("Organization", org.Score))

	b.WriteString("\nPillars:\n")
	b.WriteString(fmt.Sprintf("  %-22s %5s  %5s  %6s  %8s  %10s\n",
		"PILLAR", "SCORE", "GRADE", "ISSUES", "CAT.RATE", "DIST.ALIGN"))
	for _, p := range org.Pillars {
		b.WriteString(formatScoreRow(p.Name, p.Score))
	}
	return b.String()
}

func formatPillar(ps PillarScore) string {
	var b strings.Builder
	b.WriteString(fmt.Sprintf("%s Scorecard\n", ps.Name))
	b.WriteString(fmt.Sprintf("Generated: %s | Issues: %d\n\n",
		"", ps.Score.IssueCount))

	b.WriteString(formatScoreSummary("Pillar", ps.Score))

	b.WriteString("\nTeams:\n")
	b.WriteString(fmt.Sprintf("  %-22s %5s  %5s  %6s  %8s  %10s\n",
		"TEAM", "SCORE", "GRADE", "ISSUES", "CAT.RATE", "DIST.ALIGN"))
	for _, t := range ps.Teams {
		b.WriteString(formatScoreRow(t.Identifier, t.Score))
	}
	return b.String()
}

func formatTeam(ts TeamScore) string {
	var b strings.Builder
	b.WriteString(fmt.Sprintf("%s Scorecard\n", ts.Name))
	b.WriteString(fmt.Sprintf("Generated: %s | Issues: %d\n\n",
		"", ts.Score.IssueCount))

	b.WriteString(formatScoreSummary("Team", ts.Score))

	if len(ts.Sections) > 0 {
		for _, section := range ts.Sections {
			b.WriteString(fmt.Sprintf("\n%s:\n", section.Label))
			if section.Score.Total != nil {
				b.WriteString(fmt.Sprintf("  Section Score: %.1f (%s) | Issues: %d\n",
					*section.Score.Total, section.Score.Grade, section.Score.IssueCount))
			} else {
				b.WriteString(fmt.Sprintf("  Section Score: - (no data) | Issues: %d\n",
					section.Score.IssueCount))
			}

			b.WriteString(fmt.Sprintf("  %-34s %5s  %5s  %6s  %8s  %10s\n",
				"CADENCE", "SCORE", "GRADE", "ISSUES", "CAT.RATE", "DIST.ALIGN"))
			for _, p := range section.Periods {
				label := p.Label
				if p.Current {
					label += " *"
				}
				b.WriteString(fmt.Sprintf("  %-34s", label))
				if p.Score.Total != nil {
					b.WriteString(fmt.Sprintf(" %5.1f  %5s  %6d  %8.1f  %10.1f\n",
						*p.Score.Total, p.Score.Grade, p.Score.IssueCount,
						*p.Score.CategorizationRate, *p.Score.DistributionAlignment))
				} else {
					b.WriteString(fmt.Sprintf(" %5s  %5s  %6d  %8s  %10s\n",
						"-", "-", p.Score.IssueCount, "-", "-"))
				}
			}
		}
		b.WriteString("\n  * = current\n")
	}

	b.WriteString("\nActivity Distribution (aggregate):\n")
	catCount := ts.Distribution.AssociateWellness + ts.Distribution.IncidentsSupport +
		ts.Distribution.SecurityCompliance + ts.Distribution.QualityStability +
		ts.Distribution.FutureSustainability + ts.Distribution.ProductPortfolio
	if catCount > 0 {
		b.WriteString(formatDistRowExcluded("Associate Wellness", ts.Distribution.AssociateWellness, catCount))
		b.WriteString(formatDistRow("Incidents & Support", ts.Distribution.IncidentsSupport, catCount, 12.0/88.0))
		b.WriteString(formatDistRow("Security & Compliance", ts.Distribution.SecurityCompliance, catCount, 12.0/88.0))
		b.WriteString(formatDistRow("Quality / Stability", ts.Distribution.QualityStability, catCount, 22.0/88.0))
		b.WriteString(formatDistRow("Future Sustainability", ts.Distribution.FutureSustainability, catCount, 21.0/88.0))
		b.WriteString(formatDistRow("Product / Portfolio", ts.Distribution.ProductPortfolio, catCount, 21.0/88.0))
	}
	b.WriteString(fmt.Sprintf("  Uncategorized:          %3d\n", ts.Distribution.Uncategorized))

	return b.String()
}

func formatScoreSummary(level string, s Score) string {
	var b strings.Builder
	if s.Total != nil {
		b.WriteString(fmt.Sprintf("%s Score: %.1f (%s)\n", level, *s.Total, s.Grade))
		b.WriteString(fmt.Sprintf("  Categorization Rate:      %.1f / 70\n", *s.CategorizationRate))
		b.WriteString(fmt.Sprintf("  Distribution Alignment:   %.1f / 30\n", *s.DistributionAlignment))
	} else {
		b.WriteString(fmt.Sprintf("%s Score: - (no data)\n", level))
	}
	return b.String()
}

func formatScoreRow(name string, s Score) string {
	if s.Total != nil {
		return fmt.Sprintf("  %-22s %5.1f  %5s  %6d  %8.1f  %10.1f\n",
			name, *s.Total, s.Grade, s.IssueCount,
			*s.CategorizationRate, *s.DistributionAlignment)
	}
	return fmt.Sprintf("  %-22s %5s  %5s  %6d  %8s  %10s\n",
		name, "-", "-", s.IssueCount, "-", "-")
}

func formatDistRow(label string, count, totalCat int, target float64) string {
	pct := float64(count) / float64(totalCat) * 100.0
	return fmt.Sprintf("  %-22s %3d  (%5.1f%%)  target: %.0f%%\n",
		label+":", count, pct, target*100)
}

func formatDistRowExcluded(label string, count, totalCat int) string {
	pct := float64(count) / float64(totalCat) * 100.0
	return fmt.Sprintf("  %-22s %3d  (%5.1f%%)  target: N/A (excluded from scoring)\n",
		label+":", count, pct)
}
