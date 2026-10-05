package handlers

import (
	"testing"

	"gopkg.in/yaml.v3"
)

const securityWorkflowYAML = `
name: Security Scans
on: [pull_request]
jobs:
  codeql:
    name: CodeQL Analysis
    strategy:
      matrix:
        language: [go, javascript-typescript]
  trivy-fs:
    name: Trivy Filesystem Scan
  trivy-backend-image:
    name: Trivy Backend Docker Image Scan
  trivy-frontend-image:
    name: Trivy Frontend Docker Image Scan
  dependency-review:
    name: Dependency Review
  go-security:
    name: Go Security (govulncheck + gosec)
    needs: [trivy-fs, dependency-review]
  npm-audit:
    needs: trivy-fs
`

func TestWorkflowDefinition_KeepsJobsInFileOrder(t *testing.T) {
	var def WorkflowDefinition
	if err := yaml.Unmarshal([]byte(securityWorkflowYAML), &def); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if def.Name != "Security Scans" || def.Jobs.Len() != 7 {
		t.Fatalf("parsed %q with %d jobs", def.Name, def.Jobs.Len())
	}

	want := []string{"codeql", "trivy-fs", "trivy-backend-image", "trivy-frontend-image", "dependency-review", "go-security", "npm-audit"}
	h := newTestHandler(&mockStorage{})
	for attempt := 0; attempt < 5; attempt++ {
		deps := h.extractJobDependencies(&def, "", nil)
		if len(deps) != len(want) {
			t.Fatalf("attempt %d: got %d dependencies, want %d", attempt, len(deps), len(want))
		}
		for i, dep := range deps {
			if dep.JobID != want[i] {
				t.Fatalf("attempt %d: job %d is %q, want %q (GitHub draws jobs in file order)", attempt, i, dep.JobID, want[i])
			}
		}
	}

	deps := h.extractJobDependencies(&def, "", nil)
	if !deps[0].IsMatrix || deps[0].Name != "CodeQL Analysis" {
		t.Errorf("codeql should be a matrix job named from its name field, got %+v", deps[0])
	}
	if deps[6].Name != "npm-audit" || len(deps[6].Needs) != 1 || deps[6].Needs[0] != "trivy-fs" {
		t.Errorf("a job without a name uses its id and a scalar needs becomes one entry, got %+v", deps[6])
	}
	if len(deps[5].Needs) != 2 {
		t.Errorf("list needs must be kept, got %+v", deps[5].Needs)
	}
}

func TestWorkflowDefinition_RejectsNonMappingJobs(t *testing.T) {
	var def WorkflowDefinition
	if err := yaml.Unmarshal([]byte("jobs:\n  - build\n"), &def); err == nil {
		t.Fatal("a jobs sequence is not a valid workflow and must fail to parse")
	}
}
