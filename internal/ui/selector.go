package ui

import (
	"context"
	"fmt"
	"io"
	"os"

	"github.com/KIRKR101/hardcover-cli/internal/api"

	"github.com/charmbracelet/bubbles/list"
	tea "github.com/charmbracelet/bubbletea"
)

// stderrOrStdout returns stdout if it's a terminal, otherwise stderr.
// This keeps the selector from polluting piped output.
func stderrOrStdout() *os.File {
	if isatty(os.Stdout) {
		return os.Stdout
	}
	return os.Stderr
}

// bookItem implements list.Item for a user_book row.
type bookItem struct {
	ub     api.UserBook
	index  int
}

func (b bookItem) FilterValue() string { return b.ub.Book.Title }

type bookItemDelegate struct {
	styles *Styles
}

func (d bookItemDelegate) Height() int { return 1 }
func (d bookItemDelegate) Spacing() int { return 0 }
func (d bookItemDelegate) Update(_ tea.Msg, _ *list.Model) tea.Cmd {
	return nil
}
func (d bookItemDelegate) Render(w io.Writer, m list.Model, index int, item list.Item) {
	bi, ok := item.(bookItem)
	if !ok {
		return
	}
	title := bi.ub.Book.Title
	if title == "" {
		title = "?"
	}
	authors := ""
	if len(bi.ub.Book.Contributions) > 0 {
		names := make([]string, 0, len(bi.ub.Book.Contributions))
		for _, c := range bi.ub.Book.Contributions {
			if c.Author.Name != "" {
				names = append(names, c.Author.Name)
			}
		}
		if len(names) > 0 {
			if len(names) > 2 {
				names = names[:2]
				authors = " — " + JoinStrings(names, ", ") + "…"
			} else {
				authors = " — " + JoinStrings(names, ", ")
			}
		}
	}
	status := StatusName(bi.ub.StatusID)
	cursor := "  "
	row := fmt.Sprintf("%2d. %s%s  [%s]", bi.index+1, title, authors, status)
	if index == m.Index() {
		cursor = "▸ "
		row = d.styles.Apply(d.styles.Bold, row)
	}
	fmt.Fprint(w, cursor+row)
}

// selectorModel is the bubbletea model for the book picker.
type selectorModel struct {
	list     list.Model
	styles   *Styles
	choice   *api.UserBook
	quitting bool
	canceled bool
}

func initialSelectorModel(books []api.UserBook, styles *Styles) selectorModel {
	items := make([]list.Item, len(books))
	for i, b := range books {
		items[i] = bookItem{ub: b, index: i}
	}
	l := list.New(items, bookItemDelegate{styles: styles}, 80, 14)
	l.Title = "Select a book"
	l.SetShowStatusBar(false)
	l.SetFilteringEnabled(true)
	l.SetShowHelp(true)
	return selectorModel{list: l, styles: styles}
}

func (m selectorModel) Init() tea.Cmd { return nil }

// Update handles key events. CRITICAL: bubbles/list has its own filter
// mode and intercepts some keys (notably esc) for filter clearing. We
// make sure ctrl-c always quits, regardless of whether the list is
// in filtering mode, so cancellation works as expected.
func (m selectorModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
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
			// If we're filtering, let the list handle it (clear filter).
			// Otherwise, treat esc as cancel.
			if m.list.FilterState() == list.Filtering {
				break
			}
			m.canceled = true
			m.quitting = true
			return m, tea.Quit
		case "enter":
			if m.list.FilterState() == list.Filtering {
				// Let the list accept the filter.
				break
			}
			bi, ok := m.list.SelectedItem().(bookItem)
			if ok {
				m.choice = &bi.ub
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

func (m selectorModel) View() string {
	if m.quitting {
		return "\r\033[K"
	}
	return m.list.View()
}

// SelectBook runs an interactive picker and returns the chosen
// user_book. The context is forwarded to the bubbletea program so
// Ctrl-C cancels cleanly. Returns (nil, nil) if the user cancelled.
//
// In a non-TTY environment (e.g. piped stdin), SelectBook falls
// back to returning the first book without launching a picker.
func SelectBook(ctx context.Context, books []api.UserBook, styles *Styles) (*api.UserBook, error) {
	if len(books) == 0 {
		return nil, fmt.Errorf("no books to select from")
	}
	if len(books) == 1 || !IsInteractive() {
		b := books[0]
		return &b, nil
	}
	m := initialSelectorModel(books, styles)
	p := tea.NewProgram(m, tea.WithContext(ctx), tea.WithAltScreen(), tea.WithOutput(stderrOrStdout()))
	finalModel, err := p.Run()
	if err != nil {
		return nil, fmt.Errorf("run selector: %w", err)
	}
	fm, ok := finalModel.(selectorModel)
	if !ok {
		return nil, fmt.Errorf("unexpected model type")
	}
	if fm.canceled || fm.choice == nil {
		return nil, nil
	}
	return fm.choice, nil
}
