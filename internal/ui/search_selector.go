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

// searchItem implements list.Item for a catalog search result.
type searchItem struct {
	sr    api.SearchResult
	index int
}

func (s searchItem) FilterValue() string { return s.sr.Title }

type searchItemDelegate struct {
	styles *Styles
}

func (d searchItemDelegate) Height() int     { return 1 }
func (d searchItemDelegate) Spacing() int    { return 0 }
func (d searchItemDelegate) Update(_ tea.Msg, _ *list.Model) tea.Cmd {
	return nil
}
func (d searchItemDelegate) Render(w io.Writer, m list.Model, index int, item list.Item) {
	si, ok := item.(searchItem)
	if !ok {
		return
	}
	title := si.sr.Title
	if title == "" {
		title = "?"
	}

	authors := ""
	if len(si.sr.AuthorNames) > 0 {
		names := si.sr.AuthorNames
		if len(names) > 2 {
			names = names[:2]
			authors = " — " + JoinStrings(names, ", ") + "…"
		} else {
			authors = " — " + JoinStrings(names, ", ")
		}
	}

	meta := []string{}
	if si.sr.Pages != nil && *si.sr.Pages > 0 {
		meta = append(meta, fmt.Sprintf("%dp", *si.sr.Pages))
	}
	if si.sr.ReleaseYear != nil && *si.sr.ReleaseYear > 0 {
		meta = append(meta, strconv.Itoa(*si.sr.ReleaseYear))
	}
	if si.sr.Rating > 0 {
		meta = append(meta, fmt.Sprintf("★ %.1f", si.sr.Rating))
	}

	row := fmt.Sprintf("%2d. %s%s", si.index+1, title, authors)
	if len(meta) > 0 {
		row += "  " + JoinStrings(meta, "  •  ")
	}

	cursor := "  "
	if index == m.Index() {
		cursor = "▸ "
		row = d.styles.Apply(d.styles.Bold, row)
	}
	fmt.Fprint(w, cursor+row)
}

// searchSelectorModel is the bubbletea model for the search result picker.
type searchSelectorModel struct {
	list     list.Model
	styles   *Styles
	choice   *api.SearchResult
	quitting bool
	canceled bool
}

func initialSearchSelectorModel(results []api.SearchResult, styles *Styles) searchSelectorModel {
	items := make([]list.Item, len(results))
	for i, r := range results {
		items[i] = searchItem{sr: r, index: i}
	}
	l := list.New(items, searchItemDelegate{styles: styles}, 80, 14)
	l.Title = "Select a book"
	l.SetShowStatusBar(false)
	l.SetFilteringEnabled(true)
	l.SetShowHelp(true)
	return searchSelectorModel{list: l, styles: styles}
}

func (m searchSelectorModel) Init() tea.Cmd { return nil }

func (m searchSelectorModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
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
			si, ok := m.list.SelectedItem().(searchItem)
			if ok {
				m.choice = &si.sr
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

func (m searchSelectorModel) View() string {
	if m.quitting {
		return "\r\033[K"
	}
	return m.list.View()
}

// SelectSearchResult runs an interactive picker for catalog search results.
// Returns (nil, nil) if the user cancelled.
func SelectSearchResult(ctx context.Context, results []api.SearchResult, styles *Styles) (*api.SearchResult, error) {
	if len(results) == 0 {
		return nil, fmt.Errorf("no results to select from")
	}
	if len(results) == 1 || !IsInteractive() {
		r := results[0]
		return &r, nil
	}
	m := initialSearchSelectorModel(results, styles)
	p := tea.NewProgram(m, tea.WithContext(ctx), tea.WithAltScreen(), tea.WithOutput(stderrOrStdout()))
	finalModel, err := p.Run()
	if err != nil {
		return nil, fmt.Errorf("run selector: %w", err)
	}
	fm, ok := finalModel.(searchSelectorModel)
	if !ok {
		return nil, fmt.Errorf("unexpected model type")
	}
	if fm.canceled || fm.choice == nil {
		return nil, nil
	}
	return fm.choice, nil
}
