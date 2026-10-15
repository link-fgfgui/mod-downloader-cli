package tui

import (
	"strings"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/link-fgfgui/mod-downloader-core/appcore"
	"github.com/link-fgfgui/mod-downloader-core/configs"

	"github.com/link-fgfgui/mod-downloader-cli/internal/uistate"
	"github.com/link-fgfgui/mod-downloader-cli/tui/i18n"
)

// settingsModel renders the full configuration editor (Phase 7).
type settingsModel struct {
	app    *AppModel
	drafts uistate.SettingsDrafts
	loaded bool
	cursor int
	offset int
}

func (v *settingsModel) Update(app *AppModel, msg tea.Msg) tea.Cmd {
	switch typed := msg.(type) {
	case viewActivatedMsg:
		if !v.loaded {
			return cmdGetSettings(app.host)
		}
		return nil
	case settingsLoadedMsg:
		v.drafts = uistate.LoadSettingsDrafts(typed.View)
		v.loaded = true
		return nil
	case settingsSavedMsg:
		if typed.Theme != "" || typed.Lang != "" {
			v.drafts.Theme = defaultStr(typed.Theme, v.drafts.Theme)
			v.drafts.Language = defaultStr(typed.Lang, v.drafts.Language)
		}
		// Refresh the numeric/bool drafts from the persisted view.
		merged := uistate.LoadSettingsDrafts(typed.View)
		merged.CurseforgeKey = v.drafts.CurseforgeKey
		merged.ModrinthKey = v.drafts.ModrinthKey
		v.drafts = merged
		return nil
	case tea.KeyMsg:
		return v.key(app, typed)
	}
	return nil
}

func (v *settingsModel) key(app *AppModel, key tea.KeyMsg) tea.Cmd {
	switch key.String() {
	case "up", "k":
		v.move(-1)
	case "down", "j":
		v.move(1)
	case "home":
		v.cursor = 0
	case "end":
		v.cursor = settingsRowCount - 1
	case "left", "h":
		return v.adjust(app, -1)
	case "right", "l":
		return v.adjust(app, 1)
	case "enter", " ":
		return v.activate(app)
	}
	return nil
}

const settingsRowCount = 18

func (v *settingsModel) move(delta int) {
	v.cursor += delta
	if v.cursor < 0 {
		v.cursor = 0
	}
	if v.cursor >= settingsRowCount {
		v.cursor = settingsRowCount - 1
	}
}

func (v *settingsModel) adjust(app *AppModel, delta int) tea.Cmd {
	switch v.cursor {
	case 0: // Language
		v.drafts.Language = cycleString(v.drafts.Language, []string{"system", "zh", "en"}, delta)
		v.drafts.CurseforgeKey.Reset()
		return cmdSaveLanguage(app.host, v.drafts.Language)
	case 1: // Theme
		v.drafts.Theme = cycleString(v.drafts.Theme, []string{"dark", "light", "system"}, delta)
		return cmdSaveTheme(app.host, v.drafts.Theme)
	case 2: // Animation mode
		v.drafts.AnimationMode = cycleString(v.drafts.AnimationMode, []string{"off", "vuetify", "gsap"}, delta)
		return v.saveAnimations(app)
	case 3: // Animation duration multiplier
		v.drafts.AnimationDurationMultiplier = clampStep(v.drafts.AnimationDurationMultiplier, delta, 0.1, uistate.MinAnimationDurationMultiplier, uistate.MaxAnimationDurationMultiplier)
		return v.saveAnimations(app)
	case 6: // Preferred release type
		v.drafts.PreferredReleaseType = cycleString(v.drafts.PreferredReleaseType, []string{"release", "beta", "alpha"}, delta)
		return cmdSavePreferredReleaseType(app.host, v.drafts.PreferredReleaseType)
	case 9: // File concurrency
		v.drafts.FileConcurrency = clampInt(v.drafts.FileConcurrency+delta, configs.MinFileConcurrency, configs.MaxFileConcurrency, configs.DefaultFileConcurrency)
		return v.saveNetwork(app)
	case 11: // Target download rate
		v.drafts.TargetDownloadRateMiB = clampStep(v.drafts.TargetDownloadRateMiB, delta, 0.1, configs.MinTargetDownloadRateMiB, configs.MaxTargetDownloadRateMiB)
		return v.saveNetwork(app)
	case 13: // Concurrent downloads
		v.drafts.ConcurrentDownloads = clampInt(v.drafts.ConcurrentDownloads+delta, configs.MinConcurrentDownloads, 16, configs.DefaultConcurrentDownloads)
		return v.saveNetwork(app)
	case 14: // Requests per second
		v.drafts.RequestsPerSecond = clampInt(v.drafts.RequestsPerSecond+delta, configs.MinRequestsPerSecond, configs.MaxRequestsPerSecond, 0)
		return v.saveNetwork(app)
	}
	return nil
}

func (v *settingsModel) activate(app *AppModel) tea.Cmd {
	switch v.cursor {
	case 0:
		v.drafts.Language = cycleString(v.drafts.Language, []string{"system", "zh", "en"}, 1)
		return cmdSaveLanguage(app.host, v.drafts.Language)
	case 1:
		v.drafts.Theme = cycleString(v.drafts.Theme, []string{"dark", "light", "system"}, 1)
		return cmdSaveTheme(app.host, v.drafts.Theme)
	case 2:
		v.drafts.AnimationMode = cycleString(v.drafts.AnimationMode, []string{"off", "vuetify", "gsap"}, 1)
		return v.saveAnimations(app)
	case 4: // Simple mode
		v.drafts.SimpleMode = !v.drafts.SimpleMode
		return cmdSaveSimpleModeSettings(app.host, v.drafts.SimpleMode)
	case 5: // Auto-scan unused deps
		v.drafts.AutoScanUnusedDependencies = !v.drafts.AutoScanUnusedDependencies
		return cmdSaveUnusedCleanupSettings(app.host, v.drafts.AutoScanUnusedDependencies)
	case 6:
		v.drafts.PreferredReleaseType = cycleString(v.drafts.PreferredReleaseType, []string{"release", "beta", "alpha"}, 1)
		return cmdSavePreferredReleaseType(app.host, v.drafts.PreferredReleaseType)
	case 7: // Cache dir -> path picker
		picker := newPathPickerModal("settings-cache-dir", i18n.T("settings.cacheDir.choose"), pickerDirectory, app.settingsView.CacheDir,
			func(app *AppModel, path string) tea.Cmd {
				return cmdSaveCacheDir(app.host, path)
			})
		app.pushModal(picker)
		return picker.Init()
	case 8: // MCIM
		v.drafts.MCIMEnabled = !v.drafts.MCIMEnabled
		return cmdSaveMCIMSettings(app.host, v.drafts.MCIMEnabled)
	case 10: // Adaptive file concurrency
		v.drafts.AdaptiveFileConcurrency = !v.drafts.AdaptiveFileConcurrency
		return v.saveNetwork(app)
	case 12: // Verify SHA1
		v.drafts.VerifySHA1 = !v.drafts.VerifySHA1
		return v.saveNetwork(app)
	case 15: // CurseForge key
		return v.promptKey(app, true)
	case 16: // Modrinth key
		return v.promptKey(app, false)
	case 17: // Reset cache dir
		return cmdSaveCacheDir(app.host, "")
	}
	return nil
}

func (v *settingsModel) promptKey(app *AppModel, curseforge bool) tea.Cmd {
	title := i18n.T("settings.apiKeys.modrinth.label")
	if curseforge {
		title = i18n.T("settings.apiKeys.curseforge.label")
	}
	placeholder := i18n.T("settings.apiKeys.curseforge.placeholder")
	modal := newPromptModal("settings-api-key", title, i18n.T("settings.apiKeys.curseforge.hint"), "", placeholder,
		func(app *AppModel, value string) tea.Cmd {
			if curseforge {
				app.settings.drafts.CurseforgeKey = uistate.KeyDraft{Value: value}
				if value == "" {
					app.settings.drafts.CurseforgeKey = uistate.KeyDraft{Clear: true}
				}
			} else {
				app.settings.drafts.ModrinthKey = uistate.KeyDraft{Value: value}
				if value == "" {
					app.settings.drafts.ModrinthKey = uistate.KeyDraft{Clear: true}
				}
			}
			req := uistate.BuildSaveApiKeysRequest(app.settings.drafts.CurseforgeKey, app.settings.drafts.ModrinthKey)
			return cmdSaveApiKeys(app.host, req)
		})
	app.pushModal(modal)
	return nil
}

func (v *settingsModel) saveAnimations(app *AppModel) tea.Cmd {
	req := appcore.SaveAnimationSettingsRequest{
		AnimationMode:               v.drafts.AnimationMode,
		AnimationEnabled:            v.drafts.AnimationMode != uistate.AnimationModeOff,
		AnimationDurationMultiplier: uistate.NormalizeAnimationDurationMultiplier(v.drafts.AnimationDurationMultiplier),
	}
	return cmdSaveAnimationSettings(app.host, req)
}

func (v *settingsModel) saveNetwork(app *AppModel) tea.Cmd {
	return cmdSaveNetworkSettings(app.host, uistate.ValidateNetworkSettings(v.drafts))
}

func (v *settingsModel) View(app *AppModel) string {
	t := app.theme
	var b strings.Builder
	b.WriteString(t.Title().Render(i18n.T("settings.title")))
	b.WriteString("\n\n")

	d := v.drafts
	rows := []struct {
		label string
		value string
	}{
		{i18n.T("settings.language.label"), languageLabel(d.Language)},
		{i18n.T("settings.theme.label"), themeLabel(d.Theme)},
		{i18n.T("settings.animations.label") + " · " + i18n.T("settings.animations.mode"), d.AnimationMode},
		{i18n.T("settings.animations.label") + " ×", formatFloat(d.AnimationDurationMultiplier)},
		{i18n.T("settings.simpleMode.label"), boolLabel(d.SimpleMode)},
		{i18n.T("settings.cleanup.label"), boolLabel(d.AutoScanUnusedDependencies)},
		{i18n.T("manage.releaseType.label"), d.PreferredReleaseType},
		{i18n.T("settings.cacheDir.label"), truncate(defaultStr(app.settingsView.CacheDir, app.settingsView.CachePath), maxInt(16, app.width/3))},
		{i18n.T("settings.mcim.label"), boolLabel(d.MCIMEnabled)},
		{i18n.T("settings.network.fileConcurrency"), itoa(d.FileConcurrency)},
		{i18n.T("settings.network.adaptiveFileConcurrency"), boolLabel(d.AdaptiveFileConcurrency)},
		{i18n.T("settings.network.targetDownloadRateMiB"), formatFloat(d.TargetDownloadRateMiB)},
		{i18n.T("settings.network.verifySha1"), boolLabel(d.VerifySHA1)},
		{i18n.T("settings.network.concurrentDownloads"), itoa(d.ConcurrentDownloads)},
		{i18n.T("settings.network.requestsPerSecond"), itoa(d.RequestsPerSecond)},
		{i18n.T("settings.apiKeys.curseforge.label"), keyStatus(d, app, true)},
		{i18n.T("settings.apiKeys.modrinth.label"), keyStatus(d, app, false)},
		{i18n.T("settings.cacheDir.reset"), ""},
	}

	// Hide the target-rate row when adaptive concurrency is off (mirrors GUI).
	const rateRow = 11
	visible := make([]int, 0, len(rows))
	for i := range rows {
		if i == rateRow && !d.AdaptiveFileConcurrency {
			continue
		}
		visible = append(visible, i)
	}

	// Keep the cursor inside the visible set.
	cursorVisible := 0
	for idx, rowIdx := range visible {
		if rowIdx == v.cursor {
			cursorVisible = idx
		}
	}

	const windowRows = 14
	start := 0
	if cursorVisible >= windowRows {
		start = cursorVisible - windowRows + 1
	}
	end := start + windowRows
	if end > len(visible) {
		end = len(visible)
	}

	labelWidth := 34
	for idx := start; idx < end; idx++ {
		rowIdx := visible[idx]
		row := rows[rowIdx]
		line := padRight(truncate(row.label, labelWidth-1), labelWidth)
		if row.value != "" {
			line += " " + t.Accent().Render(row.value)
		}
		if rowIdx == v.cursor {
			line = t.Selected().Render(stripStyles(line))
		} else {
			line = t.OnSurface().Render(line)
		}
		b.WriteString(line)
		b.WriteString("\n")
	}
	if len(visible) > windowRows {
		b.WriteString(t.Muted().Render(itoa(start+1) + "-" + itoa(end) + " / " + itoa(len(visible))))
	}
	return strings.TrimRight(b.String(), "\n")
}

func (v *settingsModel) FooterHints(app *AppModel) string {
	return "↑↓ " + uiText("navigate") + " · ←→ " + uiText("toggle") + " · enter " + uiText("open") + " · esc " + uiText("close")
}

func (v *settingsModel) ClearFocus(app *AppModel) (bool, tea.Cmd) { return false, nil }

func (v *settingsModel) CapturesKeys(app *AppModel) bool { return false }

func (v *settingsModel) RowCount() int { return settingsRowCount }

func (v *settingsModel) MouseWheel(app *AppModel, delta int) tea.Cmd {
	v.move(delta)
	return nil
}

func (v *settingsModel) ClickRow(app *AppModel, index int, ev tea.MouseMsg) tea.Cmd {
	if index >= 0 && index < settingsRowCount {
		v.cursor = index
	}
	return nil
}

// --- helpers ------------------------------------------------------------------

func keyStatus(d uistate.SettingsDrafts, app *AppModel, curseforge bool) string {
	if curseforge {
		if d.CurseforgeKey.Clear {
			return i18n.T("settings.apiKeys.curseforge.statusEmpty")
		}
		if app.settingsView.HasCurseforgeKey {
			return i18n.T("settings.apiKeys.curseforge.status") + " " + app.settingsView.CurseforgeKeyMask
		}
		return i18n.T("settings.apiKeys.curseforge.statusEmpty")
	}
	if d.ModrinthKey.Clear {
		return i18n.T("settings.apiKeys.modrinth.statusEmpty")
	}
	if app.settingsView.HasModrinthKey {
		return i18n.T("settings.apiKeys.modrinth.status") + " " + app.settingsView.ModrinthKeyMask
	}
	return i18n.T("settings.apiKeys.modrinth.statusEmpty")
}

func languageLabel(value string) string {
	switch value {
	case "zh":
		return i18n.T("settings.language.zh")
	case "en":
		return i18n.T("settings.language.en")
	default:
		return i18n.T("settings.language.system")
	}
}

func themeLabel(value string) string {
	switch value {
	case "light":
		return i18n.T("settings.theme.light")
	case "system":
		return i18n.T("settings.theme.system")
	default:
		return i18n.T("settings.theme.dark")
	}
}

func boolLabel(value bool) string {
	if value {
		return uiText("yes")
	}
	return uiText("no")
}

func cycleString(value string, options []string, delta int) string {
	idx := 0
	for i, option := range options {
		if option == value {
			idx = i
			break
		}
	}
	idx = (idx + delta) % len(options)
	if idx < 0 {
		idx += len(options)
	}
	return options[idx]
}

func clampStep(value float64, delta int, step, min, max float64) float64 {
	value += float64(delta) * step
	if value < min {
		value = min
	}
	if value > max {
		value = max
	}
	// Avoid floating-point drift from repeated addition.
	return roundTo(value, 1)
}

func roundTo(value float64, decimals int) float64 {
	mult := 1.0
	for i := 0; i < decimals; i++ {
		mult *= 10
	}
	return float64(int(value*mult+0.5)) / mult
}

func formatFloat(value float64) string {
	return strings.TrimRight(strings.TrimRight(formatFixed(value), "0"), ".")
}

func formatFixed(value float64) string {
	// One decimal place, matching the GUI's 0.1 precision.
	scaled := int(value*10 + 0.5)
	return itoa(scaled/10) + "." + itoa(scaled%10)
}

func defaultStr(value, fallback string) string {
	if value == "" {
		return fallback
	}
	return value
}

func clampInt(value, min, max, fallback int) int {
	if value < min {
		return min
	}
	if value > max {
		return max
	}
	return value
}
