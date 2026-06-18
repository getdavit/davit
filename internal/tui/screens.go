package tui

import (
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/lipgloss"
)

// renderAppDetail renders the app detail screen.
func (m *Model) renderAppDetail() string {
	if m.selectedApp == nil {
		m.screen = screenDashboard
		return m.renderDashboard()
	}

	content := m.renderAppDetailContent()
	m.appViewport.SetContent(content)
	return m.appViewport.View()
}

// renderAppDetailContent generates the app detail content for the viewport.
func (m *Model) renderAppDetailContent() string {
	if m.selectedApp == nil {
		return ""
	}
	a := m.selectedApp
	statusColor := colorSecondary
	if a.status != "running" {
		statusColor = colorMuted
	}
	statusDot := lipgloss.NewStyle().Foreground(lipgloss.Color(statusColor)).Render("●")

	header := lipgloss.JoinHorizontal(lipgloss.Left,
		TitleStyle.Render(a.name),
		SubtitleStyle.Render(statusDot+" "+a.status),
	)

	separator := lipgloss.NewStyle().
		Foreground(lipgloss.Color(colorSurface)).
		Render(strings.Repeat("─", m.width-4))

	infoLines := []string{
		fmt.Sprintf("  Domain:    https://%s", a.domain),
		fmt.Sprintf("  Branch:    %s", a.branch),
		fmt.Sprintf("  Commit:    %s", a.commit),
	}

	if m.db != nil {
		if dep, err := m.db.LastDeployment(a.name); err == nil && !dep.DeployedAt.IsZero() {
			ago := time.Since(dep.DeployedAt).Round(time.Minute)
			infoLines = append(infoLines, fmt.Sprintf("  Deployed:  %s ago", ago))
		}
	}

	infoBlock := lipgloss.JoinVertical(lipgloss.Left, infoLines...)

	actions := []string{}
	actionSpecs := []struct {
		key  string
		desc string
	}{
		{"d", "Deploy now"},
		{"l", "Logs"},
		{"e", "Env vars"},
		{"x", "Stop"},
		{"t", "Start"},
		{"!", "Remove"},
	}

	for _, a := range actionSpecs {
		actions = append(actions,
			ActionKeyStyle.Render("["+a.key+"]")+
				ActionStyle.Render(a.desc))
	}
	actionBar := strings.Join(actions, "  ")
	backHint := HelpStyle.Render("[esc/q] back to dashboard")

	return lipgloss.JoinVertical(lipgloss.Left,
		header,
		separator,
		"",
		infoBlock,
		"",
		separator,
		"",
		actionBar,
		"",
		backHint,
	)
}

// renderSetupWizard renders the guided setup wizard.
func (m *Model) renderSetupWizard() string {
	var content string

	switch m.wizardStep {
	case 0:
		content = m.renderWizardWelcome()
	case 1:
		content = m.renderWizardSettings()
	case 2:
		content = m.renderWizardChecklist()
	case 3:
		content = m.renderWizardProgress()
	case 4:
		content = m.renderWizardSummary()
	}

	steps := m.renderWizardSteps()
	return lipgloss.JoinVertical(lipgloss.Left,
		steps,
		"",
		content,
	)
}

// renderWizardSteps shows the step indicator.
func (m *Model) renderWizardSteps() string {
	stepNames := []string{
		"Welcome",
		"Settings",
		"Hardening",
		"Install",
		"Summary",
	}

	stepStrs := make([]string, len(stepNames))
	for i, name := range stepNames {
		if i < m.wizardStep {
			stepStrs[i] = WizardDoneStyle.Render(fmt.Sprintf("✓ %s", name))
		} else if i == m.wizardStep {
			stepStrs[i] = TitleStyle.Render(fmt.Sprintf("▸ %s", name))
		} else {
			stepStrs[i] = WizardPendingStyle.Render(fmt.Sprintf("○ %s", name))
		}
	}

	return strings.Join(stepStrs, "  ")
}

// renderWizardWelcome renders the welcome screen.
func (m *Model) renderWizardWelcome() string {
	return lipgloss.JoinVertical(lipgloss.Left,
		TitleStyle.Render("Welcome to Davit"),
		"",
		SubtitleStyle.Render("The crane arm for your containers."),
		"",
		"This wizard will:",
		WizardDoneStyle.Render("  ✓ Harden your Linux server"),
		WizardDoneStyle.Render("  ✓ Install Docker and Caddy"),
		WizardDoneStyle.Render("  ✓ Set up firewall rules"),
		WizardDoneStyle.Render("  ✓ Configure TLS automation"),
		"",
		HelpStyle.Render("[Enter] to continue  [Esc] to skip"),
	)
}

// renderWizardSettings renders the settings confirmation screen.
func (m *Model) renderWizardSettings() string {
	return lipgloss.JoinVertical(lipgloss.Left,
		TitleStyle.Render("Server Settings"),
		"",
		fmt.Sprintf("  Hostname:  %s", m.health.hostname),
		fmt.Sprintf("  Timezone:  UTC"),
		fmt.Sprintf("  Email:     admin@example.com"),
		"",
		PanelStyle.Render("These settings can be changed later via 'davit server init'"),
		"",
		HelpStyle.Render("[Enter] to continue  [Esc] back"),
	)
}

// renderWizardChecklist renders the hardening steps checklist.
func (m *Model) renderWizardChecklist() string {
	steps := []string{
		"System update",
		"Install core utilities",
		"Configure timezone",
		"SSH hardening",
		"Configure firewall (22, 80, 443)",
		"Install fail2ban",
		"Install Docker Engine",
		"Install Caddy",
		"Create directory structure",
		"Initialise state database",
		"Install Davit daemon",
	}

	items := make([]string, len(steps))
	for i, step := range steps {
		items[i] = WizardDoneStyle.Render("  ☑ ") + step
	}

	return lipgloss.JoinVertical(lipgloss.Left,
		TitleStyle.Render("Hardening Steps"),
		"",
		lipgloss.JoinVertical(lipgloss.Left, items...),
		"",
		HelpStyle.Render("[Enter] to begin  [Esc] back"),
	)
}

// renderWizardProgress renders the provisioning progress.
func (m *Model) renderWizardProgress() string {
	steps := []struct {
		name   string
		status string
	}{
		{"System update", "ok"},
		{"Core utilities", "ok"},
		{"Timezone", "ok"},
		{"SSH hardening", "ok"},
		{"Firewall", "ok"},
		{"fail2ban", "running"},
		{"Docker Engine", "pending"},
		{"Caddy", "pending"},
		{"Directory structure", "pending"},
		{"State DB", "pending"},
		{"Daemon unit", "pending"},
	}

	lines := make([]string, len(steps))
	for i, s := range steps {
		switch s.status {
		case "ok":
			lines[i] = WizardDoneStyle.Render(fmt.Sprintf("  ✓ %s", s.name))
		case "running":
			lines[i] = TitleStyle.Render(fmt.Sprintf("  ⟳ %s", s.name))
		default:
			lines[i] = WizardPendingStyle.Render(fmt.Sprintf("  ○ %s", s.name))
		}
	}

	return lipgloss.JoinVertical(lipgloss.Left,
		TitleStyle.Render("Provisioning..."),
		"",
		lipgloss.JoinVertical(lipgloss.Left, lines...),
	)
}

// renderWizardSummary renders the final summary.
func (m *Model) renderWizardSummary() string {
	return lipgloss.JoinVertical(lipgloss.Left,
		TitleStyle.Render("Provisioning Complete"),
		"",
		WizardDoneStyle.Render("✓ Docker installed and running"),
		WizardDoneStyle.Render("✓ Caddy installed and running"),
		WizardDoneStyle.Render("✓ Firewall configured"),
		WizardDoneStyle.Render("✓ TLS automation enabled"),
		"",
		SubtitleStyle.Render("Your server is ready to deploy applications."),
		"",
		fmt.Sprintf("  %s", PanelStyle.Render("Run 'davit app create --help' to get started")),
		"",
		HelpStyle.Render("[Enter] to enter dashboard"),
	)
}