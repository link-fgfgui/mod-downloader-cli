package tviewui

import (
	"github.com/link-fgfgui/mod-downloader/core/appcore"
	"github.com/link-fgfgui/mod-downloader/core/host"
	appstructs "github.com/link-fgfgui/mod-downloader/core/structs"
	structs "github.com/link-fgfgui/mod-downloader/core/structs/minecraft"
	appview "github.com/link-fgfgui/mod-downloader/core/structs/view"

	"github.com/link-fgfgui/mod-downloader-cli/internal/i18n"
)

// --- events ------------------------------------------------------------------

// service returns the started core service, or nil for offline test apps and
// hosts whose Start has not succeeded. Every controller method treats nil as
// "no backend" and returns without side effects.
func (a *App) service() *appcore.Service {
	if a.offline {
		return nil
	}
	return a.host.Service()
}

// registerEvents wires the typed core events into the batching pump. Handlers
// run on the emitting goroutine, so they only enqueue work for the UI goroutine.
func (a *App) registerEvents(h *host.Host) {
	host.On(h, appcore.EventSearchModsUpdated, func(update appview.SearchModsUpdate) {
		a.enqueueEvent(false, func() { a.handleSearchUpdate(update) })
	})
	host.OnEvent(h, appcore.EventDownloadStatesUpdated, func() {
		a.enqueueEvent(false, a.handleDownloadStatesUpdated)
	})
	host.On(h, appcore.EventDownloadQueueUpdated, func(state appview.DownloadQueueState) {
		a.enqueueEvent(true, func() { a.handleQueueUpdate(state) })
	})
	host.On(h, appcore.EventDownloadFailed, func(failed appstructs.DownloadFailedEvent) {
		a.enqueueEvent(false, func() { a.handleDownloadFailed(failed) })
	})
	host.OnEvent(h, appcore.EventDownloadCompleted, func() {
		a.enqueueEvent(false, a.handleDownloadCompleted)
	})
	host.On(h, appcore.EventMinecraftDirChanged, func(string) {
		a.enqueueEvent(false, a.handleMinecraftDirChanged)
	})
	host.On(h, appcore.EventSelectedVersionChanged, func(version structs.VersionInfo) {
		a.enqueueEvent(false, func() { a.handleSelectedVersionChanged(version) })
	})
	host.OnEvent(h, appcore.EventUsageStatsUpdated, func() {
		a.enqueueEvent(false, a.handleUsageStatsUpdated)
	})
	host.On(h, appcore.EventExtensionModsAccepted, func(update appview.SearchModsUpdate) {
		a.enqueueEvent(false, func() { a.handleExtensionModsAccepted(update) })
	})
}

// handleSearchUpdate merges a typed search page update into the download view.
func (a *App) handleSearchUpdate(update appview.SearchModsUpdate) {
	accepted, refresh := a.search.ApplySearchUpdate(update)
	if accepted {
		a.refreshDownloadView()
	}
	if refresh {
		a.refreshDownloadStates()
	}
}

func (a *App) handleDownloadStatesUpdated() { a.refreshDownloadStates() }

func (a *App) handleQueueUpdate(state appview.DownloadQueueState) { a.applyQueue(state) }

func (a *App) handleDownloadFailed(failed appstructs.DownloadFailedEvent) {
	fileName := failed.FileName
	if fileName == "" {
		fileName = i18n.T("download.errors.unknownFile")
	}
	reason := failed.Reason
	if reason == "" {
		reason = i18n.T("download.errors.generic")
	}
	a.toastT("download.errors.failedWithReason", map[string]string{"fileName": fileName, "reason": reason})
	a.refreshDownloadStates()
}

func (a *App) handleDownloadCompleted() { a.queueState.NoteDownloadCompleted() }

func (a *App) handleMinecraftDirChanged() { go a.refreshMinecraft() }

func (a *App) handleSelectedVersionChanged(version structs.VersionInfo) {
	a.applySelectedVersion(version, true)
}

func (a *App) handleUsageStatsUpdated() { go a.refreshUsageStats() }

func (a *App) handleExtensionModsAccepted(update appview.SearchModsUpdate) {
	if a.search.ApplyExtensionResults(update.Results) {
		a.switchView(ViewDownload)
		a.downloadPage().Refresh()
		a.toastT("download.queued")
		a.refreshDownloadStates()
	}
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

// applyQueue projects a queue snapshot onto the drawer, dock and page labels.
func (a *App) applyQueue(state appview.DownloadQueueState) {
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
