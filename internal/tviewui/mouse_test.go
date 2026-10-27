package tviewui

import (
	"strings"
	"testing"

	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"

	"github.com/link-fgfgui/mod-downloader-core/models"
	structs "github.com/link-fgfgui/mod-downloader-core/structs/minecraft"

	"github.com/link-fgfgui/mod-downloader-cli/internal/i18n"
)

// mouse dispatches one event exactly like tview's mouse capture + root handler.
func (h *testApp) mouse(x, y int, buttons tcell.ButtonMask, action tview.MouseAction) bool {
	h.t.Helper()
	event := tcell.NewEventMouse(x, y, buttons, tcell.ModNone)
	event, action = h.app.handleGlobalMouse(event, action)
	if event == nil {
		return false
	}
	handler := h.app.outer.MouseHandler()
	if handler == nil {
		return false
	}
	consumed, _ := handler(action, event, func(p tview.Primitive) { h.app.application.SetFocus(p) })
	h.draw()
	return consumed
}

func (h *testApp) centerOf(p tview.Primitive, row int) (int, int) {
	h.t.Helper()
	x, y, _, _ := p.GetRect()
	return x + 2, y + row
}

func downloadResultsFixture(t *testing.T, count int) *testApp {
	t.Helper()
	h := newTestApp(t, 100, 30)
	h.app.mc.SelectedVersion = &structs.VersionInfo{ID: "inst", Name: "inst", MinecraftVersion: "1.20.1", ModLoader: "Fabric"}
	results := make([]models.ModProject, count)
	for i := range results {
		results[i] = models.ModProject{
			ID:        "modrinth:mod-" + itoa(i),
			Platform:  "modrinth",
			ProjectID: "mod-" + itoa(i),
			Title:     "Mod " + itoa(i),
		}
	}
	h.app.search.SearchResults = results
	h.app.downloadPage().Refresh()
	h.switchPage(ViewDownload)
	return h
}

// TestWheelScrollsResultList guards the fix for tview's wheel handling, which
// only moved an offset that Draw reset.
func TestWheelScrollsResultList(t *testing.T) {
	h := downloadResultsFixture(t, 30)
	page := h.app.downloadPage()
	x, y := h.centerOf(page.results, 3)
	for i := 0; i < 4; i++ {
		if !h.mouse(x, y, tcell.WheelDown, tview.MouseScrollDown) {
			t.Fatal("wheel event was not consumed by the result list")
		}
	}
	if got := page.results.GetCurrentItem(); got != 4 {
		t.Fatalf("cursor after wheel=%d, want 4", got)
	}
	h.mouse(x, y, tcell.WheelUp, tview.MouseScrollUp)
	if got := page.results.GetCurrentItem(); got != 3 {
		t.Fatalf("cursor after wheel up=%d, want 3", got)
	}
}

// TestClickTogglesResultCheckbox ensures the checkbox column selects instead of
// installing. Title/description hit zones are covered in list_click_test.go.
func TestClickTogglesResultCheckbox(t *testing.T) {
	h := downloadResultsFixture(t, 5)
	page := h.app.downloadPage()
	x, y := h.centerOf(page.results, 1)
	h.mouse(x, y, tcell.Button1, tview.MouseLeftClick)
	if len(page.selected) != 1 {
		t.Fatalf("selected=%d after click, want 1", len(page.selected))
	}
	h.mouse(x, y, tcell.Button1, tview.MouseLeftClick)
	if len(page.selected) != 0 {
		t.Fatalf("selected=%d after second click, want 0", len(page.selected))
	}
}

// TestManageClickTogglesCheckbox covers the row-activation side effect that
// used to select rows without repainting them.
func TestManageClickTogglesCheckbox(t *testing.T) {
	h := newTestApp(t, 120, 40)
	h.app.mc.SelectedModLoader = "Fabric"
	h.app.mc.SelectedVersion = &structs.VersionInfo{
		ID: "inst", Name: "inst", MinecraftVersion: "1.20.1", ModLoader: "Fabric",
		Mods: []structs.ModInfo{
			{ID: "alpha", Name: "Alpha", FileName: "alpha.jar", Path: "/mods/alpha.jar", Enabled: true},
			{ID: "beta", Name: "Beta", FileName: "beta.jar", Path: "/mods/beta.jar", Enabled: true},
		},
	}
	h.switchPage(ViewManage)
	page := h.app.managePage()
	x, y := h.centerOf(page.list, 1)
	h.mouse(x, y, tcell.Button1, tview.MouseLeftClick)
	if len(page.selected) != 1 {
		t.Fatalf("selected=%d after click, want 1", len(page.selected))
	}
	h.mustContain("[X]")
	h.mouse(x, y, tcell.Button1, tview.MouseLeftClick)
	if len(page.selected) != 0 {
		t.Fatalf("selected=%d after second click, want 0", len(page.selected))
	}
	h.mustContain("[ ]")
}

// TestActionBarIsClickable checks that a bar chip performs its action.
func TestActionBarIsClickable(t *testing.T) {
	h := downloadResultsFixture(t, 5)
	page := h.app.downloadPage()
	if len(page.bar.buttons) == 0 {
		t.Fatal("action bar has no buttons")
	}
	x, y, _, _ := page.bar.buttons[2].GetRect() // "Select all"
	h.mouse(x+1, y, tcell.Button1, tview.MouseLeftClick)
	if len(page.selected) != 5 {
		t.Fatalf("selected=%d after clicking Select all, want 5", len(page.selected))
	}
}

// TestThemeSwitchRestylesPages verifies that a palette change repaints widgets
// that were created with the old colours.
func TestThemeSwitchRestylesPages(t *testing.T) {
	h := newTestApp(t, 100, 30)
	dark := h.app.downloadPage().results.GetBackgroundColor()
	settings := h.app.settings
	settings.Theme = ThemeLight
	h.app.applySettings(settings)
	if h.app.theme.Name != ThemeLight {
		t.Fatalf("theme=%s, want light", h.app.theme.Name)
	}
	light := h.app.downloadPage().results.GetBackgroundColor()
	if light == dark {
		t.Fatal("result list kept the dark palette after switching to light")
	}
	if want := lightTheme().Surface; light != want {
		t.Fatalf("list background=%v, want %v", light, want)
	}
	if got := h.app.sidebar.GetBackgroundColor(); got != lightTheme().Surface {
		t.Fatalf("sidebar background=%v, want %v", got, lightTheme().Surface)
	}
	i18n.Apply(i18n.LanguageEnglish)
}

// TestWheelScrollsSettingsForm covers the settings page, whose Form has no
// usable scrolling of its own.
func TestWheelScrollsSettingsForm(t *testing.T) {
	h := newTestApp(t, 100, 30)
	h.switchPage(ViewSettings)
	page := h.app.settingsPage()
	x, y, _, _ := page.form.GetRect()
	before := h.text()
	for i := 0; i < 5; i++ {
		if !h.mouse(x+5, y+5, tcell.WheelDown, tview.MouseScrollDown) {
			t.Fatal("wheel event was not consumed by the settings form")
		}
	}
	if h.text() == before {
		t.Fatal("wheel did not scroll the settings form")
	}
	for i := 0; i < 5; i++ {
		h.mouse(x+5, y+5, tcell.WheelUp, tview.MouseScrollUp)
	}
	if h.text() != before {
		t.Fatal("wheel up did not restore the original view")
	}
}

// TestSettingsKeyboardKeepsFocusVisible checks that tabbing through the form
// scrolls it instead of moving the focus off screen.
func TestSettingsKeyboardKeepsFocusVisible(t *testing.T) {
	h := newTestApp(t, 100, 30)
	h.switchPage(ViewSettings)
	page := h.app.settingsPage()
	for i := 0; i < 30; i++ {
		h.key(tcell.KeyTab)
	}
	element := page.form.focusedElement()
	if element == nil {
		t.Fatal("form lost its focus")
	}
	_, ey, _, eh := element.GetRect()
	_, y, _, height := page.form.GetRect()
	if ey < y || ey+eh > y+height {
		t.Fatalf("focused element at y=%d..%d is outside the viewport %d..%d", ey, ey+eh, y, y+height)
	}
}

// TestStatusHintIsClickable checks that the bottom bar segments trigger their
// shortcuts when clicked.
func TestStatusHintIsClickable(t *testing.T) {
	h := newTestApp(t, 100, 30)
	h.switchPage(ViewHome)
	h.draw()
	row := h.lineCount() - 1
	cells, cw, _ := h.sim.GetContents()
	var line []rune
	for x := 0; x < cw; x++ {
		cell := cells[row*cw+x]
		if len(cell.Runes) > 0 {
			line = append(line, cell.Runes[0])
		} else {
			line = append(line, ' ')
		}
	}
	text := string(line)
	col := strings.Index(text, "t Instance")
	if col < 0 {
		t.Fatalf("hint segment not found in %q", text)
	}
	h.mouse(col+1, row, tcell.Button1, tview.MouseLeftClick)
	if h.app.topDialog() == nil {
		t.Fatal("clicking the instance hint did not open the target dialog")
	}
}

// TestDoubleClickActivatesRow guards against tview turning a fast second click
// into a double-click that no widget handles.
func TestDoubleClickActivatesRow(t *testing.T) {
	h := newTestApp(t, 100, 30)
	h.switchPage(ViewHome)
	x, y, _, _ := h.app.sidebar.GetRect()
	if !h.mouse(x+2, y+1+3, tcell.Button1, tview.MouseLeftDoubleClick) {
		t.Fatal("double-click was not consumed by the sidebar")
	}
	if h.app.active != ViewFavorites {
		t.Fatalf("active view=%d after double-click, want favorites", h.app.active)
	}
}

// TestBusySpinnerShowsInStatusBar checks the loading indicator used while an
// instance switch or refresh runs.
func TestBusySpinnerShowsInStatusBar(t *testing.T) {
	h := newTestApp(t, 100, 30)
	h.app.beginBusy("Loading test")
	h.app.updateStatus()
	h.draw()
	h.mustContain("Loading test")
	h.app.endBusy()
	h.app.updateStatus()
	h.draw()
	h.mustNotContain("Loading test")
}

// TestBusyLabelAppearsInTargetDialog checks the visible loading state while an
// instance switch runs with the switcher open.
func TestBusyLabelAppearsInTargetDialog(t *testing.T) {
	h := newTestApp(t, 100, 30)
	h.app.openTargetDialog()
	h.app.beginBusy("Switching test")
	h.app.targetDialog.refresh()
	h.draw()
	h.mustContain("Switching test")
	h.app.endBusy()
	h.app.targetDialog.refresh()
	h.draw()
	h.mustNotContain("Switching test")
}

// TestSettingsCheckboxRendersCheckedState guards the tag-parsing bug that made
// a checked box disappear.
func TestSettingsCheckboxRendersCheckedState(t *testing.T) {
	h := newTestApp(t, 100, 30)
	h.switchPage(ViewSettings)
	page := h.app.settingsPage()
	h.draw()
	h.mustContain("[ ]")
	page.simpleMode.SetChecked(true)
	h.draw()
	h.mustContain("[X]")
}

// TestThemeSwitchRepaintsTextBackgrounds checks that a live palette change
// repaints the text cells too, not only the widget borders.
func TestThemeSwitchRepaintsTextBackgrounds(t *testing.T) {
	h := newTestApp(t, 120, 40)
	i18n.Apply(i18n.LanguageEnglish)
	h.switchPage(ViewHome)
	h.draw()
	settings := h.app.settings
	settings.Theme = ThemeLight
	h.app.applySettings(settings)
	h.switchPage(ViewHome)
	h.draw()
	cells, w, _ := h.sim.GetContents()
	dark := 0
	for y := 0; y < 40; y++ {
		for x := 0; x < 120; x++ {
			cell := cells[y*w+x]
			if len(cell.Runes) == 0 || cell.Runes[0] == ' ' {
				continue
			}
			_, bg, _ := cell.Style.Decompose()
			if bg == darkTheme().Surface {
				dark++
			}
		}
	}
	if dark > 20 {
		t.Fatalf("%d text cells kept the dark background after switching to light", dark)
	}
}
