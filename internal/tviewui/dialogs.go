package tviewui

import (
	"fmt"
	"strings"

	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"

	"github.com/link-fgfgui/mod-downloader-cli/internal/i18n"
)

// dialog is one entry on the overlay stack. Dialog primitives are full-screen
// pages that paint only a centered box, so the page content stays visible.
type dialog struct {
	name      string
	page      tview.Primitive
	focus     tview.Primitive
	handleKey func(event *tcell.EventKey) bool
	onClose   func()
	restore   tview.Primitive
}

func (a *App) topDialog() *dialog {
	if len(a.dialogs) == 0 {
		return nil
	}
	return a.dialogs[len(a.dialogs)-1]
}

// pushDialog adds an overlay page, saves the previous focus and focuses the
// dialog's primary widget.
func (a *App) pushDialog(d *dialog) {
	if d == nil {
		return
	}
	for _, existing := range a.dialogs {
		if existing.name == d.name {
			return
		}
	}
	d.restore = a.application.GetFocus()
	a.dialogs = append(a.dialogs, d)
	a.outer.AddPage(d.name, d.page, true, true)
	a.outer.ShowPage(d.name)
	if d.focus != nil {
		a.application.SetFocus(d.focus)
	}
	a.updateStatus()
}

// popDialog removes the top overlay and restores focus.
func (a *App) popDialog() {
	if len(a.dialogs) == 0 {
		return
	}
	d := a.dialogs[len(a.dialogs)-1]
	a.dialogs = a.dialogs[:len(a.dialogs)-1]
	a.outer.RemovePage(d.name)
	if d.onClose != nil {
		d.onClose()
	}
	if d.restore != nil {
		a.application.SetFocus(d.restore)
	} else if a.queue != nil && a.queue.Visible() {
		a.queue.Focus()
	} else if a.active >= 0 && int(a.active) < len(a.views) {
		a.views[a.active].Focus()
	}
	a.updateStatus()
}

// popDialogName removes a specific overlay, used when callbacks close a nested
// dialog early.
func (a *App) popDialogName(name string) {
	for i := len(a.dialogs) - 1; i >= 0; i-- {
		if a.dialogs[i].name == name {
			if i == len(a.dialogs)-1 {
				a.popDialog()
				return
			}
			d := a.dialogs[i]
			a.dialogs = append(a.dialogs[:i], a.dialogs[i+1:]...)
			a.outer.RemovePage(name)
			if d.onClose != nil {
				d.onClose()
			}
			a.updateStatus()
			return
		}
	}
}

// centered paints its content centered inside the page rectangle.
type centered struct {
	content       tview.Primitive
	width, height int
	minW, minH    int
	rectX, rectY  int
	rectW, rectH  int
}

func newCentered(content tview.Primitive, width, height int) *centered {
	return &centered{content: content, width: width, height: height, minW: 24, minH: 5}
}

func (c *centered) Draw(screen tcell.Screen) {
	sw, sh := screen.Size()
	w := c.width
	if w <= 0 {
		w = sw - 4
	}
	if w < c.minW {
		w = c.minW
	}
	if w > sw-2 {
		w = sw - 2
	}
	h := c.height
	if h <= 0 {
		h = sh - 4
	}
	if h < c.minH {
		h = c.minH
	}
	if h > sh-2 {
		h = sh - 2
	}
	if w < 1 {
		w = 1
	}
	if h < 1 {
		h = 1
	}
	x := (sw - w) / 2
	y := (sh - h) / 2
	c.rectX, c.rectY, c.rectW, c.rectH = x, y, w, h
	// Dialogs sit on top of a live page; tview's Flex does not clear its own
	// background, so paint the whole dialog rectangle first or page text shows
	// through the gaps between children.
	background := tcell.StyleDefault.Background(tview.Styles.PrimitiveBackgroundColor)
	for row := y; row < y+h; row++ {
		for col := x; col < x+w; col++ {
			screen.SetContent(col, row, ' ', nil, background)
		}
	}
	c.content.SetRect(x, y, w, h)
	c.content.Draw(screen)
}

func (c *centered) GetRect() (int, int, int, int) { return c.rectX, c.rectY, c.rectW, c.rectH }
func (c *centered) SetRect(x, y, w, h int) {
	c.rectX, c.rectY, c.rectW, c.rectH = x, y, w, h
}
func (c *centered) InputHandler() func(*tcell.EventKey, func(tview.Primitive)) {
	return c.content.InputHandler()
}
func (c *centered) Focus(delegate func(tview.Primitive)) { delegate(c.content) }
func (c *centered) HasFocus() bool                       { return c.content.HasFocus() }
func (c *centered) Blur()                                { c.content.Blur() }
func (c *centered) MouseHandler() func(tview.MouseAction, *tcell.EventMouse, func(tview.Primitive)) (bool, tview.Primitive) {
	return func(action tview.MouseAction, event *tcell.EventMouse, setFocus func(tview.Primitive)) (bool, tview.Primitive) {
		if event == nil {
			return false, nil
		}
		x, y, w, h := c.GetRect()
		px, py := event.Position()
		inside := w > 0 && h > 0 && px >= x && px < x+w && py >= y && py < y+h
		if handler := c.content.MouseHandler(); handler != nil {
			if consumed, capture := handler(action, event, setFocus); consumed {
				return true, capture
			}
		}
		// Swallow clicks on the dialog's padding/border so the page behind the
		// modal cannot receive them.
		return inside, nil
	}
}
func (c *centered) PasteHandler() func(string, func(tview.Primitive)) {
	return c.content.PasteHandler()
}

// dialogBox builds the shared bordered container for custom dialogs.
func dialogBox(a *App, title string) *tview.Flex {
	box := tview.NewFlex().SetDirection(tview.FlexRow)
	box.SetBorder(true)
	box.SetTitle(title)
	a.theme.styleBox(box.Box)
	return box
}

// --- confirm -----------------------------------------------------------------

type confirmOption struct {
	Label       string
	Value       string
	Destructive bool
	Primary     bool
}

type confirmView struct {
	app     *App
	title   string
	body    *tview.TextView
	buttons []*tview.Button
	cursor  int
	options []confirmOption
	onDone  func(value string)
	dialog  *dialog
	scroll  int
	closed  bool
}

func (a *App) openConfirm(title, body string, options []confirmOption, onDone func(value string)) {
	v := &confirmView{app: a, title: title, options: options, onDone: onDone}
	v.body = tview.NewTextView()
	v.body.SetDynamicColors(false)
	v.body.SetScrollable(true)
	v.body.SetText(body)
	v.body.SetBackgroundColor(a.theme.Surface)
	v.body.SetTextColor(a.theme.OnSurface)

	buttonRow := tview.NewFlex().SetDirection(tview.FlexColumn)
	for i, opt := range options {
		idx := i
		label := opt.Label
		btn := tview.NewButton(label)
		btn.SetSelectedFunc(func() { v.activate(idx) })
		if opt.Destructive {
			btn.SetStyle(tcell.StyleDefault.Background(a.theme.Surface).Foreground(a.theme.Error))
			btn.SetActivatedStyle(tcell.StyleDefault.Background(a.theme.Error).Foreground(a.theme.SelectedFg))
		} else {
			a.theme.styleButton(btn)
		}
		v.buttons = append(v.buttons, btn)
		buttonRow.AddItem(btn, 0, 1, false)
	}

	box := dialogBox(a, title)
	box.AddItem(v.body, 0, 1, false)
	box.AddItem(buttonRow, 1, 0, false)

	page := newCentered(box, 64, v.height())
	d := &dialog{name: a.dialogName("confirm"), page: page, focus: v.body}
	d.handleKey = v.handleKey
	// Esc closes the dialog without activating a button, but it still means
	// "cancel": callers rely on onDone to release their in-flight state (e.g.
	// the install confirmation clears its downloading key).
	d.onClose = func() {
		if v.closed {
			return
		}
		v.closed = true
		if v.onDone != nil {
			v.onDone("cancel")
		}
	}
	v.dialog = d
	a.pushDialog(d)
	if len(v.buttons) > 0 {
		a.application.SetFocus(v.buttons[0])
	} else {
		a.application.SetFocus(v.body)
	}
}

func (v *confirmView) height() int {
	lines := strings.Count(v.body.GetText(false), "\n") + 1
	h := lines + 4
	if h < 8 {
		h = 8
	}
	if h > 18 {
		h = 18
	}
	return h
}

func (v *confirmView) handleKey(event *tcell.EventKey) bool {
	switch event.Key() {
	case tcell.KeyLeft, tcell.KeyRight, tcell.KeyTab, tcell.KeyBacktab:
		if len(v.buttons) == 0 {
			return true
		}
		delta := 1
		if event.Key() == tcell.KeyLeft || event.Key() == tcell.KeyBacktab {
			delta = -1
		}
		v.cursor = (v.cursor + delta + len(v.buttons)) % len(v.buttons)
		v.app.application.SetFocus(v.buttons[v.cursor])
		return true
	case tcell.KeyEnter:
		if len(v.buttons) > 0 {
			v.activate(v.cursor)
		}
		return true
	case tcell.KeyPgUp:
		v.scrollBy(-5)
		return true
	case tcell.KeyPgDn:
		v.scrollBy(5)
		return true
	case tcell.KeyUp:
		v.scrollBy(-1)
		return true
	case tcell.KeyDown:
		v.scrollBy(1)
		return true
	}
	return false
}

func (v *confirmView) scrollBy(delta int) {
	row, _ := v.body.GetScrollOffset()
	v.scroll = row + delta
	if v.scroll < 0 {
		v.scroll = 0
	}
	v.body.ScrollTo(v.scroll, 0)
}

func (v *confirmView) activate(index int) {
	if index < 0 || index >= len(v.options) {
		return
	}
	value := v.options[index].Value
	v.closed = true
	v.app.popDialogName(v.dialog.name)
	if v.onDone != nil {
		v.onDone(value)
	}
}

// --- prompt ------------------------------------------------------------------

type promptView struct {
	app       *App
	title     string
	input     *tview.InputField
	errorView *tview.TextView
	buttons   []*tview.Button
	cursor    int
	onSubmit  func(value string)
	validate  func(value string) string
	dialog    *dialog
}

// openPrompt shows a single-line input with Confirm/Cancel. validate may return
// an error message; returning it keeps the dialog open and displays the error.
func (a *App) openPrompt(title, hint, initial, placeholder string, validate func(string) string, onSubmit func(string)) {
	v := &promptView{app: a, title: title, onSubmit: onSubmit, validate: validate}
	v.input = tview.NewInputField()
	v.input.SetLabel("")
	v.input.SetPlaceholder(placeholder)
	v.input.SetText(initial)
	v.input.SetFieldWidth(0)
	a.theme.styleInput(v.input)
	v.input.SetDoneFunc(func(key tcell.Key) {
		if key == tcell.KeyEnter {
			v.submit()
		}
	})

	v.errorView = tview.NewTextView()
	v.errorView.SetTextColor(a.theme.Error)
	v.errorView.SetBackgroundColor(a.theme.Surface)

	buttonRow := tview.NewFlex().SetDirection(tview.FlexColumn)
	confirm := tview.NewButton(uiText("confirm"))
	a.theme.styleButton(confirm)
	confirm.SetSelectedFunc(func() { v.submit() })
	cancel := tview.NewButton(uiText("cancel"))
	cancel.SetStyle(tcell.StyleDefault.Background(a.theme.Surface).Foreground(a.theme.Error))
	cancel.SetActivatedStyle(tcell.StyleDefault.Background(a.theme.Error).Foreground(a.theme.SelectedFg))
	cancel.SetSelectedFunc(func() { a.popDialogName(v.dialog.name) })
	v.buttons = []*tview.Button{confirm, cancel}
	buttonRow.AddItem(confirm, 0, 1, false)
	buttonRow.AddItem(cancel, 0, 1, false)

	box := dialogBox(a, title)
	if hint != "" {
		hintView := tview.NewTextView()
		hintView.SetText(hint)
		hintView.SetTextColor(a.theme.Muted)
		hintView.SetBackgroundColor(a.theme.Surface)
		box.AddItem(hintView, 1, 0, false)
	}
	box.AddItem(v.input, 1, 0, false)
	box.AddItem(v.errorView, 1, 0, false)
	box.AddItem(buttonRow, 1, 0, false)

	page := newCentered(box, 64, 8)
	d := &dialog{name: a.dialogName("prompt"), page: page, focus: v.input}
	d.handleKey = v.handleKey
	v.dialog = d
	a.pushDialog(d)
}

func (v *promptView) handleKey(event *tcell.EventKey) bool {
	switch event.Key() {
	case tcell.KeyEnter:
		if v.app.application.GetFocus() == v.buttons[1] {
			v.app.popDialogName(v.dialog.name)
			return true
		}
		v.submit()
		return true
	case tcell.KeyTab, tcell.KeyBacktab, tcell.KeyLeft, tcell.KeyRight:
		if v.app.application.GetFocus() == v.input {
			v.app.application.SetFocus(v.buttons[0])
			v.cursor = 0
			return true
		}
		if event.Key() == tcell.KeyLeft || event.Key() == tcell.KeyBacktab {
			v.cursor = (v.cursor + 1) % 2
		} else {
			v.cursor = (v.cursor + 1) % 2
		}
		v.app.application.SetFocus(v.buttons[v.cursor])
		return true
	}
	return false
}

func (v *promptView) submit() {
	value := strings.TrimSpace(v.input.GetText())
	if v.validate != nil {
		if msg := v.validate(value); msg != "" {
			v.errorView.SetText(msg)
			return
		}
	}
	v.app.popDialogName(v.dialog.name)
	if v.onSubmit != nil {
		v.onSubmit(value)
	}
}

// --- select ------------------------------------------------------------------

type selectItem struct {
	Label    string
	Value    string
	Detail   string
	Disabled bool
}

type selectView struct {
	app      *App
	items    []selectItem
	list     *wheelList
	onSelect func(selectItem)
	dialog   *dialog
}

func (a *App) openSelect(title string, items []selectItem, onSelect func(selectItem)) {
	v := &selectView{app: a, items: items, onSelect: onSelect}
	v.list = newWheelList()
	v.list.ShowSecondaryText(true)
	a.theme.styleList(v.list.List)
	for _, item := range items {
		it := item
		main := tview.Escape(it.Label)
		if it.Disabled {
			main = "[-] " + main
		}
		v.list.AddItem(main, tview.Escape(it.Detail), 0, func() {
			if it.Disabled {
				return
			}
			a.popDialogName(v.dialog.name)
			if v.onSelect != nil {
				v.onSelect(it)
			}
		})
	}
	box := dialogBox(a, title)
	box.AddItem(v.list, 0, 1, true)
	d := &dialog{name: a.dialogName("select"), focus: v.list}
	cancel := a.chipButton(uiText("cancel"), func() { a.popDialogName(d.name) })
	box.AddItem(tview.NewFlex().SetDirection(tview.FlexColumn).
		AddItem(cancel, chipWidth(uiText("cancel")), 0, false), 1, 0, false)
	page := newCentered(box, 72, 19)
	d.page = page
	v.dialog = d
	a.pushDialog(d)
}

// --- help --------------------------------------------------------------------

func (a *App) openHelpDialog() {
	t := a.theme
	rows := [][2]string{
		{"1-6", uiText("helpViews")},
		{"Tab", uiText("helpCycle")},
		{"↑↓ / j k", uiText("helpMove")},
		{"Space", uiText("helpSelect")},
		{"Enter", uiText("helpActivate")},
		{"q", i18n.T("download.queue.title")},
		{"t", uiText("helpTarget")},
		{"r", uiText("refresh")},
		{"Wheel", uiText("helpWheel")},
		{"Click", uiText("helpClick")},
		{"Esc", uiText("helpBack")},
		{"Ctrl+C", uiText("helpQuit")},
	}
	var b strings.Builder
	for _, row := range rows {
		b.WriteString(t.tag(t.Secondary))
		b.WriteString(row[0])
		b.WriteString(resetTag)
		b.WriteString("  ")
		b.WriteString(row[1])
		b.WriteString("\n")
	}
	b.WriteString("\n")
	b.WriteString(t.tag(t.Muted))
	b.WriteString(uiText("helpClose"))
	b.WriteString(resetTag)

	view := tview.NewTextView().SetDynamicColors(true)
	view.SetText(b.String())
	view.SetBackgroundColor(t.Surface)
	view.SetTextColor(t.OnSurface)

	box := dialogBox(a, uiText("helpTitle"))
	box.AddItem(view, 0, 1, true)
	d := &dialog{name: a.dialogName("help"), focus: view}
	closeButton := a.chipButton(uiText("close"), func() { a.popDialogName(d.name) })
	box.AddItem(tview.NewFlex().SetDirection(tview.FlexColumn).
		AddItem(closeButton, chipWidth(uiText("close")), 0, false), 1, 0, false)
	page := newCentered(box, 60, 19)
	d.page = page
	a.pushDialog(d)
}

// --- naming ------------------------------------------------------------------

func (a *App) dialogName(kind string) string {
	a.dialogSeq++
	return fmt.Sprintf("%s-%d", kind, a.dialogSeq)
}
