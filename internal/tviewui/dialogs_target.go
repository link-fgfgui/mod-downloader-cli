package tviewui

import (
	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"

	structs "github.com/link-fgfgui/mod-downloader-core/structs/minecraft"

	"github.com/link-fgfgui/mod-downloader-cli/internal/i18n"
	"github.com/link-fgfgui/mod-downloader-cli/internal/uistate"
)

// targetRowKind enumerates the editable rows of the instance/target dialog.
type targetRowKind int

const (
	targetRowDir targetRowKind = iota
	targetRowInstance
	targetRowRefreshMods
	targetRowRefreshVersions
	targetRowMCVersion
	targetRowLoader
	targetRowConnector
)

type targetRow struct {
	kind  targetRowKind
	label string
	value string
}

type targetDialog struct {
	app    *App
	rows   []targetRow
	list   *wheelList
	box    *tview.Flex
	dialog *dialog
}

// openTargetDialog shows the instance/target switcher (SideBar + VersionChoose).
func (a *App) openTargetDialog() {
	v := &targetDialog{app: a}
	v.list = newWheelList()
	v.list.ShowSecondaryText(false)
	a.theme.styleList(v.list.List)
	v.list.SetSelectedFunc(func(index int, _, _ string, _ rune) {
		v.activate(index)
	})
	v.rebuild()
	v.box = dialogBox(a, uiText("instance")+" / "+uiText("target"))
	v.box.AddItem(v.list, 0, 1, true)
	closeButton := a.chipButton(uiText("close"), func() { a.popDialog() })
	buttonRow := tview.NewFlex().SetDirection(tview.FlexColumn).AddItem(closeButton, chipWidth(uiText("close")), 0, false)
	v.box.AddItem(buttonRow, 1, 0, false)
	page := newCentered(v.box, 74, 15)
	d := &dialog{name: a.dialogName("target"), page: page, focus: v.list}
	d.handleKey = v.handleKey
	d.onClose = func() { a.targetDialog = nil }
	v.dialog = d
	a.targetDialog = v
	a.pushDialog(d)
}

func (v *targetDialog) buildRows() []targetRow {
	app := v.app
	rows := []targetRow{
		{kind: targetRowDir, label: i18n.T("settings.minecraftDir.label"), value: defaultString(app.mc.MinecraftDir, i18n.T("home.status.notConfigured"))},
		{kind: targetRowInstance, label: i18n.T("home.status.selectedInstance"), value: defaultString(app.mc.SelectedInstanceLabel(), i18n.T("home.status.none"))},
		{kind: targetRowRefreshMods, label: i18n.T("manage.refresh")},
		{kind: targetRowRefreshVersions, label: i18n.T("home.refresh")},
		{kind: targetRowMCVersion, label: i18n.T("favorites.dialog.minecraftVersion"), value: defaultString(app.mc.SelectedMinecraftVersion, "—")},
		{kind: targetRowLoader, label: i18n.T("favorites.dialog.modLoader"), value: defaultString(app.mc.SelectedModLoader, "—")},
	}
	if app.selectedConnectorAvailable() {
		rows = append(rows, targetRow{kind: targetRowConnector, label: uiText("connector"), value: app.connectorLabel()})
	}
	return rows
}

// rebuild redraws the row list while preserving the cursor position.
func (v *targetDialog) rebuild() {
	cursor := v.list.GetCurrentItem()
	v.rows = v.buildRows()
	v.list.Clear()
	t := v.app.theme
	for _, row := range v.rows {
		main := t.tag(t.Muted) + row.label + resetTag
		if busy := v.app.busyStatus(); busy != "" && row.kind == targetRowInstance {
			// Show the switch in progress instead of the stale instance name.
			main += "  " + t.tag(t.Warning) + tview.Escape(busy) + resetTag
		} else if row.value != "" {
			main += "  " + t.tag(t.Secondary) + tview.Escape(row.value) + resetTag
		}
		v.list.AddItem(main, "", 0, nil)
	}
	if cursor < 0 || cursor >= len(v.rows) {
		cursor = 0
	}
	v.list.SetCurrentItem(cursor)
}

func (v *targetDialog) handleKey(event *tcell.EventKey) bool {
	if event.Key() == tcell.KeyLeft || event.Key() == tcell.KeyRight {
		delta := 1
		if event.Key() == tcell.KeyLeft {
			delta = -1
		}
		v.adjust(delta)
		return true
	}
	return false
}

func (v *targetDialog) adjust(delta int) {
	idx := v.list.GetCurrentItem()
	if idx < 0 || idx >= len(v.rows) {
		return
	}
	switch v.rows[idx].kind {
	case targetRowMCVersion:
		next := cycleString(v.app.mc.SelectedMinecraftVersion, v.app.mc.ReleaseVersions, delta)
		if next == "" || next == v.app.mc.SelectedMinecraftVersion {
			return
		}
		v.app.mc.SetSelectedMinecraftVersion(next)
		v.app.search.SetTargetTuple(v.app.mc.SelectedMinecraftVersion, v.app.mc.SelectedModLoader, v.app.mc.HasSelectedInstance())
		v.app.favoritesPage().SetScope(v.app.mc.SelectedMinecraftVersion, v.app.mc.SelectedModLoader)
		v.app.updateSidebarFooter()
		v.rebuild()
		go v.app.refreshDownloadStates()
	case targetRowLoader:
		next := cycleString(v.app.mc.SelectedModLoader, uistate.ModLoaderOptions, delta)
		v.app.mc.SetSelectedModLoader(next)
		v.app.search.SetTargetTuple(v.app.mc.SelectedMinecraftVersion, v.app.mc.SelectedModLoader, v.app.mc.HasSelectedInstance())
		v.app.favoritesPage().SetScope(v.app.mc.SelectedMinecraftVersion, v.app.mc.SelectedModLoader)
		v.app.updateSidebarFooter()
		v.rebuild()
		go v.app.refreshDownloadStates()
	}
}

func (v *targetDialog) activate(index int) {
	if index < 0 || index >= len(v.rows) {
		return
	}
	row := v.rows[index]
	switch row.kind {
	case targetRowDir:
		v.app.openPathDialog(i18n.T("settings.minecraftDir.choose"), false, v.app.mc.MinecraftDir, func(path string) {
			go v.app.setMinecraftDir(path)
		})
	case targetRowInstance:
		v.openInstancePicker()
	case targetRowRefreshMods:
		go v.app.refreshSelectedMods()
	case targetRowRefreshVersions:
		go v.app.refreshMinecraft()
	case targetRowMCVersion, targetRowLoader:
		v.adjust(1)
	case targetRowConnector:
		go v.app.toggleConnector()
	}
}

func (v *targetDialog) openInstancePicker() {
	items := make([]selectItem, 0, len(v.app.mc.Versions))
	for _, version := range v.app.mc.Versions {
		items = append(items, selectItem{
			Label:  versionLabel(version),
			Value:  version.ID,
			Detail: version.ID,
		})
	}
	if len(items) == 0 {
		v.app.toastT("home.status.notAvailable")
		return
	}
	v.app.openSelect(i18n.T("home.status.selectedInstance"), items, func(item selectItem) {
		go v.app.selectVersion(item.Value)
	})
}

func versionLabel(version structs.VersionInfo) string {
	name := defaultString(version.Name, version.ID)
	target := version.MinecraftVersion
	if version.ModLoader != "" {
		target += " | " + version.ModLoader
	}
	if target != "" {
		return name + "   " + target
	}
	return name
}

// refresh is called by the controller when instance state changes.
func (v *targetDialog) refresh() {
	if v == nil {
		return
	}
	v.rebuild()
}
