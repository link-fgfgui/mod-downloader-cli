package tviewui

import (
	"flag"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"

	"github.com/link-fgfgui/mod-downloader-core/models"
	appstructs "github.com/link-fgfgui/mod-downloader-core/structs"
	structs "github.com/link-fgfgui/mod-downloader-core/structs/minecraft"

	"github.com/link-fgfgui/mod-downloader-cli/internal/i18n"
	"github.com/link-fgfgui/mod-downloader-cli/internal/tuihost"
)

var updateGolden = flag.Bool("update", false, "update golden frames")

func structsVersion(index int, mcVersion, modLoader string) structs.VersionInfo {
	return structs.VersionInfo{
		ID:               "inst-" + itoa(index),
		Name:             "Instance " + itoa(index),
		MinecraftVersion: mcVersion,
		ModLoader:        modLoader,
	}
}

// TestRenderMatrix asserts that every page renders inside every required
// terminal size in both languages and never overflows a line.
func TestRenderMatrix(t *testing.T) {
	sizes := [][2]int{{50, 20}, {80, 24}, {100, 30}, {120, 40}, {160, 50}}
	for _, lang := range []string{i18n.LanguageEnglish, i18n.LanguageChinese} {
		for _, size := range sizes {
			h := newTestApp(t, size[0], size[1])
			i18n.Apply(lang)
			h.app.rebuildAllText()
			h.draw()
			for id := ViewID(0); id < ViewID(viewCount); id++ {
				h.switchPage(id)
				lines := strings.Split(strings.TrimRight(h.text(), "\n"), "\n")
				if len(lines) != size[1] {
					t.Fatalf("%s %dx%d view %d: %d lines", lang, size[0], size[1], id, len(lines))
				}
				for row, line := range lines {
					if width := tview.TaggedStringWidth(line); width > size[0] {
						t.Fatalf("%s %dx%d view %d row %d: width %d exceeds %d: %q",
							lang, size[0], size[1], id, row, width, size[0], line)
					}
				}
			}
		}
	}
}

// TestDialogRenderMatrix opens every dialog at every required size in both
// languages and asserts the frame stays within bounds.
func TestDialogRenderMatrix(t *testing.T) {
	sizes := [][2]int{{50, 20}, {80, 24}, {100, 30}, {120, 40}, {160, 50}}
	for _, lang := range []string{i18n.LanguageEnglish, i18n.LanguageChinese} {
		for _, size := range sizes {
			h := newTestApp(t, size[0], size[1])
			i18n.Apply(lang)
			h.app.rebuildAllText()
			h.draw()
			assertFrameBounds(t, h, size[0], size[1])

			h.app.openConfirm(
				i18n.T("download.confirmReplace.updateTitle"),
				strings.Repeat(i18n.T("download.confirmReplace.updateBody")+"\n", 8),
				[]confirmOption{
					{Label: uiText("confirm"), Value: "ok", Primary: true},
					{Label: uiText("cancel"), Value: "cancel"},
				}, nil)
			assertFrameBounds(t, h, size[0], size[1])
			h.key(tcell.KeyEsc)

			h.app.openPrompt(i18n.T("favorites.dialog.createTitle"), i18n.T("favorites.dialog.name"), "", i18n.T("favorites.dialog.name"), nil, nil)
			assertFrameBounds(t, h, size[0], size[1])
			h.key(tcell.KeyEsc)

			h.app.openSelect(i18n.T("favorites.dialog.copyListTitle"), []selectItem{
				{Label: "列表一", Value: "1", Detail: "modrinth", Disabled: false},
				{Label: "列表二", Value: "2", Detail: "", Disabled: true},
			}, nil)
			assertFrameBounds(t, h, size[0], size[1])
			h.key(tcell.KeyEsc)

			h.app.openPathDialog(i18n.T("settings.cacheDir.choose"), false, t.TempDir(), nil)
			assertFrameBounds(t, h, size[0], size[1])
			h.key(tcell.KeyEsc)

			h.app.openTargetDialog()
			assertFrameBounds(t, h, size[0], size[1])
			h.key(tcell.KeyEsc)

			h.app.openHelpDialog()
			assertFrameBounds(t, h, size[0], size[1])
			h.key(tcell.KeyEsc)

			h.app.openVersionDialog(projectFixture(), "pin", func(models.ModVersion) {}, nil)
			assertFrameBounds(t, h, size[0], size[1])
			h.key(tcell.KeyEsc)

			h.app.mc.SelectedMinecraftVersion = "1.20.1"
			h.app.openAddFavoriteDialog([]favoriteDraft{{Platform: "modrinth", ModID: "sodium", Title: "Sodium 钠"}})
			assertFrameBounds(t, h, size[0], size[1])
			h.key(tcell.KeyEsc)

			h.app.openFavoriteRefsDialog("list-1")
			assertFrameBounds(t, h, size[0], size[1])
			h.key(tcell.KeyEsc)

			h.app.openDisableImpactDialog(nil, "disable", appstructs.LocalModDisableImpactResult{
				Impacts: []appstructs.LocalModDependencyImpact{{
					ModID:        "fabric-api",
					AffectedMods: []appstructs.LocalModDependencyMod{{Name: "钠 Sodium", FileName: "sodium.jar"}},
					Candidates:   []appstructs.CachedDependencyCandidate{{Platform: "modrinth", ProjectID: "fabric-api", VersionID: "1"}},
				}},
			})
			assertFrameBounds(t, h, size[0], size[1])
			h.key(tcell.KeyEsc)
		}
	}
}

func assertFrameBounds(t *testing.T, h *testApp, w, height int) {
	t.Helper()
	h.draw()
	lines := strings.Split(strings.TrimRight(h.text(), "\n"), "\n")
	if len(lines) != height {
		t.Fatalf("%d lines, want %d", len(lines), height)
	}
	for row, line := range lines {
		if width := tview.TaggedStringWidth(line); width > w {
			t.Fatalf("row %d width %d exceeds %d: %q", row, width, w, line)
		}
	}
}

// TestVersionChangeDoesNotScheduleRefresh is the regression guard for the
// refresh -> event -> refresh loop: adopting the core's version snapshot marks
// the instance's mods as loaded so no extra refresh is requested.
func TestVersionChangeDoesNotScheduleRefresh(t *testing.T) {
	h := newTestApp(t, 80, 24)
	h.app.mc.MinecraftDir = "/tmp/.minecraft"
	version := structsVersion(1, "1.20.1", "Fabric")
	for i := 0; i < 25; i++ {
		h.app.handleEvent(tuihost.Event{Kind: tuihost.EventSelectedVersionChanged, Payload: version})
	}
	if h.app.mc.SelectedVersion == nil || h.app.mc.SelectedVersion.ID != "inst-1" {
		t.Fatalf("selected version not applied: %+v", h.app.mc.SelectedVersion)
	}
	if h.app.mc.EnsureSelectedModsLoadedNeeded() {
		t.Fatal("version event scheduled another mods refresh")
	}
	if got := h.app.targetTuple(); got != "1.20.1 | Fabric" {
		t.Fatalf("tuple=%q", got)
	}
	if h.app.mc.InitializedSelectedModsKey == "" {
		t.Fatal("mods-loaded key was not recorded")
	}
}

// TestPathDialogValidation keeps the dialog open when a directory is invalid
// and accepts an existing one.
func TestPathDialogValidation(t *testing.T) {
	h := newTestApp(t, 80, 24)
	accepted := ""
	h.app.openPathDialog("Choose", false, "/definitely/not/here", func(path string) { accepted = path })
	h.key(tcell.KeyEnter)
	if accepted != "" {
		t.Fatalf("invalid path accepted: %q", accepted)
	}
	if len(h.app.dialogs) != 1 {
		t.Fatal("dialog closed on validation error")
	}
	h.key(tcell.KeyEsc)
	if len(h.app.dialogs) != 0 {
		t.Fatal("dialog did not close")
	}
}

func TestDialogCancelButtons(t *testing.T) {
	h := newTestApp(t, 80, 24)

	submitted := ""
	h.app.openPrompt("Name", "", "", "", nil, func(value string) { submitted = value })
	h.key(tcell.KeyTab) // input -> Confirm
	h.key(tcell.KeyTab) // Confirm -> Cancel
	h.key(tcell.KeyEnter)
	if submitted != "" {
		t.Fatalf("cancel button submitted %q", submitted)
	}
	if len(h.app.dialogs) != 0 {
		t.Fatal("prompt did not close via cancel")
	}

	accepted := ""
	h.app.openPathDialog("Path", false, t.TempDir(), func(path string) { accepted = path })
	h.key(tcell.KeyBacktab) // input -> Confirm
	h.key(tcell.KeyBacktab) // Confirm -> Cancel
	h.key(tcell.KeyEnter)
	if accepted != "" {
		t.Fatalf("cancel button accepted %q", accepted)
	}
	if len(h.app.dialogs) != 0 {
		t.Fatal("path dialog did not close via cancel")
	}
}

func TestPathDialogTabCompletionUI(t *testing.T) {
	dir := t.TempDir()
	for _, name := range []string{"alpha", "alpine", "beta"} {
		if err := os.Mkdir(filepath.Join(dir, name), 0o755); err != nil {
			t.Fatalf("mkdir: %v", err)
		}
	}
	h := newTestApp(t, 80, 24)
	v := h.app.openPathDialog("Path", false, dir, nil)
	h.draw()
	v.input.SetText(filepath.Join(dir, "al"))

	v.complete()
	if got, want := v.input.GetText(), filepath.Join(dir, "alp"); got != want {
		t.Fatalf("first Tab = %q, want %q", got, want)
	}
	v.complete()
	text := v.list.GetText(true)
	if !strings.Contains(text, "alpha") || !strings.Contains(text, "alpine") {
		t.Fatalf("second Tab candidates missing:\n%s", text)
	}
	if len(h.app.dialogs) != 1 {
		t.Fatal("Tab completion closed the dialog")
	}
	h.key(tcell.KeyEsc)
}

func TestSettingsShowsExpandedMinecraftDir(t *testing.T) {
	h := newTestApp(t, 100, 50)
	h.app.settings.MinecraftDir = "$HOME/.minecraft"
	h.app.settingsPage().Refresh()
	home, err := os.UserHomeDir()
	if err != nil {
		t.Fatalf("home: %v", err)
	}
	view := h.app.settingsPage().texts[settingsTextMinecraftDir]
	if view == nil {
		t.Fatal("MC dir row is missing")
	}
	if got := view.GetText(true); !strings.Contains(got, filepath.Join(home, ".minecraft")) {
		t.Fatalf("MC dir text=%q", got)
	}
}

// TestPathDialogAcceptsHomeMinecraftDir covers the P5 acceptance path: a
// $HOME/.minecraft input resolves to the absolute directory and the settings
// page echoes it after selection.
func TestPathDialogAcceptsHomeMinecraftDir(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	dir := filepath.Join(home, ".minecraft")
	if err := os.Mkdir(dir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}

	h := newTestApp(t, 100, 30)
	accepted := ""
	h.app.openPathDialog("Choose", false, "$HOME/.minecraft", func(path string) { accepted = path })
	h.key(tcell.KeyEnter)
	if accepted != dir {
		t.Fatalf("accepted=%q, want %q", accepted, dir)
	}
	if len(h.app.dialogs) != 0 {
		t.Fatal("path dialog stayed open after a valid selection")
	}

	h.app.settings.MinecraftDir = accepted
	h.app.settingsPage().Refresh()
	view := h.app.settingsPage().texts[settingsTextMinecraftDir]
	if view == nil {
		t.Fatal("MC dir row is missing")
	}
	if got := view.GetText(true); !strings.Contains(got, dir) {
		t.Fatalf("settings MC dir text=%q, want %q", got, dir)
	}
}

func TestDialogBackgroundMouseIsBlocked(t *testing.T) {
	h := newTestApp(t, 80, 24)
	h.app.openConfirm("Confirm", "body", []confirmOption{{Label: "OK", Value: "ok"}}, nil)
	h.draw()

	outside := tcell.NewEventMouse(0, 0, tcell.Button1, tcell.ModNone)
	if event, _ := h.app.handleGlobalMouse(outside, tview.MouseLeftClick); event != nil {
		t.Fatal("background click was forwarded while a dialog is open")
	}
	centered, ok := h.app.topDialog().page.(*centered)
	if !ok {
		t.Fatalf("dialog page is %T", h.app.topDialog().page)
	}
	x, y, w, hgt := centered.GetRect()
	inside := tcell.NewEventMouse(x+w/2, y+hgt/2, tcell.Button1, tcell.ModNone)
	if event, _ := h.app.handleGlobalMouse(inside, tview.MouseLeftClick); event == nil {
		t.Fatal("click inside the dialog was blocked")
	}
	if len(h.app.dialogs) != 1 {
		t.Fatal("click changed the dialog stack")
	}
}

// TestGoldenPages snapshots every page across the supported sizes and both
// languages.
func TestGoldenPages(t *testing.T) {
	sizes := [][2]int{{50, 20}, {80, 24}, {100, 30}, {120, 40}, {160, 50}}
	for _, lang := range []string{i18n.LanguageEnglish, i18n.LanguageChinese} {
		for _, size := range sizes {
			h := newTestApp(t, size[0], size[1])
			i18n.Apply(lang)
			h.app.rebuildAllText()
			h.draw()
			for id := ViewID(0); id < ViewID(viewCount); id++ {
				h.switchPage(id)
				name := viewPageNames[id] + "_" + itoa(size[0]) + "x" + itoa(size[1]) + "_" + lang + ".golden"
				golden(t, name, h.text())
			}
		}
	}
}

func golden(t *testing.T, name, got string) {
	t.Helper()
	path := filepath.Join("testdata", "frames", name)
	if *updateGolden {
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatalf("mkdir golden dir: %v", err)
		}
		if err := os.WriteFile(path, []byte(got), 0o644); err != nil {
			t.Fatalf("write golden: %v", err)
		}
		return
	}
	want, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read golden %s (run with -update): %v", name, err)
	}
	if string(want) != got {
		t.Fatalf("golden mismatch for %s\n--- want ---\n%s\n--- got ---\n%s", name, want, got)
	}
}
