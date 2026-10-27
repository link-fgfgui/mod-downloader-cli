package tviewui

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/gdamore/tcell/v2"

	"github.com/link-fgfgui/mod-downloader-core/appcore"
	"github.com/link-fgfgui/mod-downloader-core/global"
	"github.com/link-fgfgui/mod-downloader-core/logging"
	"github.com/link-fgfgui/mod-downloader-core/models"
	"github.com/link-fgfgui/mod-downloader-core/storage"
	appstructs "github.com/link-fgfgui/mod-downloader-core/structs"
	structs "github.com/link-fgfgui/mod-downloader-core/structs/minecraft"

	"github.com/link-fgfgui/mod-downloader-cli/internal/i18n"
	"github.com/link-fgfgui/mod-downloader-cli/internal/uistate"
)

// liveApp drives the real host + core on a simulation screen. The core owns
// process-wide state, so live tests must run sequentially (the default for a
// single Go test package) and must always shut down through Cleanup.
type liveApp struct {
	t     *testing.T
	app   *App
	sim   tcell.SimulationScreen
	errCh chan error
	done  chan struct{}
}

func newLiveApp(t *testing.T) *liveApp {
	t.Helper()
	return newLiveAppWith(t, Options{Version: "test"})
}

func newLiveAppWith(t *testing.T, opts Options) *liveApp {
	t.Helper()
	t.Chdir(t.TempDir())
	cacheDir := t.TempDir()
	t.Setenv("XDG_CACHE_HOME", cacheDir)
	t.Setenv("LOCALAPPDATA", cacheDir)
	t.Setenv("HOME", cacheDir)
	t.Setenv("TMPDIR", cacheDir)

	_ = logging.Configure(logging.Options{Disabled: true})
	previousReleases := global.GetMinecraftReleaseVersions()
	global.SetMinecraftReleaseVersions([]string{"1.21.1", "1.20.1"})
	global.SetMinecraftDir("")
	global.SetSelectedVersion(structs.VersionInfo{})
	global.ClearLocalMods()
	t.Cleanup(func() {
		global.SetMinecraftReleaseVersions(previousReleases)
		i18n.Apply(i18n.LanguageEnglish)
	})

	sim := tcell.NewSimulationScreen("UTF-8")
	started := make(chan *App, 1)
	errCh := make(chan error, 1)
	done := make(chan struct{})
	go func() {
		defer close(done)
		errCh <- runWith(context.Background(), opts, sim, func(a *App) {
			started <- a
		})
	}()

	startTimeout := 20 * time.Second
	if opts.LoadMinecraftReleases {
		// A cold start fetches the Mojang manifest.
		startTimeout = 60 * time.Second
	}
	var app *App
	select {
	case app = <-started:
	case err := <-errCh:
		t.Fatalf("live TUI failed to start: %v", err)
	case <-time.After(startTimeout):
		t.Fatal("live TUI did not start")
	}
	select {
	case <-app.startupDone:
	case <-time.After(startTimeout):
		t.Fatal("startup refreshes did not finish")
	}
	sim.SetSize(100, 30)
	app.application.QueueUpdateDraw(func() {})

	l := &liveApp{t: t, app: app, sim: sim, errCh: errCh, done: done}
	t.Cleanup(func() {
		app.Quit()
		select {
		case <-done:
		case <-time.After(15 * time.Second):
			t.Error("live TUI did not stop")
		}
	})
	return l
}

// waitUI queues a UI-thread function behind every pending host event and waits
// until it has run.
func (l *liveApp) waitUI(f func(*App)) {
	l.t.Helper()
	done := make(chan struct{})
	l.app.uiUpdate(func() {
		if f != nil {
			f(l.app)
		}
		close(done)
	})
	select {
	case <-done:
	case <-time.After(10 * time.Second):
		l.t.Fatal("UI update did not run")
	}
}

// waitFor polls a condition on the UI thread until it holds.
func (l *liveApp) waitFor(desc string, cond func(*App) bool) {
	l.t.Helper()
	l.waitForTimeout(desc, 15*time.Second, cond)
}

// waitForTimeout is waitFor with an explicit deadline for network-backed flows.
func (l *liveApp) waitForTimeout(desc string, timeout time.Duration, cond func(*App) bool) {
	l.t.Helper()
	deadline := time.Now().Add(timeout)
	for {
		done := make(chan bool, 1)
		l.app.application.QueueUpdate(func() {
			done <- cond(l.app)
		})
		if <-done {
			return
		}
		if time.Now().After(deadline) {
			l.t.Fatalf("timed out waiting for %s", desc)
		}
		time.Sleep(10 * time.Millisecond)
	}
}

func (l *liveApp) read(f func(*App)) {
	l.t.Helper()
	l.app.application.QueueUpdate(func() { f(l.app) })
}

// doUI invokes a controller method on the UI goroutine, matching how page
// callbacks run in production.
func (l *liveApp) doUI(f func()) {
	l.t.Helper()
	l.app.application.QueueUpdate(f)
}

func writeInstanceFixture(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	versionDir := filepath.Join(dir, "versions", "fabric-1.20.1")
	if err := os.MkdirAll(filepath.Join(versionDir, "mods"), 0o755); err != nil {
		t.Fatal(err)
	}
	manifest := map[string]any{
		"id":           "fabric-1.20.1",
		"name":         "Fabric 1.20.1",
		"inheritsFrom": "1.20.1",
		"patches":      []map[string]string{{"id": "fabric", "version": "0.15.11"}},
	}
	raw, err := json.Marshal(manifest)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(versionDir, "fabric-1.20.1.json"), raw, 0o644); err != nil {
		t.Fatal(err)
	}
	return dir
}

func TestLiveSettingsControllers(t *testing.T) {
	l := newLiveApp(t)

	l.doUI(func() { l.app.saveTheme(ThemeLight) })
	l.waitFor("theme applied", func(a *App) bool { return a.theme.Name == ThemeLight })
	l.waitUI(func(a *App) {
		if a.settings.Theme != ThemeLight {
			t.Errorf("settings theme=%q", a.settings.Theme)
		}
		if a.toastText != i18n.T("settings.theme.saved") {
			t.Errorf("theme toast=%q", a.toastText)
		}
	})

	l.doUI(func() { l.app.saveLanguage(i18n.LanguageChinese) })
	l.waitFor("language applied", func(a *App) bool { return i18n.Current() == i18n.LanguageChinese })
	l.waitUI(func(a *App) {
		if a.settings.Language != i18n.LanguageChinese {
			t.Errorf("settings language=%q", a.settings.Language)
		}
	})
	l.doUI(func() { l.app.saveLanguage(i18n.LanguageEnglish) })
	l.waitFor("language restored", func(a *App) bool { return i18n.Current() == i18n.LanguageEnglish })

	cacheDir := filepath.Join(t.TempDir(), "cache")
	l.doUI(func() {
		l.app.saveSimpleMode(true)
		l.app.saveUnusedCleanup(true)
		l.app.saveMCIM(true)
		l.app.savePreferredReleaseType(uistate.ReleaseTypeBeta)
		l.app.saveNetworkSettings(uistate.SettingsDrafts{
			FileConcurrency:       4,
			ConcurrentDownloads:   2,
			TargetDownloadRateMiB: 1.234,
			RequestsPerSecond:     3,
		})
		l.app.saveCacheDir(cacheDir)
		l.app.saveApiKeys(uistate.BuildSaveApiKeysRequest(
			uistate.KeyDraft{Value: "cf-key-12345678"},
			uistate.KeyDraft{Clear: true},
		))
	})

	l.waitUI(func(a *App) {
		if !a.settings.SimpleMode || !a.settings.AutoScanUnusedDependencies || !a.settings.MCIMEnabled {
			t.Errorf("boolean settings not applied: %+v", a.settings)
		}
		if a.settings.PreferredReleaseType != uistate.ReleaseTypeBeta {
			t.Errorf("release type=%q", a.settings.PreferredReleaseType)
		}
		if a.settings.FileConcurrency != 4 || a.settings.ConcurrentDownloads != 2 {
			t.Errorf("network settings=%+v", a.settings)
		}
		if a.settings.TargetDownloadRateMiB != 1.2 {
			t.Errorf("target rate=%v", a.settings.TargetDownloadRateMiB)
		}
		if !a.settings.HasCurseforgeKey || strings.TrimSpace(a.settings.CurseforgeKeyMask) == "" {
			t.Errorf("curseforge key not stored: %+v", a.settings)
		}
		if a.settings.HasModrinthKey {
			t.Errorf("modrinth key should be cleared: %+v", a.settings)
		}
		if !strings.Contains(a.settings.CacheDir, "cache") {
			t.Errorf("cache dir=%q", a.settings.CacheDir)
		}
		if a.settingsPage().cfKey != (uistate.KeyDraft{}) || a.settingsPage().mrKey != (uistate.KeyDraft{}) {
			t.Errorf("key drafts were not reset: %+v %+v", a.settingsPage().cfKey, a.settingsPage().mrKey)
		}
		if a.toastText == "" {
			t.Errorf("settings save did not toast")
		}
	})
}

func TestLiveFavoritesControllers(t *testing.T) {
	l := newLiveApp(t)
	svc := l.app.host.Service()

	l.doUI(func() { l.app.createFavoriteList("My List", "1.20.1", "Fabric") })
	l.waitFor("favorite list created", func(a *App) bool {
		for _, list := range a.favoritesPage().lists {
			if list.Name == "My List" {
				return true
			}
		}
		return false
	})
	list := svc.ListFavoriteLists()[0]
	sourceID := list.ID

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
		a.favoritesPage().SetScope("1.20.1", "Fabric")
		a.favoritesPage().selectedListID = sourceID
		a.loadFavoriteContents(sourceID)
	})
	l.waitFor("favorite mod rendered", func(a *App) bool {
		return len(a.favoritesPage().items) == 1
	})

	l.doUI(func() {
		l.app.loadFavoriteLists(sourceID)
		l.app.loadFavoriteContents(sourceID)
	})
	l.waitUI(nil)

	l.doUI(func() { l.app.renameFavoriteList(sourceID, "Renamed") })
	l.waitFor("favorite list renamed", func(a *App) bool {
		for _, list := range a.favoritesPage().lists {
			if list.ID == sourceID && list.Name == "Renamed" {
				return true
			}
		}
		return false
	})

	pinned := list
	pinned.Pinned = true
	l.doUI(func() { l.app.updateFavoriteList(pinned) })
	l.waitFor("favorite list pinned", func(a *App) bool {
		for _, list := range a.favoritesPage().lists {
			if list.ID == sourceID && list.Pinned {
				return true
			}
		}
		return false
	})

	second := svc.CreateFavoriteListForScope("Second", "1.20.1", "Fabric")
	l.doUI(func() { l.app.copyFavoriteList(sourceID, second.ID) })
	l.waitFor("favorite list copied", func(a *App) bool {
		return len(svc.ListFavoriteContents(second.ID).Mods) == 1
	})

	l.doUI(func() { l.app.setFavoriteListIcon(pinned, "mdi", "mdi-star") })
	l.waitFor("favorite icon stored", func(a *App) bool {
		for _, list := range a.favoritesPage().lists {
			if list.ID == sourceID && list.IconKind == "mdi" && list.IconValue == "mdi-star" {
				return true
			}
		}
		return false
	})

	// References are edited through the refs dialog, which also drives
	// loadFavoriteRefs.
	l.read(func(a *App) {
		a.openFavoriteRefsDialog(sourceID)
	})
	l.waitUI(nil)
	l.doUI(func() { l.app.addFavoriteReference(sourceID, second.ID) })
	l.waitFor("favorite reference added", func(a *App) bool {
		return len(svc.ListFavoriteListRefs(sourceID)) == 1
	})
	l.doUI(func() { l.app.loadFavoriteRefs(sourceID) })
	l.waitUI(func(a *App) {
		if a.refsDialog == nil || len(a.refsDialog.refs) != 1 {
			t.Errorf("refs dialog not refreshed: %+v", a.refsDialog)
		}
	})
	l.doUI(func() {
		l.app.refsDialog.handleKey(tcell.NewEventKey(tcell.KeyRune, 'x', tcell.ModNone))
	})
	l.waitFor("favorite reference removed", func(a *App) bool {
		return len(svc.ListFavoriteListRefs(sourceID)) == 0
	})
	l.doUI(func() {
		if l.app.topDialog() != nil {
			l.app.popDialog()
		}
	})

	// Page-level pin toggle and reorder go through the controller.
	l.doUI(func() {
		page := l.app.favoritesPage()
		page.pane = 0
		for i, list := range page.visibleLists() {
			if list.ID == sourceID {
				page.left.SetCurrentItem(i)
			}
		}
		page.togglePin()
	})
	l.waitFor("favorite pin toggled", func(a *App) bool {
		list, _ := findFavoriteList(svc.ListFavoriteLists(), sourceID)
		return !list.Pinned
	})

	l.doUI(func() { l.app.favoritesPage().reorder(1) })
	l.waitUI(nil)
	if got := len(svc.ListFavoriteLists()); got != 2 {
		t.Fatalf("reorder changed the list count: %d", got)
	}

	// Removing via the page selection drives removeFavoriteMods.
	l.doUI(func() {
		a := l.app
		a.favoritesPage().selectedListID = sourceID
		a.loadFavoriteContents(sourceID)
	})
	l.waitFor("favorite contents reloaded", func(a *App) bool {
		return len(a.favoritesPage().items) == 1
	})
	l.doUI(func() {
		page := l.app.favoritesPage()
		page.pane = 1
		page.selected[uistate.FavoriteKey(page.items[0].FavoriteMod)] = true
		page.removeSelected()
	})
	l.waitFor("favorite mod removed", func(a *App) bool {
		return len(svc.ListFavoriteContents(sourceID).Mods) == 0
	})

	// Exporting to an invalid destination surfaces a toast instead of panicking.
	l.doUI(func() {
		l.app.exportPackwiz(sourceID, filepath.Join(t.TempDir(), "missing", "out.zip"), "1.20.1", "Fabric")
	})
	l.waitUI(func(a *App) {
		if a.toastText == "" {
			t.Errorf("export failure produced no toast")
		}
		if a.toastTimer != nil {
			a.toastTimer.Stop()
		}
	})

	if name := l.app.packwizDefaultFilename(sourceID); !strings.HasSuffix(name, ".zip") {
		t.Errorf("packwiz default filename=%q", name)
	}

	l.doUI(func() { l.app.deleteFavoriteList(second.ID) })
	l.waitFor("favorite list deleted", func(a *App) bool {
		_, found := findFavoriteList(svc.ListFavoriteLists(), second.ID)
		return !found
	})

	// Scope filtering helpers used by the page.
	l.waitUI(func(a *App) {
		if a.favoritesPage().loaded != true {
			t.Errorf("favorites page did not record a load")
		}
	})
}

func findFavoriteList(lists []storage.FavoriteList, id string) (storage.FavoriteList, bool) {
	for _, list := range lists {
		if list.ID == id {
			return list, true
		}
	}
	return storage.FavoriteList{}, false
}

func TestLivePinControllers(t *testing.T) {
	l := newLiveApp(t)
	svc := l.app.host.Service()
	req := appstructs.ModVersionPinRequest{
		Platform: "modrinth", ModID: "sodium", VersionID: "v1",
		MinecraftVersion: "1.20.1", ModLoader: "Fabric",
	}
	l.doUI(func() { l.app.pinModVersion(req) })
	l.waitFor("pin stored", func(a *App) bool {
		return len(svc.ListPinnedMods()) == 1
	})
	pin := svc.ListPinnedMods()[0]

	l.doUI(func() { l.app.refreshPinnedMods() })
	l.waitFor("unpin page refreshed", func(a *App) bool {
		page := a.views[ViewUnpin].(*unpinPage)
		return len(page.pins) == 1
	})

	// batchUnpin toggles an existing pin off.
	l.doUI(func() {
		l.app.batchUnpin([]models.ModProject{{Platform: "modrinth", ProjectID: "sodium"}}, "1.20.1", "Fabric")
	})
	l.waitFor("batch unpin", func(a *App) bool {
		return len(svc.ListPinnedMods()) == 0
	})

	// Re-pin and unpin a single entry through the page.
	l.doUI(func() { l.app.pinModVersion(req) })
	l.waitFor("pin stored again", func(a *App) bool {
		return len(svc.ListPinnedMods()) == 1
	})
	l.doUI(func() { l.app.refreshPinnedMods() })
	l.waitUI(nil)
	l.doUI(func() { l.app.unpinMod(pin) })
	l.waitFor("unpin single", func(a *App) bool {
		return len(svc.ListPinnedMods()) == 0
	})
	l.waitUI(func(a *App) {
		if a.toastText != i18n.T("unpin.removed") {
			t.Errorf("unpin toast=%q", a.toastText)
		}
	})

	// unpinAll clears everything left.
	l.doUI(func() {
		for i := 0; i < 2; i++ {
			l.app.pinModVersion(appstructs.ModVersionPinRequest{
				Platform: "modrinth", ModID: "mod-" + itoa(i), VersionID: "v" + itoa(i),
				MinecraftVersion: "1.20.1", ModLoader: "Fabric",
			})
		}
	})
	l.waitFor("pins restored", func(a *App) bool {
		return len(svc.ListPinnedMods()) == 2
	})
	l.doUI(func() { l.app.unpinAll(svc.ListPinnedMods()) })
	l.waitFor("unpin all", func(a *App) bool {
		return len(svc.ListPinnedMods()) == 0
	})
}

func TestLiveQueueAndDownloadControllers(t *testing.T) {
	l := newLiveApp(t)
	svc := l.app.host.Service()

	// Invalid requests are rejected without touching the network and surface a
	// localized toast.
	l.doUI(func() { l.app.queueDownload(appstructs.ModDownloadRequest{}) })
	l.waitUI(func(a *App) {
		if a.toastText == "" {
			t.Errorf("skipped download did not toast")
		}
		if a.search.DownloadingKeys != nil {
			t.Errorf("download keys were not reset: %#v", a.search.DownloadingKeys)
		}
		if a.toastTimer != nil {
			a.toastTimer.Stop()
		}
	})

	l.doUI(func() {
		l.app.cancelDownload("missing")
		l.app.retryDownload("missing")
		l.app.removeDownload("missing")
		l.app.dismissOptionalReminder("missing")
		l.app.clearOptionalReminders()
		l.app.installOptionalDependencies("missing")
	})
	l.waitFor("queue refresh", func(a *App) bool {
		return len(a.queueState.Queue.Items) == 0
	})

	l.doUI(func() {
		l.app.refreshQueueState()
		l.app.refreshUsageStats()
		l.app.refreshSettings()
	})
	l.waitUI(nil)

	// No selected instance -> disabled states, no network.
	l.doUI(func() {
		l.app.search.SearchResults = []models.ModProject{projectAt(0)}
		l.app.search.SelectedVersion = "1.20.1"
		l.app.refreshDownloadStates()
	})
	l.waitUI(func(a *App) {
		if len(a.search.DownloadStates) != 1 {
			t.Fatalf("download states=%d", len(a.search.DownloadStates))
		}
		if !a.search.DownloadStates[0].Disabled || a.search.DownloadStates[0].Status != "noSelectedVersion" {
			t.Fatalf("state=%+v", a.search.DownloadStates[0])
		}
	})

	// Empty batch analysis installs nothing and opens no dialog.
	l.doUI(func() { l.app.analyzeBatchConflicts(appstructs.BatchDownloadRequest{}) })
	l.waitUI(func(a *App) {
		if a.topDialog() != nil {
			t.Errorf("empty batch analysis opened a dialog")
		}
	})

	// A no-op batch (empty result list) is settled without errors.
	l.doUI(func() { l.app.search.ClearQueueMembership() })
	if svc == nil {
		t.Fatal("live service missing")
	}
}

func TestLiveMinecraftControllers(t *testing.T) {
	l := newLiveApp(t)
	dir := writeInstanceFixture(t)

	l.doUI(func() { l.app.setMinecraftDir(dir) })
	l.waitFor("minecraft dir applied", func(a *App) bool {
		return a.mc.MinecraftDir == dir && len(a.mc.Versions) > 0
	})

	l.doUI(func() { l.app.selectVersion("fabric-1.20.1") })
	l.waitFor("instance selected", func(a *App) bool {
		return a.mc.SelectedVersion != nil && a.mc.SelectedVersion.ID == "fabric-1.20.1"
	})
	l.waitFor("selected mods loaded", func(a *App) bool {
		return !a.mc.EnsureSelectedModsLoadedNeeded()
	})

	l.doUI(func() { l.app.refreshMinecraft() })
	l.waitFor("versions refreshed", func(a *App) bool {
		return len(a.mc.Versions) > 0
	})
	l.doUI(func() {
		l.app.refreshSelectedMods()
		l.app.refreshUsageStats()
		l.app.refreshSettings()
	})
	l.waitUI(nil)

	// Connector toggle fails (no connector) and surfaces the error.
	l.doUI(func() { l.app.toggleConnector() })
	l.waitUI(func(a *App) {
		if a.toastText == "" {
			t.Errorf("connector error produced no toast")
		}
		if a.toastTimer != nil {
			a.toastTimer.Stop()
		}
	})

	// Local mod operations on unknown paths fail gracefully.
	l.doUI(func() {
		l.app.analyzeDisableImpact(appstructs.LocalModDisableImpactRequest{
			Paths:  []string{filepath.Join(dir, "versions", "fabric-1.20.1", "mods", "missing.jar")},
			Action: "disable",
		})
	})
	l.waitUI(nil)
	l.doUI(func() { l.app.applyLocalBatch(nil, "enable") })
	l.waitFor("local batch failure toast", func(a *App) bool {
		return a.toastText == i18n.T("manage.operationFailed")
	})
	l.doUI(func() {
		l.app.scanUnusedDependencies(appstructs.UnusedDependencyScanRequest{})
		l.app.scanUnusedAll()
		l.app.restoreCachedDependency(appstructs.RestoreCachedDependencyRequest{})
	})
	l.waitUI(nil)

	// Page-level batch operations drive the same controller path.
	l.doUI(func() {
		page := l.app.managePage()
		group := uistate.ModGroup{Primary: structs.ModInfo{
			ID: "missing", FileName: "missing.jar",
			Path: filepath.Join(dir, "versions", "fabric-1.20.1", "mods", "missing.jar"),
		}}
		row := uistate.ManageRow{Kind: uistate.RowKindMod, Group: group}
		page.rows = []uistate.ManageRow{row}
		page.selected = map[string]bool{row.RowKey(): true}
		page.applyBatch("enable")
	})
	l.waitFor("manage batch failure toast", func(a *App) bool {
		return a.toastText == i18n.T("manage.operationFailed")
	})
	l.waitUI(func(a *App) {
		if a.toastTimer != nil {
			a.toastTimer.Stop()
		}
	})
	l.doUI(func() { l.app.managePage().scanUnused() })
	l.waitFor("manage scan toast", func(a *App) bool {
		return a.toastText == i18n.T("manage.cleanup.none")
	})
	l.waitUI(nil)

	// The disable-impact dialog's single-candidate restore path is wired to the
	// controller and clears its busy marker even when the cache rejects it.
	l.doUI(func() {
		l.app.openDisableImpactDialog(nil, "disable", appstructs.LocalModDisableImpactResult{
			Impacts: []appstructs.LocalModDependencyImpact{{
				ModID: "fabric-api",
				Candidates: []appstructs.CachedDependencyCandidate{{
					ModID: "fabric-api", Platform: "modrinth", ProjectID: "fabric-api", VersionID: "v1",
				}},
			}},
		})
	})
	l.waitUI(nil)
	l.doUI(func() {
		dialog := l.app.impactDialog
		dialog.list.SetCurrentItem(0)
		dialog.handleKey(tcell.NewEventKey(tcell.KeyRune, 'r', tcell.ModNone))
	})
	l.waitFor("restore completed", func(a *App) bool {
		return a.impactDialog != nil && a.impactDialog.restoring == ""
	})
	l.doUI(func() {
		if l.app.topDialog() != nil {
			l.app.popDialog()
		}
	})

	// Invalid version selection reports an error instead of changing state.
	var before string
	l.waitUI(func(a *App) {
		if a.mc.SelectedVersion != nil {
			before = a.mc.SelectedVersion.ID
		}
	})
	l.doUI(func() { l.app.selectVersion("does-not-exist") })
	l.waitUI(func(a *App) {
		if a.mc.SelectedVersion == nil || a.mc.SelectedVersion.ID != before {
			t.Errorf("invalid select changed the instance: %+v", a.mc.SelectedVersion)
		}
		if a.toastText == "" {
			t.Errorf("invalid select produced no toast")
		}
		if a.toastTimer != nil {
			a.toastTimer.Stop()
		}
	})

	// Chaos: switch back and forth between tuple edits and instances.
	l.doUI(func() {
		l.app.mc.SetSelectedMinecraftVersion("1.20.1")
		l.app.search.SetTargetTuple("1.20.1", "Fabric", false)
		l.app.refreshDownloadStates()
	})
	l.waitUI(nil)
}

func TestLiveQuitStopsTheApplication(t *testing.T) {
	l := newLiveApp(t)
	done := make(chan struct{})
	go func() {
		l.app.Quit()
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(10 * time.Second):
		t.Fatal("Quit did not return")
	}
	select {
	case <-l.done:
	case <-time.After(15 * time.Second):
		t.Fatal("application did not stop after Quit")
	}
	if err := <-l.errCh; err != nil {
		t.Fatalf("run returned %v", err)
	}
	// Cleanup will call Quit again through the once-guard and see done closed.
}
