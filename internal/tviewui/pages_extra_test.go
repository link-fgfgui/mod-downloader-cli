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
	"github.com/link-fgfgui/mod-downloader-cli/internal/tuihost"
	"github.com/link-fgfgui/mod-downloader-cli/internal/uistate"
)

func downloadPageFixture(t *testing.T, results []models.ModProject) (*testApp, *downloadPage) {
	t.Helper()
	h := newTestApp(t, 100, 30)
	h.app.mc.SelectedVersion = &structs.VersionInfo{ID: "inst", Name: "inst", MinecraftVersion: "1.20.1", ModLoader: "Fabric"}
	h.app.search.SelectedVersion = "1.20.1"
	h.app.search.SelectedModLoader = "Fabric"
	h.app.search.HasSelectedInstance = true
	h.app.search.SearchResults = results
	page := h.app.downloadPage()
	page.Refresh()
	h.switchPage(ViewDownload)
	return h, page
}

func projectAt(index int) models.ModProject {
	return models.ModProject{
		ID:        "modrinth:mod-" + itoa(index),
		Platform:  "modrinth",
		ProjectID: "mod-" + itoa(index),
		Title:     "Mod " + itoa(index),
	}
}

func TestDownloadRunSearchAndAppend(t *testing.T) {
	h, page := downloadPageFixture(t, nil)
	results := make([]models.ModProject, uistate.SearchPageSize)
	for i := range results {
		results[i] = projectAt(i)
	}
	page.search.SetText("sodium")
	page.runSearch(false)
	if !h.app.search.IsSearching || h.app.search.ActiveSearchRequestID == "" {
		t.Fatalf("search state not started: %+v", h.app.search)
	}
	if h.app.search.NextSearchOffset != uistate.SearchPageSize {
		t.Fatalf("next offset=%d", h.app.search.NextSearchOffset)
	}

	requestID := h.app.search.ActiveSearchRequestID
	h.app.handleEvent(tuihost.Event{Kind: tuihost.EventSearchModsUpdated, Payload: appstructs.SearchModsUpdate{
		RequestID: requestID, Results: results,
	}})
	if h.app.search.IsSearching || len(h.app.search.SearchResults) != len(results) {
		t.Fatalf("search update not applied: %+v", h.app.search)
	}
	if !h.app.search.HasMoreResults {
		t.Fatal("full page should keep more results available")
	}

	// Append mode starts from the next offset and records the base snapshot.
	h.app.search.IsSearching = false
	h.app.search.IsLoadingMore = false
	page.runSearch(true)
	if !h.app.search.IsLoadingMore {
		t.Fatal("append search did not enter loading-more")
	}
	if len(h.app.search.AppendBaseResults) != len(results) {
		t.Fatalf("append base=%d, want %d", len(h.app.search.AppendBaseResults), len(results))
	}

	// A blocked search (no tuple) refreshes the view without starting a request.
	h.app.search.SelectedVersion = ""
	page.runSearch(false)
	if h.app.search.IsSearching {
		t.Fatal("search started without a target tuple")
	}
}

func TestDownloadInstallConfirmationAndMetadataRetry(t *testing.T) {
	result := projectAt(0)
	h, page := downloadPageFixture(t, []models.ModProject{result})
	key := result.ID

	// "update" requires confirmation and keeps the dialog open on Esc.
	h.app.search.DownloadStates = []appstructs.ModDownloadButtonState{{Key: key, Status: "update", ConflictFileName: "old.jar"}}
	page.installCurrent(0)
	if h.app.topDialog() == nil {
		t.Fatal("update status did not open the confirmation dialog")
	}
	h.draw()
	if !strings.Contains(h.text(), "old.jar") {
		t.Fatalf("conflict file missing from the dialog:\n%s", h.text())
	}
	h.key(tcell.KeyEsc)
	if h.app.search.DownloadingKeys[key] {
		t.Fatal("cancel did not clear the in-flight download key")
	}

	// Confirming queues the download and closes the dialog.
	page.installCurrent(0)
	h.key(tcell.KeyEnter)
	if h.app.topDialog() != nil {
		t.Fatal("confirm did not close the dialog")
	}

	// Metadata-loading statuses retry the metadata instead of queueing.
	h.app.search.DownloadingKeys = nil
	h.app.search.DownloadStates = []appstructs.ModDownloadButtonState{{Key: key, Status: "metadataLoading"}}
	page.installCurrent(0)
	if h.app.topDialog() != nil {
		t.Fatal("metadata retry opened a confirmation dialog")
	}
	if h.app.search.DownloadingKeys[key] {
		t.Fatal("metadata retry left the key in flight")
	}
	if h.app.toastText != i18n.T("download.errors.metadataLoading") {
		t.Fatalf("metadata toast=%q", h.app.toastText)
	}
	if h.app.toastTimer != nil {
		h.app.toastTimer.Stop()
	}

	// Without an instance nothing is queued.
	h.app.mc.SelectedVersion = nil
	h.app.search.DownloadingKeys = nil
	h.app.search.DownloadStates = nil
	page.installCurrent(0)
	if h.app.search.DownloadingKeys[key] {
		t.Fatal("download started without a selected instance")
	}
}

func TestDownloadInstallBatchAndStatuses(t *testing.T) {
	results := []models.ModProject{projectAt(0), projectAt(1)}
	h, page := downloadPageFixture(t, results)
	page.selected["modrinth:mod-0"] = true
	page.selected["modrinth:mod-1"] = true
	selected := page.selectedResults()
	if len(selected) != 2 {
		t.Fatalf("selectedResults=%d, want 2", len(selected))
	}

	// The first result is already in flight, so the batch skips it.
	h.app.search.BeginDownloading(results[0].ID)
	page.installBatch(results)
	if !h.app.search.DownloadingKeys[results[0].ID] || !h.app.search.DownloadingKeys[results[1].ID] {
		t.Fatalf("batch keys=%#v", h.app.search.DownloadingKeys)
	}

	for _, status := range []string{"installed", "update", "conflict", "incompatible", "metadataLoading", "metadataFailed", "install", ""} {
		glyph, label := downloadStateGlyph(h.app.theme, appstructs.ModDownloadButtonState{Status: status})
		if glyph == "" {
			t.Fatalf("status %q has no glyph", status)
		}
		if status != "" && label == "" {
			t.Fatalf("status %q has no label", status)
		}
	}
}

func TestDownloadQueueRowsAndItemText(t *testing.T) {
	results := []models.ModProject{projectAt(0), projectAt(1), projectAt(2)}
	h, page := downloadPageFixture(t, results)
	h.app.queueState.Queue.Items = []appstructs.DownloadQueueItem{
		{ID: "run", ProjectID: "mod-0", Status: "running", Title: "Running", TotalBytes: 10, BytesComplete: 5},
		{ID: "pending", ProjectID: "mod-1", Status: "pending", Title: "Pending"},
		{ID: "failed", ProjectID: "mod-2", Status: "failed", Title: "Failed"},
	}
	page.Refresh()
	idx := page.buildRowIndex()
	for i, result := range results {
		if _, ok := idx.queueItemFor(result); !ok {
			t.Fatalf("row %d has no queue item", i)
		}
		main := page.itemMain(idx, result, idx.keyOf(result))
		if !strings.Contains(main, result.Title) {
			t.Fatalf("row %d main=%q, want title %q", i, main, result.Title)
		}
	}
	// The running row shows a percentage.
	main := page.itemMain(idx, results[0], idx.keyOf(results[0]))
	if !strings.Contains(main, "50%") {
		t.Fatalf("running row missing progress: %q", main)
	}
	// The failed row shows the failure label.
	main = page.itemMain(idx, results[2], idx.keyOf(results[2]))
	if !strings.Contains(main, uiText("failed")) {
		t.Fatalf("failed row missing label: %q", main)
	}
}

func TestDownloadVersionsAndFavoritesDialogs(t *testing.T) {
	result := projectAt(0)
	h, page := downloadPageFixture(t, []models.ModProject{result})
	page.results.SetCurrentItem(0)
	page.openVersions()
	if top := h.app.topDialog(); top == nil || !strings.HasPrefix(top.name, "versions") {
		t.Fatalf("version dialog=%v", top)
	}
	h.key(tcell.KeyEsc)

	// Missing favorite scope reports a toast and does not open a dialog.
	h.app.mc.SelectedMinecraftVersion = ""
	page.addToFavorites()
	if h.app.topDialog() != nil {
		t.Fatal("favorites dialog opened without a scope")
	}
	if h.app.toastText != i18n.T("favorites.empty.selectOrCreate") {
		t.Fatalf("scope toast=%q", h.app.toastText)
	}
	if h.app.toastTimer != nil {
		h.app.toastTimer.Stop()
	}

	// With a scope and an existing list the picker opens.
	h.app.mc.SelectedMinecraftVersion = "1.20.1"
	h.app.mc.SelectedModLoader = "Fabric"
	favorites := h.app.favoritesPage()
	favorites.SetScope("1.20.1", "Fabric")
	favorites.setLists([]storage.FavoriteList{{ID: "l1", Name: "List", MinecraftVersion: "1.20.1", ModLoader: "Fabric"}}, storage.FavoriteListContents{}, "")
	page.addToFavorites()
	if top := h.app.topDialog(); top == nil || !strings.HasPrefix(top.name, "add-favorite") {
		t.Fatalf("add favorite dialog=%v", top)
	}
	h.key(tcell.KeyEsc)
}

func TestDownloadChooseDirAndBatchUnpin(t *testing.T) {
	h, page := downloadPageFixture(t, []models.ModProject{projectAt(0)})
	h.app.mc.SelectedVersion = nil
	page.HandleKey(tcell.NewEventKey(tcell.KeyRune, 'o', tcell.ModNone))
	if top := h.app.topDialog(); top == nil || !strings.HasPrefix(top.name, "path") {
		t.Fatalf("o did not open the path picker: %v", top)
	}
	h.key(tcell.KeyEsc)

	// Batch download/pin are controller no-ops offline.
	page.selected["modrinth:mod-0"] = true
	page.batchDownload()
	page.batchUnpin()
	h.app.search.DownloadStates = []appstructs.ModDownloadButtonState{{Key: "modrinth:mod-0", Status: "installed"}}
	page.ClearFocus()
	if len(page.selected) != 0 {
		t.Fatal("ClearFocus did not clear the selection")
	}
}

func TestDownloadCursorAppendTrigger(t *testing.T) {
	results := make([]models.ModProject, uistate.SearchPageSize)
	for i := range results {
		results[i] = projectAt(i)
	}
	h, page := downloadPageFixture(t, results)
	h.app.search.HasMoreResults = true
	h.app.search.IsSearching = false
	h.app.search.IsLoadingMore = false
	h.app.search.NextSearchOffset = uistate.SearchPageSize
	page.onCursorChanged(len(results) - 1)
	if !h.app.search.IsLoadingMore {
		t.Fatal("cursor at the last row did not trigger the append search")
	}
	// Rebuilds must not trigger another append.
	page.rebuilding = true
	h.app.search.IsLoadingMore = false
	page.onCursorChanged(len(results) - 1)
	if h.app.search.IsLoadingMore {
		t.Fatal("rebuild triggered an append search")
	}
}

// --- manage -----------------------------------------------------------------

func manageFixture() []structs.ModInfo {
	return []structs.ModInfo{
		{
			ID: "sodium", Name: "Sodium", FileName: "sodium.jar", Path: "/mods/sodium.jar",
			Enabled: true, Loaders: []string{"fabric"}, Categories: []string{"Optimization"},
			OnlinePlatform: "modrinth", OnlineProjectID: "sodium", OnlineVersionID: "v1",
			JijMods: []structs.JijModInfo{{ID: "jij", Name: "Jij"}},
		},
		{ID: "sodium-extra", Name: "Sodium Extra", FileName: "sodium.jar", Path: "/mods/sodium.jar", Enabled: true, Loaders: []string{"fabric"}},
		{ID: "lithium", Name: "Lithium", FileName: "lithium.jar", Path: "/mods/lithium.jar", Enabled: false, Loaders: []string{"fabric"}},
		{ID: "forge-only", Name: "Forge Only", FileName: "forge.jar", Path: "/mods/forge.jar", Enabled: true, Loaders: []string{"forge"}},
	}
}

func managePageFixture(t *testing.T) (*testApp, *managePage) {
	t.Helper()
	h := newTestApp(t, 120, 40)
	h.app.mc.SelectedVersion = &structs.VersionInfo{ID: "inst", Name: "inst", MinecraftVersion: "1.20.1", ModLoader: "Fabric", Mods: manageFixture()}
	h.app.mc.SelectedModLoader = "Fabric"
	h.app.search.SelectedVersion = "1.20.1"
	h.app.search.SelectedModLoader = "Fabric"
	h.switchPage(ViewManage)
	page := h.app.managePage()
	page.Refresh()
	return h, page
}

func manageRowIndex(page *managePage, modID string) int {
	for i, row := range page.rows {
		if row.Selectable() && row.Group.Primary.ID == modID {
			return i
		}
	}
	return -1
}

func TestManageRenderingFiltersAndSelection(t *testing.T) {
	h, page := managePageFixture(t)
	if len(page.groups) != 3 {
		t.Fatalf("groups=%d, want 3", len(page.groups))
	}
	if !strings.Contains(h.text(), "Sodium") || !strings.Contains(h.text(), "Lithium") {
		t.Fatalf("manage rows missing:\n%s", h.text())
	}
	if !strings.Contains(h.text(), i18n.T("manage.incompatible.title", map[string]string{"n": "1"})) {
		t.Fatal("incompatible toggle missing")
	}

	// Filter cycles all -> enabled -> disabled -> all.
	page.filter = uistate.FilterAll
	h.rune('F')
	if page.filter != uistate.FilterEnabled {
		t.Fatalf("filter=%q, want enabled", page.filter)
	}
	h.rune('F')
	if page.filter != uistate.FilterDisabled {
		t.Fatalf("filter=%q, want disabled", page.filter)
	}
	h.rune('F')
	if page.filter != uistate.FilterAll {
		t.Fatalf("filter=%q, want all", page.filter)
	}

	// Search text filters rows and clears the selection.
	page.search.SetText("lithium")
	if len(page.rows) != 1 || page.rows[0].Group.Primary.ID != "lithium" {
		t.Fatalf("filtered rows=%+v", page.rows)
	}
	page.search.SetText("")

	// Space toggles the current row; Ctrl+A ticks every selectable row.
	page.list.SetCurrentItem(0)
	h.rune(' ')
	if len(page.selected) != 1 {
		t.Fatalf("space selected=%d", len(page.selected))
	}
	h.key(tcell.KeyCtrlA)
	selectable := 0
	for _, row := range page.rows {
		if row.Selectable() {
			selectable++
		}
	}
	if len(page.selected) != selectable {
		t.Fatalf("ctrl+a selected=%d, want %d", len(page.selected), selectable)
	}
	if !page.ClearFocus() {
		t.Fatal("ClearFocus did not clear the selection")
	}
	if page.ClearFocus() {
		t.Fatal("ClearFocus reported work with nothing to clear")
	}
}

func TestManageIncompatibleToggleAndDeletePrompt(t *testing.T) {
	h, page := managePageFixture(t)
	// Collapse/expand via Enter on the toggle row.
	toggleRow := -1
	for i, row := range page.rows {
		if row.Kind == uistate.RowKindIncompatibleToggle {
			toggleRow = i
			break
		}
	}
	if toggleRow < 0 {
		t.Fatal("toggle row missing")
	}
	page.list.SetCurrentItem(toggleRow)
	h.key(tcell.KeyEnter)
	if !page.expanded {
		t.Fatal("Enter did not expand incompatible rows")
	}
	h.key(tcell.KeyEnter)
	if page.expanded {
		t.Fatal("Enter did not collapse incompatible rows")
	}

	// Delete asks for confirmation and cancel keeps the selection intact.
	page.list.SetCurrentItem(0)
	h.rune(' ')
	h.rune('x')
	if top := h.app.topDialog(); top == nil || !strings.HasPrefix(top.name, "confirm") {
		t.Fatalf("x did not open the delete confirmation: %v", top)
	}
	h.key(tcell.KeyEsc)
	if len(page.selected) != 1 {
		t.Fatalf("cancel cleared the selection: %d", len(page.selected))
	}

	// A non-selectable row (the incompatible toggle) has nothing to delete.
	page.selected = map[string]bool{}
	page.list.SetCurrentItem(toggleRow)
	page.confirmDelete()
	if h.app.topDialog() != nil {
		t.Fatal("delete dialog opened without a selectable row")
	}
}

func TestManageUnusedScanDialog(t *testing.T) {
	h, page := managePageFixture(t)
	page.onUnusedScan(appstructs.UnusedDependencyScanResult{})
	if h.app.toastText != i18n.T("manage.cleanup.none") {
		t.Fatalf("empty scan toast=%q", h.app.toastText)
	}
	if h.app.toastTimer != nil {
		h.app.toastTimer.Stop()
	}

	candidates := make([]appstructs.UnusedDependencyCandidate, 10)
	for i := range candidates {
		candidates[i] = appstructs.UnusedDependencyCandidate{
			Path:     "/mods/u" + itoa(i) + ".jar",
			FileName: "u" + itoa(i) + ".jar",
			ModIDs:   []string{"u" + itoa(i)},
		}
	}
	candidates[0].OnlineName = "Online Name"
	candidates[1].Name = "Local Name"
	page.onUnusedScan(appstructs.UnusedDependencyScanResult{Candidates: candidates})
	if top := h.app.topDialog(); top == nil || !strings.HasPrefix(top.name, "confirm") {
		t.Fatalf("scan candidates did not open a confirmation: %v", top)
	}
	h.draw()
	text := h.text()
	if !strings.Contains(text, "Online Name") || !strings.Contains(text, "u2.jar") {
		t.Fatalf("scan candidates missing names:\n%s", text)
	}
	if !strings.Contains(text, "…") {
		t.Fatal("long candidate list was not truncated")
	}
	h.key(tcell.KeyEsc)
}

func TestManageFavoriteAndVersionGuards(t *testing.T) {
	h, page := managePageFixture(t)
	h.app.mc.SelectedMinecraftVersion = "1.20.1"
	h.app.mc.SelectedModLoader = "Fabric"
	page.list.SetCurrentItem(0)
	h.rune(' ')
	// The grouped row has online metadata, so the favorites picker opens.
	favorites := h.app.favoritesPage()
	favorites.SetScope("1.20.1", "Fabric")
	favorites.setLists([]storage.FavoriteList{{ID: "l1", Name: "List", MinecraftVersion: "1.20.1", ModLoader: "Fabric"}}, storage.FavoriteListContents{}, "")
	page.addToFavorites()
	if top := h.app.topDialog(); top == nil || !strings.HasPrefix(top.name, "add-favorite") {
		t.Fatalf("favorites picker=%v", top)
	}
	h.key(tcell.KeyEsc)

	// A grouped local mod without online metadata cannot favorite.
	page.selected = map[string]bool{}
	lithium := manageRowIndex(page, "lithium")
	if lithium < 0 {
		t.Fatal("lithium row missing")
	}
	page.list.SetCurrentItem(lithium)
	h.rune(' ')
	page.addToFavorites()
	if h.app.topDialog() != nil {
		t.Fatal("favorites picker opened for a local-only mod")
	}
	if h.app.toastText != i18n.T("favorites.invalidSelection") {
		t.Fatalf("invalid selection toast=%q", h.app.toastText)
	}
	if h.app.toastTimer != nil {
		h.app.toastTimer.Stop()
	}

	// Version dialog opens for exactly one online selection.
	page.selected = map[string]bool{}
	page.list.SetCurrentItem(manageRowIndex(page, "sodium"))
	h.rune(' ')
	page.openVersionDialog()
	if top := h.app.topDialog(); top == nil || !strings.HasPrefix(top.name, "versions") {
		t.Fatalf("version dialog=%v", top)
	}
	h.key(tcell.KeyEsc)

	// Two selected groups -> version dialog stays closed.
	page.selected = map[string]bool{}
	page.list.SetCurrentItem(manageRowIndex(page, "sodium"))
	h.rune(' ')
	page.list.SetCurrentItem(manageRowIndex(page, "lithium"))
	h.rune(' ')
	page.openVersionDialog()
	if h.app.topDialog() != nil {
		t.Fatal("version dialog opened for a multi-selection")
	}
}

func TestManageRowHelpers(t *testing.T) {
	groups := uistate.BuildGroups(manageFixture())
	page := newTestApp(t, 100, 30).app.managePage()
	page.groups = groups
	page.filtered = groups
	page.app.mc.SelectedModLoader = "Fabric"
	for _, group := range groups {
		row := uistate.ManageRow{Kind: uistate.RowKindMod, Group: group}
		main := page.rowMain(row)
		if main == "" {
			t.Fatalf("empty main text for %q", group.Primary.ID)
		}
		_ = page.rowSecondary(row)
	}
	if got := filterLabel(uistate.FilterEnabled); got != i18n.T("manage.enabled") {
		t.Fatalf("filterLabel=%q", got)
	}
	if got := filterLabel(uistate.FilterDisabled); got != i18n.T("manage.disabled") {
		t.Fatalf("filterLabel=%q", got)
	}
	if got := cycleString("a", []string{"a", "b", "c"}, -1); got != "c" {
		t.Fatalf("cycleString backwards=%q", got)
	}
}

// --- favorites ---------------------------------------------------------------

func favoritesFixture(t *testing.T) (*testApp, *favoritesPage) {
	t.Helper()
	h := newTestApp(t, 120, 40)
	h.app.mc.SelectedMinecraftVersion = "1.20.1"
	h.app.mc.SelectedModLoader = "Fabric"
	page := h.app.favoritesPage()
	page.SetScope("1.20.1", "Fabric")
	page.setLists([]storage.FavoriteList{
		{ID: "l1", Name: "First", MinecraftVersion: "1.20.1", ModLoader: "Fabric", SortOrder: 1},
		{ID: "l2", Name: "Second", MinecraftVersion: "1.20.1", ModLoader: "Fabric", SortOrder: 2},
		{ID: "l3", Name: "Pinned", Pinned: true},
	}, storage.FavoriteListContents{
		Mods: []storage.FavoriteModEntry{{
			FavoriteMod: storage.FavoriteMod{
				Platform: "modrinth", ModID: "sodium", MinecraftVersion: "1.20.1", ModLoader: "Fabric", Title: "Sodium",
			},
		}},
	}, "l1")
	page.selectedListID = "l1"
	page.setContents("l1", storage.FavoriteListContents{
		Mods: []storage.FavoriteModEntry{{
			FavoriteMod: storage.FavoriteMod{
				Platform: "modrinth", ModID: "sodium", MinecraftVersion: "1.20.1", ModLoader: "Fabric", Title: "Sodium",
			},
		}},
	})
	h.switchPage(ViewFavorites)
	return h, page
}

func TestFavoritesRenderingPanesAndSelection(t *testing.T) {
	h, page := favoritesFixture(t)
	if len(page.visibleLists()) != 3 {
		t.Fatalf("visible lists=%d", len(page.visibleLists()))
	}
	if !strings.Contains(h.text(), "First") || !strings.Contains(h.text(), "Sodium") {
		t.Fatalf("favorites rows missing:\n%s", h.text())
	}

	h.key(tcell.KeyTab)
	if page.pane != 1 {
		t.Fatalf("pane=%d, want items", page.pane)
	}
	h.rune(' ')
	if len(page.selected) != 1 {
		t.Fatalf("selected=%d", len(page.selected))
	}
	h.key(tcell.KeyCtrlA)
	if len(page.selected) != len(page.items) {
		t.Fatalf("ctrl+a selected=%d", len(page.selected))
	}
	if !page.ClearFocus() {
		t.Fatal("ClearFocus did not clear the item selection")
	}
	if !page.ClearFocus() {
		t.Fatal("ClearFocus did not leave the items pane")
	}
	if page.ClearFocus() {
		t.Fatal("ClearFocus reported work with an empty list pane")
	}
}

func TestFavoritesDialogs(t *testing.T) {
	h, page := favoritesFixture(t)
	page.pane = 0
	page.left.SetCurrentItem(0)

	page.promptCreate()
	if top := h.app.topDialog(); top == nil || !strings.HasPrefix(top.name, "prompt") {
		t.Fatalf("create prompt=%v", top)
	}
	h.key(tcell.KeyEsc)

	page.promptRename()
	if top := h.app.topDialog(); top == nil || !strings.HasPrefix(top.name, "prompt") {
		t.Fatalf("rename prompt=%v", top)
	}
	h.key(tcell.KeyEsc)

	page.confirmDelete()
	if top := h.app.topDialog(); top == nil || !strings.HasPrefix(top.name, "confirm") {
		t.Fatalf("delete confirmation=%v", top)
	}
	h.key(tcell.KeyEsc)

	page.copyListDialog()
	if top := h.app.topDialog(); top == nil || !strings.HasPrefix(top.name, "select") {
		t.Fatalf("copy list dialog=%v", top)
	}
	h.key(tcell.KeyEsc)

	page.referenceDialog()
	if top := h.app.topDialog(); top == nil || !strings.HasPrefix(top.name, "select") {
		t.Fatalf("reference dialog=%v", top)
	}
	h.key(tcell.KeyEsc)

	page.iconDialog()
	if top := h.app.topDialog(); top == nil || !strings.HasPrefix(top.name, "select") {
		t.Fatalf("icon dialog=%v", top)
	}
	h.key(tcell.KeyEsc)

	page.manageReferences()
	if top := h.app.topDialog(); top == nil || !strings.HasPrefix(top.name, "refs") {
		t.Fatalf("refs dialog=%v", top)
	}
	h.key(tcell.KeyEsc)

	page.exportPackwiz()
	if top := h.app.topDialog(); top == nil || !strings.HasPrefix(top.name, "path") {
		t.Fatalf("export path dialog=%v", top)
	}
	h.key(tcell.KeyEsc)
}

func TestFavoritesCrossVersionCopyValidation(t *testing.T) {
	h, page := favoritesFixture(t)
	page.crossVersionCopy()
	if top := h.app.topDialog(); top == nil || !strings.HasPrefix(top.name, "prompt") {
		t.Fatalf("cross-version prompt=%v", top)
	}
	box := dialogFlex(t, h)
	input := box.GetItem(1).(*tview.InputField)
	errView := box.GetItem(2).(*tview.TextView)
	input.SetText(" |Fabric")
	h.key(tcell.KeyEnter)
	if !strings.Contains(errView.GetText(true), i18n.T("settings.minecraftDir.invalid")) {
		t.Fatalf("validation error=%q", errView.GetText(true))
	}
	if h.app.topDialog() == nil {
		t.Fatal("invalid cross-version input closed the prompt")
	}
	h.key(tcell.KeyEsc)
}

func TestFavoritesOpenAsSearchResults(t *testing.T) {
	h, page := favoritesFixture(t)
	page.openAsSearchResults()
	if h.app.active != ViewDownload {
		t.Fatalf("active=%v, want download", h.app.active)
	}
	if len(h.app.search.SearchResults) != 1 || h.app.search.SearchResults[0].ProjectID != "sodium" {
		t.Fatalf("search results=%+v", h.app.search.SearchResults)
	}
	if h.app.search.HasMoreResults {
		t.Fatal("snapshot results should not paginate")
	}
}

func TestFavoritesListKeysOpenTheirDialogs(t *testing.T) {
	h, page := favoritesFixture(t)
	page.pane = 0
	page.left.SetCurrentItem(0)

	cases := []struct {
		key    rune
		prefix string
	}{
		{'n', "prompt"},
		{'R', "prompt"},
		{'x', "confirm"},
		{'e', "path"},
		{'l', "select"},
		{'f', "select"},
		{'F', "refs"},
		{'i', "select"},
		{'V', "prompt"},
	}
	for _, tc := range cases {
		h.rune(tc.key)
		if top := h.app.topDialog(); top == nil || !strings.HasPrefix(top.name, tc.prefix) {
			t.Fatalf("key %q opened %v, want %s", tc.key, top, tc.prefix)
		}
		h.key(tcell.KeyEsc)
	}

	// Snapshot keys do not open dialogs but must be handled.
	h.rune('o')
	if h.app.active != ViewDownload {
		t.Fatalf("o did not open the results snapshot: %v", h.app.active)
	}
	h.switchPage(ViewFavorites)
	h.rune('r')
}

func TestFavoritesSelectAllAndCopyItems(t *testing.T) {
	h, page := favoritesFixture(t)
	page.pane = 1
	page.items = append(page.items, storage.FavoriteModEntry{FavoriteMod: storage.FavoriteMod{
		Platform: "modrinth", ModID: "lithium", MinecraftVersion: "1.20.1", ModLoader: "Fabric", Title: "Lithium",
	}})
	page.renderRight()
	page.selectAllItems()
	if len(page.selected) != len(page.items) {
		t.Fatalf("selectAllItems=%d, want %d", len(page.selected), len(page.items))
	}
	page.copyItemsDialog()
	if top := h.app.topDialog(); top == nil || !strings.HasPrefix(top.name, "select") {
		t.Fatalf("copy items dialog=%v", top)
	}
	h.key(tcell.KeyEsc)

	page.selected = map[string]bool{}
	page.copyItemsDialog()
	if h.app.topDialog() != nil {
		t.Fatal("copy items dialog opened with no selection")
	}
	if h.app.toastText != i18n.T("favorites.empty.noItems") {
		t.Fatalf("empty copy toast=%q", h.app.toastText)
	}
	if h.app.toastTimer != nil {
		h.app.toastTimer.Stop()
	}
}

func TestAddFavoriteDialogRefresh(t *testing.T) {
	h := newTestApp(t, 100, 30)
	h.app.mc.SelectedMinecraftVersion = "1.20.1"
	h.app.mc.SelectedModLoader = "Fabric"
	page := h.app.favoritesPage()
	page.SetScope("1.20.1", "Fabric")
	page.setLists([]storage.FavoriteList{{ID: "l1", Name: "First", MinecraftVersion: "1.20.1", ModLoader: "Fabric"}}, storage.FavoriteListContents{}, "")
	h.app.openAddFavoriteDialog([]favoriteDraft{{Platform: "modrinth", ModID: "sodium"}})
	v := h.app.addFavoriteDialog
	if v == nil {
		t.Fatal("add favorite dialog missing")
	}
	page.setLists([]storage.FavoriteList{
		{ID: "l1", Name: "First", MinecraftVersion: "1.20.1", ModLoader: "Fabric"},
		{ID: "l2", Name: "Second", MinecraftVersion: "1.20.1", ModLoader: "Fabric"},
	}, storage.FavoriteListContents{}, "")
	v.refresh() // nil-safe only when the dialog is gone; here it must pick up the new list
	if len(v.lists) != 2 {
		t.Fatalf("refresh lists=%d, want 2", len(v.lists))
	}
	var nilDialog *addFavoriteDialog
	nilDialog.refresh() // must not panic after close

	h.key(tcell.KeyEsc)
	v.refresh() // closed dialog is nil-safe through the app pointer check
}

func TestManageScanUnusedWithoutInstance(t *testing.T) {
	h, page := managePageFixture(t)
	h.app.mc.SelectedVersion = nil
	page.scanUnused()
	if h.app.toastText != "" {
		t.Fatalf("empty instance scan produced a toast: %q", h.app.toastText)
	}
}

func TestFavoritesDraftsAndHelpers(t *testing.T) {
	h := newTestApp(t, 100, 30)
	h.app.mc.SelectedMinecraftVersion = "1.20.1"
	h.app.mc.SelectedModLoader = "Fabric"
	project := models.ModProject{
		Platform: "modrinth", ProjectID: "sodium", Slug: "sodium", Title: "Sodium",
		IconURL: "icon", Description: "desc", Categories: []string{"Optimization"},
	}
	draft := favoriteDraftFromProject(project, h.app)
	if draft.MinecraftVersion != "1.20.1" || draft.ModLoader != "Fabric" || draft.ModID != "sodium" {
		t.Fatalf("project draft=%+v", draft)
	}
	groups := uistate.BuildGroups([]structs.ModInfo{{
		ID: "sodium", Name: "Sodium", OnlinePlatform: "modrinth", OnlineProjectID: "sodium",
		OnlineSlug: "sodium", Categories: []string{"Optimization"},
	}})
	groupDraft := favoriteDraftFromGroup(groups[0], h.app)
	if groupDraft.ModID != "sodium" || groupDraft.Platform != "modrinth" {
		t.Fatalf("group draft=%+v", groupDraft)
	}
	if defaultString("", "fallback") != "fallback" || defaultString("x", "fallback") != "x" {
		t.Fatal("defaultString wrong")
	}
	if !containsString([]string{"a", "b"}, "b") || containsString([]string{"a"}, "b") {
		t.Fatal("containsString wrong")
	}
}

func TestFavoritesScopeSwitchResetsSelection(t *testing.T) {
	h, page := favoritesFixture(t)
	page.selected["x"] = true
	page.SetScope("1.19", "Forge")
	if page.selectedListID != "" || page.items != nil || len(page.selected) != 0 {
		t.Fatalf("scope switch kept stale state: list=%q items=%d selected=%d", page.selectedListID, len(page.items), len(page.selected))
	}
	if page.selectedList() != nil {
		t.Fatal("out-of-scope list still selected")
	}
	_ = h
}

func TestFavoritesToggleItemUpdatesOnlyOneRow(t *testing.T) {
	h, page := favoritesFixture(t)
	page.pane = 1
	page.toggleItem(0)
	if len(page.selected) != 1 {
		t.Fatalf("toggle selected=%d", len(page.selected))
	}
	h.draw()
	if !strings.Contains(h.text(), "[X]") {
		t.Fatal("toggle did not render the checked box")
	}
	page.toggleItem(0)
	if len(page.selected) != 0 {
		t.Fatalf("second toggle left %d selected", len(page.selected))
	}
	page.toggleItem(9) // out of range must not panic
}

// --- unpin -------------------------------------------------------------------

func TestUnpinPageActions(t *testing.T) {
	h := newTestApp(t, 100, 30)
	page := h.app.views[ViewUnpin].(*unpinPage)
	pins := []storage.PinnedMod{
		{Platform: "modrinth", ModID: "sodium", VersionID: "v1", MinecraftVersion: "1.20.1", ModLoader: "Fabric"},
		{Platform: "curseforge", ModID: "jei", VersionID: "v2", MinecraftVersion: "1.20.1", ModLoader: "Fabric"},
	}
	page.setPins(pins)
	h.switchPage(ViewUnpin)
	if rows := page.table.GetRowCount(); rows != 3 {
		t.Fatalf("table rows=%d, want header+2", rows)
	}
	page.table.Select(1, 0)
	if got := page.selectedKey(); got != pinKey(pins[0]) {
		t.Fatalf("selectedKey=%q, want %q", got, pinKey(pins[0]))
	}
	h.rune('u') // offline no-op
	h.rune('U') // confirmation dialog
	if top := h.app.topDialog(); top == nil || !strings.HasPrefix(top.name, "confirm") {
		t.Fatalf("U did not open the confirmation: %v", top)
	}
	h.key(tcell.KeyEsc)
	h.rune('r') // offline refresh no-op

	page.setPins(nil)
	h.draw()
	if !strings.Contains(h.text(), i18n.T("unpin.empty")) {
		t.Fatal("empty pins state missing")
	}
	page.unpinCurrent() // no rows -> no-op
}

// --- settings ---------------------------------------------------------------

func settingsViewFixture() appstructs.SettingsView {
	return appstructs.SettingsView{
		Theme:                      "light",
		Language:                   "zh",
		SimpleMode:                 true,
		AutoScanUnusedDependencies: true,
		MCIMEnabled:                true,
		PreferredReleaseType:       "beta",
		FileConcurrency:            8,
		ConcurrentDownloads:        3,
		AdaptiveFileConcurrency:    true,
		TargetDownloadRateMiB:      2.5,
		VerifySHA1:                 true,
		RequestsPerSecond:          4,
		CacheDir:                   "/tmp/cache",
		CachePath:                  "/tmp/cache/mods.gob.zst",
		MinecraftDir:               "/tmp/.minecraft",
		CurseforgeKeyMask:          "****abcd",
		HasCurseforgeKey:           true,
		ModrinthKeyMask:            "****efgh",
		HasModrinthKey:             false,
	}
}

func TestSettingsRefreshProjectsSnapshot(t *testing.T) {
	h := newTestApp(t, 100, 40)
	h.app.settings = settingsViewFixture()
	page := h.app.settingsPage()
	page.Refresh()

	if idx, _ := page.language.GetCurrentOption(); idx != 1 {
		t.Fatalf("language index=%d, want zh", idx)
	}
	if idx, _ := page.theme.GetCurrentOption(); idx != 1 {
		t.Fatalf("theme index=%d, want light", idx)
	}
	if idx, _ := page.release.GetCurrentOption(); idx != 1 {
		t.Fatalf("release index=%d, want beta", idx)
	}
	if !page.simpleMode.IsChecked() || !page.cleanup.IsChecked() || !page.mcim.IsChecked() || !page.adaptive.IsChecked() || !page.verifySHA1.IsChecked() {
		t.Fatal("checkbox state did not follow the snapshot")
	}
	if got := page.cacheDir.GetText(); got != "/tmp/cache" {
		t.Fatalf("cacheDir=%q", got)
	}
	if got := page.fileConc.GetText(); got != "8" {
		t.Fatalf("fileConc=%q", got)
	}
	if got := page.concurrent.GetText(); got != "3" {
		t.Fatalf("concurrent=%q", got)
	}
	if got := page.targetRate.GetText(); got != "2.5" {
		t.Fatalf("targetRate=%q", got)
	}
	if got := page.rps.GetText(); got != "4" {
		t.Fatalf("rps=%q", got)
	}
	if got := page.texts[settingsTextCurseforge].GetText(true); !strings.Contains(got, "****abcd") {
		t.Fatalf("curseforge status=%q", got)
	}
	if got := page.texts[settingsTextModrinth].GetText(true); !strings.Contains(got, i18n.T("settings.apiKeys.curseforge.statusEmpty")) {
		t.Fatalf("modrinth empty status=%q", got)
	}
}

func TestSettingsKeyStatusClearsImmediately(t *testing.T) {
	h := newTestApp(t, 100, 40)
	h.app.settings = settingsViewFixture()
	page := h.app.settingsPage()
	page.cfKey = uistate.KeyDraft{Clear: true}
	page.mrKey = uistate.KeyDraft{Value: "new"}
	got := page.keyStatus(true, uistate.LoadSettingsDrafts(h.app.settings))
	if strings.Contains(got, "****abcd") {
		t.Fatalf("cleared curseforge key still shown: %q", got)
	}
	got = page.keyStatus(false, uistate.LoadSettingsDrafts(h.app.settings))
	if strings.Contains(got, "****efgh") || !strings.Contains(got, i18n.T("settings.apiKeys.curseforge.statusEmpty")) {
		t.Fatalf("modrinth key status=%q", got)
	}
}

func TestSettingsParseHelpers(t *testing.T) {
	if got := parseInt(" 12 ", 3); got != 12 {
		t.Fatalf("parseInt=%d", got)
	}
	if got := parseInt("x", 3); got != 3 {
		t.Fatalf("parseInt fallback=%d", got)
	}
	if got := parseFloat("1.5", 0); got != 1.5 {
		t.Fatalf("parseFloat=%v", got)
	}
	if got := parseFloat("bad", 2); got != 2 {
		t.Fatalf("parseFloat fallback=%v", got)
	}
	if got := formatFloat(2.0); got != "2" {
		t.Fatalf("formatFloat=%q", got)
	}
	if got := formatFloat(2.5); got != "2.5" {
		t.Fatalf("formatFloat=%q", got)
	}
}

func TestSettingsClearFocusAndOfflineSave(t *testing.T) {
	h := newTestApp(t, 100, 40)
	h.switchPage(ViewSettings)
	page := h.app.settingsPage()

	page.form.SetFocus(page.languageIdx)
	if page.ClearFocus() {
		t.Fatal("ClearFocus fired on a dropdown")
	}

	// Focus the cache-dir input and clear it.
	for i := 0; i < page.form.GetFormItemCount(); i++ {
		if page.form.GetFormItem(i) == page.cacheDir {
			page.form.SetFocus(i)
			break
		}
	}
	h.app.application.SetFocus(page.form)
	if !page.ClearFocus() {
		t.Fatal("ClearFocus did not leave the text input")
	}
	if item, _ := page.form.GetFocusedItemIndex(); item != page.languageIdx {
		t.Fatalf("focus index=%d, want language", item)
	}

	// Saving network settings offline is a no-op.
	before := h.app.settings
	page.fileConc.SetText("7")
	page.saveNetwork()
	if h.app.settings != before {
		t.Fatal("offline saveNetwork changed the settings snapshot")
	}
	page.OnActivate() // offline refresh no-op
	if hints := page.Hints(); len(hints) == 0 {
		t.Fatal("settings hints are empty")
	}
}

func TestSettingsLanguageSwitchRebuildsForm(t *testing.T) {
	h := newTestApp(t, 100, 40)
	h.switchPage(ViewSettings)
	page := h.app.settingsPage()
	oldLanguage := page.language
	oldDropCount := page.form.GetFormItemCount()
	if got := i18n.Current(); got != i18n.LanguageEnglish {
		t.Fatalf("starting language=%q", got)
	}
	settings := h.app.settings
	settings.Language = i18n.LanguageChinese
	h.app.applySettings(settings)
	h.app.rebuildAllText()
	if page.language == oldLanguage {
		t.Fatal("language switch reused the old dropdown")
	}
	if page.form.GetFormItemCount() != oldDropCount {
		t.Fatalf("form item count changed: %d -> %d", oldDropCount, page.form.GetFormItemCount())
	}
	i18n.Apply(i18n.LanguageEnglish)
}

// --- home -------------------------------------------------------------------

func TestHomeBuildAndActions(t *testing.T) {
	h := newTestApp(t, 120, 40)
	h.app.stats = storage.UsageStats{
		DownloadsCompleted: 12, ModsEnabled: 3, ModsDisabled: 1,
		ModsDeleted: 2, FavoritesAdded: 5, PackwizExports: 1,
	}
	page := h.app.homePage()
	for _, width := range []int{20, 60, 90, 160} {
		page.build(width)
	}
	h.switchPage(ViewHome)
	h.draw()
	text := h.text()
	if !strings.Contains(text, "12") || !strings.Contains(text, i18n.T("home.usage.downloads")) {
		t.Fatalf("home stats missing:\n%s", text)
	}
	page.HandleKey(tcell.NewEventKey(tcell.KeyRune, 'r', tcell.ModNone))
	page.OnActivate()

	cards := metricCard(h.app.theme, "↓", "1", i18n.T("home.usage.downloads"), h.app.theme.Secondary, 4)
	if len(cards) != 4 {
		t.Fatalf("metric card lines=%d, want 4", len(cards))
	}
	for _, line := range cards {
		if tviewWidth(line) > 8 {
			t.Fatalf("narrow card exceeded 8 cells: %q (%d)", line, tviewWidth(line))
		}
	}
}
