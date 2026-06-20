package tui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"
)

// renderDashboard renders the main dashboard screen.
func (m *Model) renderDashboard() string {
	if m.loading {
		return m.renderLoading()
	}

	m.checkScreen()

	header := m.renderHeader()
	body := m.renderAppList()
	footer := m.renderFooter()

	available := m.height - lipgloss.Height(header) - lipgloss.Height(footer) - 2
	if available < 10 {
		available = 10
	}

	return lipgloss.JoinVertical(lipgloss.Top,
		header,
		lipgloss.NewStyle().Height(available).Render(body),
		footer,
	)
}

// renderHeader renders the top bar with server info and health.
func (m *Model) renderHeader() string {
	hostname := m.health.hostname
	if hostname == "" {
		hostname = "localhost"
	}
	version := m.health.davitVersion
	if version == "" {
		version = "dev"
	}

	// Server identity
	left := TitleStyle.Render("davit") + " " +
		SubtitleStyle.Render(hostname)

	// Uptime and version
	right := SubtitleStyle.Render(fmt.Sprintf("↑ %s    %s", m.health.uptime, version))

	return HeaderStyle.Render(
		lipgloss.JoinHorizontal(lipgloss.Top,
			left,
			lipgloss.NewStyle().Width(m.width - lipgloss.Width(left) - 10).Align(lipgloss.Right).Render(right),
		),
	)
}

// renderAppList renders the apps section of the dashboard.
func (m *Model) renderAppList() string {
	if m.err != nil {
		return ErrorDialogStyle.Render(fmt.Sprintf("Error: %v", m.err))
	}

	sections := []string{}

	// App list header
	appHeader := lipgloss.NewStyle().
		Underline(true).
		Foreground(lipgloss.Color(colorMuted)).
		Render("APPS")
	sections = append(sections, appHeader)

	displayApps := m.filteredApps
	if displayApps == nil {
		displayApps = m.apps
	}

	if len(displayApps) == 0 {
		if m.filterQuery != "" {
			sections = append(sections, fmt.Sprintf("  No apps matching '%s'.", m.filterQuery))
		} else {
			sections = append(sections, "  No applications deployed.\n  Press [n] to create one.")
		}
	} else {
		for i, app := range displayApps {
			icon := StatusIcon(app.status)
			line := fmt.Sprintf("%s %-20s %-35s %-10s %s",
				icon,
				app.name,
				app.domain,
				app.status,
				app.commit,
			)

			if i == m.cursor {
				sections = append(sections, SelectedItemStyle.Render("▸ "+line))
			} else {
				sections = append(sections, AppItemStyle.Render("  "+line))
			}
		}
	}

	// Server health section
	healthHeader := lipgloss.NewStyle().
		Underline(true).
		Foreground(lipgloss.Color(colorMuted)).
		Render("\nSERVER HEALTH")
	sections = append(sections, healthHeader)

	healthLine := m.renderHealthLine()
	sections = append(sections, "  "+healthLine)

	return lipgloss.JoinVertical(lipgloss.Left, sections...)
}

// renderHealthLine renders the server health status row.
func (m *Model) renderHealthLine() string {
	docker := healthIcon(m.health.dockerRunning)
	caddy := healthIcon(m.health.caddyRunning)
	daemon := healthIcon(m.health.daemonRunning)
	fail2ban := healthIcon(m.health.fail2banRunning)

	diskLabel := fmt.Sprintf("Disk %s", m.health.diskUsage)
	memLabel := fmt.Sprintf("Mem %s", m.health.memUsage)

	return fmt.Sprintf("%s Docker  %s Caddy  %s fail2ban  %s Daemon  %s  %s",
		docker, caddy, fail2ban, daemon, diskLabel, memLabel)
}

func healthIcon(ok bool) string {
	if ok {
		return HealthOKStyle.Render("✓")
	}
	return HealthFailStyle.Render("✗")
}

// renderFooter renders the action bar.
func (m *Model) renderFooter() string {
	actions := []string{}
	actionSpecs := []struct {
		key  string
		desc string
	}{
		{"n", "New app"},
		{"/", "Filter"},
		{"d", "Deploy"},
		{"r", "Refresh"},
		{"s", "Server"},
		{"?", "Help"},
	}

	for _, a := range actionSpecs {
		actions = append(actions,
			ActionKeyStyle.Render("["+a.key+"]")+
				ActionStyle.Render(a.desc))
	}

	return FooterStyle.Render(
		strings.Join(actions, "  "),
	)
}

// renderHelp renders the help overlay using keymap bindings.
func (m *Model) renderHelp() string {
	keyStrs := m.keys.FullHelp()
	var lines []string
	lines = append(lines, TitleStyle.Render("Help"))
	lines = append(lines, "")
	lines = append(lines, "Navigation:")
	for _, group := range keyStrs {
		for _, b := range group {
			helpKeys := strings.Join(b.Keys(), ", ")
			helpText := b.Help().Desc
			if helpText != "" {
				lines = append(lines, fmt.Sprintf("  %-16s %s", helpKeys, helpText))
			}
		}
	}
	lines = append(lines, "")
	lines = append(lines, HelpStyle.Render("Press [?] or [Esc] to close."))

	return lipgloss.JoinVertical(lipgloss.Left, lines...)
}

// renderLoading renders a loading indicator.
func (m *Model) renderLoading() string {
	return lipgloss.NewStyle().
		Padding(2, 0).
		Foreground(lipgloss.Color(colorSubtle)).
		Render("  Loading...")
}