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
	if !contains(view, "Navigation") || !contains(view, "Actions") {
		t.Errorf("expected help content, got: %s", view[:min(len(view), 200)])
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