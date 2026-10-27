package uistate

import (
	"math"

	"github.com/link-fgfgui/mod-downloader-core/configs"
	appstructs "github.com/link-fgfgui/mod-downloader-core/structs"
)

// Animation duration bounds mirrored from useAnimationSettings.ts.
const (
	DefaultAnimationDurationMultiplier = 1.0
	MinAnimationDurationMultiplier     = 0.1
	MaxAnimationDurationMultiplier     = 3.0
)

// Preferred release types.
const (
	ReleaseTypeRelease = "release"
	ReleaseTypeBeta    = "beta"
	ReleaseTypeAlpha   = "alpha"
)

// NormalizeAnimationDurationMultiplier clamps to [0.1, 3.0] with 1.0 fallback.
func NormalizeAnimationDurationMultiplier(value float64) float64 {
	if math.IsNaN(value) || math.IsInf(value, 0) || value <= 0 {
		return DefaultAnimationDurationMultiplier
	}
	return math.Min(MaxAnimationDurationMultiplier, math.Max(MinAnimationDurationMultiplier, value))
}

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

// LoadSettingsDrafts mirrors settings.load(): derive drafts from the view,
// clamping every numeric field into its documented range.
func LoadSettingsDrafts(view appstructs.SettingsView) SettingsDrafts {
	return SettingsDrafts{
		Theme:                       defaultIfEmpty(view.Theme, string(configs.ThemeDark)),
		Language:                    defaultIfEmpty(view.Language, "system"),
		AnimationDurationMultiplier: NormalizeAnimationDurationMultiplier(view.AnimationDurationMultiplier),
		SimpleMode:                  view.SimpleMode,
		AutoScanUnusedDependencies:  view.AutoScanUnusedDependencies,
		MCIMEnabled:                 view.MCIMEnabled,
		PreferredReleaseType:        defaultIfEmpty(view.PreferredReleaseType, ReleaseTypeRelease),
		FileConcurrency:             clampInt(view.FileConcurrency, configs.MinFileConcurrency, configs.MaxFileConcurrency, configs.DefaultFileConcurrency),
		ConcurrentDownloads:         clampInt(view.ConcurrentDownloads, configs.MinConcurrentDownloads, configs.MaxConcurrentDownloads, configs.DefaultConcurrentDownloads),
		AdaptiveFileConcurrency:     view.AdaptiveFileConcurrency,
		TargetDownloadRateMiB:       clampRate(view.TargetDownloadRateMiB),
		VerifySHA1:                  view.VerifySHA1,
		RequestsPerSecond:           clampInt(view.RequestsPerSecond, configs.MinRequestsPerSecond, configs.MaxRequestsPerSecond, 0),
	}
}

// ValidateNetworkSettings clamps and normalizes the network draft values
// (mirrors the documented ranges in Phase 7.2, including the 1-decimal
// precision requirement for the target rate).
func ValidateNetworkSettings(d SettingsDrafts) appstructs.SaveNetworkSettingsRequest {
	rate := math.Round(clampRate(d.TargetDownloadRateMiB)*10) / 10
	return appstructs.SaveNetworkSettingsRequest{
		FileConcurrency:         clampInt(d.FileConcurrency, configs.MinFileConcurrency, configs.MaxFileConcurrency, configs.DefaultFileConcurrency),
		ConcurrentDownloads:     clampInt(d.ConcurrentDownloads, configs.MinConcurrentDownloads, configs.MaxConcurrentDownloads, configs.DefaultConcurrentDownloads),
		AdaptiveFileConcurrency: d.AdaptiveFileConcurrency,
		TargetDownloadRateMiB:   rate,
		VerifySHA1:              d.VerifySHA1,
		RequestsPerSecond:       clampInt(d.RequestsPerSecond, configs.MinRequestsPerSecond, configs.MaxRequestsPerSecond, 0),
	}
}

func clampRate(value float64) float64 {
	if math.IsNaN(value) || math.IsInf(value, 0) || value <= 0 {
		return configs.DefaultTargetDownloadRateMiB
	}
	return math.Min(configs.MaxTargetDownloadRateMiB, math.Max(configs.MinTargetDownloadRateMiB, value))
}

func clampInt(value, min, max, fallback int) int {
	if value == 0 && min > 0 {
		// A zero value on a positive-minimum field is treated as "unset" and
		// falls back to the documented default (matches the GUI's `?? default`).
		return fallback
	}
	if value < min {
		return min
	}
	if value > max {
		return max
	}
	return value
}

func defaultIfEmpty(value, fallback string) string {
	if value == "" {
		return fallback
	}
	return value
}
