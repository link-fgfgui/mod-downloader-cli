// Package tuihost owns the UI-independent host layer: the core service, the
// HTTP server and the lifecycle around them. Terminal frontends only register
// an event sink; they do not depend on each other's UI toolkit.
package tuihost

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"strings"
	"sync"

	"github.com/link-fgfgui/mod-downloader-core/appcore"
	"github.com/link-fgfgui/mod-downloader-core/configs"
	"github.com/link-fgfgui/mod-downloader-core/httpserver"
	"github.com/link-fgfgui/mod-downloader-core/logging"
	appstructs "github.com/link-fgfgui/mod-downloader-core/structs"
	structs "github.com/link-fgfgui/mod-downloader-core/structs/minecraft"
)

// EventKind identifies a core or HTTP-server event forwarded to the sink.
type EventKind string

const (
	EventSearchModsUpdated      EventKind = "search-mods-updated"
	EventDownloadStatesUpdated  EventKind = "download-states-updated"
	EventDownloadQueueUpdated   EventKind = "download-queue-updated"
	EventDownloadFailed         EventKind = "download-failed"
	EventDownloadCompleted      EventKind = "download-completed"
	EventMinecraftDirChanged    EventKind = "minecraft-dir-changed"
	EventSelectedVersionChanged EventKind = "selected-version-changed"
	EventUsageStatsUpdated      EventKind = "usage-stats-updated"
	EventExtensionModsAccepted  EventKind = "extension-mods-accepted"
)

// Event is a UI-neutral adapter event. Payload keeps the core type:
//   - search updates: appstructs.SearchModsUpdate
//   - queue: appstructs.DownloadQueueState
//   - download failure: appstructs.DownloadFailedEvent
//   - dir change: string
//   - selected version: structs.VersionInfo
//   - extension results: appstructs.SearchModsUpdate
type Event struct {
	Kind    EventKind
	Payload any
}

// EventSink receives every adapter event. It may be called from core worker
// goroutines, so implementations must marshal onto their own UI loop.
type EventSink func(Event)

// Options configures the host lifecycle.
type Options struct {
	// Version is the normalized app version (empty falls back to "dev").
	Version string
	// LoadMinecraftReleases mirrors the GUI (true).
	LoadMinecraftReleases bool
	// Config optionally supplies a pre-loaded config.
	Config *configs.Config
	// LogLevel is the minimum level emitted by the core logger (zero = info).
	LogLevel slog.Level
	// LogToFileOnly keeps logs out of stderr; terminal UIs own the terminal.
	LogToFileOnly bool
}

// Host owns the core service and the HTTP server.
type Host struct {
	version               string
	loadMinecraftReleases bool
	config                *configs.Config
	logLevel              slog.Level
	logToFileOnly         bool

	core   *appcore.Service
	server *httpserver.Server
	cfg    *configs.Config

	mu           sync.Mutex
	sink         EventSink
	serverNotice string
}

// New builds the host without starting it.
func New(opts Options) *Host {
	return &Host{
		version:               normalizeVersion(opts.Version),
		loadMinecraftReleases: opts.LoadMinecraftReleases,
		config:                opts.Config,
		logLevel:              opts.LogLevel,
		logToFileOnly:         opts.LogToFileOnly,
	}
}

// normalizeVersion mirrors currentAppVersion(): trim and fall back to "dev".
func normalizeVersion(version string) string {
	if v := strings.TrimSpace(version); v != "" {
		return v
	}
	return "dev"
}

// SetEventSink registers the adapter event sink. Events emitted before
// registration are dropped.
func (h *Host) SetEventSink(sink EventSink) {
	h.mu.Lock()
	h.sink = sink
	h.mu.Unlock()
}

// emit forwards an event to the registered sink.
func (h *Host) emit(kind EventKind, payload any) {
	h.mu.Lock()
	sink := h.sink
	h.mu.Unlock()
	if sink != nil {
		sink(Event{Kind: kind, Payload: payload})
	}
}

// Start creates and starts the core service, then starts the HTTP server. A
// listen failure is logged and surfaced as a non-fatal notice.
func (h *Host) Start(ctx context.Context) error {
	core := appcore.New(appcore.Options{
		Config:                h.config,
		Runtime:               appRuntimeOptions(),
		Version:               h.version,
		LoadMinecraftReleases: h.loadMinecraftReleases,
		OnEvent:               h.emitCoreEvent,
		LogLevel:              h.logLevel,
		LogToFileOnly:         h.logToFileOnly,
	})
	h.mu.Lock()
	h.core = core
	h.mu.Unlock()

	if err := core.Startup(ctx); err != nil {
		h.mu.Lock()
		h.cfg = core.Config()
		h.mu.Unlock()
		return err
	}
	h.mu.Lock()
	h.cfg = core.Config()
	h.mu.Unlock()

	server := httpserver.New(httpserver.DefaultAddr, httpserver.Options{OnEvent: h.emitHTTPServerEvent})
	h.mu.Lock()
	h.server = server
	h.mu.Unlock()
	if err := server.Start(); err != nil {
		logging.Error("start http server failed", "error", err)
		h.mu.Lock()
		h.serverNotice = fmt.Sprintf("http server unavailable on %s: %v", httpserver.DefaultAddr, err)
		h.mu.Unlock()
	}
	return nil
}

// appRuntimeOptions mirrors the GUI's default cache dir resolution.
func appRuntimeOptions() appcore.RuntimeOptions {
	dir, err := os.Getwd()
	if err != nil {
		logging.Error("resolve tui default cache dir failed", "error", err)
		return appcore.RuntimeOptions{}
	}
	return appcore.RuntimeOptions{DefaultCacheDir: dir}
}

// Core returns the shared core service.
func (h *Host) Core() *appcore.Service {
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.core
}

// Service returns the core, lazily creating it when Startup failed. Controller
// goroutines call this concurrently, so the cached config is guarded.
func (h *Host) Service() *appcore.Service {
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.core == nil {
		h.core = appcore.New(appcore.Options{
			Config:        h.cfg,
			Runtime:       appRuntimeOptions(),
			Version:       h.version,
			OnEvent:       h.emitCoreEvent,
			LogLevel:      h.logLevel,
			LogToFileOnly: h.logToFileOnly,
		})
	}
	h.cfg = h.core.Config()
	return h.core
}

// Config returns the latest loaded config.
func (h *Host) Config() *configs.Config {
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.core != nil {
		h.cfg = h.core.Config()
	}
	return h.cfg
}

// Version returns the normalized version string.
func (h *Host) Version() string { return h.version }

// ServerNotice returns a non-blocking notice about HTTP server availability.
func (h *Host) ServerNotice() string {
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.serverNotice
}

// Shutdown stops the server then the core.
func (h *Host) Shutdown() {
	h.mu.Lock()
	server := h.server
	core := h.core
	h.mu.Unlock()
	if server != nil {
		server.Stop()
	}
	if core != nil {
		core.Shutdown()
	}
}

// emitCoreEvent translates every appcore event into a UI-neutral Event.
func (h *Host) emitCoreEvent(event appcore.Event) {
	switch event.Kind {
	case appcore.EventSearchModsUpdated:
		if update, ok := event.Payload.(appstructs.SearchModsUpdate); ok {
			h.emit(EventSearchModsUpdated, update)
		}
	case appcore.EventDownloadStatesUpdated:
		h.emit(EventDownloadStatesUpdated, nil)
	case appcore.EventDownloadQueueUpdated:
		if state, ok := event.Payload.(appstructs.DownloadQueueState); ok {
			h.emit(EventDownloadQueueUpdated, state)
		}
	case appcore.EventDownloadFailed:
		if failed, ok := event.Payload.(appstructs.DownloadFailedEvent); ok {
			h.emit(EventDownloadFailed, failed)
		}
	case appcore.EventDownloadCompleted:
		h.emit(EventDownloadCompleted, nil)
	case appcore.EventMinecraftDirChanged:
		dir, _ := event.Payload.(string)
		h.emit(EventMinecraftDirChanged, dir)
	case appcore.EventSelectedVersionChanged:
		if version, ok := event.Payload.(structs.VersionInfo); ok {
			h.emit(EventSelectedVersionChanged, version)
		}
	case appcore.EventUsageStatsUpdated:
		h.emit(EventUsageStatsUpdated, nil)
	}
}

// emitHTTPServerEvent bridges the browser-extension event.
func (h *Host) emitHTTPServerEvent(event httpserver.Event) {
	if event.Name != httpserver.ExtensionModsAcceptedEvent {
		return
	}
	if update, ok := event.Payload.(appstructs.SearchModsUpdate); ok {
		h.emit(EventExtensionModsAccepted, update)
	}
}
