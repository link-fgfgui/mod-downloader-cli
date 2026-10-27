package tviewui

import (
	"os"
	"path/filepath"
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

func dialogFlex(t *testing.T, h *testApp) *tview.Flex {
	t.Helper()
	centered, ok := h.app.topDialog().page.(*centered)
	if !ok {
		t.Fatalf("dialog page=%T", h.app.topDialog().page)
	}
	box, ok := centered.content.(*tview.Flex)
	if !ok {
		t.Fatalf("dialog content=%T", centered.content)
	}
	return box
}

func dialogItem(t *testing.T, h *testApp, index int) tview.Primitive {
	t.Helper()
	box := dialogFlex(t, h)
	if index < 0 || index >= box.GetItemCount() {
		t.Fatalf("dialog item %d out of range (%d items)", index, box.GetItemCount())
	}
	return box.GetItem(index)
}

func TestConfirmDialogScrollsAndActivates(t *testing.T) {
	h := newTestApp(t, 80, 24)
	chosen := ""
	h.app.openConfirm("Confirm", strings.Repeat("line\n", 30), []confirmOption{
		{Label: "OK", Value: "ok", Primary: true},
		{Label: "Cancel", Value: "cancel"},
	}, func(value string) { chosen = value })
	h.draw()

	body, ok := dialogItem(t, h, 0).(*tview.TextView)
	if !ok {
		t.Fatalf("confirm body=%T", dialogItem(t, h, 0))
	}
	if row, _ := body.GetScrollOffset(); row != 0 {
		t.Fatalf("body started scrolled at %d", row)
	}
	h.key(tcell.KeyPgDn)
	if row, _ := body.GetScrollOffset(); row == 0 {
		t.Fatal("PgDn did not scroll the confirm body")
	}
	h.key(tcell.KeyPgUp)
	h.key(tcell.KeyDown)
	h.key(tcell.KeyUp)
	if row, _ := body.GetScrollOffset(); row != 0 {
		t.Fatalf("scroll keys did not return the body to the top: %d", row)
	}

	// Long bodies are capped so the dialog stays inside small terminals.
	if _, _, _, height := h.app.topDialog().page.GetRect(); height > 18 {
		t.Fatalf("confirm dialog height=%d, want <=18", height)
	}

	h.key(tcell.KeyEnter)
	if chosen != "ok" {
		t.Fatalf("confirm value=%q", chosen)
	}
	if h.app.topDialog() != nil {
		t.Fatal("confirm dialog stayed open after activation")
	}
}

func TestConfirmDialogButtonNavigation(t *testing.T) {
	h := newTestApp(t, 80, 24)
	chosen := ""
	h.app.openConfirm("Confirm", "body", []confirmOption{
		{Label: "One", Value: "1", Primary: true},
		{Label: "Two", Value: "2"},
		{Label: "Three", Value: "3"},
	}, func(value string) { chosen = value })
	h.key(tcell.KeyRight)
	h.key(tcell.KeyRight)
	h.key(tcell.KeyEnter)
	if chosen != "3" {
		t.Fatalf("right navigation chose %q, want 3", chosen)
	}

	chosen = ""
	h.app.openConfirm("Confirm", "body", []confirmOption{
		{Label: "One", Value: "1"},
		{Label: "Two", Value: "2"},
	}, func(value string) { chosen = value })
	h.key(tcell.KeyLeft) // wrap to the last option
	h.key(tcell.KeyEnter)
	if chosen != "2" {
		t.Fatalf("left wrap chose %q, want 2", chosen)
	}

	chosen = ""
	h.app.openConfirm("Confirm", "body", []confirmOption{
		{Label: "One", Value: "1"},
		{Label: "Two", Value: "2"},
	}, func(value string) { chosen = value })
	h.key(tcell.KeyBacktab)
	h.key(tcell.KeyEnter)
	if chosen != "2" {
		t.Fatalf("backtab chose %q, want 2", chosen)
	}
}

func TestPromptDialogValidationKeepsDialogOpen(t *testing.T) {
	h := newTestApp(t, 80, 24)
	submitted := ""
	h.app.openPrompt("Name", "hint", "", "name", func(value string) string {
		if value == "" {
			return "required"
		}
		return ""
	}, func(value string) { submitted = value })
	h.draw()

	// Layout with a hint: input, error, buttons.
	input, ok := dialogItem(t, h, 1).(*tview.InputField)
	if !ok {
		t.Fatalf("prompt input=%T", dialogItem(t, h, 1))
	}
	errView, ok := dialogItem(t, h, 2).(*tview.TextView)
	if !ok {
		t.Fatalf("prompt error view=%T", dialogItem(t, h, 2))
	}
	h.key(tcell.KeyEnter)
	if !strings.Contains(errView.GetText(true), "required") {
		t.Fatalf("validation error not shown: %q", errView.GetText(true))
	}
	if len(h.app.dialogs) != 1 || submitted != "" {
		t.Fatal("invalid prompt was submitted or closed")
	}

	input.SetText("  hello  ")
	h.key(tcell.KeyEnter)
	if submitted != "hello" {
		t.Fatalf("submitted=%q, want trimmed hello", submitted)
	}
	if h.app.topDialog() != nil {
		t.Fatal("valid prompt did not close")
	}
}

func TestPromptDialogCancelButton(t *testing.T) {
	h := newTestApp(t, 80, 24)
	submitted := ""
	h.app.openPrompt("Name", "", "", "name", nil, func(value string) { submitted = value })
	h.key(tcell.KeyTab)     // input -> Confirm
	h.key(tcell.KeyBacktab) // Confirm -> Cancel (wraps)
	h.key(tcell.KeyEnter)
	if submitted != "" {
		t.Fatalf("cancel submitted %q", submitted)
	}
	if h.app.topDialog() != nil {
		t.Fatal("cancel did not close the prompt")
	}
}

func TestSelectDialogSkipsDisabledItems(t *testing.T) {
	h := newTestApp(t, 80, 24)
	chosen := ""
	h.app.openSelect("Select", []selectItem{
		{Label: "Disabled", Value: "d", Disabled: true},
		{Label: "Enabled", Value: "e"},
	}, func(item selectItem) { chosen = item.Value })
	h.draw()
	h.key(tcell.KeyEnter)
	if chosen != "" {
		t.Fatalf("disabled item was selected: %q", chosen)
	}
	if len(h.app.dialogs) != 1 {
		t.Fatal("disabled selection closed the dialog")
	}
	h.key(tcell.KeyDown)
	h.key(tcell.KeyEnter)
	if chosen != "e" {
		t.Fatalf("enabled item selected %q, want e", chosen)
	}
	if h.app.topDialog() != nil {
		t.Fatal("select dialog stayed open after activation")
	}

	h.app.openSelect("Select", []selectItem{{Label: "Only", Value: "1"}}, func(selectItem) {})
	h.key(tcell.KeyEsc)
	if h.app.topDialog() != nil {
		t.Fatal("Esc did not close the select dialog")
	}
}

func TestPathDialogSaveModeAndValidation(t *testing.T) {
	dir := t.TempDir()

	h := newTestApp(t, 100, 30)
	accepted := ""
	v := h.app.openPathDialog("Save", true, filepath.Join(dir, "out"), func(path string) { accepted = path })
	h.draw()
	v.input.SetText(filepath.Join(dir, "out"))
	h.key(tcell.KeyEnter)
	if want := filepath.Join(dir, "out.zip"); accepted != want {
		t.Fatalf("save path=%q, want %q", accepted, want)
	}

	accepted = ""
	h.app.openPathDialog("Save", true, filepath.Join(dir, "ok"), func(path string) { accepted = path })
	h.key(tcell.KeyCtrlS)
	if want := filepath.Join(dir, "ok.zip"); accepted != want {
		t.Fatalf("Ctrl+S accepted %q, want %q", accepted, want)
	}

	accepted = ""
	h.app.openPathDialog("Save", true, dir, func(path string) { accepted = path })
	pathDialog := h.app.topDialog().page.(*centered).content.(*tview.Flex)
	input := pathDialog.GetItem(1).(*tview.InputField)
	errView := pathDialog.GetItem(2).(*tview.TextView)
	input.SetText(filepath.Join(dir, "missing", "out"))
	h.key(tcell.KeyEnter)
	if accepted != "" || len(h.app.dialogs) != 1 {
		t.Fatal("invalid save path was accepted or closed the dialog")
	}
	if !strings.Contains(errView.GetText(true), uiText("invalidDirectory")) {
		t.Fatalf("invalid path error=%q", errView.GetText(true))
	}
	h.key(tcell.KeyEsc)
	if h.app.topDialog() != nil {
		t.Fatal("Esc did not close the invalid path dialog")
	}

	// Cancel button: Backtab twice from the input reaches Cancel.
	accepted = ""
	h.app.openPathDialog("Save", true, dir, func(path string) { accepted = path })
	h.key(tcell.KeyBacktab)
	h.key(tcell.KeyBacktab)
	h.key(tcell.KeyEnter)
	if accepted != "" {
		t.Fatalf("cancel accepted %q", accepted)
	}
	if h.app.topDialog() != nil {
		t.Fatal("cancel did not close the path dialog")
	}
}

func TestPathDialogCompletionStates(t *testing.T) {
	dir := t.TempDir()
	if err := os.Mkdir(filepath.Join(dir, "alpha"), 0o755); err != nil {
		t.Fatal(err)
	}
	h := newTestApp(t, 100, 30)
	v := h.app.openPathDialog("Path", false, dir, nil)
	v.input.SetText(filepath.Join(dir, "zzz"))
	h.key(tcell.KeyTab)
	box := dialogFlex(t, h)
	errView := box.GetItem(2).(*tview.TextView)
	if !strings.Contains(errView.GetText(true), uiText("noCompletions")) {
		t.Fatalf("no-match error=%q", errView.GetText(true))
	}
	if len(h.app.dialogs) != 1 {
		t.Fatal("no-match completion closed the dialog")
	}

	v.completions = []string{"a", "b", "c", "d", "e", "f", "g", "h", "i", "j"}
	v.renderCandidates()
	list := box.GetItem(3).(*tview.TextView)
	text := list.GetText(true)
	if !strings.Contains(text, "…") {
		t.Fatalf("candidate list did not truncate: %q", text)
	}
	if lines := strings.Count(strings.TrimRight(text, "\n"), "\n") + 1; lines > 9 {
		t.Fatalf("candidate list rendered %d lines", lines)
	}
	h.key(tcell.KeyEsc)
}

func TestInitialPathResolution(t *testing.T) {
	home, err := os.UserHomeDir()
	if err != nil {
		t.Fatal(err)
	}
	if got, want := initialPath(true, ""), filepath.Join(home, "packwiz.zip"); got != want {
		t.Fatalf("save default=%q, want %q", got, want)
	}
	if got, want := initialPath(true, "mods.zip"), filepath.Join(home, "mods.zip"); got != want {
		t.Fatalf("save relative=%q, want %q", got, want)
	}
	if got, want := initialPath(false, ""), home; got != want {
		t.Fatalf("directory default=%q, want %q", got, want)
	}
	if got, want := initialPath(false, "~"), home; got != want {
		t.Fatalf("tilde=%q, want %q", got, want)
	}
	dir := t.TempDir()
	file := filepath.Join(dir, "a.txt")
	if err := os.WriteFile(file, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	if got := initialPath(false, file); got != dir {
		t.Fatalf("file start=%q, want parent %q", got, dir)
	}
}

func TestTargetDialogRowActivation(t *testing.T) {
	h := newTestApp(t, 100, 30)
	h.app.mc.ReleaseVersions = []string{"1.20.1", "1.20.4", "1.21.1"}
	h.app.mc.MinecraftDir = "/tmp/.minecraft"
	h.app.mc.Versions = []structs.VersionInfo{
		{ID: "inst-1", Name: "Instance One", MinecraftVersion: "1.20.1", ModLoader: "Fabric"},
		{ID: "inst-2", Name: "Instance Two", MinecraftVersion: "1.20.4", ModLoader: "Fabric"},
	}
	h.app.openTargetDialog()
	h.draw()
	if !strings.Contains(h.text(), "Minecraft Version") {
		t.Fatalf("target dialog rows missing:\n%s", h.text())
	}

	// Directory row opens the path picker; closing it keeps the target dialog.
	h.app.targetDialog.list.SetCurrentItem(0)
	h.key(tcell.KeyEnter)
	if top := h.app.topDialog(); top == nil || !strings.HasPrefix(top.name, "path") {
		t.Fatalf("dir row opened %v", top)
	}
	h.key(tcell.KeyEsc)
	if top := h.app.topDialog(); top == nil || !strings.HasPrefix(top.name, "target") {
		t.Fatalf("target dialog was not restored after the path picker: %v", top)
	}

	// Instance row opens the instance selector.
	h.app.targetDialog.list.SetCurrentItem(1)
	h.key(tcell.KeyEnter)
	if top := h.app.topDialog(); top == nil || !strings.HasPrefix(top.name, "select") {
		t.Fatalf("instance row opened %v", top)
	}
	h.key(tcell.KeyEsc)
	if top := h.app.topDialog(); top == nil || !strings.HasPrefix(top.name, "target") {
		t.Fatal("target dialog lost after closing the instance picker")
	}

	// Refresh rows are no-ops in offline mode but must be handled.
	h.app.targetDialog.list.SetCurrentItem(2)
	h.key(tcell.KeyEnter)
	h.app.targetDialog.list.SetCurrentItem(3)
	h.key(tcell.KeyEnter)

	// Empty instance list shows a toast instead of an empty picker.
	saved := h.app.mc.Versions
	h.app.mc.Versions = nil
	h.app.targetDialog.list.SetCurrentItem(1)
	h.key(tcell.KeyEnter)
	if h.app.toastText != i18n.T("home.status.notAvailable") {
		t.Fatalf("empty instance toast=%q", h.app.toastText)
	}
	if h.app.toastTimer != nil {
		h.app.toastTimer.Stop()
	}
	h.app.mc.Versions = saved
	h.key(tcell.KeyEsc)
}

func TestVersionDialogStatesAndPinning(t *testing.T) {
	h := newTestApp(t, 100, 30)
	var pinned models.ModVersion
	v := h.app.openVersionDialog(projectFixture(), "pin", func(version models.ModVersion) { pinned = version }, nil)
	h.draw()
	if !strings.Contains(h.text(), uiText("loading")) {
		t.Fatal("version dialog did not render its loading state")
	}

	v.setVersions([]models.ModVersion{
		{ID: "v1", Name: "One", Version: "1.0", ReleaseType: "release"},
		{ID: "v2", Name: "Two", Version: "2.0", ReleaseType: "beta"},
	}, storage.PinnedMod{VersionID: "v1"})
	h.draw()
	if got := v.list.GetItemCount(); got != 2 {
		t.Fatalf("version rows=%d, want 2", got)
	}
	main, _ := v.list.GetItemText(0)
	if !strings.Contains(main, "★") {
		t.Fatalf("pinned marker missing: %q", main)
	}
	h.key(tcell.KeyEnter)
	if pinned.ID != "v1" {
		t.Fatalf("pinned version=%q, want v1", pinned.ID)
	}
	if v.busyVersionID != "v1" {
		t.Fatalf("busy marker=%q", v.busyVersionID)
	}
	v.setPinned(storage.PinnedMod{VersionID: "v1"})
	if v.busyVersionID != "" {
		t.Fatal("setPinned did not clear the busy marker")
	}

	v.installedID = "v2"
	pinned = models.ModVersion{}
	h.key(tcell.KeyDown) // current row v2 (installed)
	h.key(tcell.KeyEnter)
	if pinned.ID != "" {
		t.Fatalf("installed version was pinned: %q", pinned.ID)
	}

	pinned = models.ModVersion{}
	h.key(tcell.KeyUp) // back to v1
	h.rune('p')
	if pinned.ID != "v1" {
		t.Fatalf("p key pinned %q, want v1", pinned.ID)
	}

	h.key(tcell.KeyEsc)
	if h.app.versionDialog != nil {
		t.Fatal("version dialog was not cleared on close")
	}

	// Empty result state.
	h2 := newTestApp(t, 100, 30)
	v2 := h2.app.openVersionDialog(projectFixture(), "pin", nil, nil)
	v2.setVersions(nil, storage.PinnedMod{})
	h2.draw()
	if !strings.Contains(h2.text(), i18n.T("download.noMatchingVersions")) {
		t.Fatal("empty version list state missing")
	}
	h2.key(tcell.KeyEsc)

	if !isInstalledVersion(models.ModVersion{ID: "x"}, "x", "") {
		t.Fatal("installed ID was not detected")
	}
	if !isInstalledVersion(models.ModVersion{SHA1: "ABCDEF"}, "", "abcdef") {
		t.Fatal("installed SHA1 was not detected case-insensitively")
	}
	if isInstalledVersion(models.ModVersion{ID: "x"}, "y", "") {
		t.Fatal("different version reported as installed")
	}
}

func TestVersionDialogReplaceAction(t *testing.T) {
	h := newTestApp(t, 100, 30)
	selected := ""
	v := h.app.openVersionDialog(projectFixture(), "replace", nil, func(version models.ModVersion) { selected = version.ID })
	v.setVersions([]models.ModVersion{{ID: "v9", Name: "Nine", Version: "9.0"}}, storage.PinnedMod{})
	h.key(tcell.KeyEnter)
	if selected != "v9" {
		t.Fatalf("replace selected %q, want v9", selected)
	}
	if h.app.topDialog() != nil {
		t.Fatal("replace did not close the version dialog")
	}
}

func TestAddFavoriteDialogSelectionAndCancel(t *testing.T) {
	h := newTestApp(t, 100, 30)
	h.app.mc.SelectedMinecraftVersion = "1.20.1"
	h.app.mc.SelectedModLoader = "Fabric"
	page := h.app.favoritesPage()
	page.SetScope("1.20.1", "Fabric")
	page.setLists([]storage.FavoriteList{
		{ID: "l1", Name: "First", MinecraftVersion: "1.20.1", ModLoader: "Fabric"},
		{ID: "l2", Name: "Second", MinecraftVersion: "1.20.1", ModLoader: "Fabric"},
	}, storage.FavoriteListContents{}, "")

	h.app.openAddFavoriteDialog([]favoriteDraft{{Platform: "modrinth", ModID: "sodium", Title: "Sodium"}})
	h.draw()
	if top := h.app.topDialog(); top == nil || !strings.HasPrefix(top.name, "add-favorite") {
		t.Fatalf("add favorite dialog=%v", top)
	}
	h.rune(' ') // tick the first list
	if !strings.Contains(h.text(), "[X]") {
		t.Fatal("space did not tick a favorite list")
	}
	h.key(tcell.KeyEnter)
	if h.app.topDialog() != nil {
		t.Fatal("confirming did not close the add favorite dialog")
	}

	// "n" opens a nested create prompt and Esc returns to the picker.
	h.app.openAddFavoriteDialog([]favoriteDraft{{Platform: "modrinth", ModID: "lithium"}})
	h.rune('n')
	if top := h.app.topDialog(); top == nil || !strings.HasPrefix(top.name, "prompt") {
		t.Fatalf("n did not open the create prompt: %v", top)
	}
	h.key(tcell.KeyEsc)
	if top := h.app.topDialog(); top == nil || !strings.HasPrefix(top.name, "add-favorite") {
		t.Fatalf("closing the prompt lost the picker: %v", top)
	}
	h.key(tcell.KeyEsc)

	// Missing scope reports a toast instead of opening a broken dialog.
	h.app.mc.SelectedMinecraftVersion = ""
	h.app.openAddFavoriteDialog([]favoriteDraft{{Platform: "modrinth", ModID: "sodium"}})
	if h.app.topDialog() != nil {
		t.Fatal("dialog opened without a target scope")
	}
	if h.app.toastText != i18n.T("favorites.empty.selectOrCreate") {
		t.Fatalf("missing-scope toast=%q", h.app.toastText)
	}
	if h.app.toastTimer != nil {
		h.app.toastTimer.Stop()
	}
}

func TestFavoriteRefsDialogRendering(t *testing.T) {
	h := newTestApp(t, 100, 30)
	h.app.openFavoriteRefsDialog("parent")
	v := h.app.refsDialog
	v.setRefs([]storage.FavoriteListRef{
		{ParentListID: "parent", ChildListID: "child-1"},
		{ParentListID: "parent", ChildListID: "missing"},
	}, []storage.FavoriteList{{ID: "child-1", Name: "Child One"}})
	h.draw()
	text := h.text()
	if !strings.Contains(text, "Child One") || !strings.Contains(text, "missing") {
		t.Fatalf("refs dialog text:\n%s", text)
	}
	v.setRefs(nil, nil)
	h.draw()
	if !strings.Contains(h.text(), i18n.T("favorites.empty.noItems")) {
		t.Fatal("empty refs state missing")
	}
	h.key(tcell.KeyEsc)
	if h.app.refsDialog != nil {
		t.Fatal("refs dialog pointer was not cleared")
	}
}

func TestDisableImpactDialogRenderingAndRestoreStates(t *testing.T) {
	h := newTestApp(t, 100, 40)
	result := appstructs.LocalModDisableImpactResult{
		Impacts: []appstructs.LocalModDependencyImpact{{
			ModID:        "fabric-api",
			AffectedMods: []appstructs.LocalModDependencyMod{{Name: "", FileName: "sodium.jar"}},
			Candidates:   nil,
		}},
	}
	h.app.managePage().pending = []uistate.ModGroup{{}}
	h.app.managePage().pendingAction = "disable"
	h.app.openDisableImpactDialog(nil, "disable", result)
	h.draw()
	if !strings.Contains(h.text(), "fabric-api") {
		t.Fatalf("impact row missing:\n%s", h.text())
	}
	if !strings.Contains(h.text(), i18n.T("manage.disableImpact.cacheMiss")) {
		t.Fatal("cache-miss marker missing")
	}

	// No cached candidates -> toast, no controller call.
	h.rune('r')
	if h.app.toastText != i18n.T("manage.disableImpact.cacheMiss") {
		t.Fatalf("cache miss toast=%q", h.app.toastText)
	}
	if h.app.toastTimer != nil {
		h.app.toastTimer.Stop()
	}

	// Multiple candidates -> source picker; Esc returns to the impact dialog.
	v := h.app.impactDialog
	v.result.Impacts[0].Candidates = []appstructs.CachedDependencyCandidate{
		{Platform: "modrinth", ProjectID: "fabric-api", VersionID: "1"},
		{Platform: "curseforge", ProjectID: "306612", VersionID: "2"},
	}
	v.render()
	h.rune('r')
	if top := h.app.topDialog(); top == nil || !strings.HasPrefix(top.name, "select") {
		t.Fatalf("multi-candidate restore opened %v", top)
	}
	h.key(tcell.KeyEsc)
	if top := h.app.topDialog(); top == nil || !strings.HasPrefix(top.name, "disable-impact") {
		t.Fatal("impact dialog lost after closing the source picker")
	}

	// Restore completion marks the row without calling the core.
	v.restoring = "fabric-api"
	v.restoreDone("fabric-api", true)
	if !v.restored["fabric-api"] {
		t.Fatal("restoreDone did not mark the impact")
	}
	h.draw()
	if !strings.Contains(h.text(), "✓") {
		t.Fatal("restored marker missing")
	}
	v.restoring = "fabric-api"
	v.restoreDone("fabric-api", false)
	if v.restored["fabric-api"] != true {
		t.Fatal("failed restore removed the completed marker")
	}
	if h.app.toastTimer != nil {
		h.app.toastTimer.Stop()
	}

	h.key(tcell.KeyEsc)
	if h.app.impactDialog != nil || h.app.managePage().pending != nil {
		t.Fatal("impact dialog close did not clear pending state")
	}
}

func TestDialogStackOperations(t *testing.T) {
	h := newTestApp(t, 80, 24)
	first := &dialog{name: "first", page: tview.NewBox(), focus: tview.NewBox()}
	second := &dialog{name: "second", page: tview.NewBox(), focus: tview.NewBox()}
	third := &dialog{name: "third", page: tview.NewBox(), focus: tview.NewBox()}
	closed := ""
	second.onClose = func() { closed = "second" }
	third.onClose = func() { closed = "third" }

	h.app.pushDialog(first)
	h.app.pushDialog(second)
	h.app.pushDialog(third)
	if len(h.app.dialogs) != 3 {
		t.Fatalf("dialogs=%d, want 3", len(h.app.dialogs))
	}

	// Duplicate names are ignored.
	h.app.pushDialog(&dialog{name: "second", page: tview.NewBox()})
	if len(h.app.dialogs) != 3 {
		t.Fatal("duplicate dialog name was pushed")
	}

	// Removing a middle dialog keeps the stack order and calls its onClose.
	h.app.popDialogName("second")
	if len(h.app.dialogs) != 2 || h.app.topDialog().name != "third" {
		t.Fatalf("stack after middle pop: %+v", h.app.dialogs)
	}
	if closed != "second" {
		t.Fatalf("middle dialog onClose=%q", closed)
	}

	// Esc closes only the top.
	h.key(tcell.KeyEsc)
	if len(h.app.dialogs) != 1 || h.app.topDialog().name != "first" {
		t.Fatalf("Esc stack=%+v", h.app.dialogs)
	}
	h.key(tcell.KeyEsc)
	if len(h.app.dialogs) != 0 {
		t.Fatal("Esc did not close the last dialog")
	}

	// popDialog on an empty stack is a no-op.
	h.app.popDialog()
	h.app.popDialogName("missing")

	names := map[string]bool{}
	for i := 0; i < 5; i++ {
		names[h.app.dialogName("x")] = true
	}
	if len(names) != 5 {
		t.Fatalf("dialogName collisions: %#v", names)
	}
}

func TestCenteredClampsToScreen(t *testing.T) {
	sim := tcell.NewSimulationScreen("UTF-8")
	if err := sim.Init(); err != nil {
		t.Fatal(err)
	}
	defer sim.Fini()
	sim.SetSize(20, 10)

	page := newCentered(tview.NewBox(), 10, 4)
	page.Draw(sim)
	x, y, w, h := page.GetRect()
	if w != 18 || h != 5 {
		t.Fatalf("clamped rect=%dx%d, want 18x5", w, h)
	}
	if x != 1 || y != 2 {
		t.Fatalf("centered at %d,%d", x, y)
	}
	if page.InputHandler() == nil || page.PasteHandler() == nil || page.MouseHandler() == nil {
		t.Fatal("centered handlers are nil")
	}
	if !page.HasFocus() && page.HasFocus() { // HasFocus must be callable
		t.Fatal("unreachable")
	}
	page.Focus(func(tview.Primitive) {})
	page.Blur()

	// Clicks inside are consumed, outside are not.
	handler := page.MouseHandler()
	inside := tcell.NewEventMouse(x+1, y+1, tcell.Button1, tcell.ModNone)
	if consumed, _ := handler(tview.MouseLeftClick, inside, func(tview.Primitive) {}); !consumed {
		t.Fatal("inside click was not consumed")
	}
	outside := tcell.NewEventMouse(0, 0, tcell.Button1, tcell.ModNone)
	if consumed, _ := handler(tview.MouseLeftClick, outside, func(tview.Primitive) {}); consumed {
		t.Fatal("outside click was consumed")
	}
}
