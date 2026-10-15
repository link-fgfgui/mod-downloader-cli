package tui

import (
	"strings"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/link-fgfgui/mod-downloader-core/storage"

	"github.com/link-fgfgui/mod-downloader-cli/tui/i18n"
)

// homeModel renders the usage-statistics dashboard (Phase 2).
type homeModel struct {
	app           *AppModel
	stats         storage.UsageStats
	loadError     bool
	active        bool
	refreshQueued bool
}

// ApplyStats mirrors the `usage-stats-updated` listener.
func (v *homeModel) ApplyStats(stats storage.UsageStats) {
	v.stats = stats
	v.loadError = false
	if !v.active {
		v.refreshQueued = true
	}
}

func (v *homeModel) Update(app *AppModel, msg tea.Msg) tea.Cmd {
	switch msg.(type) {
	case viewActivatedMsg:
		v.active = true
		if v.refreshQueued {
			v.refreshQueued = false
		}
		return cmdGetUsageStats(app.host)
	case UsageStatsUpdatedMsg:
		v.ApplyStats(app.home.stats) // value already applied by the shell
		return nil
	case settingsSavedMsg, settingsLoadedMsg, localModsMsg, favoritesLoadedMsg, pinnedModsMsg:
		return nil
	}
	if _, ok := msg.(tea.KeyMsg); ok {
		// "r" refreshes the board (mirrors the GUI refresh button).
		if key, ok := msg.(tea.KeyMsg); ok && key.String() == "r" {
			return cmdGetUsageStats(app.host)
		}
	}
	return nil
}

func (v *homeModel) View(app *AppModel) string {
	t := app.theme
	stats := v.stats
	var b strings.Builder

	b.WriteString(t.Accent().Render(i18n.T("home.kicker")))
	b.WriteString("\n")
	b.WriteString(t.Title().Render(i18n.T("home.title")))
	b.WriteString("\n")
	b.WriteString(t.Muted().Render(i18n.T("home.subtitle")))
	b.WriteString("\n\n")

	status := t.Success().Render("● " + i18n.T("home.status.tracking"))
	if v.loadError {
		status = t.Danger().Render("● " + i18n.T("home.status.unavailable"))
	}
	b.WriteString(status)
	b.WriteString("\n\n")

	total := stats.DownloadsCompleted + stats.ModsEnabled + stats.ModsDisabled +
		stats.ModsDeleted + stats.FavoritesAdded + stats.PackwizExports

	b.WriteString(t.Title().Render(i18n.T("home.usage.title")))
	b.WriteString("  " + t.Muted().Render(i18n.T("home.usage.sinceTracking")))
	b.WriteString("\n")
	b.WriteString(t.OnSurface().Render(compactNumber(total)) + "  " + t.Muted().Render(i18n.T("home.usage.total")))
	b.WriteString("\n\n")

	metrics := []struct {
		label string
		value int64
		style func(...string) string
	}{
		{i18n.T("home.usage.downloads"), stats.DownloadsCompleted, t.Accent().Render},
		{i18n.T("home.usage.enabled"), stats.ModsEnabled, t.Success().Render},
		{i18n.T("home.usage.disabled"), stats.ModsDisabled, t.Warning().Render},
		{i18n.T("home.usage.deleted"), stats.ModsDeleted, t.Danger().Render},
		{i18n.T("home.usage.favorites"), stats.FavoritesAdded, t.Accent().Render},
		{i18n.T("home.usage.exports"), stats.PackwizExports, t.Muted().Render},
	}

	// Three-column grid of metric cards, mirroring the GUI metric-grid.
	const cols = 3
	cellWidth := maxInt(18, (app.width-4)/cols)
	for i := 0; i < len(metrics); i += cols {
		row := []string{}
		for j := 0; j < cols && i+j < len(metrics); j++ {
			metric := metrics[i+j]
			line := " · " + metric.style(compactNumber(metric.value)) + "  " + metric.label
			row = append(row, padRight(truncate(line, cellWidth), cellWidth))
		}
		b.WriteString(strings.Join(row, "  "))
		b.WriteString("\n")
	}

	// Current status block (mirrors home.status).
	b.WriteString("\n")
	b.WriteString(t.Title().Render(i18n.T("home.status.title")))
	b.WriteString("\n")
	b.WriteString(t.Muted().Render(i18n.T("home.status.instance") + ": "))
	b.WriteString(t.OnSurface().Render(app.mc.SelectedInstanceLabel()))
	b.WriteString("\n")
	b.WriteString(t.Muted().Render(i18n.T("home.status.minecraftDir") + ": "))
	dir := app.mc.MinecraftDir
	if dir == "" {
		dir = i18n.T("home.status.notConfigured")
	}
	b.WriteString(t.OnSurface().Render(dir))
	b.WriteString("\n")
	b.WriteString(t.Muted().Render(i18n.T("home.status.queue") + ": "))
	if app.queue.Queue.Active {
		b.WriteString(t.Accent().Render(i18n.T("home.status.active")))
	} else {
		b.WriteString(t.Muted().Render(i18n.T("home.status.idle")))
	}
	b.WriteString("  " + i18n.T("home.status.running") + " " + itoa(app.queue.Queue.Running) +
		" · " + i18n.T("home.status.pending") + " " + itoa(app.queue.Queue.Pending))
	b.WriteString("\n")
	b.WriteString(t.Muted().Render(i18n.T("home.status.source") + ": "))
	if app.settingsView.MCIMEnabled {
		b.WriteString(t.Accent().Render(i18n.T("home.status.mcim")))
	} else {
		b.WriteString(t.OnSurface().Render(i18n.T("home.status.official")))
	}
	return b.String()
}

func (v *homeModel) FooterHints(app *AppModel) string {
	return "r " + i18n.T("home.refresh") + " · 1-6 / ⇥ " + uiText("navigate") + " · q " + i18n.T("download.queue.title")
}

func (v *homeModel) ClearFocus(app *AppModel) (bool, tea.Cmd) { return false, nil }

func (v *homeModel) CapturesKeys(app *AppModel) bool { return false }
