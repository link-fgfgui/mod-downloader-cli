package tviewui

import (
	"strings"

	"github.com/atotto/clipboard"
	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"

	"github.com/link-fgfgui/mod-downloader-core/models"
	appstructs "github.com/link-fgfgui/mod-downloader-core/structs"

	"github.com/link-fgfgui/mod-downloader-cli/internal/i18n"
	"github.com/link-fgfgui/mod-downloader-cli/internal/uistate"
)

// managePage renders local Mod management.
type managePage struct {
	app *App
	box *tview.Flex

	header *tview.TextView
	search *tview.InputField
	bar    *tview.TextView
	list   *wheelList
	action *actionBar

	filter        string
	barLang       string
	focusedSearch bool
	selected      map[string]bool
	expanded      bool
	rebuilding    bool

	groups   []uistate.ModGroup
	rows     []uistate.ManageRow
	filtered []uistate.ModGroup

	pending       []uistate.ModGroup
	pendingAction string
}

func newManagePage(a *App) *managePage {
	v := &managePage{app: a, filter: uistate.FilterAll, selected: map[string]bool{}}

	v.header = tview.NewTextView().SetDynamicColors(true)
	v.header.SetBackgroundColor(a.theme.Surface)

	v.search = tview.NewInputField()
	v.search.SetLabel(" › ")
	v.search.SetPlaceholder(i18n.T("manage.search.label"))
	v.search.SetFieldWidth(0)
	v.search.SetBorder(true)
	v.search.SetTitle(i18n.T("manage.search.label"))
	a.theme.styleInput(v.search)
	a.theme.styleBox(v.search.Box)
	v.search.SetDoneFunc(func(key tcell.Key) {
		switch key {
		case tcell.KeyEnter:
			v.focusedSearch = false
			v.app.application.SetFocus(v.list)
		case tcell.KeyEscape:
			v.focusedSearch = false
			v.app.application.SetFocus(v.list)
		}
		v.app.updateStatus()
	})
	v.search.SetChangedFunc(func(text string) {
		v.selected = map[string]bool{}
		v.rebuildRows(false)
	})

	v.bar = tview.NewTextView().SetDynamicColors(true)
	v.bar.SetBackgroundColor(a.theme.Surface)
	v.bar.SetTextColor(a.theme.OnSurface)

	v.list = newWheelList()
	v.list.ShowSecondaryText(true)
	v.list.SetUseStyleTags(true, true)
	v.list.SetWrapAround(false)
	v.list.SetBorder(true)
	v.list.SetTitle(i18n.T("manage.title"))
	a.theme.styleBox(v.list.Box)
	a.theme.styleList(v.list.List)
	// A click toggles the checkbox; activating a row stays on Enter so the
	// mouse cannot enable/disable a mod by accident.
	v.list.SetSelectedFunc(func(index int, _, _ string, _ rune) {
		v.toggleAt(index)
	})

	v.action = newActionBar(a)
	v.action.SetRefocus(v.Focus)

	v.box = tview.NewFlex().SetDirection(tview.FlexRow).
		AddItem(v.header, 1, 0, false).
		AddItem(v.search, 3, 0, false).
		AddItem(v.bar, 1, 0, false).
		AddItem(v.list, 0, 1, true).
		AddItem(v.action.Primitive(), 1, 0, false)

	v.buildActions()
	v.renderHeader()
	v.Refresh()
	return v
}

func (v *managePage) Primitive() tview.Primitive { return v.box }

func (v *managePage) Focus() {
	if v.focusedSearch {
		v.app.application.SetFocus(v.search)
		return
	}
	v.app.application.SetFocus(v.list)
}

func (v *managePage) OnActivate() {
	v.Refresh()
	if v.app.mc.HasSelectedInstance() {
		go v.app.refreshSelectedMods()
	}
}

func (v *managePage) ClearFocus() bool {
	if v.focusedSearch {
		v.focusedSearch = false
		v.search.Blur()
		v.app.application.SetFocus(v.list)
		v.app.updateStatus()
		return true
	}
	if len(v.selected) > 0 {
		v.selected = map[string]bool{}
		v.Refresh()
		return true
	}
	return false
}

func (v *managePage) HandleKey(event *tcell.EventKey) bool {
	if v.focusedSearch {
		if event.Key() == tcell.KeyTab || event.Key() == tcell.KeyBacktab {
			v.focusedSearch = false
			v.search.Blur()
			v.app.application.SetFocus(v.list)
			return true
		}
		return false
	}
	if event.Key() == tcell.KeyTab || event.Key() == tcell.KeyBacktab {
		v.focusedSearch = true
		v.app.application.SetFocus(v.search)
		return true
	}
	if event.Key() == tcell.KeyEnter {
		v.enterRow(v.list.GetCurrentItem())
		return true
	}
	if event.Key() == tcell.KeyCtrlA {
		v.selectAll()
		return true
	}
	if event.Key() != tcell.KeyRune {
		return false
	}
	switch event.Rune() {
	case '/':
		v.focusedSearch = true
		v.app.application.SetFocus(v.search)
		return true
	case ' ':
		v.toggleCurrent()
		return true
	case 'F':
		v.filter = cycleString(v.filter, []string{uistate.FilterAll, uistate.FilterEnabled, uistate.FilterDisabled}, 1)
		v.selected = map[string]bool{}
		v.rebuildRows(true)
		return true
	case 'e':
		v.applyBatch("enable")
		return true
	case 'd':
		v.applyBatch("disable")
		return true
	case 'i':
		v.applyBatch("invert")
		return true
	case 'x':
		v.confirmDelete()
		return true
	case 'S':
		v.scanUnused()
		return true
	case 'c':
		v.copySelection(false)
		return true
	case 'C':
		v.copySelection(true)
		return true
	case 'f':
		v.addToFavorites()
		return true
	case 'v':
		v.openVersionDialog()
		return true
	case 'r':
		go v.app.refreshSelectedMods()
		return true
	}
	return false
}

func (v *managePage) Hints() []hint {
	if v.focusedSearch {
		return []hint{
			hintText("type"),
			hintText("esc " + uiText("cancel")),
			hintText("tab " + uiText("navigate")),
		}
	}
	return []hint{
		hintAction("/ "+i18n.T("manage.search.label"), func() {
			v.focusedSearch = true
			v.app.application.SetFocus(v.search)
			v.app.updateStatus()
		}),
		hintText("↑↓ " + uiText("navigate")),
		hintAction("space "+uiText("space"), v.toggleCurrent),
		hintAction("enter "+uiText("toggle"), func() { v.enterRow(v.list.GetCurrentItem()) }),
		hintAction("x "+i18n.T("manage.deleteSelected"), v.confirmDelete),
		hintAction("S "+uiText("scan"), v.scanUnused),
		hintAction("v "+i18n.T("manage.columns.version"), v.openVersionDialog),
		hintAction("r "+uiText("refresh"), func() { go v.app.refreshSelectedMods() }),
	}
}

func (v *managePage) OnResize(_ int) {
	v.renderHeader()
	v.action.Reflow()
}

func (v *managePage) Restyle() {
	t := v.app.theme
	t.styleInput(v.search)
	t.styleBox(v.search.Box)
	t.styleList(v.list.List)
	t.styleBox(v.list.Box)
	v.header.SetBackgroundColor(t.Surface)
	v.bar.SetBackgroundColor(t.Surface)
	v.bar.SetTextColor(t.OnSurface)
	v.action.Restyle()
}

func (v *managePage) renderHeader() {
	t := v.app.theme
	instance := v.app.mc.SelectedInstanceLabel()
	if instance == "" {
		instance = uiText("noInstance")
	}
	v.header.SetText(t.tagWith(t.Primary, "b") + i18n.T("manage.title") + resetTag +
		"  " + t.tag(t.Muted) + tview.Escape(truncate(instance, maxInt(12, v.app.mainWidth()-20))) + resetTag)
}

// buildActions (re)labels the clickable action bar for the current language.
func (v *managePage) buildActions() {
	v.barLang = i18n.Current()
	v.action.Set(
		pageAction{Label: uiText("selectAll"), Run: func() { v.selectAll() }},
		pageAction{Label: i18n.T("manage.enableSelected"), Run: func() { v.applyBatch("enable") }},
		pageAction{Label: i18n.T("manage.disableSelected"), Run: func() { v.applyBatch("disable") }},
		pageAction{Label: i18n.T("manage.deleteSelected"), Run: func() { v.confirmDelete() }},
		pageAction{Label: uiText("scan"), Run: func() { v.scanUnused() }},
		pageAction{Label: uiText("refresh"), Run: func() { go v.app.refreshSelectedMods() }},
	)
}

func (v *managePage) Refresh() {
	if v.barLang != i18n.Current() {
		v.buildActions()
	}
	v.search.SetTitle(i18n.T("manage.search.label"))
	v.search.SetPlaceholder(i18n.T("manage.search.label"))
	v.list.SetTitle(i18n.T("manage.title"))
	v.renderHeader()
	v.buildRows()
	v.rebuildList()
	v.renderBar()
}

func (v *managePage) buildRows() {
	mods := v.app.mc.Mods()
	v.groups = uistate.BuildGroups(mods)
	filtered := uistate.FilterGroups(v.groups, strings.ToLower(strings.TrimSpace(v.search.GetText())), v.filter)
	v.filtered = filtered
	rows, _ := uistate.PartitionAndBuildRows(filtered, v.app.mc.SelectedModLoader, v.expanded)
	v.rows = rows
	// Drop selections whose rows are no longer visible so the action bar cannot
	// claim a selection the user cannot see.
	visible := map[string]bool{}
	for _, row := range rows {
		if row.Selectable() {
			visible[row.RowKey()] = true
		}
	}
	for key := range v.selected {
		if !visible[key] {
			delete(v.selected, key)
		}
	}
}

// rebuildRows rebuilds the row model and list, optionally preserving the
// cursor; buildRows alone is used by Refresh.
func (v *managePage) rebuildRows(keepCursor bool) {
	key := ""
	if keepCursor {
		key = v.currentRowKey()
	}
	v.buildRows()
	v.rebuildListAt(key)
	v.renderBar()
}

func (v *managePage) currentRowKey() string {
	idx := v.list.GetCurrentItem()
	if idx < 0 || idx >= len(v.rows) {
		return ""
	}
	return v.rows[idx].RowKey()
}

func (v *managePage) renderBar() {
	if len(v.selected) == 0 {
		t := v.app.theme
		bar := "F " + t.tag(t.Secondary) + "[" + filterLabel(v.filter) + "]" + resetTag +
			"   " + t.tag(t.Secondary) + "S " + i18n.T("manage.cleanup.scan") + resetTag
		v.bar.SetText(bar)
		return
	}
	t := v.app.theme
	bar := i18n.T("download.selection.count", map[string]string{"n": itoa(len(v.selected))}) +
		"  e " + uiText("stateInstalled") + " · d " + uiText("stateIncompatible") +
		" · i · x " + i18n.T("manage.deleteSelected") +
		" · f " + i18n.T("favorites.addToFavorites") +
		" · esc " + i18n.T("download.selection.deselectAll")
	v.bar.SetText(t.tagWith(t.Secondary, "b") + bar + resetTag)
}

func (v *managePage) rebuildList() { v.rebuildListAt(v.currentRowKey()) }

func (v *managePage) rebuildListAt(restoreKey string) {
	v.rebuilding = true
	defer func() { v.rebuilding = false }()
	v.list.Clear()
	if !v.app.mc.HasSelectedInstance() {
		v.list.SetTitle(i18n.T("manage.title"))
		v.list.AddItem(v.app.theme.tag(v.app.theme.Muted)+i18n.T("manage.noInstance")+resetTag, "", 0, nil)
		return
	}
	if len(v.rows) == 0 {
		text := i18n.T("manage.noMods")
		if strings.TrimSpace(v.search.GetText()) != "" || v.filter != uistate.FilterAll {
			text = i18n.T("manage.search.empty")
		}
		v.list.AddItem(v.app.theme.tag(v.app.theme.Muted)+text+resetTag, "", 0, nil)
		return
	}
	for _, row := range v.rows {
		v.list.AddItem(v.rowMain(row), v.rowSecondary(row), 0, nil)
	}
	if restoreKey != "" {
		for i, row := range v.rows {
			if row.RowKey() == restoreKey {
				v.list.SetCurrentItem(i)
				return
			}
		}
	}
	if cur := v.list.GetCurrentItem(); cur >= len(v.rows) {
		v.list.SetCurrentItem(maxInt(0, len(v.rows)-1))
	}
}

func (v *managePage) rowMain(row uistate.ManageRow) string {
	t := v.app.theme
	if row.Kind == uistate.RowKindIncompatibleToggle {
		_, incompatible := uistate.PartitionAndBuildRows(v.filtered, v.app.mc.SelectedModLoader, v.expanded)
		hint := i18n.T("manage.incompatible.collapsed")
		if v.expanded {
			hint = i18n.T("manage.incompatible.expanded")
		}
		return t.tag(t.Warning) + "⚠ " + i18n.T("manage.incompatible.title", map[string]string{"n": itoa(incompatible)}) + resetTag +
			"  " + t.tag(t.Muted) + hint + resetTag
	}
	group := row.Group
	check := checkBoxTagged(v.selected[row.RowKey()])
	state := t.tag(t.Warning) + "○" + resetTag
	if group.Primary.Enabled {
		state = t.tag(t.Success) + "●" + resetTag
	}
	main := check + " " + state + " " + tview.Escape(uistate.DisplayModName(group))
	if group.Primary.Version != "" {
		main += "   " + t.tag(t.Secondary) + tview.Escape(group.Primary.Version) + resetTag
	}
	categories := uistate.ModCategories(group)
	if len(categories) > 0 && v.app.mainWidth() > 70 {
		main += " " + t.tag(t.Muted) + "[" + tview.Escape(categories[0]) + "]" + resetTag
	}
	return main
}

func (v *managePage) rowSecondary(row uistate.ManageRow) string {
	t := v.app.theme
	if row.Kind == uistate.RowKindIncompatibleToggle {
		return ""
	}
	group := row.Group
	detail := strings.Join(uistate.StrongModIDs(group), ", ")
	if group.Primary.FileName != "" {
		if detail != "" {
			detail += " · "
		}
		detail += group.Primary.FileName
	}
	if uistate.HasGroupedDetails(group) {
		detail += "  [+]"
	}
	return t.tag(t.Muted) + tview.Escape(truncate(detail, maxInt(16, v.app.mainWidth()-8))) + resetTag
}

// enterRow activates a row: the incompatible header expands or collapses and
// every other row applies its primary action (enable/disable). Selection is
// left alone; the checkbox is toggled by Space or a click.
func (v *managePage) enterRow(index int) {
	if index < 0 || index >= len(v.rows) {
		return
	}
	row := v.rows[index]
	if row.Kind == uistate.RowKindIncompatibleToggle {
		v.expanded = !v.expanded
		v.rebuildRows(true)
		return
	}
	v.applyBatchTo([]uistate.ModGroup{row.Group}, uistate.PrimaryBatchAction([]uistate.ModGroup{row.Group}))
}

// toggleAt flips the checkbox of one row and repaints just that row.
func (v *managePage) toggleAt(index int) {
	if index < 0 || index >= len(v.rows) {
		return
	}
	row := v.rows[index]
	if !row.Selectable() {
		return
	}
	key := row.RowKey()
	if v.selected[key] {
		delete(v.selected, key)
	} else {
		v.selected[key] = true
	}
	v.list.SetItemText(index, v.rowMain(row), v.rowSecondary(row))
	v.renderBar()
}

func (v *managePage) toggleCurrent() { v.toggleAt(v.list.GetCurrentItem()) }

func (v *managePage) selectAll() {
	for _, row := range v.rows {
		if row.Selectable() {
			v.selected[row.RowKey()] = true
		}
	}
	v.rebuildList()
	v.renderBar()
}

func (v *managePage) selectedGroups() []uistate.ModGroup {
	groups := []uistate.ModGroup{}
	for _, row := range v.rows {
		if row.Selectable() && v.selected[row.RowKey()] {
			groups = append(groups, row.Group)
		}
	}
	if len(groups) == 0 {
		idx := v.list.GetCurrentItem()
		if idx >= 0 && idx < len(v.rows) && v.rows[idx].Selectable() {
			groups = append(groups, v.rows[idx].Group)
		}
	}
	return groups
}

func (v *managePage) applyBatch(action string) {
	groups := v.selectedGroups()
	if len(groups) == 0 {
		return
	}
	v.applyBatchTo(groups, action)
}

func (v *managePage) applyBatchTo(groups []uistate.ModGroup, action string) {
	paths := uistate.SelectedGroupPaths(groups)
	if len(paths) == 0 {
		return
	}
	if action == "disable" || action == "invert" {
		v.pending = groups
		v.pendingAction = action
		go v.app.analyzeDisableImpact(appstructs.LocalModDisableImpactRequest{Paths: paths, Action: action})
		return
	}
	v.executeBatch(groups, action)
}

func (v *managePage) executeBatch(groups []uistate.ModGroup, action string) {
	paths := uistate.SelectedGroupPaths(groups)
	if len(paths) == 0 {
		return
	}
	v.selected = map[string]bool{}
	go v.app.applyLocalBatch(paths, action)
}

// onDisableImpact continues or prompts after the core analysis.
func (v *managePage) onDisableImpact(result appstructs.LocalModDisableImpactResult, request appstructs.LocalModDisableImpactRequest) {
	if len(result.Impacts) == 0 {
		groups := v.pending
		action := v.pendingAction
		v.pending = nil
		v.executeBatch(groups, action)
		return
	}
	groups := v.pending
	action := v.pendingAction
	v.app.openDisableImpactDialog(groups, action, result)
}

func (v *managePage) confirmDelete() {
	groups := v.selectedGroups()
	if len(groups) == 0 {
		return
	}
	paths := uistate.SelectedGroupPaths(groups)
	autoScan := v.app.settings.AutoScanUnusedDependencies
	v.app.openConfirm(
		i18n.T("manage.confirmDelete.title"),
		i18n.T("manage.confirmDelete.body", map[string]string{"n": itoa(len(paths))}),
		[]confirmOption{
			{Label: i18n.T("manage.confirmDelete.confirm"), Value: "confirm", Destructive: true},
			{Label: i18n.T("manage.confirmDelete.cancel"), Value: "cancel"},
		},
		func(value string) {
			if value != "confirm" {
				return
			}
			if autoScan {
				go v.app.scanUnusedDependencies(appstructs.UnusedDependencyScanRequest{ExcludedPaths: paths})
				go v.app.applyLocalBatch(paths, "delete")
				return
			}
			v.executeBatch(groups, "delete")
		},
	)
}

func (v *managePage) scanUnused() {
	if !v.app.mc.HasSelectedInstance() {
		return
	}
	go v.app.scanUnusedAll()
}

// onUnusedScan renders the cleanup confirmation.
func (v *managePage) onUnusedScan(result appstructs.UnusedDependencyScanResult) {
	if len(result.Candidates) == 0 {
		v.app.toastT("manage.cleanup.none")
		return
	}
	lines := []string{}
	for i, c := range result.Candidates {
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
		lines = append(lines, "· "+name+"  "+strings.Join(c.ModIDs, ", "))
	}
	candidates := result.Candidates
	v.app.toastT("manage.cleanup.found", map[string]string{"n": itoa(len(candidates))})
	v.app.openConfirm(
		i18n.T("manage.cleanup.dialogTitle"),
		strings.Join(lines, "\n"),
		[]confirmOption{
			{Label: i18n.T("manage.cleanup.deleteCandidates", map[string]string{"n": itoa(len(candidates))}), Value: "confirm", Destructive: true},
			{Label: i18n.T("manage.cleanup.cancel"), Value: "cancel"},
		},
		func(value string) {
			if value != "confirm" {
				return
			}
			paths := make([]string, 0, len(candidates))
			for _, c := range candidates {
				if c.Path != "" {
					paths = append(paths, c.Path)
				}
			}
			go v.app.applyLocalBatch(paths, "delete")
		},
	)
}

func (v *managePage) copySelection(names bool) {
	groups := v.selectedGroups()
	if len(groups) == 0 {
		return
	}
	values := []string{}
	for _, group := range groups {
		if names {
			values = append(values, uistate.StrongModNames(group)...)
		} else {
			values = append(values, uistate.StrongModIDs(group)...)
		}
	}
	if err := clipboard.WriteAll(strings.Join(values, "\n")); err != nil {
		v.app.toast(err.Error())
		return
	}
	v.app.toast(uiText("copy"))
}

func (v *managePage) addToFavorites() {
	groups := v.selectedGroups()
	if len(groups) == 0 {
		return
	}
	if !uistate.CanFavoriteSelection(groups, v.app.mc.SelectedMinecraftVersion, v.app.mc.SelectedModLoader) {
		v.app.toastT("favorites.invalidSelection")
		return
	}
	drafts := make([]favoriteDraft, 0, len(groups))
	for _, group := range groups {
		drafts = append(drafts, favoriteDraftFromGroup(group, v.app))
	}
	v.app.openAddFavoriteDialog(drafts)
}

func (v *managePage) openVersionDialog() {
	groups := v.selectedGroups()
	if len(groups) != 1 {
		return
	}
	group := groups[0]
	if !uistate.CanReplaceVersion(group, v.app.mc.HasSelectedInstance()) {
		return
	}
	project := uistate.OnlineProjectFromGroup(group)
	v.app.openVersionDialog(project, "replace", func(version models.ModVersion) {
		go v.app.pinModVersion(appstructs.ModVersionPinRequest{
			Platform: project.Platform, ModID: project.ProjectID, VersionID: version.ID,
			MinecraftVersion: v.app.mc.SelectedMinecraftVersion, ModLoader: v.app.mc.SelectedModLoader,
		})
	}, func(version models.ModVersion) {
		go v.app.queueDownload(appstructs.ModDownloadRequest{
			ProjectID:        group.Primary.OnlineProjectID,
			Result:           project,
			MinecraftVersion: v.app.mc.SelectedMinecraftVersion,
			ModLoader:        v.app.mc.SelectedModLoader,
			VersionID:        version.ID,
		})
	})
	p := v.app.versionDialog
	if p != nil {
		p.installedID = group.Primary.OnlineVersionID
		p.installedSHA1 = group.Primary.SHA1
	}
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

func cycleString(value string, options []string, delta int) string {
	idx := 0
	for i, option := range options {
		if option == value {
			idx = i
			break
		}
	}
	idx = (idx + delta) % len(options)
	if idx < 0 {
		idx += len(options)
	}
	return options[idx]
}
