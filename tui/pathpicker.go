package tui

import (
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/charmbracelet/bubbles/filepicker"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"

	"github.com/link-fgfgui/mod-downloader-cli/tui/i18n"
)

// pathPickerMode selects what the picker accepts.
type pathPickerMode int

const (
	pickerDirectory pathPickerMode = iota
	pickerSaveFile
)

// pathPickerModal replaces the three Wails native-dialog methods
// (ChooseMinecraftDir / ChooseCacheDir / ExportFavoriteListPackwizZip). It
// combines a typed path input with a built-in directory/file tree and Tab-based
// path completion.
type pathPickerModal struct {
	id          string
	title       string
	mode        pathPickerMode
	input       textinput.Model
	picker      filepicker.Model
	focusIn     bool
	completions []string
	completeIdx int
	errMsg      string
	onAccept    func(app *AppModel, path string) tea.Cmd
	onCancel    func(app *AppModel) tea.Cmd
}

func newPathPickerModal(id, title string, mode pathPickerMode, start string, onAccept func(*AppModel, string) tea.Cmd) *pathPickerModal {
	dir := start
	if dir == "" {
		dir, _ = os.UserHomeDir()
	}
	if mode == pickerSaveFile {
		if info, err := os.Stat(dir); err != nil || !info.IsDir() {
			dir = filepath.Dir(dir)
		}
	}

	fp := filepicker.New()
	fp.DirAllowed = mode == pickerDirectory
	fp.FileAllowed = mode == pickerSaveFile
	fp.ShowHidden = true
	fp.AutoHeight = false
	fp.CurrentDirectory = dir
	if mode == pickerSaveFile {
		fp.AllowedTypes = []string{".zip"}
	}
	fp.SetHeight(10)

	ti := textinput.New()
	ti.Placeholder = uiText("typePath")
	ti.Width = 56
	if mode == pickerSaveFile {
		ti.SetValue(filepath.Join(dir, filepath.Base(start)))
	}

	m := &pathPickerModal{id: id, title: title, mode: mode, input: ti, picker: fp, focusIn: true}
	m.input.Focus()
	return m
}

func (m *pathPickerModal) ID() string          { return m.id }
func (m *pathPickerModal) CapturesInput() bool { return true }
func (m *pathPickerModal) PreferredWidth() int { return 72 }

// Init returns the filepicker directory-read command.
func (m *pathPickerModal) Init() tea.Cmd { return m.picker.Init() }

func (m *pathPickerModal) Update(app *AppModel, msg tea.Msg) tea.Cmd {
	if key, ok := msg.(tea.KeyMsg); ok {
		switch key.String() {
		case "esc":
			// Esc always closes the picker (mirrors the GUI's cancel contract);
			// use shift+tab to move focus between the input and the tree.
			app.popModal(m.id)
			if m.onCancel != nil {
				return m.onCancel(app)
			}
			return nil
		case "shift+tab":
			m.focusInput(!m.focusIn)
			return nil
		case "tab":
			if m.focusIn {
				m.cycleCompletion()
				return nil
			}
		case "ctrl+s", "ctrl+y":
			return m.accept(app)
		case "enter":
			if m.focusIn {
				return m.accept(app)
			}
		}
	}

	if m.focusIn {
		var cmd tea.Cmd
		m.input, cmd = m.input.Update(msg)
		m.completions = nil
		m.completeIdx = 0
		m.errMsg = ""
		return cmd
	}

	var cmd tea.Cmd
	m.picker, cmd = m.picker.Update(msg)
	if didSelect, path := m.picker.DidSelectFile(msg); didSelect {
		if m.mode == pickerDirectory {
			m.input.SetValue(path)
			return nil
		}
		m.input.SetValue(path)
		return m.accept(app)
	}
	if didSelect, _ := m.picker.DidSelectDisabledFile(msg); didSelect {
		m.errMsg = i18nUnsupportedType(m.mode)
	}
	return cmd
}

func (m *pathPickerModal) focusInput(focus bool) {
	m.focusIn = focus
	if focus {
		m.input.Focus()
	} else {
		m.input.Blur()
	}
}

// cycleCompletion implements Tab path completion: the first Tab builds the
// candidate list, subsequent Tabs cycle through it.
func (m *pathPickerModal) cycleCompletion() {
	value := m.input.Value()
	if len(m.completions) == 0 {
		m.completions = completePath(value)
		m.completeIdx = 0
		if len(m.completions) == 0 {
			m.errMsg = uiText("none")
			return
		}
	} else {
		m.completeIdx = (m.completeIdx + 1) % len(m.completions)
	}
	m.input.SetValue(m.completions[m.completeIdx])
	m.input.CursorEnd()
	m.errMsg = ""
}

func (m *pathPickerModal) accept(app *AppModel) tea.Cmd {
	path := strings.TrimSpace(m.input.Value())
	if path == "" {
		path = m.picker.Path
	}
	if path == "" {
		path = m.picker.CurrentDirectory
	}
	if path == "" {
		m.errMsg = uiText("unknown")
		return nil
	}
	// Expand a bare "~" and environment variables like the core does.
	path = expandUser(path)
	if m.mode == pickerSaveFile {
		if filepath.Ext(path) == "" {
			path += ".zip"
		}
	}
	app.popModal(m.id)
	if m.onAccept != nil {
		return m.onAccept(app, path)
	}
	return nil
}

func (m *pathPickerModal) View(app *AppModel) string {
	t := app.theme
	var b strings.Builder
	b.WriteString(t.Title().Render(m.title))
	b.WriteString("\n\n")

	inputLabel := "  "
	if m.focusIn {
		inputLabel = "› "
	}
	b.WriteString(t.Muted().Render(inputLabel))
	b.WriteString(m.input.View())
	b.WriteString("\n\n")

	if !m.focusIn {
		b.WriteString(m.picker.View())
	} else {
		b.WriteString(t.Muted().Render(m.picker.CurrentDirectory))
	}
	b.WriteString("\n\n")

	if m.errMsg != "" {
		b.WriteString(t.Danger().Render(m.errMsg))
		b.WriteString("\n")
	}
	hint := "[ " + uiText("confirm") + " ctrl+s / enter ]  [ " + uiText("cancel") + " esc ]  [ ⇥ " + uiText("navigate") + " shift+tab ]"
	b.WriteString(t.Muted().Render(hint))
	return b.String()
}

// completePath returns directory-entry completions for the final segment of the
// typed path, preserving the leading directory part.
func completePath(value string) []string {
	value = expandUser(value)
	dir := value
	base := ""
	if !strings.HasSuffix(value, string(os.PathSeparator)) {
		dir = filepath.Dir(value)
		base = strings.ToLower(filepath.Base(value))
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil
	}
	var out []string
	for _, entry := range entries {
		if base != "" && !strings.HasPrefix(strings.ToLower(entry.Name()), base) {
			continue
		}
		full := filepath.Join(dir, entry.Name())
		if entry.IsDir() {
			full += string(os.PathSeparator)
		}
		out = append(out, full)
	}
	sort.Strings(out)
	return out
}

func i18nUnsupportedType(mode pathPickerMode) string {
	if i18n.Current() == i18n.LanguageChinese {
		if mode == pickerSaveFile {
			return "仅支持 .zip 文件"
		}
		return "仅支持文件夹"
	}
	if mode == pickerSaveFile {
		return "Only .zip files can be selected"
	}
	return "Only directories can be selected"
}

func expandUser(path string) string {
	if path == "~" || strings.HasPrefix(path, "~/") {
		if home, err := os.UserHomeDir(); err == nil {
			return filepath.Join(home, strings.TrimPrefix(strings.TrimPrefix(path, "~"), "/"))
		}
	}
	return path
}
