package tui

import "github.com/charmbracelet/lipgloss"

type styles struct {
	app          lipgloss.Style
	header       lipgloss.Style
	subtle       lipgloss.Style
	preview      lipgloss.Style
	previewTitle lipgloss.Style
	status       lipgloss.Style
	rowAlt       lipgloss.Style
	error        lipgloss.Style
	help         lipgloss.Style
	inputLabel   lipgloss.Style
	overlay      lipgloss.Style
	selected     lipgloss.Style
	normal       lipgloss.Style
	vault        lipgloss.Style
	masked       lipgloss.Style
	revealed     lipgloss.Style
	banner       lipgloss.Style
	loading      lipgloss.Style
	flash        lipgloss.Style
	welcome      lipgloss.Style
	welcomeTitle lipgloss.Style
	welcomeKey   lipgloss.Style
	welcomeDesc  lipgloss.Style
	prefix       lipgloss.Style
	borderRed    lipgloss.Style
	borderGold   lipgloss.Style
}

func newStyles() styles {
	chiefsRed := lipgloss.Color("#E31837")
	chiefsGold := lipgloss.Color("#FFB81C")
	chiefsDark := lipgloss.Color("#1a1a2e")
	muted := lipgloss.Color("241")

	return styles{
		app:          lipgloss.NewStyle().Padding(1, 2),
		header:       lipgloss.NewStyle().Bold(true).Foreground(chiefsGold).Background(chiefsDark).Padding(0, 1),
		subtle:       lipgloss.NewStyle().Foreground(muted),
		preview:      lipgloss.NewStyle().Border(lipgloss.RoundedBorder(), false, true, true, true).BorderForeground(chiefsGold).Padding(1, 2),
		previewTitle: lipgloss.NewStyle().Foreground(chiefsGold).Bold(true),
		status:       lipgloss.NewStyle().Foreground(chiefsGold).Background(chiefsDark).Padding(0, 1),
		rowAlt:       lipgloss.NewStyle().Background(chiefsDark),
		error:        lipgloss.NewStyle().Foreground(chiefsRed),
		help:         lipgloss.NewStyle().Foreground(muted),
		inputLabel:   lipgloss.NewStyle().Bold(true).Foreground(chiefsGold),
		overlay:      lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(chiefsRed).Padding(1, 2),
		selected:     lipgloss.NewStyle().Foreground(chiefsGold).Background(chiefsRed).Bold(true),
		normal:       lipgloss.NewStyle().Foreground(lipgloss.Color("252")),
		vault:        lipgloss.NewStyle().Foreground(chiefsGold),
		masked:       lipgloss.NewStyle().Foreground(muted),
		revealed:     lipgloss.NewStyle().Foreground(chiefsRed),
		banner:       lipgloss.NewStyle().Foreground(chiefsGold).Bold(true),
		loading:      lipgloss.NewStyle().Foreground(muted).Italic(true),
		flash:        lipgloss.NewStyle().Foreground(lipgloss.Color("#4CAF50")).Bold(true),
		welcome:      lipgloss.NewStyle().Padding(2).Align(lipgloss.Center),
		welcomeTitle: lipgloss.NewStyle().Foreground(chiefsGold).Bold(true),
		welcomeKey:   lipgloss.NewStyle().Foreground(chiefsGold).Bold(true),
		welcomeDesc:  lipgloss.NewStyle().Foreground(muted),
		prefix:       lipgloss.NewStyle().Padding(0, 1).MarginRight(1).Bold(true),
		borderRed:    lipgloss.NewStyle().Foreground(chiefsRed),
		borderGold:   lipgloss.NewStyle().Foreground(chiefsGold),
	}
}

func (s styles) Prefix(name string) lipgloss.Style {
	colors := []string{"#FFB81C", "#E31837", "#4CAF50", "#2196F3", "#9C27B0", "#FF9800"}
	sum := 0
	for _, c := range name {
		sum += int(c)
	}
	color := colors[sum%len(colors)]
	return s.prefix.Foreground(lipgloss.Color(color))
}
