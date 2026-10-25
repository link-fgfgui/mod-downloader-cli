package tviewui

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gdamore/tcell/v2"

	"github.com/link-fgfgui/mod-downloader-core/models"
	"github.com/link-fgfgui/mod-downloader-core/storage"
	appstructs "github.com/link-fgfgui/mod-downloader-core/structs"
	structs "github.com/link-fgfgui/mod-downloader-core/structs/minecraft"
)

func projectFixture() models.ModProject {
	return models.ModProject{
		ID:        "modrinth:sodium",
		Platform:  "modrinth",
		ProjectID: "sodium",
		Title:     "Sodium",
	}
}

func TestShellResponsiveSidebar(t *testing.T) {
	h := newTestApp(t, 120, 40)
	if got := h.sidebarWidth(); got != 30 {
		t.Fatalf("120 columns: sidebar=%d, want 30", got)
	}
	h.mustContain("Home")

	for _, tc := range []struct{ w, want int }{
		{119, 7}, {80, 7}, {50, 7}, {49, 0}, {40, 0},
	} {
		h.sim.SetSize(tc.w, 30)
		h.draw()
		if got := h.sidebarWidth(); got != tc.want {
			t.Fatalf("%d columns: sidebar=%d, want %d", tc.w, got, tc.want)
		}
	}
}

func TestShellPagesRender(t *testing.T) {
	h := newTestApp(t, 120, 40)
	cases := []struct {
		id     ViewID
		marker string
	}{
		{ViewHome, "Application status and history"},
		{ViewDownload, "Please select a valid .minecraft directory first"},
		{ViewManage, "Local Mod Management"},
		{ViewFavorites, "Select or create a favorite list"},
		{ViewUnpin, "Pinned Mod Versions"},
		{ViewSettings, "Configure language, theme, downloads and API keys"},
	}
	for _, tc := range cases {
		h.switchPage(tc.id)
		if h.app.active != tc.id {
			t.Fatalf("active=%v, want %v", h.app.active, tc.id)
		}
		h.mustContain(tc.marker)
		if lines := h.lineCount(); lines != 40 {
			t.Fatalf("%v: %d lines, want 40", tc.id, lines)
		}
	}
}

func TestShellDigitNavigation(t *testing.T) {
	h := newTestApp(t, 80, 24)
	h.rune('3')
	if h.app.active != ViewManage {
		t.Fatalf("active=%v, want manage", h.app.active)
	}
	h.mustContain("Local Mod Management")
	h.rune('5')
	if h.app.active != ViewUnpin {
		t.Fatalf("active=%v, want unpin", h.app.active)
	}
	h.mustContain("Pinned Mod Versions")
	h.rune('1')
	if h.app.active != ViewHome {
		t.Fatalf("active=%v, want home", h.app.active)
	}
}

func TestDigitsTypeInInputInsteadOfNavigating(t *testing.T) {
	h := newTestApp(t, 80, 24)
	h.switchPage(ViewDownload)
	h.rune('/')
	h.rune('3')
	if h.app.active != ViewDownload {
		t.Fatalf("digit navigated away from a focused input: active=%v", h.app.active)
	}
	if got := h.app.downloadPage().search.GetText(); got != "3" {
		t.Fatalf("input text=%q, want 3", got)
	}
}

// A closed DropDown inside the settings form must not swallow global digits.
func TestSettingsDigitsNavigateWhenDropdownClosed(t *testing.T) {
	h := newTestApp(t, 80, 24)
	h.switchPage(ViewSettings)
	h.rune('3')
	if h.app.active != ViewManage {
		t.Fatalf("active=%v, want manage", h.app.active)
	}
}

func TestTabMovesBetweenSearchAndList(t *testing.T) {
	h := newTestApp(t, 80, 24)
	h.switchPage(ViewDownload)
	h.rune('/')
	if got := h.focus(); got != h.app.downloadPage().search {
		t.Fatalf("focus=%T, want search", got)
	}
	h.key(tcell.KeyTab)
	if got := h.focus(); got != h.app.downloadPage().results {
		t.Fatalf("focus=%T, want results", got)
	}
	h.key(tcell.KeyTab)
	if got := h.focus(); got != h.app.downloadPage().search {
		t.Fatalf("focus=%T, want search", got)
	}
}

func TestDownloadMultiSelectAndScrolling(t *testing.T) {
	h := newTestApp(t, 100, 30)
	h.app.mc.SelectedVersion = &structs.VersionInfo{
		ID:               "inst",
		Name:             "inst",
		MinecraftVersion: "1.20.1",
		ModLoader:        "Fabric",
	}
	results := make([]models.ModProject, 30)
	for i := range results {
		results[i] = models.ModProject{
			ID:        "modrinth:mod-" + itoa(i),
			Platform:  "modrinth",
			ProjectID: "mod-" + itoa(i),
			Title:     "Mod " + itoa(i),
		}
	}
	h.app.search.SearchResults = results
	page := h.app.downloadPage()
	page.Refresh()
	h.switchPage(ViewDownload)

	h.rune(' ')
	if len(page.selected) != 1 {
		t.Fatalf("selected=%d, want 1", len(page.selected))
	}
	h.key(tcell.KeyCtrlA)
	if len(page.selected) != len(results) {
		t.Fatalf("selected=%d, want %d", len(page.selected), len(results))
	}
	h.key(tcell.KeyEnd)
	if got := page.results.GetCurrentItem(); got != len(results)-1 {
		t.Fatalf("current row=%d, want %d", got, len(results)-1)
	}
	h.key(tcell.KeyEsc)
	if len(page.selected) != 0 {
		t.Fatalf("Esc left %d rows selected", len(page.selected))
	}
}

func TestManageIncompatibleToggle(t *testing.T) {
	h := newTestApp(t, 120, 40)
	h.app.mc.SelectedModLoader = "Fabric"
	h.app.mc.SelectedVersion = &structs.VersionInfo{
		ID:               "inst",
		Name:             "inst",
		MinecraftVersion: "1.20.1",
		ModLoader:        "Fabric",
		Mods: []structs.ModInfo{
			{ID: "fabric-mod", Name: "Fabric Mod", FileName: "fabric-mod.jar", Path: "/mods/fabric-mod.jar", Enabled: true, Loaders: []string{"fabric"}},
			{ID: "forge-mod", Name: "Forge Mod", FileName: "forge-mod.jar", Path: "/mods/forge-mod.jar", Enabled: true, Loaders: []string{"forge"}},
		},
	}
	h.switchPage(ViewManage)
	h.mustContain("Incompatible with current loader (1)")
	// Collapsed by default: the incompatible row is hidden.
	h.mustNotContain("Forge Mod")
	h.key(tcell.KeyDown)  // compatible row -> toggle row
	h.key(tcell.KeyEnter) // expand
	h.draw()
	h.mustContain("Forge Mod")
	h.key(tcell.KeyEnter) // collapse
	h.draw()
	h.mustNotContain("Forge Mod")
}

func TestShellZhongWenRendering(t *testing.T) {
	h := newTestApp(t, 120, 40)
	h.setChinese()
	h.mustContain("应用状态与历史统计")
	h.switchPage(ViewSettings)
	h.mustContain("设置")
	h.mustContain("语言")
	h.setEnglish()
	h.mustContain("Settings")
}

// Static widget titles must follow a language switch, not only dynamic text.
func TestLanguageSwitchRebuildsStaticTitles(t *testing.T) {
	h := newTestApp(t, 120, 40)
	h.setChinese()
	h.switchPage(ViewFavorites)
	h.mustContain("收藏夹")
	h.mustNotContain("Lists")
	h.setEnglish()
	h.mustContain("Lists")
	h.switchPage(ViewHome)
	h.mustContain("Application status and history")
	h.setChinese()
	h.switchPage(ViewManage)
	h.mustContain("本地 Mod 管理")
	h.mustNotContain("Local Mod Management")
}

func TestShellGlobalKeysAndDialogs(t *testing.T) {
	h := newTestApp(t, 80, 24)
	h.rune('?')
	h.mustContain("Keyboard shortcuts")
	if len(h.app.dialogs) != 1 {
		t.Fatalf("dialogs=%d, want 1", len(h.app.dialogs))
	}
	// Digits must not switch views while a dialog is open.
	h.rune('3')
	if h.app.active != ViewHome {
		t.Fatalf("dialog did not capture digit: active=%v", h.app.active)
	}
	h.key(tcell.KeyEsc)
	h.mustNotContain("Keyboard shortcuts")
	if len(h.app.dialogs) != 0 {
		t.Fatalf("dialogs=%d, want 0", len(h.app.dialogs))
	}

	h.rune('t')
	h.mustContain("Instance / Target")
	h.mustContain("Minecraft Version")
	h.key(tcell.KeyEsc)
	if len(h.app.dialogs) != 0 {
		t.Fatalf("target dialog did not close")
	}
}

func TestShellConfirmPromptAndPathDialogs(t *testing.T) {
	h := newTestApp(t, 80, 24)

	chosen := ""
	h.app.openConfirm("Replace?", strings.Repeat("long body line\n", 30), []confirmOption{
		{Label: "Replace", Value: "confirm", Primary: true},
		{Label: "Cancel", Value: "cancel"},
	}, func(value string) { chosen = value })
	h.draw()
	h.mustContain("Replace?")
	h.key(tcell.KeyEnter)
	if chosen != "confirm" {
		t.Fatalf("confirm value=%q", chosen)
	}

	got := ""
	h.app.openPrompt("New name", "hint", "", "name", nil, func(value string) { got = value })
	h.draw()
	h.mustContain("New name")
	for _, r := range "hello" {
		h.rune(r)
	}
	h.key(tcell.KeyEnter)
	if got != "hello" {
		t.Fatalf("prompt value=%q", got)
	}

	dir := t.TempDir()
	accepted := ""
	h.app.openPathDialog("Choose directory", false, dir, func(path string) { accepted = path })
	h.draw()
	h.mustContain("Choose directory")
	h.mustContain(dir)
	h.key(tcell.KeyEnter)
	if accepted != dir {
		t.Fatalf("path=%q, want %q", accepted, dir)
	}
}

func TestPathCompletion(t *testing.T) {
	dir := t.TempDir()
	for _, name := range []string{"alpha", "alpine", "beta"} {
		if err := os.Mkdir(filepath.Join(dir, name), 0o755); err != nil {
			t.Fatalf("mkdir: %v", err)
		}
	}
	base := filepath.Join(dir, "al")
	got := completePath(base, false)
	if len(got) != 2 {
		t.Fatalf("completions=%v, want 2", got)
	}
	if prefix := longestCommonPrefix(got); prefix != filepath.Join(dir, "alp") {
		t.Fatalf("prefix=%q", prefix)
	}
	if files := completePath(filepath.Join(dir, "missing"), false); len(files) != 0 {
		t.Fatalf("missing dir completions=%v", files)
	}
}

func TestQueueDock(t *testing.T) {
	h := newTestApp(t, 120, 40)
	state := appstructs.DownloadQueueState{
		Active:  true,
		Running: 1,
		Pending: 1,
		Items: []appstructs.DownloadQueueItem{
			{ID: "a", Status: "running", Title: "Sodium", Platform: "modrinth", TotalBytes: 100, BytesComplete: 50},
			{ID: "b", Status: "pending", Title: "Lithium", Platform: "modrinth"},
		},
	}
	h.app.applyQueue(state)
	h.draw()
	if !h.app.queue.Visible() {
		t.Fatal("queue dock did not auto-open")
	}
	h.mustContain("Download Queue")
	h.mustContain("Sodium")
	h.mustContain("50%")
	if w := h.app.queue.Width(); w < 28 || w > 40 {
		t.Fatalf("dock width=%d, want 28..40", w)
	}
	h.rune('q')
	h.draw()
	if h.app.queue.Visible() {
		t.Fatal("q did not close the queue dock")
	}

	h.app.applyQueue(appstructs.DownloadQueueState{})
	h.draw()
	if h.app.queue.Visible() {
		t.Fatal("queue stayed open after the cycle drained")
	}
}

// TestQueueDockWidthResponsive checks the dock clamp and that opening it never
// pushes the main area off-screen at any required terminal width.
func TestQueueDockWidthResponsive(t *testing.T) {
	for _, width := range []int{50, 80, 120, 160} {
		h := newTestApp(t, width, 30)
		h.app.applyQueue(appstructs.DownloadQueueState{
			Active:  true,
			Running: 1,
			Items:   []appstructs.DownloadQueueItem{{ID: "a", Status: "running", Title: "Sodium"}},
		})
		h.draw()
		if !h.app.queue.Visible() {
			t.Fatalf("%d columns: queue did not open", width)
		}
		if got := h.app.queue.Width(); got < 28 || got > 40 {
			t.Fatalf("%d columns: dock width=%d, want 28..40", width, got)
		}
		if got := h.app.mainWidth(); got <= 0 {
			t.Fatalf("%d columns: main width=%d", width, got)
		}
		assertFrameBounds(t, h, width, 30)
	}
}

func TestToastStatusBar(t *testing.T) {
	h := newTestApp(t, 100, 30)
	h.app.toast("Saved to disk")
	h.draw()
	h.mustContain("Saved to disk")
	if h.app.toastTimer == nil {
		t.Fatal("toast did not schedule its expiry")
	}
	h.app.toastTimer.Stop()
	h.app.toastText = ""
	h.app.updateStatus()
	h.draw()
	h.mustNotContain("Saved to disk")
}

func TestQueueFocusRestoration(t *testing.T) {
	h := newTestApp(t, 120, 40)
	state := appstructs.DownloadQueueState{
		Active:  true,
		Running: 1,
		Items:   []appstructs.DownloadQueueItem{{ID: "a", Status: "running", Title: "Sodium"}},
	}
	h.app.applyQueue(state)
	h.draw()
	if !h.app.queue.Visible() {
		t.Fatal("queue did not auto-open")
	}
	if h.app.queue.Focused() {
		t.Fatal("auto-open stole focus")
	}
	if got := h.focus(); got != h.app.homePage().view {
		t.Fatalf("focus=%T, want home view", got)
	}

	h.rune('q') // close the auto-opened dock
	if h.app.queue.Visible() {
		t.Fatal("q did not close the auto-opened dock")
	}
	h.rune('q') // open and focus the dock
	if !h.app.queue.Visible() || !h.app.queue.Focused() {
		t.Fatal("q did not open and focus the dock")
	}
	h.rune('q') // close and restore page focus
	if h.app.queue.Visible() {
		t.Fatal("q did not close the dock")
	}
	if got := h.focus(); got != h.app.homePage().view {
		t.Fatalf("focus=%T, want home view after close", got)
	}
}

func TestQueueDockOptionalTab(t *testing.T) {
	h := newTestApp(t, 100, 30)
	state := appstructs.DownloadQueueState{
		Active: false,
		OptionalReminders: []appstructs.OptionalDependencyReminder{
			{ID: "r1", MainTitle: "Create", Dependencies: []appstructs.OptionalDependencyCandidate{{ProjectID: "jei", Platform: "curseforge"}}},
		},
	}
	h.app.applyQueue(state)
	h.draw()
	if !h.app.queue.Visible() {
		// Reminder-only queues do not auto-open; the capsule does.
		h.rune('q')
		h.draw()
	}
	if !h.app.queue.Visible() {
		t.Fatal("optional reminders did not open the dock")
	}
	if h.app.queueState.ActiveTab != "optional" {
		t.Fatalf("tab=%s, want optional", h.app.queueState.ActiveTab)
	}
	h.mustContain("Create")
}

func TestQueueActionKeysAndTabs(t *testing.T) {
	h := newTestApp(t, 120, 40)
	h.app.applyQueue(appstructs.DownloadQueueState{
		Active:  true,
		Running: 1,
		Items: []appstructs.DownloadQueueItem{
			{ID: "a", Status: "pending", Title: "Sodium", Cancelable: true, Retryable: true},
		},
	})
	h.draw()
	h.app.queue.Focus()
	for _, key := range []rune{'c', 'r', 'x'} {
		h.rune(key)
	}
	if !h.app.queue.Visible() {
		t.Fatal("an item action closed the dock")
	}
	h.key(tcell.KeyTab)
	if got := h.app.queueState.ActiveTab; got != "optional" {
		t.Fatalf("tab=%q, want optional", got)
	}
	h.key(tcell.KeyTab)
	if got := h.app.queueState.ActiveTab; got != "downloads" {
		t.Fatalf("tab=%q, want downloads", got)
	}
}

func TestDialogStackEscClosesTopOnly(t *testing.T) {
	h := newTestApp(t, 80, 24)
	h.app.applyQueue(appstructs.DownloadQueueState{
		Active:  true,
		Running: 1,
		Items:   []appstructs.DownloadQueueItem{{ID: "a", Status: "running", Title: "Sodium"}},
	})
	h.app.closeQueue()
	h.draw()

	h.app.openConfirm("First", "body", []confirmOption{{Label: "OK", Value: "ok"}}, nil)
	h.app.openPrompt("Second", "hint", "", "name", nil, nil)
	h.draw()
	h.mustContain("Second")
	h.rune('q') // a dialog owns the keyboard; global shortcuts must not fire
	if h.app.queue.Visible() {
		t.Fatal("q leaked through the dialog to the queue dock")
	}
	h.key(tcell.KeyEsc)
	if len(h.app.dialogs) != 1 {
		t.Fatalf("dialog stack=%d, want 1", len(h.app.dialogs))
	}
	h.mustContain("First")
	h.key(tcell.KeyEsc)
	if len(h.app.dialogs) != 0 {
		t.Fatalf("dialog stack=%d, want 0", len(h.app.dialogs))
	}
}

func TestUnpinTableRows(t *testing.T) {
	h := newTestApp(t, 100, 30)
	page := h.app.views[ViewUnpin].(*unpinPage)
	page.setPins([]storage.PinnedMod{{
		Platform:         "modrinth",
		ModID:            "sodium",
		VersionID:        "ver-1",
		MinecraftVersion: "1.20.1",
		ModLoader:        "Fabric",
	}})
	h.switchPage(ViewUnpin)
	h.mustContain("modrinth")
	h.mustContain("sodium")
	h.mustContain("ver-1")
	if rows := page.table.GetRowCount(); rows != 2 {
		t.Fatalf("table rows=%d, want header + 1", rows)
	}
	h.rune('u') // offline controller must no-op instead of dereferencing a nil core
}

func TestTargetDialogCycleLeftRight(t *testing.T) {
	h := newTestApp(t, 100, 30)
	h.app.mc.SelectedMinecraftVersion = "1.20.1"
	h.app.mc.ReleaseVersions = []string{"1.20.1", "1.20.4", "1.21.1"}
	h.app.mc.SelectedModLoader = "Fabric"
	h.app.openTargetDialog()
	h.app.targetDialog.list.SetCurrentItem(4) // MC version row

	h.key(tcell.KeyRight)
	if got := h.app.mc.SelectedMinecraftVersion; got != "1.20.4" {
		t.Fatalf("MC version=%q, want 1.20.4", got)
	}
	h.key(tcell.KeyLeft)
	if got := h.app.mc.SelectedMinecraftVersion; got != "1.20.1" {
		t.Fatalf("MC version=%q, want 1.20.1", got)
	}
	h.app.targetDialog.list.SetCurrentItem(5) // loader row
	h.key(tcell.KeyRight)
	if got := h.app.mc.SelectedModLoader; got != "Forge" {
		t.Fatalf("loader=%q, want Forge", got)
	}
	h.key(tcell.KeyEsc)
}

func TestVersionDialog(t *testing.T) {
	h := newTestApp(t, 100, 30)
	h.app.openVersionDialog(
		projectFixture(),
		"pin",
		func(version models.ModVersion) {},
		nil,
	)
	h.draw()
	h.mustContain("Loading")
	h.key(tcell.KeyEsc)
	if len(h.app.dialogs) != 0 {
		t.Fatalf("version dialog did not close")
	}
}

func TestSettingsSimpleModeReflectsState(t *testing.T) {
	h := newTestApp(t, 100, 50)
	page := h.app.settingsPage()
	h.app.settings.SimpleMode = true
	page.Refresh()
	if !page.simpleMode.IsChecked() {
		t.Fatal("simple mode checkbox did not follow settings")
	}
	page.simpleMode.SetChecked(false)
	if page.simpleMode.IsChecked() {
		t.Fatal("simple mode checkbox did not toggle")
	}
}

func TestSettingsFormOrder(t *testing.T) {
	h := newTestApp(t, 100, 50)
	h.switchPage(ViewSettings)
	text := h.text()
	order := []string{
		"Language",
		"Theme",
		"Release variant",
		"Simple mode",
		"Cleanup",
		"Cache Directory",
		"MCIM",
		"Chunks/file",
		"Concurrent",
	}
	last := -1
	for _, label := range order {
		idx := strings.Index(text, label)
		if idx < 0 {
			t.Fatalf("settings label %q missing:\n%s", label, text)
		}
		if idx < last {
			t.Fatalf("settings label %q out of order", label)
		}
		last = idx
	}
}
