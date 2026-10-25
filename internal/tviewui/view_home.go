package tviewui

import (
	"strings"

	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"

	"github.com/link-fgfgui/mod-downloader-cli/internal/i18n"
)

// homePage renders the usage-statistics dashboard.
type homePage struct {
	app     *App
	root    *tview.Flex
	view    *tview.TextView
	bar     *actionBar
	barLang string
}

func newHomePage(a *App) *homePage {
	v := &homePage{app: a}
	v.view = tview.NewTextView().SetDynamicColors(true)
	v.view.SetScrollable(true)
	v.view.SetBorder(true)
	v.view.SetTitle(i18n.T("home.title"))
	a.theme.styleTextView(v.view)
	v.view.SetTextColor(a.theme.OnSurface)
	v.bar = newActionBar(a)
	v.bar.SetRefocus(v.Focus)
	v.buildActions()
	v.root = tview.NewFlex().SetDirection(tview.FlexRow).
		AddItem(v.view, 0, 1, true).
		AddItem(v.bar.Primitive(), 1, 0, false)
	v.Refresh()
	return v
}

// buildActions (re)labels the clickable action bar for the current language.
func (v *homePage) buildActions() {
	v.barLang = i18n.Current()
	v.bar.Set(
		pageAction{Label: uiText("refresh"), Run: func() { go v.app.refreshUsageStats() }},
		pageAction{Label: uiText("target"), Run: func() { v.app.openTargetDialog() }},
	)
}

func (v *homePage) Primitive() tview.Primitive { return v.root }
func (v *homePage) Focus()                     { v.app.application.SetFocus(v.view) }

func (v *homePage) HandleKey(event *tcell.EventKey) bool {
	if event.Key() == tcell.KeyRune && event.Rune() == 'r' {
		go v.app.refreshUsageStats()
		return true
	}
	return false
}

func (v *homePage) Hints() []hint {
	return []hint{
		hintAction("r "+i18n.T("home.refresh"), func() { go v.app.refreshUsageStats() }),
		hintText("1-6 " + uiText("navigate")),
		hintAction("t "+uiText("instance"), v.app.openTargetDialog),
		hintAction("q "+i18n.T("download.queue.title"), v.app.toggleQueue),
	}
}

func (v *homePage) OnActivate() { go v.app.refreshUsageStats() }

func (v *homePage) OnResize(width int) {
	v.build(width)
	v.bar.Reflow()
}

func (v *homePage) Restyle() {
	t := v.app.theme
	t.styleTextView(v.view)
	v.view.SetTextColor(t.OnSurface)
	v.bar.Restyle()
}

func (v *homePage) Refresh() {
	if v.barLang != i18n.Current() {
		v.buildActions()
	}
	v.build(v.app.mainWidth())
}

func (v *homePage) build(width int) {
	if width <= 0 {
		width = 80
	}
	v.view.SetTitle(i18n.T("home.title"))
	t := v.app.theme
	stats := v.app.stats
	inner := maxInt(8, width-2)
	var b strings.Builder

	b.WriteString(t.tag(t.Secondary) + i18n.T("home.kicker") + resetTag + "\n")
	b.WriteString(t.tagWith(t.Primary, "b") + i18n.T("home.title") + resetTag + "\n")
	b.WriteString(t.tag(t.Muted) + i18n.T("home.subtitle") + resetTag + "\n\n")
	b.WriteString(t.tag(t.Success) + "● " + i18n.T("home.status.tracking") + resetTag + "\n\n")

	total := stats.DownloadsCompleted + stats.ModsEnabled + stats.ModsDisabled +
		stats.ModsDeleted + stats.FavoritesAdded + stats.PackwizExports
	b.WriteString(t.tagWith(t.Primary, "b") + i18n.T("home.usage.title") + resetTag + "  ")
	b.WriteString(t.tag(t.Muted) + i18n.T("home.usage.sinceTracking") + resetTag + "\n")
	b.WriteString(t.tagWith(t.OnSurface, "b") + compactNumber(total) + resetTag + "  ")
	b.WriteString(t.tag(t.Muted) + i18n.T("home.usage.total") + resetTag + "\n\n")

	metrics := []struct {
		icon  string
		label string
		value int64
		color tcell.Color
	}{
		{"↓", i18n.T("home.usage.downloads"), stats.DownloadsCompleted, t.Secondary},
		{"◉", i18n.T("home.usage.enabled"), stats.ModsEnabled, t.Success},
		{"○", i18n.T("home.usage.disabled"), stats.ModsDisabled, t.Warning},
		{"✕", i18n.T("home.usage.deleted"), stats.ModsDeleted, t.Error},
		{"★", i18n.T("home.usage.favorites"), stats.FavoritesAdded, t.Secondary},
		{"⧉", i18n.T("home.usage.exports"), stats.PackwizExports, t.Muted},
	}

	cols := 3
	switch {
	case inner < 56:
		cols = 1
	case inner < 84:
		cols = 2
	}
	const gap = 2
	cellW := (inner - gap*(cols-1)) / cols
	if cellW < 18 {
		cellW = 18
	}
	for i := 0; i < len(metrics); i += cols {
		cards := make([][]string, 0, cols)
		for j := 0; j < cols && i+j < len(metrics); j++ {
			m := metrics[i+j]
			cards = append(cards, metricCard(t, m.icon, compactNumber(m.value), m.label, m.color, cellW))
		}
		for line := 0; line < len(cards[0]); line++ {
			parts := make([]string, 0, len(cards))
			for _, card := range cards {
				parts = append(parts, card[line])
			}
			b.WriteString(strings.Join(parts, strings.Repeat(" ", gap)))
			b.WriteString("\n")
		}
		b.WriteString("\n")
	}

	instance := v.app.mc.SelectedInstanceLabel()
	if instance == "" {
		instance = uiText("noInstance")
	}
	dir := v.app.mc.MinecraftDir
	if dir == "" {
		dir = i18n.T("home.status.notConfigured")
	}
	b.WriteString(t.tag(t.Muted) + i18n.T("home.status.instance") + ": " + resetTag)
	b.WriteString(t.tag(t.OnSurface) + tview.Escape(truncate(instance, maxInt(10, inner/2))) + resetTag)
	b.WriteString("  " + t.tag(t.Muted) + "MC " + resetTag + t.tag(t.OnSurface) + tview.Escape(v.app.targetTuple()) + resetTag)
	b.WriteString("  " + t.tag(t.Muted) + tview.Escape(truncate(dir, maxInt(10, inner/3))) + resetTag)

	v.view.SetText(strings.TrimRight(b.String(), "\n"))
}

// metricCard draws a bordered card with an icon, a large value and a label.
func metricCard(t Theme, icon, value, label string, color tcell.Color, width int) []string {
	if width < 8 {
		width = 8
	}
	inner := width - 4
	top := t.tag(t.Border) + "╭" + strings.Repeat("─", width-2) + "╮" + resetTag
	bottom := t.tag(t.Border) + "╰" + strings.Repeat("─", width-2) + "╯" + resetTag
	content := icon + " " + value
	valueLine := t.tag(t.Border) + "│ " + resetTag + t.tagWith(color, "b") + truncate(content, inner) + resetTag +
		strings.Repeat(" ", maxInt(0, inner-tviewWidth(content))) + t.tag(t.Border) + " │" + resetTag
	labelLine := t.tag(t.Border) + "│ " + resetTag + t.tag(t.OnSurface) + truncate(label, inner) + resetTag +
		strings.Repeat(" ", maxInt(0, inner-tviewWidth(label))) + t.tag(t.Border) + " │" + resetTag
	return []string{top, valueLine, labelLine, bottom}
}
