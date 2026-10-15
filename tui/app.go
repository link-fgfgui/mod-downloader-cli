package tui

import (
	"fmt"
	"os"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/link-fgfgui/mod-downloader-core/appcore"

	"github.com/link-fgfgui/mod-downloader-cli/internal/uistate"
	"github.com/link-fgfgui/mod-downloader-cli/tui/i18n"
)

// ViewID enumerates the six primary views (mirrors the GUI router).
type ViewID int

const (
	ViewHome ViewID = iota
	ViewDownload
	ViewManage
	ViewFavorites
	ViewUnpin
	ViewSettings
)

var viewCount = 6

// view is implemented by every primary view.
type view interface {
	Update(app *AppModel, msg tea.Msg) tea.Cmd
	View(app *AppModel) string
}

// listView is optionally implemented by views that own a scrollable, selectable
// list so the shell can route mouse wheel + click events.
type listView interface {
	RowCount() int
	ClickRow(app *AppModel, index int, ev tea.MouseMsg) tea.Cmd
	MouseWheel(app *AppModel, delta int) tea.Cmd
}

type toast struct {
	text    string
	color   toastColor
	expires time.Time
}

// AppModel is the single Bubble Tea model: it owns navigation, the modal stack,
// toasts, the shared instance/target tuple, and the six views.
type AppModel struct {
	host  *Host
	theme Theme

	width  int
	height int

	active ViewID

	modals modalStack
	toasts []toast

	queue       uistate.QueueState
	queueOpen   bool
	queueCursor int

	mc     uistate.MinecraftState
	search uistate.SearchState

	settingsView appcore.SettingsView

	home      homeModel
	download  downloadModel
	manage    manageModel
	favorites favoritesModel
	unpin     unpinModel
	settings  settingsModel

	views []view

	quitting   bool
	serverNote string
	ready      bool

	// topBarRegions records the interactive segments of the second top-bar row
	// (instance/target area and the Connector toggle) so mouse clicks hit the
	// same spans that were rendered.
	topBarRegions []hitRegion
}

// hitRegion is a clickable span on a rendered row.
type hitRegion struct {
	name        string
	row, x0, x1 int
}

// NewAppModel constructs the root model, applying persisted preferences.
func NewAppModel(h *Host) *AppModel {
	prefs := h.Service().GetPreferences()
	i18n.Apply(prefs.Language)

	m := &AppModel{
		host:       h,
		theme:      NewTheme(prefs.Theme, false),
		queue:      uistate.NewQueueState(),
		queueOpen:  false,
		mc:         uistate.NewMinecraftState(),
		search:     uistate.SearchState{SelectedModLoader: uistate.DefaultModLoader},
		serverNote: h.ServerNotice(),
		// Fallback geometry until the first WindowSizeMsg arrives, so the shell
		// renders immediately even when the terminal size cannot be queried.
		width:  defaultWidth,
		height: defaultHeight,
		ready:  true,
	}
	m.views = []view{&m.home, &m.download, &m.manage, &m.favorites, &m.unpin, &m.settings}
	m.home.app = m
	m.download.app = m
	m.manage.app = m
	m.favorites.app = m
	m.unpin.app = m
	m.settings.app = m
	return m
}

// Init bootstraps the shell: instance/versions + queue + preferences.
func (m *AppModel) Init() tea.Cmd {
	return tea.Batch(
		cmdLoadMinecraft(m.host),
		cmdGetQueueState(m.host),
		cmdGetUsageStats(m.host),
		cmdGetSettings(m.host),
	)
}

func (m *AppModel) current() view         { return m.views[m.active] }
func (m *AppModel) pushModal(modal Modal) { m.modals.push(modal) }
func (m *AppModel) popModal(id string) {
	if m.modals.top() != nil && m.modals.top().ID() == id {
		m.modals.pop()
		return
	}
	// Fall back to removing by scanning (defensive).
	var rebuilt modalStack
	for _, existing := range m.modals {
		if existing.ID() != id {
			rebuilt = append(rebuilt, existing)
		}
	}
	m.modals = rebuilt
}

func (m *AppModel) hasModal(id string) bool { return m.modals.has(id) }

// closeTopModal closes the topmost modal returning its cancel command.
func (m *AppModel) closeTopModal() tea.Cmd {
	top := m.modals.top()
	if top == nil {
		return nil
	}
	m.modals.pop()
	return nil
}

func (m *AppModel) toast(text string, color toastColor) tea.Cmd {
	m.toasts = append(m.toasts, toast{text: text, color: color, expires: time.Now().Add(4 * time.Second)})
	if len(m.toasts) > 3 {
		m.toasts = m.toasts[len(m.toasts)-3:]
	}
	return tea.Tick(4*time.Second, func(time.Time) tea.Msg { return toastExpireMsg{} })
}

type toastExpireMsg struct{}

func (m *AppModel) toastT(key string, color toastColor, params ...map[string]string) tea.Cmd {
	return m.toast(i18n.T(key, params...), color)
}

// CurrentTheme exposes the active theme for views.
func (m *AppModel) CurrentTheme() Theme { return m.theme }

// --- update -------------------------------------------------------------------

func (m *AppModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	// 1. Terminal resize.
	if size, ok := msg.(tea.WindowSizeMsg); ok {
		m.width = size.Width
		m.height = size.Height
		m.ready = true
		// Propagate size to text input-backed modals implicitly via View.
		return m, nil
	}

	var cmds []tea.Cmd

	// 2. Shared/async messages mutate cross-view state.
	if cmd, handled := m.handleSharedMessage(msg); handled {
		if cmd != nil {
			cmds = append(cmds, cmd)
		}
		return m, tea.Batch(cmds...)
	}

	// 3. Route input to the top capturing modal.
	if key, ok := msg.(tea.KeyMsg); ok {
		if top := m.modals.top(); top != nil && top.CapturesInput() {
			if cmd := top.Update(m, msg); cmd != nil {
				cmds = append(cmds, cmd)
			}
			return m, tea.Batch(cmds...)
		}
		// A view with a focused text input owns every keystroke (the GUI's
		// "非输入框焦点时" guard for digit/tab shortcuts).
		if capturer, ok := m.current().(interface {
			CapturesKeys(app *AppModel) bool
		}); ok && capturer.CapturesKeys(m) {
			if cmd := m.current().Update(m, key); cmd != nil {
				cmds = append(cmds, cmd)
			}
			return m, tea.Batch(cmds...)
		}
		if cmd, handled := m.handleGlobalKey(key); handled {
			if cmd != nil {
				cmds = append(cmds, cmd)
			}
			return m, tea.Batch(cmds...)
		}
		if m.queueOpen {
			if cmd := m.queueDrawerKey(key); cmd != nil {
				cmds = append(cmds, cmd)
			}
			return m, tea.Batch(cmds...)
		}
		if cmd := m.current().Update(m, key); cmd != nil {
			cmds = append(cmds, cmd)
		}
		return m, tea.Batch(cmds...)
	}

	if mouse, ok := msg.(tea.MouseMsg); ok {
		if top := m.modals.top(); top != nil && top.CapturesInput() {
			if cmd := top.Update(m, msg); cmd != nil {
				cmds = append(cmds, cmd)
			}
			return m, tea.Batch(cmds...)
		}
		if cmd := m.handleMouse(mouse); cmd != nil {
			cmds = append(cmds, cmd)
		}
		return m, tea.Batch(cmds...)
	}

	// 4. Everything else flows to the active view.
	if cmd := m.current().Update(m, msg); cmd != nil {
		cmds = append(cmds, cmd)
	}
	return m, tea.Batch(cmds...)
}

// handleSharedMessage processes adapter + async command results shared by all
// views. It returns handled=true when the message was consumed.
func (m *AppModel) handleSharedMessage(msg tea.Msg) (tea.Cmd, bool) {
	switch typed := msg.(type) {
	case minecraftSnapshotMsg:
		m.mc.Versions = typed.Versions
		m.mc.ReleaseVersions = typed.ReleaseList
		m.mc.MinecraftDir = typed.MinecraftDir
		m.mc.ApplySelectedVersion(&typed.Selected)
		m.search.SetTargetTuple(m.mc.SelectedMinecraftVersion, m.mc.SelectedModLoader, m.mc.HasSelectedInstance())
		m.favorites.SetScope(m.mc.SelectedMinecraftVersion, m.mc.SelectedModLoader)
		var cmd tea.Cmd
		if m.mc.EnsureSelectedModsLoadedNeeded() {
			cmd = cmdRefreshSelectedMods(m.host)
		}
		return tea.Batch(cmd, m.toastT("home.refresh", toastSuccess)), true

	case SelectedVersionChangedMsg:
		version := typed.Version
		m.mc.ApplySelectedVersion(&version)
		m.search.SetTargetTuple(m.mc.SelectedMinecraftVersion, m.mc.SelectedModLoader, m.mc.HasSelectedInstance())
		m.favorites.SetScope(m.mc.SelectedMinecraftVersion, m.mc.SelectedModLoader)
		return cmdRefreshSelectedMods(m.host), true

	case MinecraftDirChangedMsg:
		return cmdRefreshMinecraftDir(m.host), true

	case SearchModsUpdatedMsg:
		_, refresh := m.search.ApplySearchUpdate(typed.Update)
		if refresh {
			return m.currentRefreshStates(), true
		}
		return nil, true

	case ExtensionModsAcceptedMsg:
		if m.search.ApplyExtensionResults(typed.Update.Results) {
			m.active = ViewDownload
			return tea.Batch(m.currentRefreshStates(), m.toastT("download.queued", toastSuccess)), true
		}
		return nil, true

	case DownloadStatesUpdatedMsg:
		return m.currentRefreshStates(), true

	case searchResultMsg:
		m.search.ApplySearchUpdate(typed.Update)
		return nil, false // let the download view observe completion

	case downloadStatesMsg:
		if m.search.ActiveDownloadStateRequest == typed.RequestID {
			m.search.DownloadStates = typed.States
		}
		return nil, false

	case DownloadQueueUpdatedMsg:
		ring := m.queue.ApplyQueue(typed.State)
		m.queue.SyncPresentation()
		if ring {
			cmds := []tea.Cmd{m.currentRefreshStates()}
			cmds = append(cmds, ringBell())
			return tea.Batch(cmds...), true
		}
		return m.currentRefreshStates(), true

	case queueRefreshMsg:
		return cmdGetQueueState(m.host), true

	case DownloadCompletedMsg:
		m.queue.NoteDownloadCompleted()
		return nil, true

	case DownloadFailedMsg:
		fileName := typed.Event.FileName
		if fileName == "" {
			fileName = i18n.T("download.errors.unknownFile")
		}
		reason := typed.Event.Reason
		if reason == "" {
			reason = i18n.T("download.errors.generic")
		}
		return tea.Batch(
			m.toastT("download.errors.failedWithReason", toastError, map[string]string{"fileName": fileName, "reason": reason}),
			m.currentRefreshStates(),
		), true

	case UsageStatsUpdatedMsg:
		m.home.ApplyStats(typed.Stats)
		return nil, true

	case settingsLoadedMsg:
		m.applySettingsView(typed.View)
		return m.current().Update(m, typed), true

	case settingsSavedMsg:
		m.applySettingsView(typed.View)
		if typed.Toast != "" {
			return tea.Batch(m.toastT(typed.Toast, toastSuccess), m.current().Update(m, typed)), true
		}
		return m.current().Update(m, typed), true

	case errMsg:
		if typed.Err == nil {
			return m.toast(typed.Context, toastWarning), true
		}
		return m.toast(typed.Context+": "+typed.Err.Error(), toastError), true

	case toastMsg:
		return m.toast(typed.Text, typed.Color), true

	case toastExpireMsg:
		m.pruneToasts()
		return nil, true

	case favoritesLoadedMsg, matchingVersionsMsg, pinnedMsg, restoreDependencyMsg:
		// These results are consumed by the top modal when one is open, and by
		// the active view otherwise.
		return m.dispatchResult(msg), true

	case localModsMsg:
		if typed.Err == nil {
			m.mc.ApplySelectedVersion(&typed.Version)
		}
		return m.current().Update(m, typed), true

	case pinnedModsMsg:
		return m.current().Update(m, typed), true

	case batchConflictMsg, disableImpactMsg, unusedScanMsg,
		crossVersionPreviewMsg, crossVersionAppliedMsg,
		bulkFavoriteResultMsg, exportPackwizMsg, downloadQueuedMsg, downloadSkipMsg,
		installWaitMsg, projectResolvedMsg, searchSnapshotMsg:
		return m.current().Update(m, typed), true
	}
	return nil, false
}

// dispatchResult routes a result message to the top modal if present, else to
// the active view.
func (m *AppModel) dispatchResult(msg tea.Msg) tea.Cmd {
	if top := m.modals.top(); top != nil {
		return top.Update(m, msg)
	}
	return m.current().Update(m, msg)
}

// applySettingsView stores the settings snapshot and refreshes the active theme,
// language and simple-mode flags live.
func (m *AppModel) applySettingsView(view appcore.SettingsView) {
	m.settingsView = view
	i18n.Apply(view.Language)
	m.theme.Reconfigure(view.Theme, view.SimpleMode)
}

func (m *AppModel) pruneToasts() {
	now := time.Now()
	kept := m.toasts[:0]
	for _, t := range m.toasts {
		if t.expires.After(now) {
			kept = append(kept, t)
		}
	}
	m.toasts = kept
}

// currentRefreshStates triggers a download-state refresh when the active view
// exposes one (Download/Manage).
func (m *AppModel) currentRefreshStates() tea.Cmd {
	if refresher, ok := m.current().(interface {
		RefreshStates(app *AppModel) tea.Cmd
	}); ok {
		return refresher.RefreshStates(m)
	}
	return nil
}

// handleGlobalKey handles the shell-level shortcuts. handled reports whether the
// key was consumed by the shell.
func (m *AppModel) handleGlobalKey(key tea.KeyMsg) (tea.Cmd, bool) {
	switch key.String() {
	case "esc":
		return m.globalEscape(), true
	case "q":
		if m.queue.HasVisibleItems() {
			m.queueOpen = !m.queueOpen
		} else {
			m.queueOpen = false
		}
		return nil, true
	case "Q":
		if m.queue.ReminderOnly() {
			return cmdClearOptionalReminders(m.host), true
		}
		return nil, true
	case "t":
		// Open the instance / target switcher (SideBar + VersionChoose parity).
		m.pushModal(newInstanceTargetModal())
		return nil, true
	case "1", "2", "3", "4", "5", "6":
		idx := int(key.String()[0] - '1')
		if idx >= 0 && idx < viewCount {
			return m.switchView(ViewID(idx)), true
		}
	case "tab":
		return m.switchView(ViewID((int(m.active) + 1) % viewCount)), true
	case "shift+tab":
		return m.switchView(ViewID((int(m.active) + viewCount - 1) % viewCount)), true
	case "ctrl+c":
		m.quitting = true
		return tea.Quit, true
	}
	return nil, false
}

// globalEscape mirrors App.vue onGlobalEscape: modal -> input focus -> selection
// -> view back.
func (m *AppModel) globalEscape() tea.Cmd {
	if m.queueOpen {
		m.queueOpen = false
		return nil
	}
	if top := m.modals.top(); top != nil {
		if top.CapturesInput() {
			return top.Update(m, tea.KeyMsg{Type: tea.KeyEsc})
		}
		m.modals.pop()
		return nil
	}
	if clearer, ok := m.current().(interface {
		ClearFocus(app *AppModel) (bool, tea.Cmd)
	}); ok {
		if handled, cmd := clearer.ClearFocus(m); handled {
			return cmd
		}
	}
	if m.active != ViewHome {
		return m.switchView(ViewHome)
	}
	return nil
}

func (m *AppModel) switchView(target ViewID) tea.Cmd {
	if target == m.active {
		return nil
	}
	m.active = target
	m.queueOpen = false
	return m.current().Update(m, viewActivatedMsg{})
}

type viewActivatedMsg struct{}

// handleMouse implements the documented mouse interactions.
func (m *AppModel) handleMouse(mouse tea.MouseMsg) tea.Cmd {
	// Wheel routing to the active list view.
	switch mouse.Action {
	case tea.MouseActionPress:
		switch mouse.Button {
		case tea.MouseButtonWheelUp:
			if lv, ok := m.current().(listView); ok {
				return lv.MouseWheel(m, -1)
			}
		case tea.MouseButtonWheelDown:
			if lv, ok := m.current().(listView); ok {
				return lv.MouseWheel(m, 1)
			}
		}
	}

	// Queue capsule region: right side of the bottom bar.
	if m.hitQueueCapsule(mouse.X, mouse.Y) {
		if mouse.Action == tea.MouseActionPress {
			switch mouse.Button {
			case tea.MouseButtonRight:
				if m.queue.ReminderOnly() {
					return cmdClearOptionalReminders(m.host)
				}
			case tea.MouseButtonLeft:
				if m.queue.HasVisibleItems() {
					m.queueOpen = !m.queueOpen
				}
			}
		}
		return nil
	}

	// Top tab click (row 0).
	if mouse.Action == tea.MouseActionPress && mouse.Button == tea.MouseButtonLeft && mouse.Y == 0 {
		if id, ok := m.hitTab(mouse.X); ok {
			return m.switchView(id)
		}
	}

	// Second top-bar row: clicking the instance/target area opens the switcher
	// modal; clicking the Connector button toggles the virtual loader.
	if mouse.Action == tea.MouseActionPress && mouse.Button == tea.MouseButtonLeft && mouse.Y == 1 {
		switch m.hitTopBarRegion(mouse.X, 1) {
		case "instance":
			m.pushModal(newInstanceTargetModal())
			return nil
		case "connector":
			return cmdToggleConnector(m.host)
		}
	}

	// Row click inside the main viewport (rows start at y=2).
	if mouse.Action == tea.MouseActionPress && mouse.Button == tea.MouseButtonLeft && mouse.Y >= 2 && mouse.Y < m.mainHeight()+2 {
		if lv, ok := m.current().(listView); ok {
			idx := m.rowFromY(lv, mouse.Y-2)
			if idx >= 0 {
				return lv.ClickRow(m, idx, mouse)
			}
		}
	}
	return nil
}

// mainHeight is the height available to the active view: total height minus the
// two top-bar rows and the single bottom-bar row.
func (m *AppModel) mainHeight() int {
	h := m.height - 3
	if h < 3 {
		h = 3
	}
	return h
}

// rowFromY maps a viewport-local Y to a list row index, accounting for the
// view's scroll offset and header lines. Views may override via listGeometry.
func (m *AppModel) rowFromY(lv listView, localY int) int {
	if geo, ok := lv.(interface {
		ListGeometry(app *AppModel) (headerLines, offset, rows int)
	}); ok {
		header, offset, rows := geo.ListGeometry(m)
		idx := localY - header + offset
		if idx < 0 || idx >= rows {
			return -1
		}
		return idx
	}
	return -1
}

// hitTab maps an X coordinate on the top tab row to a view id.
func (m *AppModel) hitTab(x int) (ViewID, bool) {
	pos := displayWidth(m.appTitle()) + 2
	for i := 0; i < viewCount; i++ {
		label := m.tabLabel(ViewID(i))
		width := displayWidth(label) + 2
		if x >= pos && x < pos+width {
			return ViewID(i), true
		}
		pos += width
	}
	return 0, false
}

// hitTopBarRegion resolves a click on a rendered top-bar row-2 segment.
func (m *AppModel) hitTopBarRegion(x, row int) string {
	for _, region := range m.topBarRegions {
		if region.row == row && x >= region.x0 && x < region.x1 {
			return region.name
		}
	}
	return ""
}

func (m *AppModel) hitQueueCapsule(x, y int) bool {
	if y != m.height-1 {
		return false
	}
	capsule := m.queueCapsule()
	start := m.width - displayWidth(capsule) - 1
	return x >= start && x < m.width
}

// --- view ---------------------------------------------------------------------

func (m *AppModel) View() string {
	if m.width <= 0 || m.height <= 0 {
		m.width, m.height = defaultWidth, defaultHeight
	}
	top := m.renderTopBar()
	main := m.renderMain()
	bottom := m.renderBottomBar()

	content := top + "\n" + main + "\n" + bottom

	if top := m.modals.top(); top != nil {
		overlay := centerOverlay(m, top, top.View(m))
		content = overlayCentered(content, overlay, m.width, m.height)
	}
	if m.queueOpen {
		content = overlayCentered(content, m.renderQueueDrawer(), m.width, m.height)
	}
	if toast := m.renderToasts(); toast != "" {
		content = overlayBottomRight(content, toast, m.width, m.height)
	}
	return content
}

func (m *AppModel) renderMain() string {
	body := m.current().View(m)
	return fitHeight(body, m.mainHeight(), m.width)
}

func (m *AppModel) appTitle() string {
	return "mod-downloader"
}

func (m *AppModel) tabLabel(id ViewID) string {
	key := []string{"nav.home", "nav.download", "nav.manage", "nav.favorites", "nav.unpin", "nav.settings"}[id]
	return "[" + itoa(int(id)+1) + "] " + i18n.T(key)
}

func (m *AppModel) renderTopBar() string {
	t := m.theme
	// Row 1: title + tabs.
	line1 := t.Title().Render(m.appTitle())
	active := int(m.active)
	for i := 0; i < viewCount; i++ {
		label := m.tabLabel(ViewID(i))
		if i == active {
			line1 += "  " + t.Selected().Render(" "+label+" ")
		} else {
			line1 += "  " + t.Muted().Render(label)
		}
	}

	// Row 2: instance + target tuple + connector. The rendered segments are also
	// recorded as hit regions so clicks open the instance/target modal or toggle
	// the Connector loader.
	m.topBarRegions = m.topBarRegions[:0]
	dir := m.mc.MinecraftDir
	if dir == "" {
		dir = i18n.T("home.status.notConfigured")
	}
	instance := m.mc.SelectedInstanceLabel()
	if instance == "" {
		instance = i18n.T("home.status.none")
	}

	line2 := ""
	writeSegment := func(text, regionName string) {
		start := displayWidth(line2)
		line2 += text
		if regionName != "" {
			m.topBarRegions = append(m.topBarRegions, hitRegion{name: regionName, row: 1, x0: start, x1: displayWidth(line2)})
		}
	}
	writeSegment(t.Muted().Render(uiText("minecraftDir")+": ")+truncate(dir, maxInt(12, m.width/3)), "instance")
	writeSegment("  "+t.Muted().Render(uiText("target")+": ")+t.OnSurface().Render(m.targetTuple()), "")
	writeSegment("  "+t.Muted().Render(uiText("instance")+": ")+t.OnSurface().Render(truncate(instance, maxInt(10, m.width/4))), "instance")
	if m.selectedConnectorAvailable() {
		writeSegment("  "+t.Accent().Render(m.connectorLabel()), "connector")
	}
	if m.serverNote != "" {
		writeSegment("  "+t.Warning().Render("⚠ "+truncate(m.serverNote, maxInt(10, m.width/3))), "")
	}

	line1 = padRight(line1, m.width)
	line2 = padRight(line2, m.width)
	return t.Bar().Render(truncate(line1, m.width)) + "\n" + t.Bar().Render(truncate(line2, m.width))
}

func (m *AppModel) targetTuple() string {
	mc := m.mc.SelectedMinecraftVersion
	if mc == "" {
		mc = "—"
	}
	loader := m.mc.SelectedModLoader
	if loader == "" {
		loader = "—"
	}
	return mc + " | " + loader
}

func (m *AppModel) selectedConnectorAvailable() bool {
	return m.mc.SelectedVersion != nil && m.mc.SelectedVersion.ConnectorAvailable
}

func (m *AppModel) connectorLabel() string {
	v := m.mc.SelectedVersion
	if v == nil {
		return ""
	}
	target := v.ActualModLoader
	if v.ConnectorVirtual {
		// Show the loader we would switch back to.
		return i18n.T("connector.switchTo", map[string]string{"loader": target})
	}
	return i18n.T("connector.switchTo", map[string]string{"loader": "Fabric"})
}

func (m *AppModel) renderBottomBar() string {
	t := m.theme
	left := ""
	if hints, ok := m.current().(interface {
		FooterHints(app *AppModel) string
	}); ok {
		left = hints.FooterHints(m)
	}
	if left == "" {
		left = uiText("keys")
	}
	capsule := m.queueCapsule()

	line := left
	pad := m.width - displayWidth(left) - displayWidth(capsule)
	if pad < 1 {
		left = truncate(left, maxInt(0, m.width-displayWidth(capsule)-1))
		pad = m.width - displayWidth(left) - displayWidth(capsule)
		if pad < 1 {
			pad = 1
		}
	}
	line = left + strings.Repeat(" ", pad) + t.Accent().Render(capsule)
	line = padRight(line, m.width)
	return t.Bar().Render(truncate(line, m.width))
}

// queueCapsule mirrors the GUI FAB label.
func (m *AppModel) queueCapsule() string {
	q := m.queue
	if !q.Queue.Active && !q.HasVisibleItems() {
		return "[↓ 0/0]"
	}
	progress := ""
	if total := q.Queue.TotalBytes; total > 0 {
		progress = " " + formatPercent(q.Queue.BytesComplete, total)
	}
	speed := ""
	if q.Queue.BytesPerSecond > 0 {
		speed = " " + formatSpeed(q.Queue.BytesPerSecond)
	}
	reminders := ""
	if q.MessageCount() > 0 {
		reminders = " 🔔" + itoa(q.MessageCount())
	}
	return "[↓ " + itoa(q.Queue.Running) + "/" + itoa(q.Queue.Pending) + progress + speed + reminders + "]"
}

func (m *AppModel) renderToasts() string {
	if len(m.toasts) == 0 {
		return ""
	}
	t := m.theme
	lines := make([]string, 0, len(m.toasts))
	for _, item := range m.toasts {
		style := t.Success()
		switch item.color {
		case toastWarning:
			style = t.Warning()
		case toastError:
			style = t.Danger()
		case toastInfo:
			style = t.Accent()
		}
		lines = append(lines, style.Render(truncate(item.text, maxInt(20, m.width/3))))
	}
	return strings.Join(lines, "\n")
}

// Quitting reports whether the program should exit.
func (m *AppModel) Quitting() bool { return m.quitting }

// ringBell writes the BEL control character to stdout, replacing the GUI's
// Web Audio completion sound. It is emitted once per drained download cycle.
func ringBell() tea.Cmd {
	return func() tea.Msg {
		_, _ = fmt.Fprint(os.Stdout, "\a")
		return nil
	}
}
