package scorer

// Category weights (must sum to 100 for percentage)
const (
	WeightSecurity      = 25
	WeightTesting       = 20
	WeightCICD          = 15
	WeightDocumentation = 15
	WeightCodeQuality   = 10
	WeightMaintenance   = 10
	WeightCommunity     = 5
)

// Tier thresholds (Bronze at 30 so repos with basic README + CI achieve it without strict security)
const (
	TierGoldMinScore   = 90
	TierSilverMinScore = 70
	TierBronzeMinScore = 30
)

// Tier names
const (
	TierGold   = "gold"
	TierSilver = "silver"
	TierBronze = "bronze"
	TierNone   = "none"
)

// ComputeTier returns the tier from overall score and whether critical/silver checks failed.
func ComputeTier(overallScore float64, hasCriticalFailure, hasHighRiskFailure bool) string {
	if overallScore >= TierGoldMinScore && !hasCriticalFailure {
		return TierGold
	}
	if overallScore >= TierSilverMinScore && !hasHighRiskFailure {
		return TierSilver
	}
	if overallScore >= TierBronzeMinScore {
		return TierBronze
	}
	return TierNone
}
