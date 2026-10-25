package tviewui

import (
	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"

	"github.com/link-fgfgui/mod-downloader-core/storage"

	"github.com/link-fgfgui/mod-downloader-cli/internal/i18n"
)

// unpinPage renders the pinned-version table.
type unpinPage struct {
	app     *App
	box     *tview.Flex
	head    *tview.TextView
	table   *wheelTable
	bar     *actionBar
	barLang string
	pins    []storage.PinnedMod
}

func newUnpinPage(a *App) *unpinPage {
	v := &unpinPage{app: a}
	v.head = tview.NewTextView().SetDynamicColors(true)
	v.head.SetBackgroundColor(a.theme.Surface)
	v.head.SetTextColor(a.theme.Muted)

	v.table = newWheelTable()
	v.table.SetFixed(1, 0)
	v.table.SetSelectable(true, false)
	v.table.SetBorders(false)
	v.table.SetBorder(true)
	v.table.SetTitle(i18n.T("unpin.title"))
	a.theme.styleBox(v.table.Box)
	a.theme.styleTable(v.table.Table)
	v.table.SetSelectedFunc(func(row, _ int) {
		if row > 0 {
			v.unpinRow(row - 1)
		}
	})

	v.bar = newActionBar(a)
	v.bar.SetRefocus(v.Focus)
	v.buildActions()

	v.box = tview.NewFlex().SetDirection(tview.FlexRow).
		AddItem(v.head, 1, 0, false).
		AddItem(v.table, 0, 1, true).
		AddItem(v.bar.Primitive(), 1, 0, false)
	v.Refresh()
	return v
}

// buildActions (re)labels the clickable action bar for the current language.
func (v *unpinPage) buildActions() {
	v.barLang = i18n.Current()
	v.bar.Set(
		pageAction{Label: i18n.T("unpin.unpin"), Run: func() { v.unpinCurrent() }},
		pageAction{Label: i18n.T("unpin.unpinAll"), Run: func() { v.confirmUnpinAll() }},
		pageAction{Label: i18n.T("unpin.refresh"), Run: func() { go v.app.refreshPinnedMods() }},
	)
}

func (v *unpinPage) Primitive() tview.Primitive { return v.box }
func (v *unpinPage) Focus()                     { v.app.application.SetFocus(v.table) }

func (v *unpinPage) OnActivate() { go v.app.refreshPinnedMods() }

func (v *unpinPage) OnResize(_ int) { v.bar.Reflow() }

func (v *unpinPage) HandleKey(event *tcell.EventKey) bool {
	if event.Key() != tcell.KeyRune {
		return false
	}
	switch event.Rune() {
	case 'r':
		go v.app.refreshPinnedMods()
		return true
	case 'u':
		v.unpinCurrent()
		return true
	case 'U':
		v.confirmUnpinAll()
		return true
	}
	return false
}

func (v *unpinPage) Hints() []hint {
	return []hint{
		hintText("↑↓ " + uiText("navigate")),
		hintAction("u "+i18n.T("unpin.unpin"), v.unpinCurrent),
		hintAction("U "+i18n.T("unpin.unpinAll"), v.confirmUnpinAll),
		hintAction("r "+i18n.T("unpin.refresh"), func() { go v.app.refreshPinnedMods() }),
	}
}

func (v *unpinPage) Restyle() {
	t := v.app.theme
	t.styleBox(v.table.Box)
	t.styleTable(v.table.Table)
	v.head.SetBackgroundColor(t.Surface)
	v.head.SetTextColor(t.Muted)
	v.bar.Restyle()
}

func (v *unpinPage) Refresh() {
	if v.barLang != i18n.Current() {
		v.buildActions()
	}
	v.build()
}

// setPins replaces the row source (called by the controller on the UI thread).
func (v *unpinPage) setPins(pins []storage.PinnedMod) {
	v.pins = pins
	v.build()
}

func (v *unpinPage) selectedKey() string {
	row, _ := v.table.GetSelection()
	if row <= 0 || row > len(v.pins) {
		return ""
	}
	return pinKey(v.pins[row-1])
}

func pinKey(pin storage.PinnedMod) string {
	return pin.Platform + "|" + pin.ModID + "|" + pin.VersionID
}

func (v *unpinPage) build() {
	pins := v.pins
	restore := v.selectedKey()
	row, _ := v.table.GetSelection()

	v.table.Clear()
	headers := []string{
		i18n.T("unpin.columns.platform"),
		i18n.T("unpin.columns.modId"),
		i18n.T("unpin.columns.versionId"),
		i18n.T("unpin.columns.minecraftVersion"),
		i18n.T("unpin.columns.modLoader"),
		" " + i18n.T("unpin.unpin"),
	}
	expansions := []int{0, 3, 2, 0, 0, 0}
	for i, title := range headers {
		cell := tview.NewTableCell(title).
			SetSelectable(false).
			SetAttributes(tcell.AttrBold).
			SetTextColor(v.app.theme.Secondary).
			SetExpansion(expansions[i])
		v.table.SetCell(0, i, cell)
	}
	for i, pin := range pins {
		cells := []string{pin.Platform, pin.ModID, pin.VersionID, pin.MinecraftVersion, pin.ModLoader, uiText("unpin")}
		for c, text := range cells {
			cell := tview.NewTableCell(tview.Escape(text)).SetTextColor(v.app.theme.OnSurface).SetExpansion(expansions[c])
			if c == 5 {
				cell.SetTextColor(v.app.theme.Error).SetExpansion(0)
			}
			v.table.SetCell(i+1, c, cell)
		}
	}

	switch {
	case len(pins) == 0:
		v.head.SetText("[-]" + i18n.T("unpin.empty"))
	case restore != "":
		for i, pin := range pins {
			if pinKey(pin) == restore {
				v.table.Select(i+1, 0)
				break
			}
		}
	default:
		if row >= 1 && row <= len(pins) {
			v.table.Select(row, 0)
		}
	}
	if len(pins) > 0 {
		v.head.SetText(v.app.theme.tag(v.app.theme.Muted) + itoa(len(pins)) + " " + i18n.T("unpin.columns.modId") + resetTag)
	}
	v.table.SetTitle(i18n.T("unpin.title"))
}

func (v *unpinPage) unpinCurrent() {
	row, _ := v.table.GetSelection()
	if row <= 0 || row > len(v.pins) {
		return
	}
	v.unpinRow(row - 1)
}

func (v *unpinPage) unpinRow(index int) {
	if index < 0 || index >= len(v.pins) {
		return
	}
	pin := v.pins[index]
	go v.app.unpinMod(pin)
}

func (v *unpinPage) confirmUnpinAll() {
	if len(v.pins) == 0 {
		return
	}
	pins := append([]storage.PinnedMod(nil), v.pins...)
	v.app.openConfirm(
		i18n.T("unpin.confirmAll.title"),
		i18n.T("unpin.confirmAll.body"),
		[]confirmOption{
			{Label: i18n.T("unpin.confirmAll.confirm"), Value: "confirm", Destructive: true},
			{Label: i18n.T("unpin.confirmAll.cancel"), Value: "cancel"},
		},
		func(value string) {
			if value != "confirm" {
				return
			}
			go v.app.unpinAll(pins)
		},
	)
}
