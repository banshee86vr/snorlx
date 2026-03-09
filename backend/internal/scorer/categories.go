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

// Tier thresholds (Bronze from 0; no "none" tier)
const (
	TierGoldMinScore   = 90
	TierSilverMinScore = 70
)

// Tier names (bronze is the minimum; no "none")
const (
	TierGold   = "gold"
	TierSilver = "silver"
	TierBronze = "bronze"
)

// ComputeTier returns the tier from overall score and whether critical/silver checks failed.
// Bronze starts at 0 so every repository gets at least bronze.
func ComputeTier(overallScore float64, hasCriticalFailure, hasHighRiskFailure bool) string {
	if overallScore >= TierGoldMinScore && !hasCriticalFailure {
		return TierGold
	}
	if overallScore >= TierSilverMinScore && !hasHighRiskFailure {
		return TierSilver
	}
	return TierBronze
}
