package tviewui

import (
	"strings"
	"testing"

	"github.com/gdamore/tcell/v2"

	"github.com/link-fgfgui/mod-downloader-core/models"
	appstructs "github.com/link-fgfgui/mod-downloader-core/structs"
	structs "github.com/link-fgfgui/mod-downloader-core/structs/minecraft"

	"github.com/link-fgfgui/mod-downloader-cli/internal/i18n"
	"github.com/link-fgfgui/mod-downloader-cli/internal/uistate"
)

func TestShellLayoutDelegates(t *testing.T) {
	h := newTestApp(t, 100, 30)
	shell := h.app.shell
	if _, _, w, hgt := shell.GetRect(); w != 100 || hgt != 30 {
		t.Fatalf("shell rect=%dx%d, want 100x30", w, hgt)
	}
	shell.SetRect(1, 2, 50, 20)
	if x, y, w, hgt := shell.GetRect(); x != 1 || y != 2 || w != 50 || hgt != 20 {
		t.Fatalf("shell rect after SetRect=%d,%d,%dx%d", x, y, w, hgt)
	}
	shell.SetRect(0, 0, 100, 30)
	if shell.InputHandler() == nil {
		t.Fatal("shell InputHandler is nil")
	}
	if shell.PasteHandler() == nil {
		t.Fatal("shell PasteHandler is nil")
	}
	if shell.MouseHandler() == nil {
		t.Fatal("shell MouseHandler is nil")
	}
	shell.Blur() // must not panic even when the shell has no single focused child

}

func TestBreakpointsAndSidebarWidth(t *testing.T) {
	tests := []struct {
		width int
		bp    Breakpoint
		bar   int
	}{
		{0, BPCompact, 0},
		{49, BPCompact, 0},
		{50, BPRail, 7},
		{119, BPRail, 7},
		{120, BPFull, 30},
		{200, BPFull, 30},
	}
	for _, tt := range tests {
		if got := breakpointFor(tt.width); got != tt.bp {
			t.Fatalf("breakpointFor(%d)=%v, want %v", tt.width, got, tt.bp)
		}
		if got := sidebarWidth(tt.width); got != tt.bar {
			t.Fatalf("sidebarWidth(%d)=%d, want %d", tt.width, got, tt.bar)
		}
	}
}

func TestMainWidthWithAndWithoutQueue(t *testing.T) {
	h := newTestApp(t, 100, 30)
	h.app.width = 100
	if got, want := h.app.mainWidth(), 100-sidebarWidth(100); got != want {
		t.Fatalf("mainWidth=%d, want %d", got, want)
	}
	h.app.applyQueue(appstructs.DownloadQueueState{
		Active:  true,
		Running: 1,
		Items:   []appstructs.DownloadQueueItem{{ID: "a", Status: "running", Title: "Sodium"}},
	})
	h.draw()
	if got, want := h.app.mainWidth(), 100-sidebarWidth(100)-h.app.queue.Width(); got != want {
		t.Fatalf("mainWidth with dock=%d, want %d", got, want)
	}
	// The main area never collapses to zero even on a tiny terminal.
	h2 := newTestApp(t, 28, 20)
	h2.app.width = 28
	h2.app.queue.SetOpen(true)
	h2.app.resizeQueue()
	if got := h2.app.mainWidth(); got < 1 {
		t.Fatalf("mainWidth on tiny terminal=%d, want >=1", got)
	}
}

func TestConnectorLabelAndAvailability(t *testing.T) {
	i18n.Apply(i18n.LanguageEnglish)
	h := newTestApp(t, 100, 30)
	if got := h.app.connectorLabel(); got != "" {
		t.Fatalf("nil version connectorLabel=%q", got)
	}
	h.app.mc.SelectedVersion = &structs.VersionInfo{ActualModLoader: "Fabric"}
	if got, want := h.app.connectorLabel(), i18n.T("sidebar.connector.switchTo", map[string]string{"loader": "Fabric"}); got != want {
		t.Fatalf("non-virtual connectorLabel=%q, want %q", got, want)
	}
	h.app.mc.SelectedVersion = &structs.VersionInfo{ActualModLoader: "NeoForge", ConnectorVirtual: true}
	if got, want := h.app.connectorLabel(), i18n.T("sidebar.connector.switchTo", map[string]string{"loader": "NeoForge"}); got != want {
		t.Fatalf("virtual connectorLabel=%q, want %q", got, want)
	}
	if h.app.selectedConnectorAvailable() {
		t.Fatal("connector should be unavailable when ConnectorAvailable is false")
	}
	h.app.mc.SelectedVersion.ConnectorAvailable = true
	if !h.app.selectedConnectorAvailable() {
		t.Fatal("connector should be available")
	}
}

func TestTargetTuplePlaceholders(t *testing.T) {
	h := newTestApp(t, 100, 30)
	if got := h.app.targetTuple(); got != "— | Fabric" {
		t.Fatalf("empty tuple=%q", got)
	}
	h.app.mc.SelectedMinecraftVersion = "1.20.1"
	h.app.mc.SelectedModLoader = "Forge"
	if got := h.app.targetTuple(); got != "1.20.1 | Forge" {
		t.Fatalf("tuple=%q", got)
	}
}

func TestQueueCapsuleFormatting(t *testing.T) {
	tests := []struct {
		name string
		q    uistate.QueueState
		want string
	}{
		{"empty", uistate.NewQueueState(), "↓ 0/0"},
		{"running", uistate.QueueState{Queue: appstructs.DownloadQueueState{Active: true, Running: 2, Pending: 3}}, "↓ 2/3"},
		{
			name: "progress and speed",
			q: uistate.QueueState{Queue: appstructs.DownloadQueueState{
				Active: true, Running: 1, Pending: 1,
				TotalBytes: 100, BytesComplete: 50, BytesPerSecond: 1024 * 1024,
			}},
			want: "↓ 1/1 50% 1.00 MiB/s",
		},
		{
			name: "reminder count",
			q: uistate.QueueState{Queue: appstructs.DownloadQueueState{
				Active: true, Running: 1, MessageCount: 2,
			}},
			want: "↓ 1/0 🔔2",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := queueCapsule(tt.q); got != tt.want {
				t.Fatalf("queueCapsule=%q, want %q", got, tt.want)
			}
		})
	}
}

func TestBusyStatus(t *testing.T) {
	h := newTestApp(t, 100, 30)
	if got := h.app.busyStatus(); got != "" {
		t.Fatalf("idle busyStatus=%q", got)
	}
	h.app.beginBusy("Loading test")
	if got := h.app.busyStatus(); !strings.Contains(got, "Loading test") {
		t.Fatalf("busyStatus=%q", got)
	}
	h.app.busyMu.Lock()
	h.app.busyFrame = 2
	h.app.busyMu.Unlock()
	if got := h.app.busyStatus(); !strings.HasPrefix(got, spinnerFrames[2]) {
		t.Fatalf("busyStatus frame=%q, want prefix %q", got, spinnerFrames[2])
	}
	h.app.endBusy()
	if got := h.app.busyStatus(); got != "" {
		t.Fatalf("busyStatus after end=%q", got)
	}
	h.app.beginBusy("")
	if got := h.app.busyStatus(); !strings.Contains(got, uiText("loading")) {
		t.Fatalf("default busy label=%q", got)
	}
	h.app.endBusy()
}

func TestToastSupportsTranslationAndIgnoresBlank(t *testing.T) {
	h := newTestApp(t, 100, 30)
	h.app.toast("   ")
	if h.app.toastText != "" {
		t.Fatalf("blank toast set text %q", h.app.toastText)
	}
	h.app.toastT("download.queued")
	if h.app.toastText != i18n.T("download.queued") {
		t.Fatalf("toastT text=%q", h.app.toastText)
	}
	if h.app.toastTimer != nil {
		h.app.toastTimer.Stop()
	}
}

func TestGlobalEscapeOrder(t *testing.T) {
	h := newTestApp(t, 120, 40)
	h.app.applyQueue(appstructs.DownloadQueueState{
		Active:  true,
		Running: 1,
		Items:   []appstructs.DownloadQueueItem{{ID: "a", Status: "running", Title: "Sodium"}},
	})
	h.draw()
	if !h.app.queue.Visible() {
		t.Fatal("queue did not open")
	}
	// 1. A visible dock closes first.
	h.key(tcell.KeyEsc)
	if h.app.queue.Visible() {
		t.Fatal("Esc did not close the queue dock first")
	}
	// 2. Then a page selection/input focus is cleared.
	h.switchPage(ViewDownload)
	h.rune('/')
	h.key(tcell.KeyEsc)
	if h.focus() == h.app.downloadPage().search {
		t.Fatal("Esc did not clear the search focus")
	}
	// 3. Then the page returns home.
	h.switchPage(ViewManage)
	h.key(tcell.KeyEsc)
	if h.app.active != ViewHome {
		t.Fatalf("Esc did not return home, active=%v", h.app.active)
	}
}

func TestSwitchViewClosesQueueAndFocusesPage(t *testing.T) {
	h := newTestApp(t, 120, 40)
	h.app.applyQueue(appstructs.DownloadQueueState{
		Active:  true,
		Running: 1,
		Items:   []appstructs.DownloadQueueItem{{ID: "a", Status: "running", Title: "Sodium"}},
	})
	h.draw()
	if !h.app.queue.Visible() {
		t.Fatal("queue did not open")
	}
	h.switchPage(ViewUnpin)
	if h.app.queue.Visible() {
		t.Fatal("switchView left the queue dock open")
	}
	if h.app.active != ViewUnpin {
		t.Fatalf("active=%v", h.app.active)
	}
	if h.focus() != h.app.views[ViewUnpin].(*unpinPage).table {
		t.Fatalf("focus=%T, want unpin table", h.focus())
	}
}

func TestTextInputFocusAndDropdownState(t *testing.T) {
	h := newTestApp(t, 100, 40)
	h.switchPage(ViewDownload)
	if h.app.textInputFocused() {
		t.Fatal("no input focused yet")
	}
	h.rune('/')
	if !h.app.textInputFocused() {
		t.Fatal("search field should count as a text input")
	}
	if h.app.dropdownOpen() {
		t.Fatal("search field reported as an open dropdown")
	}

	h.switchPage(ViewSettings)
	page := h.app.settingsPage()
	page.form.SetFocus(page.languageIdx)
	h.app.application.SetFocus(page.form)
	if h.app.textInputFocused() {
		t.Fatal("a closed dropdown must not own character keys")
	}
	if h.app.dropdownOpen() {
		t.Fatal("dropdown should start closed")
	}
	// Enter hands focus to tview's internal dropdown list. Global shortcuts must
	// not fire while that list is open; Esc returns to the form.
	h.key(tcell.KeyEnter)
	if !page.language.IsOpen() {
		t.Skip("tview did not open the dropdown through the form input handler")
	}
	h.key(tcell.KeyEsc)
	if h.app.active != ViewSettings {
		t.Fatalf("Esc on an open dropdown navigated away: active=%v", h.app.active)
	}
}

func TestApplySettingsRebuildsThemeAndLanguage(t *testing.T) {
	h := newTestApp(t, 120, 40)
	oldLanguage := h.app.settingsPage().language
	settings := h.app.settings
	settings.Theme = ThemeLight
	settings.Language = i18n.LanguageChinese
	h.app.applySettings(settings)
	if h.app.theme.Name != ThemeLight {
		t.Fatalf("theme=%q", h.app.theme.Name)
	}
	if i18n.Current() != i18n.LanguageChinese {
		t.Fatalf("language=%q", i18n.Current())
	}
	if h.app.settingsPage().language == oldLanguage {
		t.Fatal("language switch did not rebuild the settings form widgets")
	}
	h.switchPage(ViewSettings)
	h.mustContain("设置")
	i18n.Apply(i18n.LanguageEnglish)
}

func TestRebuildSidebarLabelsPerBreakpoint(t *testing.T) {
	h := newTestApp(t, 80, 24)
	h.draw()
	if h.app.bp != BPRail {
		t.Fatalf("bp=%v, want rail", h.app.bp)
	}
	if h.app.sidebar.GetItemCount() != viewCount {
		t.Fatalf("rail items=%d", h.app.sidebar.GetItemCount())
	}
	main, _ := h.app.sidebar.GetItemText(0)
	if strings.Contains(main, "Home") {
		t.Fatalf("rail should be icon-only, got %q", main)
	}
	h.sim.SetSize(120, 30)
	h.draw()
	if h.app.bp != BPFull {
		t.Fatalf("bp=%v, want full", h.app.bp)
	}
	main, _ = h.app.sidebar.GetItemText(0)
	if !strings.Contains(main, "Home") {
		t.Fatalf("full sidebar should include the label, got %q", main)
	}
	h.sim.SetSize(40, 20)
	h.draw()
	if h.app.bp != BPCompact {
		t.Fatalf("bp=%v, want compact", h.app.bp)
	}
	if _, _, width, _ := h.app.sidebarCol.GetRect(); width != 0 {
		t.Fatalf("compact sidebar width=%d, want 0", width)
	}
}

func TestSidebarFooterStates(t *testing.T) {
	h := newTestApp(t, 120, 40)
	h.app.updateSidebarFooter()
	text := h.app.sidebarFoot.GetText(true)
	if !strings.Contains(text, i18n.T("home.status.notConfigured")) {
		t.Fatalf("empty footer=%q", text)
	}
	h.app.mc.SelectedVersion = &structs.VersionInfo{ID: "inst", Name: "Inst", MinecraftVersion: "1.20.1", ModLoader: "Fabric", ConnectorAvailable: true}
	h.app.mc.SelectedMinecraftVersion = "1.20.1"
	h.app.mc.SelectedModLoader = "Fabric"
	h.app.mc.MinecraftDir = "/tmp/.minecraft"
	h.app.updateSidebarFooter()
	text = h.app.sidebarFoot.GetText(true)
	for _, want := range []string{"Inst", "1.20.1 | Fabric", "/tmp/.minecraft", i18n.T("sidebar.connector.switchTo", map[string]string{"loader": "Fabric"})} {
		if !strings.Contains(text, want) {
			t.Fatalf("footer missing %q: %q", want, text)
		}
	}
}

func TestBeforeDrawSizeIsIdempotent(t *testing.T) {
	h := newTestApp(t, 100, 30)
	h.app.beforeDrawSize(90, 25)
	w, hgt := h.app.width, h.app.height
	h.app.beforeDrawSize(90, 25)
	if h.app.width != w || h.app.height != hgt {
		t.Fatalf("size changed on identical call: %dx%d", h.app.width, h.app.height)
	}
	h.app.beforeDrawSize(130, 40)
	if h.app.bp != BPFull {
		t.Fatalf("breakpoint=%v after widening", h.app.bp)
	}
}

func TestStartupNoticeAndAfterStartHelpers(t *testing.T) {
	// applySelectedVersion must update search/favorites scope and sidebar.
	h := newTestApp(t, 120, 40)
	h.app.mc.MinecraftDir = "/tmp/.minecraft"
	h.app.applySelectedVersion(structs.VersionInfo{
		ID: "inst-1", Name: "Instance", MinecraftVersion: "1.20.1", ModLoader: "fabric",
	}, true)
	if got := h.app.search.SelectedVersion; got != "1.20.1" {
		t.Fatalf("search version=%q", got)
	}
	if got := h.app.search.SelectedModLoader; got != "Fabric" {
		t.Fatalf("search loader=%q", got)
	}
	if !h.app.search.HasSelectedInstance {
		t.Fatal("search did not record selected instance")
	}
	if got := h.app.favoritesPage().mcVersion; got != "1.20.1" {
		t.Fatalf("favorites scope=%q", got)
	}
	if h.app.mc.EnsureSelectedModsLoadedNeeded() {
		t.Fatal("applySelectedVersion(markLoaded=true) left a refresh pending")
	}
}

// TestApplyQueueAutoOpenAndClose covers the dock auto-open lifecycle and the
// auto-close rule when a user-opened dock drains.
func TestApplyQueueAutoOpenAndClose(t *testing.T) {
	h := newTestApp(t, 120, 40)
	h.app.applyQueue(appstructs.DownloadQueueState{
		Active:  true,
		Running: 1,
		Items:   []appstructs.DownloadQueueItem{{ID: "a", Status: "running", Title: "Sodium"}},
	})
	if !h.app.queue.Visible() || !h.app.queueAutoOpened {
		t.Fatal("active queue did not auto-open")
	}
	h.app.applyQueue(appstructs.DownloadQueueState{})
	if h.app.queue.Visible() {
		t.Fatal("drained auto-opened dock stayed open")
	}
	// A user-opened dock stays open after the cycle drains.
	h.app.applyQueue(appstructs.DownloadQueueState{
		Active:  true,
		Running: 1,
		Items:   []appstructs.DownloadQueueItem{{ID: "a", Status: "running", Title: "Sodium"}},
	})
	h.app.closeQueue()
	h.rune('q')
	if !h.app.queue.Visible() {
		t.Fatal("q did not open the dock")
	}
	h.app.applyQueue(appstructs.DownloadQueueState{})
	if !h.app.queue.Visible() {
		t.Fatal("user-opened dock was auto-closed")
	}
}

func TestSearchModsAndQueueDownloadOfflineAreNoOps(t *testing.T) {
	h := newTestApp(t, 100, 30)
	h.app.searchMods(appstructs.SearchModsRequest{RequestID: "r", Query: "sodium"})
	h.app.queueDownload(appstructs.ModDownloadRequest{ProjectID: "modrinth:sodium"})
	h.app.cancelDownload("x")
	h.app.retryDownload("x")
	h.app.removeDownload("x")
	h.app.dismissOptionalReminder("x")
	h.app.clearOptionalReminders()
	h.app.installOptionalDependencies("x")
	if h.app.search.IsSearching {
		t.Fatal("offline searchMods changed the search state")
	}
	if h.app.toastText != "" {
		t.Fatalf("offline queueDownload showed a toast: %q", h.app.toastText)
	}
}

func TestDownloadErrorI18nMapping(t *testing.T) {
	h := newTestApp(t, 100, 30)
	for reason, key := range map[string]string{
		"invalid request":      "download.errors.invalidRequest",
		"no matching version":  "download.errors.noMatchingVersion",
		"missing download url": "download.errors.missingDownloadUrl",
		"no selected version":  "download.errors.noSelectedVersion",
		"something else":       "download.errors.generic",
	} {
		h.app.toastDownloadSkip(reason)
		if got, want := h.app.toastText, i18n.T(key); got != want {
			t.Fatalf("reason %q toast=%q, want %q", reason, got, want)
		}
		if h.app.toastTimer != nil {
			h.app.toastTimer.Stop()
		}
	}
}

func TestQueuePreviewHelpers(t *testing.T) {
	h := newTestApp(t, 100, 30)
	h.app.queueState.Queue.Items = []appstructs.DownloadQueueItem{
		{ID: "a", ProjectID: "Sodium", Status: "running", Title: "Sodium"},
		{ID: "b", ProjectID: "lithium", Status: "pending", Title: "Lithium"},
	}
	h.app.applyQueue(h.app.queueState.Queue)
	page := h.app.downloadPage()
	state := page.stateFor("missing")
	if state.Status != "" {
		t.Fatalf("unknown state=%+v", state)
	}
	idx := page.buildRowIndex()
	if item, ok := idx.queueItemFor(projectFixture()); !ok || item.ID != "a" {
		t.Fatalf("running item lookup=%+v ok=%v", item, ok)
	}
	if item, ok := idx.queueItemFor(models.ModProject{ID: "lithium", Platform: "modrinth", ProjectID: "lithium"}); !ok || item.ID != "b" {
		t.Fatalf("pending item lookup=%+v ok=%v", item, ok)
	}
	label := downloadProgressLabel(h.app.theme, appstructs.DownloadQueueItem{TotalBytes: 100, BytesComplete: 25})
	if !strings.Contains(label, "25%") {
		t.Fatalf("progress label=%q", label)
	}
	if label := downloadProgressLabel(h.app.theme, appstructs.DownloadQueueItem{}); !strings.Contains(label, uiText("loading")) {
		t.Fatalf("unknown size label=%q", label)
	}
}
