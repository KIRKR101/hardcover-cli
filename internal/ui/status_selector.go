package ui

import (
	"context"
	"fmt"
	"io"

	"github.com/charmbracelet/bubbles/list"
	tea "github.com/charmbracelet/bubbletea"
)

// statusItem implements list.Item for a reading status.
type statusItem struct {
	statusID int
	name     string
	index    int
}

func (s statusItem) FilterValue() string { return s.name }

type statusItemDelegate struct {
	styles *Styles
}

func (d statusItemDelegate) Height() int     { return 1 }
func (d statusItemDelegate) Spacing() int    { return 0 }
func (d statusItemDelegate) Update(_ tea.Msg, _ *list.Model) tea.Cmd {
	return nil
}
func (d statusItemDelegate) Render(w io.Writer, m list.Model, index int, item list.Item) {
	si, ok := item.(statusItem)
	if !ok {
		return
	}
	cursor := "  "
	row := fmt.Sprintf("%2d. %s", si.index+1, si.name)
	if index == m.Index() {
		cursor = "▸ "
		row = d.styles.Apply(d.styles.Bold, row)
	}
	fmt.Fprint(w, cursor+row)
}

// statusSelectorModel is the bubbletea model for the status picker.
type statusSelectorModel struct {
	list     list.Model
	styles   *Styles
	choice   *statusItem
	quitting bool
	canceled bool
}

var allStatuses = []statusItem{
	{statusID: 1, name: "Want to Read"},
	{statusID: 2, name: "Currently Reading"},
	{statusID: 3, name: "Read"},
	{statusID: 4, name: "Paused"},
	{statusID: 5, name: "Did Not Finish"},
	{statusID: 6, name: "Ignored"},
}

func initialStatusSelectorModel(styles *Styles) statusSelectorModel {
	items := make([]list.Item, len(allStatuses))
	for i, s := range allStatuses {
		items[i] = statusItem{statusID: s.statusID, name: s.name, index: i}
	}
	l := list.New(items, statusItemDelegate{styles: styles}, 40, 10)
	l.Title = "Set status"
	l.SetShowStatusBar(false)
	l.SetFilteringEnabled(false)
	l.SetShowHelp(true)
	return statusSelectorModel{list: l, styles: styles}
}

func (m statusSelectorModel) Init() tea.Cmd { return nil }

func (m statusSelectorModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.list.SetSize(msg.Width, msg.Height-2)
		return m, nil
	case tea.KeyMsg:
		switch msg.String() {
		case "ctrl+c":
			m.canceled = true
			m.quitting = true
			return m, tea.Quit
		case "esc":
			m.canceled = true
			m.quitting = true
			return m, tea.Quit
		case "enter":
			si, ok := m.list.SelectedItem().(statusItem)
			if ok {
				m.choice = &si
			}
			m.quitting = true
			return m, tea.Quit
		case "q":
			m.canceled = true
			m.quitting = true
			return m, tea.Quit
		}
	}
	var cmd tea.Cmd
	m.list, cmd = m.list.Update(msg)
	return m, cmd
}

func (m statusSelectorModel) View() string {
	if m.quitting {
		return "\r\033[K"
	}
	return m.list.View()
}

// SelectStatus runs an interactive picker for reading statuses.
// Returns the chosen status ID, or (0, nil) if the user cancelled.
// In non-interactive mode, returns 1 ("Want to Read") without prompting.
func SelectStatus(ctx context.Context, styles *Styles) (int, error) {
	if !IsInteractive() {
		return 1, nil
	}
	m := initialStatusSelectorModel(styles)
	p := tea.NewProgram(m, tea.WithContext(ctx), tea.WithAltScreen(), tea.WithOutput(stderrOrStdout()))
	finalModel, err := p.Run()
	if err != nil {
		return 0, fmt.Errorf("run selector: %w", err)
	}
	fm, ok := finalModel.(statusSelectorModel)
	if !ok {
		return 0, fmt.Errorf("unexpected model type")
	}
	if fm.canceled || fm.choice == nil {
		return 0, nil
	}
	return fm.choice.statusID, nil
}
