package tviewui

import (
	"strings"
	"testing"

	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"

	"github.com/link-fgfgui/mod-downloader-core/models"
	"github.com/link-fgfgui/mod-downloader-core/storage"
	appstructs "github.com/link-fgfgui/mod-downloader-core/structs"
	structs "github.com/link-fgfgui/mod-downloader-core/structs/minecraft"

	"github.com/link-fgfgui/mod-downloader-cli/internal/i18n"
	"github.com/link-fgfgui/mod-downloader-cli/internal/uistate"
)

func enterPrimitive(p tview.Primitive) {
	if handler := p.InputHandler(); handler != nil {
		handler(tcell.NewEventKey(tcell.KeyEnter, 0, tcell.ModNone), func(tview.Primitive) {})
	}
}

func hintRun(t *testing.T, hints []hint, textFragment string) {
	t.Helper()
	for _, h := range hints {
		if h.Run != nil && strings.Contains(h.Text, textFragment) {
			h.Run()
			return
		}
	}
	t.Fatalf("no clickable hint containing %q", textFragment)
}

// --- manage -----------------------------------------------------------------

func TestManageSearchFocusAndUnhandledKeys(t *testing.T) {
	h, page := managePageFixture(t)
	h.app.application.SetFocus(page.list)

	h.key(tcell.KeyTab)
	if !page.focusedSearch {
		t.Fatal("tab did not focus the search field")
	}
	if h.focus() != page.search {
		t.Fatalf("focus=%T, want search", h.focus())
	}
	if hints := page.Hints(); len(hints) != 3 {
		t.Fatalf("search hints=%d, want 3", len(hints))
	}
	// While typing, rune and function keys must fall through to the field.
	if page.HandleKey(tcell.NewEventKey(tcell.KeyRune, 'x', tcell.ModNone)) {
		t.Fatal("search mode consumed a rune")
	}
	if page.HandleKey(tcell.NewEventKey(tcell.KeyF1, 0, tcell.ModNone)) {
		t.Fatal("search mode consumed F1")
	}

	h.key(tcell.KeyBacktab)
	if page.focusedSearch {
		t.Fatal("shift+tab did not leave the search field")
	}

	h.rune('/')
	if !page.focusedSearch {
		t.Fatal("/ did not focus the search field")
	}
	if !page.ClearFocus() {
		t.Fatal("ClearFocus did not leave the search field")
	}
	if page.focusedSearch || h.focus() != page.list {
		t.Fatalf("ClearFocus left focus on %T", h.focus())
	}
	if page.ClearFocus() {
		t.Fatal("ClearFocus reported work with nothing to clear")
	}

	if page.HandleKey(tcell.NewEventKey(tcell.KeyF1, 0, tcell.ModNone)) {
		t.Fatal("page consumed an unrelated function key")
	}
	if page.HandleKey(tcell.NewEventKey(tcell.KeyRune, 'z', tcell.ModNone)) {
		t.Fatal("page consumed an unknown rune")
	}

	// Focus() honours the search flag directly.
	page.focusedSearch = true
	page.Focus()
	if h.focus() != page.search {
		t.Fatalf("Focus with focusedSearch=%T", h.focus())
	}
	page.focusedSearch = false
	page.Focus()
}

func TestManageHintsAndActionBar(t *testing.T) {
	h, page := managePageFixture(t)
	hints := page.Hints()
	if len(hints) != 8 {
		t.Fatalf("hints=%d, want 8", len(hints))
	}
	hintRun(t, hints, "/ ")
	if !page.focusedSearch {
		t.Fatal("search hint did not focus the field")
	}
	page.focusedSearch = false
	h.app.application.SetFocus(page.list)

	// Space and enter hints operate on the current row.
	page.list.SetCurrentItem(0)
	hintRun(t, hints, "space ")
	if len(page.selected) != 1 {
		t.Fatalf("space hint selected=%d", len(page.selected))
	}
	page.selected = map[string]bool{}

	// The delete hint opens a confirmation that can be cancelled.
	page.list.SetCurrentItem(0)
	h.rune(' ')
	hintRun(t, hints, "x ")
	if top := h.app.topDialog(); top == nil || !strings.HasPrefix(top.name, "confirm") {
		t.Fatalf("delete hint dialog=%v", top)
	}
	h.key(tcell.KeyEsc)

	// Scan and refresh are offline no-ops but must stay wired.
	page.selected = map[string]bool{}
	hintRun(t, hints, "S ")
	hintRun(t, hints, "r ")

	// The version hint needs exactly one selectable selection.
	page.list.SetCurrentItem(manageRowIndex(page, "sodium"))
	h.rune(' ')
	hintRun(t, hints, "v ")
	if top := h.app.topDialog(); top == nil || !strings.HasPrefix(top.name, "versions") {
		t.Fatalf("version hint dialog=%v", top)
	}
	h.key(tcell.KeyEsc)

	// The enter hint activates a row; on the non-selectable toggle row this
	// only expands the incompatible section and never queues work.
	page.selected = map[string]bool{}
	for i, row := range page.rows {
		if row.Kind == uistate.RowKindIncompatibleToggle {
			page.list.SetCurrentItem(i)
			break
		}
	}
	hintRun(t, hints, "enter ")
	if !page.expanded {
		t.Fatal("enter hint did not activate the toggle row")
	}

	// Action bar entries mirror the shortcuts.
	page.selected = map[string]bool{}
	for i, row := range page.rows {
		if row.Kind == uistate.RowKindIncompatibleToggle {
			page.list.SetCurrentItem(i)
			break
		}
	}
	for _, index := range []int{1, 2, 3, 4, 5} {
		page.action.actions[index].Run()
	}
	page.action.actions[0].Run()
	if len(page.selected) == 0 {
		t.Fatal("select-all action selected nothing")
	}
	page.selected = map[string]bool{}
}

func TestManageCopySelectionReportsClipboardFailure(t *testing.T) {
	h, page := managePageFixture(t)
	page.list.SetCurrentItem(manageRowIndex(page, "sodium"))
	h.rune(' ')
	page.selected[page.rows[manageRowIndex(page, "sodium")].RowKey()] = true

	t.Setenv("PATH", t.TempDir()) // force clipboard.WriteAll to fail deterministically
	page.copySelection(false)
	if h.app.toastText == "" {
		t.Fatal("clipboard failure produced no toast")
	}
	page.copySelection(true)
	if h.app.toastText == "" {
		t.Fatal("name copy failure produced no toast")
	}
	if h.app.toastTimer != nil {
		h.app.toastTimer.Stop()
	}
	page.selected = map[string]bool{}
	page.copySelection(false) // nothing selected: no toast
}

func TestManageListPlaceholdersAndRowDetails(t *testing.T) {
	h, page := managePageFixture(t)

	// Search with no matches renders the search-empty placeholder.
	page.search.SetText("no-such-mod")
	page.Refresh()
	h.draw()
	if !strings.Contains(h.text(), i18n.T("manage.search.empty")) {
		t.Fatalf("missing empty-search placeholder:\n%s", h.text())
	}
	page.search.SetText("")
	page.Refresh()

	// Without an instance the list shows the guidance row.
	version := h.app.mc.SelectedVersion
	h.app.mc.SelectedVersion = nil
	page.Refresh()
	h.draw()
	if !strings.Contains(h.text(), i18n.T("manage.noInstance")) {
		t.Fatalf("missing no-instance placeholder:\n%s", h.text())
	}
	h.app.mc.SelectedVersion = version
	page.Refresh()

	// Expanded incompatible rows use the expanded hint.
	page.expanded = true
	page.Refresh()
	h.draw()
	if !strings.Contains(h.text(), i18n.T("manage.incompatible.expanded")) {
		t.Fatal("expanded hint missing")
	}
	page.expanded = false
	page.Refresh()

	// Categories appear only on wide terminals.
	sodium := manageRowIndex(page, "sodium")
	main := page.rowMain(page.rows[sodium])
	if !strings.Contains(main, "optimization") {
		t.Fatalf("row main missing category: %q", main)
	}
	secondary := page.rowSecondary(page.rows[sodium])
	if !strings.Contains(secondary, "sodium.jar") {
		t.Fatalf("row secondary missing file: %q", secondary)
	}

	// rebuildListAt restores a row by key.
	target := page.rows[sodium].RowKey()
	page.rebuildListAt(target)
	if page.list.GetCurrentItem() != sodium {
		t.Fatalf("restore cursor=%d, want %d", page.list.GetCurrentItem(), sodium)
	}

	// The cursor is clamped when the previous position no longer exists.
	page.list.SetCurrentItem(len(page.rows) - 1)
	page.filter = uistate.FilterEnabled
	page.rebuildRows(false)
	page.filter = uistate.FilterAll
	page.Refresh()

	// selectedGroups falls back to the cursor row.
	page.selected = map[string]bool{}
	page.list.SetCurrentItem(sodium)
	if groups := page.selectedGroups(); len(groups) != 1 || groups[0].Primary.ID != "sodium" {
		t.Fatalf("cursor fallback groups=%+v", groups)
	}

	// Empty batches do nothing.
	page.selected = map[string]bool{}
	page.applyBatch("enable")
	page.applyBatchTo(nil, "disable")

	// An impact result without rows continues with empty pending state.
	page.pending = nil
	page.pendingAction = ""
	page.onDisableImpact(appstructs.LocalModDisableImpactResult{}, appstructs.LocalModDisableImpactRequest{})
	if h.app.topDialog() != nil {
		t.Fatal("empty impact result opened a dialog")
	}

	// A non-empty impact result opens the restore dialog.
	page.onDisableImpact(appstructs.LocalModDisableImpactResult{
		Impacts: []appstructs.LocalModDependencyImpact{{ModID: "fabric-api"}},
	}, appstructs.LocalModDisableImpactRequest{})
	if top := h.app.topDialog(); top == nil || !strings.HasPrefix(top.name, "disable-impact") {
		t.Fatalf("impact dialog=%v", top)
	}
	h.key(tcell.KeyEsc)
}

func TestManageOpenVersionDialogDownloadCallback(t *testing.T) {
	h, page := managePageFixture(t)
	page.list.SetCurrentItem(manageRowIndex(page, "sodium"))
	page.selected[page.rows[manageRowIndex(page, "sodium")].RowKey()] = true
	page.openVersionDialog()
	vd := h.app.versionDialog
	if vd == nil {
		t.Fatal("version dialog missing")
	}
	if vd.installedID != "v1" {
		t.Fatalf("installedID=%q", vd.installedID)
	}
	vd.loading = false
	vd.versions = []models.ModVersion{{ID: "new", Name: "New", Version: "1.0", ReleaseType: "release"}}
	vd.render()
	vd.activate(0)
	if h.app.topDialog() != nil {
		t.Fatalf("replace action left the dialog open: %v", h.app.topDialog())
	}
}

// --- download ----------------------------------------------------------------

func TestDownloadFocusHintsAndKeyboard(t *testing.T) {
	results := []models.ModProject{projectAt(0), projectAt(1)}
	h, page := downloadPageFixture(t, results)

	page.focusedSearch = true
	page.Focus()
	if h.focus() != page.search {
		t.Fatalf("focusedSearch Focus=%T", h.focus())
	}
	if hints := page.Hints(); len(hints) != 4 {
		t.Fatalf("search hints=%d, want 4", len(hints))
	}
	page.OnActivate()
	if h.focus() != page.search {
		t.Fatalf("OnActivate focus=%T", h.focus())
	}
	page.focusedSearch = false
	page.Focus()

	// Tab toggles search focus both ways.
	h.key(tcell.KeyTab)
	if !page.focusedSearch || h.focus() != page.search {
		t.Fatalf("tab focus=%T", h.focus())
	}
	h.key(tcell.KeyBacktab)
	if page.focusedSearch {
		t.Fatal("backtab did not leave the search field")
	}

	// "/" and "s" both focus the search field.
	for _, r := range []rune{'/', 's'} {
		h.rune(r)
		if !page.focusedSearch {
			t.Fatalf("%q did not focus search", r)
		}
		page.focusedSearch = false
		h.app.application.SetFocus(page.results)
	}

	// "l" starts an append search once more results are advertised.
	h.app.search.HasMoreResults = true
	h.rune('l')
	if !h.app.search.IsLoadingMore {
		t.Fatal("l did not start an append search")
	}
	h.app.search.IsLoadingMore = false
	page.results.SetCurrentItem(0)
	h.rune('i') // queues the current result
	if !h.app.search.DownloadingKeys[results[0].ID] {
		t.Fatalf("i did not mark the result downloading: %#v", h.app.search.DownloadingKeys)
	}
	h.rune('v')
	if top := h.app.topDialog(); top == nil || !strings.HasPrefix(top.name, "versions") {
		t.Fatalf("v dialog=%v", top)
	}
	h.key(tcell.KeyEsc)
	h.rune('r')
	if page.HandleKey(tcell.NewEventKey(tcell.KeyRune, 'z', tcell.ModNone)) {
		t.Fatal("unknown rune was consumed")
	}
	if page.HandleKey(tcell.NewEventKey(tcell.KeyF1, 0, tcell.ModNone)) {
		t.Fatal("function key was consumed")
	}
}

func TestDownloadHintsAndListStates(t *testing.T) {
	h, page := downloadPageFixture(t, []models.ModProject{projectAt(0)})

	// Default hints expose the search shortcut.
	hintRun(t, page.Hints(), "/ ")
	if !page.focusedSearch {
		t.Fatal("search hint did not focus the field")
	}
	page.focusedSearch = false
	h.app.application.SetFocus(page.results)

	// Selection hints appear once rows are ticked.
	h.rune(' ')
	if len(page.selected) != 1 {
		t.Fatal("space did not select the row")
	}
	if hints := page.Hints(); len(hints) != 5 {
		t.Fatalf("selection hints=%d, want 5", len(hints))
	}
	hintRun(t, page.Hints(), "u ")
	hintRun(t, page.Hints(), "esc ")
	if page.selected == nil {
		t.Fatal("escape hint did not clear the selection")
	}

	// No instance: the list explains how to choose a directory.
	h.app.mc.SelectedVersion = nil
	page.Refresh()
	h.draw()
	if page.results.GetItemCount() != 2 {
		t.Fatalf("no-instance rows=%d, want 2", page.results.GetItemCount())
	}
	if !strings.Contains(h.text(), i18n.T("download.selectDirTitle")) {
		t.Fatalf("missing directory guidance:\n%s", h.text())
	}
	h.app.mc.SelectedVersion = &structs.VersionInfo{ID: "inst", Name: "inst", MinecraftVersion: "1.20.1", ModLoader: "Fabric"}

	// Searching with no rows renders the loading placeholder.
	h.app.search.SearchResults = nil
	h.app.search.IsSearching = true
	page.Refresh()
	h.draw()
	if !strings.Contains(h.text(), uiText("loading")) {
		t.Fatalf("missing loading placeholder:\n%s", h.text())
	}

	// An empty completed search renders the empty placeholder.
	h.app.search.IsSearching = false
	page.Refresh()
	h.draw()
	if !strings.Contains(h.text(), i18n.T("manage.search.empty")) {
		t.Fatalf("missing empty placeholder:\n%s", h.text())
	}

	// Result rows include download counts and fall back to the project id.
	project := models.ModProject{
		ID: "modrinth:x", Platform: "modrinth", ProjectID: "x", Title: "X", Downloads: 123456,
	}
	h.app.search.SearchResults = []models.ModProject{project}
	page.Refresh()
	h.draw()
	if !strings.Contains(h.text(), compactNumber(123456)) {
		t.Fatalf("downloads missing:\n%s", h.text())
	}
	secondary := page.itemSecondary(project)
	if !strings.Contains(secondary, "x") {
		t.Fatalf("secondary=%q", secondary)
	}

	// Stale selections are dropped on refresh.
	page.selected["gone"] = true
	page.Refresh()
	if page.selected["gone"] {
		t.Fatal("stale selection survived a refresh")
	}

	// Out-of-range helpers are no-ops / fall back to a full refresh.
	page.toggleAt(99)
	page.refreshCurrentItemText(99)
	if page.results.GetItemCount() != 1 {
		t.Fatalf("refresh fallback rows=%d", page.results.GetItemCount())
	}
	page.results.SetCurrentItem(99)
	if key := page.currentKey(); key != "" && key != "modrinth:x" {
		t.Fatalf("currentKey=%q", key)
	}
	page.selected = map[string]bool{}
	if selected := page.selectedResults(); len(selected) != 0 {
		t.Fatalf("selectedResults=%+v", selected)
	}
}

func TestDownloadInstallGuardsAndCancel(t *testing.T) {
	result := projectAt(0)
	h, page := downloadPageFixture(t, []models.ModProject{result})

	// No instance: enter does nothing.
	version := h.app.mc.SelectedVersion
	h.app.mc.SelectedVersion = nil
	page.installCurrent(0)
	if len(h.app.search.DownloadingKeys) != 0 {
		t.Fatalf("install started without an instance: %#v", h.app.search.DownloadingKeys)
	}
	h.app.mc.SelectedVersion = version

	// Missing tuple: enter does nothing.
	mcVersion := h.app.search.SelectedVersion
	h.app.search.SelectedVersion = ""
	page.installCurrent(0)
	if len(h.app.search.DownloadingKeys) != 0 {
		t.Fatal("install started without a version tuple")
	}
	h.app.search.SelectedVersion = mcVersion

	// An in-flight key blocks a second install.
	h.app.search.DownloadingKeys = map[string]bool{result.ID: true}
	page.installCurrent(0)
	h.app.search.DownloadingKeys = nil

	// Cancelling the conflict dialog releases the key.
	h.app.search.DownloadStates = []appstructs.ModDownloadButtonState{{Key: result.ID, Status: "conflict"}}
	page.installCurrent(0)
	if top := h.app.topDialog(); top == nil || !strings.HasPrefix(top.name, "confirm") {
		t.Fatalf("conflict dialog=%v", top)
	}
	h.key(tcell.KeyEsc)
	if h.app.search.DownloadingKeys[result.ID] {
		t.Fatal("cancel did not release the downloading key")
	}

	// Out-of-range installs and empty version lists are ignored.
	page.installCurrent(-1)
	page.installCurrent(5)
	h.app.search.SearchResults = nil
	page.openVersions()
	if h.app.topDialog() != nil {
		t.Fatal("version dialog opened without results")
	}
	page.addToFavorites()
	if h.app.topDialog() != nil {
		t.Fatal("favorites dialog opened without results")
	}
	page.batchDownload()
	page.batchUnpin()
}

// --- favorites ---------------------------------------------------------------

func TestFavoritesItemKeysAndRenderingBranches(t *testing.T) {
	h, page := favoritesFixture(t)
	page.items = append(page.items, storage.FavoriteModEntry{
		FavoriteMod: storage.FavoriteMod{Platform: "modrinth", ModID: "lithium", Title: "Lithium"},
		Referenced:  true,
	})
	page.pane = 1
	page.renderRight()
	h.draw()
	if !strings.Contains(h.text(), i18n.T("favorites.references.count")) {
		t.Fatalf("reference marker missing:\n%s", h.text())
	}

	// Ctrl+A and "A" select every item.
	h.key(tcell.KeyCtrlA)
	if len(page.selected) != len(page.items) {
		t.Fatalf("ctrl+a selected=%d", len(page.selected))
	}
	page.selected = map[string]bool{}
	h.rune('A')
	if len(page.selected) != len(page.items) {
		t.Fatalf("A selected=%d", len(page.selected))
	}

	// Space toggles the current row.
	page.right.SetCurrentItem(0)
	h.rune(' ')
	page.right.SetCurrentItem(0)
	h.rune(' ')

	// "C" opens the copy picker and Esc cancels it.
	h.rune('C')
	if top := h.app.topDialog(); top == nil || !strings.HasPrefix(top.name, "select") {
		t.Fatalf("copy items dialog=%v", top)
	}
	h.key(tcell.KeyEsc)

	// "r" with no selected list is a no-op.
	page.selectedListID = ""
	h.rune('r')
	if page.HandleKey(tcell.NewEventKey(tcell.KeyF1, 0, tcell.ModNone)) {
		t.Fatal("function key consumed in the items pane")
	}
	if page.HandleKey(tcell.NewEventKey(tcell.KeyRune, 'z', tcell.ModNone)) {
		t.Fatal("unknown rune consumed in the items pane")
	}

	// List selection callbacks set the pane and toggle items.
	page.selectedListID = "l1"
	page.renderLeft()
	for i, list := range page.visibleLists() {
		if list.ID == "l1" {
			page.left.SetCurrentItem(i)
		}
	}
	enterPrimitive(page.left)
	if page.pane != 1 || h.focus() != page.right {
		t.Fatalf("list enter pane=%d focus=%T", page.pane, h.focus())
	}
	page.selected = map[string]bool{}
	page.right.SetCurrentItem(0)
	enterPrimitive(page.right)
	if len(page.selected) != 1 {
		t.Fatalf("right enter selected=%d idx=%d items=%d", len(page.selected), page.right.GetCurrentItem(), len(page.items))
	}
}

func TestFavoritesEmptyScopeActionsAndLayout(t *testing.T) {
	h, page := favoritesFixture(t)
	page.pane = 0
	lists := append([]storage.FavoriteList(nil), page.lists...)

	// Out-of-scope lists leave no selectable list; every list action is a no-op.
	page.lists = nil
	page.selectedListID = ""
	page.Refresh()
	page.SetScope("1.9", "Forge")
	page.left.SetCurrentItem(0)
	page.promptRename()
	if h.app.topDialog() != nil {
		t.Fatal("rename prompt opened without a list")
	}
	page.togglePin()
	page.confirmDelete()
	page.reorder(1)
	page.exportPackwiz()
	page.openAsSearchResults()
	page.copyListDialog()
	page.referenceDialog()
	page.manageReferences()
	page.iconDialog()
	page.crossVersionCopy()
	page.removeSelected()
	if h.app.topDialog() != nil {
		t.Fatal("a list action opened a dialog without a list")
	}
	if h.app.active != ViewFavorites {
		t.Fatalf("openAsSearchResults switched page without a list: %v", h.app.active)
	}
	if page.currentVisibleList() != nil {
		t.Fatal("currentVisibleList returned a list out of scope")
	}

	// Pinned list markers and narrow layouts.
	page.setLists(lists, storage.FavoriteListContents{}, "")
	page.SetScope("1.20.1", "Fabric")
	page.selectedListID = "l3"
	page.renderLeft()
	h.draw()
	if !strings.Contains(h.text(), "▸★") {
		t.Fatalf("pinned selection marker missing:\n%s", h.text())
	}
	narrow := newTestApp(t, 60, 20)
	fav := narrow.app.favoritesPage()
	fav.pane = 0
	fav.applyLayout()
	if !fav.narrow {
		t.Fatal("60-column favorites layout is not narrow")
	}
	fav.pane = 1
	fav.applyLayout()
	if !fav.narrow {
		t.Fatal("items pane reported a wide layout at 60 columns")
	}

	// targetListItems excludes the source list.
	if items := page.targetListItems("l1"); len(items) != 2 {
		t.Fatalf("target lists=%+v", items)
	}

	// setLists/setContents ignore mismatched snapshots.
	page.selectedListID = "l1"
	page.items = nil
	page.setContents("l2", storage.FavoriteListContents{Mods: []storage.FavoriteModEntry{{
		FavoriteMod: storage.FavoriteMod{Platform: "modrinth", ModID: "other", MinecraftVersion: "1.20.1", ModLoader: "Fabric"},
	}}})
	if page.items != nil {
		t.Fatal("mismatched contents were applied")
	}
	page.setLists([]storage.FavoriteList{}, storage.FavoriteListContents{}, "")
	if page.selectedListID != "" {
		t.Fatalf("empty snapshot kept list %q", page.selectedListID)
	}

	// Hints for both panes are clickable; safe actions are wired.
	hintRun(t, page.Hints(), "⇥")
	page.pane = 0
	hintRun(t, page.Hints(), "n ")
	if top := h.app.topDialog(); top == nil || !strings.HasPrefix(top.name, "prompt") {
		t.Fatalf("create hint dialog=%v", top)
	}
	h.key(tcell.KeyEsc)
}

func TestFavoritesListHintsAndDialogs(t *testing.T) {
	h, page := favoritesFixture(t)
	page.pane = 0
	page.left.SetCurrentItem(0)
	hints := page.Hints()
	if len(hints) != 7 {
		t.Fatalf("list hints=%d, want 7", len(hints))
	}
	hintRun(t, hints, "R ")
	if top := h.app.topDialog(); top == nil || !strings.HasPrefix(top.name, "prompt") {
		t.Fatalf("rename hint dialog=%v", top)
	}
	h.key(tcell.KeyEsc)
	hintRun(t, hints, "x ")
	if top := h.app.topDialog(); top == nil || !strings.HasPrefix(top.name, "confirm") {
		t.Fatalf("delete hint dialog=%v", top)
	}
	h.key(tcell.KeyEsc)
	hintRun(t, hints, "e ")
	if top := h.app.topDialog(); top == nil || !strings.HasPrefix(top.name, "path") {
		t.Fatalf("export hint dialog=%v", top)
	}
	h.key(tcell.KeyEsc)
	hintRun(t, hints, "r ")

	page.pane = 1
	itemHints := page.Hints()
	if len(itemHints) != 5 {
		t.Fatalf("item hints=%d, want 5", len(itemHints))
	}
	page.items = nil
	page.renderRight()
	hintRun(t, itemHints, "space ")
	hintRun(t, itemHints, "C ")
	if h.app.toastText != i18n.T("favorites.empty.noItems") {
		t.Fatalf("empty copy toast=%q", h.app.toastText)
	}
	if h.app.toastTimer != nil {
		h.app.toastTimer.Stop()
	}
	page.selectedListID = ""
	hintRun(t, itemHints, "r ")
}

// --- unpin -------------------------------------------------------------------

func TestUnpinBuildAndKeyboardBranches(t *testing.T) {
	h := newTestApp(t, 100, 30)
	h.switchPage(ViewUnpin)
	page := h.app.views[ViewUnpin].(*unpinPage)

	// Empty table: enter, unpin-all and row helpers are no-ops.
	page.confirmUnpinAll()
	if h.app.topDialog() != nil {
		t.Fatal("unpin-all opened a dialog without pins")
	}
	page.unpinRow(-1)
	page.unpinRow(0)
	if page.selectedKey() != "" {
		t.Fatalf("empty selectedKey=%q", page.selectedKey())
	}
	if hints := page.Hints(); len(hints) != 4 {
		t.Fatalf("hints=%d, want 4", len(hints))
	}
	if page.HandleKey(tcell.NewEventKey(tcell.KeyF1, 0, tcell.ModNone)) {
		t.Fatal("function key consumed")
	}
	if page.HandleKey(tcell.NewEventKey(tcell.KeyRune, 'z', tcell.ModNone)) {
		t.Fatal("unknown rune consumed")
	}
	h.rune('r') // offline refresh no-op
	h.rune('u') // empty unpin no-op
	h.rune('U') // empty unpin-all no-op

	pins := []storage.PinnedMod{
		{Platform: "modrinth", ModID: "sodium", VersionID: "v1", MinecraftVersion: "1.20.1", ModLoader: "Fabric"},
		{Platform: "curseforge", ModID: "jei", VersionID: "v2", MinecraftVersion: "1.20.1", ModLoader: "Fabric"},
	}
	page.setPins(pins)
	if page.selectedKey() != pinKey(pins[0]) {
		t.Fatalf("default selection=%q, want %q", page.selectedKey(), pinKey(pins[0]))
	}
	page.table.Select(1, 0)
	if page.selectedKey() != pinKey(pins[0]) {
		t.Fatalf("selectedKey=%q, want %q", page.selectedKey(), pinKey(pins[0]))
	}
	// Rebuilding restores the selection by key.
	page.setPins(pins)
	if row, _ := page.table.GetSelection(); row != 1 {
		t.Fatalf("restored row=%d, want 1", row)
	}
	// Rebuilding keeps the row when the selected pin disappears.
	page.table.Select(2, 0)
	page.setPins(pins[:1])
	if row, _ := page.table.GetSelection(); row != 2 {
		t.Fatalf("collapsed row=%d, want 2 (tview keeps the stale selection)", row)
	}
	page.table.Select(0, 0)
	page.setPins(pins)
	if row, _ := page.table.GetSelection(); row != 0 {
		t.Fatalf("default row=%d, want 0", row)
	}

	// Enter on a row unpins it; the controller is a no-op offline.
	page.table.Select(1, 0)
	enterPrimitive(page.table)
	page.unpinCurrent()

	// Unpin-all asks for confirmation; cancel keeps the rows.
	page.confirmUnpinAll()
	if top := h.app.topDialog(); top == nil || !strings.HasPrefix(top.name, "confirm") {
		t.Fatalf("unpin-all dialog=%v", top)
	}
	h.key(tcell.KeyEsc)
	if len(page.pins) != len(pins) {
		t.Fatal("cancelling unpin-all dropped pins")
	}

	// Bar actions stay wired.
	for _, action := range page.bar.actions {
		action.Run()
	}
	if top := h.app.topDialog(); top == nil || !strings.HasPrefix(top.name, "confirm") {
		t.Fatalf("unpin-all action dialog=%v", top)
	}
	h.key(tcell.KeyEsc)
}

// --- settings ----------------------------------------------------------------

func TestSettingsFormCallbacksAndFocus(t *testing.T) {
	h := newTestApp(t, 100, 40)
	h.switchPage(ViewSettings)
	page := h.app.settingsPage()
	page.Refresh()

	// ClearFocus only leaves text fields.
	page.form.SetFocus(page.languageIdx)
	if page.ClearFocus() {
		t.Fatal("ClearFocus reported work for a dropdown")
	}
	inputIndex := -1
	for i := 0; i < page.form.GetFormItemCount(); i++ {
		if _, ok := page.form.GetFormItem(i).(*tview.InputField); ok {
			inputIndex = i
			break
		}
	}
	if inputIndex < 0 {
		t.Fatal("no input field in the settings form")
	}
	page.form.SetFocus(inputIndex)
	if !page.ClearFocus() {
		t.Fatal("ClearFocus did not leave the text field")
	}
	page.form.SetFocus(page.languageIdx)

	// Hints expose the escape action.
	if hints := page.Hints(); len(hints) != 4 {
		t.Fatalf("hints=%d, want 4", len(hints))
	}
	hintRun(t, page.Hints(), "esc ")
	if h.app.active != ViewHome {
		t.Fatalf("escape hint left the active page at %v", h.app.active)
	}
	h.switchPage(ViewSettings)
	page.Refresh()

	// Dropdown callbacks are muted while refreshing and fire when edited.
	page.refreshing = true
	page.language.SetCurrentOption(1)
	page.theme.SetCurrentOption(1)
	page.release.SetCurrentOption(1)
	page.refreshing = false
	page.language.SetCurrentOption(1)
	page.theme.SetCurrentOption(1)
	page.release.SetCurrentOption(2)
	page.language.SetCurrentOption(4) // out of range: ignored
	page.theme.SetCurrentOption(9)
	page.release.SetCurrentOption(-1)

	// Checkbox callbacks tolerate no-op saves offline.
	page.simpleMode.SetChecked(!page.simpleMode.IsChecked())
	page.cleanup.SetChecked(!page.cleanup.IsChecked())
	page.mcim.SetChecked(!page.mcim.IsChecked())
	page.adaptive.SetChecked(!page.adaptive.IsChecked())
	page.verifySHA1.SetChecked(!page.verifySHA1.IsChecked())

	// Input fields save on Enter/Tab/Backtab and ignore other keys.
	enter := func(p tview.Primitive) {
		p.InputHandler()(tcell.NewEventKey(tcell.KeyEnter, 0, tcell.ModNone), func(tview.Primitive) {})
	}
	enter(page.cacheDir)
	enter(page.fileConc)
	enter(page.concurrent)
	enter(page.rps)
	enter(page.targetRate)
	page.refreshing = true
	enter(page.cacheDir)
	page.refreshing = false

	// Integer/float acceptance filters.
	page.fileConc.SetText("")
	page.fileConc.InputHandler()(tcell.NewEventKey(tcell.KeyRune, 'a', tcell.ModNone), func(tview.Primitive) {})
	if page.fileConc.GetText() != "" {
		t.Fatalf("int field accepted a letter: %q", page.fileConc.GetText())
	}
	page.fileConc.InputHandler()(tcell.NewEventKey(tcell.KeyRune, '7', tcell.ModNone), func(tview.Primitive) {})
	if page.fileConc.GetText() != "7" {
		t.Fatalf("int field rejected a digit: %q", page.fileConc.GetText())
	}
	page.targetRate.SetText("")
	for _, r := range []rune{'1', '.', '5', '.'} {
		page.targetRate.InputHandler()(tcell.NewEventKey(tcell.KeyRune, r, tcell.ModNone), func(tview.Primitive) {})
	}
	if page.targetRate.GetText() != "1.5" {
		t.Fatalf("float field text=%q", page.targetRate.GetText())
	}

	// saveNetwork is muted while refreshing.
	page.refreshing = true
	page.saveNetwork()
	page.refreshing = false
	page.saveNetwork()
}

func TestSettingsButtonsOpenTheirDialogs(t *testing.T) {
	h := newTestApp(t, 100, 40)
	h.switchPage(ViewSettings)
	page := h.app.settingsPage()
	page.Refresh()

	for i := 0; i < page.form.GetButtonCount(); i++ {
		enterPrimitive(page.form.GetButton(i))
		if h.app.topDialog() != nil {
			h.key(tcell.KeyEsc)
		}
	}
	if h.app.topDialog() != nil {
		t.Fatalf("settings buttons left a dialog open: %v", h.app.topDialog())
	}

	// The API key prompts prefill their current drafts and can be cancelled.
	for _, button := range []int{4, 6} {
		enterPrimitive(page.form.GetButton(button))
		if top := h.app.topDialog(); top == nil || !strings.HasPrefix(top.name, "prompt") {
			t.Fatalf("api key prompt=%v", top)
		}
		h.key(tcell.KeyEsc)
	}
	if h.app.topDialog() != nil {
		t.Fatalf("api key prompt did not close: %v", h.app.topDialog())
	}
}

func TestSettingsRefreshRebuildsOnLanguageChange(t *testing.T) {
	h := newTestApp(t, 100, 40)
	h.switchPage(ViewSettings)
	page := h.app.settingsPage()
	page.Refresh()
	built := page.builtLang
	i18n.Apply(i18n.LanguageChinese)
	t.Cleanup(func() { i18n.Apply(i18n.LanguageEnglish) })
	page.Refresh()
	if page.builtLang == built {
		t.Fatal("language change did not rebuild the form")
	}
	h.draw()
	h.mustContain(i18n.T("settings.title"))
}

// --- home --------------------------------------------------------------------

func TestHomeActionsAndHints(t *testing.T) {
	h := newTestApp(t, 100, 30)
	page := h.app.homePage()

	if !page.HandleKey(tcell.NewEventKey(tcell.KeyRune, 'r', tcell.ModNone)) {
		t.Fatal("r was not handled")
	}
	if page.HandleKey(tcell.NewEventKey(tcell.KeyRune, 'z', tcell.ModNone)) {
		t.Fatal("unknown rune was handled")
	}
	if page.HandleKey(tcell.NewEventKey(tcell.KeyF1, 0, tcell.ModNone)) {
		t.Fatal("function key was handled")
	}

	for _, action := range page.bar.actions {
		action.Run()
		if top := h.app.topDialog(); top != nil {
			h.key(tcell.KeyEsc)
		}
	}
	if h.app.active != ViewHome {
		t.Fatalf("home actions changed the page: %v", h.app.active)
	}

	hints := page.Hints()
	if len(hints) != 4 {
		t.Fatalf("hints=%d, want 4", len(hints))
	}
	hintRun(t, hints, "r ")
	hintRun(t, hints, "t ")
	if top := h.app.topDialog(); top == nil || !strings.HasPrefix(top.name, "target") {
		t.Fatalf("target hint dialog=%v", top)
	}
	h.key(tcell.KeyEsc)
}

// --- help dialog -------------------------------------------------------------

func TestHelpDialogRendersAndCloses(t *testing.T) {
	h := newTestApp(t, 100, 30)
	h.rune('?')
	if top := h.app.topDialog(); top == nil || !strings.HasPrefix(top.name, "help") {
		t.Fatalf("help dialog=%v", top)
	}
	h.mustContain("Ctrl+C")
	h.key(tcell.KeyEsc)
	if h.app.topDialog() != nil {
		t.Fatal("help dialog did not close")
	}
}

// --- download action bar ----------------------------------------------------

func TestDownloadActionBarCallbacks(t *testing.T) {
	h, page := downloadPageFixture(t, []models.ModProject{projectAt(0)})
	h.app.mc.SelectedMinecraftVersion = "1.20.1"
	h.app.mc.SelectedModLoader = "Fabric"
	page.Refresh()

	actions := page.bar.actions
	if len(actions) != 5 {
		t.Fatalf("download actions=%d, want 5", len(actions))
	}

	// Download-all with no selection returns without starting a batch.
	actions[1].Run()

	// With an empty selection the cursor row becomes the favorite draft.
	actions[4].Run()
	if top := h.app.topDialog(); top == nil || !strings.HasPrefix(top.name, "add-favorite") {
		t.Fatalf("favorites dialog=%v", top)
	}
	h.key(tcell.KeyEsc)
	if h.app.topDialog() != nil {
		t.Fatal("favorites dialog did not close")
	}

	// Install the current row; the offline queue call is a no-op.
	actions[0].Run()

	// Select-all feeds the selection-aware favorite path.
	actions[2].Run()
	if len(page.selected) != 1 {
		t.Fatalf("select-all selected %d rows, want 1", len(page.selected))
	}
	actions[4].Run()
	if top := h.app.topDialog(); top == nil || !strings.HasPrefix(top.name, "add-favorite") {
		t.Fatalf("favorites dialog after select-all=%v", top)
	}
	h.key(tcell.KeyEsc)

	// The target picker opens and closes.
	actions[3].Run()
	if top := h.app.topDialog(); top == nil || !strings.HasPrefix(top.name, "target") {
		t.Fatalf("target dialog=%v", top)
	}
	h.key(tcell.KeyEsc)
	if h.app.topDialog() != nil {
		t.Fatal("target dialog did not close")
	}
}
