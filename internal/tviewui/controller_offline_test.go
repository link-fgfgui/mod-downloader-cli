package tviewui

import (
	"testing"

	"github.com/link-fgfgui/mod-downloader-core/appcore"
	"github.com/link-fgfgui/mod-downloader-core/models"
	"github.com/link-fgfgui/mod-downloader-core/storage"
	appstructs "github.com/link-fgfgui/mod-downloader-core/structs"

	"github.com/link-fgfgui/mod-downloader-cli/internal/uistate"
)

// TestOfflineControllersAreNoOps is the developer contract for the offline
// harness: every controller entry point must return without panicking when no
// core has been started. New controller methods must be added here.
func TestOfflineControllersAreNoOps(t *testing.T) {
	h := newTestApp(t, 100, 30)
	a := h.app
	if svc := a.service(); svc != nil {
		t.Fatalf("offline app returned a service: %p", svc)
	}

	calls := []struct {
		name string
		call func()
	}{
		{"refreshMinecraft", a.refreshMinecraft},
		{"refreshSelectedMods", a.refreshSelectedMods},
		{"setMinecraftDir", func() { a.setMinecraftDir("/tmp/minecraft") }},
		{"selectVersion", func() { a.selectVersion("1.20.1") }},
		{"toggleConnector", a.toggleConnector},
		{"searchMods", func() { a.searchMods(appstructs.SearchModsRequest{}) }},
		{"refreshDownloadStates", a.refreshDownloadStates},
		{"refreshQueueState", a.refreshQueueState},
		{"queueDownload", func() { a.queueDownload(appstructs.ModDownloadRequest{}) }},
		{"cancelDownload", func() { a.cancelDownload("missing") }},
		{"retryDownload", func() { a.retryDownload("missing") }},
		{"removeDownload", func() { a.removeDownload("missing") }},
		{"dismissOptionalReminder", func() { a.dismissOptionalReminder("missing") }},
		{"clearOptionalReminders", a.clearOptionalReminders},
		{"installOptionalDependencies", func() { a.installOptionalDependencies("missing") }},
		{"analyzeBatchConflicts", func() { a.analyzeBatchConflicts(appstructs.BatchDownloadRequest{}) }},
		{"refreshUsageStats", a.refreshUsageStats},
		{"refreshSettings", a.refreshSettings},
		{"saveTheme", func() { a.saveTheme(ThemeLight) }},
		{"saveLanguage", func() { a.saveLanguage("en") }},
		{"saveSimpleMode", func() { a.saveSimpleMode(true) }},
		{"saveUnusedCleanup", func() { a.saveUnusedCleanup(true) }},
		{"saveMCIM", func() { a.saveMCIM(true) }},
		{"savePreferredReleaseType", func() { a.savePreferredReleaseType(uistate.ReleaseTypeBeta) }},
		{"saveNetworkSettings", func() { a.saveNetworkSettings(uistate.SettingsDrafts{}) }},
		{"saveCacheDir", func() { a.saveCacheDir("/tmp/cache") }},
		{"saveApiKeys", func() { a.saveApiKeys(appstructs.SaveApiKeysRequest{}) }},
		{"refreshPinnedMods", a.refreshPinnedMods},
		{"unpinMod", func() { a.unpinMod(storage.PinnedMod{}) }},
		{"unpinAll", func() { a.unpinAll(nil) }},
		{"batchUnpin", func() { a.batchUnpin(nil, "1.20.1", "Fabric") }},
		{"pinModVersion", func() { a.pinModVersion(appstructs.ModVersionPinRequest{}) }},
		{"listMatchingVersions", func() { a.listMatchingVersions(models.ModProject{}, "1.20.1", "Fabric") }},
		{"scanUnusedAll", a.scanUnusedAll},
		{"applyLocalBatch", func() { a.applyLocalBatch(nil, "enable") }},
		{"analyzeDisableImpact", func() { a.analyzeDisableImpact(appstructs.LocalModDisableImpactRequest{}) }},
		{"scanUnusedDependencies", func() { a.scanUnusedDependencies(appstructs.UnusedDependencyScanRequest{}) }},
		{"restoreCachedDependency", func() { a.restoreCachedDependency(appstructs.RestoreCachedDependencyRequest{}) }},
		{"loadFavoriteLists", func() { a.loadFavoriteLists("list") }},
		{"loadFavoriteContents", func() { a.loadFavoriteContents("list") }},
		{"createFavoriteList", func() { a.createFavoriteList("name", "1.20.1", "Fabric") }},
		{"renameFavoriteList", func() { a.renameFavoriteList("list", "name") }},
		{"deleteFavoriteList", func() { a.deleteFavoriteList("list") }},
		{"updateFavoriteList", func() { a.updateFavoriteList(storage.FavoriteList{}) }},
		{"reorderFavoriteLists", func() { a.reorderFavoriteLists([]string{"a", "b"}) }},
		{"addFavoriteMods", func() { a.addFavoriteMods(appcore.FavoriteBulkAddRequest{}) }},
		{"removeFavoriteMods", func() { a.removeFavoriteMods("list", nil) }},
		{"exportPackwiz", func() { a.exportPackwiz("list", "/tmp/out.zip", "1.20.1", "Fabric") }},
		{"previewCrossVersionCopy", func() { a.previewCrossVersionCopy(appcore.FavoriteCrossVersionCopyRequest{}) }},
		{"applyCrossVersionCopy", func() { a.applyCrossVersionCopy(appcore.FavoriteCrossVersionCopyRequest{}) }},
		{"copyFavoriteList", func() { a.copyFavoriteList("a", "b") }},
		{"addFavoriteReference", func() { a.addFavoriteReference("a", "b") }},
		{"removeFavoriteReference", func() { a.removeFavoriteReference("a", "b") }},
		{"loadFavoriteRefs", func() { a.loadFavoriteRefs("a") }},
		{"setFavoriteListIcon", func() { a.setFavoriteListIcon(storage.FavoriteList{}, "mdi", "mdi-star") }},
	}

	for _, call := range calls {
		call := call
		t.Run(call.name, func(t *testing.T) {
			defer func() {
				if recovered := recover(); recovered != nil {
					t.Fatalf("%s panicked without a started core: %v", call.name, recovered)
				}
			}()
			call.call()
		})
	}

	if got := a.packwizDefaultFilename("list"); got != "packwiz.zip" {
		t.Fatalf("offline packwizDefaultFilename=%q, want packwiz.zip", got)
	}
}
