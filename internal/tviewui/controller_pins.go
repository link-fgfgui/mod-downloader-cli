package tviewui

import (
	"github.com/link-fgfgui/mod-downloader-core/models"
	"github.com/link-fgfgui/mod-downloader-core/storage"
	appstructs "github.com/link-fgfgui/mod-downloader-core/structs"
)

// --- pins --------------------------------------------------------------------

func (a *App) refreshPinnedMods() {
	svc := a.service()
	if svc == nil {
		return
	}
	pins := svc.ListPinnedMods()
	a.uiUpdate(func() {
		if page, ok := a.views[ViewUnpin].(*unpinPage); ok {
			page.setPins(pins)
		}
	})
}

func (a *App) unpinMod(pin storage.PinnedMod) {
	svc := a.service()
	if svc == nil {
		return
	}
	svc.UnpinMod(pin.Platform, pin.ModID, pin.MinecraftVersion, pin.ModLoader)
	pins := svc.ListPinnedMods()
	a.uiUpdate(func() {
		if page, ok := a.views[ViewUnpin].(*unpinPage); ok {
			page.setPins(pins)
		}
		a.toastT("unpin.removed")
	})
}

func (a *App) unpinAll(pins []storage.PinnedMod) {
	svc := a.service()
	if svc == nil {
		return
	}
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
	svc := a.service()
	if svc == nil {
		return
	}
	for _, result := range results {
		svc.UnpinMod(result.Platform, result.ProjectID, mcVersion, modLoader)
	}
	a.refreshDownloadStates()
}

func (a *App) pinModVersion(req appstructs.ModVersionPinRequest) {
	svc := a.service()
	if svc == nil {
		return
	}
	pin := svc.PinModVersion(req)
	a.uiUpdate(func() {
		if a.versionDialog != nil {
			a.versionDialog.setPinned(pin)
		}
		go a.refreshDownloadStates()
	})
}

func (a *App) listMatchingVersions(project models.ModProject, mcVersion, modLoader string) {
	svc := a.service()
	if svc == nil {
		return
	}
	versions := svc.ListMatchingProjectVersions(project, mcVersion, modLoader)
	pin := svc.GetPinnedModVersion(project.Platform, project.ProjectID, mcVersion, modLoader)
	a.uiUpdate(func() {
		if a.versionDialog != nil && a.versionDialog.project.ProjectID == project.ProjectID {
			a.versionDialog.setVersions(versions, pin)
		}
	})
}
