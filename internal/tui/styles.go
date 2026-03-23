package tui

import "github.com/charmbracelet/lipgloss"

type styles struct {
	app        lipgloss.Style
	header     lipgloss.Style
	subtle     lipgloss.Style
	preview    lipgloss.Style
	status     lipgloss.Style
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
	border := lipgloss.Color("63")
	muted := lipgloss.Color("241")
	accent := lipgloss.Color("86")
	return styles{
		app:        lipgloss.NewStyle().Padding(1, 2),
		header:     lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("230")),
		subtle:     lipgloss.NewStyle().Foreground(muted),
		preview:    lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(border).Padding(1, 2),
		status:     lipgloss.NewStyle().Foreground(accent),
		error:      lipgloss.NewStyle().Foreground(lipgloss.Color("204")),
		help:       lipgloss.NewStyle().Foreground(muted),
		inputLabel: lipgloss.NewStyle().Bold(true).Foreground(accent),
		overlay:    lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(border).Padding(1, 2),
		selected:   lipgloss.NewStyle().Foreground(lipgloss.Color("230")).Bold(true),
		normal:     lipgloss.NewStyle().Foreground(lipgloss.Color("252")),
		vault:      lipgloss.NewStyle().Foreground(accent),
		masked:     lipgloss.NewStyle().Foreground(muted),
		revealed:   lipgloss.NewStyle().Foreground(lipgloss.Color("230")),
	}
}
