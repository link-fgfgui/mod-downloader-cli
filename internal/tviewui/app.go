package tviewui

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"time"

	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"

	"github.com/link-fgfgui/mod-downloader-core/logging"
	"github.com/link-fgfgui/mod-downloader-core/storage"
	appstructs "github.com/link-fgfgui/mod-downloader-core/structs"

	"github.com/link-fgfgui/mod-downloader-cli/internal/i18n"
	"github.com/link-fgfgui/mod-downloader-cli/internal/tuihost"
	"github.com/link-fgfgui/mod-downloader-cli/internal/uistate"
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

var viewPageNames = []string{"home", "download", "manage", "favorites", "unpin", "settings"}

var navKeys = []string{"nav.home", "nav.download", "nav.manage", "nav.favorites", "nav.unpin", "nav.settings"}

var navIcons = []string{"⌂", "↓", "☰", "★", "⚑", "⚙"}

// page is one primary view. Implementations own their primitives and read
// shared state from the App; they never call the core directly.
type page interface {
	Primitive() tview.Primitive
	Refresh()
	Focus()
	HandleKey(event *tcell.EventKey) bool
	Hints() []hint
}

// pageActivator is optionally implemented by pages that load data on activation.
type pageActivator interface {
	OnActivate()
}

// pageResizer is optionally implemented by pages that rebuild content on resize.
type pageResizer interface {
	OnResize(width int)
}

// pageRestyler is optionally implemented by pages that reapply widget colors
// after a theme change (text content is rebuilt by Refresh).
type pageRestyler interface {
	Restyle()
}

// pageClearer is optionally implemented by pages that own a transient focus
// (input field, multi-selection) which Esc should clear first.
type pageClearer interface {
	ClearFocus() bool
}

// Options configures a tview TUI run.
type Options struct {
	// Version is the normalized app version (ldflags-injected, "dev" fallback).
	Version string
	// LoadMinecraftReleases mirrors the GUI (true).
	LoadMinecraftReleases bool
	// LogLevel defaults to slog.LevelInfo (the zero value).
	LogLevel slog.Level
}

// prepareLogging gives the core logger a file-only sink before any startup
// work: the TUI owns the terminal, so stderr logging would corrupt it.
func prepareLogging(level slog.Level) {
	if level == 0 {
		level = slog.LevelInfo
	}
	if err := logging.Configure(logging.Options{
		Level:    level,
		NoStderr: true,
		FilePath: logging.DefaultPath(),
	}); err != nil {
		_ = logging.Configure(logging.Options{Disabled: true, NoStderr: true})
	}
}

// Run starts the tview TUI and blocks until the user quits, then performs the
// ordered shutdown (HTTP server stop -> core shutdown).
func Run(ctx context.Context, opts Options) error {
	return run(ctx, opts, nil)
}

// run is Run with an optional pre-initialized screen. Tests pass a simulation
// screen so the real event loop and shutdown path stay deterministic.
func run(ctx context.Context, opts Options, screen tcell.Screen) error {
	return runWith(ctx, opts, screen, nil)
}

// runWith additionally exposes the app once the host is up, so tests can wait
// for the initial refreshes before quitting.
func runWith(ctx context.Context, opts Options, screen tcell.Screen, started func(*App)) error {
	level := opts.LogLevel
	if level == 0 {
		level = slog.LevelInfo
	}
	prepareLogging(level)

	host := tuihost.New(tuihost.Options{
		Version:               opts.Version,
		LoadMinecraftReleases: opts.LoadMinecraftReleases,
		LogLevel:              level,
		LogToFileOnly:         true,
	})
	a := New(host)
	if screen != nil {
		a.application.SetScreen(screen)
	}

	host.SetEventSink(a.enqueueEvent)

	if err := host.Start(ctx); err != nil {
		host.Shutdown()
		return err
	}
	// The core now exists; controller methods may call it.
	a.offline = false
	// Start the pump first: the initial refreshes enqueue UI tasks immediately
	// and must not wake an uninitialized signal channel.
	a.startEventPump()
	a.afterStart()
	if started != nil {
		started(a)
	}

	runErr := a.application.Run()
	host.SetEventSink(nil)
	a.stopSpinner()
	a.stopEventPump()
	a.waitEventPump()
	if a.toastTimer != nil {
		a.toastTimer.Stop()
	}
	host.Shutdown()
	return runErr
}

// App is the single tview application shell.
type App struct {
	application *tview.Application
	host        *tuihost.Host
	theme       Theme

	width  int
	height int
	bp     Breakpoint

	outer *tview.Pages
	shell *shellLayout

	sidebar     *wheelList
	sidebarCol  *tview.Flex
	sidebarFoot *tview.TextView

	content   *tview.Flex
	main      *tview.Pages
	statusBar *statusBar
	queue     *queueDock

	active ViewID
	views  []page

	// Shared, adapter-neutral view state. The tview widgets are projections of
	// these values; business state is never stored twice.
	mc         uistate.MinecraftState
	search     uistate.SearchState
	queueState uistate.QueueState
	settings   appstructs.SettingsView
	stats      storage.UsageStats

	dialogs []*dialog

	targetDialog      *targetDialog
	versionDialog     *versionDialog
	addFavoriteDialog *addFavoriteDialog
	refsDialog        *favoriteRefsDialog
	impactDialog      *disableImpactDialog

	toastText  string
	toastSeq   int
	toastTimer *time.Timer
	dialogSeq  int

	// Host events are buffered and applied in batches so a burst of download
	// progress updates costs one redraw instead of one per event.
	eventMu       sync.Mutex
	pendingEvents []tuihost.Event
	eventSignal   chan struct{}
	eventDone     chan struct{}

	// Download-state lookups are expensive (one install check per result), so
	// only one may run at a time; bursts collapse into a trailing refresh.
	downloadStatesMu       sync.Mutex
	downloadStatesInFlight bool
	downloadStatesDirty    bool

	// Long-running background work (instance switches, refreshes) drives a
	// spinner in the status bar so the UI never looks frozen.
	busyMu    sync.Mutex
	busyCount int
	busyLabel string
	busyFrame int
	busyStop  chan struct{}

	queueAutoOpened bool
	// offline is set when New is built without a started core (SimulationScreen
	// tests); controller methods become no-ops so rendering stays deterministic.
	offline bool

	stopOnce sync.Once
	// eventExited closes when the pump goroutine returns, so stop() can avoid
	// stopping the event loop while the pump is blocked on QueueUpdate.
	eventExited chan struct{}

	startupWG   sync.WaitGroup
	startupDone chan struct{}
}

// New builds the shell around an unstarted host.
func New(host *tuihost.Host) *App {
	prefs := appstructs.AppPreferences{}
	if host.Core() != nil {
		prefs = host.Service().GetPreferences()
	}
	i18n.Apply(prefs.Language)

	a := &App{
		application: tview.NewApplication(),
		host:        host,
		theme:       resolveTheme(prefs.Theme),
		mc:          uistate.NewMinecraftState(),
		search:      uistate.SearchState{SelectedModLoader: uistate.DefaultModLoader},
		queueState:  uistate.NewQueueState(),
		startupDone: make(chan struct{}),
		// Leave the size unset so the first Draw always runs the responsive
		// layout pass, even when the terminal happens to be 80x24.
		offline: host.Core() == nil,
	}
	a.theme.apply()
	a.application.EnableMouse(true).EnablePaste(true)

	a.buildSidebar()
	a.main = tview.NewPages()

	a.views = []page{
		newHomePage(a),
		newDownloadPage(a),
		newManagePage(a),
		newFavoritesPage(a),
		newUnpinPage(a),
		newSettingsPage(a),
	}
	for i, p := range a.views {
		a.main.AddPage(viewPageNames[i], p.Primitive(), true, i == 0)
	}
	a.main.SwitchToPage(viewPageNames[ViewHome])

	a.queue = newQueueDock(a)
	a.statusBar = newStatusBar(a)

	a.content = tview.NewFlex().SetDirection(tview.FlexColumn).
		AddItem(a.sidebarCol, sidebarWidth(a.width), 0, true).
		AddItem(a.main, 0, 1, false).
		AddItem(a.queue.Primitive(), 0, 0, false)

	rows := tview.NewFlex().SetDirection(tview.FlexRow).
		AddItem(a.content, 0, 1, true).
		AddItem(a.statusBar.Primitive(), 1, 0, false)

	a.shell = &shellLayout{app: a, flex: rows}

	a.outer = tview.NewPages()
	a.outer.AddPage("shell", a.shell, true, true)

	a.application.SetRoot(a.outer, true)
	a.application.SetInputCapture(a.handleGlobalKey)
	a.application.SetMouseCapture(a.handleGlobalMouse)
	a.application.SetBeforeDrawFunc(func(screen tcell.Screen) bool {
		a.beforeDraw(screen)
		return false
	})

	a.active = ViewHome
	a.restyleContainers()
	a.rebuildSidebarLabels()
	a.views[a.active].Focus()
	a.updateStatus()
	return a
}

// spinnerFrames animate the status bar while background work is in flight.
var spinnerFrames = []string{"⠋", "⠙", "⠹", "⠸", "⠼", "⠴", "⠦", "⠧", "⠇", "⠏"}

// beginBusy marks a background operation as running and starts the spinner.
func (a *App) beginBusy(label string) {
	a.busyMu.Lock()
	a.busyCount++
	if label != "" {
		a.busyLabel = label
	}
	first := a.busyCount == 1
	a.busyMu.Unlock()
	if first {
		a.startSpinner()
	}
	a.uiUpdate(a.refreshBusyViews)
}

// refreshBusyViews repaints the widgets that show background work.
func (a *App) refreshBusyViews() {
	a.updateStatus()
	if a.targetDialog != nil {
		a.targetDialog.refresh()
	}
}

// endBusy clears one running operation and stops the spinner when idle.
func (a *App) endBusy() {
	a.busyMu.Lock()
	if a.busyCount > 0 {
		a.busyCount--
	}
	idle := a.busyCount == 0
	if idle {
		a.busyLabel = ""
	}
	a.busyMu.Unlock()
	if idle {
		a.stopSpinner()
	}
	a.uiUpdate(a.refreshBusyViews)
}

// startSpinner repaints the status bar while work is running.
func (a *App) startSpinner() {
	a.busyMu.Lock()
	if a.busyStop != nil {
		a.busyMu.Unlock()
		return
	}
	stop := make(chan struct{})
	a.busyStop = stop
	a.busyMu.Unlock()
	go func() {
		ticker := time.NewTicker(120 * time.Millisecond)
		defer ticker.Stop()
		for {
			select {
			case <-stop:
				return
			case <-ticker.C:
				a.uiUpdate(func() {
					a.busyMu.Lock()
					a.busyFrame++
					a.busyMu.Unlock()
					a.refreshBusyViews()
				})
			}
		}
	}()
}

// stopSpinner stops the animation and repaints the bar without it.
func (a *App) stopSpinner() {
	a.busyMu.Lock()
	if a.busyStop != nil {
		close(a.busyStop)
		a.busyStop = nil
	}
	a.busyMu.Unlock()
}

// busyStatus renders the spinner segment, or "" when nothing is running.
func (a *App) busyStatus() string {
	a.busyMu.Lock()
	defer a.busyMu.Unlock()
	if a.busyCount == 0 {
		return ""
	}
	label := a.busyLabel
	if label == "" {
		label = uiText("loading")
	}
	return spinnerFrames[a.busyFrame%len(spinnerFrames)] + " " + label
}

// eventUITask carries a UI update requested by a background goroutine. Routing
// it through the event pump keeps such goroutines from blocking on
// QueueUpdateDraw once the application has stopped.
const eventUITask tuihost.EventKind = "ui-task"

// uiUpdate runs f on the UI goroutine. It never blocks: a stopped application
// simply drops the task instead of leaking the calling goroutine.
func (a *App) uiUpdate(f func()) {
	a.enqueueEvent(tuihost.Event{Kind: eventUITask, Payload: f})
}

// enqueueEvent records a host event without blocking the emitting worker. Core
// goroutines call this from download workers, so it must never wait on the UI.
func (a *App) enqueueEvent(event tuihost.Event) {
	a.eventMu.Lock()
	a.pendingEvents = append(a.pendingEvents, event)
	a.eventMu.Unlock()
	select {
	case a.eventSignal <- struct{}{}:
	default:
	}
}

// startEventPump runs the goroutine that applies buffered events on the UI
// goroutine.
func (a *App) startEventPump() {
	signal := make(chan struct{}, 1)
	done := make(chan struct{})
	exited := make(chan struct{})
	a.eventSignal = signal
	a.eventDone = done
	a.eventExited = exited
	go func() {
		defer close(exited)
		for {
			select {
			case <-done:
				return
			case <-signal:
			}
			a.flushEvents()
		}
	}()
}

// stopEventPump asks the pump to exit. A pump already inside QueueUpdate keeps
// running until its batch is applied; waitEventPump covers that case.
func (a *App) stopEventPump() {
	if a.eventDone == nil {
		return
	}
	close(a.eventDone)
	a.eventDone = nil
}

// waitEventPump waits for the pump goroutine to return.
func (a *App) waitEventPump() {
	if a.eventExited == nil {
		return
	}
	<-a.eventExited
}

// stop shuts the UI down without stranding the pump: the pump finishes its
// current batch while the event loop is still running, then the loop stops.
func (a *App) stop() {
	a.stopOnce.Do(func() {
		a.stopEventPump()
		exited := a.eventExited
		go func() {
			if exited != nil {
				<-exited
			}
			a.application.Stop()
		}()
	})
}

// flushEvents applies every buffered event in one UI update and draws once.
func (a *App) flushEvents() {
	a.eventMu.Lock()
	batch := a.pendingEvents
	a.pendingEvents = nil
	a.eventMu.Unlock()
	if len(batch) == 0 {
		return
	}
	batch = compactEvents(batch)
	a.application.QueueUpdate(func() {
		for _, event := range batch {
			if event.Kind == eventUITask {
				if task, ok := event.Payload.(func()); ok {
					task()
				}
				continue
			}
			a.handleEvent(event)
		}
		// ForceDraw is the only draw call that is safe inside a queued update.
		a.application.ForceDraw()
	})
	// Events may have arrived while the batch was waiting for the UI loop.
	a.eventMu.Lock()
	more := len(a.pendingEvents) > 0
	a.eventMu.Unlock()
	if more {
		select {
		case a.eventSignal <- struct{}{}:
		default:
		}
	}
}

// compactEvents drops superseded queue snapshots: each queue event carries the
// full state, so only the newest one in a batch is applied.
func compactEvents(events []tuihost.Event) []tuihost.Event {
	last := -1
	for i, event := range events {
		if event.Kind == tuihost.EventDownloadQueueUpdated {
			last = i
		}
	}
	if last < 0 {
		return events
	}
	compacted := events[:0]
	for i, event := range events {
		if event.Kind == tuihost.EventDownloadQueueUpdated && i != last {
			continue
		}
		compacted = append(compacted, event)
	}
	return compacted
}

// afterStart loads the initial state and binds the first focus after the host
// has started. It runs before the event loop, so its async work is queued.
func (a *App) afterStart() {
	prefs := a.host.Service().GetPreferences()
	i18n.Apply(prefs.Language)
	a.theme = resolveTheme(prefs.Theme)
	a.theme.apply()
	a.rebuildAllText()

	a.startupWG.Add(4)
	go func() { defer a.startupWG.Done(); a.refreshMinecraft() }()
	go func() { defer a.startupWG.Done(); a.refreshQueueState() }()
	go func() { defer a.startupWG.Done(); a.refreshUsageStats() }()
	go func() { defer a.startupWG.Done(); a.refreshSettings() }()
	go func() {
		a.startupWG.Wait()
		close(a.startupDone)
	}()
	if notice := a.host.ServerNotice(); notice != "" {
		// afterStart runs on the main goroutine before the event loop starts;
		// queued updates are applied once it does.
		a.toast(notice)
	}
}

// shellLayout is the root primitive: it computes responsive widths from the
// actual screen size before delegating to the Flex tree.
type shellLayout struct {
	app  *App
	flex *tview.Flex
}

func (s *shellLayout) Draw(screen tcell.Screen) {
	w, h := screen.Size()
	s.app.beforeDrawSize(w, h)
	s.flex.SetRect(0, 0, w, h)
	s.flex.Draw(screen)
}

func (s *shellLayout) GetRect() (int, int, int, int) { return s.flex.GetRect() }
func (s *shellLayout) SetRect(x, y, w, h int)        { s.flex.SetRect(x, y, w, h) }
func (s *shellLayout) InputHandler() func(*tcell.EventKey, func(tview.Primitive)) {
	return s.flex.InputHandler()
}
func (s *shellLayout) Focus(delegate func(tview.Primitive)) { s.flex.Focus(delegate) }
func (s *shellLayout) HasFocus() bool                       { return s.flex.HasFocus() }
func (s *shellLayout) Blur()                                { s.flex.Blur() }
func (s *shellLayout) MouseHandler() func(tview.MouseAction, *tcell.EventMouse, func(tview.Primitive)) (bool, tview.Primitive) {
	return s.flex.MouseHandler()
}
func (s *shellLayout) PasteHandler() func(string, func(tview.Primitive)) {
	return s.flex.PasteHandler()
}

// Breakpoint selects the sidebar chrome for the current width.
type Breakpoint int

const (
	BPCompact Breakpoint = iota
	BPRail
	BPFull
)

func sidebarWidth(width int) int {
	switch {
	case width >= 120:
		return 30
	case width >= 50:
		return 7
	default:
		return 0
	}
}

func breakpointFor(width int) Breakpoint {
	switch {
	case width >= 120:
		return BPFull
	case width >= 50:
		return BPRail
	default:
		return BPCompact
	}
}

// beforeDrawSize computes responsive geometry and rebuilds width-dependent
// widgets. It is called from Draw (UI goroutine) before the tree is painted.
func (a *App) beforeDrawSize(w, h int) {
	if w == a.width && h == a.height {
		return
	}
	first := a.width == 0
	a.width, a.height = w, h
	bp := breakpointFor(w)
	if bp != a.bp || first {
		a.bp = bp
		a.rebuildSidebarLabels()
	}
	a.content.ResizeItem(a.sidebarCol, sidebarWidth(w), 0)
	a.resizeQueue()
	for _, p := range a.views {
		if resizer, ok := p.(pageResizer); ok {
			resizer.OnResize(a.mainWidth())
		}
	}
	a.updateStatus()
}

// beforeDraw is the Application before-draw hook (used for the initial frame).
func (a *App) beforeDraw(screen tcell.Screen) {
	w, h := screen.Size()
	a.beforeDrawSize(w, h)
}

// mainWidth is the width available to the active page (excluding the queue).
func (a *App) mainWidth() int {
	w := a.width - sidebarWidth(a.width)
	if a.queue != nil && a.queue.Visible() {
		w -= a.queue.Width()
	}
	if a.width > 0 && w < 1 {
		w = 1
	}
	return w
}

func (a *App) buildSidebar() {
	a.sidebar = newWheelList()
	a.sidebar.ShowSecondaryText(false)
	a.sidebar.SetWrapAround(true)
	a.sidebar.SetBorder(true)
	a.theme.styleList(a.sidebar.List)

	a.sidebarFoot = tview.NewTextView().SetDynamicColors(true)
	a.sidebarFoot.SetBackgroundColor(a.theme.Surface)

	a.sidebarCol = tview.NewFlex().SetDirection(tview.FlexRow).
		AddItem(a.sidebar, 0, 1, true).
		AddItem(a.sidebarFoot, 5, 0, false)
}

// rebuildSidebarLabels rewrites the nav list for the current breakpoint.
func (a *App) rebuildSidebarLabels() {
	full := a.bp == BPFull
	a.sidebar.Clear()
	for i := 0; i < viewCount; i++ {
		idx := i
		icon := navIcons[i]
		label := i18n.T(navKeys[i])
		text := icon
		if full {
			text = fmt.Sprintf("%d %s  %s", i+1, icon, label)
		}
		a.sidebar.AddItem(text, "", 0, func() { a.switchView(ViewID(idx)) })
	}
	if full {
		a.sidebar.SetTitle("Mod Downloader")
	} else {
		a.sidebar.SetTitle("MD")
	}
	a.sidebar.SetCurrentItem(int(a.active))
	a.updateSidebarFooter()
}

func (a *App) updateSidebarFooter() {
	if a.sidebarFoot == nil {
		return
	}
	t := a.theme
	if a.bp != BPFull {
		version := a.mc.SelectedMinecraftVersion
		if version == "" {
			version = "—"
		}
		text := t.tag(t.Primary) + version + resetTag
		if a.selectedConnectorAvailable() {
			text += "\n" + t.tag(t.Secondary) + "⇄" + resetTag
		}
		a.sidebarFoot.SetText(text)
		return
	}
	instance := a.mc.SelectedInstanceLabel()
	if instance == "" {
		instance = uiText("noInstance")
	}
	dir := a.mc.MinecraftDir
	if dir == "" {
		dir = i18n.T("home.status.notConfigured")
	}
	text := t.tag(t.Muted) + strings.ToUpper(uiText("instance")) + resetTag + "\n" +
		t.tag(t.OnSurface) + tview.Escape(instance) + resetTag + "\n" +
		t.tag(t.Secondary) + tview.Escape(a.targetTuple()) + resetTag + "\n" +
		t.tag(t.Muted) + tview.Escape(dir) + resetTag
	if a.selectedConnectorAvailable() {
		text += "\n" + t.tag(t.Secondary) + tview.Escape(a.connectorLabel()) + resetTag
	}
	a.sidebarFoot.SetText(text)
}

// switchView changes the active page, closes the queue dock and moves focus.
func (a *App) switchView(target ViewID) {
	if target < 0 || target >= ViewID(viewCount) {
		return
	}
	a.closeQueue()
	prev := a.active
	a.active = target
	a.main.SwitchToPage(viewPageNames[target])
	a.sidebar.SetCurrentItem(int(target))
	if prev != target {
		if activator, ok := a.views[target].(pageActivator); ok {
			activator.OnActivate()
		}
	}
	a.views[target].Focus()
	a.updateStatus()
}

// targetTuple renders the active [MC version | loader] tuple.
func (a *App) targetTuple() string {
	mc := a.mc.SelectedMinecraftVersion
	if mc == "" {
		mc = "—"
	}
	loader := a.mc.SelectedModLoader
	if loader == "" {
		loader = "—"
	}
	return mc + " | " + loader
}

func (a *App) selectedConnectorAvailable() bool {
	return a.mc.SelectedVersion != nil && a.mc.SelectedVersion.ConnectorAvailable
}

func (a *App) connectorLabel() string {
	v := a.mc.SelectedVersion
	if v == nil {
		return ""
	}
	target := v.ActualModLoader
	if v.ConnectorVirtual {
		return i18n.T("connector.switchTo", map[string]string{"loader": target})
	}
	return i18n.T("connector.switchTo", map[string]string{"loader": "Fabric"})
}

// applySettings stores the settings snapshot and re-applies language/theme.
func (a *App) applySettings(view appstructs.SettingsView) {
	a.settings = view
	i18n.Apply(view.Language)
	restyle := false
	if next := resolveTheme(view.Theme); next.Name != a.theme.Name {
		a.theme = next
		a.theme.apply()
		restyle = true
	}
	if restyle {
		// Existing widgets carry explicit colours, so a palette change has to
		// rebuild and restyle every page, not just the sidebar.
		a.rebuildAllText()
		return
	}
	a.rebuildSidebarLabels()
	a.queue.Refresh()
	a.updateStatus()
}

// toast shows a transient message in the status bar for four seconds.
func (a *App) toast(text string) {
	if strings.TrimSpace(text) == "" {
		return
	}
	a.toastText = text
	a.toastSeq++
	seq := a.toastSeq
	if a.toastTimer != nil {
		a.toastTimer.Stop()
	}
	a.updateStatus()
	a.toastTimer = time.AfterFunc(4*time.Second, func() {
		a.uiUpdate(func() {
			if a.toastSeq != seq {
				return
			}
			a.toastText = ""
			a.updateStatus()
		})
	})
}

func (a *App) toastT(key string, params ...map[string]string) {
	a.toast(i18n.T(key, params...))
}

// updateStatus refreshes the bottom bar hints and queue capsule.
// restyleContainers repaints the layout primitives behind the pages. tview
// Flexes do not fill their own background, so the Pages below them show
// through; without this a live theme switch leaves most of the screen in the
// old palette.
func (a *App) restyleContainers() {
	if a.main != nil {
		a.main.SetBackgroundColor(a.theme.Surface)
	}
	if a.outer != nil {
		a.outer.SetBackgroundColor(a.theme.Surface)
	}
	if a.content != nil {
		a.content.SetBackgroundColor(a.theme.Surface)
	}
	if a.sidebarCol != nil {
		a.sidebarCol.SetBackgroundColor(a.theme.Surface)
	}
}

func (a *App) updateStatus() {
	if a.statusBar == nil {
		return
	}
	var hints []hint
	if a.queue != nil && a.queue.Focused() {
		hints = a.queue.Hints()
	} else if a.active >= 0 && int(a.active) < len(a.views) {
		hints = a.views[a.active].Hints()
	}
	if a.toastText != "" {
		hints = []hint{hintText(a.toastText)}
	} else if busy := a.busyStatus(); busy != "" {
		hints = append([]hint{hintText(busy)}, hints...)
	}
	a.statusBar.SetQueueLabel(queueCapsule(a.queueState))
	a.statusBar.SetHints(hints)
}

// queueCapsule mirrors the GUI FAB label.
func queueCapsule(q uistate.QueueState) string {
	if !q.Queue.Active && !q.HasVisibleItems() {
		return "↓ 0/0"
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
	return "↓ " + itoa(q.Queue.Running) + "/" + itoa(q.Queue.Pending) + progress + speed + reminders
}

// handleGlobalKey is the Application input capture. It only consumes global
// and page-level shortcuts; unhandled keys fall through to the focused widget.
func (a *App) handleGlobalKey(event *tcell.EventKey) *tcell.EventKey {
	if event == nil {
		return nil
	}
	if event.Key() == tcell.KeyCtrlC {
		a.stop()
		return nil
	}

	if top := a.topDialog(); top != nil {
		if event.Key() == tcell.KeyEsc {
			a.popDialog()
			return nil
		}
		if top.handleKey != nil && top.handleKey(event) {
			return nil
		}
		return event
	}

	if event.Key() == tcell.KeyEsc {
		// An open drop-down consumes Esc itself, so it must not navigate away.
		if a.dropdownOpen() {
			return event
		}
		a.globalEscape()
		return nil
	}

	// Text entry owns every character except Esc/Ctrl+C. Tab still moves focus
	// between a search field and its list on pages that own a two-part focus.
	if a.textInputFocused() {
		if (event.Key() == tcell.KeyTab || event.Key() == tcell.KeyBacktab) &&
			a.active >= 0 && int(a.active) < len(a.views) && a.views[a.active].HandleKey(event) {
			return nil
		}
		return event
	}

	// The queue dock owns its navigation and action keys while focused.
	if a.queue != nil && a.queue.Focused() {
		if a.queue.HandleKey(event) {
			return nil
		}
		return event
	}

	switch event.Key() {
	case tcell.KeyRune:
		switch r := event.Rune(); {
		case r >= '1' && r <= '6':
			a.switchView(ViewID(r - '1'))
			return nil
		case r == 'q':
			a.toggleQueue()
			return nil
		case r == 'Q':
			if a.queueState.ReminderOnly() {
				go a.clearOptionalReminders()
			}
			return nil
		case r == 't':
			a.openTargetDialog()
			return nil
		case r == '?':
			a.openHelpDialog()
			return nil
		}
	case tcell.KeyTab, tcell.KeyBacktab:
		if a.active >= 0 && int(a.active) < len(a.views) {
			if a.views[a.active].HandleKey(event) {
				return nil
			}
		}
		return event
	}

	if a.active >= 0 && int(a.active) < len(a.views) && a.views[a.active].HandleKey(event) {
		return nil
	}
	return event
}

// handleGlobalMouse intercepts right-clicks on the queue capsule (clear
// reminders), blocks background clicks while a dialog is open, and passes
// everything else through.
func (a *App) handleGlobalMouse(event *tcell.EventMouse, action tview.MouseAction) (*tcell.EventMouse, tview.MouseAction) {
	if top := a.topDialog(); top != nil {
		if action == tview.MouseMove {
			return event, action
		}
		if centered, ok := top.page.(*centered); ok {
			x, y, w, h := centered.GetRect()
			px, py := event.Position()
			if px >= x && px < x+w && py >= y && py < y+h {
				return event, action
			}
		}
		// The page behind the dialog must not react to clicks.
		return nil, 0
	}
	if action == tview.MouseRightClick && a.statusBar != nil && a.statusBar.QueueButton().InRect(event.Position()) {
		if a.queueState.ReminderOnly() {
			go a.clearOptionalReminders()
		}
		return nil, 0
	}
	// tview reports a second click within 500 ms as a double-click, which no
	// widget handles. Nothing in the TUI uses double-clicks, so treat one as a
	// normal click; otherwise fast clicks (e.g. switching two pages in a row)
	// silently do nothing.
	if action == tview.MouseLeftDoubleClick {
		action = tview.MouseLeftClick
	}
	return event, action
}

// textInputFocused reports whether the focused widget consumes character keys.
func (a *App) textInputFocused() bool {
	focus := a.application.GetFocus()
	switch typed := focus.(type) {
	case *tview.InputField, *tview.TextArea:
		return true
	case *tview.DropDown:
		// A closed DropDown only treats runes as a prefix search that opens the
		// list; global shortcuts should win there, but an open list owns input.
		return typed.IsOpen()
	case *tview.Form:
		item, _ := typed.GetFocusedItemIndex()
		if item >= 0 {
			switch field := typed.GetFormItem(item).(type) {
			case *tview.InputField, *tview.TextArea:
				return true
			case *tview.DropDown:
				return field.IsOpen()
			}
		}
	}
	return false
}

// dropdownOpen reports whether a DropDown is currently expanded.
func (a *App) dropdownOpen() bool {
	focus := a.application.GetFocus()
	switch typed := focus.(type) {
	case *tview.DropDown:
		return typed.IsOpen()
	case *tview.Form:
		item, _ := typed.GetFocusedItemIndex()
		if item >= 0 {
			if drop, ok := typed.GetFormItem(item).(*tview.DropDown); ok {
				return drop.IsOpen()
			}
		}
	}
	return false
}

// globalEscape mirrors App.vue onGlobalEscape: dialog -> queue -> page focus ->
// home.
func (a *App) globalEscape() {
	if a.queue != nil && a.queue.Visible() {
		a.closeQueue()
		return
	}
	if a.active >= 0 && int(a.active) < len(a.views) {
		if clearer, ok := a.views[a.active].(pageClearer); ok && clearer.ClearFocus() {
			return
		}
	}
	if a.active != ViewHome {
		a.switchView(ViewHome)
	}
}

// toggleQueue opens or closes the right-side dock.
func (a *App) toggleQueue() {
	if a.queue.Visible() {
		a.closeQueue()
		return
	}
	if !a.queueState.HasVisibleItems() {
		a.queueAutoOpened = false
		a.updateStatus()
		return
	}
	a.queue.SetOpen(true)
	a.queueAutoOpened = false
	a.resizeQueue()
	a.queue.Focus()
	a.updateStatus()
}

// closeQueue hides the dock and restores page focus only when the dock owned it.
func (a *App) closeQueue() {
	if a.queue == nil || !a.queue.Visible() {
		return
	}
	focused := a.queue.Focused()
	a.queue.SetOpen(false)
	a.queueAutoOpened = false
	a.resizeQueue()
	if focused && a.topDialog() == nil && a.active >= 0 && int(a.active) < len(a.views) {
		a.views[a.active].Focus()
	}
	a.updateStatus()
}

// resizeQueue applies the dock width (0 when hidden) and recomputes the main area.
func (a *App) resizeQueue() {
	if a.queue == nil {
		return
	}
	width := 0
	if a.queue.Visible() {
		width = a.queue.Width()
	}
	a.content.ResizeItem(a.queue.Primitive(), width, 0)
	for _, p := range a.views {
		if resizer, ok := p.(pageResizer); ok {
			resizer.OnResize(a.mainWidth())
		}
	}
	if a.queue != nil {
		a.queue.bar.Reflow()
	}
}

// Quit stops the application (used by tests and Ctrl+C).
func (a *App) Quit() { a.stop() }

// favoritesPage returns the favorites view (the slice is built by New).
func (a *App) favoritesPage() *favoritesPage {
	return a.views[ViewFavorites].(*favoritesPage)
}

// managePage returns the manage view.
func (a *App) managePage() *managePage {
	return a.views[ViewManage].(*managePage)
}

// settingsPage returns the settings view.
func (a *App) settingsPage() *settingsPage {
	return a.views[ViewSettings].(*settingsPage)
}

// downloadPage returns the download view.
func (a *App) downloadPage() *downloadPage {
	return a.views[ViewDownload].(*downloadPage)
}

// homePage returns the home view.
func (a *App) homePage() *homePage {
	return a.views[ViewHome].(*homePage)
}
