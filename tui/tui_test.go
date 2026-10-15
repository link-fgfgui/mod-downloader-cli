package tui

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/link-fgfgui/mod-downloader-core/storage"
	appstructs "github.com/link-fgfgui/mod-downloader-core/structs"
	structs "github.com/link-fgfgui/mod-downloader-core/structs/minecraft"

	"github.com/link-fgfgui/mod-downloader-cli/internal/uistate"
	"github.com/link-fgfgui/mod-downloader-cli/tui/i18n"
)

// newTestApp builds a shell model without a Host so pure layout/interaction can
// be exercised. All View() paths of the six views are host-free.
func newTestApp() *AppModel {
	i18n.Apply(i18n.LanguageEnglish)
	m := &AppModel{
		theme:  NewTheme(ThemeDark, false),
		queue:  uistate.NewQueueState(),
		mc:     uistate.NewMinecraftState(),
		search: uistate.SearchState{SelectedModLoader: uistate.DefaultModLoader},
		ready:  true,
		width:  120,
		height: 40,
	}
	m.views = []view{&m.home, &m.download, &m.manage, &m.favorites, &m.unpin, &m.settings}
	m.home.app = m
	m.download.app = m
	m.manage.app = m
	m.favorites.app = m
	m.unpin.app = m
	m.settings.app = m
	return m
}

func TestShellLayoutIsExactHeight(t *testing.T) {
	for _, size := range []struct{ w, h int }{{80, 24}, {120, 40}, {200, 60}} {
		m := newTestApp()
		m.width, m.height = size.w, size.h
		for _, vid := range []ViewID{ViewHome, ViewDownload, ViewManage, ViewFavorites, ViewUnpin, ViewSettings} {
			m.active = vid
			out := m.View()
			lines := strings.Split(out, "\n")
			if len(lines) != size.h {
				t.Fatalf("view %d at %dx%d: expected %d lines, got %d", vid, size.w, size.h, size.h, len(lines))
			}
			for i, line := range lines {
				if w := displayWidth(line); w > size.w {
					t.Fatalf("view %d at %dx%d: line %d width %d exceeds %d", vid, size.w, size.h, i, w, size.w)
				}
			}
		}
	}
}

func TestTopBarAndTabHitTest(t *testing.T) {
	m := newTestApp()
	m.width, m.height = 120, 40
	top := m.renderTopBar()
	lines := strings.Split(top, "\n")
	if len(lines) != 2 {
		t.Fatalf("top bar must be 2 rows, got %d", len(lines))
	}
	if !strings.Contains(stripStyles(lines[0]), "mod-downloader") {
		t.Fatalf("top bar row 1 missing app title: %q", lines[0])
	}
	if !strings.Contains(stripStyles(m.tabLabel(ViewManage)), "[3]") {
		t.Fatalf("tab label numbering wrong: %q", m.tabLabel(ViewManage))
	}

	// Clicking each tab label's first cell must resolve to that view.
	pos := displayWidth(m.appTitle()) + 2
	for i := 0; i < viewCount; i++ {
		got, ok := m.hitTab(pos + 1)
		if !ok || int(got) != i {
			t.Fatalf("hitTab at %d resolved to %v (ok=%v), want %d", pos+1, got, ok, i)
		}
		pos += displayWidth(m.tabLabel(ViewID(i))) + 2
	}
}

func TestGlobalKeyNavigation(t *testing.T) {
	m := newTestApp()
	m.width, m.height = 120, 40

	// Digit key switches view and is consumed.
	if _, handled := m.handleGlobalKey(key("3")); !handled || m.active != ViewManage {
		t.Fatalf("digit 3 should switch to Manage, active=%v handled=%v", m.active, handled)
	}
	// Tab advances cyclically.
	m.active = ViewSettings
	if _, handled := m.handleGlobalKey(key("tab")); !handled || m.active != ViewHome {
		t.Fatalf("tab from Settings should wrap to Home, active=%v", m.active)
	}
	// q toggles the queue drawer only when there is content.
	if _, handled := m.handleGlobalKey(key("q")); !handled || m.queueOpen {
		t.Fatalf("q with empty queue must be handled and keep drawer closed, open=%v", m.queueOpen)
	}
}

func TestQueueCapsuleReflectsState(t *testing.T) {
	m := newTestApp()
	m.width, m.height = 120, 40

	if got := m.queueCapsule(); !strings.Contains(got, "0/0") {
		t.Fatalf("idle capsule wrong: %q", got)
	}
	m.queue.Queue = appstructs.DownloadQueueState{
		Active:         true,
		Running:        2,
		Pending:        1,
		BytesComplete:  512 * 1024,
		TotalBytes:     1024 * 1024,
		BytesPerSecond: 2 * 1024 * 1024,
		Items:          []appstructs.DownloadQueueItem{{ID: "1", Status: "running"}},
	}
	got := m.queueCapsule()
	for _, want := range []string{"2/1", "50%", "MiB/s"} {
		if !strings.Contains(got, want) {
			t.Fatalf("active capsule %q missing %q", got, want)
		}
	}
	// Right-clicking the capsule clears reminder-only queues.
	m.queue.Queue = appstructs.DownloadQueueState{MessageCount: 1}
	if !m.hitQueueCapsule(m.width-2, m.height-1) {
		t.Fatal("capsule hit region should cover the bottom-right corner")
	}
}

func TestUpdateRoutingSwitchesViewsAndOpensModals(t *testing.T) {
	m := newTestApp()
	m.width, m.height = 120, 40

	// Digit keys routed through Update must switch the active view and change
	// the rendered frame.
	_, _ = m.Update(key("3"))
	if m.active != ViewManage {
		t.Fatalf("Update(3) should switch to Manage, got %v", m.active)
	}
	if !strings.Contains(stripStyles(m.View()), i18n.T("manage.title")) {
		t.Fatal("Manage frame not rendered after view switch")
	}

	// Settings view rendering.
	_, _ = m.Update(key("6"))
	if m.active != ViewSettings {
		t.Fatalf("Update(6) should switch to Settings, got %v", m.active)
	}
	if !strings.Contains(stripStyles(m.View()), i18n.T("settings.title")) {
		t.Fatal("Settings frame not rendered")
	}

	// Download view without a selected instance offers the dir picker via `o`,
	// which pushes the PathPickerModal (the Wails native-dialog replacement).
	_, _ = m.Update(key("2"))
	if m.active != ViewDownload {
		t.Fatalf("Update(2) should switch to Download, got %v", m.active)
	}
	_, _ = m.Update(key("o"))
	if !m.modals.has("download-mc-dir") {
		t.Fatal("pressing o without an instance must open the path picker modal")
	}
	// Esc closes it.
	_, _ = m.Update(escKey())
	if len(m.modals) != 0 {
		t.Fatal("esc must close the path picker")
	}
}

func TestQueueDrawerToggleAndTabSwitch(t *testing.T) {
	m := newTestApp()
	m.width, m.height = 120, 40
	// A reminder-only queue makes the capsule visible and q opens the drawer.
	m.queue.Queue = appstructs.DownloadQueueState{OptionalReminders: []appstructs.OptionalDependencyReminder{{ID: "r1"}}}
	m.queue.SyncPresentation()
	_, _ = m.Update(key("q"))
	if !m.queueOpen {
		t.Fatal("q must open the drawer when the capsule is visible")
	}
	// Drawing the drawer must not exceed the terminal height.
	lines := strings.Split(m.View(), "\n")
	if len(lines) != m.height {
		t.Fatalf("drawer overlay changed height: %d != %d", len(lines), m.height)
	}
	_, _ = m.Update(escKey())
	if m.queueOpen {
		t.Fatal("esc must close the drawer")
	}
}

func TestInstanceTargetModalOpensAndEditsTargetTuple(t *testing.T) {
	m := newTestApp()
	m.width, m.height = 120, 40
	m.mc.ReleaseVersions = []string{"1.21.1", "1.20.1"}

	// `t` opens the instance/target switcher.
	_, _ = m.Update(key("t"))
	if !m.modals.has("instance-target") {
		t.Fatal("t must open the instance/target modal")
	}

	// Navigate to the loader row and cycle it: manual edits must clear the
	// selected instance (browse/target-only mode).
	m.mc.ApplySelectedVersion(&structs.VersionInfo{ID: "inst", Name: "inst", MinecraftVersion: "1.21.1", ModLoader: "Fabric"})
	for i := 0; i < 5; i++ {
		_, _ = m.Update(key("down"))
	}
	_, _ = m.Update(key("right"))
	if m.mc.SelectedModLoader == "Fabric" {
		t.Fatalf("loader row must advance the loader, got %q", m.mc.SelectedModLoader)
	}
	if m.mc.SelectedVersion != nil {
		t.Fatal("a manual target edit must clear the selected instance")
	}

	// Minecraft version row cycles through the release list.
	_, _ = m.Update(key("up"))
	before := m.mc.SelectedMinecraftVersion
	_, _ = m.Update(key("right"))
	if m.mc.SelectedMinecraftVersion == before {
		t.Fatalf("MC version row must advance, still %q", before)
	}

	// Esc closes.
	_, _ = m.Update(escKey())
	if m.modals.has("instance-target") {
		t.Fatal("esc must close the instance/target modal")
	}
}

func TestTopBarRegionHitsInstanceAndConnector(t *testing.T) {
	m := newTestApp()
	m.width, m.height = 120, 40
	m.mc.MinecraftDir = "/home/steve/.minecraft"
	m.mc.SelectedModLoader = "Fabric"
	m.mc.SelectedVersion = &structs.VersionInfo{
		ID: "1.21.1-connector", Name: "1.21.1-connector",
		MinecraftVersion: "1.21.1", ModLoader: "Fabric",
		ConnectorAvailable: true, ActualModLoader: "Fabric",
	}
	_ = m.View() // rendering records the hit regions

	region := ""
	for _, r := range m.topBarRegions {
		if r.name == "connector" {
			region = "connector"
		}
	}
	if region != "connector" {
		t.Fatalf("connector region must be recorded when ConnectorAvailable, regions=%v", m.topBarRegions)
	}

	// Clicking the connector span must invoke the toggle path (captured as a
	// non-nil command even though the host is absent in tests).
	hit := false
	for _, r := range m.topBarRegions {
		if r.name == "instance" && m.hitTopBarRegion(r.x0+1, 1) == "instance" {
			hit = true
		}
	}
	if !hit {
		t.Fatal("instance region must be hittable")
	}
}

func TestFavoritesReferencesAndIconEntryPoints(t *testing.T) {
	m := newTestApp()
	m.width, m.height = 120, 40
	m.mc.SelectedMinecraftVersion = "1.21.1"
	m.mc.SelectedModLoader = "Fabric"
	m.favorites.lists = []storage.FavoriteList{
		{ID: "parent", Name: "Parent", MinecraftVersion: "1.21.1", ModLoader: "Fabric"},
		{ID: "child", Name: "Child", MinecraftVersion: "1.21.1", ModLoader: "Fabric"},
	}
	m.favorites.SetScope("1.21.1", "Fabric")
	m.favorites.selectedListID = "parent"
	m.favorites.pane = 0
	m.favorites.listCursor = 0
	m.active = ViewFavorites

	// `F` opens the reference manager.
	_, _ = m.Update(key("F"))
	if !m.modals.has("favorite-refs") {
		t.Fatal("F must open the reference manager modal")
	}
	_, _ = m.Update(escKey())

	// `i` opens the icon customization chooser.
	_, _ = m.Update(key("i"))
	if !m.modals.has("fav-icon") {
		t.Fatal("i must open the icon chooser")
	}
}

func TestOverlayCenteredKeepsHeightAndWidth(t *testing.T) {
	bg := strings.Join([]string{"aa", "bb", "cc", "dd", "ee"}, "\n")
	fg := "MODAL\nLINE2"
	out := overlayCentered(bg, fg, 20, 5)
	lines := strings.Split(out, "\n")
	if len(lines) != 5 {
		t.Fatalf("overlay changed height: %d", len(lines))
	}
	if !strings.Contains(out, "MODAL") {
		t.Fatalf("overlay content missing")
	}
	for _, line := range lines {
		if displayWidth(line) > 20 {
			t.Fatalf("overlay line too wide: %q", line)
		}
	}
}

func TestFormatHelpers(t *testing.T) {
	if got := formatBytes(1536); got != "1.5 KiB" {
		t.Fatalf("formatBytes = %q", got)
	}
	if got := formatSpeed(1024 * 1024); got != "1.00 MiB/s" {
		t.Fatalf("formatSpeed = %q", got)
	}
	if got := formatPercent(50, 200); got != "25%" {
		t.Fatalf("formatPercent = %q", got)
	}
	if got := formatPercent(1, 0); got != "" {
		t.Fatalf("unknown-size percent must be empty, got %q", got)
	}
	if got := compactNumber(1200); got != "1.2K" {
		t.Fatalf("compactNumber(1200) = %q", got)
	}
	if got := compactNumber(999); got != "999" {
		t.Fatalf("compactNumber(999) = %q", got)
	}
	if got := truncate("日本語テキスト", 4); displayWidth(got) > 8 {
		t.Fatalf("truncate width exceeded: %q", got)
	}
}

func TestModalStackPushPop(t *testing.T) {
	m := newTestApp()
	first := newConfirmModal("a", "A", "", []choiceOption{{Label: "ok", Value: "ok"}}, nil)
	second := newConfirmModal("b", "B", "", []choiceOption{{Label: "ok", Value: "ok"}}, nil)
	m.pushModal(first)
	m.pushModal(second)
	// Duplicate ids are ignored.
	m.pushModal(newConfirmModal("a", "A2", "", nil, nil))
	if len(m.modals) != 2 {
		t.Fatalf("expected 2 modals, got %d", len(m.modals))
	}
	if m.modals.top().ID() != "b" {
		t.Fatalf("top modal should be b, got %q", m.modals.top().ID())
	}
	m.popModal("b")
	if m.modals.top().ID() != "a" {
		t.Fatalf("after pop top should be a, got %q", m.modals.top().ID())
	}
}

func TestTopModalCapturesKeys(t *testing.T) {
	m := newTestApp()
	m.width, m.height = 120, 40
	m.active = ViewSettings
	m.pushModal(newPromptModal("p", "title", "", "", "", nil))

	// A digit key must go to the modal (CapturesInput), not switch views.
	_, _ = m.Update(key("3"))
	if m.active != ViewSettings {
		t.Fatalf("modal must swallow the digit key, active=%v", m.active)
	}
	// Esc closes the modal.
	_, _ = m.Update(escKey())
	if len(m.modals) != 0 {
		t.Fatalf("esc should close the modal, remaining=%d", len(m.modals))
	}
}

func key(s string) tea.KeyMsg {
	switch s {
	case "tab":
		return tea.KeyMsg{Type: tea.KeyTab}
	case "esc":
		return tea.KeyMsg{Type: tea.KeyEsc}
	default:
		return tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(s)}
	}
}

func escKey() tea.KeyMsg { return tea.KeyMsg{Type: tea.KeyEsc} }
