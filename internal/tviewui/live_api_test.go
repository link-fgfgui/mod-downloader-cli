package tviewui

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/gdamore/tcell/v2"
)

// Real network tests are opt-in so the default suite stays deterministic:
//
//	MOD_DOWNLOADER_LIVE_API=1 go test ./internal/tviewui -run TestLiveAPI
func requireLiveAPI(t *testing.T) {
	t.Helper()
	if os.Getenv("MOD_DOWNLOADER_LIVE_API") != "1" {
		t.Skip("set MOD_DOWNLOADER_LIVE_API=1 to run real network tests")
	}
}

// TestLiveAPIMinecraftReleaseStartup exercises the exact startup path used by
// `mod-downloader-cli` (LoadMinecraftReleases=true) against the real Mojang
// manifest.
func TestLiveAPIMinecraftReleaseStartup(t *testing.T) {
	requireLiveAPI(t)

	l := newLiveAppWith(t, Options{Version: "test", LoadMinecraftReleases: true})
	l.waitForTimeout("release versions", 30*time.Second, func(a *App) bool {
		return len(a.mc.ReleaseVersions) > 0
	})
	l.waitUI(func(a *App) {
		for _, version := range a.mc.ReleaseVersions {
			if strings.TrimSpace(version) == "" {
				t.Errorf("release version list contains an empty entry: %v", a.mc.ReleaseVersions)
				return
			}
		}
	})
}

// TestLiveAPISearchAndVersionDialog drives the real Modrinth search through the
// keyboard path, then opens the version dialog for the first result. It covers
// providers -> appcore -> tuihost -> event pump -> uistate -> tview.
func TestLiveAPISearchAndVersionDialog(t *testing.T) {
	requireLiveAPI(t)

	l := newLiveApp(t)
	dir := writeInstanceFixture(t)
	l.doUI(func() { l.app.setMinecraftDir(dir) })
	l.waitFor("minecraft dir applied", func(a *App) bool { return a.mc.MinecraftDir == dir })
	l.doUI(func() { l.app.selectVersion("fabric-1.20.1") })
	l.waitFor("instance selected", func(a *App) bool {
		return a.mc.SelectedVersion != nil && a.mc.SelectedVersion.ID == "fabric-1.20.1"
	})
	l.waitFor("search target tuple", func(a *App) bool {
		return a.search.SelectedVersion == "1.20.1" && strings.EqualFold(a.search.SelectedModLoader, "fabric")
	})

	l.pressRune('2')
	l.waitFor("download view", func(a *App) bool { return a.active == ViewDownload })
	l.pressRune('/')
	l.waitFor("search focused", func(a *App) bool { return a.downloadPage().focusedSearch })
	l.typeText("sodium")
	l.waitFor("query typed", func(a *App) bool { return a.downloadPage().search.GetText() == "sodium" })
	l.pressKey(tcell.KeyEnter)
	l.waitForTimeout("search response", 90*time.Second, func(a *App) bool {
		return !a.search.IsSearching && !a.search.IsLoadingMore && len(a.search.SearchResults) > 0
	})

	var sodiumFound bool
	var titles []string
	l.waitUI(func(a *App) {
		for _, result := range a.search.SearchResults {
			titles = append(titles, result.Platform+":"+result.Title)
			if strings.EqualFold(result.Platform, "modrinth") && strings.Contains(strings.ToLower(result.Title), "sodium") {
				sodiumFound = true
			}
		}
	})
	if !sodiumFound {
		t.Fatalf("real Modrinth search returned no sodium result: %v", titles)
	}
	l.waitText("Sodium")

	l.pressRune('v')
	l.waitForTimeout("version dialog", 90*time.Second, func(a *App) bool {
		return a.versionDialog != nil && !a.versionDialog.loading && len(a.versionDialog.versions) > 0
	})
	l.waitText("Mod Versions")
	l.pressKey(tcell.KeyEsc)
	l.waitFor("version dialog closed", func(a *App) bool { return a.versionDialog == nil })
	l.waitFor("still on download", func(a *App) bool { return a.active == ViewDownload })
}

// TestLiveAPIDownloadWritesJar installs the first real search result through the
// keyboard path and verifies the jar lands in the selected instance's mods
// directory. This covers provider resolution, the download queue, the event
// bridge and the filesystem, not just UI state.
func TestLiveAPIDownloadWritesJar(t *testing.T) {
	requireLiveAPI(t)

	l := newLiveApp(t)
	dir := writeInstanceFixture(t)
	modsDir := filepath.Join(dir, "versions", "fabric-1.20.1", "mods")
	l.doUI(func() { l.app.setMinecraftDir(dir) })
	l.waitFor("minecraft dir applied", func(a *App) bool { return a.mc.MinecraftDir == dir })
	l.doUI(func() { l.app.selectVersion("fabric-1.20.1") })
	l.waitFor("instance selected", func(a *App) bool {
		return a.mc.SelectedVersion != nil && a.mc.SelectedVersion.ID == "fabric-1.20.1"
	})

	l.pressRune('2')
	l.waitFor("download view", func(a *App) bool { return a.active == ViewDownload })
	l.pressRune('/')
	l.typeText("sodium")
	l.pressKey(tcell.KeyEnter)
	l.waitForTimeout("search response", 90*time.Second, func(a *App) bool {
		return !a.search.IsSearching && len(a.search.SearchResults) > 0
	})

	l.pressRune('i')
	jar := waitForInstalledJar(t, modsDir, "sodium", 120*time.Second)
	if !strings.Contains(strings.ToLower(filepath.Base(jar)), "sodium") {
		t.Fatalf("downloaded jar %q does not look like sodium", jar)
	}
	// The UI must reflect the completed queue rather than staying stuck busy.
	l.waitForTimeout("queue settled", 30*time.Second, func(a *App) bool {
		return !a.queueState.Queue.Active
	})
}

func waitForInstalledJar(t *testing.T, dir, name string, timeout time.Duration) string {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		entries, err := os.ReadDir(dir)
		if err != nil && !os.IsNotExist(err) {
			t.Fatalf("read mods dir: %v", err)
		}
		for _, entry := range entries {
			lower := strings.ToLower(entry.Name())
			if strings.Contains(lower, name) && strings.HasSuffix(lower, ".jar") {
				return filepath.Join(dir, entry.Name())
			}
		}
		time.Sleep(100 * time.Millisecond)
	}
	t.Fatalf("no %s jar appeared in %s after %s", name, dir, timeout)
	return ""
}
