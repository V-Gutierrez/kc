package tui

import (
	"fmt"
	"io"
	"strings"

	"github.com/charmbracelet/bubbles/list"
	tea "github.com/charmbracelet/bubbletea"
)

type itemDelegate struct {
	styles *styles
	model  *Model
}

func (d itemDelegate) Height() int                             { return 2 }
func (d itemDelegate) Spacing() int                            { return 0 }
func (d itemDelegate) Update(_ tea.Msg, _ *list.Model) tea.Cmd { return nil }

func (d itemDelegate) Render(w io.Writer, m list.Model, index int, listItem list.Item) {
	item, ok := listItem.(entry)
	if !ok {
		return
	}

	titleStyle := d.styles.normal
	if index%2 == 1 && index != m.Index() {
		titleStyle = titleStyle.Faint(true)
	}
	if index == m.Index() {
		titleStyle = d.styles.selected
	}

	prefix := item.prefix()
	prefixLabel := d.styles.Prefix(prefix).Render(strings.ToUpper(prefix))

	masked := d.styles.masked.Render(maskedValue(item, d.model.preview))
	line1 := prefixLabel + " " + titleStyle.Render(item.Key)
	line2 := d.styles.vault.Render(item.Vault) + "  " + masked
	row := line1 + "\n" + line2
	if index%2 == 1 && index != m.Index() {
		row = d.styles.rowAlt.Render(row)
	}
	fmt.Fprint(w, row)
}
