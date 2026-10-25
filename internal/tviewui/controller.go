package tviewui

import (
	"strings"

	"github.com/link-fgfgui/mod-downloader-core/appcore"
	"github.com/link-fgfgui/mod-downloader-core/models"
	"github.com/link-fgfgui/mod-downloader-core/storage"
	appstructs "github.com/link-fgfgui/mod-downloader-core/structs"
	structs "github.com/link-fgfgui/mod-downloader-core/structs/minecraft"

	"github.com/link-fgfgui/mod-downloader-cli/internal/i18n"
	"github.com/link-fgfgui/mod-downloader-cli/internal/tuihost"
	"github.com/link-fgfgui/mod-downloader-cli/internal/uistate"
)

// --- events ------------------------------------------------------------------

// handleEvent applies one host event on the UI goroutine.
func (a *App) handleEvent(event tuihost.Event) {
	switch event.Kind {
	case tuihost.EventSearchModsUpdated:
		if update, ok := event.Payload.(appstructs.SearchModsUpdate); ok {
			accepted, refresh := a.search.ApplySearchUpdate(update)
			if accepted {
				a.refreshDownloadView()
			}
			if refresh {
				a.refreshDownloadStates()
			}
		}
	case tuihost.EventDownloadStatesUpdated:
		a.refreshDownloadStates()
	case tuihost.EventDownloadQueueUpdated:
		if state, ok := event.Payload.(appstructs.DownloadQueueState); ok {
			a.applyQueue(state)
		}
	case tuihost.EventDownloadFailed:
		if failed, ok := event.Payload.(appstructs.DownloadFailedEvent); ok {
			fileName := failed.FileName
			if fileName == "" {
				fileName = i18n.T("download.errors.unknownFile")
			}
			reason := failed.Reason
			if reason == "" {
				reason = i18n.T("download.errors.generic")
			}
			a.toastT("download.errors.failedWithReason", map[string]string{"fileName": fileName, "reason": reason})
		}
		a.refreshDownloadStates()
	case tuihost.EventDownloadCompleted:
		a.queueState.NoteDownloadCompleted()
	case tuihost.EventMinecraftDirChanged:
		go a.refreshMinecraft()
	case tuihost.EventSelectedVersionChanged:
		if version, ok := event.Payload.(structs.VersionInfo); ok {
			a.applySelectedVersion(version, true)
		}
	case tuihost.EventUsageStatsUpdated:
		go a.refreshUsageStats()
	case tuihost.EventExtensionModsAccepted:
		if update, ok := event.Payload.(appstructs.SearchModsUpdate); ok {
			if a.search.ApplyExtensionResults(update.Results) {
				a.switchView(ViewDownload)
				a.downloadPage().Refresh()
				a.toastT("download.queued")
				a.refreshDownloadStates()
			}
		}
	}
	a.updateStatus()
}

// applySelectedVersion adopts a core version snapshot without scheduling a
// refresh (the core already refreshed mods before emitting the event).
func (a *App) applySelectedVersion(version structs.VersionInfo, markLoaded bool) {
	a.mc.ApplySelectedVersion(&version)
	if markLoaded {
		a.mc.MarkSelectedModsLoaded()
	}
	a.search.SetTargetTuple(a.mc.SelectedMinecraftVersion, a.mc.SelectedModLoader, a.mc.HasSelectedInstance())
	a.favoritesPage().SetScope(a.mc.SelectedMinecraftVersion, a.mc.SelectedModLoader)
	a.updateSidebarFooter()
	if a.targetDialog != nil {
		a.targetDialog.refresh()
	}
	a.downloadPage().Refresh()
	a.managePage().Refresh()
	a.homePage().Refresh()
	a.refreshDownloadStates()
}

// --- instance / search -------------------------------------------------------

func (a *App) refreshMinecraft() {
	if a.offline {
		return
	}
	a.beginBusy(uiText("refreshing"))
	defer a.endBusy()
	svc := a.host.Service()
	versions := svc.RefreshVersions()
	selected := svc.GetSelectedVersion()
	releases := svc.GetMinecraftReleaseVersions()
	dir := svc.GetMinecraftDir()

	a.uiUpdate(func() {
		a.mc.Versions = versions
		a.mc.ReleaseVersions = releases
		a.mc.MinecraftDir = dir
		a.mc.ApplySelectedVersion(&selected)
		a.search.SetTargetTuple(a.mc.SelectedMinecraftVersion, a.mc.SelectedModLoader, a.mc.HasSelectedInstance())
		a.favoritesPage().SetScope(a.mc.SelectedMinecraftVersion, a.mc.SelectedModLoader)
		a.updateSidebarFooter()
		if a.targetDialog != nil {
			a.targetDialog.refresh()
		}
		if a.mc.EnsureSelectedModsLoadedNeeded() {
			go a.refreshSelectedMods()
		}
		a.downloadPage().Refresh()
		a.managePage().Refresh()
		a.homePage().Refresh()
		a.refreshDownloadStates()
		a.updateStatus()
	})
}

func (a *App) refreshSelectedMods() {
	if a.offline {
		return
	}
	a.beginBusy(uiText("refreshing"))
	defer a.endBusy()
	version := a.host.Service().RefreshSelectedVersionMods()
	a.uiUpdate(func() {
		a.mc.ApplySelectedVersion(&version)
		a.mc.MarkSelectedModsLoaded()
		a.updateSidebarFooter()
		a.managePage().Refresh()
		a.downloadPage().Refresh()
	})
}

func (a *App) setMinecraftDir(dir string) {
	a.host.Service().SetMinecraftDir(dir)
	a.refreshMinecraft()
	a.refreshSettings()
}

func (a *App) selectVersion(key string) {
	a.beginBusy(uiText("switchingInstance"))
	defer a.endBusy()
	version, err := a.host.Service().SelectVersion(key)
	if err != nil {
		a.uiUpdate(func() { a.toast(err.Error()) })
		return
	}
	a.uiUpdate(func() {
		a.applySelectedVersion(version, true)
		go a.refreshSelectedMods()
	})
}

func (a *App) toggleConnector() {
	a.beginBusy(uiText("switchingInstance"))
	defer a.endBusy()
	version, err := a.host.Service().ToggleConnectorLoader()
	if err != nil {
		a.uiUpdate(func() { a.toast(err.Error()) })
		return
	}
	a.uiUpdate(func() {
		a.applySelectedVersion(version, true)
		go a.refreshSelectedMods()
	})
}

func (a *App) searchMods(req appstructs.SearchModsRequest) {
	if a.offline {
		return
	}
	a.host.Service().SearchMods(req)
}

// refreshDownloadStates mirrors store.refreshDownloadStates. It never blocks.
func (a *App) refreshDownloadStates() {
	if a.offline {
		return
	}
	if a.search.IsSearching || a.search.IsLoadingMore {
		return
	}
	if a.search.SelectedVersion == "" || a.search.SelectedModLoader == "" {
		a.search.DownloadStates = a.search.DisabledDownloadStates("invalidRequest")
		a.refreshDownloadView()
		return
	}
	if !a.mc.HasSelectedInstance() {
		a.search.DownloadStates = a.search.DisabledDownloadStates("noSelectedVersion")
		a.refreshDownloadView()
		return
	}
	// Queue membership changes arrive in bursts (a batch install emits one per
	// mod); each lookup costs an install check per result, so collapse them.
	a.downloadStatesMu.Lock()
	if a.downloadStatesInFlight {
		a.downloadStatesDirty = true
		a.downloadStatesMu.Unlock()
		return
	}
	a.downloadStatesInFlight = true
	a.downloadStatesMu.Unlock()

	requestID := uistate.NewRequestID()
	a.search.ActiveDownloadStateRequest = requestID
	req := appstructs.DownloadStatesRequest{
		Results:          a.search.SearchResults,
		MinecraftVersion: a.search.SelectedVersion,
		ModLoader:        a.search.SelectedModLoader,
	}
	go func() {
		states := a.host.Service().GetDownloadStates(req)
		a.uiUpdate(func() {
			a.downloadStatesMu.Lock()
			a.downloadStatesInFlight = false
			dirty := a.downloadStatesDirty
			a.downloadStatesDirty = false
			a.downloadStatesMu.Unlock()
			if a.search.ActiveDownloadStateRequest == requestID {
				a.search.DownloadStates = states
				a.refreshDownloadView()
			}
			if dirty {
				a.refreshDownloadStates()
			}
		})
	}()
}

// --- queue -------------------------------------------------------------------

func (a *App) refreshQueueState() {
	if a.offline {
		return
	}
	state := a.host.Service().GetDownloadQueueState()
	a.uiUpdate(func() { a.applyQueue(state) })
}

// applyQueue projects a queue snapshot onto the drawer, dock and page labels.
func (a *App) applyQueue(state appstructs.DownloadQueueState) {
	wasActive := a.queueState.Queue.Active
	a.queueState.ApplyQueue(state)
	a.queueState.SyncPresentation()
	if !wasActive && state.Active && a.queueState.HasVisibleItems() && !a.queue.Visible() {
		a.queue.SetOpen(true)
		a.queueAutoOpened = true
		a.resizeQueue()
	}
	if a.queueAutoOpened && !state.Active && !a.queueState.HasVisibleItems() {
		a.closeQueue()
	}
	if a.queue.Visible() {
		a.queue.Refresh()
	}
	if a.search.ApplyQueueMembership(state.Items) {
		a.refreshDownloadStates()
	}
	a.refreshDownloadView()
	a.updateStatus()
}

// refreshDownloadView repaints the search page only while it is visible. Hidden
// pages are rebuilt when they are next activated, so queue progress no longer
// pays for a result list nobody is looking at.
func (a *App) refreshDownloadView() {
	if a.active == ViewDownload {
		a.downloadPage().Refresh()
	}
}

func (a *App) queueDownload(req appstructs.ModDownloadRequest) {
	if a.offline {
		return
	}
	result := a.host.Service().QueueModDownload(req)
	a.uiUpdate(func() {
		a.search.DownloadingKeys = nil
		if result.Skipped {
			a.toastDownloadSkip(result.Reason)
		} else {
			a.toastT("download.queued")
		}
		a.refreshDownloadStates()
		a.downloadPage().Refresh()
	})
	go a.refreshQueueState()
}

func (a *App) toastDownloadSkip(reason string) {
	key := "download.errors.generic"
	if suffix, ok := uistate.DownloadErrorI18nKey(reason); ok {
		key = "download.errors." + suffix
	}
	a.toastT(key)
}

func (a *App) cancelDownload(id string) {
	if a.offline {
		return
	}
	a.host.Service().CancelDownload(id)
	a.refreshQueueState()
}

func (a *App) retryDownload(id string) {
	if a.offline {
		return
	}
	a.host.Service().RetryDownload(id)
	a.refreshQueueState()
}

func (a *App) removeDownload(id string) {
	if a.offline {
		return
	}
	a.host.Service().RemoveDownload(id)
	a.refreshQueueState()
}

func (a *App) dismissOptionalReminder(id string) {
	if a.offline {
		return
	}
	a.host.Service().DismissOptionalDependencyReminder(id)
	a.refreshQueueState()
}

func (a *App) clearOptionalReminders() {
	if a.offline {
		return
	}
	a.host.Service().ClearOptionalDependencyReminders()
	a.refreshQueueState()
}

func (a *App) installOptionalDependencies(id string) {
	if a.offline {
		return
	}
	a.host.Service().InstallOptionalDependencies(id)
	a.refreshQueueState()
}

// analyzeBatchConflicts implements the batch replacement preflight.
func (a *App) analyzeBatchConflicts(req appstructs.BatchDownloadRequest) {
	analysis := a.host.Service().AnalyzeBatchIncompatibleConflicts(req)
	a.uiUpdate(func() {
		if len(analysis.Conflicts) == 0 {
			a.downloadPage().installBatch(req.Results)
			return
		}
		conflicted := map[string]bool{}
		for _, conflict := range analysis.Conflicts {
			conflicted[conflict.Key] = true
		}
		a.openConfirm(
			i18n.T("download.confirmReplace.batchIncompatibleTitle"),
			i18n.T("download.confirmReplace.batchIncompatibleBody"),
			[]confirmOption{
				{Label: i18n.T("download.confirmReplace.confirm"), Value: "continue", Primary: true},
				{Label: i18n.T("download.confirmReplace.skipConflicted"), Value: "skip"},
				{Label: i18n.T("download.confirmReplace.cancel"), Value: "cancel"},
			},
			func(value string) {
				switch value {
				case "continue":
					a.downloadPage().installBatch(req.Results)
				case "skip":
					filtered := make([]models.ModProject, 0, len(req.Results))
					for _, result := range req.Results {
						if !conflicted[a.downloadPage().itemKey(result)] {
							filtered = append(filtered, result)
						}
					}
					a.downloadPage().installBatch(filtered)
				}
			},
		)
	})
}

// --- usage / settings --------------------------------------------------------

func (a *App) refreshUsageStats() {
	if a.offline {
		return
	}
	stats := a.host.Service().GetUsageStats()
	a.uiUpdate(func() {
		a.stats = stats
		if page, ok := a.views[ViewHome].(*homePage); ok {
			page.Refresh()
		}
	})
}

func (a *App) refreshSettings() {
	if a.offline {
		return
	}
	view := a.host.Service().GetSettings()
	a.uiUpdate(func() {
		a.applySettings(view)
		a.settingsPage().Refresh()
	})
}

func (a *App) saveTheme(theme string) {
	if a.offline {
		return
	}
	next := a.host.Service().SaveTheme(theme)
	view := a.host.Service().GetSettings()
	a.uiUpdate(func() {
		if next != "" {
			view.Theme = next
		}
		a.applySettings(view)
		a.rebuildAllText()
		a.toastT("settings.theme.saved")
	})
}

func (a *App) saveLanguage(language string) {
	if a.offline {
		return
	}
	next := a.host.Service().SaveLanguage(language)
	view := a.host.Service().GetSettings()
	a.uiUpdate(func() {
		if next != "" {
			view.Language = next
		}
		a.applySettings(view)
		a.rebuildAllText()
		a.toastT("settings.language.saved")
	})
}

func (a *App) saveSimpleMode(simple bool) {
	if a.offline {
		return
	}
	view := a.host.Service().SaveSimpleModeSettings(appcore.SaveSimpleModeSettingsRequest{SimpleMode: simple})
	a.uiUpdate(func() {
		a.applySettings(view)
		a.settingsPage().Refresh()
	})
}

func (a *App) saveUnusedCleanup(autoScan bool) {
	if a.offline {
		return
	}
	view := a.host.Service().SaveUnusedDependencyCleanupSettings(appcore.SaveUnusedDependencyCleanupSettingsRequest{AutoScanUnusedDependencies: autoScan})
	a.uiUpdate(func() {
		a.applySettings(view)
		a.settingsPage().Refresh()
		a.toastT("settings.cleanup.saved")
	})
}

func (a *App) saveMCIM(enabled bool) {
	if a.offline {
		return
	}
	view := a.host.Service().SaveMCIMSettings(appcore.SaveMCIMSettingsRequest{MCIMEnabled: enabled})
	a.uiUpdate(func() {
		a.applySettings(view)
		a.settingsPage().Refresh()
		a.toastT("settings.mcim.saved")
	})
}

func (a *App) savePreferredReleaseType(releaseType string) {
	if a.offline {
		return
	}
	view := a.host.Service().SavePreferredReleaseTypeSettings(appcore.SavePreferredReleaseTypeSettingsRequest{PreferredReleaseType: releaseType})
	a.uiUpdate(func() {
		a.applySettings(view)
		a.settingsPage().Refresh()
	})
}

func (a *App) saveNetworkSettings(d uistate.SettingsDrafts) {
	if a.offline {
		return
	}
	req := uistate.ValidateNetworkSettings(d)
	view := a.host.Service().SaveNetworkSettings(req)
	a.uiUpdate(func() {
		a.applySettings(view)
		a.settingsPage().Refresh()
	})
}

func (a *App) saveCacheDir(dir string) {
	if a.offline {
		return
	}
	view := a.host.Service().SaveCacheDirPreference(dir)
	a.uiUpdate(func() {
		a.applySettings(view)
		a.settingsPage().Refresh()
		a.toastT("settings.cacheDir.saved")
	})
}

func (a *App) saveApiKeys(req appcore.SaveApiKeysRequest) {
	if a.offline {
		return
	}
	view := a.host.Service().SaveApiKeys(req)
	a.uiUpdate(func() {
		a.applySettings(view)
		a.settingsPage().cfKey = uistate.KeyDraft{}
		a.settingsPage().mrKey = uistate.KeyDraft{}
		a.settingsPage().Refresh()
		a.toastT("settings.apiKeys.saved")
	})
}

// rebuildAllText re-renders every page after a language or theme change.
func (a *App) rebuildAllText() {
	a.restyleContainers()
	a.theme.styleList(a.sidebar.List)
	a.theme.styleBox(a.sidebar.Box)
	a.sidebarFoot.SetBackgroundColor(a.theme.Surface)
	a.rebuildSidebarLabels()
	a.settingsPage().Refresh()
	for _, p := range a.views {
		if p == page(a.settingsPage()) {
			continue
		}
		if restyler, ok := p.(pageRestyler); ok {
			restyler.Restyle()
		}
		p.Refresh()
	}
	a.queue.Refresh()
	a.queue.restyle()
	a.statusBar.restyle()
	a.updateStatus()
}

// --- pins --------------------------------------------------------------------

func (a *App) refreshPinnedMods() {
	if a.offline {
		return
	}
	pins := a.host.Service().ListPinnedMods()
	a.uiUpdate(func() {
		if page, ok := a.views[ViewUnpin].(*unpinPage); ok {
			page.setPins(pins)
		}
	})
}

func (a *App) unpinMod(pin storage.PinnedMod) {
	if a.offline {
		return
	}
	a.host.Service().UnpinMod(pin.Platform, pin.ModID, pin.MinecraftVersion, pin.ModLoader)
	pins := a.host.Service().ListPinnedMods()
	a.uiUpdate(func() {
		if page, ok := a.views[ViewUnpin].(*unpinPage); ok {
			page.setPins(pins)
		}
		a.toastT("unpin.removed")
	})
}

func (a *App) unpinAll(pins []storage.PinnedMod) {
	if a.offline {
		return
	}
	svc := a.host.Service()
	for _, pin := range pins {
		svc.UnpinMod(pin.Platform, pin.ModID, pin.MinecraftVersion, pin.ModLoader)
	}
	remaining := svc.ListPinnedMods()
	a.uiUpdate(func() {
		if page, ok := a.views[ViewUnpin].(*unpinPage); ok {
			page.setPins(remaining)
		}
		a.toastT("unpin.removed")
	})
}

// batchUnpin clears pins for the selected search results.
func (a *App) batchUnpin(results []models.ModProject, mcVersion, modLoader string) {
	if a.offline {
		return
	}
	svc := a.host.Service()
	for _, result := range results {
		pin := svc.GetPinnedModVersion(result.Platform, result.ProjectID, mcVersion, modLoader)
		if pin.VersionID != "" {
			svc.PinModVersion(appstructs.ModVersionPinRequest{
				Platform:         result.Platform,
				ModID:            result.ProjectID,
				VersionID:        "",
				MinecraftVersion: mcVersion,
				ModLoader:        modLoader,
			})
		}
	}
	a.refreshDownloadStates()
}

// scanUnusedAll refreshes the instance then scans for unused dependencies.
func (a *App) scanUnusedAll() {
	if a.offline {
		return
	}
	a.host.Service().RefreshSelectedVersionMods()
	a.scanUnusedDependencies(appstructs.UnusedDependencyScanRequest{})
}

// packwizDefaultFilename is the suggested export name for a favorite list.
func (a *App) packwizDefaultFilename(listID string) string {
	if a.offline {
		return "packwiz.zip"
	}
	return a.host.Service().FavoriteListPackwizDefaultFilename(listID)
}

func (a *App) pinModVersion(req appstructs.ModVersionPinRequest) {
	pin := a.host.Service().PinModVersion(req)
	a.uiUpdate(func() {
		if a.versionDialog != nil {
			a.versionDialog.setPinned(pin)
		}
		go a.refreshDownloadStates()
	})
}

func (a *App) listMatchingVersions(project models.ModProject, mcVersion, modLoader string) {
	if a.offline {
		return
	}
	versions := a.host.Service().ListMatchingProjectVersions(project, mcVersion, modLoader)
	pin := a.host.Service().GetPinnedModVersion(project.Platform, project.ProjectID, mcVersion, modLoader)
	a.uiUpdate(func() {
		if a.versionDialog != nil && a.versionDialog.project.ProjectID == project.ProjectID {
			a.versionDialog.setVersions(versions, pin)
		}
	})
}

// --- local mods --------------------------------------------------------------

func (a *App) applyLocalBatch(paths []string, action string) {
	version, err := a.host.Service().ApplyLocalModBatchOperation(appstructs.LocalModBatchOperationRequest{Paths: paths, Action: action})
	a.uiUpdate(func() {
		if err != nil {
			a.toastT("manage.operationFailed")
			return
		}
		a.mc.ApplySelectedVersion(&version)
		a.mc.MarkSelectedModsLoaded()
		a.managePage().Refresh()
		a.updateSidebarFooter()
		go a.refreshUsageStats()
	})
}

func (a *App) analyzeDisableImpact(req appstructs.LocalModDisableImpactRequest) {
	result, err := a.host.Service().AnalyzeLocalModDisableImpact(req)
	a.uiUpdate(func() {
		if err != nil {
			a.toastT("manage.operationFailed")
			return
		}
		a.managePage().onDisableImpact(result, req)
	})
}

func (a *App) scanUnusedDependencies(req appstructs.UnusedDependencyScanRequest) {
	result, err := a.host.Service().ScanUnusedDependencies(req)
	a.uiUpdate(func() {
		if err != nil {
			a.toastT("manage.cleanup.scanFailed")
			return
		}
		a.managePage().onUnusedScan(result)
	})
}

func (a *App) restoreCachedDependency(req appstructs.RestoreCachedDependencyRequest) {
	result := a.host.Service().RestoreCachedDependency(req)
	a.uiUpdate(func() {
		if a.impactDialog != nil {
			a.impactDialog.restoreDone(req.ModID, result.Queued)
		}
	})
}

// --- favorites ---------------------------------------------------------------

func (a *App) loadFavoriteLists(listID string) {
	if a.offline {
		return
	}
	svc := a.host.Service()
	lists := svc.ListFavoriteLists()
	var contents storage.FavoriteListContents
	if listID != "" {
		contents = svc.ListFavoriteContents(listID)
	}
	a.uiUpdate(func() {
		a.favoritesPage().setLists(lists, contents, listID)
		if a.addFavoriteDialog != nil {
			a.addFavoriteDialog.refresh()
		}
	})
}

func (a *App) loadFavoriteContents(listID string) {
	contents := a.host.Service().ListFavoriteContents(listID)
	a.uiUpdate(func() {
		a.favoritesPage().setContents(listID, contents)
	})
}

func (a *App) createFavoriteList(name, mcVersion, modLoader string) {
	list := a.host.Service().CreateFavoriteListForScope(name, mcVersion, modLoader)
	svc := a.host.Service()
	lists := svc.ListFavoriteLists()
	a.uiUpdate(func() {
		a.favoritesPage().setLists(lists, storage.FavoriteListContents{}, list.ID)
		a.favoritesPage().selectedListID = list.ID
		a.favoritesPage().Refresh()
		if a.addFavoriteDialog != nil {
			a.addFavoriteDialog.refresh()
		}
	})
}

func (a *App) renameFavoriteList(id, name string) {
	a.host.Service().RenameFavoriteList(id, name)
	a.loadFavoriteLists(id)
}

func (a *App) deleteFavoriteList(id string) {
	a.host.Service().DeleteFavoriteList(id)
	a.loadFavoriteLists("")
}

func (a *App) updateFavoriteList(list storage.FavoriteList) {
	a.host.Service().UpdateFavoriteListMetadata(list)
	a.loadFavoriteLists(list.ID)
}

func (a *App) reorderFavoriteLists(ids []string) {
	a.host.Service().ReorderFavoriteLists(ids)
	a.loadFavoriteLists(a.favoritesPage().selectedListID)
}

func (a *App) addFavoriteMods(req appcore.FavoriteBulkAddRequest) {
	if a.offline {
		return
	}
	result := a.host.Service().AddFavoriteModsToLists(req)
	a.uiUpdate(func() {
		a.toastT("favorites.added")
		a.favoritesPage().Refresh()
		if a.addFavoriteDialog != nil {
			a.addFavoriteDialog.refresh()
		}
		_ = result
	})
	go a.loadFavoriteLists(a.favoritesPage().selectedListID)
}

func (a *App) removeFavoriteMods(selectedListID string, entries []storage.FavoriteModEntry) {
	svc := a.host.Service()
	for _, entry := range entries {
		listID := entry.SourceListID
		if listID == "" {
			listID = selectedListID
		}
		svc.RemoveFavoriteMod(listID, entry.Platform, entry.ModID, entry.MinecraftVersion, entry.ModLoader)
	}
	a.loadFavoriteLists(selectedListID)
}

func (a *App) exportPackwiz(listID, path, mcVersion, modLoader string) {
	_, err := a.host.Service().ExportFavoriteListPackwizZipForScope(listID, path, mcVersion, modLoader)
	a.uiUpdate(func() {
		if err != nil {
			a.toastT("favorites.export.failed", map[string]string{"reason": err.Error()})
			return
		}
		a.toastT("favorites.export.success")
		go a.refreshUsageStats()
	})
}

func (a *App) previewCrossVersionCopy(req appcore.FavoriteCrossVersionCopyRequest) {
	preview := a.host.Service().PreviewFavoriteListCrossVersionCopy(req)
	a.uiUpdate(func() {
		body := i18n.T("favorites.crossVersionCopy.target", map[string]string{
			"name":             preview.TargetListName,
			"minecraftVersion": preview.MinecraftVersion,
			"modLoader":        preview.ModLoader,
		})
		body += "\n" + i18n.T("favorites.crossVersionCopy.matched", map[string]string{"n": itoa(len(preview.Matched))})
		if len(preview.Conflicts) > 0 {
			body += "\n" + i18n.T("favorites.crossVersionCopy.conflicts", map[string]string{"n": itoa(len(preview.Conflicts))})
		}
		if preview.NameConflict {
			body += "\n" + i18n.T("favorites.crossVersionCopy.nameConflict")
		}
		a.openConfirm(
			i18n.T("favorites.actions.copyAcrossVersions"),
			body,
			[]confirmOption{
				{Label: i18n.T("favorites.actions.apply"), Value: "confirm", Primary: true},
				{Label: i18n.T("favorites.actions.cancel"), Value: "cancel"},
			},
			func(value string) {
				if value == "confirm" {
					go a.applyCrossVersionCopy(req)
				}
			},
		)
	})
}

func (a *App) applyCrossVersionCopy(req appcore.FavoriteCrossVersionCopyRequest) {
	result := a.host.Service().ApplyFavoriteListCrossVersionCopy(req)
	a.uiUpdate(func() {
		if result.Applied {
			a.toastT("favorites.crossVersionCopy.success", map[string]string{
				"name":    result.TargetList.Name,
				"added":   itoa(result.Result.Added),
				"skipped": itoa(result.Result.Skipped),
			})
		} else {
			reason := i18n.T("favorites.crossVersionCopy.unknownError")
			if len(result.Result.Errors) > 0 {
				reason = strings.Join(result.Result.Errors, "; ")
			} else if len(result.Preview.Errors) > 0 {
				reason = strings.Join(result.Preview.Errors, "; ")
			}
			a.toastT("favorites.crossVersionCopy.notApplied", map[string]string{"reason": reason})
		}
		go a.loadFavoriteLists(a.favoritesPage().selectedListID)
	})
}

func (a *App) copyFavoriteList(sourceID, targetID string) {
	result := a.host.Service().CopyFavoriteListToList(appcore.FavoriteListCopyRequest{SourceListID: sourceID, TargetListID: targetID})
	a.uiUpdate(func() {
		a.toastT("favorites.resultSummary", map[string]string{
			"added":   itoa(result.Added),
			"updated": itoa(result.Updated),
			"skipped": itoa(result.Skipped),
		})
		go a.loadFavoriteLists(a.favoritesPage().selectedListID)
	})
}

func (a *App) addFavoriteReference(parentID, childID string) {
	a.host.Service().AddFavoriteListReference(parentID, childID)
	a.uiUpdate(func() {
		a.toastT("favorites.references.added")
		go a.loadFavoriteLists(parentID)
	})
}

func (a *App) removeFavoriteReference(parentID, childID string) {
	a.host.Service().RemoveFavoriteListReference(parentID, childID)
	a.uiUpdate(func() {
		go a.loadFavoriteLists(parentID)
		go a.loadFavoriteRefs(parentID)
	})
}

func (a *App) loadFavoriteRefs(parentID string) {
	if a.offline {
		return
	}
	svc := a.host.Service()
	refs := svc.ListFavoriteListRefs(parentID)
	lists := svc.ListFavoriteLists()
	a.uiUpdate(func() {
		if a.refsDialog != nil {
			a.refsDialog.setRefs(refs, lists)
		}
	})
}

func (a *App) setFavoriteListIcon(list storage.FavoriteList, mode, value string) {
	iconURL := ""
	if mode == "project" && strings.TrimSpace(value) != "" {
		if project, ok := a.host.Service().LookupProjectBySlug("modrinth", value, "", ""); ok {
			iconURL = project.IconURL
		}
	}
	if mode == "" {
		value = ""
	}
	list.IconKind = mode
	list.IconValue = value
	list.IconURL = iconURL
	a.host.Service().UpdateFavoriteListMetadata(list)
	a.loadFavoriteLists(list.ID)
}
