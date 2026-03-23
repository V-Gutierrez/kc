package tui

import (
	"fmt"
	"io"

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
	if index == m.Index() {
		titleStyle = d.styles.selected
	}

	masked := d.styles.masked.Render(maskedValue(item, d.model.preview))
	line1 := titleStyle.Render(item.Key)
	line2 := d.styles.vault.Render(item.Vault) + "  " + masked
	fmt.Fprint(w, line1+"\n"+line2)
}
