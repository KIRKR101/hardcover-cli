package ui

import (
	"context"
	"fmt"
	"io"
	"strconv"

	"github.com/KIRKR101/hardcover-cli/internal/api"

	"github.com/charmbracelet/bubbles/list"
	tea "github.com/charmbracelet/bubbletea"
)

// editionItem implements list.Item for an edition result.
type editionItem struct {
	ed    api.EditionResult
	index int
}

func (e editionItem) FilterValue() string {
	if e.ed.Title != nil {
		return *e.ed.Title
	}
	return ""
}

type editionItemDelegate struct {
	styles *Styles
}

func (d editionItemDelegate) Height() int     { return 1 }
func (d editionItemDelegate) Spacing() int    { return 0 }
func (d editionItemDelegate) Update(_ tea.Msg, _ *list.Model) tea.Cmd {
	return nil
}
func (d editionItemDelegate) Render(w io.Writer, m list.Model, index int, item list.Item) {
	ei, ok := item.(editionItem)
	if !ok {
		return
	}

	format := ""
	if ei.ed.ReadingFormat != nil && ei.ed.ReadingFormat.Format != "" {
		format = ei.ed.ReadingFormat.Format
	} else if ei.ed.PhysicalFormat != nil && *ei.ed.PhysicalFormat != "" {
		format = *ei.ed.PhysicalFormat
	} else if ei.ed.EditionFormat != nil && *ei.ed.EditionFormat != "" {
		format = *ei.ed.EditionFormat
	}

	meta := []string{}
	if format != "" {
		meta = append(meta, format)
	}
	if ei.ed.Pages != nil && *ei.ed.Pages > 0 {
		meta = append(meta, fmt.Sprintf("%dp", *ei.ed.Pages))
	}
	if ei.ed.ReleaseYear != nil && *ei.ed.ReleaseYear > 0 {
		meta = append(meta, strconv.Itoa(*ei.ed.ReleaseYear))
	}
	if ei.ed.Publisher != nil && ei.ed.Publisher.Name != "" {
		meta = append(meta, ei.ed.Publisher.Name)
	}
	if ei.ed.Language != nil && ei.ed.Language.Language != "" {
		meta = append(meta, ei.ed.Language.Language)
	}

	row := fmt.Sprintf("%2d.", ei.index+1)
	if len(meta) > 0 {
		row += " " + JoinStrings(meta, "  •  ")
	}

	cursor := "  "
	if index == m.Index() {
		cursor = "▸ "
		row = d.styles.Apply(d.styles.Bold, row)
	}
	fmt.Fprint(w, cursor+row)
}

// editionSelectorModel is the bubbletea model for the edition picker.
type editionSelectorModel struct {
	list     list.Model
	styles   *Styles
	choice   *api.EditionResult
	quitting bool
	canceled bool
}

func initialEditionSelectorModel(editions []api.EditionResult, styles *Styles) editionSelectorModel {
	items := make([]list.Item, len(editions))
	for i, ed := range editions {
		items[i] = editionItem{ed: ed, index: i}
	}
	l := list.New(items, editionItemDelegate{styles: styles}, 80, 14)
	l.Title = "Select an edition"
	l.SetShowStatusBar(false)
	l.SetFilteringEnabled(true)
	l.SetShowHelp(true)
	return editionSelectorModel{list: l, styles: styles}
}

func (m editionSelectorModel) Init() tea.Cmd { return nil }

func (m editionSelectorModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
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
			if m.list.FilterState() == list.Filtering {
				break
			}
			m.canceled = true
			m.quitting = true
			return m, tea.Quit
		case "enter":
			if m.list.FilterState() == list.Filtering {
				break
			}
			ei, ok := m.list.SelectedItem().(editionItem)
			if ok {
				m.choice = &ei.ed
			}
			m.quitting = true
			return m, tea.Quit
		case "q":
			if m.list.FilterState() != list.Filtering {
				m.canceled = true
				m.quitting = true
				return m, tea.Quit
			}
		}
	}
	var cmd tea.Cmd
	m.list, cmd = m.list.Update(msg)
	return m, cmd
}

func (m editionSelectorModel) View() string {
	if m.quitting {
		return "\r\033[K"
	}
	return m.list.View()
}

// SelectEdition runs an interactive picker for book editions.
// If there is only one edition it is returned directly. Returns
// (nil, nil) if the user cancelled.
func SelectEdition(ctx context.Context, editions []api.EditionResult, styles *Styles) (*api.EditionResult, error) {
	if len(editions) == 0 {
		return nil, fmt.Errorf("no editions to select from")
	}
	if len(editions) == 1 || !IsInteractive() {
		e := editions[0]
		return &e, nil
	}
	m := initialEditionSelectorModel(editions, styles)
	p := tea.NewProgram(m, tea.WithContext(ctx), tea.WithAltScreen(), tea.WithOutput(stderrOrStdout()))
	finalModel, err := p.Run()
	if err != nil {
		return nil, fmt.Errorf("run selector: %w", err)
	}
	fm, ok := finalModel.(editionSelectorModel)
	if !ok {
		return nil, fmt.Errorf("unexpected model type")
	}
	if fm.canceled || fm.choice == nil {
		return nil, nil
	}
	return fm.choice, nil
}
