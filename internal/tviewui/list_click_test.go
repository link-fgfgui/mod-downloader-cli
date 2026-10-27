package tviewui

import (
	"reflect"
	"testing"

	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"

	"github.com/link-fgfgui/mod-downloader-core/storage"
	structs "github.com/link-fgfgui/mod-downloader-core/structs/minecraft"

	"github.com/link-fgfgui/mod-downloader-cli/internal/uistate"
)

func clickListCell(h *testApp, list *wheelList, row, line, column int) {
	h.t.Helper()
	x, y, _, _ := list.GetInnerRect()
	h.mouse(x+column, y+row*2+line, tcell.Button1, tview.MouseLeftClick)
}

func cloneSelection(selected map[string]bool) map[string]bool {
	out := make(map[string]bool, len(selected))
	for key, value := range selected {
		out[key] = value
	}
	return out
}

func TestDownloadClickZones(t *testing.T) {
	h := downloadResultsFixture(t, 5)
	page := h.app.downloadPage()
	idx := page.buildRowIndex()
	keys := make([]string, len(page.app.search.SearchResults))
	for i, result := range page.app.search.SearchResults {
		keys[i] = idx.keyOf(result)
	}
	page.selected[keys[0]] = true
	page.selected[keys[2]] = true
	page.Refresh()
	h.draw()

	zones := page.rowClickZones(1)
	// Title click selects only that row.
	clickListCell(h, page.results, 1, 0, zones.TitleStart+1)
	if want := map[string]bool{keys[1]: true}; !reflect.DeepEqual(page.selected, want) {
		t.Fatalf("title click selection=%v, want %v", page.selected, want)
	}
	if got := page.results.GetCurrentItem(); got != 1 {
		t.Fatalf("title click cursor=%d, want 1", got)
	}

	// Checkbox click adds to the multi-selection.
	clickListCell(h, page.results, 3, 0, zones.CheckboxEnd-1)
	if !page.selected[keys[1]] || !page.selected[keys[3]] || len(page.selected) != 2 {
		t.Fatalf("checkbox click selection=%v, want rows 1 and 3", page.selected)
	}

	// Description line clicks are ignored, even in the checkbox column.
	before := cloneSelection(page.selected)
	clickListCell(h, page.results, 4, 1, zones.CheckboxEnd-1)
	if !reflect.DeepEqual(page.selected, before) {
		t.Fatalf("description click changed selection: %v -> %v", before, page.selected)
	}
	if got := page.results.GetCurrentItem(); got != 3 {
		t.Fatalf("description click moved cursor to %d, want 3", got)
	}

	// The gap after the title is not part of the title either.
	clickListCell(h, page.results, 2, 0, zones.TitleEnd)
	if !reflect.DeepEqual(page.selected, before) {
		t.Fatalf("gap click changed selection: %v -> %v", before, page.selected)
	}
	if got := page.results.GetCurrentItem(); got != 3 {
		t.Fatalf("gap click moved cursor to %d, want 3", got)
	}
}

func TestDownloadTitleClickWideCharacters(t *testing.T) {
	h := downloadResultsFixture(t, 3)
	page := h.app.downloadPage()
	page.app.search.SearchResults[0].Title = "钠 Sodium"
	page.Refresh()
	h.draw()
	idx := page.buildRowIndex()
	key := idx.keyOf(page.app.search.SearchResults[0])

	zones := page.rowClickZones(0)
	if got, want := zones.TitleEnd-zones.TitleStart, escapedWidth("钠 Sodium"); got != want {
		t.Fatalf("title width=%d, want %d", got, want)
	}
	// The first cell of the wide character is inside the title.
	clickListCell(h, page.results, 0, 0, zones.TitleStart)
	if !page.selected[key] || len(page.selected) != 1 {
		t.Fatalf("wide title click selection=%v, want the first row", page.selected)
	}
	// The cell immediately after the title is not.
	page.selected = map[string]bool{}
	clickListCell(h, page.results, 0, 0, zones.TitleEnd)
	if len(page.selected) != 0 {
		t.Fatalf("click after wide title selected %v", page.selected)
	}
}

func TestDownloadClickAfterScrollingUsesOffset(t *testing.T) {
	h := downloadResultsFixture(t, 40)
	page := h.app.downloadPage()
	innerX, innerY, _, _ := page.results.GetInnerRect()
	for i := 0; i < 24; i++ {
		h.mouse(innerX+2, innerY+2, tcell.WheelDown, tview.MouseScrollDown)
	}
	h.draw()
	itemOffset, _ := page.results.GetOffset()
	if itemOffset == 0 {
		t.Skip("list did not scroll; screen is tall enough for every row")
	}
	visibleRow := 1
	index := itemOffset + visibleRow
	if index >= len(page.app.search.SearchResults) {
		t.Fatalf("visible index %d out of range", index)
	}
	zones := page.rowClickZones(index)
	x, y, _, _ := page.results.GetInnerRect()
	h.mouse(x+zones.TitleStart+1, y+visibleRow*2, tcell.Button1, tview.MouseLeftClick)

	idx := page.buildRowIndex()
	key := idx.keyOf(page.app.search.SearchResults[index])
	if !page.selected[key] || len(page.selected) != 1 {
		t.Fatalf("scrolled title click selection=%v, want only %q", page.selected, key)
	}
	if got := page.results.GetCurrentItem(); got != index {
		t.Fatalf("scrolled title click cursor=%d, want %d", got, index)
	}
}

func TestManageClickZones(t *testing.T) {
	h := newTestApp(t, 120, 40)
	h.app.mc.SelectedModLoader = "Fabric"
	h.app.mc.SelectedVersion = &structs.VersionInfo{
		ID: "inst", Name: "inst", MinecraftVersion: "1.20.1", ModLoader: "Fabric",
		Mods: []structs.ModInfo{
			{ID: "alpha", Name: "Alpha", FileName: "alpha.jar", Path: "/mods/alpha.jar", Enabled: true},
			{ID: "beta", Name: "Beta", FileName: "beta.jar", Path: "/mods/beta.jar", Enabled: true},
			{ID: "gamma", Name: "Gamma", FileName: "gamma.jar", Path: "/mods/gamma.jar", Enabled: true},
		},
	}
	h.switchPage(ViewManage)
	page := h.app.managePage()
	if len(page.rows) != 3 {
		t.Fatalf("manage rows=%d, want 3", len(page.rows))
	}
	page.selected[page.rows[0].RowKey()] = true
	page.selected[page.rows[2].RowKey()] = true
	page.rebuildList()
	h.draw()

	zones := page.rowClickZones(1)
	clickListCell(h, page.list, 1, 0, zones.TitleStart+1)
	want := map[string]bool{page.rows[1].RowKey(): true}
	if !reflect.DeepEqual(page.selected, want) {
		t.Fatalf("manage title click selection=%v, want %v", page.selected, want)
	}
	if got := page.list.GetCurrentItem(); got != 1 {
		t.Fatalf("manage title click cursor=%d, want 1", got)
	}

	clickListCell(h, page.list, 2, 0, zones.CheckboxEnd-1)
	if !page.selected[page.rows[1].RowKey()] || !page.selected[page.rows[2].RowKey()] || len(page.selected) != 2 {
		t.Fatalf("manage checkbox click selection=%v", page.selected)
	}

	before := cloneSelection(page.selected)
	clickListCell(h, page.list, 1, 1, 1)
	if !reflect.DeepEqual(page.selected, before) {
		t.Fatalf("manage description click changed selection: %v -> %v", before, page.selected)
	}
	if got := page.list.GetCurrentItem(); got != 2 {
		t.Fatalf("manage description click moved cursor to %d, want 2", got)
	}
}

func TestFavoritesClickZones(t *testing.T) {
	h, page := favoritesFixture(t)
	page.items = []storage.FavoriteModEntry{
		{FavoriteMod: storage.FavoriteMod{Platform: "modrinth", ModID: "alpha", Title: "Alpha", MinecraftVersion: "1.20.1", ModLoader: "Fabric"}},
		{FavoriteMod: storage.FavoriteMod{Platform: "modrinth", ModID: "beta", Title: "Beta", MinecraftVersion: "1.20.1", ModLoader: "Fabric"}},
		{FavoriteMod: storage.FavoriteMod{Platform: "modrinth", ModID: "gamma", Title: "Gamma", MinecraftVersion: "1.20.1", ModLoader: "Fabric"}},
	}
	page.renderRight()
	h.draw()
	keys := make([]string, len(page.items))
	for i, item := range page.items {
		keys[i] = uistate.FavoriteKey(item.FavoriteMod)
	}
	page.selected[keys[0]] = true
	page.selected[keys[2]] = true
	page.renderRight()
	h.draw()

	zones := page.itemClickZones(1)
	clickListCell(h, page.right, 1, 0, zones.TitleStart+1)
	if want := map[string]bool{keys[1]: true}; !reflect.DeepEqual(page.selected, want) {
		t.Fatalf("favorites title click selection=%v, want %v", page.selected, want)
	}
	if got := page.right.GetCurrentItem(); got != 1 {
		t.Fatalf("favorites title click cursor=%d, want 1", got)
	}

	clickListCell(h, page.right, 2, 0, zones.CheckboxEnd-1)
	if !page.selected[keys[1]] || !page.selected[keys[2]] || len(page.selected) != 2 {
		t.Fatalf("favorites checkbox click selection=%v", page.selected)
	}

	before := cloneSelection(page.selected)
	clickListCell(h, page.right, 0, 1, 1)
	if !reflect.DeepEqual(page.selected, before) {
		t.Fatalf("favorites description click changed selection: %v -> %v", before, page.selected)
	}
	if got := page.right.GetCurrentItem(); got != 2 {
		t.Fatalf("favorites description click moved cursor to %d, want 2", got)
	}
}
