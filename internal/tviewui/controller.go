package tviewui

import (
	"github.com/link-fgfgui/mod-downloader-core/appcore"
	appstructs "github.com/link-fgfgui/mod-downloader-core/structs"
	structs "github.com/link-fgfgui/mod-downloader-core/structs/minecraft"

	"github.com/link-fgfgui/mod-downloader-cli/internal/i18n"
	"github.com/link-fgfgui/mod-downloader-cli/internal/tuihost"
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
