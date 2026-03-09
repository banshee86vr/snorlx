package scorer

import (
	"strings"
)

// Check IDs used in check_results map
const (
	// Security
	CheckBranchProtection      = "branch_protection"
	CheckRequiresReviews       = "requires_reviews"
	CheckRequiresStatusChecks   = "requires_status_checks"
	CheckEnforceAdmins         = "enforce_admins"
	CheckNoForcePushes         = "no_force_pushes"
	CheckDependabotEnabled     = "dependabot_enabled"
	CheckCodeScanningEnabled   = "code_scanning_enabled"
	CheckSecurityMD            = "security_md"
	// Testing
	CheckTestDirectory     = "test_directory"
	CheckTestConfig        = "test_config"
	CheckCIWorkflowTests   = "ci_workflow_tests"
	// CI/CD
	CheckHasWorkflows     = "has_workflows"
	CheckStatusChecksReq  = "status_checks_required"
	// Documentation
	CheckReadmeExists     = "readme_exists"
	CheckReadmeSize       = "readme_size"
	CheckLicenseExists    = "license_exists"
	CheckContributing     = "contributing"
	CheckChangelog        = "changelog"
	CheckDescriptionSet   = "description_set"
	// Code quality
	CheckLinterConfig     = "linter_config"
	CheckEditorConfig     = "editorconfig"
	CheckTypeConfig       = "type_config"
	CheckDockerfile       = "dockerfile"
	CheckPreCommit        = "pre_commit"
	// Maintenance
	CheckPushedRecently30 = "pushed_recently_30"
	CheckPushedRecently90 = "pushed_recently_90"
	CheckNotArchived      = "not_archived"
	// Community
	CheckTopicsSet        = "topics_set"
	CheckIssueTemplates   = "issue_templates"
	CheckPRTemplate      = "pr_template"
	CheckCodeowners       = "codeowners"
	CheckCodeOfConduct    = "code_of_conduct"
)

// File names we look for in root and .github
var (
	RootFiles = []string{
		"README.md", "LICENSE", "CHANGELOG.md", ".editorconfig", "Dockerfile",
		".eslintrc", ".eslintrc.js", ".eslintrc.cjs", ".eslintrc.json", ".eslintrc.yaml",
		".prettierrc", ".prettierrc.js", ".prettierrc.json", ".prettierrc.yaml",
		".rubocop.yml", "tsconfig.json", ".pre-commit-config.yaml",
		"CODEOWNERS", ".github/CODEOWNERS",
		"jest.config.js", "jest.config.ts", "jest.config.mjs", "pytest.ini", "vitest.config.ts", "vitest.config.js", "karma.conf.js",
	}
	TestDirs = []string{"test", "tests", "spec", "__tests__"}
)

// HasFile checks if a file name is in the directory listing (supports nested .github/CODEOWNERS via githubContents).
func HasFile(dirContents []string, name string) bool {
	for _, n := range dirContents {
		if n == name {
			return true
		}
	}
	return false
}

// HasFilePrefix checks if any listed name has the given prefix (e.g. "README" for README.md).
func HasFilePrefix(dirContents []string, prefix string) bool {
	for _, n := range dirContents {
		if strings.HasPrefix(n, prefix) {
			return true
		}
	}
	return false
}

// HasAnyOf checks if the directory contains any of the given names.
func HasAnyOf(dirContents []string, names []string) bool {
	for _, name := range names {
		if HasFile(dirContents, name) {
			return true
		}
	}
	return false
}
