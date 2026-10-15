package tui

import (
	"strings"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/link-fgfgui/mod-downloader-core/appcore"
	"github.com/link-fgfgui/mod-downloader-core/models"
	"github.com/link-fgfgui/mod-downloader-core/storage"

	"github.com/link-fgfgui/mod-downloader-cli/internal/uistate"
	"github.com/link-fgfgui/mod-downloader-cli/tui/i18n"
)

// favoriteDraft is the UI-neutral payload for adding mods to a favorite list.
type favoriteDraft struct {
	Platform         string
	ModID            string
	VersionID        string
	MinecraftVersion string
	ModLoader        string
	Title            string
	Slug             string
	IconURL          string
	Description      string
	Categories       []string
}

func favoriteDraftFromProject(project models.ModProject, app *AppModel) favoriteDraft {
	return favoriteDraft{
		Platform:         project.Platform,
		ModID:            project.ProjectID,
		MinecraftVersion: app.mc.SelectedMinecraftVersion,
		ModLoader:        app.mc.SelectedModLoader,
		Title:            project.Title,
		Slug:             project.Slug,
		IconURL:          project.IconURL,
		Description:      project.Description,
		Categories:       project.Categories,
	}
}

func favoriteDraftFromGroup(group uistate.ModGroup, app *AppModel) favoriteDraft {
	primary := group.Primary
	mcVersion := defaultStr(app.mc.SelectedMinecraftVersion, primary.Version)
	modLoader := defaultStr(app.mc.SelectedModLoader, "")
	return favoriteDraft{
		Platform:         primary.OnlinePlatform,
		ModID:            primary.OnlineProjectID,
		MinecraftVersion: mcVersion,
		ModLoader:        modLoader,
		Title:            uistate.DisplayModName(group),
		Slug:             primary.OnlineSlug,
		IconURL:          primary.IconURL,
		Description:      primary.Description,
		Categories:       uistate.ModCategories(group),
	}
}

// openAddFavorites pushes the add-to-favorite dialog for the given drafts.
func openAddFavorites(app *AppModel, drafts []favoriteDraft) tea.Cmd {
	if len(drafts) == 0 {
		return nil
	}
	if !uistate.HasFavoriteScope(app.mc.SelectedMinecraftVersion, app.mc.SelectedModLoader) {
		return app.toastT("favorites.empty.selectOrCreate", toastWarning)
	}
	lists := filterScopedLists(app.favorites.lists, app)
	modal := newAddFavoriteModal("add-favorites", lists, len(drafts), func(app *AppModel, ids []string) tea.Cmd {
		mods := make([]storage.FavoriteMod, 0, len(drafts))
		for _, d := range drafts {
			mods = append(mods, storage.FavoriteMod{
				Platform:         d.Platform,
				ModID:            d.ModID,
				VersionID:        d.VersionID,
				MinecraftVersion: d.MinecraftVersion,
				ModLoader:        d.ModLoader,
				Title:            d.Title,
				Slug:             d.Slug,
				IconURL:          d.IconURL,
				Description:      d.Description,
				Categories:       d.Categories,
			})
		}
		return cmdAddFavoriteMods(app.host, appcore.FavoriteBulkAddRequest{TargetListIDs: ids, Mods: mods})
	})
	app.pushModal(modal)
	return cmdListFavoriteLists(app.host, "")
}

// favoritesModel renders the favorites view (Phase 5).
type favoritesModel struct {
	app            *AppModel
	lists          []storage.FavoriteList
	selectedListID string
	items          []storage.FavoriteModEntry
	pane           int // 0 = lists, 1 = items
	listCursor     int
	itemCursor     int
	itemOffset     int
	itemSelected   map[string]bool
	mcVersion      string
	modLoader      string
	loaded         bool
}

func (v *favoritesModel) ensureInit() {
	if v.itemSelected == nil {
		v.itemSelected = map[string]bool{}
	}
}

// SetScope mirrors favorites.setDisplayScope.
func (v *favoritesModel) SetScope(mcVersion, modLoader string) bool {
	nextMC := uistate.NormalizeMinecraftVersion(mcVersion)
	nextLoader := uistate.NormalizeKeyPart(modLoader)
	if v.mcVersion == nextMC && v.modLoader == nextLoader {
		return false
	}
	v.mcVersion = nextMC
	v.modLoader = nextLoader
	if list := v.selectedList(); list == nil {
		v.closeSelected()
	}
	return true
}

func (v *favoritesModel) selectedList() *storage.FavoriteList {
	for i := range v.lists {
		if v.lists[i].ID == v.selectedListID && uistate.ListMatchesScope(v.lists[i], v.mcVersion, v.modLoader) {
			return &v.lists[i]
		}
	}
	return nil
}

func (v *favoritesModel) closeSelected() {
	v.selectedListID = ""
	v.items = nil
	v.itemSelected = map[string]bool{}
}

// visibleLists mirrors pinnedLists + ungroupedLists flattened for cursor nav.
func (v *favoritesModel) visibleLists() []storage.FavoriteList {
	pinned, ungrouped := uistate.PartitionLists(v.lists, v.mcVersion, v.modLoader)
	return append(pinned, ungrouped...)
}

func (v *favoritesModel) Update(app *AppModel, msg tea.Msg) tea.Cmd {
	v.ensureInit()
	switch typed := msg.(type) {
	case viewActivatedMsg:
		return cmdListFavoriteLists(app.host, v.selectedListID)
	case favoritesLoadedMsg:
		v.lists = typed.Lists
		if typed.ListID != "" && typed.ListID == v.selectedListID {
			v.items = uistate.FavoriteContentsForScope(typed.Contents, v.mcVersion, v.modLoader).Mods
		}
		if typed.ListID == "" {
			v.ensureInit()
		}
		return nil
	case bulkFavoriteResultMsg:
		return app.toastT("favorites.added", toastSuccess)
	case exportPackwizMsg:
		return app.toastT("favorites.export.success", toastSuccess)
	case crossVersionPreviewMsg:
		return v.onCrossVersionPreview(app, typed)
	case crossVersionAppliedMsg:
		if typed.Result.Applied {
			return tea.Batch(app.toastT("favorites.crossVersionCopy.success", toastSuccess),
				cmdListFavoriteLists(app.host, v.selectedListID))
		}
		return app.toastT("favorites.crossVersionCopy.notApplied", toastWarning)
	case tea.KeyMsg:
		return v.key(app, typed)
	}
	return nil
}

func (v *favoritesModel) key(app *AppModel, key tea.KeyMsg) tea.Cmd {
	if key.String() == "tab" {
		v.pane = 1 - v.pane
		return nil
	}
	if v.pane == 1 {
		return v.itemsKey(app, key)
	}
	return v.listsKey(app, key)
}

func (v *favoritesModel) listsKey(app *AppModel, key tea.KeyMsg) tea.Cmd {
	lists := v.visibleLists()
	switch key.String() {
	case "up", "k":
		if v.listCursor > 0 {
			v.listCursor--
		}
	case "down", "j":
		if v.listCursor < len(lists)-1 {
			v.listCursor++
		}
	case "enter":
		if v.listCursor < len(lists) {
			v.selectedListID = lists[v.listCursor].ID
			v.items = nil
			v.itemSelected = map[string]bool{}
			return cmdListFavoriteLists(app.host, v.selectedListID)
		}
	case "n":
		return v.promptCreate(app)
	case "R":
		return v.promptRename(app, lists)
	case "p":
		if v.listCursor < len(lists) {
			list := lists[v.listCursor]
			list.Pinned = !list.Pinned
			return cmdUpdateFavoriteMetadata(app.host, list)
		}
	case "x":
		return v.confirmDelete(app, lists)
	case "J":
		return v.reorder(app, lists, 1)
	case "K":
		return v.reorder(app, lists, -1)
	case "e":
		return v.exportPackwiz(app)
	case "o":
		return v.openAsSearchResults(app)
	case "l":
		return v.copyListDialog(app)
	case "f":
		return v.referenceDialog(app)
	case "F":
		return v.manageReferences(app)
	case "i":
		return v.iconDialog(app, lists)
	case "V":
		return v.crossVersionCopy(app)
	}
	return nil
}

// manageReferences opens the reference manager for the selected list.
func (v *favoritesModel) manageReferences(app *AppModel) tea.Cmd {
	list := v.selectedList()
	if list == nil {
		return nil
	}
	app.pushModal(newFavoriteRefsModal(list.ID))
	return cmdListFavoriteRefs(app.host, list.ID)
}

// iconDialog mirrors updateListIcon/clearListIcon: choose between an mdi icon,
// a project icon resolved via LookupProjectBySlug, or clearing the icon.
func (v *favoritesModel) iconDialog(app *AppModel, lists []storage.FavoriteList) tea.Cmd {
	if v.listCursor >= len(lists) {
		return nil
	}
	list := lists[v.listCursor]
	app.pushModal(newSelectModal("fav-icon", i18n.T("favorites.dialog.customizeTitle"), []selectItem{
		{Label: i18n.T("favorites.dialog.mdiIcon"), Value: "mdi"},
		{Label: i18n.T("favorites.dialog.projectIcon"), Value: "project"},
		{Label: i18n.T("favorites.actions.clearIcon"), Value: "clear"},
	}, func(app *AppModel, item selectItem) tea.Cmd {
		switch item.Value {
		case "clear":
			return cmdSetFavoriteListIcon(app.host, list, "", "", "")
		case "mdi":
			app.pushModal(newPromptModal("fav-icon-mdi", i18n.T("favorites.dialog.mdiIcon"), "", list.IconValue, "mdi-star",
				func(app *AppModel, value string) tea.Cmd {
					return cmdSetFavoriteListIcon(app.host, list, "mdi", value, "modrinth")
				}))
			return nil
		case "project":
			app.pushModal(newPromptModal("fav-icon-project", i18n.T("favorites.dialog.projectIcon"), "", list.IconValue, i18n.T("favorites.dialog.projectSlug"),
				func(app *AppModel, value string) tea.Cmd {
					return cmdSetFavoriteListIcon(app.host, list, "project", value, "modrinth")
				}))
			return nil
		}
		return nil
	}))
	return nil
}

// crossVersionCopy mirrors the GUI's cross-version migration flow: prompt for
// the target tuple, preview, then apply.
func (v *favoritesModel) crossVersionCopy(app *AppModel) tea.Cmd {
	source := v.selectedList()
	if source == nil {
		return nil
	}
	initial := app.mc.SelectedMinecraftVersion + "|" + app.mc.SelectedModLoader
	modal := newPromptModal("fav-cross-version", i18n.T("favorites.dialog.crossVersionCopyTitle"), i18n.T("favorites.dialog.minecraftVersion")+"|"+i18n.T("favorites.dialog.modLoader"), initial, "1.20.1|Fabric",
		func(app *AppModel, value string) tea.Cmd {
			parts := strings.SplitN(value, "|", 2)
			mcVersion := strings.TrimSpace(parts[0])
			modLoader := uistate.DefaultModLoader
			if len(parts) == 2 && strings.TrimSpace(parts[1]) != "" {
				modLoader = uistate.MatchingModLoader(parts[1])
			}
			if mcVersion == "" {
				return nil
			}
			return cmdPreviewCrossVersionCopy(app.host, appcore.FavoriteCrossVersionCopyRequest{
				SourceListID:     source.ID,
				MinecraftVersion: mcVersion,
				ModLoader:        modLoader,
			})
		})
	app.pushModal(modal)
	return nil
}

// onCrossVersionPreview renders the preview then applies on confirmation.
func (v *favoritesModel) onCrossVersionPreview(app *AppModel, msg crossVersionPreviewMsg) tea.Cmd {
	preview := msg.Preview
	body := i18n.T("favorites.crossVersionCopy.target", map[string]string{
		"target": preview.MinecraftVersion + "|" + preview.ModLoader,
	})
	body += "\n" + i18n.T("favorites.crossVersionCopy.matched", map[string]string{"n": itoa(len(preview.Matched))})
	if len(preview.Conflicts) > 0 {
		body += "\n" + i18n.T("favorites.crossVersionCopy.conflicts", map[string]string{"n": itoa(len(preview.Conflicts))})
	}
	if preview.NameConflict {
		body += "\n" + i18n.T("favorites.crossVersionCopy.nameConflict")
	}
	req := msg.Request
	app.pushModal(newConfirmModal(
		"fav-cross-preview",
		i18n.T("favorites.actions.copyAcrossVersions"),
		body,
		[]choiceOption{
			{Label: i18n.T("favorites.actions.apply"), Value: "confirm", Primary: true},
			{Label: i18n.T("favorites.actions.cancel"), Value: "cancel"},
		},
		func(app *AppModel, value string) tea.Cmd {
			if value != "confirm" {
				return nil
			}
			return cmdApplyCrossVersionCopy(app.host, req)
		},
	))
	return nil
}

func (v *favoritesModel) itemsKey(app *AppModel, key tea.KeyMsg) tea.Cmd {
	switch key.String() {
	case "up", "k":
		if v.itemCursor > 0 {
			v.itemCursor--
			v.clampItems()
		}
	case "down", "j":
		if v.itemCursor < len(v.items)-1 {
			v.itemCursor++
			v.clampItems()
		}
	case " ":
		if v.itemCursor < len(v.items) {
			key := uistate.FavoriteKey(v.items[v.itemCursor].FavoriteMod)
			if v.itemSelected[key] {
				delete(v.itemSelected, key)
			} else {
				v.itemSelected[key] = true
			}
		}
	case "A":
		for _, item := range v.items {
			v.itemSelected[uistate.FavoriteKey(item.FavoriteMod)] = true
		}
	case "x":
		return v.removeSelected(app)
	case "C":
		return v.copyItemsDialog(app)
	}
	return nil
}

func (v *favoritesModel) clampItems() {
	const window = 8
	if v.itemCursor < v.itemOffset {
		v.itemOffset = v.itemCursor
	}
	if v.itemCursor >= v.itemOffset+window {
		v.itemOffset = v.itemCursor - window + 1
	}
	if v.itemOffset < 0 {
		v.itemOffset = 0
	}
}

func (v *favoritesModel) promptCreate(app *AppModel) tea.Cmd {
	modal := newPromptModal("fav-create", i18n.T("favorites.dialog.createTitle"), "", "", i18n.T("favorites.dialog.name"),
		func(app *AppModel, value string) tea.Cmd {
			if strings.TrimSpace(value) == "" {
				return nil
			}
			return cmdCreateFavoriteList(app.host, value, app.mc.SelectedMinecraftVersion, app.mc.SelectedModLoader)
		})
	app.pushModal(modal)
	return nil
}

func (v *favoritesModel) promptRename(app *AppModel, lists []storage.FavoriteList) tea.Cmd {
	if v.listCursor >= len(lists) {
		return nil
	}
	list := lists[v.listCursor]
	id := list.ID
	modal := newPromptModal("fav-rename", i18n.T("favorites.dialog.renameTitle"), "", list.Name, i18n.T("favorites.dialog.name"),
		func(app *AppModel, value string) tea.Cmd {
			if strings.TrimSpace(value) == "" {
				return nil
			}
			return cmdRenameFavoriteList(app.host, id, value)
		})
	app.pushModal(modal)
	return nil
}

func (v *favoritesModel) confirmDelete(app *AppModel, lists []storage.FavoriteList) tea.Cmd {
	if v.listCursor >= len(lists) {
		return nil
	}
	list := lists[v.listCursor]
	app.pushModal(newConfirmModal(
		"fav-delete",
		i18n.T("favorites.dialog.deleteTitle"),
		i18n.T("favorites.dialog.deleteBody", map[string]string{"name": list.Name}),
		[]choiceOption{
			{Label: i18n.T("favorites.actions.delete"), Value: "confirm", Destructive: true},
			{Label: i18n.T("favorites.actions.cancel"), Value: "cancel"},
		},
		func(app *AppModel, value string) tea.Cmd {
			if value != "confirm" {
				return nil
			}
			return cmdDeleteFavoriteList(app.host, list.ID)
		},
	))
	return nil
}

func (v *favoritesModel) reorder(app *AppModel, lists []storage.FavoriteList, delta int) tea.Cmd {
	if v.listCursor+delta < 0 || v.listCursor+delta >= len(lists) {
		return nil
	}
	ids := make([]string, 0, len(lists))
	for _, l := range lists {
		ids = append(ids, l.ID)
	}
	ids[v.listCursor], ids[v.listCursor+delta] = ids[v.listCursor+delta], ids[v.listCursor]
	v.listCursor += delta
	return cmdReorderFavoriteLists(app.host, ids)
}

func (v *favoritesModel) exportPackwiz(app *AppModel) tea.Cmd {
	list := v.selectedList()
	if list == nil {
		return nil
	}
	// Build the suggested filename from the core helper, then open the picker.
	defaultName := app.host.Service().FavoriteListPackwizDefaultFilename(list.ID)
	picker := newPathPickerModal("fav-export", i18n.T("favorites.actions.exportPackwiz"), pickerSaveFile, defaultName,
		func(app *AppModel, path string) tea.Cmd {
			return cmdExportPackwiz(app.host, list.ID, path, app.favorites.mcVersion, app.favorites.modLoader)
		})
	app.pushModal(picker)
	return picker.Init()
}

func (v *favoritesModel) openAsSearchResults(app *AppModel) tea.Cmd {
	list := v.selectedList()
	if list == nil {
		return nil
	}
	projects := make([]models.ModProject, 0, len(v.items))
	for _, entry := range v.items {
		projects = append(projects, models.ModProject{
			ID:         models.ProjectKey(entry.Platform, entry.ModID),
			Platform:   entry.Platform,
			ProjectID:  entry.ModID,
			Slug:       entry.Slug,
			Title:      defaultStr(entry.Title, entry.ModID),
			IconURL:    entry.IconURL,
			Categories: entry.Categories,
		})
	}
	app.search.ShowResults(projects)
	app.active = ViewDownload
	app.download.ensureInit()
	if cmd := app.refreshDownloadStates(); cmd != nil {
		return tea.Batch(cmd, app.download.Update(app, viewActivatedMsg{}))
	}
	return nil
}

func (v *favoritesModel) copyListDialog(app *AppModel) tea.Cmd {
	source := v.selectedList()
	if source == nil {
		return nil
	}
	items := v.targetListItems(app, source.ID)
	app.pushModal(newSelectModal("fav-copy-list", i18n.T("favorites.dialog.copyListTitle"), items,
		func(app *AppModel, item selectItem) tea.Cmd {
			return cmdCopyListToList(app.host, source.ID, item.Value)
		}))
	return nil
}

func (v *favoritesModel) referenceDialog(app *AppModel) tea.Cmd {
	parent := v.selectedList()
	if parent == nil {
		return nil
	}
	items := v.targetListItems(app, parent.ID)
	app.pushModal(newSelectModal("fav-reference", i18n.T("favorites.dialog.referenceTitle"), items,
		func(app *AppModel, item selectItem) tea.Cmd {
			return cmdAddListReference(app.host, parent.ID, item.Value)
		}))
	return nil
}

func (v *favoritesModel) targetListItems(app *AppModel, excludeID string) []selectItem {
	items := []selectItem{}
	for _, list := range v.visibleLists() {
		if list.ID == excludeID {
			continue
		}
		items = append(items, selectItem{Label: list.Name, Value: list.ID})
	}
	return items
}

func (v *favoritesModel) removeSelected(app *AppModel) tea.Cmd {
	list := v.selectedList()
	if list == nil {
		return nil
	}
	keys := []string{}
	for key, on := range v.itemSelected {
		if on {
			keys = append(keys, key)
		}
	}
	if len(keys) == 0 {
		if v.itemCursor < len(v.items) {
			keys = append(keys, uistate.FavoriteKey(v.items[v.itemCursor].FavoriteMod))
		} else {
			return nil
		}
	}
	cmds := []tea.Cmd{}
	for _, item := range v.items {
		if !containsStr(keys, uistate.FavoriteKey(item.FavoriteMod)) {
			continue
		}
		cmds = append(cmds, cmdRemoveFavoriteMod(app.host, item.ListID, item.Platform, item.ModID, item.MinecraftVersion, item.ModLoader))
	}
	v.itemSelected = map[string]bool{}
	return tea.Batch(cmds...)
}

func (v *favoritesModel) copyItemsDialog(app *AppModel) tea.Cmd {
	items := []selectItem{}
	for _, list := range v.visibleLists() {
		items = append(items, selectItem{Label: list.Name, Value: list.ID})
	}
	selected := []storage.FavoriteMod{}
	for _, item := range v.items {
		if v.itemSelected[uistate.FavoriteKey(item.FavoriteMod)] {
			selected = append(selected, item.FavoriteMod)
		}
	}
	if len(selected) == 0 {
		return app.toastT("favorites.empty.noItems", toastWarning)
	}
	app.pushModal(newSelectModal("fav-copy-items", i18n.T("favorites.dialog.copySelectedTitle"), items,
		func(app *AppModel, item selectItem) tea.Cmd {
			return cmdAddFavoriteMods(app.host, appcore.FavoriteBulkAddRequest{TargetListIDs: []string{item.Value}, Mods: selected})
		}))
	return nil
}

func (v *favoritesModel) ClearFocus(app *AppModel) (bool, tea.Cmd) {
	if len(v.itemSelected) > 0 {
		v.itemSelected = map[string]bool{}
		return true, nil
	}
	return false, nil
}

func (v *favoritesModel) CapturesKeys(app *AppModel) bool { return false }

func (v *favoritesModel) View(app *AppModel) string {
	v.ensureInit()
	t := app.theme
	header := t.Title().Render(i18n.T("favorites.title"))
	lists := v.visibleLists()
	leftWidth := maxInt(20, app.width/3)
	rightWidth := app.width - leftWidth - 3

	// Narrow terminals collapse to a single column with a pane toggle.
	if app.width < 90 {
		if v.pane == 0 {
			return header + "\n\n" + v.renderLists(t, lists, app.width) + "\n\n" + t.Muted().Render("⇥ "+i18n.T("favorites.sections.lists"))
		}
		return header + "\n\n" + v.renderItems(t, app.width) + "\n\n" + t.Muted().Render("⇥ "+i18n.T("favorites.sections.lists"))
	}

	leftLines := strings.Split(v.renderLists(t, lists, leftWidth), "\n")
	rightLines := strings.Split(v.renderItems(t, rightWidth), "\n")
	rows := maxInt(len(leftLines), len(rightLines))
	var grid strings.Builder
	for i := 0; i < rows; i++ {
		l, r := "", ""
		if i < len(leftLines) {
			l = leftLines[i]
		}
		if i < len(rightLines) {
			r = rightLines[i]
		}
		grid.WriteString(padRight(l, leftWidth) + " │ " + r)
		grid.WriteString("\n")
	}
	return header + "\n\n" + strings.TrimRight(grid.String(), "\n")
}

func (v *favoritesModel) renderLists(t Theme, lists []storage.FavoriteList, width int) string {
	pinned, ungrouped := uistate.PartitionLists(v.lists, v.mcVersion, v.modLoader)
	if len(pinned) == 0 && len(ungrouped) == 0 {
		return t.Muted().Render(i18n.T("favorites.empty.noLists"))
	}
	var b strings.Builder
	idx := 0
	if len(pinned) > 0 {
		b.WriteString(t.Subtitle().Render(i18n.T("favorites.sections.pinned")))
		b.WriteString("\n")
		for _, list := range pinned {
			b.WriteString(v.renderListRow(t, list, idx, width))
			b.WriteString("\n")
			idx++
		}
	}
	if len(ungrouped) > 0 {
		if len(pinned) > 0 {
			b.WriteString("\n")
		}
		b.WriteString(t.Subtitle().Render(i18n.T("favorites.sections.ungrouped")))
		b.WriteString("\n")
		for _, list := range ungrouped {
			b.WriteString(v.renderListRow(t, list, idx, width))
			b.WriteString("\n")
			idx++
		}
	}
	return strings.TrimRight(b.String(), "\n")
}

func (v *favoritesModel) renderListRow(t Theme, list storage.FavoriteList, idx, width int) string {
	marker := "  "
	if list.Pinned {
		marker = "★ "
	}
	if list.ID == v.selectedListID {
		marker = "▸ "
	}
	line := marker + truncate(list.Name, maxInt(8, width-6))
	if idx == v.listCursor && v.pane == 0 {
		return t.Selected().Render(stripStyles(line))
	}
	return t.OnSurface().Render(line)
}

func (v *favoritesModel) renderItems(t Theme, width int) string {
	if v.selectedList() == nil {
		return t.Muted().Render(i18n.T("favorites.empty.selectOrCreate"))
	}
	if len(v.items) == 0 {
		return t.Muted().Render(i18n.T("favorites.empty.noItems"))
	}
	var b strings.Builder
	const window = 8
	start := v.itemOffset
	end := start + window
	if end > len(v.items) {
		end = len(v.items)
	}
	for i := start; i < end; i++ {
		item := v.items[i]
		check := "[ ]"
		if v.itemSelected[uistate.FavoriteKey(item.FavoriteMod)] {
			check = "[✓]"
		}
		line := check + " " + truncate(defaultStr(item.Title, item.ModID), maxInt(10, width-16))
		line += "  " + t.Muted().Render(item.Platform)
		if item.Referenced {
			line += "  " + t.Accent().Render(i18n.T("favorites.references.count"))
		}
		if i == v.itemCursor && v.pane == 1 {
			line = t.Selected().Render(stripStyles(line))
		} else {
			line = t.OnSurface().Render(line)
		}
		b.WriteString(line)
		b.WriteString("\n")
	}
	return strings.TrimRight(b.String(), "\n")
}

func (v *favoritesModel) FooterHints(app *AppModel) string {
	if v.pane == 0 {
		return "⇥ panes · ↑↓ · enter " + uiText("open") + " · n " + i18n.T("favorites.actions.create") + " · R " + i18n.T("favorites.actions.rename") + " · p " + i18n.T("favorites.actions.pin") + " · i " + i18n.T("favorites.actions.customize") + " · f/F " + i18n.T("favorites.actions.referenceList") + " · x " + i18n.T("favorites.actions.delete") + " · J/K " + uiText("sort")
	}
	return "⇥ panes · ↑↓ · space " + uiText("space") + " · x " + i18n.T("favorites.actions.removeSelected") + " · C " + i18n.T("favorites.actions.copySelected")
}

func (v *favoritesModel) RowCount() int {
	if v.pane == 1 {
		return len(v.items)
	}
	return len(v.visibleLists())
}

func (v *favoritesModel) MouseWheel(app *AppModel, delta int) tea.Cmd {
	if v.pane == 1 {
		if v.itemCursor+delta >= 0 && v.itemCursor+delta < len(v.items) {
			v.itemCursor += delta
			v.clampItems()
		}
		return nil
	}
	lists := v.visibleLists()
	if v.listCursor+delta >= 0 && v.listCursor+delta < len(lists) {
		v.listCursor += delta
	}
	return nil
}

func (v *favoritesModel) ClickRow(app *AppModel, index int, ev tea.MouseMsg) tea.Cmd {
	if v.pane == 1 {
		if index >= 0 && index < len(v.items) {
			v.itemCursor = index
			if ev.Ctrl {
				key := uistate.FavoriteKey(v.items[index].FavoriteMod)
				v.itemSelected[key] = !v.itemSelected[key]
			}
		}
		return nil
	}
	if index >= 0 && index < len(v.visibleLists()) {
		v.listCursor = index
	}
	return nil
}

func containsStr(values []string, target string) bool {
	for _, v := range values {
		if v == target {
			return true
		}
	}
	return false
}
