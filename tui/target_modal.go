package tui

import (
	"strings"

	tea "github.com/charmbracelet/bubbletea"

	structs "github.com/link-fgfgui/mod-downloader-core/structs/minecraft"

	"github.com/link-fgfgui/mod-downloader-cli/internal/uistate"
	"github.com/link-fgfgui/mod-downloader-cli/tui/i18n"
)

// instanceTargetModal replaces SideBar.vue + VersionChoose.vue: it lets the user
// pick the active instance, refresh it, and edit the manual [MC version |
// loader] target tuple, plus toggle the Connector virtual loader when available.
type instanceTargetModal struct {
	id     string
	cursor int
}

func newInstanceTargetModal() *instanceTargetModal {
	return &instanceTargetModal{id: "instance-target"}
}

func (m *instanceTargetModal) ID() string { return m.id }

// CapturesInput is true so navigation keys reach the row list instead of
// leaking to the shell's view-switching shortcuts.
func (m *instanceTargetModal) CapturesInput() bool { return true }

func (m *instanceTargetModal) PreferredWidth() int { return 74 }

// targetRowKind enumerates the editable rows of the modal.
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

func (m *instanceTargetModal) rows(app *AppModel) []targetRow {
	rows := []targetRow{
		{kind: targetRowDir, label: i18n.T("settings.minecraftDir.label"), value: defaultStr(app.mc.MinecraftDir, i18n.T("home.status.notConfigured"))},
		{kind: targetRowInstance, label: i18n.T("home.status.selectedInstance"), value: defaultStr(app.mc.SelectedInstanceLabel(), i18n.T("home.status.none"))},
		{kind: targetRowRefreshMods, label: i18n.T("manage.refresh"), value: ""},
		{kind: targetRowRefreshVersions, label: i18n.T("home.refresh"), value: ""},
		{kind: targetRowMCVersion, label: i18n.T("favorites.dialog.minecraftVersion"), value: defaultStr(app.mc.SelectedMinecraftVersion, "—")},
		{kind: targetRowLoader, label: i18n.T("favorites.dialog.modLoader"), value: defaultStr(app.mc.SelectedModLoader, "—")},
	}
	if app.selectedConnectorAvailable() {
		rows = append(rows, targetRow{kind: targetRowConnector, label: uiText("connector"), value: app.connectorLoaderTarget()})
	}
	return rows
}

func (m *instanceTargetModal) Update(app *AppModel, msg tea.Msg) tea.Cmd {
	key, ok := msg.(tea.KeyMsg)
	if !ok {
		return nil
	}
	rows := m.rows(app)
	switch key.String() {
	case "esc":
		app.popModal(m.id)
	case "up", "k":
		if m.cursor > 0 {
			m.cursor--
		}
	case "down", "j":
		if m.cursor < len(rows)-1 {
			m.cursor++
		}
	case "left", "h":
		return m.adjust(app, rows, -1)
	case "right", "l":
		return m.adjust(app, rows, 1)
	case "enter", " ":
		if m.cursor < len(rows) {
			return m.activate(app, rows[m.cursor])
		}
	}
	return nil
}

func (m *instanceTargetModal) adjust(app *AppModel, rows []targetRow, delta int) tea.Cmd {
	if m.cursor >= len(rows) {
		return nil
	}
	switch rows[m.cursor].kind {
	case targetRowMCVersion:
		next := cycleString(app.mc.SelectedMinecraftVersion, app.mc.ReleaseVersions, delta)
		if next == "" || next == app.mc.SelectedMinecraftVersion {
			return nil
		}
		// Manual edit clears the selected instance (browse/target-only mode),
		// mirroring minecraft.ts setSelectedMinecraftVersion.
		app.mc.SetSelectedMinecraftVersion(next)
		app.search.SetTargetTuple(app.mc.SelectedMinecraftVersion, app.mc.SelectedModLoader, app.mc.HasSelectedInstance())
		return app.refreshDownloadStates()
	case targetRowLoader:
		next := cycleString(app.mc.SelectedModLoader, uistate.ModLoaderOptions, delta)
		app.mc.SetSelectedModLoader(next)
		app.search.SetTargetTuple(app.mc.SelectedMinecraftVersion, app.mc.SelectedModLoader, app.mc.HasSelectedInstance())
		return app.refreshDownloadStates()
	}
	return nil
}

func (m *instanceTargetModal) activate(app *AppModel, row targetRow) tea.Cmd {
	switch row.kind {
	case targetRowDir:
		picker := newPathPickerModal("target-mc-dir", i18n.T("settings.minecraftDir.choose"), pickerDirectory, app.mc.MinecraftDir,
			func(app *AppModel, path string) tea.Cmd { return cmdSetMinecraftDir(app.host, path) })
		app.pushModal(picker)
		return picker.Init()
	case targetRowInstance:
		return m.openInstancePicker(app)
	case targetRowRefreshMods:
		return cmdRefreshSelectedMods(app.host)
	case targetRowRefreshVersions:
		return cmdRefreshMinecraftDir(app.host)
	case targetRowMCVersion:
		return m.adjust(app, m.rows(app), 1)
	case targetRowLoader:
		return m.adjust(app, m.rows(app), 1)
	case targetRowConnector:
		return cmdToggleConnector(app.host)
	}
	return nil
}

func (m *instanceTargetModal) openInstancePicker(app *AppModel) tea.Cmd {
	items := make([]selectItem, 0, len(app.mc.Versions))
	for _, version := range app.mc.Versions {
		items = append(items, selectItem{
			Label:  versionLabel(version),
			Value:  version.ID,
			Detail: version.ID,
		})
	}
	if len(items) == 0 {
		return app.toastT("home.status.notAvailable", toastWarning)
	}
	app.pushModal(newSelectModal("instance-picker", i18n.T("home.status.selectedInstance"), items,
		func(app *AppModel, item selectItem) tea.Cmd {
			return cmdSelectVersion(app.host, item.Value)
		}))
	return nil
}

func versionLabel(version structs.VersionInfo) string {
	name := defaultStr(version.Name, version.ID)
	target := version.MinecraftVersion
	if version.ModLoader != "" {
		target += " | " + version.ModLoader
	}
	if target != "" {
		return name + "   " + target
	}
	return name
}

func (m *instanceTargetModal) View(app *AppModel) string {
	t := app.theme
	rows := m.rows(app)
	var b strings.Builder
	b.WriteString(t.Title().Render(uiText("instance") + " / " + uiText("target")))
	b.WriteString("\n\n")
	width := maxInt(20, m.PreferredWidth()-4)
	for i, row := range rows {
		line := padRight(truncate(row.label, 16), 17)
		if row.value != "" {
			line += " " + t.Accent().Render(truncate(row.value, width-18))
		}
		if i == m.cursor {
			line = t.Selected().Render(stripStyles(line))
		} else {
			line = t.OnSurface().Render(line)
		}
		b.WriteString(line)
		b.WriteString("\n")
	}
	b.WriteString("\n")
	b.WriteString(t.Muted().Render("↑↓ " + uiText("navigate") + " · ←→ " + uiText("toggle") + " · enter " + uiText("open") + " · esc " + uiText("close") + " · t"))
	return b.String()
}

// connectorLoaderTarget reports the loader the connector toggle would switch to.
func (m *AppModel) connectorLoaderTarget() string {
	v := m.mc.SelectedVersion
	if v == nil {
		return ""
	}
	if v.ConnectorVirtual {
		return defaultStr(v.ActualModLoader, "Fabric")
	}
	return "Fabric"
}
