package tui

import (
	"strings"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/link-fgfgui/mod-downloader-cli/tui/i18n"
)

// Modal is one entry in the global modal/drawer stack.
type Modal interface {
	// ID is a stable identifier used for de-duplication and focus routing.
	ID() string
	// Update handles a message. It may mutate the AppModel and return a command.
	Update(app *AppModel, msg tea.Msg) tea.Cmd
	// View renders the modal body (already sized to the app viewport).
	View(app *AppModel) string
	// CapturesInput reports whether the modal swallows global keys (1..6, q, Esc).
	CapturesInput() bool
	// PreferredWidth returns a desired width; <= 0 means auto/full.
	PreferredWidth() int
}

// modalStack manages ordering and focus.
type modalStack []Modal

func (s modalStack) top() Modal {
	if len(s) == 0 {
		return nil
	}
	return s[len(s)-1]
}

func (s modalStack) has(id string) bool {
	for _, m := range s {
		if m.ID() == id {
			return true
		}
	}
	return false
}

// push adds a modal unless one with the same id already exists.
func (s *modalStack) push(m Modal) {
	if m == nil || s.has(m.ID()) {
		return
	}
	*s = append(*s, m)
}

func (s *modalStack) pop() {
	if len(*s) == 0 {
		return
	}
	*s = (*s)[:len(*s)-1]
}

// --- choice (confirmation) modal ---------------------------------------------

type choiceOption struct {
	Label       string
	Value       string
	Destructive bool
	Primary     bool
}

type choiceModal struct {
	id       string
	title    string
	body     []string
	options  []choiceOption
	cursor   int
	onSelect func(app *AppModel, value string) tea.Cmd
	onCancel func(app *AppModel) tea.Cmd
}

func (m *choiceModal) ID() string          { return m.id }
func (m *choiceModal) CapturesInput() bool { return true }
func (m *choiceModal) PreferredWidth() int { return 64 }

func newConfirmModal(id, title, body string, options []choiceOption, onSelect func(*AppModel, string) tea.Cmd) *choiceModal {
	lines := []string{}
	if body != "" {
		lines = strings.Split(body, "\n")
	}
	return &choiceModal{id: id, title: title, body: lines, options: options, onSelect: onSelect}
}

func (m *choiceModal) Update(app *AppModel, msg tea.Msg) tea.Cmd {
	key, ok := msg.(tea.KeyMsg)
	if !ok {
		return nil
	}
	switch key.String() {
	case "left", "h", "shift+tab":
		if m.cursor > 0 {
			m.cursor--
		}
	case "right", "l", "tab":
		if m.cursor < len(m.options)-1 {
			m.cursor++
		}
	case "enter":
		if len(m.options) == 0 {
			return nil
		}
		value := m.options[m.cursor].Value
		app.popModal(m.id)
		if m.onSelect != nil {
			return m.onSelect(app, value)
		}
	case "esc":
		app.popModal(m.id)
		if m.onCancel != nil {
			return m.onCancel(app)
		}
	}
	return nil
}

func (m *choiceModal) View(app *AppModel) string {
	t := app.theme
	var b strings.Builder
	b.WriteString(t.Title().Render(m.title))
	b.WriteString("\n")
	for _, line := range m.body {
		b.WriteString(t.OnSurface().Render(line))
		b.WriteString("\n")
	}
	b.WriteString("\n")
	var buttons []string
	for i, opt := range m.options {
		style := t.Muted()
		if opt.Destructive {
			style = t.Danger()
		} else if opt.Primary {
			style = t.Accent()
		}
		label := "[ " + opt.Label + " ]"
		if i == m.cursor {
			label = t.Selected().Render(" " + opt.Label + " ")
		} else {
			label = style.Render(label)
		}
		buttons = append(buttons, label)
	}
	b.WriteString(strings.Join(buttons, "  "))
	return b.String()
}

// --- prompt modal -------------------------------------------------------------

type promptModal struct {
	id        string
	title     string
	hint      string
	input     textinput.Model
	onSubmit  func(app *AppModel, value string) tea.Cmd
	multiline bool
}

func newPromptModal(id, title, hint, initial, placeholder string, onSubmit func(*AppModel, string) tea.Cmd) *promptModal {
	ti := textinput.New()
	ti.SetValue(initial)
	ti.Placeholder = placeholder
	ti.Width = 48
	ti.Focus()
	return &promptModal{id: id, title: title, hint: hint, input: ti, onSubmit: onSubmit}
}

func (m *promptModal) ID() string          { return m.id }
func (m *promptModal) CapturesInput() bool { return true }
func (m *promptModal) PreferredWidth() int { return 64 }

func (m *promptModal) Update(app *AppModel, msg tea.Msg) tea.Cmd {
	if key, ok := msg.(tea.KeyMsg); ok {
		switch key.String() {
		case "enter":
			value := strings.TrimSpace(m.input.Value())
			app.popModal(m.id)
			if m.onSubmit != nil {
				return m.onSubmit(app, value)
			}
			return nil
		case "esc":
			app.popModal(m.id)
			return nil
		}
	}
	var cmd tea.Cmd
	m.input, cmd = m.input.Update(msg)
	return cmd
}

func (m *promptModal) View(app *AppModel) string {
	t := app.theme
	var b strings.Builder
	b.WriteString(t.Title().Render(m.title))
	b.WriteString("\n")
	if m.hint != "" {
		b.WriteString(t.Muted().Render(m.hint))
		b.WriteString("\n")
	}
	b.WriteString("\n")
	b.WriteString(m.input.View())
	b.WriteString("\n\n")
	b.WriteString(t.Muted().Render(uiText("enter") + " ↵  ·  " + uiText("esc") + " esc"))
	return b.String()
}

// --- select-list modal --------------------------------------------------------

type selectItem struct {
	Label    string
	Value    string
	Detail   string
	Disabled bool
	Badge    string
}

type selectModal struct {
	id          string
	title       string
	items       []selectItem
	cursor      int
	offset      int
	height      int
	onSelect    func(app *AppModel, item selectItem) tea.Cmd
	allowCancel bool
}

func newSelectModal(id, title string, items []selectItem, onSelect func(*AppModel, selectItem) tea.Cmd) *selectModal {
	return &selectModal{id: id, title: title, items: items, onSelect: onSelect, allowCancel: true, height: 12}
}

func (m *selectModal) ID() string          { return m.id }
func (m *selectModal) CapturesInput() bool { return true }
func (m *selectModal) PreferredWidth() int { return 72 }

func (m *selectModal) setItems(items []selectItem) {
	m.items = items
	if m.cursor >= len(items) {
		m.cursor = maxInt(0, len(items)-1)
	}
}

func (m *selectModal) moveCursor(delta int) {
	if len(m.items) == 0 {
		return
	}
	m.cursor += delta
	if m.cursor < 0 {
		m.cursor = 0
	}
	if m.cursor >= len(m.items) {
		m.cursor = len(m.items) - 1
	}
	m.clampOffset()
}

func (m *selectModal) clampOffset() {
	if m.height <= 0 {
		return
	}
	if m.cursor < m.offset {
		m.offset = m.cursor
	}
	if m.cursor >= m.offset+m.height {
		m.offset = m.cursor - m.height + 1
	}
	if m.offset < 0 {
		m.offset = 0
	}
}

func (m *selectModal) Update(app *AppModel, msg tea.Msg) tea.Cmd {
	key, ok := msg.(tea.KeyMsg)
	if !ok {
		return nil
	}
	switch key.String() {
	case "up", "k":
		m.moveCursor(-1)
	case "down", "j":
		m.moveCursor(1)
	case "pgup":
		m.moveCursor(-m.height)
	case "pgdown":
		m.moveCursor(m.height)
	case "home":
		m.cursor = 0
		m.clampOffset()
	case "end":
		if len(m.items) > 0 {
			m.cursor = len(m.items) - 1
			m.clampOffset()
		}
	case "enter":
		if len(m.items) == 0 {
			return nil
		}
		item := m.items[m.cursor]
		if item.Disabled {
			return nil
		}
		app.popModal(m.id)
		if m.onSelect != nil {
			return m.onSelect(app, item)
		}
	case "esc":
		if m.allowCancel {
			app.popModal(m.id)
		}
	}
	return nil
}

func (m *selectModal) View(app *AppModel) string {
	t := app.theme
	var b strings.Builder
	b.WriteString(t.Title().Render(m.title))
	b.WriteString("\n\n")
	if len(m.items) == 0 {
		b.WriteString(t.Muted().Render(i18n.T("manage.noMods")))
		return b.String()
	}
	end := m.offset + m.height
	if end > len(m.items) {
		end = len(m.items)
	}
	for i := m.offset; i < end; i++ {
		item := m.items[i]
		cursor := "  "
		if i == m.cursor {
			cursor = "▶ "
		}
		line := cursor + item.Label
		if item.Badge != "" {
			line += "  " + t.Warning().Render(item.Badge)
		}
		if item.Disabled {
			line = t.Muted().Render(line)
		} else if i == m.cursor {
			line = t.Selected().Render(line)
		} else {
			line = t.OnSurface().Render(line)
		}
		b.WriteString(line)
		if item.Detail != "" {
			b.WriteString("  ")
			b.WriteString(t.Muted().Render(item.Detail))
		}
		b.WriteString("\n")
	}
	if len(m.items) > m.height {
		b.WriteString(t.Muted().Render(itoa(m.offset+1) + "-" + itoa(end) + " / " + itoa(len(m.items))))
	}
	return b.String()
}

// --- helpers ------------------------------------------------------------------

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}

func itoa(v int) string {
	if v == 0 {
		return "0"
	}
	neg := v < 0
	if neg {
		v = -v
	}
	buf := [20]byte{}
	i := len(buf)
	for v > 0 {
		i--
		buf[i] = byte('0' + v%10)
		v /= 10
	}
	if neg {
		i--
		buf[i] = '-'
	}
	return string(buf[i:])
}

func centerOverlay(app *AppModel, modal Modal, body string) string {
	width := modal.PreferredWidth()
	if width <= 0 {
		width = app.width - 8
	}
	if width > app.width-2 {
		width = app.width - 2
	}
	if width < 24 {
		width = maxInt(24, app.width-2)
	}
	return app.theme.Overlay().Width(width).Render(body)
}

var _ = lipgloss.NewStyle
