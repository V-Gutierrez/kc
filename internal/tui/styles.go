package tui

import "github.com/charmbracelet/lipgloss"

type styles struct {
	app        lipgloss.Style
	header     lipgloss.Style
	subtle     lipgloss.Style
	preview    lipgloss.Style
	status     lipgloss.Style
	rowAlt     lipgloss.Style
	error      lipgloss.Style
	help       lipgloss.Style
	inputLabel lipgloss.Style
	overlay    lipgloss.Style
	selected   lipgloss.Style
	normal     lipgloss.Style
	vault      lipgloss.Style
	masked     lipgloss.Style
	revealed   lipgloss.Style
}

func newStyles() styles {
	chiefsRed := lipgloss.Color("#E31837")
	chiefsGold := lipgloss.Color("#FFB81C")
	chiefsDark := lipgloss.Color("#1a1a2e")
	muted := lipgloss.Color("241")

	return styles{
		app:        lipgloss.NewStyle().Padding(1, 2),
		header:     lipgloss.NewStyle().Bold(true).Foreground(chiefsGold).Background(chiefsDark).Padding(0, 1),
		subtle:     lipgloss.NewStyle().Foreground(muted),
		preview:    lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(chiefsGold).Padding(1, 2),
		status:     lipgloss.NewStyle().Foreground(chiefsGold).Background(chiefsDark).Padding(0, 1),
		rowAlt:     lipgloss.NewStyle().Background(chiefsDark),
		error:      lipgloss.NewStyle().Foreground(chiefsRed),
		help:       lipgloss.NewStyle().Foreground(muted),
		inputLabel: lipgloss.NewStyle().Bold(true).Foreground(chiefsGold),
		overlay:    lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(chiefsRed).Padding(1, 2),
		selected:   lipgloss.NewStyle().Foreground(chiefsGold).Background(chiefsRed).Bold(true),
		normal:     lipgloss.NewStyle().Foreground(lipgloss.Color("252")),
		vault:      lipgloss.NewStyle().Foreground(chiefsGold),
		masked:     lipgloss.NewStyle().Foreground(muted),
		revealed:   lipgloss.NewStyle().Foreground(chiefsRed),
	}
}
