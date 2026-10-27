package tviewui

import (
	"strings"

	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"

	"github.com/link-fgfgui/mod-downloader-core/appcore"
	"github.com/link-fgfgui/mod-downloader-core/models"
	"github.com/link-fgfgui/mod-downloader-core/storage"

	"github.com/link-fgfgui/mod-downloader-cli/internal/i18n"
	"github.com/link-fgfgui/mod-downloader-cli/internal/uistate"
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

func favoriteDraftFromProject(project models.ModProject, app *App) favoriteDraft {
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

func favoriteDraftFromGroup(group uistate.ModGroup, app *App) favoriteDraft {
	primary := group.Primary
	mcVersion := primary.Version
	if app.mc.SelectedMinecraftVersion != "" {
		mcVersion = app.mc.SelectedMinecraftVersion
	}
	return favoriteDraft{
		Platform:         primary.OnlinePlatform,
		ModID:            primary.OnlineProjectID,
		MinecraftVersion: mcVersion,
		ModLoader:        app.mc.SelectedModLoader,
		Title:            uistate.DisplayModName(group),
		Slug:             primary.OnlineSlug,
		IconURL:          primary.IconURL,
		Description:      primary.Description,
		Categories:       uistate.ModCategories(group),
	}
}

// favoritesPage renders the favorites list/item two-pane view.
type favoritesPage struct {
	app  *App
	root *tview.Flex
	box  *tview.Flex
	head *tview.TextView

	left   *wheelList
	right  *wheelList
	bar    *actionBar
	barSig string

	lists          []storage.FavoriteList
	selectedListID string
	items          []storage.FavoriteModEntry
	pane           int // 0 lists, 1 items
	selected       map[string]bool
	mcVersion      string
	modLoader      string
	loaded         bool
	narrow         bool
	rebuilding     bool
}

func newFavoritesPage(a *App) *favoritesPage {
	v := &favoritesPage{app: a, selected: map[string]bool{}}

	v.head = tview.NewTextView().SetDynamicColors(true)
	v.head.SetBackgroundColor(a.theme.Surface)

	v.left = newWheelList()
	v.left.ShowSecondaryText(false)
	v.left.SetBorder(true)
	v.left.SetTitle(i18n.T("favorites.sections.lists"))
	a.theme.styleBox(v.left.Box)
	a.theme.styleList(v.left.List)
	v.left.SetChangedFunc(func(index int, _, _ string, _ rune) {
		if v.rebuilding {
			return
		}
		lists := v.visibleLists()
		if index < 0 || index >= len(lists) {
			return
		}
		if lists[index].ID == v.selectedListID {
			return
		}
		v.selectedListID = lists[index].ID
		v.items = nil
		v.selected = map[string]bool{}
		v.renderHeader()
		v.renderRight()
		go v.app.loadFavoriteContents(lists[index].ID)
	})
	v.left.SetSelectedFunc(func(index int, _, _ string, _ rune) {
		v.pane = 1
		v.applyLayout()
		v.app.application.SetFocus(v.right)
		v.app.updateStatus()
	})

	v.right = newWheelList()
	v.right.ShowSecondaryText(true)
	v.right.SetBorder(true)
	v.right.SetTitle(i18n.T("favorites.title"))
	v.right.SetUseStyleTags(true, true)
	a.theme.styleBox(v.right.Box)
	a.theme.styleList(v.right.List)
	v.right.SetSelectedFunc(func(index int, _, _ string, _ rune) {
		v.toggleItem(index)
	})
	v.right.SetRowClickHandlers(v.itemClickZones, v.toggleItem, v.selectOnlyItem)

	v.box = tview.NewFlex().SetDirection(tview.FlexColumn).
		AddItem(v.left, 0, 1, true).
		AddItem(v.right, 0, 2, false)

	v.bar = newActionBar(a)
	v.bar.SetRefocus(v.Focus)

	row := tview.NewFlex().SetDirection(tview.FlexRow).
		AddItem(v.head, 1, 0, false).
		AddItem(v.box, 0, 1, true).
		AddItem(v.bar.Primitive(), 1, 0, false)

	v.root = row
	v.renderHeader()
	v.Refresh()
	return v
}

// root holds the header + panes row.
func (v *favoritesPage) Primitive() tview.Primitive { return v.root }

func (v *favoritesPage) Focus() {
	if v.pane == 1 {
		v.app.application.SetFocus(v.right)
		return
	}
	v.app.application.SetFocus(v.left)
}

func (v *favoritesPage) OnActivate() { go v.app.loadFavoriteLists(v.selectedListID) }

func (v *favoritesPage) ClearFocus() bool {
	if len(v.selected) > 0 {
		v.selected = map[string]bool{}
		v.renderRight()
		return true
	}
	if v.pane == 1 {
		v.pane = 0
		v.applyLayout()
		v.app.application.SetFocus(v.left)
		return true
	}
	return false
}

func (v *favoritesPage) HandleKey(event *tcell.EventKey) bool {
	if event.Key() == tcell.KeyTab || event.Key() == tcell.KeyBacktab {
		v.pane = 1 - v.pane
		v.applyLayout()
		v.Focus()
		return true
	}
	if v.pane == 1 {
		return v.itemsKey(event)
	}
	return v.listsKey(event)
}

func (v *favoritesPage) listsKey(event *tcell.EventKey) bool {
	if event.Key() != tcell.KeyRune {
		return false
	}
	switch event.Rune() {
	case 'n':
		v.promptCreate()
		return true
	case 'R':
		v.promptRename()
		return true
	case 'p':
		v.togglePin()
		return true
	case 'x':
		v.confirmDelete()
		return true
	case 'J':
		v.reorder(1)
		return true
	case 'K':
		v.reorder(-1)
		return true
	case 'e':
		v.exportPackwiz()
		return true
	case 'o':
		v.openAsSearchResults()
		return true
	case 'l':
		v.copyListDialog()
		return true
	case 'f':
		v.referenceDialog()
		return true
	case 'F':
		v.manageReferences()
		return true
	case 'i':
		v.iconDialog()
		return true
	case 'V':
		v.crossVersionCopy()
		return true
	case 'r':
		go v.app.loadFavoriteLists(v.selectedListID)
		return true
	}
	return false
}

func (v *favoritesPage) itemsKey(event *tcell.EventKey) bool {
	if event.Key() == tcell.KeyCtrlA {
		for _, item := range v.items {
			v.selected[uistate.FavoriteKey(item.FavoriteMod)] = true
		}
		v.renderRight()
		return true
	}
	if event.Key() != tcell.KeyRune {
		return false
	}
	switch event.Rune() {
	case ' ':
		v.toggleItem(v.right.GetCurrentItem())
		return true
	case 'A':
		for _, item := range v.items {
			v.selected[uistate.FavoriteKey(item.FavoriteMod)] = true
		}
		v.renderRight()
		return true
	case 'x':
		v.removeSelected()
		return true
	case 'C':
		v.copyItemsDialog()
		return true
	case 'r':
		if v.selectedListID != "" {
			go v.app.loadFavoriteContents(v.selectedListID)
		}
		return true
	}
	return false
}

func (v *favoritesPage) Hints() []hint {
	panes := hintAction("⇥ "+uiText("panes"), func() {
		v.pane = 1 - v.pane
		v.applyLayout()
		v.Focus()
		v.app.updateStatus()
	})
	if v.pane == 0 {
		return []hint{
			panes,
			hintAction("enter "+uiText("open"), func() {
				v.pane = 1
				v.applyLayout()
				v.Focus()
				v.app.updateStatus()
			}),
			hintAction("n "+i18n.T("favorites.actions.create"), v.promptCreate),
			hintAction("R "+i18n.T("favorites.actions.rename"), v.promptRename),
			hintAction("x "+i18n.T("favorites.actions.delete"), v.confirmDelete),
			hintAction("e "+uiText("export"), v.exportPackwiz),
			hintAction("r "+uiText("refresh"), func() { go v.app.loadFavoriteLists(v.selectedListID) }),
		}
	}
	return []hint{
		panes,
		hintAction("space "+uiText("space"), func() { v.toggleItem(v.right.GetCurrentItem()) }),
		hintAction("x "+i18n.T("favorites.actions.removeSelected"), v.removeSelected),
		hintAction("C "+i18n.T("favorites.actions.copySelected"), v.copyItemsDialog),
		hintAction("r "+uiText("refresh"), func() { go v.app.loadFavoriteContents(v.selectedListID) }),
	}
}

func (v *favoritesPage) OnResize(_ int) {
	v.applyLayout()
	v.renderHeader()
	v.bar.Reflow()
}

func (v *favoritesPage) Restyle() {
	t := v.app.theme
	t.styleBox(v.left.Box)
	t.styleBox(v.right.Box)
	t.styleList(v.left.List)
	t.styleList(v.right.List)
	v.head.SetBackgroundColor(t.Surface)
	v.bar.Restyle()
}

// SetScope mirrors favorites.setDisplayScope.
func (v *favoritesPage) SetScope(mcVersion, modLoader string) {
	nextMC := uistate.NormalizeMinecraftVersion(mcVersion)
	nextLoader := uistate.NormalizeKeyPart(modLoader)
	if v.mcVersion == nextMC && v.modLoader == nextLoader {
		return
	}
	v.mcVersion = nextMC
	v.modLoader = nextLoader
	if v.selectedList() == nil {
		v.selectedListID = ""
		v.items = nil
		v.selected = map[string]bool{}
	}
	v.Refresh()
}

func (v *favoritesPage) visibleLists() []storage.FavoriteList {
	pinned, ungrouped := uistate.PartitionLists(v.lists, v.mcVersion, v.modLoader)
	return append(pinned, ungrouped...)
}

func (v *favoritesPage) selectedList() *storage.FavoriteList {
	for i := range v.lists {
		if v.lists[i].ID == v.selectedListID && uistate.ListMatchesScope(v.lists[i], v.mcVersion, v.modLoader) {
			return &v.lists[i]
		}
	}
	return nil
}

// setLists receives the favorites snapshot from the controller.
func (v *favoritesPage) setLists(lists []storage.FavoriteList, contents storage.FavoriteListContents, listID string) {
	v.lists = lists
	if v.selectedListID != "" && v.selectedList() == nil {
		v.selectedListID = ""
		v.items = nil
		v.selected = map[string]bool{}
	}
	if listID != "" && listID == v.selectedListID {
		v.items = uistate.FavoriteContentsForScope(contents, v.mcVersion, v.modLoader).Mods
	}
	if listID == "" && v.selectedListID == "" {
		v.items = nil
	}
	v.loaded = true
	v.Refresh()
}

func (v *favoritesPage) setContents(listID string, contents storage.FavoriteListContents) {
	if listID != v.selectedListID {
		return
	}
	v.items = uistate.FavoriteContentsForScope(contents, v.mcVersion, v.modLoader).Mods
	v.renderRight()
}

func (v *favoritesPage) Refresh() {
	v.left.SetTitle(i18n.T("favorites.sections.lists"))
	v.renderHeader()
	v.renderLeft()
	v.renderRight()
	v.applyLayout()
}

func (v *favoritesPage) renderHeader() {
	t := v.app.theme
	loader := uistate.MatchingModLoader(v.modLoader)
	scope := v.mcVersion
	if loader != "" {
		if scope != "" {
			scope += " | "
		}
		scope += loader
	}
	if scope == "" {
		scope = uiText("noInstance")
	}
	v.head.SetText(t.tagWith(t.Primary, "b") + i18n.T("favorites.title") + resetTag +
		"  " + t.tag(t.Muted) + scope + resetTag)
}

func (v *favoritesPage) renderLeft() {
	lists := v.visibleLists()
	restore := v.selectedListID
	v.rebuilding = true
	v.left.Clear()
	if len(lists) == 0 {
		v.left.AddItem(v.app.theme.tag(v.app.theme.Muted)+i18n.T("favorites.empty.noLists")+resetTag, "", 0, nil)
		v.rebuilding = false
		return
	}
	for _, list := range lists {
		marker := "  "
		if list.Pinned {
			marker = "★ "
		}
		if list.ID == v.selectedListID {
			marker = "▸ "
		}
		name := list.Name
		if list.ID == v.selectedListID && list.Pinned {
			marker = "▸★"
		}
		v.left.AddItem(marker+" "+tview.Escape(name), "", 0, nil)
	}
	for i, list := range lists {
		if list.ID == restore {
			v.left.SetCurrentItem(i)
			break
		}
	}
	v.rebuilding = false
}

func (v *favoritesPage) renderRight() {
	selected := v.selectedList()
	restore := ""
	if idx := v.right.GetCurrentItem(); idx >= 0 && idx < len(v.items) {
		restore = uistate.FavoriteKey(v.items[idx].FavoriteMod)
	}
	v.rebuilding = true
	v.right.Clear()
	if selected == nil {
		v.right.SetTitle(i18n.T("favorites.title"))
		v.right.AddItem(v.app.theme.tag(v.app.theme.Muted)+i18n.T("favorites.empty.selectOrCreate")+resetTag, "", 0, nil)
		v.rebuilding = false
		return
	}
	v.right.SetTitle(tview.Escape(selected.Name))
	if len(v.items) == 0 {
		v.right.AddItem(v.app.theme.tag(v.app.theme.Muted)+i18n.T("favorites.empty.noItems")+resetTag, "", 0, nil)
		v.rebuilding = false
		return
	}
	t := v.app.theme
	for _, item := range v.items {
		key := uistate.FavoriteKey(item.FavoriteMod)
		check := checkBoxTagged(v.selected[key])
		main := check + " " + tview.Escape(defaultString(item.Title, item.ModID))
		detail := item.Platform
		if item.Referenced {
			detail += "  " + i18n.T("favorites.references.count")
		}
		v.right.AddItem(main, t.tag(t.Muted)+detail+resetTag, 0, nil)
	}
	if restore != "" {
		for i, item := range v.items {
			if uistate.FavoriteKey(item.FavoriteMod) == restore {
				v.right.SetCurrentItem(i)
				break
			}
		}
	}
	v.rebuilding = false
}

// syncActions relabels the clickable bar for the active pane.
func (v *favoritesPage) syncActions() {
	sig := i18n.Current() + ":" + itoa(v.pane)
	if v.barSig == sig {
		return
	}
	v.barSig = sig
	if v.pane == 1 {
		v.bar.Set(
			pageAction{Label: uiText("selectAll"), Run: func() { v.selectAllItems() }},
			pageAction{Label: i18n.T("favorites.actions.removeSelected"), Run: func() { v.removeSelected() }},
			pageAction{Label: i18n.T("favorites.actions.copySelected"), Run: func() { v.copyItemsDialog() }},
		)
		return
	}
	v.bar.Set(
		pageAction{Label: i18n.T("favorites.actions.create"), Run: func() { v.promptCreate() }},
		pageAction{Label: i18n.T("favorites.actions.rename"), Run: func() { v.promptRename() }},
		pageAction{Label: i18n.T("favorites.actions.delete"), Run: func() { v.confirmDelete() }},
		pageAction{Label: uiText("export"), Run: func() { v.exportPackwiz() }},
	)
}

// selectAllItems ticks every favorite in the right pane.
func (v *favoritesPage) selectAllItems() {
	for _, item := range v.items {
		v.selected[uistate.FavoriteKey(item.FavoriteMod)] = true
	}
	v.renderRight()
}

// applyLayout adapts the two-pane split to the current width.
func (v *favoritesPage) applyLayout() {
	v.syncActions()
	width := v.app.mainWidth()
	v.narrow = width < 72
	if v.narrow {
		if v.pane == 0 {
			v.box.ResizeItem(v.left, width, 0)
			v.box.ResizeItem(v.right, 0, 0)
		} else {
			v.box.ResizeItem(v.left, 0, 0)
			v.box.ResizeItem(v.right, width, 0)
		}
		return
	}
	left := clampRange(width/3, 22, 32)
	right := maxInt(20, width-left)
	v.box.ResizeItem(v.left, left, 0)
	v.box.ResizeItem(v.right, right, 0)
}

func (v *favoritesPage) toggleItem(index int) {
	if index < 0 || index >= len(v.items) {
		return
	}
	key := uistate.FavoriteKey(v.items[index].FavoriteMod)
	if v.selected[key] {
		delete(v.selected, key)
	} else {
		v.selected[key] = true
	}
	// Update only the current row so the cursor does not jump.
	item := v.items[index]
	check := checkBoxTagged(v.selected[key])
	main := check + " " + tview.Escape(defaultString(item.Title, item.ModID))
	detail := item.Platform
	if item.Referenced {
		detail += "  " + i18n.T("favorites.references.count")
	}
	v.right.SetItemText(index, main, v.app.theme.tag(v.app.theme.Muted)+detail+resetTag)
}

// itemClickZones exposes the checkbox and title columns of one favorite row.
func (v *favoritesPage) itemClickZones(index int) listRowZones {
	if index < 0 || index >= len(v.items) {
		return listRowZones{}
	}
	title := defaultString(v.items[index].Title, v.items[index].ModID)
	start := plainWidth(checkBox(false) + " ")
	return listRowZones{
		Enabled:     true,
		CheckboxEnd: plainWidth(checkBox(false)),
		TitleStart:  start,
		TitleEnd:    start + escapedWidth(title),
	}
}

// selectOnlyItem replaces the multi-selection with this favorite, mirroring a
// click on the title rather than on the checkbox.
func (v *favoritesPage) selectOnlyItem(index int) {
	if index < 0 || index >= len(v.items) {
		return
	}
	v.selected = map[string]bool{uistate.FavoriteKey(v.items[index].FavoriteMod): true}
	v.right.SetCurrentItem(index)
	v.renderRight()
}

func (v *favoritesPage) promptCreate() {
	v.app.openPrompt(i18n.T("favorites.dialog.createTitle"), "", "", i18n.T("favorites.dialog.name"), nil,
		func(name string) {
			if name == "" {
				return
			}
			go v.app.createFavoriteList(name, v.app.mc.SelectedMinecraftVersion, v.app.mc.SelectedModLoader)
		})
}

func (v *favoritesPage) promptRename() {
	list := v.currentVisibleList()
	if list == nil {
		return
	}
	id := list.ID
	v.app.openPrompt(i18n.T("favorites.dialog.renameTitle"), "", list.Name, i18n.T("favorites.dialog.name"), nil,
		func(name string) {
			if name == "" {
				return
			}
			go v.app.renameFavoriteList(id, name)
		})
}

func (v *favoritesPage) togglePin() {
	list := v.currentVisibleList()
	if list == nil {
		return
	}
	next := *list
	next.Pinned = !next.Pinned
	go v.app.updateFavoriteList(next)
}

func (v *favoritesPage) confirmDelete() {
	list := v.currentVisibleList()
	if list == nil {
		return
	}
	id := list.ID
	v.app.openConfirm(
		i18n.T("favorites.dialog.deleteTitle"),
		i18n.T("favorites.dialog.deleteBody", map[string]string{"name": list.Name}),
		[]confirmOption{
			{Label: i18n.T("favorites.actions.delete"), Value: "confirm", Destructive: true},
			{Label: i18n.T("favorites.actions.cancel"), Value: "cancel"},
		},
		func(value string) {
			if value == "confirm" {
				go v.app.deleteFavoriteList(id)
			}
		},
	)
}

func (v *favoritesPage) reorder(delta int) {
	lists := v.visibleLists()
	idx := v.left.GetCurrentItem()
	if idx < 0 || idx >= len(lists) || idx+delta < 0 || idx+delta >= len(lists) {
		return
	}
	ids := make([]string, 0, len(lists))
	for _, list := range lists {
		ids = append(ids, list.ID)
	}
	ids[idx], ids[idx+delta] = ids[idx+delta], ids[idx]
	go v.app.reorderFavoriteLists(ids)
}

func (v *favoritesPage) exportPackwiz() {
	list := v.selectedList()
	if list == nil {
		return
	}
	defaultName := v.app.packwizDefaultFilename(list.ID)
	v.app.openPathDialog(i18n.T("favorites.actions.exportPackwiz"), true, defaultName, func(path string) {
		go v.app.exportPackwiz(list.ID, path, v.mcVersion, v.modLoader)
	})
}

func (v *favoritesPage) openAsSearchResults() {
	list := v.selectedList()
	if list == nil {
		return
	}
	projects := make([]models.ModProject, 0, len(v.items))
	for _, entry := range v.items {
		projects = append(projects, models.ModProject{
			ID:         models.ProjectKey(entry.Platform, entry.ModID),
			Platform:   entry.Platform,
			ProjectID:  entry.ModID,
			Slug:       entry.Slug,
			Title:      defaultString(entry.Title, entry.ModID),
			IconURL:    entry.IconURL,
			Categories: entry.Categories,
		})
	}
	v.app.search.ShowResults(projects)
	v.app.switchView(ViewDownload)
	v.app.refreshDownloadStates()
}

func (v *favoritesPage) copyListDialog() {
	source := v.selectedList()
	if source == nil {
		return
	}
	items := v.targetListItems(source.ID)
	v.app.openSelect(i18n.T("favorites.dialog.copyListTitle"), items, func(item selectItem) {
		go v.app.copyFavoriteList(source.ID, item.Value)
	})
}

func (v *favoritesPage) referenceDialog() {
	parent := v.selectedList()
	if parent == nil {
		return
	}
	items := v.targetListItems(parent.ID)
	v.app.openSelect(i18n.T("favorites.dialog.referenceTitle"), items, func(item selectItem) {
		go v.app.addFavoriteReference(parent.ID, item.Value)
	})
}

func (v *favoritesPage) targetListItems(excludeID string) []selectItem {
	items := []selectItem{}
	for _, list := range v.visibleLists() {
		if list.ID == excludeID {
			continue
		}
		items = append(items, selectItem{Label: list.Name, Value: list.ID})
	}
	return items
}

func (v *favoritesPage) manageReferences() {
	list := v.selectedList()
	if list == nil {
		return
	}
	v.app.openFavoriteRefsDialog(list.ID)
}

func (v *favoritesPage) iconDialog() {
	list := v.currentVisibleList()
	if list == nil {
		return
	}
	value := *list
	v.app.openSelect(i18n.T("favorites.dialog.customizeTitle"), []selectItem{
		{Label: i18n.T("favorites.dialog.mdiIcon"), Value: "mdi"},
		{Label: i18n.T("favorites.dialog.projectIcon"), Value: "project"},
		{Label: i18n.T("favorites.actions.clearIcon"), Value: "clear"},
	}, func(item selectItem) {
		switch item.Value {
		case "clear":
			go v.app.setFavoriteListIcon(value, "", "")
		case "mdi":
			v.app.openPrompt(i18n.T("favorites.dialog.mdiIcon"), "", value.IconValue, "mdi-star", nil, func(text string) {
				go v.app.setFavoriteListIcon(value, "mdi", text)
			})
		case "project":
			v.app.openPrompt(i18n.T("favorites.dialog.projectIcon"), "", value.IconValue, i18n.T("favorites.dialog.projectSlug"), nil, func(text string) {
				go v.app.setFavoriteListIcon(value, "project", text)
			})
		}
	})
}

func (v *favoritesPage) crossVersionCopy() {
	source := v.selectedList()
	if source == nil {
		return
	}
	initial := v.app.mc.SelectedMinecraftVersion + "|" + v.app.mc.SelectedModLoader
	v.app.openPrompt(
		i18n.T("favorites.dialog.crossVersionCopyTitle"),
		i18n.T("favorites.dialog.minecraftVersion")+"|"+i18n.T("favorites.dialog.modLoader"),
		initial, "1.20.1|Fabric",
		func(text string) string {
			parts := strings.SplitN(text, "|", 2)
			if strings.TrimSpace(parts[0]) == "" {
				return i18n.T("settings.minecraftDir.invalid")
			}
			return ""
		},
		func(text string) {
			parts := strings.SplitN(text, "|", 2)
			mcVersion := strings.TrimSpace(parts[0])
			modLoader := uistate.DefaultModLoader
			if len(parts) == 2 && strings.TrimSpace(parts[1]) != "" {
				modLoader = uistate.MatchingModLoader(parts[1])
			}
			go v.app.previewCrossVersionCopy(appcore.FavoriteCrossVersionCopyRequest{
				SourceListID:     source.ID,
				MinecraftVersion: mcVersion,
				ModLoader:        modLoader,
			})
		},
	)
}

func (v *favoritesPage) removeSelected() {
	list := v.selectedList()
	if list == nil {
		return
	}
	keys := []string{}
	for key, on := range v.selected {
		if on {
			keys = append(keys, key)
		}
	}
	if len(keys) == 0 {
		idx := v.right.GetCurrentItem()
		if idx < 0 || idx >= len(v.items) {
			return
		}
		keys = append(keys, uistate.FavoriteKey(v.items[idx].FavoriteMod))
	}
	entries := []storage.FavoriteModEntry{}
	for _, item := range v.items {
		if containsString(keys, uistate.FavoriteKey(item.FavoriteMod)) {
			entries = append(entries, item)
		}
	}
	v.selected = map[string]bool{}
	go v.app.removeFavoriteMods(list.ID, entries)
}

func (v *favoritesPage) copyItemsDialog() {
	selected := []storage.FavoriteMod{}
	for _, item := range v.items {
		if v.selected[uistate.FavoriteKey(item.FavoriteMod)] {
			selected = append(selected, item.FavoriteMod)
		}
	}
	if len(selected) == 0 {
		v.app.toastT("favorites.empty.noItems")
		return
	}
	items := []selectItem{}
	for _, list := range v.visibleLists() {
		items = append(items, selectItem{Label: list.Name, Value: list.ID})
	}
	v.app.openSelect(i18n.T("favorites.dialog.copySelectedTitle"), items, func(item selectItem) {
		go v.app.addFavoriteMods(appcore.FavoriteBulkAddRequest{TargetListIDs: []string{item.Value}, Mods: selected})
	})
}

func (v *favoritesPage) currentVisibleList() *storage.FavoriteList {
	lists := v.visibleLists()
	idx := v.left.GetCurrentItem()
	if idx < 0 || idx >= len(lists) {
		return nil
	}
	return &lists[idx]
}

func defaultString(value, fallback string) string {
	if value != "" {
		return value
	}
	return fallback
}

func containsString(values []string, target string) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}
