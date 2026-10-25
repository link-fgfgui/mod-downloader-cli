package tviewui

import (
	"github.com/gdamore/tcell/v2"
	"github.com/mattn/go-runewidth"
	"github.com/rivo/tview"
)

// wheelList is a tview.List whose wheel events move the cursor. tview's own
// wheel handling only shifts an internal offset that Draw immediately resets to
// keep the cursor visible, so scrolling over a list appeared to do nothing.
type wheelList struct {
	*tview.List
}

func newWheelList() *wheelList { return &wheelList{List: tview.NewList()} }

func (l *wheelList) MouseHandler() func(action tview.MouseAction, event *tcell.EventMouse, setFocus func(tview.Primitive)) (bool, tview.Primitive) {
	inner := l.List.MouseHandler()
	return func(action tview.MouseAction, event *tcell.EventMouse, setFocus func(tview.Primitive)) (bool, tview.Primitive) {
		if event != nil && l.InRect(event.Position()) {
			switch action {
			case tview.MouseScrollUp:
				l.moveCursor(-1)
				return true, nil
			case tview.MouseScrollDown:
				l.moveCursor(1)
				return true, nil
			}
		}
		return inner(action, event, setFocus)
	}
}

func (l *wheelList) moveCursor(delta int) {
	count := l.GetItemCount()
	if count == 0 {
		return
	}
	l.SetCurrentItem(clampRange(l.GetCurrentItem()+delta, 0, count-1))
}

// wheelTable is a tview.Table with the same wheel behaviour as wheelList.
type wheelTable struct {
	*tview.Table
}

func newWheelTable() *wheelTable { return &wheelTable{Table: tview.NewTable()} }

func (t *wheelTable) MouseHandler() func(action tview.MouseAction, event *tcell.EventMouse, setFocus func(tview.Primitive)) (bool, tview.Primitive) {
	inner := t.Table.MouseHandler()
	return func(action tview.MouseAction, event *tcell.EventMouse, setFocus func(tview.Primitive)) (bool, tview.Primitive) {
		if event != nil && t.InRect(event.Position()) {
			switch action {
			case tview.MouseScrollUp:
				t.moveSelection(-1)
				return true, nil
			case tview.MouseScrollDown:
				t.moveSelection(1)
				return true, nil
			}
		}
		return inner(action, event, setFocus)
	}
}

func (t *wheelTable) moveSelection(delta int) {
	rows := t.GetRowCount()
	if rows == 0 {
		return
	}
	row, _ := t.GetSelection()
	if row < 1 {
		row = 1
	}
	next := clampRange(row+delta, 1, rows-1)
	t.Select(next, 0)
}

// wheelForm is a tview.Form that scrolls as a whole. tview's Form only keeps
// the focused element in view (and not reliably for long forms), so the wheel
// moves a viewport offset here instead of the focus.
type wheelForm struct {
	*tview.Form
	offset  int
	padding int
}

func newWheelForm() *wheelForm { return &wheelForm{Form: tview.NewForm(), padding: 1} }

// SetItemPadding records the padding so contentHeight stays accurate.
func (f *wheelForm) SetItemPadding(padding int) *tview.Form {
	f.padding = padding
	return f.Form.SetItemPadding(padding)
}

func (f *wheelForm) Draw(screen tcell.Screen) {
	x, y, w, h := f.GetRect()
	total := f.contentHeight()
	if total <= h {
		f.Form.Draw(screen)
		return
	}
	f.offset = clampRange(f.offset, 0, total-h)
	f.drawAt(screen, x, y, w, h, total)
	// Keyboard navigation must keep the focused element on screen.
	if focus := f.focusedElement(); focus != nil {
		_, fy, _, fh := focus.GetRect()
		shift := 0
		if fy < y {
			shift = fy - y
		} else if fy+fh > y+h {
			shift = fy + fh - (y + h)
		}
		if shift != 0 {
			f.offset = clampRange(f.offset+shift, 0, total-h)
			f.drawAt(screen, x, y, w, h, total)
		}
	}
}

// drawAt paints the whole form shifted by the scroll offset, clipped to the
// viewport so items outside it stay hidden.
func (f *wheelForm) drawAt(screen tcell.Screen, x, y, w, h, total int) {
	f.Form.SetRect(x, y-f.offset, w, total)
	f.Form.Draw(clipScreen{Screen: screen, x0: x, y0: y, x1: x + w, y1: y + h})
	f.Form.SetRect(x, y, w, h)
}

// contentHeight is the height the form needs to show every item and button.
func (f *wheelForm) contentHeight() int {
	padding := f.padding
	if padding <= 0 {
		padding = 1
	}
	height := 0
	for i := 0; i < f.GetFormItemCount(); i++ {
		itemHeight := f.GetFormItem(i).GetFieldHeight()
		if itemHeight <= 0 {
			itemHeight = 1
		}
		height += itemHeight + padding
	}
	if buttons := f.GetButtonCount(); buttons > 0 {
		height += 1 + padding // buttons occupy one row
	}
	return height
}

// focusedElement returns the focused form item or button.
func (f *wheelForm) focusedElement() tview.Primitive {
	if item, button := f.GetFocusedItemIndex(); item >= 0 {
		return f.GetFormItem(item)
	} else if button >= 0 && button < f.GetButtonCount() {
		return f.GetButton(button)
	}
	return nil
}

func (f *wheelForm) MouseHandler() func(action tview.MouseAction, event *tcell.EventMouse, setFocus func(tview.Primitive)) (bool, tview.Primitive) {
	inner := f.Form.MouseHandler()
	return func(action tview.MouseAction, event *tcell.EventMouse, setFocus func(tview.Primitive)) (bool, tview.Primitive) {
		if event != nil && f.InRect(event.Position()) {
			switch action {
			case tview.MouseScrollUp:
				f.scroll(-1)
				return true, nil
			case tview.MouseScrollDown:
				f.scroll(1)
				return true, nil
			}
		}
		return inner(action, event, setFocus)
	}
}

// scroll moves the viewport and keeps the keyboard focus inside it.
func (f *wheelForm) scroll(delta int) {
	_, _, _, h := f.GetRect()
	total := f.contentHeight()
	maxOffset := total - h
	if maxOffset <= 0 {
		return
	}
	f.offset = clampRange(f.offset+delta, 0, maxOffset)
	// Move the focus along so Tab/Enter keep acting on what is visible.
	f.moveFocusToViewport()
}

// moveFocusToViewport focuses the first focusable element inside the viewport.
func (f *wheelForm) moveFocusToViewport() {
	total := f.GetFormItemCount() + f.GetButtonCount()
	index := f.currentIndex()
	if index < 0 {
		index = 0
	}
	for step := 0; step < total; step++ {
		candidate := clampRange(index+step, 0, total-1)
		if !f.focusable(candidate) {
			continue
		}
		f.SetFocus(candidate)
		if element := f.focusedElement(); element != nil {
			_, ey, _, eh := element.GetRect()
			_, y, _, h := f.GetRect()
			if ey >= y && ey+eh <= y+h {
				return
			}
		}
	}
}

// currentIndex is the focused form item (items first, then buttons) or -1.
func (f *wheelForm) currentIndex() int {
	if item, button := f.GetFocusedItemIndex(); item >= 0 {
		return item
	} else if button >= 0 {
		return f.GetFormItemCount() + button
	}
	return -1
}

// focusable reports whether an element accepts focus. Section headers and
// read-only rows are plain TextViews; focusing one makes tview recurse into
// itself, so the wheel has to step over them.
func (f *wheelForm) focusable(index int) bool {
	total := f.GetFormItemCount() + f.GetButtonCount()
	if index < 0 || index >= total {
		return false
	}
	if index >= f.GetFormItemCount() {
		return true
	}
	_, isText := f.GetFormItem(index).(*tview.TextView)
	return !isText
}

// clipScreen drops writes outside the viewport so a taller form cannot paint
// over the rest of the page.
type clipScreen struct {
	tcell.Screen
	x0, y0, x1, y1 int
}

func (c clipScreen) SetContent(x, y int, mainc rune, combc []rune, style tcell.Style) {
	if x < c.x0 || x >= c.x1 || y < c.y0 || y >= c.y1 {
		return
	}
	c.Screen.SetContent(x, y, mainc, combc, style)
}

func (c clipScreen) ShowCursor(x, y int) {
	if x < c.x0 || x >= c.x1 || y < c.y0 || y >= c.y1 {
		c.Screen.HideCursor()
		return
	}
	c.Screen.ShowCursor(x, y)
}

// chipButton builds a themed clickable chip with a padded label. Brackets are
// avoided because tview parses them as style tags.
func (a *App) chipButton(label string, run func()) *tview.Button {
	button := tview.NewButton(" " + label + " ")
	a.theme.styleActionButton(button)
	button.SetSelectedFunc(run)
	return button
}

// chipWidth is the cell width of a chip label.
func chipWidth(label string) int { return runewidth.StringWidth(" " + label + " ") }

// pageAction is one clickable page operation, mirroring a keyboard shortcut.
type pageAction struct {
	Label string
	Run   func()
}

// actionBar is a single row of buttons that keeps pages without text input
// fully operable with the mouse.
type actionBar struct {
	app     *App
	flex    *tview.Flex
	actions []pageAction
	buttons []*tview.Button
	// width, when set, overrides the page width used to decide how many
	// actions fit (the queue dock is narrower than the page area).
	width func() int
	// refocus returns keyboard focus to the page list after a click.
	refocus func()
}

// SetRefocus installs the focus target used after a button click, so keyboard
// navigation keeps working on the list the user was browsing.
func (b *actionBar) SetRefocus(f func()) { b.refocus = f }

func newActionBar(a *App, actions ...pageAction) *actionBar {
	bar := &actionBar{app: a, flex: tview.NewFlex().SetDirection(tview.FlexColumn)}
	bar.flex.SetBackgroundColor(a.theme.Surface)
	bar.Set(actions...)
	return bar
}

// Set replaces the bar's buttons (the favorites page swaps them per pane).
func (b *actionBar) Set(actions ...pageAction) {
	b.actions = actions
	b.layout()
}

// Reflow re-lays the bar out for the current width.
func (b *actionBar) Reflow() { b.layout() }

// layout keeps as many actions as fit; the rest stay available on the keyboard.
func (b *actionBar) layout() {
	b.flex.Clear()
	b.buttons = b.buttons[:0]
	available := b.app.mainWidth()
	if b.width != nil {
		available = b.width()
	}
	used := 0
	for _, action := range b.actions {
		// Bracketed labels look like tview style tags and are swallowed when
		// drawn, so buttons are padded chips separated by a one-cell gap.
		label := " " + action.Label + " "
		width := runewidth.StringWidth(label)
		gap := 0
		if len(b.buttons) > 0 {
			gap = 1
		}
		if used+gap+width > available {
			break
		}
		if gap == 1 {
			spacer := tview.NewTextView().SetText(" ")
			spacer.SetBackgroundColor(b.app.theme.Surface)
			b.flex.AddItem(spacer, 1, 0, false)
		}
		button := b.app.chipButton(action.Label, func() {
			action.Run()
			if b.refocus != nil && b.app.topDialog() == nil {
				b.refocus()
			}
		})
		b.buttons = append(b.buttons, button)
		b.flex.AddItem(button, width, 0, false)
		used += gap + width
	}
}

func (b *actionBar) Primitive() tview.Primitive { return b.flex }

// Restyle reapplies the palette after a theme change.
func (b *actionBar) Restyle() {
	b.flex.SetBackgroundColor(b.app.theme.Surface)
	for _, button := range b.buttons {
		b.app.theme.styleActionButton(button)
	}
}
