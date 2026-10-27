package tviewui

import (
	appstructs "github.com/link-fgfgui/mod-downloader-core/structs"

	"github.com/link-fgfgui/mod-downloader-cli/internal/uistate"
)

// --- usage / settings --------------------------------------------------------

func (a *App) refreshUsageStats() {
	svc := a.service()
	if svc == nil {
		return
	}
	stats := svc.GetUsageStats()
	a.uiUpdate(func() {
		a.stats = stats
		if page, ok := a.views[ViewHome].(*homePage); ok {
			page.Refresh()
		}
	})
}

func (a *App) refreshSettings() {
	svc := a.service()
	if svc == nil {
		return
	}
	view := svc.GetSettings()
	a.uiUpdate(func() {
		a.applySettings(view)
		a.settingsPage().Refresh()
	})
}

func (a *App) saveTheme(theme string) {
	svc := a.service()
	if svc == nil {
		return
	}
	next := svc.SaveTheme(theme)
	view := svc.GetSettings()
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
	svc := a.service()
	if svc == nil {
		return
	}
	next := svc.SaveLanguage(language)
	view := svc.GetSettings()
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
	svc := a.service()
	if svc == nil {
		return
	}
	view := svc.SaveSimpleModeSettings(appstructs.SaveSimpleModeSettingsRequest{SimpleMode: simple})
	a.uiUpdate(func() {
		a.applySettings(view)
		a.settingsPage().Refresh()
	})
}

func (a *App) saveUnusedCleanup(autoScan bool) {
	svc := a.service()
	if svc == nil {
		return
	}
	view := svc.SaveUnusedDependencyCleanupSettings(appstructs.SaveUnusedDependencyCleanupSettingsRequest{AutoScanUnusedDependencies: autoScan})
	a.uiUpdate(func() {
		a.applySettings(view)
		a.settingsPage().Refresh()
		a.toastT("settings.cleanup.saved")
	})
}

func (a *App) saveMCIM(enabled bool) {
	svc := a.service()
	if svc == nil {
		return
	}
	view := svc.SaveMCIMSettings(appstructs.SaveMCIMSettingsRequest{MCIMEnabled: enabled})
	a.uiUpdate(func() {
		a.applySettings(view)
		a.settingsPage().Refresh()
		a.toastT("settings.mcim.saved")
	})
}

func (a *App) savePreferredReleaseType(releaseType string) {
	svc := a.service()
	if svc == nil {
		return
	}
	view := svc.SavePreferredReleaseTypeSettings(appstructs.SavePreferredReleaseTypeSettingsRequest{PreferredReleaseType: releaseType})
	a.uiUpdate(func() {
		a.applySettings(view)
		a.settingsPage().Refresh()
	})
}

func (a *App) saveNetworkSettings(d uistate.SettingsDrafts) {
	svc := a.service()
	if svc == nil {
		return
	}
	req := uistate.ValidateNetworkSettings(d)
	view := svc.SaveNetworkSettings(req)
	a.uiUpdate(func() {
		a.applySettings(view)
		a.settingsPage().Refresh()
	})
}

func (a *App) saveCacheDir(dir string) {
	svc := a.service()
	if svc == nil {
		return
	}
	view := svc.SaveCacheDirPreference(dir)
	a.uiUpdate(func() {
		a.applySettings(view)
		a.settingsPage().Refresh()
		a.toastT("settings.cacheDir.saved")
	})
}

func (a *App) saveApiKeys(req appstructs.SaveApiKeysRequest) {
	svc := a.service()
	if svc == nil {
		return
	}
	view := svc.SaveApiKeys(req)
	a.uiUpdate(func() {
		a.applySettings(view)
		a.settingsPage().cfKey = uistate.KeyDraft{}
		a.settingsPage().mrKey = uistate.KeyDraft{}
		a.settingsPage().Refresh()
		a.toastT("settings.apiKeys.saved")
	})
}
