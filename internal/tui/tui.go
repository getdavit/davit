package tui

import (
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/help"
	"github.com/charmbracelet/bubbles/key"
	"github.com/charmbracelet/bubbles/textinput"
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
	screenNewApp
	screenLogs
	screenEnvVars
	screenRemoveConfirm
	screenFilter
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

	// New app creation
	newAppStep       int
	newAppNameInput  textinput.Model
	newAppRepoInput  textinput.Model
	newAppBranch     string
	newAppDomain     string
	newAppError      string
	newAppSuccessMsg string

	// Logs
	logsContent string
	logsError   string

	// Env vars
	envContent string
	envError   string

	// Remove confirmation
	removeConfirmed bool
	removeError     string

	// Filter
	filterInput   textinput.Model
	filterQuery   string
	filteredApps  []appSummaryItem
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
		{k.Stop, k.Start, k.Restart, k.Remove},
		{k.Env, k.Filter, k.Server, k.Help, k.Quit},
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
		key.WithKeys("R"),
		key.WithHelp("R", "restart"),
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
	ti := textinput.New()
	ti.Placeholder = "my-app-name"
	ti.Focus()
	ti.CharLimit = 50
	ti.Width = 40

	tiRepo := textinput.New()
	tiRepo.Placeholder = "https://github.com/user/repo.git"
	tiRepo.CharLimit = 200
	tiRepo.Width = 60

	fi := textinput.New()
	fi.Placeholder = "Type to filter apps..."
	fi.CharLimit = 50
	fi.Width = 40

	return &Model{
		db:        db,
		mgr:       mgr,
		cfg:       &configFile{adminAPI: adminAPI},
		screen:    screenSetupWizard,
		help:      help.New(),
		keys:      keys,
		cursor:    0,
		loading:   true,
		wizardStep: 0,
		provisioned: false,
		newAppNameInput:  ti,
		newAppRepoInput:  tiRepo,
		filterInput:      fi,
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
		// Apply active filter
		if m.filterQuery == "" {
			m.filteredApps = make([]appSummaryItem, len(m.apps))
			copy(m.filteredApps, m.apps)
		} else {
			m.filteredApps = nil
			q := strings.ToLower(m.filterQuery)
			for _, a := range m.apps {
				if strings.Contains(strings.ToLower(a.name), q) ||
					strings.Contains(strings.ToLower(a.domain), q) ||
					strings.Contains(strings.ToLower(a.status), q) {
					m.filteredApps = append(m.filteredApps, a)
				}
			}
		}

	case healthLoadedMsg:
		m.loading = false
		m.health = msg.health
		m.provisioned = msg.health.provisioned
		if m.provisioned && m.screen == screenSetupWizard {
			m.screen = screenDashboard
		}

	case logsLoadedMsg:
		m.loading = false
		if msg.err != nil {
			m.logsContent = fmt.Sprintf("Error loading logs: %v", msg.err)
		} else {
			m.logsContent = msg.content
		}

	case envLoadedMsg:
		m.loading = false
		if msg.err != nil {
			m.envContent = fmt.Sprintf("Error loading env vars: %v", msg.err)
		} else {
			m.envContent = msg.content
		}

	case createAppResultMsg:
		m.loading = false
		if msg.err != nil {
			m.newAppError = fmt.Sprintf("Failed to create app: %v", msg.err)
		} else {
			m.newAppSuccessMsg = fmt.Sprintf("✓ App '%s' created successfully", msg.name)
		}
		m.newAppStep = 2 // show result

	case removeAppResultMsg:
		m.loading = false
		if msg.err != nil {
			m.removeError = fmt.Sprintf("Failed to remove app: %v", msg.err)
		} else {
			m.screen = screenDashboard
			m.selectedApp = nil
			// Refresh app list
			return m, tea.Batch(
				loadAppsCmd(m.mgr),
				loadHealthCmd(m.db),
			)
		}
	}

	return m, nil
}

// handleKey routes key presses based on the current screen.
func (m *Model) handleKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	if key.Matches(msg, m.keys.Quit) {
		// If on a sub-screen other than dashboard, go back instead of quitting
		switch m.screen {
		case screenNewApp, screenLogs, screenEnvVars, screenRemoveConfirm, screenFilter:
			m.screen = screenDashboard
			return m, nil
		}
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
	case screenNewApp:
		return m.handleNewAppKey(msg)
	case screenLogs:
		return m.handleLogsKey(msg)
	case screenEnvVars:
		return m.handleEnvKey(msg)
	case screenRemoveConfirm:
		return m.handleRemoveConfirmKey(msg)
	case screenFilter:
		return m.handleFilterKey(msg)
	}

	return m, nil
}

// handleDashboardKey handles key events on the dashboard.
func (m *Model) handleDashboardKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	if key.Matches(msg, m.keys.Down) {
		if m.cursor < len(m.filteredApps)-1 {
			m.cursor++
		}
	} else if key.Matches(msg, m.keys.Up) {
		if m.cursor > 0 {
			m.cursor--
		}
	} else if key.Matches(msg, m.keys.Enter) {
		if len(m.filteredApps) > 0 && m.cursor < len(m.filteredApps) {
			app := m.filteredApps[m.cursor]
			m.selectedApp = &app
			m.screen = screenAppDetail
			m.appViewport.SetContent(m.renderAppDetailContent())
		}
	} else if key.Matches(msg, m.keys.Refresh) {
		m.loading = true
		m.filteredApps = nil
		m.filterQuery = ""
		m.filterInput.SetValue("")
		m.cursor = 0
		return m, tea.Batch(
			loadAppsCmd(m.mgr),
			loadHealthCmd(m.db),
		)
	} else if key.Matches(msg, m.keys.Deploy) {
		if len(m.filteredApps) > 0 && m.cursor < len(m.filteredApps) {
			appName := m.filteredApps[m.cursor].name
			return m, deployAppCmd(m.mgr, appName)
		}
	} else if key.Matches(msg, m.keys.Server) {
		return m, loadHealthCmd(m.db)
	} else if key.Matches(msg, m.keys.Filter) {
		m.screen = screenFilter
		m.filterInput.Focus()
		m.filterInput.SetValue(m.filterQuery)
		return m, nil
	} else if key.Matches(msg, m.keys.New) {
		m.screen = screenNewApp
		m.newAppStep = 0
		m.newAppNameInput.Focus()
		m.newAppNameInput.SetValue("")
		m.newAppRepoInput.SetValue("")
		m.newAppBranch = ""
		m.newAppDomain = ""
		m.newAppError = ""
		m.newAppSuccessMsg = ""
		return m, nil
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
		m.logsContent = ""
		m.logsError = ""
		m.screen = screenLogs
		return m, loadLogsCmd(m.mgr, m.selectedApp.name, 50)
	}
	if key.Matches(msg, m.keys.Stop) && m.selectedApp != nil {
		return m, stopAppCmd(m.mgr, m.selectedApp.name)
	}
	if key.Matches(msg, m.keys.Start) && m.selectedApp != nil {
		return m, startAppCmd(m.mgr, m.selectedApp.name)
	}
	if key.Matches(msg, m.keys.Remove) && m.selectedApp != nil {
		m.screen = screenRemoveConfirm
		m.removeConfirmed = false
		m.removeError = ""
		return m, nil
	}
	if key.Matches(msg, m.keys.Env) && m.selectedApp != nil {
		m.envContent = ""
		m.envError = ""
		m.screen = screenEnvVars
		return m, loadEnvCmd(m.mgr, m.selectedApp.name)
	}
	if key.Matches(msg, m.keys.Restart) && m.selectedApp != nil {
		return m, restartAppCmd(m.mgr, m.selectedApp.name)
	}
	if key.Matches(msg, m.keys.Refresh) {
		if m.selectedApp != nil {
			return m, tea.Batch(
				loadAppsCmd(m.mgr),
				loadHealthCmd(m.db),
			)
		}
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

func restartAppCmd(mgr *app.Manager, name string) tea.Cmd {
	return func() tea.Msg {
		_, err := mgr.Restart(name, 60)
		if err != nil {
			return appActionMsg{appName: name, action: "restart", err: err}
		}
		return appActionMsg{appName: name, action: "restart", success: true}
	}
}

type appActionMsg struct {
	appName string
	action  string
	success bool
	err     error
}

// handleNewAppKey handles key events on the new-app creation screen.
func (m *Model) handleNewAppKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch m.newAppStep {
	case 0:
		// Step 0: entering app name
		switch msg.String() {
		case "esc":
			m.screen = screenDashboard
			return m, nil
		case "enter":
			name := m.newAppNameInput.Value()
			if name == "" {
				m.newAppError = "App name is required"
				return m, nil
			}
			m.newAppError = ""
			m.newAppStep = 1
			m.newAppRepoInput.Focus()
			return m, nil
		default:
			var cmd tea.Cmd
			m.newAppNameInput, cmd = m.newAppNameInput.Update(msg)
			return m, cmd
		}
	case 1:
		// Step 1: entering repo URL
		switch msg.String() {
		case "esc":
			m.screen = screenDashboard
			return m, nil
		case "enter":
			name := m.newAppNameInput.Value()
			repoURL := m.newAppRepoInput.Value()
			if name == "" {
				m.newAppError = "App name is required"
				m.newAppStep = 0
				m.newAppNameInput.Focus()
				return m, nil
			}
			if repoURL == "" {
				m.newAppError = "Git repository URL is required"
				return m, nil
			}
			m.newAppError = ""
			m.newAppSuccessMsg = "Creating app..."
			return m, createAppCmd(m.mgr, name, repoURL)
		default:
			var cmd tea.Cmd
			m.newAppRepoInput, cmd = m.newAppRepoInput.Update(msg)
			return m, cmd
		}
	default:
		// After creation — Esc goes back to dashboard
		if msg.String() == "esc" || msg.String() == "enter" {
			m.screen = screenDashboard
			return m, nil
		}
		return m, nil
	}
}

// handleLogsKey handles key events on the logs screen.
func (m *Model) handleLogsKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	if msg.String() == "esc" || msg.String() == "q" || key.Matches(msg, m.keys.Back) {
		m.screen = screenAppDetail
		return m, nil
	}
	var cmd tea.Cmd
	m.appViewport, cmd = m.appViewport.Update(msg)
	return m, cmd
}

// handleEnvKey handles key events on the env vars screen.
func (m *Model) handleEnvKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	if msg.String() == "esc" || msg.String() == "q" || key.Matches(msg, m.keys.Back) {
		m.screen = screenAppDetail
		return m, nil
	}
	var cmd tea.Cmd
	m.appViewport, cmd = m.appViewport.Update(msg)
	return m, cmd
}

// handleRemoveConfirmKey handles key events on the remove confirmation screen.
func (m *Model) handleRemoveConfirmKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "y", "Y":
		if m.selectedApp != nil {
			m.removeConfirmed = true
			m.removeError = ""
			return m, removeAppCmd(m.mgr, m.selectedApp.name)
		}
	case "n", "N", "esc":
		m.screen = screenAppDetail
		return m, nil
	}
	return m, nil
}

// handleFilterKey handles key events on the filter screen.
func (m *Model) handleFilterKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "esc":
		m.screen = screenDashboard
		m.filterInput.Blur()
		return m, nil
	case "enter":
		m.filterQuery = m.filterInput.Value()
		// Update filtered list
		m.filteredApps = nil
		if m.filterQuery == "" {
			m.filteredApps = make([]appSummaryItem, len(m.apps))
			copy(m.filteredApps, m.apps)
		} else {
			q := strings.ToLower(m.filterQuery)
			for _, a := range m.apps {
				if strings.Contains(strings.ToLower(a.name), q) ||
					strings.Contains(strings.ToLower(a.domain), q) ||
					strings.Contains(strings.ToLower(a.status), q) {
					m.filteredApps = append(m.filteredApps, a)
				}
			}
		}
		m.cursor = 0
		m.screen = screenDashboard
		m.filterInput.Blur()
		return m, nil
	default:
		var cmd tea.Cmd
		m.filterInput, cmd = m.filterInput.Update(msg)
		return m, cmd
	}
}

// loadLogsCmd returns a command that fetches app logs.
func loadLogsCmd(mgr *app.Manager, appName string, lines int) tea.Cmd {
	return func() tea.Msg {
		if mgr == nil {
			return logsLoadedMsg{appName: appName, content: "No manager available (nil)."}
		}
		var buf strings.Builder
		err := mgr.Logs(appName, lines, false, &buf)
		if err != nil {
			return logsLoadedMsg{appName: appName, err: err}
		}
		return logsLoadedMsg{appName: appName, content: buf.String()}
	}
}

// loadEnvCmd returns a command that fetches env vars for an app.
func loadEnvCmd(mgr *app.Manager, appName string) tea.Cmd {
	return func() tea.Msg {
		if mgr == nil {
			return envLoadedMsg{appName: appName, content: "No manager available (nil)."}
		}
		vars, err := mgr.EnvList(appName)
		if err != nil {
			return envLoadedMsg{appName: appName, err: err}
		}
		var b strings.Builder
		for _, v := range vars {
			b.WriteString(fmt.Sprintf("  %s  (updated %s)\n", v.Key, v.UpdatedAt))
		}
		if b.Len() == 0 {
			b.WriteString("  No environment variables set.")
		}
		return envLoadedMsg{appName: appName, content: b.String()}
	}
}

// createAppCmd returns a command that creates a new app.
func createAppCmd(mgr *app.Manager, name, repoURL string) tea.Cmd {
	return func() tea.Msg {
		if mgr == nil {
			return createAppResultMsg{name: name, err: fmt.Errorf("no manager available (nil)")}
		}
		_, err := mgr.Create(app.CreateOptions{
			Name:    name,
			RepoURL: repoURL,
		})
		if err != nil {
			return createAppResultMsg{name: name, err: err}
		}
		return createAppResultMsg{name: name, success: true}
	}
}

// removeAppCmd returns a command that removes an app.
func removeAppCmd(mgr *app.Manager, appName string) tea.Cmd {
	return func() tea.Msg {
		if mgr == nil {
			return removeAppResultMsg{appName: appName, err: fmt.Errorf("no manager available (nil)")}
		}
		_, err := mgr.Remove(appName, false)
		if err != nil {
			return removeAppResultMsg{appName: appName, err: err}
		}
		return removeAppResultMsg{appName: appName, success: true}
	}
}

// Message types for new screens.
type logsLoadedMsg struct {
	appName string
	content string
	err     error
}

type envLoadedMsg struct {
	appName string
	content string
	err     error
}

type createAppResultMsg struct {
	name    string
	success bool
	err     error
}

type removeAppResultMsg struct {
	appName string
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
	case screenNewApp:
		return m.renderNewApp()
	case screenLogs:
		return m.renderLogs()
	case screenEnvVars:
		return m.renderEnvVars()
	case screenRemoveConfirm:
		return m.renderRemoveConfirm()
	case screenFilter:
		return m.renderFilter()
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