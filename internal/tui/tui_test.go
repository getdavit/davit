package tui

import (
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
)

// fakeManager is a minimal stub that satisfies the parts of *app.Manager used by the TUI.
// We can't easily instantiate a real Manager without a DB, so we test the model logic
// with nil dependencies where possible.

func TestNewModel(t *testing.T) {
	m := NewModel(nil, nil, "http://localhost:2019")
	if m == nil {
		t.Fatal("NewModel returned nil")
	}
	if m.screen != screenSetupWizard {
		t.Errorf("expected initial screen screenSetupWizard, got %d", m.screen)
	}
}

func TestModelInit(t *testing.T) {
	m := NewModel(nil, nil, "http://localhost:2019")
	cmds := m.Init()
	if cmds == nil {
		t.Error("Init() returned nil commands")
	}
}

func TestModelViewLoading(t *testing.T) {
	m := NewModel(nil, nil, "http://localhost:2019")
	view := m.View()
	// Before ready, should show loading
	if view != "\n  Loading..." {
		t.Errorf("expected loading view, got: %q", view)
	}
}

func TestModelWindowSize(t *testing.T) {
	m := NewModel(nil, nil, "http://localhost:2019")
	msg := tea.WindowSizeMsg{Width: 80, Height: 24}
	result, cmd := m.Update(msg)
	if cmd != nil {
		t.Error("expected nil cmd from WindowSizeMsg")
	}
	updated := result.(*Model)
	if !updated.ready {
		t.Error("expected model to be ready after WindowSizeMsg")
	}
	if updated.width != 80 {
		t.Errorf("expected width 80, got %d", updated.width)
	}
}

func TestModelDashboardView(t *testing.T) {
	m := NewModel(nil, nil, "http://localhost:2019")
	// Set up as ready with dashboard screen
	_, _ = m.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	_, _ = m.Update(healthLoadedMsg{
		health: serverHealth{
			provisioned:   true,
			hostname:      "test-server",
			davitVersion:  "0.4.0",
			uptime:        "5d 2h 30m",
			dockerRunning: true,
			caddyRunning:  true,
			appsTotal:     2,
			appsRunning:   2,
		},
	})
	m.screen = screenDashboard

	view := m.View()
	if view == "" {
		t.Fatal("expected non-empty dashboard view")
	}

	// Check key elements are present
	expectedStrings := []string{
		"davit", "test-server", "0.4.0", "APPS", "SERVER HEALTH", "Docker", "Caddy",
	}

	for _, s := range expectedStrings {
		if !contains(view, s) {
			t.Errorf("expected dashboard to contain %q", s)
		}
	}
}

func TestModelAppList(t *testing.T) {
	m := NewModel(nil, nil, "http://localhost:2019")
	_, _ = m.Update(tea.WindowSizeMsg{Width: 80, Height: 24})

	// Simulate loading apps
	_, _ = m.Update(appsLoadedMsg{
		apps: []appSummaryItem{
			{name: "myapi", status: "running", domain: "api.example.com", branch: "main", commit: "abc1234", url: "https://api.example.com"},
			{name: "frontend", status: "running", domain: "example.com", branch: "main", commit: "def5678", url: "https://example.com"},
			{name: "worker", status: "stopped", domain: "", branch: "develop", commit: "ghi9012", url: ""},
		},
	})
	m.screen = screenDashboard

	view := m.View()
	expectedApps := []string{"myapi", "frontend", "worker", "api.example.com", "example.com", "abc1234"}
	for _, name := range expectedApps {
		if !contains(view, name) {
			t.Errorf("expected app list to contain %q", name)
		}
	}
}

func TestCursorNavigation(t *testing.T) {
	m := NewModel(nil, nil, "http://localhost:2019")
	_, _ = m.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	_, _ = m.Update(appsLoadedMsg{
		apps: []appSummaryItem{
			{name: "app-a", status: "running", domain: "a.example.com"},
			{name: "app-b", status: "running", domain: "b.example.com"},
			{name: "app-c", status: "running", domain: "c.example.com"},
		},
	})
	m.screen = screenDashboard

	if m.cursor != 0 {
		t.Errorf("expected initial cursor at 0, got %d", m.cursor)
	}

	// Move down twice
	_, _ = m.Update(tea.KeyMsg{Type: tea.KeyDown})
	if m.cursor != 1 {
		t.Errorf("expected cursor at 1 after Down, got %d", m.cursor)
	}

	_, _ = m.Update(tea.KeyMsg{Type: tea.KeyDown})
	if m.cursor != 2 {
		t.Errorf("expected cursor at 2 after Down, got %d", m.cursor)
	}

	// Move up once
	_, _ = m.Update(tea.KeyMsg{Type: tea.KeyUp})
	if m.cursor != 1 {
		t.Errorf("expected cursor at 1 after Up, got %d", m.cursor)
	}

	// Try 'j' and 'k' key aliases
	_, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'j'}})
	if m.cursor != 2 {
		t.Errorf("expected cursor at 2 after 'j', got %d", m.cursor)
	}

	_, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'k'}})
	if m.cursor != 1 {
		t.Errorf("expected cursor at 1 after 'k', got %d", m.cursor)
	}
}

func TestBoundsNavigation(t *testing.T) {
	m := NewModel(nil, nil, "http://localhost:2019")
	_, _ = m.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	_, _ = m.Update(appsLoadedMsg{
		apps: []appSummaryItem{
			{name: "app-a", status: "running", domain: "a.example.com"},
		},
	})
	m.screen = screenDashboard

	// Try to go below the list
	_, _ = m.Update(tea.KeyMsg{Type: tea.KeyDown})
	if m.cursor != 0 {
		t.Errorf("expected cursor stays at 0, got %d", m.cursor)
	}

	// Try to go above the list
	_, _ = m.Update(tea.KeyMsg{Type: tea.KeyUp})
	if m.cursor != 0 {
		t.Errorf("expected cursor stays at 0, got %d", m.cursor)
	}
}

func TestEnterAppDetail(t *testing.T) {
	m := NewModel(nil, nil, "http://localhost:2019")
	_, _ = m.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	_, _ = m.Update(appsLoadedMsg{
		apps: []appSummaryItem{
			{name: "myapp", status: "running", domain: "myapp.example.com", branch: "main", commit: "abc123"},
		},
	})
	m.screen = screenDashboard

	// Press Enter
	_, _ = m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if m.screen != screenAppDetail {
		t.Errorf("expected screenAppDetail after Enter, got %d", m.screen)
	}
	if m.selectedApp == nil {
		t.Fatal("expected selectedApp to be set")
	}
	if m.selectedApp.name != "myapp" {
		t.Errorf("expected selected app 'myapp', got %q", m.selectedApp.name)
	}
}

func TestBackFromAppDetail(t *testing.T) {
	m := NewModel(nil, nil, "http://localhost:2019")
	_, _ = m.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	_, _ = m.Update(appsLoadedMsg{
		apps: []appSummaryItem{
			{name: "myapp", status: "running", domain: "myapp.example.com"},
		},
	})
	m.screen = screenDashboard
	m.selectedApp = &appSummaryItem{name: "myapp", status: "running", domain: "myapp.example.com"}
	m.screen = screenAppDetail

	// Press Escape
	_, _ = m.Update(tea.KeyMsg{Type: tea.KeyEsc})
	if m.screen != screenDashboard {
		t.Errorf("expected screenDashboard after Esc, got %d", m.screen)
	}
	if m.selectedApp != nil {
		t.Error("expected selectedApp to be nil after back")
	}
}

func TestQuit(t *testing.T) {
	m := NewModel(nil, nil, "http://localhost:2019")
	_, _ = m.Update(tea.WindowSizeMsg{Width: 80, Height: 24})

	result, _ := m.Update(tea.KeyMsg{Type: tea.KeyCtrlC})
	updated := result.(*Model)
	if updated.screen != screenQuit {
		t.Errorf("expected screenQuit after Ctrl+C, got %d", updated.screen)
	}
}

func TestHelpToggle(t *testing.T) {
	m := NewModel(nil, nil, "http://localhost:2019")
	_, _ = m.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	m.screen = screenDashboard

	// Open help
	_, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'?'}})
	view := m.View()
	if !contains(view, "Navigation") || !contains(view, "new") {
		t.Errorf("expected help content with Navigation and key bindings, got: %s", view[:min(len(view), 200)])
	}
}

func TestSetupWizardSteps(t *testing.T) {
	m := NewModel(nil, nil, "http://localhost:2019")
	_, _ = m.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	// No health loaded → stays in setup wizard
	m.screen = screenSetupWizard

	// Step 0: Welcome (check before advancing)
	view := m.View()
	if !contains(view, "Welcome to Davit") {
		t.Errorf("expected wizard welcome, got: %s", view[:min(len(view), 200)])
	}

	// Advance through all steps
	stepChecks := []struct {
		key      tea.KeyType
		expected string
	}{
		{tea.KeyEnter, "Server Settings"},
		{tea.KeyEnter, "Hardening Steps"},
		{tea.KeyEnter, "Provisioning"},
		{tea.KeyEnter, "Provisioning Complete"},
	}

	for i, sc := range stepChecks {
		_, _ = m.Update(tea.KeyMsg{Type: sc.key})
		view := m.View()
		if !contains(view, sc.expected) {
			t.Errorf("step %d: expected content %q not found", i+1, sc.expected)
		}
	}

	// After last step, Enter should go to dashboard
	_, _ = m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if m.screen != screenDashboard {
		t.Errorf("expected dashboard after wizard completes, got screen %d", m.screen)
	}
}

func TestProvisionedAutoDashboard(t *testing.T) {
	m := NewModel(nil, nil, "http://localhost:2019")
	_, _ = m.Update(tea.WindowSizeMsg{Width: 80, Height: 24})

	// Receive health message showing provisioned
	_, _ = m.Update(healthLoadedMsg{
		health: serverHealth{
			provisioned: true,
			hostname:    "prod",
		},
	})

	if m.screen != screenDashboard {
		t.Errorf("expected dashboard when provisioned=true, got screen %d", m.screen)
	}
}

func TestEmptyAppList(t *testing.T) {
	m := NewModel(nil, nil, "http://localhost:2019")
	_, _ = m.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	_, _ = m.Update(healthLoadedMsg{
		health: serverHealth{provisioned: true},
	})
	_, _ = m.Update(appsLoadedMsg{
		apps: []appSummaryItem{},
	})
	m.screen = screenDashboard

	view := m.View()
	if !contains(view, "No applications deployed") {
		t.Errorf("expected empty state message, got: %s", view[:min(len(view), 200)])
	}
}

func TestRefreshKey(t *testing.T) {
	m := NewModel(nil, nil, "http://localhost:2019")
	_, _ = m.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	_, _ = m.Update(healthLoadedMsg{
		health: serverHealth{provisioned: true},
	})
	m.screen = screenDashboard

	_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'r'}})
	if cmd == nil {
		t.Error("expected non-nil cmd from refresh key")
	}
}

func TestAppDetailRendering(t *testing.T) {
	m := NewModel(nil, nil, "http://localhost:2019")
	_, _ = m.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	m.selectedApp = &appSummaryItem{
		name:   "myapp",
		status: "running",
		domain: "myapp.example.com",
		branch: "main",
		commit: "abc1234",
	}
	m.screen = screenAppDetail

	view := m.View()
	if !contains(view, "myapp") || !contains(view, "myapp.example.com") {
		t.Errorf("expected app detail content, got: %s", view[:min(len(view), 200)])
	}
}

func TestStatusIcons(t *testing.T) {
	tests := []struct {
		status string
		want   string
	}{
		{"running", "●"},
		{"stopped", "○"},
		{"created", "○"},
		{"unknown", "●"},
	}

	for _, tt := range tests {
		icon := StatusIcon(tt.status)
		if icon == "" {
			t.Errorf("StatusIcon(%q) returned empty string", tt.status)
		}
	}
}

func TestTruncateSHA(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"abc1234567890", "abc1234"},
		{"abc1234", "abc1234"},
		{"abc", "abc"},
	}

	for _, tt := range tests {
		got := truncateSHA(tt.input)
		if got != tt.want {
			t.Errorf("truncateSHA(%q) = %q, want %q", tt.input, got, tt.want)
		}
	}
}

func TestFormatDuration(t *testing.T) {
	tests := []struct {
		input time.Duration
		want  string
	}{
		{30 * time.Minute, "30m"},
		{2 * time.Hour, "2h 0m"},
		{5*24*time.Hour + 2*time.Hour + 30*time.Minute, "5d 2h 30m"},
		{1 * time.Hour, "1h 0m"},
	}

	for _, tt := range tests {
		got := formatDuration(tt.input)
		if got != tt.want {
			t.Errorf("formatDuration(%v) = %q, want %q", tt.input, got, tt.want)
		}
	}
}

func TestHealthIcon(t *testing.T) {
	if healthIcon(true) != "✓" {
		t.Error("expected checkmark for healthIcon(true)")
	}
	if healthIcon(false) != "✗" {
		t.Error("expected X for healthIcon(false)")
	}
}

func TestNewAppWizard(t *testing.T) {
	m := NewModel(nil, nil, "http://localhost:2019")
	_, _ = m.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	_, _ = m.Update(healthLoadedMsg{
		health: serverHealth{provisioned: true},
	})
	_, _ = m.Update(appsLoadedMsg{
		apps: []appSummaryItem{
			{name: "myapp", status: "running", domain: "myapp.example.com"},
		},
	})
	m.screen = screenDashboard

	// Press 'n' to start new app wizard
	_, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'n'}})
	if m.screen != screenNewApp {
		t.Fatalf("expected screenNewApp after 'n', got %d", m.screen)
	}
	if m.newAppStep != 0 {
		t.Errorf("expected newAppStep 0, got %d", m.newAppStep)
	}
	if !m.newAppNameInput.Focused() {
		t.Error("expected name input to be focused on step 0")
	}

	// Step 0: Enter app name
	m.newAppNameInput.SetValue("test-app")
	_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if cmd != nil {
		t.Error("expected nil cmd after entering name")
	}
	if m.newAppStep != 1 {
		t.Errorf("expected newAppStep 1 after name entered, got %d", m.newAppStep)
	}
	if !m.newAppRepoInput.Focused() {
		t.Error("expected repo input to be focused on step 1")
	}

	// Step 0: empty name should show error
	m.newAppStep = 0
	m.newAppRepoInput.Blur()
	m.newAppNameInput.SetValue("")
	m.newAppNameInput.Focus()
	_, _ = m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if m.newAppError != "App name is required" {
		t.Errorf("expected 'App name is required', got %q", m.newAppError)
	}

	// Esc from step 1 goes back to dashboard
	m.screen = screenNewApp
	m.newAppStep = 1
	_, _ = m.Update(tea.KeyMsg{Type: tea.KeyEsc})
	if m.screen != screenDashboard {
		t.Errorf("expected dashboard after Esc from wizard, got %d", m.screen)
	}
}

func TestRemoveConfirm(t *testing.T) {
	m := NewModel(nil, nil, "http://localhost:2019")
	_, _ = m.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	_, _ = m.Update(healthLoadedMsg{
		health: serverHealth{provisioned: true},
	})
	_, _ = m.Update(appsLoadedMsg{
		apps: []appSummaryItem{
			{name: "myapp", status: "running", domain: "myapp.example.com"},
		},
	})
	m.screen = screenDashboard

	// Enter app detail
	_, _ = m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if m.screen != screenAppDetail {
		t.Fatalf("expected app detail, got screen %d", m.screen)
	}

	// Press '!' to remove
	_, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'!'}})
	if m.screen != screenRemoveConfirm {
		t.Fatalf("expected screenRemoveConfirm after '!', got %d", m.screen)
	}

	// 'n' should cancel
	_, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'n'}})
	if m.screen != screenAppDetail {
		t.Errorf("expected back to app detail after 'n' cancel, got %d", m.screen)
	}

	// Go back and test 'esc' cancel too
	m.screen = screenAppDetail
	_, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'!'}})
	_, _ = m.Update(tea.KeyMsg{Type: tea.KeyEsc})
	if m.screen != screenAppDetail {
		t.Errorf("expected back to app detail after Esc cancel, got %d", m.screen)
	}

	// 'y' should confirm (triggers a command, but mgr is nil so it will produce an error)
	m.screen = screenAppDetail
	m.selectedApp = &appSummaryItem{name: "myapp", status: "running"}
	_, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'!'}})
	_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'y'}})
	if cmd == nil {
		t.Error("expected non-nil cmd after 'y' confirm removal")
	}
	if !m.removeConfirmed {
		t.Error("expected removeConfirmed=true after 'y'")
	}
}

func TestEnvVarsScreen(t *testing.T) {
	m := NewModel(nil, nil, "http://localhost:2019")
	_, _ = m.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	_, _ = m.Update(healthLoadedMsg{
		health: serverHealth{provisioned: true},
	})
	_, _ = m.Update(appsLoadedMsg{
		apps: []appSummaryItem{
			{name: "myapp", status: "running", domain: "myapp.example.com"},
		},
	})
	m.screen = screenDashboard
	_, _ = m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m.selectedApp = &appSummaryItem{name: "myapp", status: "running"}

	// Press 'e' for env vars
	_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'e'}})
	if m.screen != screenEnvVars {
		t.Fatalf("expected screenEnvVars after 'e', got %d", m.screen)
	}
	// Should trigger a command (calls loadEnvCmd which calls mgr.EnvList)
	if cmd == nil {
		t.Error("expected non-nil cmd after 'e' (load env vars)")
	}

	// Esc should return to app detail
	_, _ = m.Update(tea.KeyMsg{Type: tea.KeyEsc})
	if m.screen != screenAppDetail {
		t.Errorf("expected back to app detail after Esc from env, got %d", m.screen)
	}
}

func TestLogsScreen(t *testing.T) {
	m := NewModel(nil, nil, "http://localhost:2019")
	_, _ = m.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	_, _ = m.Update(healthLoadedMsg{
		health: serverHealth{provisioned: true},
	})
	_, _ = m.Update(appsLoadedMsg{
		apps: []appSummaryItem{
			{name: "myapp", status: "running", domain: "myapp.example.com"},
		},
	})
	m.screen = screenDashboard
	_, _ = m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m.selectedApp = &appSummaryItem{name: "myapp", status: "running"}

	// Press 'l' for logs
	_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'l'}})
	if m.screen != screenLogs {
		t.Fatalf("expected screenLogs after 'l', got %d", m.screen)
	}
	if cmd == nil {
		t.Error("expected non-nil cmd after 'l' (load logs)")
	}

	// Esc should return to app detail
	_, _ = m.Update(tea.KeyMsg{Type: tea.KeyEsc})
	if m.screen != screenAppDetail {
		t.Errorf("expected back to app detail after Esc from logs, got %d", m.screen)
	}

	// q should also return
	m.screen = screenLogs
	_, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'q'}})
	if m.screen != screenAppDetail {
		t.Errorf("expected back to app detail after 'q' from logs, got %d", m.screen)
	}
}

func TestFilterKey(t *testing.T) {
	m := NewModel(nil, nil, "http://localhost:2019")
	_, _ = m.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	_, _ = m.Update(healthLoadedMsg{
		health: serverHealth{provisioned: true},
	})
	_, _ = m.Update(appsLoadedMsg{
		apps: []appSummaryItem{
			{name: "alpha-app", status: "running", domain: "alpha.example.com"},
			{name: "beta-svc", status: "stopped", domain: "beta.example.com"},
			{name: "gamma-api", status: "running", domain: "gamma.example.com"},
		},
	})
	m.screen = screenDashboard

	// Press '/' for filter
	_, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'/'}})
	if m.screen != screenFilter {
		t.Fatalf("expected screenFilter after '/', got %d", m.screen)
	}
	if !m.filterInput.Focused() {
		t.Error("expected filter input to be focused")
	}

	// Set filter value and press Enter
	m.filterInput.SetValue("alpha")
	_, _ = m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if m.screen != screenDashboard {
		t.Errorf("expected back to dashboard after filter, got %d", m.screen)
	}
	if m.filterQuery != "alpha" {
		t.Errorf("expected filter query 'alpha', got %q", m.filterQuery)
	}
	// filteredApps should only contain alpha-app
	if len(m.filteredApps) != 1 || m.filteredApps[0].name != "alpha-app" {
		t.Errorf("expected 1 filtered app 'alpha-app', got %d apps", len(m.filteredApps))
	}

	// Clear filter by emptying and pressing Enter
	m.screen = screenFilter
	m.filterInput.SetValue("")
	_, _ = m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if m.filterQuery != "" {
		t.Errorf("expected empty filter query, got %q", m.filterQuery)
	}
	if len(m.filteredApps) != 3 {
		t.Errorf("expected all 3 apps unfiltered, got %d", len(m.filteredApps))
	}

	// Esc should cancel without applying
	m.filterQuery = "test"
	_, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'/'}})
	m.filterInput.SetValue("cancelled")
	_, _ = m.Update(tea.KeyMsg{Type: tea.KeyEsc})
	if m.filterQuery != "test" {
		t.Errorf("expected filter query unchanged after Esc, got %q", m.filterQuery)
	}
}

func TestRestartKeyUppercase(t *testing.T) {
	m := NewModel(nil, nil, "http://localhost:2019")
	_, _ = m.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	_, _ = m.Update(healthLoadedMsg{
		health: serverHealth{provisioned: true},
	})
	_, _ = m.Update(appsLoadedMsg{
		apps: []appSummaryItem{
			{name: "myapp", status: "running", domain: "myapp.example.com"},
		},
	})
	m.screen = screenDashboard
	_, _ = m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m.selectedApp = &appSummaryItem{name: "myapp", status: "running"}

	// 'r' on app detail should trigger Refresh (not Restart)
	_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'r'}})
	if cmd == nil {
		t.Error("expected non-nil cmd from 'r' on app detail (Refresh)")
	}

	// 'R' on app detail should trigger Restart (uppercase)
	_, cmd2 := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'R'}})
	if cmd2 == nil {
		t.Error("expected non-nil cmd from 'R' on app detail (Restart)")
	}
}

// contains is a helper to check if a string contains a substring.
func contains(s, substr string) bool {
	return len(s) >= len(substr) && searchString(s, substr)
}

// searchString is a simple substring search for testing.
func searchString(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}

// min returns the smaller of a and b.
func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}