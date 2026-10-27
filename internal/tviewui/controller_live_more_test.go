package tviewui

import (
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"

	"github.com/link-fgfgui/mod-downloader-core/appcore"
	"github.com/link-fgfgui/mod-downloader-core/models"
	"github.com/link-fgfgui/mod-downloader-core/storage"
	structs "github.com/link-fgfgui/mod-downloader-core/structs/minecraft"

	"github.com/link-fgfgui/mod-downloader-cli/internal/i18n"
	"github.com/link-fgfgui/mod-downloader-cli/internal/uistate"
)

// liveFindInputField walks a dialog's widget tree to reach the prompt input.
func liveFindInputField(p tview.Primitive) *tview.InputField {
	switch v := p.(type) {
	case *tview.InputField:
		return v
	case *centered:
		return liveFindInputField(v.content)
	case *tview.Flex:
		for i := 0; i < v.GetItemCount(); i++ {
			if found := liveFindInputField(v.GetItem(i)); found != nil {
				return found
			}
		}
	}
	return nil
}

// submitPrompt types into the topmost prompt dialog and presses Enter.
func (l *liveApp) submitPrompt(text string) {
	l.t.Helper()
	l.doUI(func() {
		d := l.app.topDialog()
		if d == nil {
			l.t.Error("submitPrompt: no dialog is open")
			return
		}
		field := liveFindInputField(d.page)
		if field == nil {
			l.t.Errorf("submitPrompt: dialog %q has no input field", d.name)
			return
		}
		field.SetText(text)
		if !d.handleKey(tcell.NewEventKey(tcell.KeyEnter, 0, tcell.ModNone)) {
			l.t.Errorf("submitPrompt: dialog %q did not handle Enter", d.name)
		}
	})
}

// chooseDialogItem activates one row of the topmost select dialog.
func (l *liveApp) chooseDialogItem(index int) {
	l.t.Helper()
	l.doUI(func() {
		d := l.app.topDialog()
		if d == nil {
			l.t.Error("chooseDialogItem: no dialog is open")
			return
		}
		list, ok := d.focus.(*wheelList)
		if !ok {
			l.t.Errorf("chooseDialogItem: dialog %q focus is %T", d.name, d.focus)
			return
		}
		if index < 0 || index >= list.GetItemCount() {
			l.t.Errorf("chooseDialogItem: index %d out of range (%d items)", index, list.GetItemCount())
			return
		}
		list.SetCurrentItem(index)
		enterPrimitive(list)
	})
}

func liveListByNameAndScope(lists []storage.FavoriteList, name, scope string) (storage.FavoriteList, bool) {
	for _, list := range lists {
		if list.Name == name && list.MinecraftVersion == scope {
			return list, true
		}
	}
	return storage.FavoriteList{}, false
}

func liveFavoritesIndex(page *favoritesPage, id string) int {
	for i, list := range page.visibleLists() {
		if list.ID == id {
			return i
		}
	}
	return -1
}

// TestLiveFavoritesDialogsAndActions drives every favorites list/item dialog
// through the real core: create, rename, copy, reference, icon, item copy,
// search hand-off, delete and the add-to-favorites "new list" prompt.
func TestLiveFavoritesDialogsAndActions(t *testing.T) {
	l := newLiveApp(t)
	svc := l.app.host.Service()

	l.doUI(func() {
		a := l.app
		a.mc.SetSelectedMinecraftVersion("1.20.1")
		a.mc.SetSelectedModLoader("Fabric")
		a.favoritesPage().SetScope("1.20.1", "Fabric")
	})
	l.waitUI(nil)

	// An empty name is ignored and the prompt closes without creating a list.
	l.doUI(func() { l.app.favoritesPage().promptCreate() })
	l.waitFor("create prompt", func(a *App) bool {
		d := a.topDialog()
		return d != nil && strings.HasPrefix(d.name, "prompt-")
	})
	l.submitPrompt("   ")
	l.waitUI(func(a *App) {
		if a.topDialog() != nil {
			t.Errorf("empty create prompt stayed open")
		}
		if _, found := liveListByNameAndScope(svc.ListFavoriteLists(), "", "1.20.1"); found {
			t.Errorf("empty name created a list")
		}
	})

	l.doUI(func() { l.app.favoritesPage().promptCreate() })
	l.waitFor("create prompt again", func(a *App) bool { return a.topDialog() != nil })
	l.submitPrompt("Prompted List")
	l.waitFor("list created from prompt", func(a *App) bool {
		_, found := liveListByNameAndScope(svc.ListFavoriteLists(), "Prompted List", "1.20.1")
		return found
	})
	sourceList, ok := liveListByNameAndScope(svc.ListFavoriteLists(), "Prompted List", "1.20.1")
	if !ok {
		t.Fatal("prompted list missing from the store")
	}
	sourceID := sourceList.ID

	l.doUI(func() {
		l.app.addFavoriteMods(appcore.FavoriteBulkAddRequest{
			TargetListIDs: []string{sourceID},
			Mods: []storage.FavoriteMod{{
				Platform: "modrinth", ModID: "sodium", Title: "Sodium",
				MinecraftVersion: "1.20.1", ModLoader: "Fabric",
			}},
		})
	})
	l.waitFor("favorite mod added", func(a *App) bool {
		return len(svc.ListFavoriteContents(sourceID).Mods) == 1
	})

	l.doUI(func() {
		a := l.app
		a.favoritesPage().selectedListID = sourceID
		a.loadFavoriteLists(sourceID)
		a.loadFavoriteContents(sourceID)
	})
	l.waitFor("favorites page loaded", func(a *App) bool {
		page := a.favoritesPage()
		return page.selectedList() != nil && len(page.items) == 1
	})

	// Rename through the prompt: empty names are rejected, valid names apply.
	l.doUI(func() {
		page := l.app.favoritesPage()
		if idx := liveFavoritesIndex(page, sourceID); idx >= 0 {
			page.left.SetCurrentItem(idx)
		}
		page.promptRename()
	})
	l.waitFor("rename prompt", func(a *App) bool { return a.topDialog() != nil })
	l.submitPrompt(" ")
	l.waitUI(func(a *App) {
		if a.topDialog() != nil {
			t.Errorf("empty rename prompt stayed open")
		}
	})
	l.doUI(func() {
		page := l.app.favoritesPage()
		if idx := liveFavoritesIndex(page, sourceID); idx >= 0 {
			page.left.SetCurrentItem(idx)
		}
		page.promptRename()
	})
	l.waitFor("rename prompt reopened", func(a *App) bool { return a.topDialog() != nil })
	l.submitPrompt("Renamed List")
	l.waitFor("list renamed", func(a *App) bool {
		list, found := findFavoriteList(svc.ListFavoriteLists(), sourceID)
		return found && list.Name == "Renamed List"
	})

	l.doUI(func() { l.app.createFavoriteList("Copy Target", "1.20.1", "Fabric") })
	l.waitFor("copy target created", func(a *App) bool {
		_, found := liveListByNameAndScope(svc.ListFavoriteLists(), "Copy Target", "1.20.1")
		return found
	})
	l.doUI(func() { l.app.createFavoriteList("Delete Me", "1.20.1", "Fabric") })
	l.waitFor("delete target created", func(a *App) bool {
		_, found := liveListByNameAndScope(svc.ListFavoriteLists(), "Delete Me", "1.20.1")
		return found
	})
	targetList, _ := liveListByNameAndScope(svc.ListFavoriteLists(), "Copy Target", "1.20.1")
	deleteList, _ := liveListByNameAndScope(svc.ListFavoriteLists(), "Delete Me", "1.20.1")

	// Reordering moves the selected list one slot down in the store.
	l.doUI(func() {
		page := l.app.favoritesPage()
		page.selectedListID = sourceID
		l.app.loadFavoriteLists(sourceID)
	})
	l.waitFor("lists reloaded for reorder", func(a *App) bool {
		return len(a.favoritesPage().visibleLists()) == 3
	})
	l.doUI(func() {
		page := l.app.favoritesPage()
		page.left.SetCurrentItem(liveFavoritesIndex(page, sourceID))
		page.reorder(1)
	})
	l.waitFor("lists reordered", func(a *App) bool {
		lists := svc.ListFavoriteLists()
		return len(lists) == 3 && lists[0].ID == targetList.ID && lists[1].ID == sourceID
	})

	// Reload contents after the reorder refreshed the directory.
	l.doUI(func() {
		a := l.app
		a.favoritesPage().selectedListID = sourceID
		a.loadFavoriteLists(sourceID)
		a.loadFavoriteContents(sourceID)
	})
	l.waitFor("source contents reloaded", func(a *App) bool { return len(a.favoritesPage().items) == 1 })

	// Copy the whole list into the target list.
	l.doUI(func() {
		page := l.app.favoritesPage()
		page.selectedListID = sourceID
		page.copyListDialog()
	})
	l.waitFor("copy list dialog", func(a *App) bool {
		d := a.topDialog()
		return d != nil && strings.HasPrefix(d.name, "select-")
	})
	copyIndex := -1
	l.waitUI(func(a *App) {
		for i, item := range a.favoritesPage().targetListItems(sourceID) {
			if item.Value == targetList.ID {
				copyIndex = i
			}
		}
	})
	l.chooseDialogItem(copyIndex)
	l.waitFor("list copied", func(a *App) bool {
		return len(svc.ListFavoriteContents(targetList.ID).Mods) == 1
	})

	// Add a reference to the target list.
	l.doUI(func() {
		page := l.app.favoritesPage()
		page.selectedListID = sourceID
		page.referenceDialog()
	})
	l.waitFor("reference dialog", func(a *App) bool {
		d := a.topDialog()
		return d != nil && strings.HasPrefix(d.name, "select-")
	})
	referenceIndex := -1
	l.waitUI(func(a *App) {
		for i, item := range a.favoritesPage().targetListItems(sourceID) {
			if item.Value == targetList.ID {
				referenceIndex = i
			}
		}
	})
	l.chooseDialogItem(referenceIndex)
	l.waitFor("reference added", func(a *App) bool {
		return len(svc.ListFavoriteListRefs(sourceID)) == 1
	})

	l.doUI(func() {
		l.app.favoritesPage().selectedListID = sourceID
		l.app.favoritesPage().manageReferences()
	})
	l.waitFor("references dialog", func(a *App) bool {
		return a.refsDialog != nil && a.topDialog() != nil
	})
	l.doUI(func() { l.app.popDialog() })
	l.waitUI(func(a *App) {
		if a.refsDialog != nil {
			t.Errorf("refs dialog was not cleared on close")
		}
	})

	// Icon choices: mdi, project (empty text skips the metadata lookup) and clear.
	l.doUI(func() {
		page := l.app.favoritesPage()
		page.selectedListID = sourceID
		page.left.SetCurrentItem(liveFavoritesIndex(page, sourceID))
		page.iconDialog()
	})
	l.waitFor("icon dialog", func(a *App) bool { return a.topDialog() != nil })
	l.chooseDialogItem(0)
	l.waitFor("mdi prompt", func(a *App) bool {
		d := a.topDialog()
		return d != nil && strings.HasPrefix(d.name, "prompt-")
	})
	l.submitPrompt("mdi-star")
	l.waitFor("mdi icon stored", func(a *App) bool {
		list, found := findFavoriteList(svc.ListFavoriteLists(), sourceID)
		return found && list.IconKind == "mdi" && list.IconValue == "mdi-star"
	})

	l.doUI(func() {
		page := l.app.favoritesPage()
		page.selectedListID = sourceID
		page.left.SetCurrentItem(liveFavoritesIndex(page, sourceID))
		page.iconDialog()
	})
	l.waitFor("project icon dialog", func(a *App) bool { return a.topDialog() != nil })
	l.chooseDialogItem(1)
	l.waitFor("project icon prompt", func(a *App) bool {
		d := a.topDialog()
		return d != nil && strings.HasPrefix(d.name, "prompt-")
	})
	l.submitPrompt("")
	l.waitFor("project icon stored", func(a *App) bool {
		list, found := findFavoriteList(svc.ListFavoriteLists(), sourceID)
		return found && list.IconKind == "project" && list.IconValue == ""
	})

	l.doUI(func() {
		page := l.app.favoritesPage()
		page.selectedListID = sourceID
		page.left.SetCurrentItem(liveFavoritesIndex(page, sourceID))
		page.iconDialog()
	})
	l.waitFor("clear icon dialog", func(a *App) bool { return a.topDialog() != nil })
	l.chooseDialogItem(2)
	l.waitFor("icon cleared", func(a *App) bool {
		list, found := findFavoriteList(svc.ListFavoriteLists(), sourceID)
		return found && list.IconKind == "" && list.IconValue == ""
	})

	// Copy the selected item into another list.
	l.doUI(func() {
		a := l.app
		a.favoritesPage().selectedListID = sourceID
		a.loadFavoriteLists(sourceID)
		a.loadFavoriteContents(sourceID)
	})
	l.waitFor("items ready for copy", func(a *App) bool { return len(a.favoritesPage().items) == 1 })
	itemCopyIndex := -1
	l.waitUI(func(a *App) {
		page := a.favoritesPage()
		for i, list := range page.visibleLists() {
			if list.ID == deleteList.ID {
				itemCopyIndex = i
			}
		}
	})
	l.doUI(func() {
		page := l.app.favoritesPage()
		page.pane = 1
		page.selected[uistate.FavoriteKey(page.items[0].FavoriteMod)] = true
		page.copyItemsDialog()
	})
	l.waitFor("copy items dialog", func(a *App) bool {
		d := a.topDialog()
		return d != nil && strings.HasPrefix(d.name, "select-")
	})
	l.chooseDialogItem(itemCopyIndex)
	l.waitFor("item copied", func(a *App) bool {
		return len(svc.ListFavoriteContents(deleteList.ID).Mods) == 1
	})

	// Hand the favorites list over to the download search results.
	l.doUI(func() {
		page := l.app.favoritesPage()
		page.selectedListID = sourceID
		l.app.loadFavoriteContents(sourceID)
	})
	l.waitFor("items ready for search", func(a *App) bool { return len(a.favoritesPage().items) == 1 })
	l.doUI(func() { l.app.favoritesPage().openAsSearchResults() })
	l.waitFor("download view with search results", func(a *App) bool {
		return a.active == ViewDownload && len(a.search.SearchResults) == 1
	})
	l.doUI(func() { l.app.switchView(ViewFavorites) })

	// Delete: cancel keeps the list, confirm removes it.
	l.doUI(func() {
		page := l.app.favoritesPage()
		page.selectedListID = deleteList.ID
		l.app.loadFavoriteLists(deleteList.ID)
	})
	l.waitFor("delete target selected", func(a *App) bool { return a.favoritesPage().selectedList() != nil })
	l.doUI(func() {
		page := l.app.favoritesPage()
		page.left.SetCurrentItem(liveFavoritesIndex(page, deleteList.ID))
		page.confirmDelete()
	})
	l.waitFor("delete confirmation", func(a *App) bool {
		d := a.topDialog()
		return d != nil && strings.HasPrefix(d.name, "confirm-")
	})
	l.doUI(func() {
		d := l.app.topDialog()
		d.handleKey(tcell.NewEventKey(tcell.KeyRight, 0, tcell.ModNone))
		d.handleKey(tcell.NewEventKey(tcell.KeyEnter, 0, tcell.ModNone))
	})
	l.waitUI(func(a *App) {
		if a.topDialog() != nil {
			t.Errorf("delete confirmation stayed open after cancel")
		}
		if _, found := findFavoriteList(svc.ListFavoriteLists(), deleteList.ID); !found {
			t.Errorf("cancel deleted the list")
		}
	})
	l.doUI(func() {
		page := l.app.favoritesPage()
		page.left.SetCurrentItem(liveFavoritesIndex(page, deleteList.ID))
		page.confirmDelete()
	})
	l.waitFor("delete confirmation again", func(a *App) bool {
		d := a.topDialog()
		return d != nil && strings.HasPrefix(d.name, "confirm-")
	})
	l.doUI(func() {
		l.app.topDialog().handleKey(tcell.NewEventKey(tcell.KeyEnter, 0, tcell.ModNone))
	})
	l.waitFor("list deleted", func(a *App) bool {
		_, found := findFavoriteList(svc.ListFavoriteLists(), deleteList.ID)
		return !found
	})

	// The add-to-favorites dialog can create a new list inline.
	l.doUI(func() {
		l.app.openAddFavoriteDialog([]favoriteDraft{{
			Platform: "modrinth", ModID: "draft-mod", Title: "Draft",
			MinecraftVersion: "1.20.1", ModLoader: "Fabric",
		}})
	})
	l.waitFor("add favorite dialog", func(a *App) bool { return a.addFavoriteDialog != nil })
	l.doUI(func() { l.app.addFavoriteDialog.promptNewList() })
	l.waitFor("new list prompt", func(a *App) bool { return a.topDialog() != nil })
	l.submitPrompt("Dialog List")
	l.waitFor("dialog list created", func(a *App) bool {
		_, found := liveListByNameAndScope(svc.ListFavoriteLists(), "Dialog List", "1.20.1")
		return found
	})
	l.doUI(func() { l.app.popDialog() })
	l.waitUI(func(a *App) {
		if a.addFavoriteDialog != nil {
			t.Errorf("add favorite dialog was not cleared on close")
		}
	})
}

// TestLiveFavoritesCrossVersionCopy resolves a favorite through a seeded cache
// snapshot so no provider request is performed, then exercises the conflict and
// name-conflict branches of the preview/apply pair.
func TestLiveFavoritesCrossVersionCopy(t *testing.T) {
	l := newLiveApp(t)
	svc := l.app.host.Service()

	if err := storage.UpsertModPlatform(models.ModProject{
		Platform: "modrinth", ProjectID: "mystery-sodium", Slug: "mystery-sodium",
		Title: "Mystery Sodium", IconURL: "https://example.invalid/sodium.png",
	}); err != nil {
		t.Fatalf("seed project: %v", err)
	}
	if err := storage.SetPlatformVersionSnapshot("modrinth", "mystery-sodium", []models.ModVersion{{
		ID: "version-1", Platform: "modrinth", ProjectID: "mystery-sodium", VersionID: "version-1",
		Name: "Sodium 1.0", Version: "1.0", ReleaseType: "release",
		GameVersions: []string{"1.21.1"}, Loaders: []string{"fabric"},
	}}, time.Now().Unix(), nil); err != nil {
		t.Fatalf("seed version snapshot: %v", err)
	}

	l.doUI(func() {
		a := l.app
		a.mc.SetSelectedMinecraftVersion("1.20.1")
		a.mc.SetSelectedModLoader("Fabric")
		a.createFavoriteList("Cross Source", "1.20.1", "Fabric")
	})
	l.waitFor("cross source created", func(a *App) bool {
		_, found := liveListByNameAndScope(svc.ListFavoriteLists(), "Cross Source", "1.20.1")
		return found
	})
	source, _ := liveListByNameAndScope(svc.ListFavoriteLists(), "Cross Source", "1.20.1")

	l.doUI(func() {
		l.app.addFavoriteMods(appcore.FavoriteBulkAddRequest{
			TargetListIDs: []string{source.ID},
			Mods: []storage.FavoriteMod{{
				Platform: "modrinth", ModID: "mystery-sodium", Slug: "mystery-sodium",
				Title: "Mystery Sodium", MinecraftVersion: "1.20.1", ModLoader: "Fabric",
			}},
		})
	})
	l.waitFor("cross source mod added", func(a *App) bool {
		return len(svc.ListFavoriteContents(source.ID).Mods) == 1
	})

	l.doUI(func() {
		a := l.app
		page := a.favoritesPage()
		page.SetScope("1.20.1", "Fabric")
		page.selectedListID = source.ID
		a.loadFavoriteLists(source.ID)
		a.loadFavoriteContents(source.ID)
		page.left.SetCurrentItem(liveFavoritesIndex(page, source.ID))
		page.crossVersionCopy()
	})
	l.waitFor("cross version prompt", func(a *App) bool {
		d := a.topDialog()
		return d != nil && strings.HasPrefix(d.name, "prompt-")
	})

	// An empty version is rejected and the dialog stays open.
	l.submitPrompt("|Fabric")
	l.waitUI(func(a *App) {
		d := a.topDialog()
		if d == nil || !strings.HasPrefix(d.name, "prompt-") {
			t.Errorf("invalid scope closed the prompt")
		}
	})

	l.submitPrompt("1.21.1|Fabric")
	l.waitFor("cross version preview", func(a *App) bool {
		d := a.topDialog()
		return d != nil && strings.HasPrefix(d.name, "confirm-")
	})
	l.doUI(func() {
		l.app.topDialog().handleKey(tcell.NewEventKey(tcell.KeyEnter, 0, tcell.ModNone))
	})
	l.waitFor("cross version success toast", func(a *App) bool {
		return strings.Contains(a.toastText, "Cross Source") && strings.Contains(a.toastText, "1")
	})
	l.waitUI(func(a *App) {
		if a.toastTimer != nil {
			a.toastTimer.Stop()
		}
	})
	l.waitFor("cross version target list", func(a *App) bool {
		target, found := liveListByNameAndScope(svc.ListFavoriteLists(), "Cross Source", "1.21.1")
		if !found {
			return false
		}
		contents := svc.ListFavoriteContents(target.ID).Mods
		if len(contents) != 1 {
			return false
		}
		return contents[0].VersionID == "version-1" && contents[0].MinecraftVersion == "1.21.1"
	})

	// A second copy sees the same name in the target scope and refuses to apply.
	l.doUI(func() {
		l.app.previewCrossVersionCopy(appcore.FavoriteCrossVersionCopyRequest{
			SourceListID: source.ID, MinecraftVersion: "1.21.1", ModLoader: "Fabric",
		})
	})
	l.waitFor("name conflict confirmation", func(a *App) bool {
		d := a.topDialog()
		return d != nil && strings.HasPrefix(d.name, "confirm-")
	})
	l.doUI(func() {
		l.app.topDialog().handleKey(tcell.NewEventKey(tcell.KeyEnter, 0, tcell.ModNone))
	})
	l.waitFor("name conflict toast", func(a *App) bool {
		return strings.Contains(a.toastText, "already exists")
	})
	l.waitUI(func(a *App) {
		if a.toastTimer != nil {
			a.toastTimer.Stop()
		}
	})

	// Unknown platforms resolve to no project: conflicts block the apply.
	l.doUI(func() {
		l.app.createFavoriteList("Conflict Source", "1.20.1", "Fabric")
	})
	l.waitFor("conflict source created", func(a *App) bool {
		_, found := liveListByNameAndScope(svc.ListFavoriteLists(), "Conflict Source", "1.20.1")
		return found
	})
	conflictList, _ := liveListByNameAndScope(svc.ListFavoriteLists(), "Conflict Source", "1.20.1")
	l.doUI(func() {
		l.app.addFavoriteMods(appcore.FavoriteBulkAddRequest{
			TargetListIDs: []string{conflictList.ID},
			Mods: []storage.FavoriteMod{{
				Platform: "unknown", ModID: "mystery-unknown", Title: "Mystery Unknown",
				MinecraftVersion: "1.20.1", ModLoader: "Fabric",
			}},
		})
	})
	l.waitFor("conflict mod added", func(a *App) bool {
		return len(svc.ListFavoriteContents(conflictList.ID).Mods) == 1
	})

	l.doUI(func() {
		l.app.applyCrossVersionCopy(appcore.FavoriteCrossVersionCopyRequest{
			SourceListID: conflictList.ID, MinecraftVersion: "1.22.1", ModLoader: "Fabric",
		})
	})
	l.waitFor("conflict not applied toast", func(a *App) bool {
		return strings.Contains(a.toastText, "conflicts must be ignored")
	})
	l.waitUI(func(a *App) {
		if a.toastTimer != nil {
			a.toastTimer.Stop()
		}
		if _, found := liveListByNameAndScope(svc.ListFavoriteLists(), "Conflict Source", "1.22.1"); found {
			t.Errorf("conflicted copy created a list")
		}
	})

	l.doUI(func() {
		l.app.applyCrossVersionCopy(appcore.FavoriteCrossVersionCopyRequest{
			SourceListID: conflictList.ID, MinecraftVersion: "1.22.1", ModLoader: "Fabric",
			IgnoreConflicts: true,
		})
	})
	l.waitFor("conflict ignored without matches", func(a *App) bool {
		return strings.Contains(a.toastText, "no matching mods")
	})
	l.waitUI(func(a *App) {
		if a.toastTimer != nil {
			a.toastTimer.Stop()
		}
	})
}

// TestLiveVersionDialogAndManageEnterRow covers the asynchronous version lookup
// (with an unknown platform so no request leaves the process) and the manage
// page's Enter semantics for the compatibility header and a mod row.
func TestLiveVersionDialogAndManageEnterRow(t *testing.T) {
	l := newLiveApp(t)

	l.doUI(func() {
		l.app.openVersionDialog(models.ModProject{
			Platform: "unknown", ProjectID: "mystery", Title: "Mystery",
		}, "pin", nil, nil)
	})
	l.waitFor("version dialog loaded", func(a *App) bool {
		return a.versionDialog != nil && !a.versionDialog.loading
	})
	l.waitUI(func(a *App) {
		if len(a.versionDialog.versions) != 0 {
			t.Errorf("unknown platform returned versions: %+v", a.versionDialog.versions)
		}
		if got := a.versionDialog.list.GetItemCount(); got != 1 {
			t.Errorf("empty version placeholder rows=%d", got)
		}
		a.popDialog()
	})
	l.waitUI(func(a *App) {
		if a.versionDialog != nil {
			t.Errorf("version dialog was not cleared on close")
		}
	})

	l.doUI(func() {
		page := l.app.managePage()
		page.expanded = false
		page.rows = []uistate.ManageRow{{Kind: uistate.RowKindIncompatibleToggle}}
		page.enterRow(0)
		if !page.expanded {
			t.Error("Enter on the compatibility header did not expand it")
		}
	})
	l.waitUI(nil)

	missingPath := filepath.Join(t.TempDir(), "missing.jar")
	l.doUI(func() {
		page := l.app.managePage()
		group := uistate.ModGroup{Primary: structs.ModInfo{
			ID: "missing", FileName: "missing.jar", Path: missingPath,
		}}
		page.rows = []uistate.ManageRow{{Kind: uistate.RowKindMod, Group: group}}
		page.selected = map[string]bool{}
		page.enterRow(0)
	})
	l.waitFor("manage row failure toast", func(a *App) bool {
		return a.toastText == i18n.T("manage.operationFailed")
	})
	l.waitUI(func(a *App) {
		if a.toastTimer != nil {
			a.toastTimer.Stop()
		}
	})

	// Out-of-range rows are ignored instead of panicking.
	l.doUI(func() { l.app.managePage().enterRow(4) })
	l.waitUI(nil)
}
