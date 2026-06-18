package watch

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"strings"

	"github.com/getdavit/davit/internal/app"
	"github.com/getdavit/davit/internal/state"
)

// WebhookServer handles incoming webhook requests from GitHub, GitLab, and
// generic Git providers.
type WebhookServer struct {
	db      *state.DB
	manager *app.Manager
	mux     *http.ServeMux
}

// NewWebhookServer creates a new WebhookServer.
func NewWebhookServer(db *state.DB, manager *app.Manager) *WebhookServer {
	srv := &WebhookServer{
		db:      db,
		manager: manager,
		mux:     http.NewServeMux(),
	}
	srv.mux.HandleFunc("/.davit/webhook/", srv.handleWebhook)
	srv.mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"status":"ok"}`))
	})
	return srv
}

// Start starts the webhook HTTP server on the given address.
// Blocks until ctx is cancelled, then shuts down gracefully.
func (ws *WebhookServer) Start(ctx context.Context, addr string) error {
	srv := &http.Server{
		Addr:    addr,
		Handler: ws.mux,
	}

	errCh := make(chan error, 1)
	go func() {
		log.Printf("Webhook server listening on %s\n", addr)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			errCh <- err
		}
	}()

	select {
	case <-ctx.Done():
		log.Println("Shutting down webhook server...")
		return srv.Close()
	case err := <-errCh:
		return err
	}
}

// handleWebhook routes incoming webhook requests to the correct handler.
// URL format: /.davit/webhook/<app-name>/<token>
func (ws *WebhookServer) handleWebhook(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Parse path: /.davit/webhook/<app-name>/<token>
	parts := strings.Split(strings.TrimPrefix(r.URL.Path, "/.davit/webhook/"), "/")
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
		http.Error(w, "Invalid webhook URL", http.StatusBadRequest)
		return
	}
	appName, token := parts[0], parts[1]

	// Read body
	body, err := io.ReadAll(r.Body)
	if err != nil {
		http.Error(w, "Failed to read request body", http.StatusBadRequest)
		return
	}
	defer r.Body.Close()

	// Validate and process
	ws.processWebhook(w, r, appName, token, body)
}

// processWebhook validates the webhook token, optionally verifies HMAC
// signature, parses the payload, and triggers a deploy.
func (ws *WebhookServer) processWebhook(w http.ResponseWriter, r *http.Request, appName, token string, body []byte) {
	// Look up the watcher
	watcher, err := ws.db.GetWatcher(appName)
	if err != nil {
		log.Printf("Webhook DB error for %s: %v\n", appName, err)
		http.Error(w, "Internal error", http.StatusInternalServerError)
		return
	}
	if watcher == nil || watcher.Status != "active" {
		http.Error(w, "Not found or inactive", http.StatusNotFound)
		return
	}

	// Validate token
	if watcher.WebhookToken != token {
		http.Error(w, "Forbidden", http.StatusForbidden)
		return
	}

	// Optional HMAC signature validation
	if watcher.WebhookSecret != "" {
		sigHeader := r.Header.Get("X-Hub-Signature-256")
		if sigHeader != "" {
			if !validateHMACSignature(body, watcher.WebhookSecret, sigHeader) {
				log.Printf("Webhook HMAC validation failed for %s\n", appName)
				http.Error(w, "Invalid signature", http.StatusForbidden)
				return
			}
		}
	}

	// Parse the payload
	payload, err := parseWebhookPayload(r.Header, body)
	if err != nil {
		log.Printf("Webhook payload parse error for %s: %v\n", appName, err)
		http.Error(w, "Invalid payload: "+err.Error(), http.StatusBadRequest)
		return
	}

	// Verify the push is for the correct branch
	appRecord, err := ws.db.GetApp(appName)
	if err != nil || appRecord.Name == "" {
		log.Printf("Webhook app lookup error for %s: %v\n", appName, err)
		http.Error(w, "App not found", http.StatusNotFound)
		return
	}

	expectedRef := "refs/heads/" + appRecord.Branch
	if payload.Ref != expectedRef {
		log.Printf("Webhook [%s]: ignoring push to %s (expecting %s)\n", appName, payload.Ref, expectedRef)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"status":"ignored","reason":"wrong_branch"}`))
		return
	}

	// Update last checked/commit in DB
	_ = ws.db.UpdateWatcherCommit(appName, payload.After)

	// Respond 200 OK immediately, deploy async
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte(`{"status":"ok"}`))

	// Deploy asynchronously
	go func() {
		log.Printf("Webhook deploy triggered for %s (commit: %s)\n", appName, payload.After)
		result, err := ws.manager.Deploy(app.DeployOptions{
			AppName: appName,
			Timeout: 120,
			Force:   true,
		})
		if err != nil {
			log.Printf("Webhook deploy error [%s]: %v\n", appName, err)
		} else {
			log.Printf("Webhook deploy success [%s]: commit=%s duration=%dms\n",
				appName, result.CommitSHA, result.Duration)
		}
	}()
}

// validateHMACSignature verifies the X-Hub-Signature-256 header.
func validateHMACSignature(body []byte, secret, signatureHeader string) bool {
	// Expected: sha256=<hex>
	if !strings.HasPrefix(signatureHeader, "sha256=") {
		return false
	}
	expected := strings.TrimPrefix(signatureHeader, "sha256=")

	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write(body)
	computed := hex.EncodeToString(mac.Sum(nil))

	return hmac.Equal([]byte(computed), []byte(expected))
}

// WebhookPayload is the parsed payload from a webhook push event.
type WebhookPayload struct {
	Ref   string `json:"ref"`
	After string `json:"after"`
}

// parseWebhookPayload parses a webhook request body into a standard payload,
// supporting GitHub, GitLab, and generic JSON formats.
func parseWebhookPayload(headers http.Header, body []byte) (*WebhookPayload, error) {
	event := headers.Get("X-GitHub-Event")
	if event == "" {
		event = headers.Get("X-Gitlab-Event")
	}

	switch {
	case event == "push" || strings.HasPrefix(event, "Push Hook"):
		// GitHub push or GitLab Push Hook
		return parseGitHubOrGitLabPayload(body)
	default:
		// Try generic JSON format
		return parseGenericPayload(body)
	}
}

// parseGitHubOrGitLabPayload parses a GitHub push event or GitLab Push Hook payload.
// Both have a similar top-level structure.
func parseGitHubOrGitLabPayload(body []byte) (*WebhookPayload, error) {
	var raw struct {
		Ref   string `json:"ref"`
		After string `json:"after"`
		Check string `json:"checkout_sha"` // GitLab uses checkout_sha
	}
	if err := json.Unmarshal(body, &raw); err != nil {
		return nil, fmt.Errorf("invalid JSON: %w", err)
	}

	payload := &WebhookPayload{
		Ref: raw.Ref,
	}

	// GitLab uses checkout_sha; GitHub uses after
	if raw.Check != "" {
		payload.After = raw.Check
	} else {
		payload.After = raw.After
	}

	if payload.Ref == "" {
		return nil, fmt.Errorf("missing 'ref' field in payload")
	}
	if payload.After == "" {
		return nil, fmt.Errorf("missing commit SHA ('after' or 'checkout_sha') in payload")
	}

	return payload, nil
}

// parseGenericPayload attempts to parse a generic webhook payload with ref and after fields.
func parseGenericPayload(body []byte) (*WebhookPayload, error) {
	var payload WebhookPayload
	if err := json.Unmarshal(body, &payload); err != nil {
		return nil, fmt.Errorf("invalid JSON: %w", err)
	}
	if payload.Ref == "" {
		return nil, fmt.Errorf("missing 'ref' field")
	}
	if payload.After == "" {
		return nil, fmt.Errorf("missing 'after' field")
	}
	return &payload, nil
}