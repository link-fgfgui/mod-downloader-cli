package tui

import (
	"strings"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"

	"github.com/link-fgfgui/mod-downloader-core/models"
	appstructs "github.com/link-fgfgui/mod-downloader-core/structs"

	"github.com/link-fgfgui/mod-downloader-cli/internal/uistate"
	"github.com/link-fgfgui/mod-downloader-cli/tui/i18n"
)

// manageModel renders local Mod management (Phase 4).
type manageModel struct {
	app           *AppModel
	input         textinput.Model
	focused       bool
	search        string
	filter        string
	cursor        int
	offset        int
	expanded      bool
	selected      map[string]bool
	pending       []uistate.ModGroup
	pendingAction string
}

func (v *manageModel) ensureInit() {
	if v.input.Placeholder == "" {
		ti := textinput.New()
		ti.Placeholder = i18n.T("manage.search.label")
		ti.Width = 36
		v.input = ti
	}
	if v.selected == nil {
		v.selected = map[string]bool{}
	}
	if v.filter == "" {
		v.filter = uistate.FilterAll
	}
}

func (v *manageModel) rows() ([]uistate.ManageRow, int) {
	groups := uistate.BuildGroups(v.app.mc.Mods())
	filtered := uistate.FilterGroups(groups, v.search, v.filter)
	return uistate.PartitionAndBuildRows(filtered, v.app.mc.SelectedModLoader, v.expanded)
}

func (v *manageModel) Update(app *AppModel, msg tea.Msg) tea.Cmd {
	v.ensureInit()
	switch typed := msg.(type) {
	case viewActivatedMsg:
		v.ensureInit()
		if app.mc.HasSelectedInstance() {
			return cmdRefreshSelectedMods(app.host)
		}
		return nil
	case localModsMsg:
		v.clampCursor()
		return nil
	case unusedScanMsg:
		return v.showCleanupCandidates(app, typed.Result)
	case disableImpactMsg:
		return v.onDisableImpact(app, typed)
	case tea.KeyMsg:
		return v.key(app, typed)
	}
	return nil
}

func (v *manageModel) CapturesKeys(app *AppModel) bool { return v.focused }

func (v *manageModel) key(app *AppModel, key tea.KeyMsg) tea.Cmd {
	if v.focused {
		switch key.String() {
		case "enter":
			v.focused = false
			v.input.Blur()
			v.search = strings.ToLower(strings.TrimSpace(v.input.Value()))
			return nil
		case "esc":
			v.focused = false
			v.input.Blur()
			return nil
		}
		var cmd tea.Cmd
		v.input, cmd = v.input.Update(key)
		return cmd
	}

	switch key.String() {
	case "/":
		v.focused = true
		v.input.Focus()
		return textinput.Blink
	case "F":
		v.filter = cycleString(v.filter, []string{uistate.FilterAll, uistate.FilterEnabled, uistate.FilterDisabled}, 1)
		v.clampCursor()
	case "up", "k":
		v.move(-1)
	case "down", "j":
		v.move(1)
	case "pgup":
		v.move(-8)
	case "pgdown":
		v.move(8)
	case " ":
		v.toggleCurrent()
	case "ctrl+a", "A":
		for _, row := range v.currentRows() {
			if row.Selectable() {
				v.selected[row.RowKey()] = true
			}
		}
	case "esc":
		v.selected = map[string]bool{}
	case "enter":
		return v.enterCurrent(app)
	case "e":
		return v.applyBatch(app, "enable")
	case "d":
		return v.applyBatch(app, "disable")
	case "i":
		return v.applyBatch(app, "invert")
	case "x":
		return v.openDeleteDialog(app)
	case "S":
		return v.scanUnused(app)
	case "c":
		return v.copyToClipboard(app, false)
	case "C":
		return v.copyToClipboard(app, true)
	case "f":
		return v.addToFavorites(app)
	case "v":
		return v.openVersionDialog(app)
	case "r":
		return cmdRefreshSelectedMods(app.host)
	}
	return nil
}

func (v *manageModel) currentRows() []uistate.ManageRow {
	rows, _ := v.rows()
	return rows
}

func (v *manageModel) move(delta int) {
	rows := v.currentRows()
	if len(rows) == 0 {
		return
	}
	v.cursor += delta
	if v.cursor < 0 {
		v.cursor = 0
	}
	if v.cursor >= len(rows) {
		v.cursor = len(rows) - 1
	}
	v.clampOffset()
}

func (v *manageModel) clampOffset() {
	const window = 8
	if v.cursor < v.offset {
		v.offset = v.cursor
	}
	if v.cursor >= v.offset+window {
		v.offset = v.cursor - window + 1
	}
	if v.offset < 0 {
		v.offset = 0
	}
}

func (v *manageModel) clampCursor() {
	rows := v.currentRows()
	if len(rows) == 0 {
		v.cursor = 0
		return
	}
	if v.cursor >= len(rows) {
		v.cursor = len(rows) - 1
	}
	v.clampOffset()
}

func (v *manageModel) toggleCurrent() {
	rows := v.currentRows()
	if v.cursor >= len(rows) {
		return
	}
	row := rows[v.cursor]
	if !row.Selectable() {
		return
	}
	key := row.RowKey()
	if v.selected[key] {
		delete(v.selected, key)
	} else {
		v.selected[key] = true
	}
}

// selectedGroups returns the groups currently selected (or the current row).
func (v *manageModel) selectedGroups() []uistate.ModGroup {
	groups := []uistate.ModGroup{}
	for _, row := range v.currentRows() {
		if row.Selectable() && v.selected[row.RowKey()] {
			groups = append(groups, row.Group)
		}
	}
	if len(groups) == 0 {
		rows := v.currentRows()
		if v.cursor < len(rows) && rows[v.cursor].Selectable() {
			groups = append(groups, rows[v.cursor].Group)
		}
	}
	return groups
}

func (v *manageModel) enterCurrent(app *AppModel) tea.Cmd {
	rows := v.currentRows()
	if v.cursor >= len(rows) {
		return nil
	}
	row := rows[v.cursor]
	if row.Kind == uistate.RowKindIncompatibleToggle {
		v.expanded = !v.expanded
		return nil
	}
	v.selected[row.RowKey()] = true
	return v.applyBatchTo(app, []uistate.ModGroup{row.Group}, uistate.PrimaryBatchAction([]uistate.ModGroup{row.Group}))
}

// applyBatch applies an action to the current selection (or the current row).
func (v *manageModel) applyBatch(app *AppModel, action string) tea.Cmd {
	groups := v.selectedGroups()
	if len(groups) == 0 {
		return nil
	}
	return v.applyBatchTo(app, groups, action)
}

func (v *manageModel) applyBatchTo(app *AppModel, groups []uistate.ModGroup, action string) tea.Cmd {
	paths := uistate.SelectedGroupPaths(groups)
	if len(paths) == 0 {
		return nil
	}
	if action == "disable" || action == "invert" {
		v.pending = groups
		v.pendingAction = action
		return cmdAnalyzeDisableImpact(app.host, appstructs.LocalModDisableImpactRequest{Paths: paths, Action: action})
	}
	return v.executeBatch(app, groups, action)
}

func (v *manageModel) executeBatch(app *AppModel, groups []uistate.ModGroup, action string) tea.Cmd {
	paths := uistate.SelectedGroupPaths(groups)
	if len(paths) == 0 {
		return nil
	}
	v.selected = map[string]bool{}
	return cmdApplyLocalModBatch(app.host, appstructs.LocalModBatchOperationRequest{Paths: paths, Action: action})
}

// onDisableImpact mirrors applyBatchOperation: prompt when impacts exist.
func (v *manageModel) onDisableImpact(app *AppModel, msg disableImpactMsg) tea.Cmd {
	if len(msg.Result.Impacts) == 0 {
		groups := v.pending
		action := v.pendingAction
		v.pending = nil
		return v.executeBatch(app, groups, action)
	}
	app.pushModal(newDisableImpactModal(v, v.pending, v.pendingAction, msg.Result))
	return nil
}

func (v *manageModel) openDeleteDialog(app *AppModel) tea.Cmd {
	groups := v.selectedGroups()
	if len(groups) == 0 {
		return nil
	}
	paths := uistate.SelectedGroupPaths(groups)
	autoScan := v.app.settingsView.AutoScanUnusedDependencies
	app.pushModal(newConfirmModal(
		"manage-delete",
		i18n.T("manage.confirmDelete.title"),
		i18n.T("manage.confirmDelete.body", map[string]string{"n": itoa(len(paths))}),
		[]choiceOption{
			{Label: i18n.T("manage.confirmDelete.confirm"), Value: "confirm", Destructive: true},
			{Label: i18n.T("manage.confirmDelete.cancel"), Value: "cancel"},
		},
		func(app *AppModel, value string) tea.Cmd {
			if value != "confirm" {
				return nil
			}
			if autoScan {
				return tea.Batch(
					cmdScanUnusedDependencies(app.host, appstructs.UnusedDependencyScanRequest{ExcludedPaths: paths}),
					v.executeBatchDeferred(app, groups, paths),
				)
			}
			return v.executeBatch(app, groups, "delete")
		},
	))
	return nil
}

// executeBatchDeferred deletes without clearing the selection first, used when a
// scan runs in parallel.
func (v *manageModel) executeBatchDeferred(app *AppModel, groups []uistate.ModGroup, paths []string) tea.Cmd {
	v.selected = map[string]bool{}
	return cmdApplyLocalModBatch(app.host, appstructs.LocalModBatchOperationRequest{Paths: paths, Action: "delete"})
}

func (v *manageModel) scanUnused(app *AppModel) tea.Cmd {
	if !app.mc.HasSelectedInstance() {
		return nil
	}
	return tea.Batch(cmdRefreshSelectedMods(app.host),
		cmdScanUnusedDependencies(app.host, appstructs.UnusedDependencyScanRequest{}))
}

// showCleanupCandidates renders the unused-dependency cleanup dialog.
func (v *manageModel) showCleanupCandidates(app *AppModel, result appstructs.UnusedDependencyScanResult) tea.Cmd {
	if len(result.Candidates) == 0 {
		return app.toastT("manage.cleanup.none", toastSuccess)
	}
	candidates := make([]uistate.UnusedCandidate, 0, len(result.Candidates))
	for _, c := range result.Candidates {
		candidates = append(candidates, uistate.UnusedCandidate{
			Path: c.Path, FileName: c.FileName, ModIDs: c.ModIDs, Name: c.Name, OnlineName: c.OnlineName, Evidence: c.Evidence,
		})
	}
	lines := []string{}
	for i, c := range candidates {
		if i >= 8 {
			lines = append(lines, "…")
			break
		}
		name := c.OnlineName
		if name == "" {
			name = c.Name
		}
		if name == "" {
			name = c.FileName
		}
		lines = append(lines, "· "+name+"  "+strings.Join(c.ModIDs, ", ")+"  ["+uistate.CleanupEvidenceKey(c.Evidence)+"]")
	}
	app.pushModal(newConfirmModal(
		"manage-cleanup",
		i18n.T("manage.cleanup.dialogTitle"),
		strings.Join(lines, "\n"),
		[]choiceOption{
			{Label: i18n.T("manage.cleanup.deleteCandidates", map[string]string{"n": itoa(len(candidates))}), Value: "confirm", Destructive: true},
			{Label: i18n.T("manage.cleanup.cancel"), Value: "cancel"},
		},
		func(app *AppModel, value string) tea.Cmd {
			if value != "confirm" {
				return nil
			}
			paths := make([]string, 0, len(candidates))
			for _, c := range candidates {
				if c.Path != "" {
					paths = append(paths, c.Path)
				}
			}
			return cmdApplyLocalModBatch(app.host, appstructs.LocalModBatchOperationRequest{Paths: paths, Action: "delete"})
		},
	))
	return app.toastT("manage.cleanup.found", toastWarning, map[string]string{"n": itoa(len(candidates))})
}

func (v *manageModel) copyToClipboard(app *AppModel, names bool) tea.Cmd {
	groups := v.selectedGroups()
	if len(groups) == 0 {
		return nil
	}
	values := []string{}
	for _, group := range groups {
		if names {
			values = append(values, uistate.StrongModNames(group)...)
		} else {
			values = append(values, uistate.StrongModIDs(group)...)
		}
	}
	text := strings.Join(values, "\n")
	return copyToClipboardCmd(app, text)
}

func (v *manageModel) addToFavorites(app *AppModel) tea.Cmd {
	groups := v.selectedGroups()
	if len(groups) == 0 {
		return nil
	}
	if !uistate.CanFavoriteSelection(groups, app.mc.SelectedMinecraftVersion, app.mc.SelectedModLoader) {
		return app.toastT("favorites.invalidSelection", toastWarning)
	}
	drafts := make([]favoriteDraft, 0, len(groups))
	for _, group := range groups {
		drafts = append(drafts, favoriteDraftFromGroup(group, app))
	}
	return openAddFavorites(app, drafts)
}

func (v *manageModel) openVersionDialog(app *AppModel) tea.Cmd {
	groups := v.selectedGroups()
	if len(groups) != 1 {
		return nil
	}
	group := groups[0]
	if !uistate.CanReplaceVersion(group, app.mc.HasSelectedInstance()) {
		return nil
	}
	project := uistate.OnlineProjectFromGroup(group)
	modal := newVersionListModal("manage-versions", "replace", project, app.mc.SelectedMinecraftVersion, app.mc.SelectedModLoader,
		func(app *AppModel, version models.ModVersion) tea.Cmd {
			return cmdPinModVersion(app.host, appstructs.ModVersionPinRequest{
				Platform: project.Platform, ModID: project.ProjectID, VersionID: version.ID,
				MinecraftVersion: app.mc.SelectedMinecraftVersion, ModLoader: app.mc.SelectedModLoader,
			})
		},
		func(app *AppModel, version models.ModVersion) tea.Cmd {
			req := appstructs.ModDownloadRequest{
				ProjectID:        group.Primary.OnlineProjectID,
				Result:           project,
				MinecraftVersion: app.mc.SelectedMinecraftVersion,
				ModLoader:        app.mc.SelectedModLoader,
				VersionID:        version.ID,
			}
			app.popModal("manage-versions")
			return cmdQueueModDownload(app.host, req)
		})
	modal.installedID = group.Primary.OnlineVersionID
	modal.installedSHA1 = group.Primary.SHA1
	app.pushModal(modal)
	return cmdListMatchingVersions(app.host, project, app.mc.SelectedMinecraftVersion, app.mc.SelectedModLoader)
}

func (v *manageModel) ClearFocus(app *AppModel) (bool, tea.Cmd) {
	if v.focused {
		v.focused = false
		v.input.Blur()
		return true, nil
	}
	if len(v.selected) > 0 {
		v.selected = map[string]bool{}
		return true, nil
	}
	return false, nil
}

func (v *manageModel) View(app *AppModel) string {
	v.ensureInit()
	t := app.theme
	var b strings.Builder

	b.WriteString(t.Title().Render(i18n.T("manage.title")))
	b.WriteString("  " + t.Muted().Render(app.mc.SelectedInstanceLabel()))
	b.WriteString("\n")

	// Filters.
	searchLabel := i18n.T("manage.search.label")
	if v.focused {
		b.WriteString(t.Muted().Render(searchLabel+": ") + v.input.View())
	} else if v.search != "" {
		b.WriteString(t.Muted().Render(searchLabel+": ") + t.OnSurface().Render(v.search))
	} else {
		b.WriteString(t.Muted().Render("/ " + searchLabel))
	}
	b.WriteString("  " + t.Muted().Render("F "+i18n.T("manage.filter.label")+": ") + t.Accent().Render(filterLabel(v.filter)))
	b.WriteString("  " + t.Muted().Render("S "+i18n.T("manage.cleanup.scan")))
	if sel := len(v.selected); sel > 0 {
		b.WriteString("  " + t.Accent().Render(i18n.T("download.selection.count", map[string]string{"n": itoa(sel)})))
	}
	b.WriteString("\n\n")

	if !app.mc.HasSelectedInstance() {
		b.WriteString(t.Warning().Render(i18n.T("manage.noInstance")))
		return b.String()
	}

	rows, incompatible := v.rows()
	if len(rows) == 0 {
		if v.search != "" || v.filter != uistate.FilterAll {
			b.WriteString(t.Muted().Render(i18n.T("manage.search.empty")))
		} else {
			b.WriteString(t.Muted().Render(i18n.T("manage.noMods")))
		}
		return b.String()
	}

	const window = 8
	start := v.offset
	end := start + window
	if end > len(rows) {
		end = len(rows)
	}
	for i := start; i < end; i++ {
		row := rows[i]
		if row.Kind == uistate.RowKindIncompatibleToggle {
			line := "▸ " + i18n.T("manage.incompatible.title", map[string]string{"n": itoa(incompatible)})
			hint := i18n.T("manage.incompatible.collapsed")
			if v.expanded {
				hint = i18n.T("manage.incompatible.expanded")
			}
			if i == v.cursor {
				line = t.Selected().Render(stripStyles(line + " — " + hint))
			} else {
				line = t.Warning().Render(line) + "  " + t.Muted().Render(hint)
			}
			b.WriteString(line)
			b.WriteString("\n")
			continue
		}
		group := row.Group
		check := "[ ]"
		if v.selected[row.RowKey()] {
			check = "[✓]"
		}
		state := t.Warning().Render(i18n.T("manage.disabled"))
		if group.Primary.Enabled {
			state = t.Success().Render(i18n.T("manage.enabled"))
		}
		line := check + " " + state + " " + truncate(uistate.DisplayModName(group), 30)
		ids := strings.Join(uistate.StrongModIDs(group), ", ")
		if ids != "" {
			line += "  " + t.Muted().Render(truncate(ids, 24))
		}
		if group.Primary.Version != "" {
			line += "  " + t.Accent().Render(group.Primary.Version)
		}
		if uistate.HasGroupedDetails(group) {
			line += t.Muted().Render(" +")
		}
		if i == v.cursor {
			line = t.Selected().Render(stripStyles(line))
		} else {
			line = t.OnSurface().Render(line)
		}
		b.WriteString(line)
		b.WriteString("\n")
	}
	return strings.TrimRight(b.String(), "\n")
}

func (v *manageModel) FooterHints(app *AppModel) string {
	return "/ " + i18n.T("manage.search.label") + " · ↑↓ · space " + uiText("space") + " · enter " + uiText("toggle") + " · e/d/i · x " + i18n.T("manage.deleteSelected") + " · S " + i18n.T("manage.cleanup.scan") + " · v " + i18n.T("manage.columns.version")
}

func (v *manageModel) RowCount() int {
	rows, _ := v.rows()
	return len(rows)
}

func (v *manageModel) MouseWheel(app *AppModel, delta int) tea.Cmd {
	v.move(delta)
	return nil
}

func (v *manageModel) ClickRow(app *AppModel, index int, ev tea.MouseMsg) tea.Cmd {
	rows := v.currentRows()
	if index < 0 || index >= len(rows) {
		return nil
	}
	if ev.Ctrl {
		v.cursor = index
		v.toggleCurrent()
		return nil
	}
	v.cursor = index
	row := rows[index]
	if row.Kind == uistate.RowKindIncompatibleToggle {
		v.expanded = !v.expanded
		return nil
	}
	v.selected[row.RowKey()] = true
	return nil
}

func (v *manageModel) ListGeometry(app *AppModel) (header, offset, rows int) {
	header = 3
	rowsList, _ := v.rows()
	return header, v.offset, len(rowsList)
}

func filterLabel(filter string) string {
	switch filter {
	case uistate.FilterEnabled:
		return i18n.T("manage.enabled")
	case uistate.FilterDisabled:
		return i18n.T("manage.disabled")
	default:
		return i18n.T("manage.filter.all")
	}
}

// --- disable impact modal ------------------------------------------------------

type disableImpactModal struct {
	id        string
	parent    *manageModel
	groups    []uistate.ModGroup
	action    string
	impacts   []appstructs.LocalModDependencyImpact
	cursor    int
	restoring string
	restored  map[string]bool
}

func newDisableImpactModal(parent *manageModel, groups []uistate.ModGroup, action string, result appstructs.LocalModDisableImpactResult) *disableImpactModal {
	return &disableImpactModal{
		id:       "manage-disable-impact",
		parent:   parent,
		groups:   groups,
		action:   action,
		impacts:  result.Impacts,
		restored: map[string]bool{},
	}
}

func (m *disableImpactModal) ID() string          { return m.id }
func (m *disableImpactModal) CapturesInput() bool { return true }
func (m *disableImpactModal) PreferredWidth() int { return 78 }

func (m *disableImpactModal) Update(app *AppModel, msg tea.Msg) tea.Cmd {
	if typed, ok := msg.(restoreDependencyMsg); ok {
		m.restoring = ""
		if typed.Result.Queued {
			if m.cursor < len(m.impacts) {
				m.restored[m.impacts[m.cursor].ModID] = true
			}
			return app.toastT("manage.disableImpact.queued", toastSuccess)
		}
		return app.toastT("manage.disableImpact.restoreFailed", toastError)
	}
	key, ok := msg.(tea.KeyMsg)
	if !ok {
		return nil
	}
	switch key.String() {
	case "esc":
		app.popModal(m.id)
		m.parent.pending = nil
	case "up", "k":
		if m.cursor > 0 {
			m.cursor--
		}
	case "down", "j":
		if m.cursor < len(m.impacts)-1 {
			m.cursor++
		}
	case "enter":
		app.popModal(m.id)
		return m.parent.executeBatch(app, m.groups, m.action)
	case "r":
		return m.restoreCurrent(app)
	}
	return nil
}

func (m *disableImpactModal) restoreCurrent(app *AppModel) tea.Cmd {
	if m.restoring != "" || m.cursor >= len(m.impacts) {
		return nil
	}
	impact := m.impacts[m.cursor]
	if m.restored[impact.ModID] {
		return nil
	}
	if len(impact.Candidates) == 0 {
		return app.toastT("manage.disableImpact.cacheMiss", toastWarning)
	}
	if len(impact.Candidates) == 1 {
		return m.restore(app, impact.ModID, impact.Candidates[0])
	}
	items := make([]selectItem, 0, len(impact.Candidates))
	for _, c := range impact.Candidates {
		items = append(items, selectItem{
			Label:  defaultStr(c.Title, c.ProjectID),
			Value:  c.Platform + "\x00" + c.ProjectID + "\x00" + c.VersionID,
			Detail: c.Platform,
		})
	}
	modID := impact.ModID
	app.pushModal(newSelectModal("manage-restore-source", i18n.T("manage.disableImpact.chooseSource"), items,
		func(app *AppModel, item selectItem) tea.Cmd {
			parts := strings.Split(item.Value, "\x00")
			candidate := appstructs.CachedDependencyCandidate{ModID: modID}
			if len(parts) >= 3 {
				candidate = appstructs.CachedDependencyCandidate{ModID: modID, Platform: parts[0], ProjectID: parts[1], VersionID: parts[2]}
			}
			return m.restore(app, modID, candidate)
		}))
	return nil
}

func (m *disableImpactModal) restore(app *AppModel, modID string, candidate appstructs.CachedDependencyCandidate) tea.Cmd {
	m.restoring = modID
	return cmdRestoreCachedDependency(app.host, appstructs.RestoreCachedDependencyRequest{
		ModID:     modID,
		Platform:  candidate.Platform,
		ProjectID: candidate.ProjectID,
		VersionID: candidate.VersionID,
	})
}

func (m *disableImpactModal) View(app *AppModel) string {
	t := app.theme
	var b strings.Builder
	b.WriteString(t.Title().Render(i18n.T("manage.disableImpact.title")))
	b.WriteString("\n")
	b.WriteString(t.Muted().Render(i18n.T("manage.disableImpact.body")))
	b.WriteString("\n\n")
	for i, impact := range m.impacts {
		names := []string{}
		for _, mod := range impact.AffectedMods {
			names = append(names, defaultStr(mod.Name, defaultStr(mod.FileName, mod.Path)))
		}
		line := "⚠ " + i18n.T("manage.disableImpact.missing", map[string]string{"id": impact.ModID})
		line += "  " + t.Muted().Render(i18n.T("manage.disableImpact.affected", map[string]string{"mods": strings.Join(names, ", ")}))
		if len(impact.Candidates) == 0 {
			line += "  " + t.Muted().Render("["+i18n.T("manage.disableImpact.cacheMiss")+"]")
		} else if m.restored[impact.ModID] {
			line += "  " + t.Success().Render("✓")
		}
		if i == m.cursor {
			line = t.Selected().Render(stripStyles(line))
		} else {
			line = t.Warning().Render(stripStyles(line))
		}
		b.WriteString(line)
		b.WriteString("\n")
	}
	b.WriteString("\n")
	b.WriteString(t.Muted().Render("r " + i18n.T("manage.disableImpact.restore") + " · enter " + i18n.T("manage.disableImpact.continue") + " · esc " + i18n.T("manage.disableImpact.cancel")))
	return b.String()
}
