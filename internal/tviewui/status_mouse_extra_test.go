package tviewui

import (
	"strings"
	"testing"

	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"

	appstructs "github.com/link-fgfgui/mod-downloader-core/structs"

	"github.com/link-fgfgui/mod-downloader-cli/internal/i18n"
)

func TestStatusBarFitDropsTrailingHints(t *testing.T) {
	h := newTestApp(t, 40, 20)
	status := h.app.statusBar
	status.SetQueueLabel("↓ 0/0")
	hints := []hint{
		hintText("first-segment"),
		hintText("second-segment-that-does-not-fit"),
		hintText("third-segment"),
	}
	status.SetHints(hints)
	text := status.hints.GetText(true)
	if !strings.Contains(text, "first-segment") {
		t.Fatalf("first hint was dropped: %q", text)
	}
	if strings.Contains(text, "second-segment-that-does-not-fit") {
		t.Fatalf("overflowing hint was kept: %q", text)
	}
	// Even a too-long first segment is kept so the bar is never empty.
	status.SetHints([]hint{hintText(strings.Repeat("x", 100)), hintText("tail")})
	if got := status.hints.GetText(true); !strings.Contains(got, "xxxx") || strings.Contains(got, "tail") {
		t.Fatalf("first-hint fallback wrong: %q", got)
	}
	// No hints renders the generic key help instead of an empty bar.
	status.SetHints(nil)
	if got := status.hints.GetText(true); !strings.Contains(got, uiText("keys")) {
		t.Fatalf("empty hints fallback=%q", got)
	}
}

func TestStatusBarQueueLabelWidth(t *testing.T) {
	h := newTestApp(t, 100, 30)
	status := h.app.statusBar
	status.SetQueueLabel("↓")
	if status.queueWidth < 9 {
		t.Fatalf("queueWidth=%d, want >=9", status.queueWidth)
	}
	status.SetQueueLabel("↓ 12/34 2048% 1.23 MiB/s")
	if status.queueWidth <= 9 {
		t.Fatalf("long label did not grow the capsule: %d", status.queueWidth)
	}
	if !strings.Contains(status.queue.GetLabel(), "↓ 12/34") {
		t.Fatalf("capsule label=%q", status.queue.GetLabel())
	}
}

func TestStatusBarHintRegionsTriggerActions(t *testing.T) {
	h := newTestApp(t, 100, 30)
	status := h.app.statusBar
	ran := 0
	status.SetHints([]hint{
		hintAction("click-me", func() { ran++ }),
		hintText("plain"),
	})
	status.hints.Highlight("hint0")
	if ran != 1 {
		t.Fatalf("hint action ran %d times, want 1", ran)
	}
	// The highlight is cleared after activation so the bar reads as plain text.
	if regions := status.hints.GetHighlights(); len(regions) != 0 {
		t.Fatalf("highlight was not cleared: %#v", regions)
	}
	// Unknown region ids are ignored.
	status.hints.Highlight("missing")
	if ran != 1 {
		t.Fatalf("unknown region triggered an action: %d", ran)
	}
}

func TestWheelListMoveCursor(t *testing.T) {
	list := newWheelList()
	for _, item := range []string{"a", "b", "c"} {
		list.AddItem(item, "", 0, nil)
	}
	list.moveCursor(-5)
	if got := list.GetCurrentItem(); got != 0 {
		t.Fatalf("cursor after up=%d, want 0", got)
	}
	list.moveCursor(5)
	if got := list.GetCurrentItem(); got != 2 {
		t.Fatalf("cursor after down=%d, want 2", got)
	}
	list.Clear()
	list.moveCursor(1) // empty list must not panic
}

func TestWheelTableMoveSelection(t *testing.T) {
	table := newWheelTable()
	for row := 0; row < 4; row++ {
		table.SetCell(row, 0, tview.NewTableCell("x"))
	}
	table.moveSelection(-1)
	if row, _ := table.GetSelection(); row != 1 {
		t.Fatalf("selection after up=%d, want 1", row)
	}
	table.moveSelection(10)
	if row, _ := table.GetSelection(); row != 3 {
		t.Fatalf("selection after down=%d, want 3", row)
	}
	empty := newWheelTable()
	empty.moveSelection(1) // no rows must not panic
}

func TestWheelTableMouseHandler(t *testing.T) {
	h := newTestApp(t, 100, 30)
	table := newWheelTable()
	for row := 0; row < 4; row++ {
		table.SetCell(row, 0, tview.NewTableCell("x"))
	}
	table.SetRect(0, 0, 20, 10)
	handler := table.MouseHandler()
	setFocus := func(tview.Primitive) {}
	event := tcell.NewEventMouse(2, 2, tcell.WheelDown, tcell.ModNone)
	if consumed, _ := handler(tview.MouseScrollDown, event, setFocus); !consumed {
		t.Fatal("wheel down was not consumed")
	}
	if row, _ := table.GetSelection(); row != 2 {
		t.Fatalf("selection after wheel down=%d, want 2", row)
	}
	event = tcell.NewEventMouse(2, 2, tcell.WheelUp, tcell.ModNone)
	if consumed, _ := handler(tview.MouseScrollUp, event, setFocus); !consumed {
		t.Fatal("wheel up was not consumed")
	}
	if row, _ := table.GetSelection(); row != 1 {
		t.Fatalf("selection after wheel up=%d, want 1", row)
	}
	// Events outside the table fall through to the base handler (not consumed).
	outside := tcell.NewEventMouse(50, 20, tcell.WheelDown, tcell.ModNone)
	if consumed, _ := handler(tview.MouseScrollDown, outside, setFocus); consumed {
		t.Fatal("outside wheel was consumed by the table")
	}
	_ = h
}

func TestWheelFormScrollingHelpers(t *testing.T) {
	form := newWheelForm()
	form.SetItemPadding(1)
	field1 := tview.NewInputField()
	field2 := tview.NewInputField()
	form.AddFormItem(field1)
	form.AddTextView("readonly", "", 0, 1, true, false)
	form.AddFormItem(field2)
	form.AddButton("save", nil)

	if got, want := form.contentHeight(), 1+1+1+1+1+1+1+1; got != want {
		t.Fatalf("contentHeight=%d, want %d", got, want)
	}
	form.SetItemPadding(2)
	if got, want := form.contentHeight(), 1+2+1+2+1+2+1+2; got != want {
		t.Fatalf("contentHeight with padding 2=%d, want %d", got, want)
	}

	// Section headers / read-only rows are not focusable; inputs and buttons are.
	if form.focusable(0) != true {
		t.Fatal("input should be focusable")
	}
	if form.focusable(1) {
		t.Fatal("plain TextView should not be focusable")
	}
	if !form.focusable(3) {
		t.Fatal("button should be focusable")
	}
	if form.focusable(-1) || form.focusable(99) {
		t.Fatal("out-of-range indices must not be focusable")
	}

	// currentIndex/focusedElement reflect the focus inside a live application.
	h := newTestApp(t, 100, 30)
	h.switchPage(ViewSettings)
	settings := h.app.settingsPage()
	h.app.application.SetFocus(settings.form)
	if got := settings.form.currentIndex(); got < 0 {
		t.Fatalf("currentIndex=%d, want a focused item", got)
	}
	if element := settings.form.focusedElement(); element == nil {
		t.Fatal("focusedElement returned nil")
	}
}

func TestClipScreen(t *testing.T) {
	sim := tcell.NewSimulationScreen("UTF-8")
	if err := sim.Init(); err != nil {
		t.Fatalf("init: %v", err)
	}
	defer sim.Fini()
	sim.SetSize(10, 5)

	clip := clipScreen{Screen: sim, x0: 0, y0: 0, x1: 5, y1: 5}
	clip.SetContent(1, 1, 'A', nil, tcell.StyleDefault)
	clip.SetContent(9, 1, 'B', nil, tcell.StyleDefault)
	sim.Show()
	cells, width, _ := sim.GetContents()
	if got := cells[1*width+1].Runes; len(got) == 0 || got[0] != 'A' {
		t.Fatalf("inside write was dropped: %#v", got)
	}
	if got := cells[1*width+9].Runes; len(got) != 0 && got[0] == 'B' {
		t.Fatal("outside write reached the screen")
	}

	clip.ShowCursor(2, 2)
	if x, y, visible := sim.GetCursor(); !visible || x != 2 || y != 2 {
		t.Fatalf("cursor=%d,%d visible=%v", x, y, visible)
	}
	clip.ShowCursor(7, 1)
	if _, _, visible := sim.GetCursor(); visible {
		t.Fatal("out-of-viewport cursor was not hidden")
	}
}

func TestActionBarLayoutKeepsOnlyFittingActions(t *testing.T) {
	h := newTestApp(t, 100, 30)
	bar := newActionBar(h.app)
	bar.Set(
		pageAction{Label: strings.Repeat("a", 40), Run: func() {}},
		pageAction{Label: strings.Repeat("b", 40), Run: func() {}},
		pageAction{Label: strings.Repeat("c", 40), Run: func() {}},
	)
	if len(bar.buttons) != 2 {
		t.Fatalf("100-column bar kept %d buttons, want 2", len(bar.buttons))
	}
	h.app.width = 60
	bar.Reflow()
	if len(bar.buttons) != 1 {
		t.Fatalf("60-column bar kept %d buttons, want 1", len(bar.buttons))
	}
	bar.Restyle() // must not panic after a theme change
}

// TestRightClickQueueCapsuleClearsReminders covers the right-click gesture on
// the status-bar capsule.
func TestRightClickQueueCapsuleClearsReminders(t *testing.T) {
	h := newTestApp(t, 120, 40)
	h.app.applyQueue(appstructs.DownloadQueueState{
		OptionalReminders: []appstructs.OptionalDependencyReminder{{ID: "r1"}},
	})
	h.draw()
	x, y, w, hgt := h.app.statusBar.QueueButton().GetRect()
	if w <= 0 || hgt <= 0 {
		t.Fatalf("capsule rect=%d,%d %dx%d", x, y, w, hgt)
	}
	event := tcell.NewEventMouse(x+w/2, y+hgt/2, tcell.Button2, tcell.ModNone)
	got, action := h.app.handleGlobalMouse(event, tview.MouseRightClick)
	if got != nil || action != 0 {
		t.Fatal("right-click on the capsule was forwarded to the page")
	}
}

// TestDialogMouseClickOnPaddingIsSwallowed guards the modal hit region: clicks
// inside the dialog rectangle that miss a child must not reach the page.
func TestDialogMouseClickOnBorderIsPassedThrough(t *testing.T) {
	h := newTestApp(t, 80, 24)
	h.app.openConfirm("Confirm", "body", []confirmOption{{Label: "OK", Value: "ok"}}, nil)
	h.draw()
	centered, ok := h.app.topDialog().page.(*centered)
	if !ok {
		t.Fatalf("dialog page=%T", h.app.topDialog().page)
	}
	x, y, _, _ := centered.GetRect()
	// A point on the dialog border, outside any child widget.
	event := tcell.NewEventMouse(x, y, tcell.Button1, tcell.ModNone)
	got, _ := h.app.handleGlobalMouse(event, tview.MouseLeftClick)
	if got == nil {
		t.Fatal("click inside the dialog border was dropped instead of passed through")
	}
	if len(h.app.dialogs) != 1 {
		t.Fatal("click on the border changed the dialog stack")
	}
}

func TestQueueDockOptionalHelpers(t *testing.T) {
	h := newTestApp(t, 120, 40)
	state := appstructs.DownloadQueueState{
		OptionalReminders: []appstructs.OptionalDependencyReminder{
			{ID: "r1", MainTitle: "Create", Dependencies: []appstructs.OptionalDependencyCandidate{{ProjectID: "jei", Platform: "curseforge"}}},
		},
	}
	h.app.applyQueue(state)
	h.app.queue.SetOpen(true)
	h.app.queue.setTab("optional")
	h.app.queue.Focus()
	h.app.queue.list.SetCurrentItem(0)
	if reminder, ok := h.app.queue.currentReminder(); !ok || reminder.ID != "r1" {
		t.Fatalf("currentReminder=%+v ok=%v", reminder, ok)
	}
	h.app.queueState.Queue.OptionalReminders = nil
	if _, ok := h.app.queue.currentReminder(); ok {
		t.Fatal("reminder lookup succeeded with no reminders")
	}
	h.app.queueState.Queue.OptionalReminders = state.OptionalReminders
	h.app.queue.list.SetCurrentItem(0)
	for _, key := range []rune{'i', 'd'} {
		if !h.app.queue.HandleKey(tcell.NewEventKey(tcell.KeyRune, key, tcell.ModNone)) {
			t.Fatalf("optional key %q was not consumed", key)
		}
	}
	if !h.app.queue.clearOptional() {
		t.Fatal("clearOptional did not handle the optional tab")
	}
	h.app.queue.setTab("downloads")
	if h.app.queue.installCurrentOptional() || h.app.queue.dismissCurrentOptional() || h.app.queue.clearOptional() {
		t.Fatal("optional actions fired on the downloads tab")
	}
	if hints := h.app.queue.Hints(); len(hints) == 0 {
		t.Fatal("downloads tab has no hints")
	}
	h.app.queue.setTab("optional")
	if hints := h.app.queue.Hints(); len(hints) == 0 {
		t.Fatal("optional tab has no hints")
	}
	i18n.Apply(i18n.LanguageEnglish)
}
