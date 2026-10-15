package tui

import (
	"strings"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/link-fgfgui/mod-downloader-core/models"
	"github.com/link-fgfgui/mod-downloader-core/storage"
	appstructs "github.com/link-fgfgui/mod-downloader-core/structs"

	"github.com/link-fgfgui/mod-downloader-cli/internal/uistate"
	"github.com/link-fgfgui/mod-downloader-cli/tui/i18n"
)

// --- version list modal (pin / replace) --------------------------------------

type versionListModal struct {
	id            string
	title         string
	action        string // "pin" | "replace"
	project       models.ModProject
	mcVersion     string
	modLoader     string
	versions      []models.ModVersion
	pinnedVersion string
	installedID   string
	installedSHA1 string
	cursor        int
	offset        int
	loading       bool
	busyVersionID string
	onPin         func(app *AppModel, version models.ModVersion) tea.Cmd
	onSelect      func(app *AppModel, version models.ModVersion) tea.Cmd
}

func newVersionListModal(id, action string, project models.ModProject, mcVersion, modLoader string,
	onPin func(*AppModel, models.ModVersion) tea.Cmd,
	onSelect func(*AppModel, models.ModVersion) tea.Cmd) *versionListModal {
	titleKey := "download.versionsTitle"
	if action == "replace" {
		titleKey = "manage.version.title"
	}
	return &versionListModal{
		id:        id,
		title:     i18n.T(titleKey, map[string]string{"name": project.Title}),
		action:    action,
		project:   project,
		mcVersion: mcVersion,
		modLoader: modLoader,
		loading:   true,
		onPin:     onPin,
		onSelect:  onSelect,
	}
}

func (m *versionListModal) ID() string          { return m.id }
func (m *versionListModal) CapturesInput() bool { return true }
func (m *versionListModal) PreferredWidth() int { return 78 }

func (m *versionListModal) Update(app *AppModel, msg tea.Msg) tea.Cmd {
	switch typed := msg.(type) {
	case matchingVersionsMsg:
		if typed.Project.ProjectID != m.project.ProjectID {
			return nil
		}
		m.versions = typed.Versions
		m.pinnedVersion = typed.Pin.VersionID
		m.loading = false
		return nil
	case pinnedMsg:
		m.pinnedVersion = typed.Pin.VersionID
		m.busyVersionID = ""
		return nil
	}

	key, ok := msg.(tea.KeyMsg)
	if !ok {
		return nil
	}
	switch key.String() {
	case "esc":
		app.popModal(m.id)
	case "up", "k":
		m.move(-1)
	case "down", "j":
		m.move(1)
	case "pgup":
		m.move(-10)
	case "pgdown":
		m.move(10)
	case "p":
		if len(m.versions) > 0 {
			return m.pin(app, m.versions[m.cursor])
		}
	case "enter":
		if len(m.versions) == 0 || m.busyVersionID != "" {
			return nil
		}
		version := m.versions[m.cursor]
		if isInstalledVersion(version, m.installedID, m.installedSHA1) {
			return nil // already installed: refuse to re-queue the same version
		}
		if m.action == "replace" {
			if m.onSelect != nil {
				return m.onSelect(app, version)
			}
		}
		return m.pin(app, version)
	}
	return nil
}

func (m *versionListModal) pin(app *AppModel, version models.ModVersion) tea.Cmd {
	if m.busyVersionID != "" {
		return nil
	}
	m.busyVersionID = version.ID
	return cmdPinModVersion(app.host, appstructs.ModVersionPinRequest{
		Platform:         m.project.Platform,
		ModID:            m.project.ProjectID,
		VersionID:        version.ID,
		MinecraftVersion: m.mcVersion,
		ModLoader:        m.modLoader,
	})
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

func (m *versionListModal) move(delta int) {
	if len(m.versions) == 0 {
		return
	}
	m.cursor += delta
	if m.cursor < 0 {
		m.cursor = 0
	}
	if m.cursor >= len(m.versions) {
		m.cursor = len(m.versions) - 1
	}
	m.clamp()
}

func (m *versionListModal) clamp() {
	const rows = 10
	if m.cursor < m.offset {
		m.offset = m.cursor
	}
	if m.cursor >= m.offset+rows {
		m.offset = m.cursor - rows + 1
	}
	if m.offset < 0 {
		m.offset = 0
	}
}

func (m *versionListModal) View(app *AppModel) string {
	t := app.theme
	var b strings.Builder
	b.WriteString(t.Title().Render(m.title))
	b.WriteString("\n")
	b.WriteString(t.Muted().Render(m.project.Platform + " · " + m.project.ProjectID + " · " + m.mcVersion + " | " + m.modLoader))
	b.WriteString("\n\n")

	if m.loading {
		b.WriteString(t.Muted().Render(uiText("loading")))
		return b.String()
	}
	if len(m.versions) == 0 {
		b.WriteString(t.Muted().Render(i18n.T("download.noMatchingVersions")))
		return b.String()
	}

	const rows = 10
	end := m.offset + rows
	if end > len(m.versions) {
		end = len(m.versions)
	}
	for i := m.offset; i < end; i++ {
		version := m.versions[i]
		marker := "  "
		if version.ID == m.pinnedVersion {
			marker = "★ "
		}
		label := version.Name
		if label == "" {
			label = version.Version
		}
		line := marker + truncate(label, 34) + "  " + t.Muted().Render(version.Version+" · "+version.ReleaseType)
		if isInstalledVersion(version, m.installedID, m.installedSHA1) {
			line += "  " + t.Success().Render("("+i18n.T("manage.columns.state")+")")
		}
		if i == m.cursor {
			line = t.Selected().Render(stripStyles(line))
		} else {
			line = t.OnSurface().Render(line)
		}
		b.WriteString(line)
		b.WriteString("\n")
	}

	hint := "↑↓ " + uiText("navigate") + " · p " + i18n.T("favorites.actions.pin") + " · enter "
	if m.action == "replace" {
		hint += i18n.T("manage.version.choose")
	} else {
		hint += i18n.T("favorites.actions.pin")
	}
	b.WriteString("\n")
	b.WriteString(t.Muted().Render(hint + " · esc " + uiText("close")))
	return b.String()
}

// --- add-to-favorites modal ---------------------------------------------------

type addFavoriteModal struct {
	id      string
	lists   []storage.FavoriteList
	cursor  int
	chosen  map[string]bool
	drafts  int
	onAdd   func(app *AppModel, listIDs []string) tea.Cmd
	newName string
}

func newAddFavoriteModal(id string, lists []storage.FavoriteList, drafts int, onAdd func(*AppModel, []string) tea.Cmd) *addFavoriteModal {
	return &addFavoriteModal{id: id, lists: lists, chosen: map[string]bool{}, drafts: drafts, onAdd: onAdd}
}

func (m *addFavoriteModal) ID() string          { return m.id }
func (m *addFavoriteModal) CapturesInput() bool { return true }
func (m *addFavoriteModal) PreferredWidth() int { return 64 }

func (m *addFavoriteModal) setLists(lists []storage.FavoriteList) {
	m.lists = lists
	if m.cursor >= len(m.lists) {
		m.cursor = maxInt(0, len(m.lists)-1)
	}
}

func (m *addFavoriteModal) Update(app *AppModel, msg tea.Msg) tea.Cmd {
	switch typed := msg.(type) {
	case favoritesLoadedMsg:
		m.setLists(filterScopedLists(typed.Lists, app))
		return nil
	}
	key, ok := msg.(tea.KeyMsg)
	if !ok {
		return nil
	}
	switch key.String() {
	case "esc":
		app.popModal(m.id)
	case "up", "k":
		m.move(-1)
	case "down", "j":
		m.move(1)
	case " ":
		if len(m.lists) > 0 {
			id := m.lists[m.cursor].ID
			m.chosen[id] = !m.chosen[id]
		}
	case "enter":
		ids := []string{}
		for id, on := range m.chosen {
			if on {
				ids = append(ids, id)
			}
		}
		if len(ids) == 0 && len(m.lists) > 0 {
			ids = append(ids, m.lists[m.cursor].ID)
		}
		app.popModal(m.id)
		if m.onAdd != nil && len(ids) > 0 {
			return m.onAdd(app, ids)
		}
	case "n":
		modal := newPromptModal("favorites-new-list", i18n.T("favorites.dialog.createTitle"), "", "",
			i18n.T("favorites.dialog.name"),
			func(app *AppModel, value string) tea.Cmd {
				if strings.TrimSpace(value) == "" {
					return nil
				}
				return cmdCreateFavoriteList(app.host, value, app.mc.SelectedMinecraftVersion, app.mc.SelectedModLoader)
			})
		app.pushModal(modal)
	}
	return nil
}

func (m *addFavoriteModal) move(delta int) {
	if len(m.lists) == 0 {
		return
	}
	m.cursor += delta
	if m.cursor < 0 {
		m.cursor = 0
	}
	if m.cursor >= len(m.lists) {
		m.cursor = len(m.lists) - 1
	}
}

func (m *addFavoriteModal) View(app *AppModel) string {
	t := app.theme
	var b strings.Builder
	b.WriteString(t.Title().Render(i18n.T("favorites.addDialog.title")))
	b.WriteString("\n")
	b.WriteString(t.Muted().Render(i18n.T("favorites.addDialog.count", map[string]string{"n": itoa(m.drafts)})))
	b.WriteString("\n\n")

	if len(m.lists) == 0 {
		b.WriteString(t.Muted().Render(i18n.T("favorites.empty.noLists")))
		b.WriteString("\n")
	} else {
		for i, list := range m.lists {
			check := "[ ]"
			if m.chosen[list.ID] {
				check = "[✓]"
			}
			line := check + " " + list.Name
			if i == m.cursor {
				line = t.Selected().Render(stripStyles(line))
			} else {
				line = t.OnSurface().Render(line)
			}
			b.WriteString(line)
			b.WriteString("\n")
		}
	}
	b.WriteString("\n")
	b.WriteString(t.Muted().Render("space " + uiText("space") + " · enter " + uiText("confirm") + " · n " + uiText("newList") + " · esc " + uiText("close")))
	return b.String()
}

// filterScopedLists mirrors the favorites store scope filtering.
func filterScopedLists(lists []storage.FavoriteList, app *AppModel) []storage.FavoriteList {
	pinned, ungrouped := uistate.PartitionLists(lists, app.mc.SelectedMinecraftVersion, app.mc.SelectedModLoader)
	return append(pinned, ungrouped...)
}
