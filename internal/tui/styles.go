// Package tui implements the interactive Bubble Tea TUI for Davit.
package tui

import (
	"github.com/charmbracelet/lipgloss"
)

// Colour palette.
const (
	colorPrimary   = "#7C3AED" // violet
	colorSecondary = "#10B981" // emerald
	colorWarn      = "#F59E0B" // amber
	colorError     = "#EF4444" // red
	colorMuted     = "#6B7280" // gray-500
	colorSubtle    = "#9CA3AF" // gray-400
	colorBg        = "#1F2937" // gray-800
	colorSurface   = "#374151" // gray-700
	colorText      = "#F3F4F6" // gray-100
	colorDimText   = "#D1D5DB" // gray-300
)

// Shared styles.
var (
	// Base styles.
	DocStyle = lipgloss.NewStyle().
			Padding(1, 2, 1, 2).
			Background(lipgloss.Color(colorBg))

	TitleStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.Color(colorPrimary)).
			Padding(0, 1)

	SubtitleStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color(colorSubtle)).
			Padding(0, 1)

	// Status indicator styles.
	RunningStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color(colorSecondary)).
			SetString("●")

	StoppedStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color(colorMuted)).
			SetString("○")

	ErrorStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color(colorError)).
			SetString("●")

	// Health styles.
	HealthOKStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color(colorSecondary))

	HealthFailStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color(colorError))

	// Header/Footer.
	HeaderStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.Color(colorPrimary)).
			Padding(0, 1).
			BorderStyle(lipgloss.NormalBorder()).
			BorderBottom(true).
			BorderForeground(lipgloss.Color(colorSurface))

	FooterStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color(colorMuted)).
			Padding(0, 1).
			BorderStyle(lipgloss.NormalBorder()).
			BorderTop(true).
			BorderForeground(lipgloss.Color(colorSurface))

	HelpStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color(colorSubtle)).
			Padding(0, 1, 0, 0)

	KeyStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.Color(colorPrimary)).
			Padding(0, 0)

	// Item styles.
	AppItemStyle = lipgloss.NewStyle().
			Padding(0, 2, 0, 1)

	SelectedItemStyle = lipgloss.NewStyle().
				Padding(0, 2, 0, 1).
				Background(lipgloss.Color(colorSurface)).
				Foreground(lipgloss.Color(colorPrimary))

	// Action button.
	ActionStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color(colorSubtle)).
			Padding(0, 1)

	ActionKeyStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.Color(colorPrimary))

	// Panels.
	PanelStyle = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(lipgloss.Color(colorSurface)).
			Padding(0, 1).
			Margin(0, 0, 1, 0)

	// Wizard.
	WizardStepStyle = lipgloss.NewStyle().
			Padding(0, 1, 0, 2)

	WizardDoneStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color(colorSecondary))

	WizardPendingStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color(colorMuted))

	// Error dialog.
	ErrorDialogStyle = lipgloss.NewStyle().
				Border(lipgloss.DoubleBorder()).
				BorderForeground(lipgloss.Color(colorError)).
				Padding(1, 2).
				Foreground(lipgloss.Color(colorError))
)

// StatusIcon returns the styled status indicator for an app status string.
func StatusIcon(status string) string {
	switch status {
	case "running":
		return RunningStyle.String()
	case "stopped", "created":
		return StoppedStyle.String()
	default:
		return ErrorStyle.String()
	}
}