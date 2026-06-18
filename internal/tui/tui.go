package tui

import (
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/help"
	"github.com/charmbracelet/bubbles/key"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/getdavit/davit/internal/app"
	"github.com/getdavit/davit/internal/state"
)

// Screen types.
type screenType int

const (
	screenDashboard screenType = iota
	screenAppDetail
	screenSetupWizard
	screenHelpOverlay
	screenQuit
)

// Server health info.
type serverHealth struct {
	hostname       string
	davitVersion   string
	provisioned    bool
	uptime         string
	dockerRunning  bool
	caddyRunning   bool
	daemonRunning  bool
	fail2banRunning bool
	firewallActive bool
	appsTotal      int
	appsRunning    int
	diskUsage      string
	memUsage       string
}

// Model is the root Bubble Tea model for the Davit TUI.
type Model struct {
	db  *state.DB
	mgr *app.Manager
	cfg *configFile

	screen  screenType
	err     error
	width   int
	height  int
	ready   bool
	loading bool

	// Navigation
	cursor    int
	help      help.Model
	keys      keyMap

	// Data
	apps        []appSummaryItem
	health      serverHealth
	selectedApp *appSummaryItem

	// Sub-components
	appViewport viewport.Model
	wizardStep  int
	wizardDone  bool

	// Setup wizard
	provisioned bool
}

// appSummaryItem holds display data for one app row.
type appSummaryItem struct {
	name     string
	status   string
	domain   string
	branch   string
	commit   string
	deployed string
	url      string
}

// keyMap defines TUI key bindings.
type keyMap struct {
	Up       key.Binding
	Down     key.Binding
	Enter    key.Binding
	Back     key.Binding
	Quit     key.Binding
	Help     key.Binding
	Refresh  key.Binding
	New      key.Binding
	Deploy   key.Binding
	Logs     key.Binding
	Server   key.Binding
	Filter   key.Binding
	Stop     key.Binding
	Start    key.Binding
	Restart  key.Binding
	Remove   key.Binding
	Env      key.Binding
}

// ShortHelp returns key bindings for the help footer.
func (k keyMap) ShortHelp() []key.Binding {
	return []key.Binding{k.Up, k.Down, k.Enter, k.Help, k.Quit}
}

// FullHelp returns all key bindings.
func (k keyMap) FullHelp() [][]key.Binding {
	return [][]key.Binding{
		{k.Up, k.Down, k.Enter, k.Back},
		{k.Refresh, k.New, k.Deploy, k.Logs},
		{k.Server, k.Help, k.Quit},
	}
}

var keys = keyMap{
	Up: key.NewBinding(
		key.WithKeys("up", "k"),
		key.WithHelp("↑/k", "up"),
	),
	Down: key.NewBinding(
		key.WithKeys("down", "j"),
		key.WithHelp("↓/j", "down"),
	),
	Enter: key.NewBinding(
		key.WithKeys("enter"),
		key.WithHelp("enter", "select"),
	),
	Back: key.NewBinding(
		key.WithKeys("esc", "q"),
		key.WithHelp("esc/q", "back"),
	),
	Quit: key.NewBinding(
		key.WithKeys("ctrl+c"),
		key.WithHelp("ctrl+c", "quit"),
	),
	Help: key.NewBinding(
		key.WithKeys("?"),
		key.WithHelp("?", "help"),
	),
	Refresh: key.NewBinding(
		key.WithKeys("r"),
		key.WithHelp("r", "refresh"),
	),
	New: key.NewBinding(
		key.WithKeys("n"),
		key.WithHelp("n", "new"),
	),
	Deploy: key.NewBinding(
		key.WithKeys("d"),
		key.WithHelp("d", "deploy"),
	),
	Logs: key.NewBinding(
		key.WithKeys("l"),
		key.WithHelp("l", "logs"),
	),
	Server: key.NewBinding(
		key.WithKeys("s"),
		key.WithHelp("s", "server"),
	),
	Filter: key.NewBinding(
		key.WithKeys("/"),
		key.WithHelp("/", "filter"),
	),
	Stop: key.NewBinding(
		key.WithKeys("x"),
		key.WithHelp("x", "stop"),
	),
	Start: key.NewBinding(
		key.WithKeys("t"),
		key.WithHelp("t", "start"),
	),
	Restart: key.NewBinding(
		key.WithKeys("r"),
		key.WithHelp("r", "restart"),
	),
	Remove: key.NewBinding(
		key.WithKeys("!"),
		key.WithHelp("!", "remove"),
	),
	Env: key.NewBinding(
		key.WithKeys("e"),
		key.WithHelp("e", "env vars"),
	),
}

// configFile is a minimal config wrapper for TUI use.
type configFile struct {
	adminAPI string
}

// NewModel creates a new TUI model.
func NewModel(db *state.DB, mgr *app.Manager, adminAPI string) *Model {
	return &Model{
		db:        db,
		mgr:       mgr,
		cfg:       &configFile{adminAPI: adminAPI},
		screen:    screenSetupWizard, // default; checked on first update
		help:      help.New(),
		keys:      keys,
		cursor:    0,
		loading:   true,
		wizardStep: 0,
		provisioned: false,
	}
}

// Init initializes the TUI by loading data.
func (m *Model) Init() tea.Cmd {
	return tea.Batch(
		loadAppsCmd(m.mgr),
		loadHealthCmd(m.db),
		tea.EnterAltScreen,
	)
}

// loadAppsCmd returns a command that loads the app list.
func loadAppsCmd(mgr *app.Manager) tea.Cmd {
	return func() tea.Msg {
		apps, err := mgr.List()
		if err != nil {
			return appsLoadedMsg{err: err}
		}
		items := make([]appSummaryItem, len(apps))
		for i, a := range apps {
			items[i] = appSummaryItem{
				name:   a.Name,
				status: a.Status,
				domain: a.Domain,
				branch: a.Branch,
				commit: truncateSHA(a.CommitSHA),
				url:    a.URL,
			}
		}
		return appsLoadedMsg{apps: items}
	}
}

// loadHealthCmd returns a command that loads server health info.
func loadHealthCmd(db *state.DB) tea.Cmd {
	return func() tea.Msg {
		h := serverHealth{
			provisioned: false,
		}
		if db == nil {
			return healthLoadedMsg{health: h}
		}

		hostname, _ := db.GetSystemInfo("hostname")
		version, _ := db.GetSystemInfo("davit_version")
		provisionedStr, _ := db.GetSystemInfo("provisioned")

		h.hostname = hostname
		h.davitVersion = version
		h.provisioned = provisionedStr == "true"

		// Get uptime from /proc
		if data, err := os.ReadFile("/proc/uptime"); err == nil {
			parts := strings.Fields(string(data))
			if len(parts) > 0 {
				var secs float64
				fmt.Sscanf(parts[0], "%f", &secs)
				h.uptime = formatDuration(time.Duration(secs) * time.Second)
			}
		}

		// Get app counts
		apps, _ := db.ListApps()
		h.appsTotal = len(apps)
		for _, a := range apps {
			if a.Status == "running" {
				h.appsRunning++
			}
		}

		// Check Docker
		h.dockerRunning = true // optimistic; in real TUI we'd check

		return healthLoadedMsg{health: h}
	}
}

// Message types.
type appsLoadedMsg struct {
	apps []appSummaryItem
	err  error
}

type healthLoadedMsg struct {
	health serverHealth
}

// Update handles all messages and key events.
func (m *Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		m.ready = true
		m.appViewport = viewport.New(msg.Width-4, msg.Height-10)
		m.appViewport.Style = lipgloss.NewStyle().Padding(0, 1)
		return m, nil

	case tea.KeyMsg:
		return m.handleKey(msg)

	case appsLoadedMsg:
		m.loading = false
		if msg.err != nil {
			m.err = msg.err
			break
		}
		m.apps = msg.apps

	case healthLoadedMsg:
		m.loading = false
		m.health = msg.health
		m.provisioned = msg.health.provisioned
		if m.provisioned && m.screen == screenSetupWizard {
			m.screen = screenDashboard
		}
	}

	return m, nil
}

// handleKey routes key presses based on the current screen.
func (m *Model) handleKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	if key.Matches(msg, m.keys.Quit) {
		m.screen = screenQuit
		return m, tea.Quit
	}

	if key.Matches(msg, m.keys.Help) {
		if m.screen == screenHelpOverlay {
			m.screen = screenDashboard
		} else {
			m.screen = screenHelpOverlay
		}
		return m, nil
	}

	switch m.screen {
	case screenDashboard:
		return m.handleDashboardKey(msg)
	case screenAppDetail:
		return m.handleAppDetailKey(msg)
	case screenSetupWizard:
		return m.handleWizardKey(msg)
	case screenHelpOverlay:
		m.screen = screenDashboard
		return m, nil
	}

	return m, nil
}

// handleDashboardKey handles key events on the dashboard.
func (m *Model) handleDashboardKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	if key.Matches(msg, m.keys.Down) {
		if m.cursor < len(m.apps)-1 {
			m.cursor++
		}
	} else if key.Matches(msg, m.keys.Up) {
		if m.cursor > 0 {
			m.cursor--
		}
	} else if key.Matches(msg, m.keys.Enter) {
		if len(m.apps) > 0 && m.cursor < len(m.apps) {
			app := m.apps[m.cursor]
			m.selectedApp = &app
			m.screen = screenAppDetail
			m.appViewport.SetContent(m.renderAppDetailContent())
		}
	} else if key.Matches(msg, m.keys.Refresh) {
		m.loading = true
		return m, tea.Batch(
			loadAppsCmd(m.mgr),
			loadHealthCmd(m.db),
		)
	} else if key.Matches(msg, m.keys.Deploy) {
		if len(m.apps) > 0 && m.cursor < len(m.apps) {
			// Trigger deploy via background
			appName := m.apps[m.cursor].name
			return m, deployAppCmd(m.mgr, appName)
		}
	} else if key.Matches(msg, m.keys.Server) {
		// Re-read server status
		return m, loadHealthCmd(m.db)
	} else if key.Matches(msg, m.keys.Filter) {
		// Placeholder for filter mode
	}

	return m, nil
}

// deployAppCmd triggers a deployment.
func deployAppCmd(mgr *app.Manager, name string) tea.Cmd {
	return func() tea.Msg {
		_, err := mgr.Deploy(app.DeployOptions{
			AppName: name,
			Timeout: 60,
		})
		if err != nil {
			return deployResultMsg{appName: name, err: err}
		}
		return deployResultMsg{appName: name, success: true}
	}
}

type deployResultMsg struct {
	appName string
	success bool
	err     error
}

// handleAppDetailKey handles key events on the app detail screen.
func (m *Model) handleAppDetailKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	if key.Matches(msg, m.keys.Back) {
		m.screen = screenDashboard
		m.selectedApp = nil
		return m, nil
	}
	if key.Matches(msg, m.keys.Deploy) && m.selectedApp != nil {
		return m, deployAppCmd(m.mgr, m.selectedApp.name)
	}
	if key.Matches(msg, m.keys.Logs) && m.selectedApp != nil {
		// Logs placeholder — in real impl would show log viewer
		return m, nil
	}
	if key.Matches(msg, m.keys.Stop) && m.selectedApp != nil {
		return m, stopAppCmd(m.mgr, m.selectedApp.name)
	}
	if key.Matches(msg, m.keys.Start) && m.selectedApp != nil {
		return m, startAppCmd(m.mgr, m.selectedApp.name)
	}

	// Viewport scrolling
	var cmd tea.Cmd
	m.appViewport, cmd = m.appViewport.Update(msg)
	return m, cmd
}

func stopAppCmd(mgr *app.Manager, name string) tea.Cmd {
	return func() tea.Msg {
		_, err := mgr.Stop(name)
		if err != nil {
			return appActionMsg{appName: name, action: "stop", err: err}
		}
		return appActionMsg{appName: name, action: "stop", success: true}
	}
}

func startAppCmd(mgr *app.Manager, name string) tea.Cmd {
	return func() tea.Msg {
		_, err := mgr.Start(name, 60)
		if err != nil {
			return appActionMsg{appName: name, action: "start", err: err}
		}
		return appActionMsg{appName: name, action: "start", success: true}
	}
}

type appActionMsg struct {
	appName string
	action  string
	success bool
	err     error
}

// handleWizardKey handles key events on the setup wizard.
func (m *Model) handleWizardKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "enter":
		if m.wizardStep < 4 {
			m.wizardStep++
		} else {
			m.screen = screenDashboard
			m.loading = true
			return m, loadHealthCmd(m.db)
		}
	case "esc", "q":
		m.screen = screenDashboard
	}
	return m, nil
}

// View renders the current screen.
func (m *Model) View() string {
	if !m.ready {
		return "\n  Loading..."
	}

	switch m.screen {
	case screenDashboard:
		return m.renderDashboard()
	case screenAppDetail:
		return m.renderAppDetail()
	case screenSetupWizard:
		return m.renderSetupWizard()
	case screenHelpOverlay:
		return m.renderHelp()
	case screenQuit:
		return ""
	}
	return ""
}

// Helper functions.

func truncateSHA(sha string) string {
	if len(sha) > 7 {
		return sha[:7]
	}
	return sha
}

func formatDuration(d time.Duration) string {
	days := int(d.Hours()) / 24
	hours := int(d.Hours()) % 24
	min := int(d.Minutes()) % 60

	parts := []string{}
	if days > 0 {
		parts = append(parts, fmt.Sprintf("%dd", days))
	}
	if hours > 0 || days > 0 {
		parts = append(parts, fmt.Sprintf("%dh", hours))
	}
	parts = append(parts, fmt.Sprintf("%dm", min))
	return strings.Join(parts, " ")
}

// ensure m.screen is set to dashboard when provisioned
func (m *Model) checkScreen() {
	if m.screen == screenSetupWizard && m.provisioned {
		m.screen = screenDashboard
	}
}

// Run starts the Bubble Tea TUI program.
func Run(model *Model) error {
	p := tea.NewProgram(model, tea.WithAltScreen())
	_, err := p.Run()
	return err
}