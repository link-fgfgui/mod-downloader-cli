package tviewui

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"

	"github.com/link-fgfgui/mod-downloader-cli/internal/i18n"
	"github.com/link-fgfgui/mod-downloader-cli/internal/uistate"
)

// settingsPage renders the configuration editor as a single Form with section
// headers. Every field is addressed by a stable key, never by row order.
type settingsPage struct {
	app  *App
	root *tview.Flex
	form *wheelForm
	head *tview.TextView
	sub  *tview.TextView

	refreshing bool
	builtLang  string
	builtTheme string

	language    *tview.DropDown
	languageIdx int
	theme       *tview.DropDown
	release     *tview.DropDown
	simpleMode  *tview.Checkbox
	cleanup     *tview.Checkbox
	cacheDir    *tview.InputField
	mcim        *tview.Checkbox
	fileConc    *tview.InputField
	adaptive    *tview.Checkbox
	targetRate  *tview.InputField
	verifySHA1  *tview.Checkbox
	concurrent  *tview.InputField
	rps         *tview.InputField

	// texts maps stable keys to the read-only form rows, so refreshes never
	// depend on row positions.
	texts map[string]*tview.TextView

	cfKey uistate.KeyDraft
	mrKey uistate.KeyDraft
}

// Stable keys for the read-only settings rows refreshed from the snapshot.
const (
	settingsTextMinecraftDir = "minecraftDir"
	settingsTextCurseforge   = "curseforgeKey"
	settingsTextModrinth     = "modrinthKey"
)

var (
	languageValues = []string{"system", "zh", "en"}
	themeValues    = []string{"dark", "light", "system"}
	releaseValues  = []string{"release", "beta", "alpha"}
)

func newSettingsPage(a *App) *settingsPage {
	v := &settingsPage{app: a}
	v.head = tview.NewTextView().SetDynamicColors(true)
	v.head.SetBackgroundColor(a.theme.Surface)
	v.head.SetText(i18n.T("settings.title"))
	v.sub = tview.NewTextView().SetDynamicColors(true)
	v.sub.SetBackgroundColor(a.theme.Surface)
	v.sub.SetText(a.theme.tag(a.theme.Muted) + uiText("settingsSubtitle") + resetTag)

	v.form = newWheelForm()
	v.form.SetBorder(true)
	a.theme.styleBox(v.form.Box)
	v.builtTheme = a.theme.Name
	v.buildForm()

	v.root = tview.NewFlex().SetDirection(tview.FlexRow).
		AddItem(v.head, 1, 0, false).
		AddItem(v.sub, 1, 0, false).
		AddItem(v.form, 0, 1, true)
	return v
}

func (v *settingsPage) Primitive() tview.Primitive { return v.root }
func (v *settingsPage) Focus() {
	v.form.SetFocus(v.languageIdx)
	v.app.application.SetFocus(v.form)
}

func (v *settingsPage) OnActivate() { go v.app.refreshSettings() }

func (v *settingsPage) HandleKey(_ *tcell.EventKey) bool { return false }

// ClearFocus moves focus out of a text field before Esc can leave the page.
func (v *settingsPage) ClearFocus() bool {
	item, _ := v.form.GetFocusedItemIndex()
	if item < 0 {
		return false
	}
	if _, ok := v.form.GetFormItem(item).(*tview.InputField); !ok {
		return false
	}
	v.form.SetFocus(v.languageIdx)
	return true
}

func (v *settingsPage) Hints() []hint {
	return []hint{
		hintText("↑↓ " + uiText("navigate")),
		hintText("tab " + uiText("navigate")),
		hintText("enter " + uiText("open")),
		hintAction("esc "+uiText("close"), func() { v.app.switchView(ViewHome) }),
	}
}

func (v *settingsPage) buildForm() {
	a := v.app
	v.builtLang = i18n.Current()
	v.texts = make(map[string]*tview.TextView, 3)

	section := func(title string) {
		v.form.AddTextView("── "+title+" ──", "", 0, 1, true, false)
	}

	section(uiText("general"))
	v.languageIdx = v.form.GetFormItemCount()
	v.language = tview.NewDropDown()
	v.language.SetLabel(i18n.T("settings.language.label") + " ")
	v.language.SetOptions([]string{
		i18n.T("settings.language.system"),
		i18n.T("settings.language.zh"),
		i18n.T("settings.language.en"),
	}, func(_ string, index int) {
		if v.refreshing || index < 0 || index >= len(languageValues) {
			return
		}
		go a.saveLanguage(languageValues[index])
	})
	v.form.AddFormItem(v.language)

	v.theme = tview.NewDropDown()
	v.theme.SetLabel(i18n.T("settings.theme.label") + " ")
	v.theme.SetOptions([]string{
		i18n.T("settings.theme.dark"),
		i18n.T("settings.theme.light"),
		i18n.T("settings.theme.system"),
	}, func(_ string, index int) {
		if v.refreshing || index < 0 || index >= len(themeValues) {
			return
		}
		go a.saveTheme(themeValues[index])
	})
	v.form.AddFormItem(v.theme)

	v.release = tview.NewDropDown()
	v.release.SetLabel(uiText("releaseVariant") + " ")
	v.release.SetOptions([]string{
		i18n.T("settings.releaseType.stable"),
		i18n.T("settings.releaseType.beta"),
		i18n.T("settings.releaseType.alpha"),
	}, func(_ string, index int) {
		if v.refreshing || index < 0 || index >= len(releaseValues) {
			return
		}
		go a.savePreferredReleaseType(releaseValues[index])
	})
	v.form.AddFormItem(v.release)

	v.simpleMode = v.newCheckbox(i18n.T("settings.simpleMode.label") + " ")
	v.simpleMode.SetChangedFunc(func(checked bool) {
		if v.refreshing {
			return
		}
		go a.saveSimpleMode(checked)
	})
	v.form.AddFormItem(v.simpleMode)

	section(uiText("storage"))
	v.addTextItem(settingsTextMinecraftDir, uiText("mcDir")+" ", "")

	v.cleanup = v.newCheckbox(uiText("cleanupLabel"))
	v.cleanup.SetChangedFunc(func(checked bool) {
		if v.refreshing {
			return
		}
		go a.saveUnusedCleanup(checked)
	})
	v.form.AddFormItem(v.cleanup)

	v.cacheDir = tview.NewInputField()
	v.cacheDir.SetLabel(i18n.T("settings.cacheDir.label") + " ")
	v.cacheDir.SetFieldWidth(20)
	a.theme.styleInput(v.cacheDir)
	v.cacheDir.SetDoneFunc(func(key tcell.Key) {
		if v.refreshing {
			return
		}
		if key == tcell.KeyEnter || key == tcell.KeyTab || key == tcell.KeyBacktab {
			go a.saveCacheDir(strings.TrimSpace(v.cacheDir.GetText()))
		}
	})
	v.form.AddFormItem(v.cacheDir)

	v.form.AddButton(i18n.T("settings.cacheDir.choose"), func() {
		start := a.settings.CacheDir
		if start == "" {
			start = a.settings.CachePath
		}
		a.openPathDialog(i18n.T("settings.cacheDir.choose"), false, start, func(path string) {
			go a.saveCacheDir(path)
		})
	})
	v.form.AddButton(i18n.T("settings.cacheDir.reset"), func() {
		go a.saveCacheDir("")
	})
	v.form.AddButton(i18n.T("settings.minecraftDir.choose")+" .minecraft", func() {
		a.openPathDialog(i18n.T("settings.minecraftDir.choose"), false, a.settings.MinecraftDir, func(path string) {
			go a.setMinecraftDir(path)
		})
	})

	section(i18n.T("settings.network.label"))
	v.mcim = v.newCheckbox("MCIM")
	v.mcim.SetChangedFunc(func(checked bool) {
		if v.refreshing {
			return
		}
		go a.saveMCIM(checked)
	})
	v.form.AddFormItem(v.mcim)

	v.fileConc = v.intField(uiText("fileConc"))
	v.adaptive = v.newCheckbox(uiText("adaptiveLabel"))
	v.adaptive.SetChangedFunc(func(checked bool) {
		if v.refreshing {
			return
		}
		v.saveNetwork()
	})
	v.form.AddFormItem(v.adaptive)

	v.targetRate = v.floatField(uiText("targetRate"))
	v.verifySHA1 = v.newCheckbox(uiText("verifySha1"))
	v.verifySHA1.SetChangedFunc(func(checked bool) {
		if v.refreshing {
			return
		}
		v.saveNetwork()
	})
	v.form.AddFormItem(v.verifySHA1)

	v.concurrent = v.intField(uiText("downloads"))
	v.rps = v.intField(uiText("requests"))
	v.form.AddButton(uiText("saveNetwork"), v.saveNetwork)

	section(uiText("apiKeys"))
	v.addTextItem(settingsTextCurseforge, "CurseForge", "")
	v.form.AddButton(i18n.T("settings.apiKeys.curseforge.save"), func() {
		a.openPrompt(i18n.T("settings.apiKeys.curseforge.label"), i18n.T("settings.apiKeys.curseforge.hint"), "", i18n.T("settings.apiKeys.curseforge.placeholder"), nil,
			func(value string) {
				v.cfKey = uistate.KeyDraft{Value: value}
				if value == "" {
					v.cfKey = uistate.KeyDraft{Clear: true}
				}
				go a.saveApiKeys(uistate.BuildSaveApiKeysRequest(v.cfKey, v.mrKey))
			})
	})
	v.form.AddButton(i18n.T("settings.apiKeys.curseforge.clear"), func() {
		v.cfKey = uistate.KeyDraft{Clear: true}
		go a.saveApiKeys(uistate.BuildSaveApiKeysRequest(v.cfKey, v.mrKey))
	})

	v.addTextItem(settingsTextModrinth, "Modrinth", "")
	v.form.AddButton(i18n.T("settings.apiKeys.modrinth.save"), func() {
		a.openPrompt(i18n.T("settings.apiKeys.modrinth.label"), i18n.T("settings.apiKeys.modrinth.hint"), "", i18n.T("settings.apiKeys.modrinth.placeholder"), nil,
			func(value string) {
				v.mrKey = uistate.KeyDraft{Value: value}
				if value == "" {
					v.mrKey = uistate.KeyDraft{Clear: true}
				}
				go a.saveApiKeys(uistate.BuildSaveApiKeysRequest(v.cfKey, v.mrKey))
			})
	})
	v.form.AddButton(i18n.T("settings.apiKeys.modrinth.clear"), func() {
		v.mrKey = uistate.KeyDraft{Clear: true}
		go a.saveApiKeys(uistate.BuildSaveApiKeysRequest(v.cfKey, v.mrKey))
	})
}

// addTextItem appends a one-line read-only row and records it under a stable
// key so later refreshes never depend on the row index.
func (v *settingsPage) addTextItem(key, label, text string) *tview.TextView {
	view := tview.NewTextView().
		SetLabel(label).
		SetSize(1, 0).
		SetDynamicColors(true).
		SetScrollable(false).
		SetText(text)
	view.SetBackgroundColor(v.app.theme.Surface)
	v.form.AddFormItem(view)
	v.texts[key] = view
	return view
}

func (v *settingsPage) newCheckbox(label string) *tview.Checkbox {
	box := tview.NewCheckbox().SetLabel(label)
	// tview draws these strings with style tags enabled, so a raw "[X]" is
	// parsed as a tag and vanishes; the escaped form renders literally.
	box.SetUncheckedString(tview.Escape("[ ]"))
	box.SetCheckedString(tview.Escape("[X]"))
	return box
}

func (v *settingsPage) intField(label string) *tview.InputField {
	field := tview.NewInputField()
	field.SetLabel(label + " ")
	field.SetFieldWidth(10)
	field.SetAcceptanceFunc(func(_ string, lastChar rune) bool {
		return lastChar >= '0' && lastChar <= '9'
	})
	v.app.theme.styleInput(field)
	field.SetDoneFunc(func(key tcell.Key) {
		if (key == tcell.KeyEnter || key == tcell.KeyTab || key == tcell.KeyBacktab) && !v.refreshing {
			v.saveNetwork()
		}
	})
	v.form.AddFormItem(field)
	return field
}

func (v *settingsPage) floatField(label string) *tview.InputField {
	field := tview.NewInputField()
	field.SetLabel(label + " ")
	field.SetFieldWidth(10)
	field.SetAcceptanceFunc(func(text string, lastChar rune) bool {
		if lastChar >= '0' && lastChar <= '9' {
			return true
		}
		return lastChar == '.' && !strings.Contains(text, ".")
	})
	v.app.theme.styleInput(field)
	field.SetDoneFunc(func(key tcell.Key) {
		if (key == tcell.KeyEnter || key == tcell.KeyTab || key == tcell.KeyBacktab) && !v.refreshing {
			v.saveNetwork()
		}
	})
	v.form.AddFormItem(field)
	return field
}

// Refresh replays the settings snapshot into the form. All callbacks are muted
// while assigning values so a refresh never triggers another save.
func (v *settingsPage) Refresh() {
	if v.builtLang != i18n.Current() || v.builtTheme != v.app.theme.Name {
		v.builtLang = i18n.Current()
		v.builtTheme = v.app.theme.Name
		v.app.theme.styleBox(v.form.Box)
		v.form.Clear(true)
		v.buildForm()
		v.form.SetFocus(v.languageIdx)
	}
	v.head.SetText(v.app.theme.tagWith(v.app.theme.Primary, "b") + i18n.T("settings.title") + resetTag)
	v.sub.SetText(v.app.theme.tag(v.app.theme.Muted) + uiText("settingsSubtitle") + resetTag)
	v.head.SetBackgroundColor(v.app.theme.Surface)
	v.sub.SetBackgroundColor(v.app.theme.Surface)
	d := uistate.LoadSettingsDrafts(v.app.settings)
	v.refreshing = true
	defer func() { v.refreshing = false }()

	v.setDropdown(v.language, languageValues, d.Language)
	v.setDropdown(v.theme, themeValues, d.Theme)
	v.setDropdown(v.release, releaseValues, d.PreferredReleaseType)
	v.simpleMode.SetChecked(d.SimpleMode)
	v.cleanup.SetChecked(d.AutoScanUnusedDependencies)
	mcDir := v.app.settings.MinecraftDir
	if mcDir != "" {
		mcDir = expandPath(mcDir)
	}
	v.setText(settingsTextMinecraftDir, tview.Escape(mcDir))
	v.cacheDir.SetText(v.app.settings.CacheDir)
	v.mcim.SetChecked(d.MCIMEnabled)
	v.fileConc.SetText(strconv.Itoa(d.FileConcurrency))
	v.adaptive.SetChecked(d.AdaptiveFileConcurrency)
	v.targetRate.SetText(formatFloat(d.TargetDownloadRateMiB))
	v.verifySHA1.SetChecked(d.VerifySHA1)
	v.concurrent.SetText(strconv.Itoa(d.ConcurrentDownloads))
	v.rps.SetText(strconv.Itoa(d.RequestsPerSecond))
	v.setText(settingsTextCurseforge, v.keyStatus(true, d))
	v.setText(settingsTextModrinth, v.keyStatus(false, d))
}

// setText updates the read-only row registered under key.
func (v *settingsPage) setText(key, text string) {
	if view := v.texts[key]; view != nil {
		view.SetText(text)
	}
}

func (v *settingsPage) setDropdown(field *tview.DropDown, values []string, value string) {
	index := 0
	for i, candidate := range values {
		if candidate == value {
			index = i
			break
		}
	}
	field.SetCurrentOption(index)
}

func (v *settingsPage) keyStatus(curseforge bool, d uistate.SettingsDrafts) string {
	t := v.app.theme
	var label string
	var masked string
	var has bool
	if curseforge {
		label = i18n.T("settings.apiKeys.curseforge.label")
		masked = v.app.settings.CurseforgeKeyMask
		has = v.app.settings.HasCurseforgeKey
		if v.cfKey.Clear {
			has = false
		}
	} else {
		label = i18n.T("settings.apiKeys.modrinth.label")
		masked = v.app.settings.ModrinthKeyMask
		has = v.app.settings.HasModrinthKey
		if v.mrKey.Clear {
			has = false
		}
	}
	status := i18n.T("settings.apiKeys.curseforge.statusEmpty")
	color := t.Muted
	if has {
		status = i18n.T("settings.apiKeys.curseforge.status") + " " + masked
		color = t.Success
	}
	return t.tag(t.Muted) + label + ": " + resetTag + t.tag(color) + status + resetTag
}

func (v *settingsPage) saveNetwork() {
	if v.refreshing {
		return
	}
	d := uistate.LoadSettingsDrafts(v.app.settings)
	d.FileConcurrency = parseInt(v.fileConc.GetText(), d.FileConcurrency)
	d.ConcurrentDownloads = parseInt(v.concurrent.GetText(), d.ConcurrentDownloads)
	d.TargetDownloadRateMiB = parseFloat(v.targetRate.GetText(), d.TargetDownloadRateMiB)
	d.RequestsPerSecond = parseInt(v.rps.GetText(), 0)
	d.AdaptiveFileConcurrency = v.adaptive.IsChecked()
	d.VerifySHA1 = v.verifySHA1.IsChecked()
	go v.app.saveNetworkSettings(d)
}

func parseInt(text string, fallback int) int {
	value, err := strconv.Atoi(strings.TrimSpace(text))
	if err != nil {
		return fallback
	}
	return value
}

func parseFloat(text string, fallback float64) float64 {
	value, err := strconv.ParseFloat(strings.TrimSpace(text), 64)
	if err != nil {
		return fallback
	}
	return value
}

func formatFloat(value float64) string {
	return strings.TrimRight(strings.TrimRight(fmt.Sprintf("%.1f", value), "0"), ".")
}
