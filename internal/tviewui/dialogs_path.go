package tviewui

import (
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"

	"github.com/link-fgfgui/mod-downloader-core/minecraft"
)

// pathDialog is the path-input file chooser: a single-line input with bash-like
// Tab completion. There is intentionally no directory tree browser.
type pathDialog struct {
	app     *App
	save    bool
	title   string
	input   *tview.InputField
	err     *tview.TextView
	list    *tview.TextView
	buttons []*tview.Button
	cursor  int

	completions    []string
	completionBase string
	showCandidates bool

	dialog   *dialog
	onAccept func(path string)
}

// openPathDialog opens the path picker. save=false selects a directory,
// save=true selects a .zip destination. The dialog is returned for tests.
func (a *App) openPathDialog(title string, save bool, start string, onAccept func(string)) *pathDialog {
	v := &pathDialog{app: a, save: save, title: title, onAccept: onAccept}
	initial := initialPath(save, start)
	v.input = tview.NewInputField()
	v.input.SetLabel(" › ")
	v.input.SetText(initial)
	v.input.SetPlaceholder(uiText("typePath"))
	v.input.SetFieldWidth(0)
	a.theme.styleInput(v.input)
	v.input.SetChangedFunc(func(_ string) {
		v.err.SetText("")
		v.showCandidates = false
		v.list.SetText("")
	})
	v.input.SetDoneFunc(func(key tcell.Key) {
		if key == tcell.KeyEnter {
			v.accept()
		}
	})

	v.err = tview.NewTextView()
	v.err.SetTextColor(a.theme.Error)
	v.err.SetBackgroundColor(a.theme.Surface)

	v.list = tview.NewTextView()
	v.list.SetDynamicColors(true)
	v.list.SetTextColor(a.theme.Muted)
	v.list.SetBackgroundColor(a.theme.Surface)
	v.list.SetScrollable(true)

	confirm := tview.NewButton(uiText("confirm"))
	a.theme.styleButton(confirm)
	confirm.SetSelectedFunc(v.accept)
	cancel := tview.NewButton(uiText("cancel"))
	cancel.SetStyle(tcell.StyleDefault.Background(a.theme.Surface).Foreground(a.theme.Error))
	cancel.SetActivatedStyle(tcell.StyleDefault.Background(a.theme.Error).Foreground(a.theme.SelectedFg))
	cancel.SetSelectedFunc(func() { a.popDialogName(v.dialog.name) })
	v.buttons = []*tview.Button{confirm, cancel}
	buttonRow := tview.NewFlex().SetDirection(tview.FlexColumn).
		AddItem(confirm, 0, 1, false).
		AddItem(cancel, 0, 1, false)

	box := dialogBox(a, title)
	hint := tview.NewTextView()
	hint.SetText(uiText("typePath") + " · tab " + uiText("confirm") + " · shift+tab " + uiText("navigate"))
	hint.SetTextColor(a.theme.Muted)
	hint.SetBackgroundColor(a.theme.Surface)
	box.AddItem(hint, 1, 0, false)
	box.AddItem(v.input, 1, 0, false)
	box.AddItem(v.err, 1, 0, false)
	box.AddItem(v.list, 4, 0, false)
	box.AddItem(buttonRow, 1, 0, false)

	page := newCentered(box, 74, 15)
	d := &dialog{name: a.dialogName("path"), page: page, focus: v.input}
	d.handleKey = v.handleKey
	v.dialog = d
	a.pushDialog(d)
	a.application.SetFocus(v.input)
	return v
}

func initialPath(save bool, start string) string {
	home, _ := os.UserHomeDir()
	if save {
		name := strings.TrimSpace(start)
		if name == "" {
			name = "packwiz.zip"
		}
		if filepath.IsAbs(name) {
			return name
		}
		return filepath.Join(home, filepath.Base(name))
	}
	dir := expandPath(strings.TrimSpace(start))
	if dir == "" {
		return home
	}
	if info, err := os.Stat(dir); err == nil && !info.IsDir() {
		dir = filepath.Dir(dir)
	}
	return dir
}

func (v *pathDialog) handleKey(event *tcell.EventKey) bool {
	switch event.Key() {
	case tcell.KeyTab:
		if v.app.application.GetFocus() == v.input {
			v.complete()
			return true
		}
		return false
	case tcell.KeyBacktab:
		if v.app.application.GetFocus() == v.input {
			v.app.application.SetFocus(v.buttons[0])
			v.cursor = 0
		} else {
			v.cursor = (v.cursor + 1) % len(v.buttons)
			v.app.application.SetFocus(v.buttons[v.cursor])
		}
		return true
	case tcell.KeyEnter:
		if v.app.application.GetFocus() == v.buttons[1] {
			v.app.popDialogName(v.dialog.name)
			return true
		}
		v.accept()
		return true
	case tcell.KeyCtrlS, tcell.KeyCtrlY:
		v.accept()
		return true
	case tcell.KeyLeft, tcell.KeyRight:
		if v.app.application.GetFocus() == v.input {
			return false
		}
		if event.Key() == tcell.KeyLeft {
			v.cursor = (v.cursor + len(v.buttons) - 1) % len(v.buttons)
		} else {
			v.cursor = (v.cursor + 1) % len(v.buttons)
		}
		v.app.application.SetFocus(v.buttons[v.cursor])
		return true
	}
	return false
}

// complete implements bash-style Tab: first complete the common prefix, then
// list candidates when nothing more can be inserted.
func (v *pathDialog) complete() {
	value := v.input.GetText()
	expanded := expandPath(value)
	if len(v.completions) == 0 || v.completionBase != value {
		v.completions = completePath(expanded, v.save)
		v.completionBase = value
	}
	v.showCandidates = false
	if len(v.completions) == 0 {
		v.err.SetText(uiText("noCompletions"))
		return
	}
	common := longestCommonPrefix(v.completions)
	if common != "" && common != expanded {
		v.input.SetText(common)
		v.err.SetText("")
		v.showCandidates = false
		return
	}
	v.showCandidates = true
	v.renderCandidates()
	v.err.SetText("")
}

func (v *pathDialog) renderCandidates() {
	limit := minInt(8, len(v.completions))
	lines := make([]string, 0, limit)
	for _, candidate := range v.completions[:limit] {
		lines = append(lines, "  "+truncate(candidate, 66))
	}
	if len(v.completions) > limit {
		lines = append(lines, "  …")
	}
	v.list.SetText(strings.Join(lines, "\n"))
}

func (v *pathDialog) accept() {
	raw := strings.TrimSpace(v.input.GetText())
	if raw == "" {
		v.err.SetText(uiText("invalidDirectory"))
		return
	}
	path := expandPath(raw)
	if v.save {
		if filepath.Ext(path) == "" {
			path += ".zip"
		}
		parent := filepath.Dir(path)
		if info, err := os.Stat(parent); err != nil || !info.IsDir() {
			v.err.SetText(uiText("invalidDirectory"))
			return
		}
	} else {
		info, err := os.Stat(path)
		if err != nil || !info.IsDir() {
			v.err.SetText(uiText("invalidDirectory"))
			return
		}
	}
	v.app.popDialogName(v.dialog.name)
	if v.onAccept != nil {
		v.onAccept(filepath.Clean(path))
	}
}

// completePath returns entries matching the final segment of path. Directories
// end with the platform separator; save mode also offers files.
func completePath(path string, save bool) []string {
	dir := path
	base := ""
	if !strings.HasSuffix(path, string(os.PathSeparator)) {
		dir = filepath.Dir(path)
		base = strings.ToLower(filepath.Base(path))
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
		if entry.IsDir() {
			out = append(out, filepath.Join(dir, entry.Name())+string(os.PathSeparator))
			continue
		}
		if save {
			out = append(out, filepath.Join(dir, entry.Name()))
		}
	}
	sort.Strings(out)
	return out
}

func longestCommonPrefix(values []string) string {
	if len(values) == 0 {
		return ""
	}
	prefix := values[0]
	for _, value := range values[1:] {
		for !strings.HasPrefix(value, prefix) {
			if prefix == "" {
				return ""
			}
			prefix = prefix[:len(prefix)-1]
		}
	}
	return prefix
}

// expandPath applies ~ and $HOME/$PWD expansion.
func expandPath(path string) string {
	return filepath.Clean(minecraft.ExpandPathWithEnv(expandUser(path)))
}

func expandUser(path string) string {
	if path == "~" || strings.HasPrefix(path, "~/") {
		if home, err := os.UserHomeDir(); err == nil {
			return filepath.Join(home, strings.TrimPrefix(strings.TrimPrefix(path, "~"), "/"))
		}
	}
	return path
}
