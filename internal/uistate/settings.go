package uistate

import (
	"github.com/link-fgfgui/mod-downloader/core/configs"
	appstructs "github.com/link-fgfgui/mod-downloader/core/structs"
	appview "github.com/link-fgfgui/mod-downloader/core/structs/view"
)

// Preferred release types.
const (
	ReleaseTypeLatest  = "latest"
	ReleaseTypeRelease = "release"
	ReleaseTypeBeta    = "beta"
	ReleaseTypeAlpha   = "alpha"
)

// SettingsDrafts mirrors the settings store's editable draft fields.
type SettingsDrafts struct {
	Theme                       string
	Language                    string
	AnimationDurationMultiplier float64
	SimpleMode                  bool
	AutoScanUnusedDependencies  bool
	MCIMEnabled                 bool
	PreferredReleaseType        string
	FileConcurrency             int
	ConcurrentDownloads         int
	AdaptiveFileConcurrency     bool
	TargetDownloadRateMiB       float64
	VerifySHA1                  bool
	RequestsPerSecond           int
	CurseforgeKey               KeyDraft
	ModrinthKey                 KeyDraft
}

// LoadSettingsDrafts derives the editable drafts from a settings view. The
// view is already normalized by core, so the drafts are a straight projection.
func LoadSettingsDrafts(view appview.SettingsView) SettingsDrafts {
	view = configs.NormalizeSettingsView(view)
	return SettingsDrafts{
		Theme:                       view.Theme,
		Language:                    view.Language,
		AnimationDurationMultiplier: view.AnimationDurationMultiplier,
		SimpleMode:                  view.SimpleMode,
		AutoScanUnusedDependencies:  view.AutoScanUnusedDependencies,
		MCIMEnabled:                 view.MCIMEnabled,
		PreferredReleaseType:        view.PreferredReleaseType,
		FileConcurrency:             view.FileConcurrency,
		ConcurrentDownloads:         view.ConcurrentDownloads,
		AdaptiveFileConcurrency:     view.AdaptiveFileConcurrency,
		TargetDownloadRateMiB:       view.TargetDownloadRateMiB,
		VerifySHA1:                  view.VerifySHA1,
		RequestsPerSecond:           view.RequestsPerSecond,
	}
}

// ValidateNetworkSettings maps the draft to the core request; core applies the
// documented ranges and precision rules.
func ValidateNetworkSettings(d SettingsDrafts) appstructs.SaveNetworkSettingsRequest {
	return configs.NormalizeNetworkSettings(appstructs.SaveNetworkSettingsRequest{
		FileConcurrency:         d.FileConcurrency,
		ConcurrentDownloads:     d.ConcurrentDownloads,
		AdaptiveFileConcurrency: d.AdaptiveFileConcurrency,
		TargetDownloadRateMiB:   d.TargetDownloadRateMiB,
		VerifySHA1:              d.VerifySHA1,
		RequestsPerSecond:       d.RequestsPerSecond,
	})
}
