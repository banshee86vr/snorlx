package scorer

import (
	"context"
	"strings"
	"time"

	"snorlx/backend/internal/github"
	"snorlx/backend/internal/models"

	gh "github.com/google/go-github/v72/github"
)

// RepoMeta is optional metadata from the sync loop (from ghRepo) to avoid extra API calls.
type RepoMeta struct {
	PushedAt      *time.Time
	Archived      bool
	WorkflowNames []string
	HasWorkflows  bool
	Topics        []string
}

// Scorer computes repository scores from GitHub API data.
type Scorer struct {
	ghClient *github.Client
}

// New creates a new Scorer.
func New(ghClient *github.Client) *Scorer {
	return &Scorer{ghClient: ghClient}
}

// ScoreRepository fetches repo data from the GitHub API and returns a RepositoryScore.
func (s *Scorer) ScoreRepository(ctx context.Context, client *gh.Client, owner, repoName string, savedRepo *models.Repository, meta *RepoMeta) (*models.RepositoryScore, error) {
	results := make(models.JSONMap)
	now := time.Now()

	// Fetch all data (gracefully handle 403/404)
	community, _ := s.ghClient.GetCommunityProfile(ctx, client, owner, repoName)
	protection, _ := s.ghClient.GetBranchProtection(ctx, client, owner, repoName, savedRepo.DefaultBranch)
	rootContents, _ := s.ghClient.ListRepositoryContents(ctx, client, owner, repoName, "", "")
	githubContents, _ := s.ghClient.ListRepositoryContents(ctx, client, owner, repoName, ".github", "")
	dependabotEnabled := s.ghClient.VulnerabilityAlertsEnabled(ctx, client, owner, repoName)
	codeScanningCfg, _ := s.ghClient.CodeScanningDefaultSetup(ctx, client, owner, repoName)

	rootNames := contentNames(rootContents)
	githubNames := contentNames(githubContents)

	// Security
	securityPass, securityTotal := scoreSecurity(results, protection, dependabotEnabled, codeScanningCfg, community, rootNames, githubNames)

	// Testing
	testingPass, testingTotal := scoreTesting(results, rootNames, githubNames, meta)

	// CI/CD
	cicdPass, cicdTotal := scoreCICD(results, protection, meta, githubNames)

	// Documentation
	docPass, docTotal := scoreDocumentation(results, community, savedRepo.Description, rootNames)

	// Code quality
	cqPass, cqTotal := scoreCodeQuality(results, rootNames)

	// Maintenance
	maintPass, maintTotal := scoreMaintenance(results, meta, savedRepo.Description)

	// Community
	commPass, commTotal := scoreCommunity(results, community, rootNames, githubNames, meta)

	// Category scores (0-100)
	securityScore := percent(securityPass, securityTotal)
	testingScore := percent(testingPass, testingTotal)
	cicdScore := percent(cicdPass, cicdTotal)
	docScore := percent(docPass, docTotal)
	cqScore := percent(cqPass, cqTotal)
	maintScore := percent(maintPass, maintTotal)
	commScore := percent(commPass, commTotal)

	overall := (float64(securityScore)*WeightSecurity + float64(testingScore)*WeightTesting +
		float64(cicdScore)*WeightCICD + float64(docScore)*WeightDocumentation +
		float64(cqScore)*WeightCodeQuality + float64(maintScore)*WeightMaintenance +
		float64(commScore)*WeightCommunity) / 100.0

	hasCritical := hasCriticalFailure(results)
	hasHighRisk := hasHighRiskFailure(results)
	tier := ComputeTier(overall, hasCritical, hasHighRisk)

	return &models.RepositoryScore{
		RepoID:              savedRepo.ID,
		OverallScore:        round(overall, 2),
		Tier:                tier,
		SecurityScore:       round(float64(securityScore), 2),
		TestingScore:        round(float64(testingScore), 2),
		CICDScore:           round(float64(cicdScore), 2),
		DocumentationScore:  round(float64(docScore), 2),
		CodeQualityScore:    round(float64(cqScore), 2),
		MaintenanceScore:    round(float64(maintScore), 2),
		CommunityScore:      round(float64(commScore), 2),
		CheckResults:       results,
		ScannedAt:          now,
	}, nil
}

func contentNames(contents []*gh.RepositoryContent) []string {
	if contents == nil {
		return nil
	}
	names := make([]string, 0, len(contents))
	for _, c := range contents {
		if c.Name != nil {
			names = append(names, *c.Name)
		}
	}
	return names
}

func percent(pass, total int) float64 {
	if total == 0 {
		return 100
	}
	return float64(pass) / float64(total) * 100
}

func round(v float64, decimals int) float64 {
	mult := 1.0
	for i := 0; i < decimals; i++ {
		mult *= 10
	}
	return float64(int(v*mult+0.5)) / mult
}

func scoreSecurity(results models.JSONMap, protection *gh.Protection, dependabot bool, codeScanning *gh.DefaultSetupConfiguration, community *gh.CommunityHealthMetrics, root, githubDir []string) (pass, total int) {
	// Branch protection (only count when repo has protection; otherwise we'd penalize 5/8 for every repo without it)
	if protection != nil {
		total += 5
		set(results, CheckBranchProtection, true)
		pass++
		if protection.RequiredPullRequestReviews != nil && protection.RequiredPullRequestReviews.RequiredApprovingReviewCount >= 1 {
			set(results, CheckRequiresReviews, true)
			pass++
		} else {
			set(results, CheckRequiresReviews, false)
		}
		if protection.RequiredStatusChecks != nil {
			set(results, CheckRequiresStatusChecks, true)
			pass++
		} else {
			set(results, CheckRequiresStatusChecks, false)
		}
		if protection.EnforceAdmins != nil && protection.EnforceAdmins.Enabled {
			set(results, CheckEnforceAdmins, true)
			pass++
		} else {
			set(results, CheckEnforceAdmins, false)
		}
		if protection.AllowForcePushes == nil || !protection.AllowForcePushes.Enabled {
			set(results, CheckNoForcePushes, true)
			pass++
		} else {
			set(results, CheckNoForcePushes, false)
		}
	} else {
		set(results, CheckBranchProtection, false)
		set(results, CheckRequiresReviews, false)
		set(results, CheckRequiresStatusChecks, false)
		set(results, CheckEnforceAdmins, false)
		set(results, CheckNoForcePushes, false)
	}
	// Always-counted security checks (repos without branch protection still get 0–3 here)
	total += 3
	set(results, CheckDependabotEnabled, dependabot)
	if dependabot {
		pass++
	}
	codeScanEnabled := codeScanning != nil
	set(results, CheckCodeScanningEnabled, codeScanEnabled)
	if codeScanEnabled {
		pass++
	}
	secMD := HasFile(root, "SECURITY.md") || HasFile(githubDir, "SECURITY.md")
	set(results, CheckSecurityMD, secMD)
	if secMD {
		pass++
	}
	return pass, total
}

func scoreTesting(results models.JSONMap, root, githubDir []string, meta *RepoMeta) (pass, total int) {
	total = 3
	hasTestDir := HasAnyOf(root, TestDirs)
	set(results, CheckTestDirectory, hasTestDir)
	if hasTestDir {
		pass++
	}
	testConfigs := []string{"jest.config.js", "jest.config.ts", "jest.config.mjs", "pytest.ini", "vitest.config.ts", "vitest.config.js", "karma.conf.js"}
	hasTestConfig := HasAnyOf(root, testConfigs)
	set(results, CheckTestConfig, hasTestConfig)
	if hasTestConfig {
		pass++
	}
	hasTestWorkflow := false
	if meta != nil {
		for _, n := range meta.WorkflowNames {
			if stringsContainsAny(n, "test", "ci", "build") {
				hasTestWorkflow = true
				break
			}
		}
	}
	set(results, CheckCIWorkflowTests, hasTestWorkflow)
	if hasTestWorkflow {
		pass++
	}
	return pass, total
}

func stringsContainsAny(s string, sub ...string) bool {
	for _, x := range sub {
		if strings.Contains(strings.ToLower(s), x) {
			return true
		}
	}
	return false
}

func scoreCICD(results models.JSONMap, protection *gh.Protection, meta *RepoMeta, githubDir []string) (pass, total int) {
	total = 2
	hasWorkflows := meta != nil && meta.HasWorkflows
	set(results, CheckHasWorkflows, hasWorkflows)
	if hasWorkflows {
		pass++
	}
	statusReq := protection != nil && protection.RequiredStatusChecks != nil
	set(results, CheckStatusChecksReq, statusReq)
	if statusReq {
		pass++
	}
	return pass, total
}

func scoreDocumentation(results models.JSONMap, community *gh.CommunityHealthMetrics, description *string, root []string) (pass, total int) {
	total = 6
	readmeExists := false
	if community != nil && community.Files != nil && community.Files.Readme != nil {
		readmeExists = true
	}
	set(results, CheckReadmeExists, readmeExists)
	if readmeExists {
		pass++
	}
	set(results, CheckReadmeSize, readmeExists) // simplified: same as exists for now
	if readmeExists {
		pass++
	}
	licenseExists := community != nil && community.Files != nil && community.Files.License != nil
	set(results, CheckLicenseExists, licenseExists)
	if licenseExists {
		pass++
	}
	contributing := community != nil && community.Files != nil && community.Files.Contributing != nil
	set(results, CheckContributing, contributing)
	if contributing {
		pass++
	}
	changelog := HasFile(root, "CHANGELOG.md")
	set(results, CheckChangelog, changelog)
	if changelog {
		pass++
	}
	descSet := description != nil && *description != ""
	set(results, CheckDescriptionSet, descSet)
	if descSet {
		pass++
	}
	return pass, total
}

func scoreCodeQuality(results models.JSONMap, root []string) (pass, total int) {
	total = 5
	hasLinter := HasFilePrefix(root, ".eslintrc") || HasFilePrefix(root, ".prettierrc") || HasFile(root, ".rubocop.yml")
	set(results, CheckLinterConfig, hasLinter)
	if hasLinter {
		pass++
	}
	editorConfig := HasFile(root, ".editorconfig")
	set(results, CheckEditorConfig, editorConfig)
	if editorConfig {
		pass++
	}
	typeConfig := HasFile(root, "tsconfig.json")
	set(results, CheckTypeConfig, typeConfig)
	if typeConfig {
		pass++
	}
	dockerfile := HasFile(root, "Dockerfile")
	set(results, CheckDockerfile, dockerfile)
	if dockerfile {
		pass++
	}
	preCommit := HasFile(root, ".pre-commit-config.yaml")
	set(results, CheckPreCommit, preCommit)
	if preCommit {
		pass++
	}
	return pass, total
}

func scoreMaintenance(results models.JSONMap, meta *RepoMeta, description *string) (pass, total int) {
	total = 3
	pushed30 := false
	pushed90 := false
	if meta != nil && meta.PushedAt != nil {
		t := *meta.PushedAt
		pushed30 = time.Since(t) < 30*24*time.Hour
		pushed90 = time.Since(t) < 90*24*time.Hour
	}
	set(results, CheckPushedRecently30, pushed30)
	if pushed30 {
		pass++
	}
	set(results, CheckPushedRecently90, pushed90)
	if pushed90 {
		pass++
	}
	notArchived := meta == nil || !meta.Archived
	set(results, CheckNotArchived, notArchived)
	if notArchived {
		pass++
	}
	_ = description
	return pass, total
}

func scoreCommunity(results models.JSONMap, community *gh.CommunityHealthMetrics, root, githubDir []string, meta *RepoMeta) (pass, total int) {
	total = 5
	topicsSet := meta != nil && len(meta.Topics) > 0
	set(results, CheckTopicsSet, topicsSet)
	if topicsSet {
		pass++
	}
	issueTemplates := (community != nil && community.Files != nil && community.Files.IssueTemplate != nil) || HasFile(githubDir, "ISSUE_TEMPLATE")
	set(results, CheckIssueTemplates, issueTemplates)
	if issueTemplates {
		pass++
	}
	prTemplate := community != nil && community.Files != nil && community.Files.PullRequestTemplate != nil
	set(results, CheckPRTemplate, prTemplate)
	if prTemplate {
		pass++
	}
	codeowners := HasFile(root, "CODEOWNERS") || HasFile(githubDir, "CODEOWNERS")
	set(results, CheckCodeowners, codeowners)
	if codeowners {
		pass++
	}
	codeOfConduct := community != nil && community.Files != nil && (community.Files.CodeOfConduct != nil || community.Files.CodeOfConductFile != nil)
	set(results, CheckCodeOfConduct, codeOfConduct)
	if codeOfConduct {
		pass++
	}
	return pass, total
}

func set(m models.JSONMap, key string, v bool) {
	if m == nil {
		return
	}
	m[key] = v
}

func hasCriticalFailure(results models.JSONMap) bool {
	// Critical: no branch protection, or no license
	if v, ok := results[CheckBranchProtection].(bool); ok && !v {
		return true
	}
	if v, ok := results[CheckLicenseExists].(bool); ok && !v {
		return true
	}
	return false
}

func hasHighRiskFailure(results models.JSONMap) bool {
	// High risk: no reviews required, dependabot disabled
	if v, ok := results[CheckRequiresReviews].(bool); ok && !v {
		return true
	}
	if v, ok := results[CheckDependabotEnabled].(bool); ok && !v {
		return true
	}
	return false
}
