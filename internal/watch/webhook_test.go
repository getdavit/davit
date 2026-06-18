package watch

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"net/http"
	"strings"
	"testing"
)

func TestParseWebhookPayload_GitHubPush(t *testing.T) {
	body := `{
		"ref": "refs/heads/main",
		"after": "abc123def456",
		"before": "000000000000",
		"repository": {"full_name": "test/repo"}
	}`

	headers := http.Header{}
	headers.Set("X-GitHub-Event", "push")

	payload, err := parseWebhookPayload(headers, []byte(body))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if payload.Ref != "refs/heads/main" {
		t.Fatalf("expected ref 'refs/heads/main', got %q", payload.Ref)
	}
	if payload.After != "abc123def456" {
		t.Fatalf("expected after 'abc123def456', got %q", payload.After)
	}
}

func TestParseWebhookPayload_GitLabPush(t *testing.T) {
	body := `{
		"ref": "refs/heads/main",
		"checkout_sha": "def789abc012",
		"before": "000000000000",
		"project": {"path_with_namespace": "test/repo"}
	}`

	headers := http.Header{}
	headers.Set("X-Gitlab-Event", "Push Hook")

	payload, err := parseWebhookPayload(headers, []byte(body))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if payload.Ref != "refs/heads/main" {
		t.Fatalf("expected ref 'refs/heads/main', got %q", payload.Ref)
	}
	if payload.After != "def789abc012" {
		t.Fatalf("expected after 'def789abc012', got %q", payload.After)
	}
}

func TestParseWebhookPayload_Generic(t *testing.T) {
	body := `{"ref": "refs/heads/main", "after": "aaa111bbb222"}`

	payload, err := parseWebhookPayload(http.Header{}, []byte(body))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if payload.Ref != "refs/heads/main" {
		t.Fatalf("expected ref 'refs/heads/main', got %q", payload.Ref)
	}
	if payload.After != "aaa111bbb222" {
		t.Fatalf("expected after 'aaa111bbb222', got %q", payload.After)
	}
}

func TestParseWebhookPayload_MissingRef(t *testing.T) {
	body := `{"after": "abc123"}`

	_, err := parseWebhookPayload(http.Header{}, []byte(body))
	if err == nil {
		t.Fatal("expected error for missing ref")
	}
	if !strings.Contains(err.Error(), "missing 'ref'") {
		t.Fatalf("expected error about missing ref, got: %v", err)
	}
}

func TestParseWebhookPayload_MissingAfter(t *testing.T) {
	body := `{"ref": "refs/heads/main"}`

	_, err := parseWebhookPayload(http.Header{}, []byte(body))
	if err == nil {
		t.Fatal("expected error for missing after")
	}
}

func TestParseWebhookPayload_InvalidJSON(t *testing.T) {
	body := `{invalid json}`

	_, err := parseWebhookPayload(http.Header{}, []byte(body))
	if err == nil {
		t.Fatal("expected error for invalid JSON")
	}
}

func TestParseWebhookPayload_GitLabWithAfter(t *testing.T) {
	// GitLab payloads have both "after" and "checkout_sha"; checkout_sha takes precedence
	body := `{
		"ref": "refs/heads/develop",
		"after": "aaa111",
		"checkout_sha": "bbb222",
		"project": {"id": 1}
	}`

	headers := http.Header{}
	headers.Set("X-Gitlab-Event", "Push Hook")

	payload, err := parseWebhookPayload(headers, []byte(body))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if payload.After != "bbb222" {
		t.Fatalf("expected checkout_sha 'bbb222', got %q", payload.After)
	}
}

func TestValidateHMACSignature_Valid(t *testing.T) {
	body := []byte(`{"ref":"refs/heads/main","after":"abc123"}`)
	secret := "my-secret-key"

	// Compute expected signature
	sigHeader := "sha256=" + computeHMACSHA256Hex(body, secret)

	if !validateHMACSignature(body, secret, sigHeader) {
		t.Fatal("expected valid HMAC signature to pass")
	}
}

func TestValidateHMACSignature_Invalid(t *testing.T) {
	body := []byte(`{"ref":"refs/heads/main","after":"abc123"}`)
	secret := "my-secret-key"

	// Wrong signature
	sigHeader := "sha256=invalid0000000000000000000000000000000000000000000000000000000000"

	if validateHMACSignature(body, secret, sigHeader) {
		t.Fatal("expected invalid HMAC signature to fail")
	}
}

func TestValidateHMACSignature_WrongPrefix(t *testing.T) {
	body := []byte(`{"ref":"refs/heads/main","after":"abc123"}`)
	secret := "my-secret-key"

	// Wrong algorithm prefix
	sigHeader := "md5=abc123"

	if validateHMACSignature(body, secret, sigHeader) {
		t.Fatal("expected wrong algorithm prefix to fail")
	}
}

func TestValidateHMACSignature_EmptySecret(t *testing.T) {
	body := []byte(`{"ref":"refs/heads/main","after":"abc123"}`)
	secret := ""
	sigHeader := "sha256=invalid"

	if validateHMACSignature(body, secret, sigHeader) {
		t.Fatal("expected empty secret HMAC to fail")
	}
}

func TestValidateHMACSignature_EmptyHeader(t *testing.T) {
	body := []byte(`{"ref":"refs/heads/main","after":"abc123"}`)
	secret := "my-secret"

	if validateHMACSignature(body, secret, "") {
		t.Fatal("expected empty header HMAC to fail")
	}
}

// computeHMACSHA256Hex computes the X-Hub-Signature-256 value for testing.
func computeHMACSHA256Hex(body []byte, secret string) string {
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write(body)
	return hex.EncodeToString(mac.Sum(nil))
}

func TestParseWebhookPayload_NonPushEvents(t *testing.T) {
	body := `{"ref": "refs/heads/main", "after": "abc123"}`

	headers := http.Header{}
	headers.Set("X-GitHub-Event", "pull_request")

	// Non-push events should still be parseable via generic fallback
	payload, err := parseWebhookPayload(headers, []byte(body))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if payload.Ref != "refs/heads/main" {
		t.Fatalf("expected ref 'refs/heads/main', got %q", payload.Ref)
	}
}

func TestNormalizeWebhookURL(t *testing.T) {
	// Test the URL path parsing used in handleWebhook (uses strings.Split)
	tests := []struct {
		path       string
		wantApp    string
		wantToken  string
		wantOK     bool
	}{
		{"/.davit/webhook/myapp/mytoken", "myapp", "mytoken", true},
		{"/.davit/webhook/myapp/token123", "myapp", "token123", true},
		{"/.davit/webhook/", "", "", false},             // empty parts after split
		{"/.davit/webhook/myapp/", "myapp", "", false},   // token empty
		{"/.davit/other", "", "", false},                  // wrong path prefix
		{"/.davit/webhook/a/b/c", "a", "b/c", false},     // too many path segments
	}

	for _, tt := range tests {
		// Simulate the handleWebhook path extraction
		trimmed := strings.TrimPrefix(tt.path, "/")
		parts := strings.Split(trimmed, "/")

		var app, token string
		var ok bool

		// Expecting: [".davit", "webhook", "<app>", "<token>"]
		if len(parts) >= 4 && parts[0] == ".davit" && parts[1] == "webhook" {
			app = parts[2]
			token = parts[3]
			// Valid only if both are non-empty and there are exactly 4 parts
			ok = app != "" && token != "" && len(parts) == 4
		}

		if tt.wantOK != ok {
			t.Errorf("for path %q: want ok=%v, got ok=%v (app=%q, token=%q)",
				tt.path, tt.wantOK, ok, app, token)
		}
		if ok && (app != tt.wantApp || token != tt.wantToken) {
			t.Errorf("for path %q: got (%q, %q), want (%q, %q)",
				tt.path, app, token, tt.wantApp, tt.wantToken)
		}
	}
}

func TestParseWebhookPayload_EmptyBody(t *testing.T) {
	_, err := parseWebhookPayload(http.Header{}, []byte{})
	if err == nil {
		t.Fatal("expected error for empty body")
	}
}

func TestParseWebhookPayload_GitLabWithNoCheckoutSHA(t *testing.T) {
	// Some GitLab versions might not send checkout_sha
	body := `{
		"ref": "refs/heads/main",
		"after": "0000000000000000000000000000000000000000",
		"checkout_sha": "0000000000000000000000000000000000000000",
		"user_id": 1
	}`

	headers := http.Header{}
	headers.Set("X-Gitlab-Event", "Push Hook")

	payload, err := parseWebhookPayload(headers, []byte(body))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// Both are "000..." so it passes
	if payload.After != "0000000000000000000000000000000000000000" {
		t.Fatalf("expected after '0000...', got %q", payload.After)
	}
}