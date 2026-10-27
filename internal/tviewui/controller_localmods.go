package tviewui

import (
	appstructs "github.com/link-fgfgui/mod-downloader-core/structs"
)

// --- local mods --------------------------------------------------------------

// scanUnusedAll refreshes the instance then scans for unused dependencies.
func (a *App) scanUnusedAll() {
	svc := a.service()
	if svc == nil {
		return
	}
	svc.RefreshSelectedVersionMods()
	a.scanUnusedDependencies(appstructs.UnusedDependencyScanRequest{})
}

func (a *App) applyLocalBatch(paths []string, action string) {
	svc := a.service()
	if svc == nil {
		return
	}
	version, err := svc.ApplyLocalModBatchOperation(appstructs.LocalModBatchOperationRequest{Paths: paths, Action: action})
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
	svc := a.service()
	if svc == nil {
		return
	}
	result, err := svc.AnalyzeLocalModDisableImpact(req)
	a.uiUpdate(func() {
		if err != nil {
			a.toastT("manage.operationFailed")
			return
		}
		a.managePage().onDisableImpact(result, req)
	})
}

func (a *App) scanUnusedDependencies(req appstructs.UnusedDependencyScanRequest) {
	svc := a.service()
	if svc == nil {
		return
	}
	result, err := svc.ScanUnusedDependencies(req)
	a.uiUpdate(func() {
		if err != nil {
			a.toastT("manage.cleanup.scanFailed")
			return
		}
		a.managePage().onUnusedScan(result)
	})
}

func (a *App) restoreCachedDependency(req appstructs.RestoreCachedDependencyRequest) {
	svc := a.service()
	if svc == nil {
		return
	}
	result := svc.RestoreCachedDependency(req)
	a.uiUpdate(func() {
		if a.impactDialog != nil {
			a.impactDialog.restoreDone(req.ModID, result.Queued)
		}
	})
}
