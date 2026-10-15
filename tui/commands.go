package tui

import (
	"context"
	"strings"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/link-fgfgui/mod-downloader-core/appcore"
	"github.com/link-fgfgui/mod-downloader-core/models"
	"github.com/link-fgfgui/mod-downloader-core/storage"
	appstructs "github.com/link-fgfgui/mod-downloader-core/structs"
)

// All commands below run the (potentially blocking, network-bound) core calls
// off the Bubble Tea main loop and return a single result message. Core events
// emitted during these calls are bridged separately by Host.Send.

func cmdLoadMinecraft(h *Host) tea.Cmd {
	return func() tea.Msg {
		return minecraftSnapshotMsg{
			Versions:     h.Service().RefreshVersions(),
			Selected:     h.Service().GetSelectedVersion(),
			ReleaseList:  h.Service().GetMinecraftReleaseVersions(),
			MinecraftDir: h.Service().GetMinecraftDir(),
		}
	}
}

func cmdRefreshMinecraftDir(h *Host) tea.Cmd {
	return func() tea.Msg {
		return minecraftSnapshotMsg{
			Versions:     h.Service().RefreshVersions(),
			Selected:     h.Service().GetSelectedVersion(),
			ReleaseList:  h.Service().GetMinecraftReleaseVersions(),
			MinecraftDir: h.Service().GetMinecraftDir(),
		}
	}
}

func cmdRefreshSelectedMods(h *Host) tea.Cmd {
	return func() tea.Msg {
		version := h.Service().RefreshSelectedVersionMods()
		return localModsMsg{Version: version}
	}
}

func cmdSetMinecraftDir(h *Host, dir string) tea.Cmd {
	return func() tea.Msg {
		h.Service().SetMinecraftDir(dir)
		return minecraftSnapshotMsg{
			Versions:     h.Service().RefreshVersions(),
			Selected:     h.Service().GetSelectedVersion(),
			ReleaseList:  h.Service().GetMinecraftReleaseVersions(),
			MinecraftDir: h.Service().GetMinecraftDir(),
		}
	}
}

func cmdSelectVersion(h *Host, key string) tea.Cmd {
	return func() tea.Msg {
		version, err := h.Service().SelectVersion(key)
		if err != nil {
			return errMsg{Context: "select version", Err: err}
		}
		return SelectedVersionChangedMsg{Version: version}
	}
}

func cmdToggleConnector(h *Host) tea.Cmd {
	return func() tea.Msg {
		version, err := h.Service().ToggleConnectorLoader()
		if err != nil {
			return errMsg{Context: "toggle connector", Err: err}
		}
		return SelectedVersionChangedMsg{Version: version}
	}
}

func cmdSearchMods(h *Host, req appstructs.SearchModsRequest) tea.Cmd {
	return func() tea.Msg {
		h.Service().SearchMods(req)
		return nil
	}
}

func cmdGetDownloadStates(h *Host, requestID string, req appstructs.DownloadStatesRequest) tea.Cmd {
	return func() tea.Msg {
		states := h.Service().GetDownloadStates(req)
		return downloadStatesMsg{RequestID: requestID, States: states}
	}
}

func cmdGetQueueState(h *Host) tea.Cmd {
	return func() tea.Msg {
		return DownloadQueueUpdatedMsg{State: h.Service().GetDownloadQueueState()}
	}
}

func cmdGetUsageStats(h *Host) tea.Cmd {
	return func() tea.Msg {
		return UsageStatsUpdatedMsg{Stats: h.Service().GetUsageStats()}
	}
}

func cmdGetSettings(h *Host) tea.Cmd {
	return func() tea.Msg {
		return settingsLoadedMsg{View: h.Service().GetSettings()}
	}
}

func cmdQueueModDownload(h *Host, req appstructs.ModDownloadRequest) tea.Cmd {
	return func() tea.Msg {
		result := h.Service().QueueModDownload(req)
		if result.Skipped {
			return downloadSkipMsg{Result: result}
		}
		return downloadQueuedMsg{Result: result}
	}
}

// downloadQueuedMsg signals a successful queue operation.
type downloadQueuedMsg struct{ Result appstructs.ModDownloadResult }

// downloadSkipMsg signals a skipped queue operation with a reason.
type downloadSkipMsg struct{ Result appstructs.ModDownloadResult }

func cmdCancelDownload(h *Host, id string) tea.Cmd {
	return func() tea.Msg {
		h.Service().CancelDownload(id)
		return queueRefreshMsg{}
	}
}

func cmdRetryDownload(h *Host, id string) tea.Cmd {
	return func() tea.Msg {
		h.Service().RetryDownload(id)
		return queueRefreshMsg{}
	}
}

func cmdRemoveDownload(h *Host, id string) tea.Cmd {
	return func() tea.Msg {
		h.Service().RemoveDownload(id)
		return queueRefreshMsg{}
	}
}

func cmdDismissOptionalReminder(h *Host, id string) tea.Cmd {
	return func() tea.Msg {
		h.Service().DismissOptionalDependencyReminder(id)
		return queueRefreshMsg{}
	}
}

func cmdClearOptionalReminders(h *Host) tea.Cmd {
	return func() tea.Msg {
		h.Service().ClearOptionalDependencyReminders()
		return queueRefreshMsg{}
	}
}

func cmdInstallOptionalDependencies(h *Host, id string) tea.Cmd {
	return func() tea.Msg {
		h.Service().InstallOptionalDependencies(id)
		return queueRefreshMsg{}
	}
}

type queueRefreshMsg struct{}

func cmdAnalyzeBatchConflicts(h *Host, req appstructs.BatchDownloadRequest) tea.Cmd {
	return func() tea.Msg {
		return batchConflictMsg{Analysis: h.Service().AnalyzeBatchIncompatibleConflicts(req), Request: req}
	}
}

type batchConflictMsg struct {
	Analysis appstructs.BatchIncompatibleAnalysis
	Request  appstructs.BatchDownloadRequest
}

func cmdApplyLocalModBatch(h *Host, req appstructs.LocalModBatchOperationRequest) tea.Cmd {
	return func() tea.Msg {
		version, err := h.Service().ApplyLocalModBatchOperation(req)
		if err != nil {
			return errMsg{Context: "apply local mod batch operation", Err: err}
		}
		return localModsMsg{Version: version}
	}
}

func cmdAnalyzeDisableImpact(h *Host, req appstructs.LocalModDisableImpactRequest) tea.Cmd {
	return func() tea.Msg {
		result, err := h.Service().AnalyzeLocalModDisableImpact(req)
		if err != nil {
			return errMsg{Context: "analyze disable impact", Err: err}
		}
		return disableImpactMsg{Result: result, Request: req}
	}
}

type disableImpactMsg struct {
	Result  appstructs.LocalModDisableImpactResult
	Request appstructs.LocalModDisableImpactRequest
}

func cmdScanUnusedDependencies(h *Host, req appstructs.UnusedDependencyScanRequest) tea.Cmd {
	return func() tea.Msg {
		result, err := h.Service().ScanUnusedDependencies(req)
		if err != nil {
			return errMsg{Context: "scan unused dependencies", Err: err}
		}
		return unusedScanMsg{Result: result}
	}
}

type unusedScanMsg struct {
	Result appstructs.UnusedDependencyScanResult
}

func cmdRestoreCachedDependency(h *Host, req appstructs.RestoreCachedDependencyRequest) tea.Cmd {
	return func() tea.Msg {
		return restoreDependencyMsg{Result: h.Service().RestoreCachedDependency(req)}
	}
}

type restoreDependencyMsg struct{ Result appstructs.ModDownloadResult }

func cmdListMatchingVersions(h *Host, project models.ModProject, mcVersion, modLoader string) tea.Cmd {
	return func() tea.Msg {
		return matchingVersionsMsg{
			Project:  project,
			Versions: h.Service().ListMatchingProjectVersions(project, mcVersion, modLoader),
			Pin:      h.Service().GetPinnedModVersion(project.Platform, project.ProjectID, mcVersion, modLoader),
		}
	}
}

type matchingVersionsMsg struct {
	Project  models.ModProject
	Versions []models.ModVersion
	Pin      storage.PinnedMod
}

func cmdPinModVersion(h *Host, req appstructs.ModVersionPinRequest) tea.Cmd {
	return func() tea.Msg {
		return pinnedMsg{Pin: h.Service().PinModVersion(req)}
	}
}

type pinnedMsg struct{ Pin storage.PinnedMod }

func cmdListFavoriteLists(h *Host, listID string) tea.Cmd {
	return func() tea.Msg {
		var contents storage.FavoriteListContents
		if listID != "" {
			contents = h.Service().ListFavoriteContents(listID)
		}
		return favoritesLoadedMsg{Lists: h.Service().ListFavoriteLists(), Contents: contents, ListID: listID}
	}
}

func cmdListPinnedMods(h *Host) tea.Cmd {
	return func() tea.Msg {
		return pinnedModsMsg{Pins: h.Service().ListPinnedMods()}
	}
}

func cmdUnpinMod(h *Host, platform, modID, mcVersion, modLoader string) tea.Cmd {
	return func() tea.Msg {
		h.Service().UnpinMod(platform, modID, mcVersion, modLoader)
		return pinnedModsMsg{Pins: h.Service().ListPinnedMods()}
	}
}

// settings command results -----------------------------------------------------

type settingsSavedMsg struct {
	View    appcore.SettingsView
	Theme   string
	Lang    string
	Toast   string
	ToastKo toastColor
}

func cmdSaveTheme(h *Host, theme string) tea.Cmd {
	return func() tea.Msg {
		next := h.Service().SaveTheme(theme)
		return settingsSavedMsg{View: h.Service().GetSettings(), Theme: next, Toast: "settings.theme.saved"}
	}
}

func cmdSaveLanguage(h *Host, lang string) tea.Cmd {
	return func() tea.Msg {
		next := h.Service().SaveLanguage(lang)
		return settingsSavedMsg{View: h.Service().GetSettings(), Lang: next, Toast: "settings.language.saved"}
	}
}

func cmdSaveAnimationSettings(h *Host, req appcore.SaveAnimationSettingsRequest) tea.Cmd {
	return func() tea.Msg {
		return settingsSavedMsg{View: h.Service().SaveAnimationSettings(req), Toast: "settings.animations.saved"}
	}
}

func cmdSaveSimpleModeSettings(h *Host, simple bool) tea.Cmd {
	return func() tea.Msg {
		return settingsSavedMsg{View: h.Service().SaveSimpleModeSettings(appcore.SaveSimpleModeSettingsRequest{SimpleMode: simple})}
	}
}

func cmdSaveUnusedCleanupSettings(h *Host, autoScan bool) tea.Cmd {
	return func() tea.Msg {
		return settingsSavedMsg{View: h.Service().SaveUnusedDependencyCleanupSettings(appcore.SaveUnusedDependencyCleanupSettingsRequest{AutoScanUnusedDependencies: autoScan}), Toast: "settings.cleanup.saved"}
	}
}

func cmdSaveMCIMSettings(h *Host, enabled bool) tea.Cmd {
	return func() tea.Msg {
		return settingsSavedMsg{View: h.Service().SaveMCIMSettings(appcore.SaveMCIMSettingsRequest{MCIMEnabled: enabled}), Toast: "settings.mcim.saved"}
	}
}

func cmdSavePreferredReleaseType(h *Host, releaseType string) tea.Cmd {
	return func() tea.Msg {
		return settingsSavedMsg{View: h.Service().SavePreferredReleaseTypeSettings(appcore.SavePreferredReleaseTypeSettingsRequest{PreferredReleaseType: releaseType})}
	}
}

func cmdSaveNetworkSettings(h *Host, req appcore.SaveNetworkSettingsRequest) tea.Cmd {
	return func() tea.Msg {
		return settingsSavedMsg{View: h.Service().SaveNetworkSettings(req)}
	}
}

func cmdSaveCacheDir(h *Host, dir string) tea.Cmd {
	return func() tea.Msg {
		return settingsSavedMsg{View: h.Service().SaveCacheDirPreference(dir), Toast: "settings.cacheDir.saved"}
	}
}

func cmdSaveApiKeys(h *Host, req appcore.SaveApiKeysRequest) tea.Cmd {
	return func() tea.Msg {
		return settingsSavedMsg{View: h.Service().SaveApiKeys(req), Toast: "settings.apiKeys.saved"}
	}
}

// favorites commands -----------------------------------------------------------

func cmdCreateFavoriteList(h *Host, name, mcVersion, modLoader string) tea.Cmd {
	return func() tea.Msg {
		list := h.Service().CreateFavoriteListForScope(name, mcVersion, modLoader)
		return favoritesLoadedMsg{Lists: h.Service().ListFavoriteLists(), ListID: list.ID}
	}
}

func cmdRenameFavoriteList(h *Host, id, name string) tea.Cmd {
	return func() tea.Msg {
		h.Service().RenameFavoriteList(id, name)
		return favoritesLoadedMsg{Lists: h.Service().ListFavoriteLists()}
	}
}

func cmdDeleteFavoriteList(h *Host, id string) tea.Cmd {
	return func() tea.Msg {
		h.Service().DeleteFavoriteList(id)
		return favoritesLoadedMsg{Lists: h.Service().ListFavoriteLists()}
	}
}

func cmdUpdateFavoriteMetadata(h *Host, list storage.FavoriteList) tea.Cmd {
	return func() tea.Msg {
		h.Service().UpdateFavoriteListMetadata(list)
		return favoritesLoadedMsg{Lists: h.Service().ListFavoriteLists()}
	}
}

func cmdReorderFavoriteLists(h *Host, ids []string) tea.Cmd {
	return func() tea.Msg {
		h.Service().ReorderFavoriteLists(ids)
		return favoritesLoadedMsg{Lists: h.Service().ListFavoriteLists()}
	}
}

func cmdRemoveFavoriteMod(h *Host, listID, platform, modID, mcVersion, modLoader string) tea.Cmd {
	return func() tea.Msg {
		h.Service().RemoveFavoriteMod(listID, platform, modID, mcVersion, modLoader)
		return favoritesLoadedMsg{Lists: h.Service().ListFavoriteLists(), Contents: h.Service().ListFavoriteContents(listID), ListID: listID}
	}
}

func cmdExportPackwiz(h *Host, listID, path, mcVersion, modLoader string) tea.Cmd {
	return func() tea.Msg {
		result, err := h.Service().ExportFavoriteListPackwizZipForScope(listID, path, mcVersion, modLoader)
		if err != nil {
			return errMsg{Context: "export packwiz", Err: err}
		}
		return exportPackwizMsg{Path: result.Path}
	}
}

type exportPackwizMsg struct{ Path string }

func cmdPreviewCrossVersionCopy(h *Host, req appcore.FavoriteCrossVersionCopyRequest) tea.Cmd {
	return func() tea.Msg {
		return crossVersionPreviewMsg{Preview: h.Service().PreviewFavoriteListCrossVersionCopy(req), Request: req}
	}
}

type crossVersionPreviewMsg struct {
	Preview appcore.FavoriteCrossVersionCopyPreview
	Request appcore.FavoriteCrossVersionCopyRequest
}

func cmdApplyCrossVersionCopy(h *Host, req appcore.FavoriteCrossVersionCopyRequest) tea.Cmd {
	return func() tea.Msg {
		result := h.Service().ApplyFavoriteListCrossVersionCopy(req)
		return crossVersionAppliedMsg{Result: result}
	}
}

type crossVersionAppliedMsg struct {
	Result appcore.FavoriteCrossVersionCopyApplyResult
}

func cmdCopyListToList(h *Host, sourceID, targetID string) tea.Cmd {
	return func() tea.Msg {
		result := h.Service().CopyFavoriteListToList(appcore.FavoriteListCopyRequest{SourceListID: sourceID, TargetListID: targetID})
		return bulkFavoriteResultMsg{Result: result}
	}
}

func cmdAddListReference(h *Host, parentID, childID string) tea.Cmd {
	return func() tea.Msg {
		h.Service().AddFavoriteListReference(parentID, childID)
		return favoritesLoadedMsg{Lists: h.Service().ListFavoriteLists(), Contents: h.Service().ListFavoriteContents(parentID), ListID: parentID}
	}
}

// cmdListFavoriteRefs loads the reference relations of a parent list.
func cmdListFavoriteRefs(h *Host, parentID string) tea.Cmd {
	return func() tea.Msg {
		return favoriteRefsMsg{
			ParentID: parentID,
			Refs:     h.Service().ListFavoriteListRefs(parentID),
			Lists:    h.Service().ListFavoriteLists(),
		}
	}
}

// cmdRemoveFavoriteListRef drops a parent→child reference and refreshes both.
func cmdRemoveFavoriteListRef(h *Host, parentID, childID string) tea.Cmd {
	return func() tea.Msg {
		h.Service().RemoveFavoriteListReference(parentID, childID)
		return favoritesLoadedMsg{Lists: h.Service().ListFavoriteLists(), Contents: h.Service().ListFavoriteContents(parentID), ListID: parentID}
	}
}

// cmdSetFavoriteListIcon mirrors favorites.updateListIcon / clearListIcon: for
// the "project" mode it resolves the project icon via LookupProjectBySlug, for
// "mdi" it stores the raw icon name, and an empty mode clears the icon.
func cmdSetFavoriteListIcon(h *Host, list storage.FavoriteList, mode, value, platform string) tea.Cmd {
	return func() tea.Msg {
		iconURL := ""
		if mode == "project" && strings.TrimSpace(value) != "" {
			if project, ok := h.Service().LookupProjectBySlug(platform, value, "", ""); ok {
				iconURL = project.IconURL
			}
		}
		if mode == "" {
			value = ""
		}
		list.IconKind = mode
		list.IconValue = value
		list.IconURL = iconURL
		h.Service().UpdateFavoriteListMetadata(list)
		return favoritesLoadedMsg{Lists: h.Service().ListFavoriteLists(), Contents: h.Service().ListFavoriteContents(list.ID), ListID: list.ID}
	}
}

type bulkFavoriteResultMsg struct {
	Result appcore.FavoriteBulkOperationResult
}

func cmdAddFavoriteMods(h *Host, req appcore.FavoriteBulkAddRequest) tea.Cmd {
	return func() tea.Msg {
		result := h.Service().AddFavoriteModsToLists(req)
		return bulkFavoriteResultMsg{Result: result}
	}
}

// install-and-wait helper (used by CLI compatibility / non-interactive paths).
func cmdInstallModAndWait(ctx context.Context, h *Host, req appstructs.ModDownloadRequest) tea.Cmd {
	return func() tea.Msg {
		return installWaitMsg{Result: h.Service().InstallModAndWait(ctx, req)}
	}
}

type installWaitMsg struct{ Result appcore.InstallWaitResult }

// cmdLookupProjectBySlug resolves a slug to a project (used by icon lookup).
func cmdLookupProjectBySlug(h *Host, platform, slug string) tea.Cmd {
	return func() tea.Msg {
		resolved, ok := h.Service().LookupProjectBySlug(platform, slug, "", "")
		if !ok {
			return errMsg{Context: "lookup project"}
		}
		return projectResolvedMsg{Project: resolved}
	}
}

type projectResolvedMsg struct{ Project models.ModProject }
