package tviewui

import (
	"strings"

	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"

	"github.com/link-fgfgui/mod-downloader-core/models"
	"github.com/link-fgfgui/mod-downloader-core/storage"

	"github.com/link-fgfgui/mod-downloader-cli/internal/i18n"
)

// versionDialog lists matching project versions for pinning or replacing.
type versionDialog struct {
	app       *App
	project   models.ModProject
	action    string
	mcVersion string
	modLoader string

	versions      []models.ModVersion
	pinnedVersion string
	installedID   string
	installedSHA1 string
	loading       bool
	busyVersionID string

	head *tview.TextView
	list *wheelList
	d    *dialog

	onPin    func(models.ModVersion)
	onSelect func(models.ModVersion)
}

// openVersionDialog builds and pushes the version picker. The caller is
// responsible for loading versions via the controller.
func (a *App) openVersionDialog(project models.ModProject, action string, onPin, onSelect func(models.ModVersion)) *versionDialog {
	v := &versionDialog{
		app:       a,
		project:   project,
		action:    action,
		mcVersion: a.search.SelectedVersion,
		modLoader: a.search.SelectedModLoader,
		loading:   true,
		onPin:     onPin,
		onSelect:  onSelect,
	}
	if action == "replace" {
		v.mcVersion = a.mc.SelectedMinecraftVersion
		v.modLoader = a.mc.SelectedModLoader
	}
	title := i18n.T("download.versionsTitle")
	if action == "replace" {
		title = i18n.T("manage.version.title", map[string]string{"name": project.Title})
	}

	v.head = tview.NewTextView().SetDynamicColors(true)
	v.head.SetBackgroundColor(a.theme.Surface)
	v.head.SetText(a.theme.tag(a.theme.Muted) + tview.Escape(project.Platform+" · "+project.ProjectID+" · "+v.mcVersion+" | "+v.modLoader) + resetTag)

	v.list = newWheelList()
	v.list.ShowSecondaryText(true)
	v.list.SetUseStyleTags(true, true)
	v.list.SetWrapAround(false)
	a.theme.styleList(v.list.List)
	v.list.SetSelectedFunc(func(index int, _, _ string, _ rune) {
		v.activate(index)
	})

	box := dialogBox(a, tview.Escape(title))
	box.AddItem(v.head, 1, 0, false)
	box.AddItem(v.list, 0, 1, true)
	d := &dialog{name: a.dialogName("versions"), focus: v.list}
	closeButton := a.chipButton(uiText("close"), func() { a.popDialogName(d.name) })
	box.AddItem(tview.NewFlex().SetDirection(tview.FlexColumn).
		AddItem(closeButton, chipWidth(uiText("close")), 0, false), 1, 0, false)
	page := newCentered(box, 78, 23)
	d.page = page
	d.handleKey = v.handleKey
	d.onClose = func() {
		if a.versionDialog == v {
			a.versionDialog = nil
		}
	}
	v.d = d
	a.versionDialog = v
	a.pushDialog(d)
	v.render()
	// The controller fills in the version list asynchronously.
	go a.listMatchingVersions(v.project, v.mcVersion, v.modLoader)
	return v
}

func (v *versionDialog) handleKey(event *tcell.EventKey) bool {
	if event.Key() == tcell.KeyRune && event.Rune() == 'p' && !v.loading {
		v.pinCurrent()
		return true
	}
	return false
}

// setVersions receives the controller's matching-version result.
func (v *versionDialog) setVersions(versions []models.ModVersion, pin storage.PinnedMod) {
	if v == nil {
		return
	}
	v.versions = versions
	v.pinnedVersion = pin.VersionID
	v.loading = false
	v.render()
}

// setPinned updates the ★ marker after a pin round-trip.
func (v *versionDialog) setPinned(pin storage.PinnedMod) {
	if v == nil {
		return
	}
	v.pinnedVersion = pin.VersionID
	v.busyVersionID = ""
	v.render()
}

func (v *versionDialog) render() {
	if v.list == nil {
		return
	}
	cursor := v.list.GetCurrentItem()
	v.list.Clear()
	t := v.app.theme
	if v.loading {
		v.list.AddItem(t.tag(t.Muted)+uiText("loading")+resetTag, "", 0, nil)
		return
	}
	if len(v.versions) == 0 {
		v.list.AddItem(t.tag(t.Muted)+i18n.T("download.noMatchingVersions")+resetTag, "", 0, nil)
		return
	}
	for _, version := range v.versions {
		marker := "  "
		if version.ID == v.pinnedVersion {
			marker = "★ "
		}
		label := version.Name
		if label == "" {
			label = version.Version
		}
		installed := isInstalledVersion(version, v.installedID, v.installedSHA1)
		main := marker + tview.Escape(truncate(label, 34))
		if installed {
			main = marker + t.tag(t.Success) + "✓ " + resetTag + tview.Escape(truncate(label, 32))
		}
		if version.ID == v.busyVersionID {
			main += " " + t.tag(t.Warning) + "…" + resetTag
		}
		detail := version.Version + " · " + version.ReleaseType
		if installed {
			detail += " · " + i18n.T("manage.columns.state")
		}
		v.list.AddItem(main, t.tag(t.Muted)+tview.Escape(detail)+resetTag, 0, nil)
	}
	if cursor < 0 || cursor >= len(v.versions) {
		cursor = 0
	}
	v.list.SetCurrentItem(cursor)
}

func (v *versionDialog) activate(index int) {
	if v.loading || v.busyVersionID != "" || index < 0 || index >= len(v.versions) {
		return
	}
	version := v.versions[index]
	if isInstalledVersion(version, v.installedID, v.installedSHA1) {
		return
	}
	if v.action == "replace" && v.onSelect != nil {
		v.app.popDialogName(v.d.name)
		v.onSelect(version)
		return
	}
	v.pinCurrent()
}

func (v *versionDialog) pinCurrent() {
	if v.busyVersionID != "" || v.loading {
		return
	}
	index := v.list.GetCurrentItem()
	if index < 0 || index >= len(v.versions) {
		return
	}
	version := v.versions[index]
	if isInstalledVersion(version, v.installedID, v.installedSHA1) {
		return
	}
	v.busyVersionID = version.ID
	v.render()
	if v.onPin != nil {
		v.onPin(version)
	}
}

func isInstalledVersion(version models.ModVersion, installedID, installedSHA1 string) bool {
	if installedID != "" && version.ID == installedID {
		return true
	}
	if installedSHA1 != "" && strings.EqualFold(version.SHA1, installedSHA1) {
		return true
	}
	return false
}
