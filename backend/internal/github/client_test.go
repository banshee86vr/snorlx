package github

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"testing"

	"snorlx/backend/internal/config"

	ghLib "github.com/google/go-github/v60/github"
)

func newTestClient(webhookSecret string) *Client {
	cfg := &config.Config{
		GitHubClientID:      "test-id",
		GitHubClientSecret:  "test-secret",
		GitHubWebhookSecret: webhookSecret,
	}
	client, _ := NewClient(cfg)
	return client
}

func computeSignature(secret, payload []byte) string {
	mac := hmac.New(sha256.New, secret)
	mac.Write(payload)
	return "sha256=" + hex.EncodeToString(mac.Sum(nil))
}

// ===== ValidateWebhookSignature =====

func TestValidateWebhookSignature_Valid(t *testing.T) {
	client := newTestClient("my-webhook-secret")
	payload := []byte(`{"action":"completed"}`)
	sig := computeSignature([]byte("my-webhook-secret"), payload)

	if !client.ValidateWebhookSignature(payload, sig) {
		t.Error("expected valid signature to return true")
	}
}

func TestValidateWebhookSignature_InvalidSignature(t *testing.T) {
	client := newTestClient("my-webhook-secret")
	payload := []byte(`{"action":"completed"}`)

	if client.ValidateWebhookSignature(payload, "sha256=badbadbadbad") {
		t.Error("expected invalid signature to return false")
	}
}

func TestValidateWebhookSignature_WrongSecret(t *testing.T) {
	client := newTestClient("correct-secret")
	payload := []byte(`{"action":"completed"}`)
	sig := computeSignature([]byte("wrong-secret"), payload)

	if client.ValidateWebhookSignature(payload, sig) {
		t.Error("expected wrong secret signature to return false")
	}
}

func TestValidateWebhookSignature_MissingPrefix(t *testing.T) {
	client := newTestClient("my-secret")
	payload := []byte(`{"action":"completed"}`)
	mac := hmac.New(sha256.New, []byte("my-secret"))
	mac.Write(payload)
	sigWithoutPrefix := hex.EncodeToString(mac.Sum(nil))

	if client.ValidateWebhookSignature(payload, sigWithoutPrefix) {
		t.Error("expected signature without 'sha256=' prefix to return false")
	}
}

func TestValidateWebhookSignature_EmptySecret(t *testing.T) {
	client := newTestClient("") // no secret configured
	payload := []byte(`{"action":"completed"}`)

	if client.ValidateWebhookSignature(payload, "sha256=anything") {
		t.Error("expected empty webhook secret to return false (reject all webhooks)")
	}
}

func TestValidateWebhookSignature_InvalidHex(t *testing.T) {
	client := newTestClient("my-secret")
	payload := []byte(`{"action":"completed"}`)

	if client.ValidateWebhookSignature(payload, "sha256=not-valid-hex-ZZZ") {
		t.Error("expected invalid hex in signature to return false")
	}
}

// ===== ParseWebhookEvent =====

func TestParseWebhookEvent_WorkflowRun(t *testing.T) {
	client := newTestClient("secret")
	payload := []byte(`{
		"action": "completed",
		"workflow_run": {
			"id": 12345,
			"status": "completed",
			"conclusion": "success"
		}
	}`)

	event, err := client.ParseWebhookEvent("workflow_run", payload)
	if err != nil {
		t.Fatalf("ParseWebhookEvent workflow_run failed: %v", err)
	}

	wfEvent, ok := event.(*ghLib.WorkflowRunEvent)
	if !ok {
		t.Fatalf("expected *github.WorkflowRunEvent, got %T", event)
	}
	if wfEvent.GetAction() != "completed" {
		t.Errorf("expected action completed, got %q", wfEvent.GetAction())
	}
}

func TestParseWebhookEvent_WorkflowJob(t *testing.T) {
	client := newTestClient("secret")
	payload := []byte(`{
		"action": "completed",
		"workflow_job": {
			"id": 99,
			"name": "build",
			"status": "completed"
		}
	}`)

	event, err := client.ParseWebhookEvent("workflow_job", payload)
	if err != nil {
		t.Fatalf("ParseWebhookEvent workflow_job failed: %v", err)
	}

	_, ok := event.(*ghLib.WorkflowJobEvent)
	if !ok {
		t.Fatalf("expected *github.WorkflowJobEvent, got %T", event)
	}
}

func TestParseWebhookEvent_Deployment(t *testing.T) {
	client := newTestClient("secret")
	payload := []byte(`{
		"action": "created",
		"deployment": {
			"id": 55,
			"environment": "production"
		}
	}`)

	event, err := client.ParseWebhookEvent("deployment", payload)
	if err != nil {
		t.Fatalf("ParseWebhookEvent deployment failed: %v", err)
	}

	_, ok := event.(*ghLib.DeploymentEvent)
	if !ok {
		t.Fatalf("expected *github.DeploymentEvent, got %T", event)
	}
}

func TestParseWebhookEvent_DeploymentStatus(t *testing.T) {
	client := newTestClient("secret")
	payload := []byte(`{
		"action": "created",
		"deployment_status": {
			"id": 77,
			"state": "success"
		}
	}`)

	event, err := client.ParseWebhookEvent("deployment_status", payload)
	if err != nil {
		t.Fatalf("ParseWebhookEvent deployment_status failed: %v", err)
	}

	_, ok := event.(*ghLib.DeploymentStatusEvent)
	if !ok {
		t.Fatalf("expected *github.DeploymentStatusEvent, got %T", event)
	}
}

func TestParseWebhookEvent_UnsupportedType(t *testing.T) {
	client := newTestClient("secret")
	payload := []byte(`{}`)

	_, err := client.ParseWebhookEvent("push", payload)
	if err == nil {
		t.Error("expected error for unsupported event type")
	}
}

func TestParseWebhookEvent_InvalidJSON(t *testing.T) {
	client := newTestClient("secret")

	_, err := client.ParseWebhookEvent("workflow_run", []byte(`{invalid json}`))
	if err == nil {
		t.Error("expected error for invalid JSON payload")
	}
}

// ===== isRetryableError =====

func TestIsRetryableError_Nil(t *testing.T) {
	if isRetryableError(nil) {
		t.Error("nil error should not be retryable")
	}
}

func TestIsRetryableError_ContextCanceled(t *testing.T) {
	if isRetryableError(context.Canceled) {
		t.Error("context.Canceled should not be retryable")
	}
}

func TestIsRetryableError_ContextDeadlineExceeded(t *testing.T) {
	if isRetryableError(context.DeadlineExceeded) {
		t.Error("context.DeadlineExceeded should not be retryable")
	}
}

func TestIsRetryableError_MessageContains502(t *testing.T) {
	err := errors.New("HTTP 502 bad gateway")
	if !isRetryableError(err) {
		t.Error("error with '502' in message should be retryable")
	}
}

func TestIsRetryableError_MessageContains503(t *testing.T) {
	err := errors.New("503 service unavailable")
	if !isRetryableError(err) {
		t.Error("error with '503' in message should be retryable")
	}
}

func TestIsRetryableError_MessageContains504(t *testing.T) {
	err := errors.New("504 gateway timeout")
	if !isRetryableError(err) {
		t.Error("error with '504' in message should be retryable")
	}
}

func TestIsRetryableError_ConnectionReset(t *testing.T) {
	err := errors.New("connection reset by peer")
	if !isRetryableError(err) {
		t.Error("connection reset error should be retryable")
	}
}

func TestIsRetryableError_Timeout(t *testing.T) {
	err := errors.New("request timeout")
	if !isRetryableError(err) {
		t.Error("timeout error should be retryable")
	}
}

func TestIsRetryableError_GitHubAPI_BadGateway(t *testing.T) {
	resp := &http.Response{StatusCode: http.StatusBadGateway}
	ghErr := &ghLib.ErrorResponse{
		Response: resp,
		Message:  "bad gateway",
	}
	if !isRetryableError(ghErr) {
		t.Error("GitHub 502 error should be retryable")
	}
}

func TestIsRetryableError_GitHubAPI_ServiceUnavailable(t *testing.T) {
	resp := &http.Response{StatusCode: http.StatusServiceUnavailable}
	ghErr := &ghLib.ErrorResponse{
		Response: resp,
		Message:  "service unavailable",
	}
	if !isRetryableError(ghErr) {
		t.Error("GitHub 503 error should be retryable")
	}
}

func TestIsRetryableError_GitHubAPI_TooManyRequests(t *testing.T) {
	resp := &http.Response{StatusCode: http.StatusTooManyRequests}
	ghErr := &ghLib.ErrorResponse{
		Response: resp,
		Message:  "too many requests",
	}
	if !isRetryableError(ghErr) {
		t.Error("GitHub 429 error should be retryable")
	}
}

func TestIsRetryableError_GitHubAPI_NotFound(t *testing.T) {
	resp := &http.Response{StatusCode: http.StatusNotFound}
	ghErr := &ghLib.ErrorResponse{
		Response: resp,
		Message:  "not found",
	}
	if isRetryableError(ghErr) {
		t.Error("GitHub 404 error should NOT be retryable")
	}
}

func TestIsRetryableError_OrdinaryError(t *testing.T) {
	err := errors.New("some unrelated error")
	if isRetryableError(err) {
		t.Error("ordinary error should not be retryable")
	}
}

// ===== NewClient =====

func TestNewClient_CreatesClient(t *testing.T) {
	cfg := &config.Config{
		GitHubClientID:     "test-id",
		GitHubClientSecret: "test-secret",
	}
	client, err := NewClient(cfg)
	if err != nil {
		t.Fatalf("NewClient failed: %v", err)
	}
	if client == nil {
		t.Error("expected non-nil client")
	}
}

func TestGetAuthURL_ContainsClientID(t *testing.T) {
	client := newTestClient("")
	url := client.GetAuthURL("random-state")
	if url == "" {
		t.Error("expected non-empty auth URL")
	}
	// The URL should contain the state parameter
	if !contains(url, "state=random-state") {
		t.Errorf("expected auth URL to contain state parameter, got: %s", url)
	}
}

// ===== ParseWebhookEvent round-trip =====

func TestParseWebhookEvent_WorkflowRun_FieldsPreserved(t *testing.T) {
	client := newTestClient("secret")

	runID := int64(99887)
	payload, _ := json.Marshal(map[string]interface{}{
		"action": "requested",
		"workflow_run": map[string]interface{}{
			"id":         runID,
			"status":     "in_progress",
			"conclusion": nil,
			"name":       "CI Pipeline",
		},
	})

	event, err := client.ParseWebhookEvent("workflow_run", payload)
	if err != nil {
		t.Fatalf("ParseWebhookEvent failed: %v", err)
	}

	wfEvent := event.(*ghLib.WorkflowRunEvent)
	if wfEvent.GetWorkflowRun() == nil {
		t.Fatal("expected non-nil WorkflowRun in event")
	}
	if wfEvent.GetWorkflowRun().GetID() != runID {
		t.Errorf("expected run ID %d, got %d", runID, wfEvent.GetWorkflowRun().GetID())
	}
}

// helper: string contains substring
func contains(s, sub string) bool {
	return len(s) >= len(sub) && (s == sub || len(sub) == 0 ||
		func() bool {
			for i := 0; i <= len(s)-len(sub); i++ {
				if s[i:i+len(sub)] == sub {
					return true
				}
			}
			return false
		}())
}

// ===== retryWithBackoff =====

func TestRetryWithBackoff_SucceedsFirstTry(t *testing.T) {
	calls := 0
	result, err := retryWithBackoff(context.Background(), "test-op", func() (string, error) {
		calls++
		return "ok", nil
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result != "ok" {
		t.Errorf("expected ok, got %q", result)
	}
	if calls != 1 {
		t.Errorf("expected 1 call, got %d", calls)
	}
}

func TestRetryWithBackoff_NonRetryableError_NoRetry(t *testing.T) {
	calls := 0
	_, err := retryWithBackoff(context.Background(), "test-op", func() (string, error) {
		calls++
		return "", errors.New("some permanent error")
	})
	if err == nil {
		t.Fatal("expected error")
	}
	if calls != 1 {
		t.Errorf("expected 1 call for non-retryable error, got %d", calls)
	}
}

func TestRetryWithBackoff_ContextCanceled(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // cancel immediately

	_, err := retryWithBackoff(ctx, "test-op", func() (string, error) {
		return "", fmt.Errorf("502 gateway error")
	})
	if err == nil {
		t.Fatal("expected error for cancelled context")
	}
}
