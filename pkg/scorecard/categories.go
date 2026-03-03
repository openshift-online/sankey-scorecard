package scorecard

// Sankey category constants representing the six activity type categories.
const (
	CategoryAssociateWellness    = "Associate Wellness & Development"
	CategoryIncidentsSupport     = "Incidents & Support"
	CategorySecurityCompliance   = "Security & Compliance"
	CategoryQualityStability     = "Quality / Stability / Reliability"
	CategoryFutureSustainability = "Future Sustainability"
	CategoryProductPortfolio     = "Product / Portfolio Work"
)

// CategoriesInOrder lists all six categories in priority order (highest to lowest).
// Used for display/reference purposes.
var CategoriesInOrder = []string{
	CategoryAssociateWellness,
	CategoryIncidentsSupport,
	CategorySecurityCompliance,
	CategoryQualityStability,
	CategoryFutureSustainability,
	CategoryProductPortfolio,
}

// ScoredCategoriesInOrder lists only the five categories used in distribution
// alignment scoring. Associate Wellness is excluded because the category is not
// actively used in practice, so 0% allocation should not penalize teams.
var ScoredCategoriesInOrder = []string{
	CategoryIncidentsSupport,
	CategorySecurityCompliance,
	CategoryQualityStability,
	CategoryFutureSustainability,
	CategoryProductPortfolio,
}

// DefaultTargetDistribution is the target percentage for each scored category.
// Associate Wellness is excluded from distribution scoring; its 12% share is
// redistributed proportionally among the remaining five categories (original / 0.88).
var DefaultTargetDistribution = map[string]float64{
	CategoryIncidentsSupport:     12.0 / 88.0, // ~13.6%
	CategorySecurityCompliance:   12.0 / 88.0, // ~13.6%
	CategoryQualityStability:     22.0 / 88.0, // 25.0%
	CategoryFutureSustainability: 21.0 / 88.0, // ~23.9%
	CategoryProductPortfolio:     21.0 / 88.0, // ~23.9%
}

// activityTypeMapping maps Jira Activity Type field values to Sankey categories.
var activityTypeMapping = map[string]string{
	"Associate Wellness & Development":  CategoryAssociateWellness,
	"Incidents & Escalations":           CategoryIncidentsSupport,
	"Customer Support":                  CategoryIncidentsSupport,
	"Security & Compliance":             CategorySecurityCompliance,
	"Tech Debt":                         CategoryQualityStability,
	"Defect":                            CategoryQualityStability,
	"QE Activities":                     CategoryQualityStability,
	"Quality / Stability / Reliability": CategoryQualityStability,
	"Future Sustainability":             CategoryFutureSustainability,
	"Product / Portfolio Work":          CategoryProductPortfolio,
	"New Feature":                       CategoryProductPortfolio,
	"Feature Enhancement":               CategoryProductPortfolio,
}

// MapActivityType maps a Jira Activity Type value to a Sankey category name.
// Returns empty string for uncategorized/unknown values.
func MapActivityType(activityType string) string {
	if activityType == "" {
		return ""
	}
	cat, ok := activityTypeMapping[activityType]
	if !ok {
		return ""
	}
	return cat
}
