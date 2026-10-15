package tui

import (
	"strings"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"

	"github.com/link-fgfgui/mod-downloader-core/models"
	appstructs "github.com/link-fgfgui/mod-downloader-core/structs"

	"github.com/link-fgfgui/mod-downloader-cli/internal/uistate"
	"github.com/link-fgfgui/mod-downloader-cli/tui/i18n"
)

// downloadModel renders the search / install / pin view (Phase 3).
type downloadModel struct {
	app       *AppModel
	input     textinput.Model
	focused   bool
	cursor    int
	offset    int
	selected  map[string]bool
	anchor    int
	hasAnchor bool
}

func (v *downloadModel) ensureInit() {
	if v.input.Placeholder == "" && !v.focused && v.input.Value() == "" {
		ti := textinput.New()
		ti.Placeholder = i18n.T("search.label")
		ti.Width = 40
		v.input = ti
	}
	if v.selected == nil {
		v.selected = map[string]bool{}
	}
}

func (v *downloadModel) Update(app *AppModel, msg tea.Msg) tea.Cmd {
	v.ensureInit()
	switch typed := msg.(type) {
	case viewActivatedMsg:
		v.ensureInit()
		return nil
	case downloadStatesMsg:
		return nil
	case searchResultMsg:
		return nil
	case batchConflictMsg:
		return v.onBatchConflict(app, typed)
	case downloadQueuedMsg:
		app.search.DownloadingKeys = nil
		return tea.Batch(app.toastT("download.queued", toastSuccess), app.refreshDownloadStates())
	case downloadSkipMsg:
		app.search.DownloadingKeys = nil
		i18nKey := "download.errors.generic"
		if suffix, ok := uistate.DownloadErrorI18nKey(typed.Result.Reason); ok {
			i18nKey = "download.errors." + suffix
		}
		return app.toastT(i18nKey, toastError)
	case tea.KeyMsg:
		return v.key(app, typed)
	}
	return nil
}

// onBatchConflict mirrors batchInstall: preflight analysis, then either prompt
// or install directly.
func (v *downloadModel) onBatchConflict(app *AppModel, msg batchConflictMsg) tea.Cmd {
	conflicts := msg.Analysis.Conflicts
	if len(conflicts) == 0 {
		return v.installBatchResults(app, msg.Request.Results)
	}
	app.pushModal(newConfirmModal(
		"download-batch-conflict",
		i18n.T("download.confirmReplace.batchIncompatibleTitle"),
		i18n.T("download.confirmReplace.batchIncompatibleBody"),
		[]choiceOption{
			{Label: i18n.T("download.confirmReplace.confirm"), Value: "continue", Primary: true},
			{Label: i18n.T("download.confirmReplace.skipConflicted"), Value: "skip"},
			{Label: i18n.T("download.confirmReplace.cancel"), Value: "cancel"},
		},
		func(app *AppModel, value string) tea.Cmd {
			switch value {
			case "continue":
				return v.installBatchResults(app, msg.Request.Results)
			case "skip":
				conflicted := map[string]bool{}
				for _, c := range conflicts {
					conflicted[c.Key] = true
				}
				filtered := make([]models.ModProject, 0, len(msg.Request.Results))
				for _, result := range msg.Request.Results {
					if !conflicted[v.itemKey(result)] {
						filtered = append(filtered, result)
					}
				}
				return v.installBatchResults(app, filtered)
			}
			return nil
		},
	))
	return nil
}

func (v *downloadModel) installBatchResults(app *AppModel, results []models.ModProject) tea.Cmd {
	cmds := []tea.Cmd{}
	for _, result := range results {
		key := v.itemKey(result)
		if key == "" || !app.search.BeginDownloading(key) {
			continue
		}
		cmds = append(cmds, v.doInstall(app, result, key))
	}
	return tea.Batch(cmds...)
}

func (v *downloadModel) CapturesKeys(app *AppModel) bool { return v.focused }

func (v *downloadModel) key(app *AppModel, key tea.KeyMsg) tea.Cmd {
	if v.focused {
		switch key.String() {
		case "enter":
			v.focused = false
			v.input.Blur()
			return v.runSearch(app, false)
		case "esc":
			v.focused = false
			v.input.Blur()
			return nil
		}
		var cmd tea.Cmd
		v.input, cmd = v.input.Update(key)
		return cmd
	}

	results := v.app.search.SearchResults
	switch key.String() {
	case "/", "s":
		v.focused = true
		v.input.Focus()
		return textinput.Blink
	case "up", "k":
		v.move(-1)
	case "down", "j":
		v.move(1)
	case "pgup":
		v.move(-8)
	case "pgdown":
		v.move(8)
	case "home":
		v.cursor = 0
	case "end":
		if len(results) > 0 {
			v.cursor = len(results) - 1
		}
	case " ":
		v.toggleCurrent()
	case "ctrl+a":
		for i := range results {
			v.selected[v.itemKey(results[i])] = true
		}
	case "esc":
		v.selected = map[string]bool{}
	case "o":
		if !app.mc.HasSelectedInstance() {
			return v.chooseDir(app)
		}
	case "l":
		return v.runSearch(app, true)
	case "i", "enter":
		return v.installCurrent(app)
	case "v":
		return v.openVersions(app, "pin")
	case "f":
		return v.addToFavorites(app)
	case "b":
		return v.batchDownload(app)
	case "u":
		return v.batchUnpin(app)
	}
	return nil
}

func (v *downloadModel) itemKey(result models.ModProject) string {
	return v.app.search.StateForResultKey(result)
}

func (v *downloadModel) move(delta int) {
	results := v.app.search.SearchResults
	if len(results) == 0 {
		return
	}
	v.cursor += delta
	if v.cursor < 0 {
		v.cursor = 0
	}
	if v.cursor >= len(results) {
		v.cursor = len(results) - 1
	}
	// Auto pagination on reaching the bottom (mirrors the GUI infinite scroll).
	if v.cursor == len(results)-1 && v.app.search.HasMoreResults && delta > 0 {
		v.clampOffset()
		_ = v.runSearch(v.app, true)
	} else {
		v.clampOffset()
	}
}

func (v *downloadModel) clampOffset() {
	const rows = 8
	if v.cursor < v.offset {
		v.offset = v.cursor
	}
	if v.cursor >= v.offset+rows {
		v.offset = v.cursor - rows + 1
	}
	if v.offset < 0 {
		v.offset = 0
	}
}

func (v *downloadModel) toggleCurrent() {
	results := v.app.search.SearchResults
	if v.cursor >= len(results) {
		return
	}
	key := v.itemKey(results[v.cursor])
	if v.selected[key] {
		delete(v.selected, key)
	} else {
		v.selected[key] = true
	}
}

func (v *downloadModel) selectedResults() []models.ModProject {
	out := []models.ModProject{}
	for _, result := range v.app.search.SearchResults {
		if v.selected[v.itemKey(result)] {
			out = append(out, result)
		}
	}
	return out
}

func (v *downloadModel) runSearch(app *AppModel, appendMode bool) tea.Cmd {
	app.search.SearchText = v.input.Value()
	requestID, offset, proceed := app.search.BeginSearch(appendMode)
	if !proceed {
		return nil
	}
	req := appstructs.SearchModsRequest{
		RequestID: requestID,
		Query:     app.search.SearchText,
		Version:   app.search.SelectedVersion,
		ModLoader: app.search.SelectedModLoader,
		Offset:    offset,
		Limit:     uistate.SearchPageSize,
	}
	app.search.AdvanceOffset(requestID, offset)
	return cmdSearchMods(app.host, req)
}

// refreshStates mirrors store.refreshDownloadStates.
func (v *downloadModel) refreshStates(app *AppModel) tea.Cmd {
	return app.refreshDownloadStates()
}

// RefreshStates implements the shell's state-refresh hook.
func (v *downloadModel) RefreshStates(app *AppModel) tea.Cmd {
	return app.refreshDownloadStates()
}

// refreshDownloadStates implements the shared store logic on the AppModel side.
func (app *AppModel) refreshDownloadStates() tea.Cmd {
	if app.search.IsSearching || app.search.IsLoadingMore {
		return nil
	}
	if app.search.SelectedVersion == "" || app.search.SelectedModLoader == "" {
		app.search.DownloadStates = app.search.DisabledDownloadStates("invalidRequest")
		return nil
	}
	if !app.mc.HasSelectedInstance() {
		app.search.DownloadStates = app.search.DisabledDownloadStates("noSelectedVersion")
		return nil
	}
	requestID := uistate.NewRequestID()
	app.search.ActiveDownloadStateRequest = requestID
	req := appstructs.DownloadStatesRequest{
		Results:          app.search.SearchResults,
		MinecraftVersion: app.search.SelectedVersion,
		ModLoader:        app.search.SelectedModLoader,
	}
	return cmdGetDownloadStates(app.host, requestID, req)
}

func (v *downloadModel) installCurrent(app *AppModel) tea.Cmd {
	results := app.search.SearchResults
	if v.cursor >= len(results) {
		return nil
	}
	result := results[v.cursor]
	return v.install(app, result, false)
}

func (v *downloadModel) install(app *AppModel, result models.ModProject, confirmed bool) tea.Cmd {
	if !app.mc.HasSelectedInstance() || app.search.SelectedVersion == "" || app.search.SelectedModLoader == "" {
		return nil
	}
	key := v.itemKey(result)
	if !app.search.BeginDownloading(key) {
		return nil
	}
	state := v.stateFor(key)
	status := state.Status
	if uistate.MetadataRetryStatuses[status] {
		app.search.EndDownloading(key)
		return app.refreshDownloadStates()
	}
	if !confirmed && uistate.ConfirmStatuses[status] {
		app.search.EndDownloading(key)
		return v.confirmInstall(app, result, key, status, state.ConflictFileName)
	}
	return v.doInstall(app, result, key)
}

func (v *downloadModel) doInstall(app *AppModel, result models.ModProject, key string) tea.Cmd {
	req := appstructs.ModDownloadRequest{
		ProjectID:        key,
		Result:           result,
		MinecraftVersion: app.search.SelectedVersion,
		ModLoader:        app.search.SelectedModLoader,
	}
	return cmdQueueModDownload(app.host, req)
}

func (v *downloadModel) confirmInstall(app *AppModel, result models.ModProject, key, status, conflictFile string) tea.Cmd {
	titleKey := "download.confirmReplace.updateTitle"
	bodyKey := "download.confirmReplace.updateBody"
	switch status {
	case "conflict":
		titleKey = "download.confirmReplace.conflictTitle"
		bodyKey = "download.confirmReplace.conflictBody"
	case "incompatible":
		titleKey = "download.confirmReplace.incompatibleTitle"
		bodyKey = "download.confirmReplace.incompatibleBody"
	}
	body := i18n.T(bodyKey)
	if conflictFile != "" {
		body += "\n" + i18n.T("download.confirmReplace.conflictFile", map[string]string{"file": conflictFile})
	}
	app.pushModal(newConfirmModal(
		"download-confirm",
		i18n.T(titleKey),
		body,
		[]choiceOption{
			{Label: i18n.T("download.confirmReplace.confirm"), Value: "confirm", Primary: true},
			{Label: i18n.T("download.confirmReplace.cancel"), Value: "cancel"},
		},
		func(app *AppModel, value string) tea.Cmd {
			if value != "confirm" {
				app.search.EndDownloading(key)
				return nil
			}
			return v.doInstall(app, result, key)
		},
	))
	return nil
}

func (v *downloadModel) stateFor(key string) appstructs.ModDownloadButtonState {
	for _, state := range v.app.search.DownloadStates {
		if state.Key == key {
			return state
		}
	}
	return appstructs.ModDownloadButtonState{}
}

func (v *downloadModel) openVersions(app *AppModel, action string) tea.Cmd {
	results := app.search.SearchResults
	if v.cursor >= len(results) {
		return nil
	}
	result := results[v.cursor]
	modal := newVersionListModal("download-versions", action, result, app.search.SelectedVersion, app.search.SelectedModLoader,
		func(app *AppModel, version models.ModVersion) tea.Cmd {
			return v.pinVersion(app, result, version)
		}, nil)
	app.pushModal(modal)
	return cmdListMatchingVersions(app.host, result, app.search.SelectedVersion, app.search.SelectedModLoader)
}

func (v *downloadModel) pinVersion(app *AppModel, result models.ModProject, version models.ModVersion) tea.Cmd {
	return cmdPinModVersion(app.host, appstructs.ModVersionPinRequest{
		Platform:         result.Platform,
		ModID:            result.ProjectID,
		VersionID:        version.ID,
		MinecraftVersion: app.search.SelectedVersion,
		ModLoader:        app.search.SelectedModLoader,
	})
}

func (v *downloadModel) addToFavorites(app *AppModel) tea.Cmd {
	targets := v.selectedResults()
	if len(targets) == 0 {
		results := app.search.SearchResults
		if v.cursor < len(results) {
			targets = []models.ModProject{results[v.cursor]}
		}
	}
	if len(targets) == 0 {
		return nil
	}
	drafts := make([]favoriteDraft, 0, len(targets))
	for _, result := range targets {
		drafts = append(drafts, favoriteDraftFromProject(result, app))
	}
	return openAddFavorites(app, drafts)
}

func (v *downloadModel) batchDownload(app *AppModel) tea.Cmd {
	targets := v.selectedResults()
	if len(targets) == 0 || !app.mc.HasSelectedInstance() {
		return nil
	}
	req := appstructs.BatchDownloadRequest{
		Results:          targets,
		MinecraftVersion: app.search.SelectedVersion,
		ModLoader:        app.search.SelectedModLoader,
	}
	return cmdAnalyzeBatchConflicts(app.host, req)
}

func (v *downloadModel) batchUnpin(app *AppModel) tea.Cmd {
	targets := v.selectedResults()
	if len(targets) == 0 {
		return nil
	}
	cmds := make([]tea.Cmd, 0, len(targets))
	for _, result := range targets {
		pin := app.host.Service().GetPinnedModVersion(result.Platform, result.ProjectID, app.search.SelectedVersion, app.search.SelectedModLoader)
		if pin.VersionID != "" {
			cmds = append(cmds, cmdPinModVersion(app.host, appstructs.ModVersionPinRequest{
				Platform:         result.Platform,
				ModID:            result.ProjectID,
				VersionID:        "",
				MinecraftVersion: app.search.SelectedVersion,
				ModLoader:        app.search.SelectedModLoader,
			}))
		}
	}
	cmds = append(cmds, app.refreshDownloadStates())
	return tea.Batch(cmds...)
}

func (v *downloadModel) chooseDir(app *AppModel) tea.Cmd {
	picker := newPathPickerModal("download-mc-dir", i18n.T("settings.minecraftDir.choose"), pickerDirectory, app.mc.MinecraftDir,
		func(app *AppModel, path string) tea.Cmd {
			return cmdSetMinecraftDir(app.host, path)
		})
	app.pushModal(picker)
	return picker.Init()
}

func (v *downloadModel) ClearFocus(app *AppModel) (bool, tea.Cmd) {
	if v.focused {
		v.focused = false
		v.input.Blur()
		return true, nil
	}
	if len(v.selected) > 0 {
		v.selected = map[string]bool{}
		return true, nil
	}
	return false, nil
}

func (v *downloadModel) View(app *AppModel) string {
	v.ensureInit()
	t := app.theme
	var b strings.Builder

	if !app.mc.HasSelectedInstance() {
		b.WriteString(t.Warning().Render(i18n.T("download.selectDirTitle")))
		b.WriteString("\n")
		b.WriteString(t.Muted().Render(i18n.T("download.selectDirDesc")))
		b.WriteString("\n\n")
		b.WriteString(t.Accent().Render("o " + i18n.T("settings.minecraftDir.choose")))
		return b.String()
	}

	// Search bar.
	label := i18n.T("search.label")
	b.WriteString(t.Muted().Render(label + ": "))
	if v.focused {
		b.WriteString(v.input.View())
		b.WriteString("  " + t.Accent().Render("↵ "+i18n.T("search.action")))
	} else {
		value := app.search.SearchText
		if value == "" {
			value = t.Muted().Render("/ " + i18n.T("search.action"))
		}
		b.WriteString(t.OnSurface().Render(value))
	}
	b.WriteString("  " + t.Muted().Render(app.targetTuple()))
	b.WriteString("\n")

	if sel := len(v.selected); sel > 0 {
		b.WriteString(t.Accent().Render(i18n.T("download.selection.count", map[string]string{"n": itoa(sel)})))
		b.WriteString("  " + t.Muted().Render("b "+i18n.T("download.selection.downloadAll")+" · u "+i18n.T("download.selection.unpin")+" · f "+i18n.T("favorites.addToFavorites")+" · esc "+i18n.T("download.selection.deselectAll")))
		b.WriteString("\n")
	}
	b.WriteString("\n")

	results := app.search.SearchResults
	if app.search.IsSearching && len(results) == 0 {
		b.WriteString(t.Muted().Render(uiText("loading")))
		return b.String()
	}
	if len(results) == 0 {
		b.WriteString(t.Muted().Render(i18n.T("manage.search.empty")))
		return b.String()
	}

	const rows = 8
	end := v.offset + rows
	if end > len(results) {
		end = len(results)
	}
	for i := v.offset; i < end; i++ {
		result := results[i]
		key := v.itemKey(result)
		state := v.stateFor(key)
		check := "  "
		if v.selected[key] {
			check = "[✓]"
		} else if v.selected != nil {
			check = "[ ]"
		}
		status := downloadStatusGlyph(t, state)
		line := check + " " + status + " " + truncate(result.Title, 34)
		line += "  " + t.Muted().Render(result.ProjectID)
		if result.Downloads > 0 {
			line += "  " + t.Muted().Render(compactNumber(result.Downloads))
		}
		if i == v.cursor {
			line = t.Selected().Render(stripStyles(line))
		} else {
			line = t.OnSurface().Render(line)
		}
		b.WriteString(line)
		b.WriteString("\n")
	}
	if app.search.IsLoadingMore {
		b.WriteString(t.Muted().Render(uiText("loading")))
		b.WriteString("\n")
	}
	if !app.search.HasMoreResults && len(results) > 0 {
		b.WriteString(t.Muted().Render(i18n.T("search.noMoreResults")))
	}
	return strings.TrimRight(b.String(), "\n")
}

func (v *downloadModel) FooterHints(app *AppModel) string {
	if v.focused {
		return "type · ↵ " + i18n.T("search.action") + " · esc " + uiText("cancel")
	}
	return "/ " + i18n.T("search.action") + " · ↑↓ " + uiText("navigate") + " · space " + uiText("space") + " · enter " + i18n.T("download.selection.downloadAll") + " · v " + i18n.T("favorites.actions.pin") + " · q " + i18n.T("download.queue.title")
}

func (v *downloadModel) RowCount() int { return len(v.app.search.SearchResults) }

func (v *downloadModel) MouseWheel(app *AppModel, delta int) tea.Cmd {
	v.move(delta)
	return nil
}

func (v *downloadModel) ClickRow(app *AppModel, index int, ev tea.MouseMsg) tea.Cmd {
	results := app.search.SearchResults
	if index < 0 || index >= len(results) {
		return nil
	}
	if ev.Ctrl {
		v.cursor = index
		v.toggleCurrent()
		return nil
	}
	if ev.Shift && v.hasAnchor {
		lo, hi := v.anchor, index
		if lo > hi {
			lo, hi = hi, lo
		}
		for i := lo; i <= hi && i < len(results); i++ {
			v.selected[v.itemKey(results[i])] = true
		}
		return nil
	}
	v.cursor = index
	v.anchor = index
	v.hasAnchor = true
	return nil
}

func (v *downloadModel) ListGeometry(app *AppModel) (header, offset, rows int) {
	header = 3
	return header, v.offset, len(app.search.SearchResults)
}

func downloadStatusGlyph(t Theme, state appstructs.ModDownloadButtonState) string {
	switch state.Status {
	case "installed":
		return t.Success().Render("✓")
	case "update":
		return t.Warning().Render("↑")
	case "conflict":
		return t.Danger().Render("⚠")
	case "incompatible":
		return t.Danger().Render("✗")
	case "metadataLoading":
		return t.Muted().Render("…")
	case "metadataFailed":
		return t.Danger().Render("!")
	case "install":
		return t.Accent().Render("↓")
	default:
		return t.Muted().Render("•")
	}
}
