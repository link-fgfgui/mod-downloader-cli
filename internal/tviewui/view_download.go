package tviewui

import (
	"strings"

	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"

	"github.com/link-fgfgui/mod-downloader-core/models"
	appstructs "github.com/link-fgfgui/mod-downloader-core/structs"

	"github.com/link-fgfgui/mod-downloader-cli/internal/i18n"
	"github.com/link-fgfgui/mod-downloader-cli/internal/uistate"
)

// downloadPage renders the search / install / pin view.
type downloadPage struct {
	app *App
	box *tview.Flex

	header    *tview.TextView
	search    *tview.InputField
	selection *tview.TextView
	results   *wheelList
	bar       *actionBar

	focusedSearch bool
	barLang       string
	selected      map[string]bool
	anchor        int
	hasAnchor     bool
	rebuilding    bool
}

func newDownloadPage(a *App) *downloadPage {
	v := &downloadPage{app: a, selected: map[string]bool{}}

	v.header = tview.NewTextView().SetDynamicColors(true)
	v.header.SetBackgroundColor(a.theme.Surface)

	v.search = tview.NewInputField()
	v.search.SetLabel(" › ")
	v.search.SetPlaceholder(i18n.T("search.label"))
	v.search.SetFieldWidth(0)
	v.search.SetBorder(true)
	v.search.SetTitle(i18n.T("search.label"))
	a.theme.styleInput(v.search)
	a.theme.styleBox(v.search.Box)
	v.search.SetDoneFunc(func(key tcell.Key) {
		switch key {
		case tcell.KeyEnter:
			v.focusedSearch = false
			v.app.application.SetFocus(v.results)
			v.runSearch(false)
		case tcell.KeyEscape:
			v.focusedSearch = false
			v.app.application.SetFocus(v.results)
			v.app.updateStatus()
		}
	})
	v.search.SetChangedFunc(func(text string) {
		v.app.search.SearchText = text
	})

	v.selection = tview.NewTextView().SetDynamicColors(true)
	v.selection.SetBackgroundColor(a.theme.Surface)
	v.selection.SetTextColor(a.theme.OnSurface)

	v.results = newWheelList()
	v.results.ShowSecondaryText(true)
	v.results.SetWrapAround(false)
	v.results.SetUseStyleTags(true, true)
	a.theme.styleList(v.results.List)
	v.results.SetBorder(true)
	v.results.SetTitle(i18n.T("nav.download"))
	a.theme.styleBox(v.results.Box)
	// A click toggles the checkbox; installing stays on Enter / the action bar
	// so the mouse can select without side effects.
	v.results.SetSelectedFunc(func(index int, _, _ string, _ rune) {
		v.toggleAt(index)
	})
	v.results.SetChangedFunc(func(index int, _, _ string, _ rune) {
		v.onCursorChanged(index)
	})

	v.bar = newActionBar(a)
	v.bar.SetRefocus(v.Focus)

	v.box = tview.NewFlex().SetDirection(tview.FlexRow).
		AddItem(v.header, 2, 0, false).
		AddItem(v.search, 3, 0, false).
		AddItem(v.selection, 1, 0, false).
		AddItem(v.results, 0, 1, true).
		AddItem(v.bar.Primitive(), 1, 0, false)

	v.buildActions()
	v.Refresh()
	v.renderHeader()
	return v
}

func (v *downloadPage) Primitive() tview.Primitive { return v.box }

func (v *downloadPage) Focus() {
	if v.focusedSearch {
		v.app.application.SetFocus(v.search)
		return
	}
	v.app.application.SetFocus(v.results)
}

func (v *downloadPage) OnActivate() {
	if v.focusedSearch {
		v.app.application.SetFocus(v.search)
	}
	// The page skips repaints while hidden, so rebuild it on the way in.
	v.Refresh()
	v.app.refreshDownloadStates()
}

func (v *downloadPage) ClearFocus() bool {
	if v.focusedSearch {
		v.focusedSearch = false
		v.search.Blur()
		v.app.application.SetFocus(v.results)
		v.app.updateStatus()
		return true
	}
	if len(v.selected) > 0 {
		v.selected = map[string]bool{}
		v.Refresh()
		return true
	}
	return false
}

func (v *downloadPage) HandleKey(event *tcell.EventKey) bool {
	if v.focusedSearch {
		if event.Key() == tcell.KeyTab || event.Key() == tcell.KeyBacktab {
			v.focusedSearch = false
			v.search.Blur()
			v.app.application.SetFocus(v.results)
			return true
		}
		return false
	}

	if event.Key() == tcell.KeyTab || event.Key() == tcell.KeyBacktab {
		v.focusedSearch = true
		v.app.application.SetFocus(v.search)
		return true
	}
	if event.Key() == tcell.KeyEnter {
		v.installCurrent(v.results.GetCurrentItem())
		return true
	}
	if event.Key() == tcell.KeyCtrlA {
		v.selectAll()
		return true
	}
	if event.Key() != tcell.KeyRune {
		return false
	}
	switch event.Rune() {
	case '/', 's':
		v.focusedSearch = true
		v.app.application.SetFocus(v.search)
		return true
	case ' ':
		v.toggleCurrent()
		return true
	case 'l':
		v.runSearch(true)
		return true
	case 'i':
		v.installCurrent(v.results.GetCurrentItem())
		return true
	case 'v':
		v.openVersions()
		return true
	case 'f':
		v.addToFavorites()
		return true
	case 'b':
		v.batchDownload()
		return true
	case 'u':
		v.batchUnpin()
		return true
	case 'o':
		if !v.app.mc.HasSelectedInstance() {
			v.chooseDir()
			return true
		}
	case 'r':
		v.app.refreshDownloadStates()
		v.Refresh()
		return true
	}
	return false
}

func (v *downloadPage) Hints() []hint {
	if v.focusedSearch {
		return []hint{
			hintText("type"),
			hintText("↵ " + i18n.T("search.action")),
			hintText("tab " + uiText("navigate")),
			hintText("esc " + uiText("cancel")),
		}
	}
	if len(v.selected) > 0 {
		return []hint{
			hintText(i18n.T("download.selection.count", map[string]string{"n": itoa(len(v.selected))})),
			hintAction("b "+i18n.T("download.selection.downloadAll"), v.batchDownload),
			hintAction("u "+i18n.T("download.selection.unpin"), v.batchUnpin),
			hintAction("f "+i18n.T("favorites.addToFavorites"), v.addToFavorites),
			hintAction("esc "+i18n.T("download.selection.deselectAll"), func() { v.ClearFocus() }),
		}
	}
	return []hint{
		hintAction("/ "+i18n.T("search.action"), func() {
			v.focusedSearch = true
			v.app.application.SetFocus(v.search)
			v.app.updateStatus()
		}),
		hintText("↑↓ " + uiText("navigate")),
		hintAction("space "+uiText("space"), v.toggleCurrent),
		hintAction("enter "+uiText("stateInstall"), func() { v.installCurrent(v.results.GetCurrentItem()) }),
		hintAction("v "+i18n.T("favorites.actions.pin"), v.openVersions),
		hintAction("f "+i18n.T("favorites.addToFavorites"), v.addToFavorites),
		hintAction("q "+i18n.T("download.queue.title"), v.app.toggleQueue),
	}
}

func (v *downloadPage) OnResize(_ int) {
	v.renderHeader()
	v.bar.Reflow()
}

func (v *downloadPage) Restyle() {
	t := v.app.theme
	t.styleInput(v.search)
	t.styleBox(v.search.Box)
	t.styleList(v.results.List)
	t.styleBox(v.results.Box)
	v.header.SetBackgroundColor(t.Surface)
	v.selection.SetBackgroundColor(t.Surface)
	v.selection.SetTextColor(t.OnSurface)
	v.bar.Restyle()
}

func (v *downloadPage) renderHeader() {
	t := v.app.theme
	v.header.SetText(t.tagWith(t.Primary, "b") + i18n.T("nav.download") + resetTag +
		"  " + t.tag(t.Muted) + tview.Escape(v.app.targetTuple()) + resetTag)
}

// buildActions (re)labels the clickable action bar for the current language.
func (v *downloadPage) buildActions() {
	v.barLang = i18n.Current()
	v.bar.Set(
		pageAction{Label: uiText("stateInstall"), Run: func() { v.installCurrent(v.results.GetCurrentItem()) }},
		pageAction{Label: i18n.T("download.selection.downloadAll"), Run: func() { v.batchDownload() }},
		pageAction{Label: uiText("selectAll"), Run: func() { v.selectAll() }},
		pageAction{Label: uiText("target"), Run: func() { v.app.openTargetDialog() }},
		pageAction{Label: i18n.T("favorites.addToFavorites"), Run: func() { v.addToFavorites() }},
	)
}

// Refresh rebuilds the result list from shared state.
func (v *downloadPage) Refresh() {
	if v.barLang != i18n.Current() {
		v.buildActions()
	}
	v.search.SetTitle(i18n.T("search.label"))
	v.search.SetPlaceholder(i18n.T("search.label"))
	v.renderHeader()
	v.rebuildList()
	v.rebuildSelectionBar()
}

func (v *downloadPage) rebuildSelectionBar() {
	if len(v.selected) == 0 {
		v.selection.SetText("")
		return
	}
	t := v.app.theme
	text := i18n.T("download.selection.count", map[string]string{"n": itoa(len(v.selected))}) +
		"  b " + i18n.T("download.selection.downloadAll") +
		" · u " + i18n.T("download.selection.unpin") +
		" · f " + i18n.T("favorites.addToFavorites") +
		" · esc " + i18n.T("download.selection.deselectAll")
	v.selection.SetText(t.tagWith(t.Secondary, "b") + text + resetTag)
}

func (v *downloadPage) rebuildList() {
	current := v.currentKey()
	v.rebuilding = true
	defer func() { v.rebuilding = false }()

	idx := v.buildRowIndex()

	// Drop selections whose results are no longer visible.
	visible := make(map[string]bool, len(v.app.search.SearchResults))
	for _, result := range v.app.search.SearchResults {
		visible[idx.keyOf(result)] = true
	}
	for selectedKey := range v.selected {
		if !visible[selectedKey] {
			delete(v.selected, selectedKey)
		}
	}

	v.results.Clear()
	if !v.app.mc.HasSelectedInstance() {
		v.results.SetTitle(i18n.T("download.selectDirTitle"))
		v.results.AddItem(v.app.theme.tag(v.app.theme.Muted)+i18n.T("download.selectDirDesc")+resetTag, "", 0, nil)
		v.results.AddItem(v.app.theme.tag(v.app.theme.Secondary)+"o "+i18n.T("settings.minecraftDir.choose")+resetTag, "", 0, nil)
		return
	}
	v.results.SetTitle(i18n.T("nav.download"))

	results := v.app.search.SearchResults
	if v.app.search.IsSearching && len(results) == 0 {
		v.results.AddItem(v.app.theme.tag(v.app.theme.Muted)+uiText("loading")+resetTag, "", 0, nil)
		return
	}
	if len(results) == 0 {
		v.results.AddItem(v.app.theme.tag(v.app.theme.Muted)+i18n.T("manage.search.empty")+resetTag, "", 0, nil)
		return
	}

	for _, result := range results {
		key := idx.keyOf(result)
		v.results.AddItem(v.itemMain(idx, result, key), v.itemSecondary(result), 0, nil)
	}
	if current != "" {
		for i, result := range results {
			if idx.keyOf(result) == current {
				v.results.SetCurrentItem(i)
				return
			}
		}
	}
	if cur := v.results.GetCurrentItem(); cur >= len(results) {
		v.results.SetCurrentItem(maxInt(0, len(results)-1))
	}
}

func (v *downloadPage) currentKey() string {
	results := v.app.search.SearchResults
	idx := v.results.GetCurrentItem()
	if idx < 0 || idx >= len(results) {
		return ""
	}
	return v.itemKey(results[idx])
}

func (v *downloadPage) itemKey(result models.ModProject) string {
	return v.app.search.StateForResultKey(result)
}

func (v *downloadPage) itemMain(idx *rowIndex, result models.ModProject, key string) string {
	t := v.app.theme
	check := checkBoxTagged(v.selected[key])
	state := idx.stateOf(key)
	glyph, label := downloadStateGlyph(t, state)
	if item, ok := idx.queueItemFor(result); ok {
		switch item.Status {
		case "running":
			glyph = t.tag(t.Secondary) + "↓" + resetTag
			label = downloadProgressLabel(t, item)
		case "pending":
			glyph = t.tag(t.Muted) + "…" + resetTag
			label = t.tag(t.Muted) + uiText("queued") + resetTag
		case "failed":
			glyph = t.tag(t.Error) + "!" + resetTag
			label = t.tag(t.Error) + uiText("failed") + resetTag
		}
	}
	return check + " " + glyph + " " + tview.Escape(result.Title) + "   " + label
}

func (v *downloadPage) itemSecondary(result models.ModProject) string {
	t := v.app.theme
	right := result.Platform
	if result.Downloads > 0 {
		right += " · " + compactNumber(result.Downloads)
	}
	desc := result.Description
	if desc == "" {
		desc = result.ProjectID
	}
	return t.tag(t.Muted) + tview.Escape(truncate(desc, maxInt(20, v.app.mainWidth()-30))) + resetTag +
		"  " + t.tag(t.Muted) + tview.Escape(right) + resetTag
}

func (v *downloadPage) stateFor(key string) appstructs.ModDownloadButtonState {
	for _, state := range v.app.search.DownloadStates {
		if state.Key == key {
			return state
		}
	}
	return appstructs.ModDownloadButtonState{}
}

func downloadStateGlyph(t Theme, state appstructs.ModDownloadButtonState) (string, string) {
	switch state.Status {
	case "installed":
		return t.tag(t.Success) + "✓" + resetTag, t.tag(t.Success) + uiText("stateInstalled") + resetTag
	case "update":
		return t.tag(t.Warning) + "↑" + resetTag, t.tag(t.Warning) + uiText("stateUpdate") + resetTag
	case "conflict":
		return t.tag(t.Error) + "⚠" + resetTag, t.tag(t.Error) + uiText("stateConflict") + resetTag
	case "incompatible":
		return t.tag(t.Error) + "✗" + resetTag, t.tag(t.Error) + uiText("stateIncompatible") + resetTag
	case "metadataLoading":
		return t.tag(t.Muted) + "…" + resetTag, t.tag(t.Muted) + uiText("loading") + resetTag
	case "metadataFailed":
		return t.tag(t.Error) + "!" + resetTag, t.tag(t.Error) + uiText("failed") + resetTag
	case "install":
		return t.tag(t.Secondary) + "↓" + resetTag, t.tag(t.Secondary) + uiText("stateInstall") + resetTag
	default:
		return t.tag(t.Muted) + "•" + resetTag, ""
	}
}

func downloadProgressLabel(t Theme, item appstructs.DownloadQueueItem) string {
	if item.TotalBytes > 0 {
		return t.tag(t.Secondary) + formatPercent(item.BytesComplete, item.TotalBytes) + resetTag
	}
	return t.tag(t.Secondary) + uiText("loading") + resetTag
}

// projectIdentity is the comparable identity uistate.sameProject compares.
type projectIdentity struct {
	id        string
	platform  string
	projectID string
	slug      string
}

func identityOfProject(result models.ModProject) projectIdentity {
	return projectIdentity{id: result.ID, platform: result.Platform, projectID: result.ProjectID, slug: result.Slug}
}

// rowIndex is a per-refresh lookup cache for the result list. Building it once
// keeps a refresh linear in the number of results; looking the same data up per
// row made it quadratic, which froze the UI on long (accumulated) result lists.
type rowIndex struct {
	keys    map[projectIdentity]string
	states  map[string]appstructs.ModDownloadButtonState
	running map[string]appstructs.DownloadQueueItem
	pending map[string]appstructs.DownloadQueueItem
}

func (v *downloadPage) buildRowIndex() *rowIndex {
	search := v.app.search
	idx := &rowIndex{
		keys:    make(map[projectIdentity]string, len(search.SearchResults)),
		states:  make(map[string]appstructs.ModDownloadButtonState, len(search.DownloadStates)),
		running: make(map[string]appstructs.DownloadQueueItem),
		pending: make(map[string]appstructs.DownloadQueueItem),
	}
	// First occurrence wins, mirroring SearchState.StateForResultKey.
	for i, result := range search.SearchResults {
		identity := identityOfProject(result)
		if _, ok := idx.keys[identity]; ok {
			continue
		}
		key := result.ID
		if i < len(search.DownloadStates) && search.DownloadStates[i].Key != "" {
			key = search.DownloadStates[i].Key
		}
		idx.keys[identity] = key
	}
	for _, state := range search.DownloadStates {
		if state.Key == "" {
			continue
		}
		if _, ok := idx.states[state.Key]; !ok {
			idx.states[state.Key] = state
		}
	}
	for _, item := range v.app.queueState.Queue.Items {
		projectKey := strings.ToLower(strings.TrimSpace(item.ProjectID))
		if projectKey == "" {
			continue
		}
		switch item.Status {
		case "running":
			if _, ok := idx.running[projectKey]; !ok {
				idx.running[projectKey] = item
			}
		case "pending":
			if _, ok := idx.pending[projectKey]; !ok {
				idx.pending[projectKey] = item
			}
		}
	}
	return idx
}

// keyOf mirrors SearchState.StateForResultKey.
func (idx *rowIndex) keyOf(result models.ModProject) string {
	if key, ok := idx.keys[identityOfProject(result)]; ok {
		return key
	}
	return result.ID
}

// stateOf mirrors downloadPage.stateFor.
func (idx *rowIndex) stateOf(key string) appstructs.ModDownloadButtonState {
	return idx.states[key]
}

// queueItemFor returns the running entry for a result, falling back to the
// first pending one.
func (idx *rowIndex) queueItemFor(result models.ModProject) (appstructs.DownloadQueueItem, bool) {
	candidates := [3]string{
		strings.ToLower(strings.TrimSpace(result.ProjectID)),
		strings.ToLower(strings.TrimSpace(result.ID)),
		strings.ToLower(strings.TrimSpace(models.ProjectKey(result.Platform, result.ProjectID))),
	}
	for _, candidate := range candidates {
		if candidate == "" {
			continue
		}
		if item, ok := idx.running[candidate]; ok {
			return item, true
		}
	}
	for _, candidate := range candidates {
		if candidate == "" {
			continue
		}
		if item, ok := idx.pending[candidate]; ok {
			return item, true
		}
	}
	return appstructs.DownloadQueueItem{}, false
}

func (v *downloadPage) onCursorChanged(index int) {
	if v.rebuilding {
		return
	}
	if !v.app.search.HasMoreResults || v.app.search.IsLoadingMore || v.app.search.IsSearching {
		return
	}
	results := v.app.search.SearchResults
	if index >= len(results)-1 && len(results) > 0 {
		v.runSearch(true)
	}
}

func (v *downloadPage) toggleCurrent() { v.toggleAt(v.results.GetCurrentItem()) }

// toggleAt flips the checkbox of one row and repaints only that row.
func (v *downloadPage) toggleAt(index int) {
	results := v.app.search.SearchResults
	if index < 0 || index >= len(results) {
		return
	}
	key := v.itemKey(results[index])
	if v.selected[key] {
		delete(v.selected, key)
	} else {
		v.selected[key] = true
	}
	v.anchor = index
	v.hasAnchor = true
	v.rebuildSelectionBar()
	v.refreshCurrentItemText(index)
}

// selectAll ticks every visible result.
func (v *downloadPage) selectAll() {
	idx := v.buildRowIndex()
	for _, result := range v.app.search.SearchResults {
		v.selected[idx.keyOf(result)] = true
	}
	v.Refresh()
}

// refreshCurrentItemText updates one row in place to avoid rebuilding the list
// (which would reset the cursor while navigating with the keyboard).
func (v *downloadPage) refreshCurrentItemText(index int) {
	results := v.app.search.SearchResults
	if index < 0 || index >= len(results) || v.results.GetItemCount() != len(results) {
		v.Refresh()
		return
	}
	idx := v.buildRowIndex()
	result := results[index]
	key := idx.keyOf(result)
	v.results.SetItemText(index, v.itemMain(idx, result, key), v.itemSecondary(result))
}

func (v *downloadPage) selectedResults() []models.ModProject {
	out := []models.ModProject{}
	if len(v.selected) == 0 {
		return out
	}
	idx := v.buildRowIndex()
	for _, result := range v.app.search.SearchResults {
		if v.selected[idx.keyOf(result)] {
			out = append(out, result)
		}
	}
	return out
}

func (v *downloadPage) runSearch(appendMode bool) {
	v.app.search.SearchText = v.search.GetText()
	requestID, offset, proceed := v.app.search.BeginSearch(appendMode)
	if !proceed {
		v.app.refreshDownloadStates()
		v.Refresh()
		return
	}
	req := appstructs.SearchModsRequest{
		RequestID: requestID,
		Query:     v.app.search.SearchText,
		Version:   v.app.search.SelectedVersion,
		ModLoader: v.app.search.SelectedModLoader,
		Offset:    offset,
		Limit:     uistate.SearchPageSize,
	}
	v.app.search.AdvanceOffset(requestID, offset)
	go v.app.searchMods(req)
	v.Refresh()
}

func (v *downloadPage) installCurrent(index int) {
	results := v.app.search.SearchResults
	if index < 0 || index >= len(results) {
		return
	}
	v.install(results[index], false)
}

// installBatch queues every result without the per-item confirmation prompt;
// the batch preflight already handled incompatible conflicts.
func (v *downloadPage) installBatch(results []models.ModProject) {
	for _, result := range results {
		key := v.itemKey(result)
		if key == "" || !v.app.search.BeginDownloading(key) {
			continue
		}
		v.doInstall(result, key)
	}
}

func (v *downloadPage) install(result models.ModProject, confirmed bool) {
	app := v.app
	if !app.mc.HasSelectedInstance() || app.search.SelectedVersion == "" || app.search.SelectedModLoader == "" {
		return
	}
	key := v.itemKey(result)
	if !app.search.BeginDownloading(key) {
		return
	}
	state := v.stateFor(key)
	status := state.Status
	if uistate.MetadataRetryStatuses[status] {
		app.search.EndDownloading(key)
		app.toastT("download.errors.metadataLoading")
		app.refreshDownloadStates()
		return
	}
	if !confirmed && uistate.ConfirmStatuses[status] {
		app.search.EndDownloading(key)
		v.confirmInstall(result, key, status, state.ConflictFileName)
		return
	}
	v.doInstall(result, key)
}

func (v *downloadPage) doInstall(result models.ModProject, key string) {
	req := appstructs.ModDownloadRequest{
		ProjectID:        key,
		Result:           result,
		MinecraftVersion: v.app.search.SelectedVersion,
		ModLoader:        v.app.search.SelectedModLoader,
	}
	go v.app.queueDownload(req)
}

func (v *downloadPage) confirmInstall(result models.ModProject, key, status, conflictFile string) {
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
	v.app.openConfirm(
		i18n.T(titleKey),
		body,
		[]confirmOption{
			{Label: i18n.T("download.confirmReplace.confirm"), Value: "confirm", Primary: true},
			{Label: i18n.T("download.confirmReplace.cancel"), Value: "cancel"},
		},
		func(value string) {
			if value != "confirm" {
				v.app.search.EndDownloading(key)
				return
			}
			v.doInstall(result, key)
		},
	)
}

func (v *downloadPage) openVersions() {
	results := v.app.search.SearchResults
	idx := v.results.GetCurrentItem()
	if idx < 0 || idx >= len(results) {
		return
	}
	result := results[idx]
	v.app.openVersionDialog(result, "pin", func(version models.ModVersion) {
		req := appstructs.ModVersionPinRequest{
			Platform:         result.Platform,
			ModID:            result.ProjectID,
			VersionID:        version.ID,
			MinecraftVersion: v.app.search.SelectedVersion,
			ModLoader:        v.app.search.SelectedModLoader,
		}
		go v.app.pinModVersion(req)
	}, nil)
}

func (v *downloadPage) addToFavorites() {
	targets := v.selectedResults()
	if len(targets) == 0 {
		results := v.app.search.SearchResults
		idx := v.results.GetCurrentItem()
		if idx >= 0 && idx < len(results) {
			targets = []models.ModProject{results[idx]}
		}
	}
	if len(targets) == 0 {
		return
	}
	drafts := make([]favoriteDraft, 0, len(targets))
	for _, result := range targets {
		drafts = append(drafts, favoriteDraftFromProject(result, v.app))
	}
	v.app.openAddFavoriteDialog(drafts)
}

func (v *downloadPage) batchDownload() {
	targets := v.selectedResults()
	if len(targets) == 0 || !v.app.mc.HasSelectedInstance() {
		return
	}
	req := appstructs.BatchDownloadRequest{
		Results:          targets,
		MinecraftVersion: v.app.search.SelectedVersion,
		ModLoader:        v.app.search.SelectedModLoader,
	}
	go v.app.analyzeBatchConflicts(req)
}

func (v *downloadPage) batchUnpin() {
	targets := v.selectedResults()
	if len(targets) == 0 {
		return
	}
	go v.app.batchUnpin(targets, v.app.search.SelectedVersion, v.app.search.SelectedModLoader)
}

func (v *downloadPage) chooseDir() {
	v.app.openPathDialog(i18n.T("settings.minecraftDir.choose"), false, v.app.mc.MinecraftDir,
		func(path string) { go v.app.setMinecraftDir(path) })
}
