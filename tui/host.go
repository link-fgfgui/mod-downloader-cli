package tui

import (
	"context"
	"fmt"
	"os"
	"strings"
	"sync"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/link-fgfgui/mod-downloader-core/appcore"
	"github.com/link-fgfgui/mod-downloader-core/configs"
	"github.com/link-fgfgui/mod-downloader-core/httpserver"
	"github.com/link-fgfgui/mod-downloader-core/logging"
	appstructs "github.com/link-fgfgui/mod-downloader-core/structs"
	structs "github.com/link-fgfgui/mod-downloader-core/structs/minecraft"
)

// Host is the TUI adapter layer (Layer 2): it owns the core Service + HTTP
// server lifecycle exactly like the Wails App, and bridges their event
// callbacks into Bubble Tea messages.
type Host struct {
	core    *appcore.Service
	server  *httpserver.Server
	program *tea.Program
	cfg     *configs.Config
	version string

	mu           sync.Mutex
	serverNotice string
}

// HostOptions configures the host.
type HostOptions struct {
	// Version is the normalized app version (empty falls back to "dev").
	Version string
	// LoadMinecraftReleases mirrors the GUI (true).
	LoadMinecraftReleases bool
	// Config optionally supplies a pre-loaded config (mirrors wails App.config).
	Config *configs.Config
}

// NewHost builds the host but does not start it.
func NewHost(opts HostOptions) *Host {
	return &Host{
		version: normalizeVersion(opts.Version),
	}
}

// normalizeVersion mirrors currentAppVersion(): trim and fall back to "dev".
func normalizeVersion(version string) string {
	if v := strings.TrimSpace(version); v != "" {
		return v
	}
	return "dev"
}

// ConfigureProgram registers the program used to deliver adapter events. It is
// safe to call after Start: events emitted before registration are dropped,
// matching the GUI's `ctx == nil` guard.
func (h *Host) ConfigureProgram(p *tea.Program) {
	h.mu.Lock()
	h.program = p
	h.mu.Unlock()
}

// Start mirrors App.startup: create + start the core service, then start the
// HTTP server (a listen failure is logged and surfaced as a non-fatal notice).
func (h *Host) Start(ctx context.Context, opts HostOptions) error {
	h.core = appcore.New(appcore.Options{
		Config:                opts.Config,
		Runtime:               appRuntimeOptions(),
		Version:               h.version,
		LoadMinecraftReleases: opts.LoadMinecraftReleases,
		OnEvent:               h.emitCoreEvent,
	})
	if err := h.core.Startup(ctx); err != nil {
		h.cfg = h.core.Config()
		return err
	}
	h.cfg = h.core.Config()

	h.server = httpserver.New(httpserver.DefaultAddr, httpserver.Options{OnEvent: h.emitHTTPServerEvent})
	if err := h.server.Start(); err != nil {
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
func (h *Host) Core() *appcore.Service { return h.core }

// Service returns the core, lazily creating it when Startup failed (mirrors
// App.service()).
func (h *Host) Service() *appcore.Service {
	if h.core == nil {
		h.core = appcore.New(appcore.Options{
			Config:  h.cfg,
			Runtime: appRuntimeOptions(),
			Version: h.version,
			OnEvent: h.emitCoreEvent,
		})
	}
	h.cfg = h.core.Config()
	return h.core
}

// Config returns the latest loaded config.
func (h *Host) Config() *configs.Config {
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

// Send delivers a message to the main loop when a program is registered.
func (h *Host) Send(msg tea.Msg) {
	h.mu.Lock()
	p := h.program
	h.mu.Unlock()
	if p == nil {
		return
	}
	p.Send(msg)
}

// Shutdown mirrors App.shutdown: stop the server then the core.
func (h *Host) Shutdown() {
	if h.server != nil {
		h.server.Stop()
	}
	if h.core != nil {
		h.core.Shutdown()
	}
}

// emitCoreEvent translates every appcore event into a typed Bubble Tea message.
func (h *Host) emitCoreEvent(event appcore.Event) {
	switch event.Kind {
	case appcore.EventSearchModsUpdated:
		if update, ok := event.Payload.(appstructs.SearchModsUpdate); ok {
			h.Send(SearchModsUpdatedMsg{Update: update})
		}
	case appcore.EventDownloadStatesUpdated:
		h.Send(DownloadStatesUpdatedMsg{})
	case appcore.EventDownloadQueueUpdated:
		if state, ok := event.Payload.(appstructs.DownloadQueueState); ok {
			h.Send(DownloadQueueUpdatedMsg{State: state})
		}
	case appcore.EventDownloadFailed:
		if failed, ok := event.Payload.(appstructs.DownloadFailedEvent); ok {
			h.Send(DownloadFailedMsg{Event: failed})
		}
	case appcore.EventDownloadCompleted:
		h.Send(DownloadCompletedMsg{})
	case appcore.EventMinecraftDirChanged:
		dir, _ := event.Payload.(string)
		h.Send(MinecraftDirChangedMsg{Dir: dir})
	case appcore.EventSelectedVersionChanged:
		if version, ok := event.Payload.(structs.VersionInfo); ok {
			h.Send(SelectedVersionChangedMsg{Version: version})
		}
	case appcore.EventUsageStatsUpdated:
		h.Send(UsageStatsUpdatedMsg{})
	}
}

// emitHTTPServerEvent bridges the browser-extension event.
func (h *Host) emitHTTPServerEvent(event httpserver.Event) {
	if event.Name != httpserver.ExtensionModsAcceptedEvent {
		return
	}
	if update, ok := event.Payload.(appstructs.SearchModsUpdate); ok {
		h.Send(ExtensionModsAcceptedMsg{Update: update})
	}
}
