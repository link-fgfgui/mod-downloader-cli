package tviewui

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"

	appstructs "github.com/link-fgfgui/mod-downloader-core/structs"
	structs "github.com/link-fgfgui/mod-downloader-core/structs/minecraft"

	"github.com/link-fgfgui/mod-downloader-cli/internal/i18n"
	"github.com/link-fgfgui/mod-downloader-cli/internal/uistate"
)

// --- queue dock --------------------------------------------------------------

func TestQueueDockRenderingBranches(t *testing.T) {
	h := newTestApp(t, 120, 40)
	q := h.app.queue
	q.app.queueState.ActiveTab = uistate.QueueTabDownloads

	// Header with progress, speed and an unknown-size active queue.
	q.app.queueState.Queue = appstructs.DownloadQueueState{
		Active: true, Running: 1, Pending: 2,
		TotalBytes: 200, BytesComplete: 50, BytesPerSecond: 1024,
	}
	q.renderHeader()
	text := q.header.GetText(true)
	if !strings.Contains(text, "25%") || !strings.Contains(text, "MiB/s") {
		t.Fatalf("header=%q", text)
	}
	q.app.queueState.Queue.TotalBytes = 0
	q.app.queueState.Queue.BytesComplete = 0
	q.renderHeader()
	if !strings.Contains(q.header.GetText(true), i18n.T("download.queue.unknownSize")) {
		t.Fatalf("unknown-size header=%q", q.header.GetText(true))
	}

	// Optional reminders add a badge to the tab label.
	q.app.queueState.Queue.OptionalReminders = []appstructs.OptionalDependencyReminder{{ID: "r1"}}
	q.app.queueState.ActiveTab = uistate.QueueTabDownloads
	q.renderTabs()
	if !strings.Contains(q.opTab.GetLabel(), "(1)") {
		t.Fatalf("optional tab label=%q", q.opTab.GetLabel())
	}
	q.app.queueState.ActiveTab = uistate.QueueTabOptional
	q.renderTabs()
	if !strings.HasPrefix(q.opTab.GetLabel(), "▶ ") {
		t.Fatalf("active optional tab label=%q", q.opTab.GetLabel())
	}
	q.app.queueState.ActiveTab = uistate.QueueTabDownloads
	q.renderTabs()

	// Download rows fall back through title, file name, version id and id and
	// append the failure reason.
	q.app.queueState.Queue.Items = []appstructs.DownloadQueueItem{
		{ID: "id-only", Status: "canceled"},
		{ID: "reason", Status: "failed", FileName: "f.jar", Platform: "modrinth", Reason: "checksum"},
		{ID: "version", Status: "pending", VersionID: "ver-1"},
		{ID: "title", Status: "running", Title: "Title", TotalBytes: 10, BytesComplete: 10},
	}
	q.list.Clear()
	q.renderDownloads()
	if q.list.GetItemCount() != 4 {
		t.Fatalf("download rows=%d", q.list.GetItemCount())
	}
	main, detail := q.list.GetItemText(0)
	if !strings.Contains(main, "⊘") {
		t.Fatalf("canceled icon main=%q", main)
	}
	main, detail = q.list.GetItemText(1)
	if !strings.Contains(detail, "f.jar") || !strings.Contains(detail, "checksum") {
		t.Fatalf("failed detail=%q", detail)
	}
	main, detail = q.list.GetItemText(2)
	if !strings.Contains(main, "ver-1") {
		t.Fatalf("pending main=%q", main)
	}

	// Optional rows use the reminder id and dependency project id when titles
	// are missing.
	q.app.queueState.ActiveTab = uistate.QueueTabOptional
	q.app.queueState.Queue.OptionalReminders = []appstructs.OptionalDependencyReminder{{
		ID: "reminder-id",
		Dependencies: []appstructs.OptionalDependencyCandidate{
			{ProjectID: "dep-project"},
			{Title: "Dep Title"},
		},
	}}
	q.list.Clear()
	q.renderOptional()
	if q.list.GetItemCount() != 1 {
		t.Fatalf("optional rows=%d", q.list.GetItemCount())
	}
	main, detail = q.list.GetItemText(0)
	if !strings.Contains(main, "reminder-id") || !strings.Contains(detail, "dep-project") || !strings.Contains(detail, "Dep Title") {
		t.Fatalf("optional row main=%q detail=%q", main, detail)
	}

	// Empty tabs render their placeholders.
	q.app.queueState.Queue.Items = nil
	q.app.queueState.ActiveTab = uistate.QueueTabDownloads
	q.list.Clear()
	q.renderDownloads()
	emptyMain, _ := q.list.GetItemText(0)
	if !strings.Contains(emptyMain, i18n.T("download.queue.emptyDownloads")) {
		t.Fatalf("empty downloads=%q", emptyMain)
	}
	q.app.queueState.Queue.OptionalReminders = nil
	q.app.queueState.ActiveTab = uistate.QueueTabOptional
	q.list.Clear()
	q.renderOptional()
	emptyMain, _ = q.list.GetItemText(0)
	if !strings.Contains(emptyMain, i18n.T("download.queue.emptyOptional")) {
		t.Fatalf("empty optional=%q", emptyMain)
	}

	// Status icons cover every state.
	for _, status := range []string{"running", "pending", "failed", "canceled", "other"} {
		if icon := queueStatusIcon(h.app.theme, status); icon == "" {
			t.Fatalf("status %q has no icon", status)
		}
	}

	// Cursor preservation and a full refresh.
	q.app.queueState.ActiveTab = uistate.QueueTabDownloads
	q.app.queueState.Queue.Items = []appstructs.DownloadQueueItem{
		{ID: "a", Status: "running", Title: "A"},
		{ID: "b", Status: "pending", Title: "B"},
	}
	q.Refresh()
	q.list.SetCurrentItem(1)
	q.renderList()
	if q.list.GetCurrentItem() != 1 {
		t.Fatalf("cursor=%d, want 1", q.list.GetCurrentItem())
	}
	q.restyle()

	var nilDock *queueDock
	nilDock.Refresh() // nil-safe
}

func TestQueueDockKeyboardAndHints(t *testing.T) {
	h := newTestApp(t, 120, 40)
	q := h.app.queue
	q.app.queueState.Queue = appstructs.DownloadQueueState{
		Active: true, Running: 1,
		Items: []appstructs.DownloadQueueItem{{ID: "a", Status: "running", Title: "A"}},
	}
	q.SetOpen(true)
	q.Refresh()

	// Closed dock ignores keys.
	q.SetOpen(false)
	if q.HandleKey(tcell.NewEventKey(tcell.KeyRune, 'c', tcell.ModNone)) {
		t.Fatal("closed dock handled a key")
	}
	q.SetOpen(true)

	// Download-tab hints and their safe actions.
	hints := q.Hints()
	if len(hints) != 5 {
		t.Fatalf("download hints=%d", len(hints))
	}
	q.list.SetCurrentItem(0)
	hintRun(t, hints, "c ") // not cancelable: no-op
	hintRun(t, hints, "r ") // not retryable: no-op
	hintRun(t, hints, "x ") // pending cannot be removed: toast
	if h.app.toastText != i18n.T("download.queue.removeItem") {
		t.Fatalf("remove toast=%q", h.app.toastText)
	}
	if h.app.toastTimer != nil {
		h.app.toastTimer.Stop()
		h.app.toastTimer = nil
	}

	// Tabs switch with tab/left/right and the active tab ignores redundant sets.
	if !q.HandleKey(tcell.NewEventKey(tcell.KeyTab, 0, tcell.ModNone)) {
		t.Fatal("tab not handled")
	}
	if q.app.queueState.ActiveTab != uistate.QueueTabOptional {
		t.Fatalf("tab=%q", q.app.queueState.ActiveTab)
	}
	q.setTab(uistate.QueueTabOptional) // same tab: no-op
	q.HandleKey(tcell.NewEventKey(tcell.KeyRight, 0, tcell.ModNone))
	if q.app.queueState.ActiveTab != uistate.QueueTabDownloads {
		t.Fatalf("right tab=%q", q.app.queueState.ActiveTab)
	}
	q.HandleKey(tcell.NewEventKey(tcell.KeyLeft, 0, tcell.ModNone))
	if q.app.queueState.ActiveTab != uistate.QueueTabOptional {
		t.Fatalf("left tab=%q", q.app.queueState.ActiveTab)
	}

	// Optional-tab hints and actions with a reminder.
	q.app.queueState.Queue.OptionalReminders = []appstructs.OptionalDependencyReminder{{
		ID: "r1", MainTitle: "Reminder",
		Dependencies: []appstructs.OptionalDependencyCandidate{{ProjectID: "dep"}},
	}}
	q.app.queueState.ActiveTab = uistate.QueueTabOptional
	q.Refresh()
	optionalHints := q.Hints()
	if len(optionalHints) != 5 {
		t.Fatalf("optional hints=%d", len(optionalHints))
	}
	hintRun(t, optionalHints, "i ")
	hintRun(t, optionalHints, "d ")
	hintRun(t, optionalHints, "a ")

	// Actions are rejected on the wrong tab.
	q.app.queueState.ActiveTab = uistate.QueueTabDownloads
	if q.installCurrentOptional() || q.dismissCurrentOptional() || q.clearOptional() {
		t.Fatal("optional actions ran on the downloads tab")
	}

	// Empty queues have no current item/reminder.
	q.app.queueState.Queue.Items = nil
	q.app.queueState.Queue.OptionalReminders = nil
	if _, ok := q.currentItem(); ok {
		t.Fatal("currentItem on an empty queue")
	}
	if _, ok := q.currentReminder(); ok {
		t.Fatal("currentReminder on an empty queue")
	}
	q.app.queueState.ActiveTab = uistate.QueueTabOptional
	if q.installCurrentOptional() || q.dismissCurrentOptional() {
		t.Fatal("optional actions ran without a reminder")
	}

	// Unknown keys and q toggle the dock.
	q.app.queueState.ActiveTab = uistate.QueueTabDownloads
	if q.HandleKey(tcell.NewEventKey(tcell.KeyF1, 0, tcell.ModNone)) {
		t.Fatal("function key handled")
	}
	if q.HandleKey(tcell.NewEventKey(tcell.KeyRune, 'z', tcell.ModNone)) {
		t.Fatal("unknown rune handled")
	}
	if !q.HandleKey(tcell.NewEventKey(tcell.KeyRune, 'q', tcell.ModNone)) {
		t.Fatal("q not handled")
	}
	if q.Visible() {
		t.Fatal("q did not close the dock")
	}
}

func TestQueueDockWidthEdgeCases(t *testing.T) {
	h := newTestApp(t, 100, 30)
	q := h.app.queue
	h.app.width = 0
	if got := q.Width(); got != 0 {
		t.Fatalf("zero-width dock=%d", got)
	}
	h.app.width = 30
	if got := q.Width(); got != 28 {
		t.Fatalf("narrow dock=%d, want 28", got)
	}
	h.app.width = 40
	if got := q.Width(); got > 40 {
		t.Fatalf("wide dock=%d", got)
	}
	h.app.width = 90 // sidebar leaves 64; dock clamps to 28
	if got := q.Width(); got != 28 {
		t.Fatalf("clamped dock=%d", got)
	}
}

// --- app shell ---------------------------------------------------------------

func TestAppShellEdgeCases(t *testing.T) {
	h := newTestApp(t, 100, 30)

	// No event pump exists in the offline harness.
	h.app.waitEventPump()
	h.app.stopEventPump()
	h.app.stopEventPump()

	// Zero-value helpers must not panic.
	var bare App
	bare.updateStatus()
	bare.updateSidebarFooter()
	bare.resizeQueue()

	if h.app.handleGlobalKey(nil) != nil {
		t.Fatal("nil key was not ignored")
	}

	active := h.app.active
	h.app.switchView(-1)
	h.app.switchView(ViewID(viewCount))
	if h.app.active != active {
		t.Fatalf("invalid switch changed the page to %v", h.app.active)
	}

	h.app.mc.SelectedMinecraftVersion = ""
	h.app.mc.SelectedModLoader = ""
	if got := h.app.targetTuple(); got != "— | —" {
		t.Fatalf("empty tuple=%q", got)
	}

	// Compact sidebars show the connector marker.
	h.app.width = 80
	h.app.beforeDraw(h.sim)
	h.app.mc.SelectedVersion = &structs.VersionInfo{
		ID: "inst", Name: "inst", MinecraftVersion: "1.20.1", ModLoader: "Fabric",
		ConnectorAvailable: true, ConnectorVirtual: true, ActualModLoader: "Forge",
	}
	h.app.updateSidebarFooter()
	if !strings.Contains(h.app.sidebarFoot.GetText(true), "⇄") {
		t.Fatalf("compact footer=%q", h.app.sidebarFoot.GetText(true))
	}
	h.app.mc.SelectedVersion = nil

	// Reminder-only queues can be cleared with Q.
	h.app.queueState.Queue.OptionalReminders = []appstructs.OptionalDependencyReminder{{ID: "r"}}
	h.rune('Q')
	h.app.queueState.Queue.OptionalReminders = nil

	// Busy refreshes repaint an open target dialog.
	h.app.openTargetDialog()
	h.app.refreshBusyViews()
	h.key(tcell.KeyEsc)

	// Spinner start is idempotent and both counts must end.
	h.app.beginBusy("one")
	h.app.beginBusy("two")
	h.app.startSpinner()
	h.app.endBusy()
	h.app.endBusy()
	h.app.endBusy()
	h.app.stopSpinner()

	// Blank toasts are ignored.
	h.app.toast("   ")
	if h.app.toastText != "" {
		t.Fatalf("blank toast=%q", h.app.toastText)
	}

	// Mouse move passes through while a dialog is open.
	h.app.openConfirm("t", "b", []confirmOption{{Label: "ok", Value: "ok"}}, nil)
	ev := tcell.NewEventMouse(1, 1, tcell.ButtonNone, tcell.ModNone)
	if got, action := h.app.handleGlobalMouse(ev, tview.MouseMove); got != ev || action != tview.MouseMove {
		t.Fatalf("mouse move under dialog: %v %v", got, action)
	}
	h.key(tcell.KeyEsc)
}

func TestTextInputFocusedFormBranches(t *testing.T) {
	h := newTestApp(t, 100, 30)
	form := tview.NewForm()
	form.AddInputField("name", "", 10, nil, nil)
	form.AddDropDown("mode", []string{"a", "b"}, 0, nil)
	h.app.application.SetFocus(form)

	form.SetFocus(0)
	if !h.app.textInputFocused() {
		t.Fatal("form input field not detected as text input")
	}
	if h.app.dropdownOpen() {
		t.Fatal("closed form dropdown reported open")
	}

	form.SetFocus(1)
	h.app.application.SetFocus(form)
	if h.app.textInputFocused() {
		t.Fatal("closed form dropdown reported as text input")
	}

	// A bare input and text area are text inputs too.
	input := tview.NewInputField()
	h.app.application.SetFocus(input)
	if !h.app.textInputFocused() {
		t.Fatal("bare input field not detected")
	}
	area := tview.NewTextArea()
	h.app.application.SetFocus(area)
	if !h.app.textInputFocused() {
		t.Fatal("text area not detected")
	}
}

// --- target dialog -----------------------------------------------------------

func targetRowIndex(v *targetDialog, kind targetRowKind) int {
	for i, row := range v.rows {
		if row.kind == kind {
			return i
		}
	}
	return -1
}

func TestTargetDialogActivationAndAdjust(t *testing.T) {
	h := newTestApp(t, 100, 30)
	h.app.mc.MinecraftDir = t.TempDir()
	h.app.mc.SelectedVersion = &structs.VersionInfo{ID: "inst", Name: "inst", MinecraftVersion: "1.20.1", ModLoader: "Fabric"}
	h.app.mc.SelectedMinecraftVersion = "1.20.1"
	h.app.mc.SelectedModLoader = "Fabric"
	h.app.mc.ReleaseVersions = []string{"1.20.1", "1.19.2"}
	h.app.mc.Versions = []structs.VersionInfo{
		{ID: "inst", Name: "Inst", MinecraftVersion: "1.20.1", ModLoader: "Fabric"},
		{ID: "", MinecraftVersion: "1.19.2"}, // no name/id: target still appears as detail
	}
	h.app.openTargetDialog()
	v := h.app.targetDialog
	if v == nil {
		t.Fatal("target dialog missing")
	}

	// Out-of-range activation and non-adjustable rows are no-ops.
	v.activate(-1)
	v.activate(len(v.rows))
	v.list.SetCurrentItem(targetRowIndex(v, targetRowDir))
	v.adjust(1)

	// Directory row opens the path picker.
	v.activate(targetRowIndex(v, targetRowDir))
	if top := h.app.topDialog(); top == nil || !strings.HasPrefix(top.name, "path") {
		t.Fatalf("dir row dialog=%v", top)
	}
	h.key(tcell.KeyEsc)

	// Instance picker lists versions and can be cancelled. The name stays clean
	// while the secondary line carries the target.
	v.activate(targetRowIndex(v, targetRowInstance))
	if top := h.app.topDialog(); top == nil || !strings.HasPrefix(top.name, "select") {
		t.Fatalf("instance picker=%v", top)
	}
	h.draw()
	h.mustContain("Inst")
	h.mustContain("1.20.1 | Fabric")
	h.key(tcell.KeyEsc)

	// Refresh rows drive guarded controllers.
	v.activate(targetRowIndex(v, targetRowRefreshMods))
	v.activate(targetRowIndex(v, targetRowRefreshVersions))

	// Version/loader rows cycle the tuple when activated.
	v.list.SetCurrentItem(targetRowIndex(v, targetRowMCVersion))
	v.activate(targetRowIndex(v, targetRowMCVersion))
	if h.app.mc.SelectedMinecraftVersion != "1.19.2" {
		t.Fatalf("mc version=%q", h.app.mc.SelectedMinecraftVersion)
	}
	v.list.SetCurrentItem(targetRowIndex(v, targetRowLoader))
	v.activate(targetRowIndex(v, targetRowLoader))
	if h.app.mc.SelectedModLoader != "Forge" {
		t.Fatalf("loader=%q", h.app.mc.SelectedModLoader)
	}
	v.adjust(-1)
	if h.app.mc.SelectedModLoader != "Fabric" {
		t.Fatalf("loader after left=%q", h.app.mc.SelectedModLoader)
	}

	// Empty version list: the picker reports a toast instead of opening.
	h.app.mc.Versions = nil
	h.app.mc.SelectedVersion = nil
	v.activate(targetRowIndex(v, targetRowInstance))
	if h.app.toastText != i18n.T("home.status.notAvailable") {
		t.Fatalf("empty instance toast=%q", h.app.toastText)
	}
	if h.app.toastTimer != nil {
		h.app.toastTimer.Stop()
		h.app.toastTimer = nil
	}

	// Refresh is nil-safe both ways.
	var nilTarget *targetDialog
	nilTarget.refresh()
	v.refresh()

	// Instance labels contain only the instance name; the target is shown in
	// dedicated rows instead of being appended to every name.
	if got := versionLabel(structs.VersionInfo{ID: "id-only"}); got != "id-only" {
		t.Fatalf("label=%q", got)
	}
	if got := versionLabel(structs.VersionInfo{Name: "Name"}); got != "Name" {
		t.Fatalf("label=%q", got)
	}
	if got := versionLabel(structs.VersionInfo{Name: "Name", MinecraftVersion: "1.20.1", ModLoader: "Fabric"}); got != "Name" {
		t.Fatalf("label=%q, want just the name", got)
	}

	items := instancePickerItems([]structs.VersionInfo{
		{Name: "Fabric Pack", ID: "fabric-1.20.1", MinecraftVersion: "1.20.1", ModLoader: "fabric"},
		{Name: "1.20.1", ID: "1.20.1", MinecraftVersion: "1.20.1"},
		{Name: "Pack", ID: "folder-id"},
		{Name: "Same", ID: "Same"},
	})
	if len(items) != 4 {
		t.Fatalf("picker items=%d", len(items))
	}
	if items[0].Label != "Fabric Pack" || strings.Contains(items[0].Label, "1.20.1") {
		t.Fatalf("picker label repeats the target: %q", items[0].Label)
	}
	if items[0].Detail != "1.20.1 | Fabric" {
		t.Fatalf("picker detail=%q, want the target summary", items[0].Detail)
	}
	if items[1].Detail != "1.20.1" {
		t.Fatalf("picker detail=%q, want the mc version", items[1].Detail)
	}
	// Without target metadata the distinct folder ID is shown for context, but
	// an ID identical to the name is not repeated.
	if items[2].Detail != "folder-id" {
		t.Fatalf("picker detail=%q, want the distinct id", items[2].Detail)
	}
	if items[3].Detail != "" {
		t.Fatalf("picker duplicated name/id in detail: %q", items[3].Detail)
	}
}

func TestTargetDialogBuildsConnectorRow(t *testing.T) {
	h := newTestApp(t, 100, 30)
	h.app.mc.SelectedVersion = &structs.VersionInfo{
		ID: "inst", Name: "inst", ConnectorAvailable: true, ConnectorVirtual: true, ActualModLoader: "Forge",
	}
	h.app.openTargetDialog()
	v := h.app.targetDialog
	if targetRowIndex(v, targetRowConnector) < 0 {
		t.Fatal("connector row missing")
	}
	row := v.rows[targetRowIndex(v, targetRowConnector)]
	if !strings.Contains(row.value, "Forge") {
		t.Fatalf("connector label=%q", row.value)
	}
	h.key(tcell.KeyEsc)
}

// --- path dialog -------------------------------------------------------------

func TestPathDialogKeyboardNavigation(t *testing.T) {
	dir := t.TempDir()
	if err := os.Mkdir(filepath.Join(dir, "alpha"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "alpha.txt"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	h := newTestApp(t, 100, 30)

	accepted := ""
	v := h.app.openPathDialog("pick", false, dir, func(path string) { accepted = path })
	if h.app.application.GetFocus() != v.input {
		t.Fatal("path dialog did not focus the input")
	}

	// Tab completes while the input owns focus; from a button it is ignored.
	v.handleKey(tcell.NewEventKey(tcell.KeyTab, 0, tcell.ModNone))
	if h.app.application.GetFocus() != v.input {
		t.Fatal("tab moved focus out of the input")
	}
	v.input.SetText(filepath.Join(dir, "al"))
	v.handleKey(tcell.NewEventKey(tcell.KeyTab, 0, tcell.ModNone))
	if v.completions == nil {
		t.Fatal("tab did not compute completions")
	}

	// Backtab moves to the confirm button, then cycles to cancel.
	v.handleKey(tcell.NewEventKey(tcell.KeyBacktab, 0, tcell.ModNone))
	if h.app.application.GetFocus() != v.buttons[0] {
		t.Fatal("backtab did not focus confirm")
	}
	v.handleKey(tcell.NewEventKey(tcell.KeyBacktab, 0, tcell.ModNone))
	if h.app.application.GetFocus() != v.buttons[1] {
		t.Fatal("backtab did not cycle to cancel")
	}
	if v.handleKey(tcell.NewEventKey(tcell.KeyTab, 0, tcell.ModNone)) {
		t.Fatal("tab from a button was consumed")
	}

	// Left/right cycle buttons; left/right from the input fall through.
	v.handleKey(tcell.NewEventKey(tcell.KeyLeft, 0, tcell.ModNone))
	if h.app.application.GetFocus() != v.buttons[0] {
		t.Fatal("left did not cycle to confirm")
	}
	v.handleKey(tcell.NewEventKey(tcell.KeyRight, 0, tcell.ModNone))
	if h.app.application.GetFocus() != v.buttons[1] {
		t.Fatal("right did not cycle to cancel")
	}
	h.app.application.SetFocus(v.input)
	if v.handleKey(tcell.NewEventKey(tcell.KeyLeft, 0, tcell.ModNone)) {
		t.Fatal("left from the input was consumed")
	}
	if v.handleKey(tcell.NewEventKey(tcell.KeyRight, 0, tcell.ModNone)) {
		t.Fatal("right from the input was consumed")
	}

	// Enter on the cancel button closes without accepting.
	h.app.application.SetFocus(v.buttons[1])
	v.handleKey(tcell.NewEventKey(tcell.KeyEnter, 0, tcell.ModNone))
	if h.app.topDialog() != nil {
		t.Fatal("cancel button did not close the dialog")
	}
	if accepted != "" {
		t.Fatalf("cancel accepted %q", accepted)
	}

	// Enter on the input accepts a valid directory.
	v = h.app.openPathDialog("pick", false, dir, func(path string) { accepted = path })
	h.app.application.SetFocus(v.input)
	v.handleKey(tcell.NewEventKey(tcell.KeyEnter, 0, tcell.ModNone))
	if accepted != dir {
		t.Fatalf("accepted=%q, want %q", accepted, dir)
	}
	if h.app.topDialog() != nil {
		t.Fatal("accept did not close the dialog")
	}

	// Ctrl+S and Ctrl+Y also accept.
	for _, key := range []tcell.Key{tcell.KeyCtrlS, tcell.KeyCtrlY} {
		accepted = ""
		v = h.app.openPathDialog("pick", false, dir, func(path string) { accepted = path })
		h.app.application.SetFocus(v.input)
		v.handleKey(tcell.NewEventKey(key, 0, tcell.ModNone))
		if accepted != dir {
			t.Fatalf("%v accepted=%q", key, accepted)
		}
	}

	// An empty path is rejected and keeps the dialog open.
	v = h.app.openPathDialog("pick", false, dir, nil)
	h.app.application.SetFocus(v.input)
	v.input.SetText("")
	v.handleKey(tcell.NewEventKey(tcell.KeyEnter, 0, tcell.ModNone))
	if h.app.topDialog() == nil {
		t.Fatal("empty path closed the dialog")
	}
	if !strings.Contains(v.err.GetText(true), uiText("invalidDirectory")) {
		t.Fatalf("error=%q", v.err.GetText(true))
	}
	h.key(tcell.KeyEsc)
}

// --- disable-impact dialog ---------------------------------------------------

func TestImpactDialogRestoreGuards(t *testing.T) {
	h := newTestApp(t, 100, 30)
	h.app.openDisableImpactDialog(nil, "", appstructs.LocalModDisableImpactResult{
		Impacts: []appstructs.LocalModDependencyImpact{
			{ModID: "no-cache", AffectedMods: []appstructs.LocalModDependencyMod{{Name: "A"}}},
			{ModID: "many", Candidates: []appstructs.CachedDependencyCandidate{
				{ModID: "many", Platform: "modrinth", ProjectID: "p1", VersionID: "v1"},
				{ModID: "many", Platform: "modrinth", ProjectID: "p2", VersionID: "v2"},
			}},
		},
	})
	v := h.app.impactDialog
	if v == nil {
		t.Fatal("impact dialog missing")
	}
	if v.handleKey(tcell.NewEventKey(tcell.KeyF1, 0, tcell.ModNone)) {
		t.Fatal("function key handled")
	}

	// While a restore is in flight another restore is ignored.
	v.restoring = "busy"
	v.restoreCurrent()
	if v.restoring != "busy" {
		t.Fatalf("restoring=%q", v.restoring)
	}
	v.restoring = ""

	// Rows with no cached candidates report a cache miss.
	v.list.SetCurrentItem(0)
	v.restoreCurrent()
	if h.app.toastText != i18n.T("manage.disableImpact.cacheMiss") {
		t.Fatalf("cache miss toast=%q", h.app.toastText)
	}
	if h.app.toastTimer != nil {
		h.app.toastTimer.Stop()
		h.app.toastTimer = nil
	}

	// Already-restored rows are ignored.
	v.restored["no-cache"] = true
	v.restoreCurrent()
	if v.restoring != "" {
		t.Fatal("restored row started a restore")
	}
	v.restored = map[string]bool{}

	// Rows with several candidates open the source picker.
	v.list.SetCurrentItem(1)
	v.restoreCurrent()
	if top := h.app.topDialog(); top == nil || !strings.HasPrefix(top.name, "select") {
		t.Fatalf("candidate picker=%v", top)
	}
	h.key(tcell.KeyEsc)

	// Enter continues with the pending groups; nil groups means nothing runs.
	v.handleKey(tcell.NewEventKey(tcell.KeyEnter, 0, tcell.ModNone))
	if h.app.topDialog() != nil {
		t.Fatalf("enter did not close the impact dialog: %v", h.app.topDialog().name)
	}

	// restoreDone is nil-safe and reports both outcomes.
	var nilDialog *disableImpactDialog
	nilDialog.restoreDone("x", true)
	h.app.openDisableImpactDialog(nil, "", appstructs.LocalModDisableImpactResult{})
	d := h.app.impactDialog
	d.restoring = "mod"
	d.restoreDone("mod", true)
	if !d.restored["mod"] {
		t.Fatal("queued restore was not recorded")
	}
	d.restoring = "mod2"
	d.restoreDone("mod2", false)
	if d.restored["mod2"] {
		t.Fatal("failed restore was recorded as done")
	}
	h.key(tcell.KeyEsc)
}

// --- wheel form --------------------------------------------------------------

func TestWheelFormEdgeCases(t *testing.T) {
	h := newTestApp(t, 100, 30)
	f := newWheelForm()
	f.SetRect(0, 0, 60, 10)
	f.AddInputField("a", "", 10, nil, nil)
	f.AddInputField("b", "", 10, nil, nil)
	f.AddButton("go", func() {})

	if got := f.contentHeight(); got <= 0 {
		t.Fatalf("contentHeight=%d", got)
	}
	if f.focusable(-1) || f.focusable(f.GetFormItemCount()+f.GetButtonCount()) {
		t.Fatal("out-of-range element reported focusable")
	}

	// A form without focus returns -1 and skips to the first focusable one.
	f.Blur()
	f.moveFocusToViewport()

	// Focus the button branch and resolve it.
	f.SetFocus(f.GetFormItemCount())
	if got := f.currentIndex(); got != f.GetFormItemCount() {
		t.Fatalf("button index=%d", got)
	}
	if el := f.focusedElement(); el != f.GetButton(0) {
		t.Fatalf("focused button=%T", el)
	}

	// Scrolling is bounded when the content fits.
	f.offset = 0
	f.scroll(1)
	if f.offset != 0 {
		t.Fatalf("offset=%d", f.offset)
	}

	// Mouse wheel inside the rect scrolls; outside/other events fall through.
	setFocus := func(tview.Primitive) {}
	inside := tcell.NewEventMouse(5, 5, tcell.ButtonNone, tcell.ModNone)
	handler := f.MouseHandler()
	if consumed, _ := handler(tview.MouseScrollDown, inside, setFocus); !consumed {
		t.Fatal("wheel-down inside the form was not consumed")
	}
	if consumed, _ := handler(tview.MouseScrollUp, inside, setFocus); !consumed {
		t.Fatal("wheel-up inside the form was not consumed")
	}
	outside := tcell.NewEventMouse(500, 500, tcell.ButtonNone, tcell.ModNone)
	handler(tview.MouseLeftClick, outside, setFocus) // must not panic

	// A form with many items gets a positive scroll range.
	tall := newWheelForm()
	tall.SetRect(0, 0, 60, 3)
	for i := 0; i < 8; i++ {
		tall.AddInputField("i", "", 10, nil, nil)
	}
	tall.scroll(3)
	if tall.offset == 0 {
		t.Fatal("tall form did not scroll")
	}
	tall.offset = 0
	tall.drawAt(h.sim, 0, 0, 60, 3, tall.contentHeight())

	// No items means no focused element.
	empty := newWheelForm()
	empty.AddTextView("section", "", 0, 1, true, false)
	if el := empty.focusedElement(); el != nil {
		t.Fatalf("empty focusedElement=%T", el)
	}
}

// --- theme / formatting ------------------------------------------------------

func TestThemeDefaultColorTags(t *testing.T) {
	var empty Theme
	if got := empty.tag(tcell.ColorBlack); got != "" {
		t.Fatalf("black tag=%q", got)
	}
	if got := empty.tagWith(tcell.ColorBlack, "b"); got != "" {
		t.Fatalf("black tagWith=%q", got)
	}
	theme := resolveTheme("dark")
	if got := theme.tagWith(theme.Primary, ""); got == "" || strings.Contains(got, "::") {
		t.Fatalf("tagWith empty attrs=%q", got)
	}
	if got := theme.tagWith(theme.Primary, "b"); !strings.Contains(got, "::b") {
		t.Fatalf("tagWith bold=%q", got)
	}
}

// --- dialogs -----------------------------------------------------------------

func TestDialogEdgeCases(t *testing.T) {
	h := newTestApp(t, 100, 30)

	// A nil dialog is ignored.
	h.app.pushDialog(nil)
	if h.app.topDialog() != nil {
		t.Fatal("nil dialog was pushed")
	}

	// Duplicate names are de-duplicated; pop restores focus.
	h.app.pushDialog(&dialog{name: "dup", page: tview.NewBox()})
	h.app.pushDialog(&dialog{name: "dup", page: tview.NewBox()})
	if len(h.app.dialogs) != 1 {
		t.Fatalf("dialogs=%d, want 1", len(h.app.dialogs))
	}
	h.app.popDialog()
	if h.app.topDialog() != nil {
		t.Fatal("pop left a dialog")
	}

	// Confirm without buttons handles navigation and Enter safely.
	h.app.openConfirm("empty", "body", nil, nil)
	v := h.app.dialogs[len(h.app.dialogs)-1]
	if !v.handleKey(tcell.NewEventKey(tcell.KeyLeft, 0, tcell.ModNone)) {
		t.Fatal("left without buttons was not consumed")
	}
	if !v.handleKey(tcell.NewEventKey(tcell.KeyEnter, 0, tcell.ModNone)) {
		t.Fatal("enter without buttons was not consumed")
	}
	if !v.handleKey(tcell.NewEventKey(tcell.KeyPgUp, 0, tcell.ModNone)) {
		t.Fatal("page-up was not consumed")
	}
	if v.handleKey(tcell.NewEventKey(tcell.KeyF1, 0, tcell.ModNone)) {
		t.Fatal("function key consumed")
	}
	h.key(tcell.KeyEsc)

	// Confirm activation with an invalid index is a no-op.
	confirm := &confirmView{app: h.app, options: []confirmOption{{Label: "a", Value: "a"}}}
	confirm.activate(-1)
	confirm.activate(1)

	// Tiny screens clamp centered dialogs to at least one cell.
	for _, size := range [][2]int{{1, 1}, {10, 3}} {
		sim := tcell.NewSimulationScreen("UTF-8")
		if err := sim.Init(); err != nil {
			t.Fatal(err)
		}
		sim.SetSize(size[0], size[1])
		c := newCentered(tview.NewBox(), 0, 0)
		c.Draw(sim)
		if _, _, w, h := c.GetRect(); w < 1 || h < 1 {
			t.Fatalf("%v centered rect=%dx%d", size, w, h)
		}
		c.SetRect(1, 1, 2, 2)
		if _, _, w, h := c.GetRect(); w != 2 || h != 2 {
			t.Fatalf("SetRect=%dx%d", w, h)
		}
		sim.Fini()
	}

	// Centered mouse handling: nil events, padding clicks and content clicks.
	c := newCentered(tview.NewBox(), 20, 10)
	c.SetRect(10, 5, 20, 10)
	handler := c.MouseHandler()
	if consumed, _ := handler(tview.MouseLeftClick, nil, func(tview.Primitive) {}); consumed {
		t.Fatal("nil mouse event was consumed")
	}
	padding := tcell.NewEventMouse(12, 6, tcell.Button1, tcell.ModNone)
	if consumed, _ := handler(tview.MouseLeftClick, padding, func(tview.Primitive) {}); !consumed {
		t.Fatal("padding click was not swallowed")
	}
	outside := tcell.NewEventMouse(0, 0, tcell.Button1, tcell.ModNone)
	if consumed, _ := handler(tview.MouseLeftClick, outside, func(tview.Primitive) {}); consumed {
		t.Fatal("outside click was swallowed")
	}

	// Help dialog renders the shortcut table.
	h.app.openHelpDialog()
	h.draw()
	if !strings.Contains(h.text(), "Ctrl+C") {
		t.Fatalf("help missing shortcut:\n%s", h.text())
	}
	h.key(tcell.KeyEsc)
}
