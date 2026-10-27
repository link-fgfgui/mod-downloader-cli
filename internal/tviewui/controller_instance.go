package tviewui

// --- instance ----------------------------------------------------------------

func (a *App) refreshMinecraft() {
	svc := a.service()
	if svc == nil {
		return
	}
	a.beginBusy(uiText("refreshing"))
	defer a.endBusy()
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
	svc := a.service()
	if svc == nil {
		return
	}
	a.beginBusy(uiText("refreshing"))
	defer a.endBusy()
	version := svc.RefreshSelectedVersionMods()
	a.uiUpdate(func() {
		a.mc.ApplySelectedVersion(&version)
		a.mc.MarkSelectedModsLoaded()
		a.updateSidebarFooter()
		a.managePage().Refresh()
		a.downloadPage().Refresh()
	})
}

func (a *App) setMinecraftDir(dir string) {
	svc := a.service()
	if svc == nil {
		return
	}
	svc.SetMinecraftDir(dir)
	a.refreshMinecraft()
	a.refreshSettings()
}

func (a *App) selectVersion(key string) {
	svc := a.service()
	if svc == nil {
		return
	}
	a.beginBusy(uiText("switchingInstance"))
	defer a.endBusy()
	version, err := svc.SelectVersion(key)
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
	svc := a.service()
	if svc == nil {
		return
	}
	a.beginBusy(uiText("switchingInstance"))
	defer a.endBusy()
	version, err := svc.ToggleConnectorLoader()
	if err != nil {
		a.uiUpdate(func() { a.toast(err.Error()) })
		return
	}
	a.uiUpdate(func() {
		a.applySelectedVersion(version, true)
		go a.refreshSelectedMods()
	})
}
