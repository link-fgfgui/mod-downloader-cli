package tviewui

import (
	"github.com/link-fgfgui/mod-downloader-core/models"
	appstructs "github.com/link-fgfgui/mod-downloader-core/structs"

	"github.com/link-fgfgui/mod-downloader-cli/internal/i18n"
	"github.com/link-fgfgui/mod-downloader-cli/internal/uistate"
)

// --- download / search -------------------------------------------------------

func (a *App) searchMods(req appstructs.SearchModsRequest) {
	svc := a.service()
	if svc == nil {
		return
	}
	svc.SearchMods(req)
}

// refreshDownloadStates mirrors store.refreshDownloadStates. It never blocks.
func (a *App) refreshDownloadStates() {
	svc := a.service()
	if svc == nil {
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
		states := svc.GetDownloadStates(req)
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

func (a *App) refreshQueueState() {
	svc := a.service()
	if svc == nil {
		return
	}
	state := svc.GetDownloadQueueState()
	a.uiUpdate(func() { a.applyQueue(state) })
}

func (a *App) queueDownload(req appstructs.ModDownloadRequest) {
	svc := a.service()
	if svc == nil {
		return
	}
	result := svc.QueueModDownload(req)
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
	svc := a.service()
	if svc == nil {
		return
	}
	svc.CancelDownload(id)
	a.refreshQueueState()
}

func (a *App) retryDownload(id string) {
	svc := a.service()
	if svc == nil {
		return
	}
	svc.RetryDownload(id)
	a.refreshQueueState()
}

func (a *App) removeDownload(id string) {
	svc := a.service()
	if svc == nil {
		return
	}
	svc.RemoveDownload(id)
	a.refreshQueueState()
}

func (a *App) dismissOptionalReminder(id string) {
	svc := a.service()
	if svc == nil {
		return
	}
	svc.DismissOptionalDependencyReminder(id)
	a.refreshQueueState()
}

func (a *App) clearOptionalReminders() {
	svc := a.service()
	if svc == nil {
		return
	}
	svc.ClearOptionalDependencyReminders()
	a.refreshQueueState()
}

func (a *App) installOptionalDependencies(id string) {
	svc := a.service()
	if svc == nil {
		return
	}
	svc.InstallOptionalDependencies(id)
	a.refreshQueueState()
}

// analyzeBatchConflicts implements the batch replacement preflight.
func (a *App) analyzeBatchConflicts(req appstructs.BatchDownloadRequest) {
	svc := a.service()
	if svc == nil {
		return
	}
	analysis := svc.AnalyzeBatchIncompatibleConflicts(req)
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
