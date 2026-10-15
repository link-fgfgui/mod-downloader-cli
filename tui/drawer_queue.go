package tui

import (
	"strings"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/link-fgfgui/mod-downloader-cli/internal/uistate"
	"github.com/link-fgfgui/mod-downloader-cli/tui/i18n"
)

// queueDrawerKey handles keyboard interaction for the global download drawer.
func (m *AppModel) queueDrawerKey(key tea.KeyMsg) tea.Cmd {
	switch key.String() {
	case "tab", "shift+tab", "left", "right", "h", "l":
		if m.queue.ActiveTab == uistate.QueueTabDownloads {
			m.queue.ActiveTab = uistate.QueueTabOptional
		} else {
			m.queue.ActiveTab = uistate.QueueTabDownloads
		}
		m.queueCursor = 0
		return nil
	case "up", "k":
		m.moveQueueCursor(-1)
		return nil
	case "down", "j":
		m.moveQueueCursor(1)
		return nil
	}

	if m.queue.ActiveTab == uistate.QueueTabOptional {
		return m.queueOptionalKey(key)
	}
	return m.queueDownloadsKey(key)
}

func (m *AppModel) queueDownloadsKey(key tea.KeyMsg) tea.Cmd {
	items := m.queue.Queue.Items
	if len(items) == 0 {
		return nil
	}
	if m.queueCursor >= len(items) {
		m.queueCursor = len(items) - 1
	}
	item := items[m.queueCursor]
	switch key.String() {
	case "c":
		if item.Cancelable {
			return cmdCancelDownload(m.host, item.ID)
		}
	case "r":
		if item.Retryable {
			return cmdRetryDownload(m.host, item.ID)
		}
	case "x", "delete", "backspace":
		// Remove without confirmation; running entries stay protected (mirrors
		// download-queue-actions.md).
		if uistate.CanRemove(item.Status) {
			return cmdRemoveDownload(m.host, item.ID)
		} else {
			return m.toastT("download.queue.removeItem", toastWarning)
		}
	}
	return nil
}

// queueOptionalKey: installOptional (i), dismiss (d), clear all (a).
func (m *AppModel) queueOptionalKey(key tea.KeyMsg) tea.Cmd {
	reminders := m.queue.Queue.OptionalReminders
	switch key.String() {
	case "a":
		return cmdClearOptionalReminders(m.host)
	}
	if len(reminders) == 0 {
		return nil
	}
	if m.queueCursor >= len(reminders) {
		m.queueCursor = len(reminders) - 1
	}
	reminder := reminders[m.queueCursor]
	switch key.String() {
	case "i":
		return cmdInstallOptionalDependencies(m.host, reminder.ID)
	case "d":
		return cmdDismissOptionalReminder(m.host, reminder.ID)
	}
	return nil
}

func (m *AppModel) moveQueueCursor(delta int) {
	var count int
	if m.queue.ActiveTab == uistate.QueueTabOptional {
		count = len(m.queue.Queue.OptionalReminders)
	} else {
		count = len(m.queue.Queue.Items)
	}
	if count == 0 {
		m.queueCursor = 0
		return
	}
	m.queueCursor += delta
	if m.queueCursor < 0 {
		m.queueCursor = 0
	}
	if m.queueCursor >= count {
		m.queueCursor = count - 1
	}
}

// renderQueueDrawer renders the drawer panel (centered overlay).
func (m *AppModel) renderQueueDrawer() string {
	t := m.theme
	q := m.queue
	var b strings.Builder

	b.WriteString(t.Title().Render(i18n.T("download.queue.title")))
	b.WriteString("\n")

	// Summary header: running/pending, total progress, total speed.
	summary := i18n.T("download.queue.summary", map[string]string{
		"running": itoa(q.Queue.Running),
		"pending": itoa(q.Queue.Pending),
	})
	b.WriteString(t.OnSurface().Render(summary))
	if total := q.Queue.TotalBytes; total > 0 {
		b.WriteString("  ")
		b.WriteString(progressBar(q.Queue.BytesComplete, total, 20))
		b.WriteString(" " + formatPercent(q.Queue.BytesComplete, total))
	} else if q.Queue.Active {
		b.WriteString("  ")
		b.WriteString(t.Muted().Render(i18n.T("download.queue.unknownSize")))
	}
	if q.Queue.BytesPerSecond > 0 {
		b.WriteString("  " + t.Accent().Render(formatSpeed(q.Queue.BytesPerSecond)))
	}
	b.WriteString("\n")

	// Tabs.
	dlTab := i18n.T("download.queue.tabs.downloads")
	opTab := i18n.T("download.queue.tabs.optional")
	if len(q.Queue.OptionalReminders) > 0 {
		opTab += " (" + itoa(len(q.Queue.OptionalReminders)) + ")"
	}
	if q.ActiveTab == uistate.QueueTabDownloads {
		b.WriteString(t.Selected().Render(" " + dlTab + " "))
		b.WriteString("  ")
		b.WriteString(t.Muted().Render(opTab))
	} else {
		b.WriteString(t.Muted().Render(dlTab))
		b.WriteString("  ")
		b.WriteString(t.Selected().Render(" " + opTab + " "))
	}
	b.WriteString("\n\n")

	if q.ActiveTab == uistate.QueueTabOptional {
		b.WriteString(m.renderOptionalReminders())
	} else {
		b.WriteString(m.renderQueueItems())
	}

	b.WriteString("\n")
	b.WriteString(t.Muted().Render(m.queueDrawerHints()))
	return b.String()
}

func (m *AppModel) renderQueueItems() string {
	t := m.theme
	items := m.queue.Queue.Items
	if len(items) == 0 {
		return t.Muted().Render(i18n.T("download.queue.emptyDownloads"))
	}
	var b strings.Builder
	const maxRows = 10
	start := 0
	if m.queueCursor >= maxRows {
		start = m.queueCursor - maxRows + 1
	}
	end := start + maxRows
	if end > len(items) {
		end = len(items)
	}
	for i := start; i < end; i++ {
		item := items[i]
		icon := queueStatusIcon(m.theme, item.Status)
		title := item.Title
		if title == "" {
			title = item.FileName
		}
		if title == "" {
			title = item.VersionID
		}
		line := "  " + icon + " " + truncate(title, maxInt(10, m.drawerWidth()-24))
		line += "  " + t.Muted().Render(item.Platform+" · "+item.FileName)
		line += "  " + progressBar(item.BytesComplete, item.TotalBytes, 12)
		if item.Reason != "" {
			line += "  " + t.Danger().Render(item.Reason)
		}
		if i == m.queueCursor {
			line = t.Selected().Render(stripStyles(line))
		}
		b.WriteString(line)
		b.WriteString("\n")
	}
	return strings.TrimRight(b.String(), "\n")
}

func (m *AppModel) renderOptionalReminders() string {
	t := m.theme
	reminders := m.queue.Queue.OptionalReminders
	if len(reminders) == 0 {
		return t.Muted().Render(i18n.T("download.queue.emptyOptional"))
	}
	var b strings.Builder
	const maxRows = 10
	start := 0
	if m.queueCursor >= maxRows {
		start = m.queueCursor - maxRows + 1
	}
	end := start + maxRows
	if end > len(reminders) {
		end = len(reminders)
	}
	for i := start; i < end; i++ {
		reminder := reminders[i]
		line := "  " + truncate(reminder.MainTitle, maxInt(10, m.drawerWidth()-16))
		if i == m.queueCursor {
			line = t.Selected().Render(stripStyles(line))
		} else {
			line = t.OnSurface().Render(line)
		}
		b.WriteString(line)
		b.WriteString("\n")
		for _, dep := range reminder.Dependencies {
			status := optionalStatusIcon(t, dep.Status)
			detail := dep.Title
			if detail == "" {
				detail = dep.ProjectID
			}
			depLine := "      " + status + " " + detail + " "
			depLine += t.Muted().Render(dep.Platform)
			if dep.Reason != "" {
				depLine += "  " + t.Muted().Render(dep.Reason)
			}
			if dep.Disabled {
				depLine = t.Muted().Render(stripStyles(depLine))
			}
			b.WriteString(depLine)
			b.WriteString("\n")
		}
	}
	return strings.TrimRight(b.String(), "\n")
}

func (m *AppModel) queueDrawerHints() string {
	if m.queue.ActiveTab == uistate.QueueTabOptional {
		return "i " + i18n.T("download.queue.installOptional") + " · d " + i18n.T("download.queue.dismissOptional") + " · a " + uiText("clearAll") + " · ⇥ tab · esc " + uiText("close")
	}
	return "c " + i18n.T("download.queue.cancel") + " · r " + i18n.T("download.queue.retry") + " · x " + i18n.T("download.queue.removeItem") + " · ⇥ tab · esc " + uiText("close")
}

// drawerWidth is the panel width used for truncation math.
func (m *AppModel) drawerWidth() int {
	w := m.width * 3 / 4
	if w > 96 {
		w = 96
	}
	if w < 40 {
		w = maxInt(40, m.width-4)
	}
	return w
}

func queueStatusIcon(t Theme, status string) string {
	switch status {
	case "running":
		return t.Accent().Render("▶")
	case "pending":
		return t.Muted().Render("…")
	case "failed":
		return t.Danger().Render("✗")
	case "canceled":
		return t.Warning().Render("⊘")
	default:
		return t.Muted().Render("•")
	}
}

func optionalStatusIcon(t Theme, status string) string {
	switch status {
	case "installed":
		return t.Success().Render("✓")
	case "update":
		return t.Warning().Render("↑")
	case "conflict":
		return t.Danger().Render("⚠")
	case "incompatible":
		return t.Danger().Render("✗")
	default:
		return t.Muted().Render("•")
	}
}

// stripStyles removes ANSI sequences so a line can be re-styled as a whole.
func stripStyles(value string) string {
	var b strings.Builder
	inEsc := false
	for _, r := range value {
		if inEsc {
			if r == 'm' {
				inEsc = false
			}
			continue
		}
		if r == '\x1b' {
			inEsc = true
			continue
		}
		b.WriteRune(r)
	}
	return b.String()
}
