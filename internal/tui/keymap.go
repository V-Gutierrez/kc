package tui

import "github.com/charmbracelet/bubbles/key"

type keyMap struct {
	Up        key.Binding
	Down      key.Binding
	Top       key.Binding
	Bottom    key.Binding
	Search    key.Binding
	Copy      key.Binding
	Add       key.Binding
	Edit      key.Binding
	Delete    key.Binding
	VaultNext key.Binding
	Confirm   key.Binding
	Cancel    key.Binding
	Quit      key.Binding
	Help      key.Binding
}

func defaultKeyMap() keyMap {
	return keyMap{
		Up:        key.NewBinding(key.WithKeys("up", "k"), key.WithHelp("↑/k", "up")),
		Down:      key.NewBinding(key.WithKeys("down", "j"), key.WithHelp("↓/j", "down")),
		Top:       key.NewBinding(key.WithKeys("g", "home"), key.WithHelp("g", "top")),
		Bottom:    key.NewBinding(key.WithKeys("G", "end"), key.WithHelp("G", "bottom")),
		Search:    key.NewBinding(key.WithKeys("/"), key.WithHelp("/", "search")),
		Copy:      key.NewBinding(key.WithKeys("c"), key.WithHelp("c", "copy")),
		Add:       key.NewBinding(key.WithKeys("a"), key.WithHelp("a", "add")),
		Edit:      key.NewBinding(key.WithKeys("e"), key.WithHelp("e", "edit")),
		Delete:    key.NewBinding(key.WithKeys("d"), key.WithHelp("d", "delete")),
		VaultNext: key.NewBinding(key.WithKeys("v", "tab"), key.WithHelp("v/tab", "vault")),
		Confirm:   key.NewBinding(key.WithKeys("enter"), key.WithHelp("enter", "confirm")),
		Cancel:    key.NewBinding(key.WithKeys("esc"), key.WithHelp("esc", "cancel")),
		Quit:      key.NewBinding(key.WithKeys("q", "ctrl+c"), key.WithHelp("q", "quit")),
		Help:      key.NewBinding(key.WithKeys("?"), key.WithHelp("?", "help")),
	}
}

func (k keyMap) ShortHelp() []key.Binding {
	return []key.Binding{k.Search, k.Copy, k.Add, k.Edit, k.Delete, k.VaultNext, k.Quit}
}
