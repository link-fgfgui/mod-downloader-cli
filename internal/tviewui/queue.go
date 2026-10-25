package tviewui

import (
	"strings"

	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"

	appstructs "github.com/link-fgfgui/mod-downloader-core/structs"

	"github.com/link-fgfgui/mod-downloader-cli/internal/i18n"
	"github.com/link-fgfgui/mod-downloader-cli/internal/uistate"
)

// queueDock is the right-side download queue. tview has no transparent overlay,
// so the dock shrinks the main area instead of floating above it.
type queueDock struct {
	app    *App
	open   bool
	flex   *tview.Flex
	header *tview.TextView
	tabs   *tview.Flex
	dlTab  *tview.Button
	opTab  *tview.Button
	list   *wheelList
	bar    *actionBar
	barTab string
	hints  *tview.TextView
}

func newQueueDock(a *App) *queueDock {
	q := &queueDock{app: a}

	q.header = tview.NewTextView().SetDynamicColors(true)
	q.header.SetBackgroundColor(a.theme.Surface)

	q.dlTab = tview.NewButton(i18n.T("download.queue.tabs.downloads"))
	q.dlTab.SetSelectedFunc(func() { q.setTab(uistate.QueueTabDownloads) })
	q.opTab = tview.NewButton(i18n.T("download.queue.tabs.optional"))
	q.opTab.SetSelectedFunc(func() { q.setTab(uistate.QueueTabOptional) })

	q.tabs = tview.NewFlex().SetDirection(tview.FlexColumn).
		AddItem(q.dlTab, 0, 1, false).
		AddItem(q.opTab, 0, 1, false)

	q.list = newWheelList()
	q.list.ShowSecondaryText(true)
	q.list.SetWrapAround(false)
	q.list.SetUseStyleTags(true, true)
	a.theme.styleList(q.list.List)

	q.bar = newActionBar(a)
	q.bar.width = q.Width
	q.bar.SetRefocus(func() { a.application.SetFocus(q.list) })

	q.hints = tview.NewTextView().SetDynamicColors(true)
	q.hints.SetBackgroundColor(a.theme.Surface)
	q.hints.SetTextColor(a.theme.Muted)

	q.flex = tview.NewFlex().SetDirection(tview.FlexRow).
		AddItem(q.header, 3, 0, false).
		AddItem(q.tabs, 1, 0, false).
		AddItem(q.list, 0, 1, true).
		AddItem(q.bar.Primitive(), 1, 0, false).
		AddItem(q.hints, 1, 0, false)
	q.flex.SetBorder(true)
	q.flex.SetTitle(i18n.T("download.queue.title"))
	a.theme.styleBox(q.flex.Box)
	q.Refresh()
	return q
}

func (q *queueDock) Primitive() tview.Primitive { return q.flex }

func (q *queueDock) Visible() bool { return q.open }

func (q *queueDock) SetOpen(open bool) {
	q.open = open
	if open {
		q.Refresh()
	}
}

func (q *queueDock) Focus() {
	q.app.application.SetFocus(q.list)
	q.app.updateStatus()
}

func (q *queueDock) Focused() bool {
	return q.open && q.flex.HasFocus()
}

// Width returns the dock width for the current terminal (clamped 28..40).
func (q *queueDock) Width() int {
	w := q.app.width - sidebarWidth(q.app.width)
	if w <= 0 {
		return 0
	}
	qw := w / 3
	if qw < 28 {
		qw = 28
	}
	if qw > 40 {
		qw = 40
	}
	if qw > w {
		qw = w
	}
	return qw
}

func (q *queueDock) Hints() []hint {
	closeHint := hintAction("esc "+uiText("close"), q.app.closeQueue)
	if q.app.queueState.ActiveTab == uistate.QueueTabOptional {
		return []hint{
			hintAction("i "+uiText("install"), func() { q.installCurrentOptional() }),
			hintAction("d "+uiText("dismiss"), func() { q.dismissCurrentOptional() }),
			hintAction("a "+uiText("clearAll"), func() { q.clearOptional() }),
			hintText("⇥ tab"),
			closeHint,
		}
	}
	return []hint{
		hintAction("c "+uiText("cancel"), func() { q.cancelCurrent() }),
		hintAction("r "+uiText("retry"), func() { q.retryCurrent() }),
		hintAction("x "+uiText("remove"), func() { q.removeCurrent() }),
		hintText("⇥ tab"),
		closeHint,
	}
}

// setTab switches the queue tab and rebuilds its list.
func (q *queueDock) setTab(tab string) {
	if q.app.queueState.ActiveTab == tab {
		return
	}
	q.app.queueState.ActiveTab = tab
	q.Refresh()
	q.app.updateStatus()
}

// HandleKey handles dock-specific navigation and actions.
func (q *queueDock) HandleKey(event *tcell.EventKey) bool {
	if !q.open {
		return false
	}
	if event.Key() == tcell.KeyTab || event.Key() == tcell.KeyBacktab ||
		event.Key() == tcell.KeyLeft || event.Key() == tcell.KeyRight {
		if q.app.queueState.ActiveTab == uistate.QueueTabDownloads {
			q.setTab(uistate.QueueTabOptional)
		} else {
			q.setTab(uistate.QueueTabDownloads)
		}
		return true
	}
	if event.Key() != tcell.KeyRune {
		return false
	}
	switch event.Rune() {
	case 'q':
		q.app.toggleQueue()
		return true
	case 'c':
		return q.cancelCurrent()
	case 'r':
		return q.retryCurrent()
	case 'x':
		return q.removeCurrent()
	case 'a':
		return q.clearOptional()
	case 'i':
		return q.installCurrentOptional()
	case 'd':
		return q.dismissCurrentOptional()
	}
	return false
}

// cancelCurrent cancels the highlighted download.
func (q *queueDock) cancelCurrent() bool {
	item, ok := q.currentItem()
	if !ok || !item.Cancelable {
		return false
	}
	go q.app.cancelDownload(item.ID)
	return true
}

// retryCurrent retries the highlighted failed download.
func (q *queueDock) retryCurrent() bool {
	item, ok := q.currentItem()
	if !ok || !item.Retryable {
		return false
	}
	go q.app.retryDownload(item.ID)
	return true
}

// removeCurrent drops the highlighted finished download.
func (q *queueDock) removeCurrent() bool {
	item, ok := q.currentItem()
	if !ok {
		return false
	}
	if !uistate.CanRemove(item.Status) {
		q.app.toastT("download.queue.removeItem")
		return true
	}
	go q.app.removeDownload(item.ID)
	return true
}

// installCurrentOptional queues the highlighted optional-dependency reminder.
func (q *queueDock) installCurrentOptional() bool {
	if q.app.queueState.ActiveTab != uistate.QueueTabOptional {
		return false
	}
	reminder, ok := q.currentReminder()
	if !ok {
		return false
	}
	go q.app.installOptionalDependencies(reminder.ID)
	return true
}

// dismissCurrentOptional dismisses the highlighted reminder.
func (q *queueDock) dismissCurrentOptional() bool {
	if q.app.queueState.ActiveTab != uistate.QueueTabOptional {
		return false
	}
	reminder, ok := q.currentReminder()
	if !ok {
		return false
	}
	go q.app.dismissOptionalReminder(reminder.ID)
	return true
}

// clearOptional dismisses every optional-dependency reminder.
func (q *queueDock) clearOptional() bool {
	if q.app.queueState.ActiveTab != uistate.QueueTabOptional {
		return false
	}
	go q.app.clearOptionalReminders()
	return true
}

func (q *queueDock) currentItem() (appstructs.DownloadQueueItem, bool) {
	items := q.app.queueState.Queue.Items
	idx := q.list.GetCurrentItem()
	if idx < 0 || idx >= len(items) {
		return appstructs.DownloadQueueItem{}, false
	}
	return items[idx], true
}

func (q *queueDock) currentReminder() (appstructs.OptionalDependencyReminder, bool) {
	reminders := q.app.queueState.Queue.OptionalReminders
	idx := q.list.GetCurrentItem()
	if idx < 0 || idx >= len(reminders) {
		return appstructs.OptionalDependencyReminder{}, false
	}
	return reminders[idx], true
}

// Refresh projects the queue state onto the dock primitives.
func (q *queueDock) Refresh() {
	if q == nil || q.app == nil {
		return
	}
	q.flex.SetTitle(i18n.T("download.queue.title"))
	q.renderHeader()
	q.renderTabs()
	q.renderList()
	q.hints.SetText(hintTextLine(q.Hints()))
}

func (q *queueDock) renderHeader() {
	state := q.app.queueState.Queue
	summary := i18n.T("download.queue.summary", map[string]string{
		"running": itoa(state.Running),
		"pending": itoa(state.Pending),
	})
	progress := ""
	if state.TotalBytes > 0 {
		progress = progressBar(state.BytesComplete, state.TotalBytes, 14) + " " + formatPercent(state.BytesComplete, state.TotalBytes)
	} else if state.Active {
		progress = i18n.T("download.queue.unknownSize")
	}
	if state.BytesPerSecond > 0 {
		if progress != "" {
			progress += "  "
		}
		progress += formatSpeed(state.BytesPerSecond)
	}
	t := q.app.theme
	text := t.tag(t.OnSurface) + summary + resetTag + "\n" + t.tag(t.Secondary) + progress + resetTag
	q.header.SetText(text)
}

func (q *queueDock) renderTabs() {
	active := q.app.queueState.ActiveTab
	downloads := i18n.T("download.queue.tabs.downloads")
	optional := i18n.T("download.queue.tabs.optional")
	if n := len(q.app.queueState.Queue.OptionalReminders); n > 0 {
		optional += " (" + itoa(n) + ")"
	}
	t := q.app.theme
	rebuildBar := q.barTab != active
	q.barTab = active
	if active == uistate.QueueTabDownloads {
		q.dlTab.SetLabel("▶ " + downloads)
		q.opTab.SetLabel(optional)
		q.dlTab.SetStyle(t.selectionStyle())
		q.opTab.SetStyle(tcell.StyleDefault.Background(t.Surface).Foreground(t.Muted))
		if rebuildBar {
			q.bar.Set(
				pageAction{Label: uiText("cancel"), Run: func() { q.cancelCurrent() }},
				pageAction{Label: uiText("retry"), Run: func() { q.retryCurrent() }},
				pageAction{Label: uiText("remove"), Run: func() { q.removeCurrent() }},
			)
		}
		return
	}
	q.dlTab.SetLabel(downloads)
	q.opTab.SetLabel("▶ " + optional)
	q.dlTab.SetStyle(tcell.StyleDefault.Background(t.Surface).Foreground(t.Muted))
	q.opTab.SetStyle(t.selectionStyle())
	if rebuildBar {
		q.bar.Set(
			pageAction{Label: uiText("install"), Run: func() { q.installCurrentOptional() }},
			pageAction{Label: uiText("dismiss"), Run: func() { q.dismissCurrentOptional() }},
			pageAction{Label: uiText("clearAll"), Run: func() { q.clearOptional() }},
		)
	}
}

func (q *queueDock) renderList() {
	previous := q.list.GetCurrentItem()
	q.list.Clear()
	if q.app.queueState.ActiveTab == uistate.QueueTabOptional {
		q.renderOptional()
	} else {
		q.renderDownloads()
	}
	count := q.list.GetItemCount()
	if previous >= 0 && previous < count {
		q.list.SetCurrentItem(previous)
	}
}

func (q *queueDock) renderDownloads() {
	t := q.app.theme
	items := q.app.queueState.Queue.Items
	if len(items) == 0 {
		q.list.AddItem(t.tag(t.Muted)+i18n.T("download.queue.emptyDownloads")+resetTag, "", 0, nil)
		return
	}
	for _, item := range items {
		icon := queueStatusIcon(t, item.Status)
		title := item.Title
		if title == "" {
			title = item.FileName
		}
		if title == "" {
			title = item.VersionID
		}
		if title == "" {
			title = item.ID
		}
		main := icon + " " + tview.Escape(title)
		detail := progressBar(item.BytesComplete, item.TotalBytes, 8)
		if item.TotalBytes > 0 {
			detail += " " + formatPercent(item.BytesComplete, item.TotalBytes)
		}
		meta := item.Platform
		if item.FileName != "" {
			if meta != "" {
				meta += " · "
			}
			meta += item.FileName
		}
		if meta != "" {
			detail += "  " + tview.Escape(meta)
		}
		if item.Reason != "" {
			detail += "  " + t.tag(t.Error) + tview.Escape(item.Reason) + resetTag
		}
		q.list.AddItem(main, detail, 0, nil)
	}
}

func (q *queueDock) renderOptional() {
	t := q.app.theme
	reminders := q.app.queueState.Queue.OptionalReminders
	if len(reminders) == 0 {
		q.list.AddItem(t.tag(t.Muted)+i18n.T("download.queue.emptyOptional")+resetTag, "", 0, nil)
		return
	}
	for _, reminder := range reminders {
		title := reminder.MainTitle
		if title == "" {
			title = reminder.ID
		}
		parts := make([]string, 0, len(reminder.Dependencies))
		for _, dep := range reminder.Dependencies {
			name := dep.Title
			if name == "" {
				name = dep.ProjectID
			}
			parts = append(parts, name)
		}
		detail := itoa(len(reminder.Dependencies)) + " " + uiText("items")
		if len(parts) > 0 {
			detail += ": " + strings.Join(parts, ", ")
		}
		q.list.AddItem(tview.Escape(title), tview.Escape(detail), 0, nil)
	}
}

func queueStatusIcon(t Theme, status string) string {
	switch status {
	case "running":
		return t.tag(t.Secondary) + "▶" + resetTag
	case "pending":
		return t.tag(t.Muted) + "…" + resetTag
	case "failed":
		return t.tag(t.Error) + "✗" + resetTag
	case "canceled":
		return t.tag(t.Warning) + "⊘" + resetTag
	default:
		return t.tag(t.Muted) + "•" + resetTag
	}
}

// restyle reapplies the theme after a palette change.
func (q *queueDock) restyle() {
	t := q.app.theme
	q.header.SetBackgroundColor(t.Surface)
	q.hints.SetBackgroundColor(t.Surface)
	q.hints.SetTextColor(t.Muted)
	t.styleList(q.list.List)
	t.styleBox(q.flex.Box)
	q.bar.Restyle()
	q.renderTabs()
}
