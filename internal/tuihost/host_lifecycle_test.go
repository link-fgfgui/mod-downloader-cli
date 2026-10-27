package tuihost

import (
	"context"
	"errors"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/link-fgfgui/mod-downloader-core/appcore"
	"github.com/link-fgfgui/mod-downloader-core/httpserver"
	"github.com/link-fgfgui/mod-downloader-core/logging"
)

// newLifecycleHost isolates every filesystem lookup the core performs: the
// config is read from the working directory, while the log and cache files use
// the user cache or temp directories. Pointing all of them at one temporary
// directory keeps the real lifecycle tests repeatable and keeps them from
// touching the developer's own state.
func newLifecycleHost(t *testing.T) *Host {
	t.Helper()
	dir := t.TempDir()
	t.Chdir(dir)
	t.Setenv("XDG_CACHE_HOME", dir)
	t.Setenv("HOME", dir)
	t.Setenv("LOCALAPPDATA", dir)
	t.Setenv("TMPDIR", dir)
	h := New(Options{
		Version:               "test",
		LoadMinecraftReleases: false,
		LogToFileOnly:         true,
		LogLevel:              slog.LevelError,
	})
	t.Cleanup(h.Shutdown)
	return h
}

func TestStartShutdownLifecycle(t *testing.T) {
	h := newLifecycleHost(t)

	if err := h.Start(context.Background()); err != nil {
		t.Fatalf("Start: %v", err)
	}
	if !h.Started() {
		t.Fatal("host does not report started after Start")
	}
	if svc := h.Service(); svc == nil {
		t.Fatal("Service() = nil after Start")
	} else if svc != h.Core() {
		t.Fatal("Service() and Core() returned different services")
	}
	if cfg := h.Config(); cfg == nil {
		t.Fatal("Config() = nil after Start")
	}
	if got := h.Version(); got != "test" {
		t.Fatalf("Version() = %q, want %q", got, "test")
	}

	h.Shutdown()
	if h.Started() {
		t.Fatal("host still reports started after Shutdown")
	}
	if svc := h.Service(); svc != nil {
		t.Fatalf("Service() = %p after Shutdown, want nil", svc)
	}
	// A second Shutdown must be a harmless no-op.
	h.Shutdown()
}

func TestConfigIsNilBeforeStart(t *testing.T) {
	h := New(Options{Version: "test"})
	if cfg := h.Config(); cfg != nil {
		t.Fatalf("Config() = %p before Start, want nil", cfg)
	}
	if got := h.Version(); got != "test" {
		t.Fatalf("Version() = %q, want %q", got, "test")
	}
	h.Shutdown()
}

func TestStartWithEventSinkRegistered(t *testing.T) {
	h := newLifecycleHost(t)

	var events atomic.Int64
	h.SetEventSink(func(Event) { events.Add(1) })

	if err := h.Start(context.Background()); err != nil {
		t.Fatalf("Start: %v", err)
	}
	if !h.Started() {
		t.Fatal("host does not report started after Start")
	}
	// Start must not clear a sink registered before startup.
	h.emit(EventDownloadCompleted, nil)
	if got := events.Load(); got != 1 {
		t.Fatalf("sink calls = %d after Start, want 1", got)
	}
	h.Shutdown()
	if h.Started() {
		t.Fatal("host still reports started after Shutdown")
	}
}

// TestSecondStartIsIdempotent verifies that calling Start on an already-started
// host returns nil and keeps the original core/service pair. Replacing the core
// would leak its background tasks and HTTP listener.
func TestSecondStartIsIdempotent(t *testing.T) {
	h := newLifecycleHost(t)

	if err := h.Start(context.Background()); err != nil {
		t.Fatalf("first Start: %v", err)
	}
	first := h.Core()
	if first == nil {
		t.Fatal("Core() = nil after Start")
	}
	if err := h.Start(context.Background()); err != nil {
		t.Fatalf("second Start: %v", err)
	}
	if !h.Started() {
		t.Fatal("host does not report started after a second Start")
	}
	if got := h.Core(); got != first {
		t.Fatalf("second Start replaced the core: %p != %p", got, first)
	}
	if got := h.Service(); got != first {
		t.Fatalf("Service() = %p after a second Start, want %p", got, first)
	}

	h.Shutdown()
	if h.Started() {
		t.Fatal("host still reports started after Shutdown")
	}
	if svc := h.Service(); svc != nil {
		t.Fatalf("Service() = %p after Shutdown, want nil", svc)
	}
	h.Shutdown()
}

func TestAppRuntimeOptionsUsesWorkingDirectory(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)

	wd, err := os.Getwd()
	if err != nil {
		t.Fatalf("Getwd: %v", err)
	}
	opts := appRuntimeOptions()
	if opts.DefaultCacheDir != wd {
		t.Fatalf("DefaultCacheDir = %q, want %q", opts.DefaultCacheDir, wd)
	}
}

func TestAppRuntimeOptionsFallsBackWhenWorkingDirectoryDisappears(t *testing.T) {
	old, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	t.Chdir(dir)
	if err := os.Remove(dir); err != nil {
		t.Fatal(err)
	}
	// The test process keeps running from the deleted directory until the
	// cleanup restores the previous one; os.Getwd must report the failure.
	opts := appRuntimeOptions()
	if opts.DefaultCacheDir != "" {
		t.Fatalf("DefaultCacheDir=%q, want empty on Getwd failure", opts.DefaultCacheDir)
	}
	if err := os.Chdir(old); err != nil {
		t.Fatal(err)
	}
}

// TestStartFailureCanBeRetried injects a failing core startup and verifies the
// host never caches the failed service and that a later Start succeeds.
func TestStartFailureCanBeRetried(t *testing.T) {
	h := newLifecycleHost(t)
	boom := errors.New("startup failed")
	calls := 0
	h.startCore = func(context.Context) (*appcore.Service, error) {
		calls++
		return nil, boom
	}

	if err := h.Start(context.Background()); !errors.Is(err, boom) {
		t.Fatalf("Start error = %v, want %v", err, boom)
	}
	if calls != 1 {
		t.Fatalf("startCore calls = %d, want 1", calls)
	}
	if h.Started() {
		t.Fatal("host reports started after a failed Start")
	}
	if svc := h.Service(); svc != nil {
		t.Fatalf("Service() = %p after a failed Start, want nil", svc)
	}
	if cfg := h.Config(); cfg != nil {
		t.Fatalf("Config() = %p after a failed Start, want nil", cfg)
	}

	// The failure must not poison the host: retry with the real starter.
	h.startCore = h.startCoreDefault
	if err := h.Start(context.Background()); err != nil {
		t.Fatalf("retry Start: %v", err)
	}
	if !h.Started() || h.Service() == nil {
		t.Fatal("host did not start on retry")
	}
}

// TestRestartAfterShutdown verifies the host can be started again after a
// clean shutdown instead of staying permanently closed.
func TestRestartAfterShutdown(t *testing.T) {
	h := newLifecycleHost(t)

	if err := h.Start(context.Background()); err != nil {
		t.Fatalf("first Start: %v", err)
	}
	first := h.Core()
	h.Shutdown()
	if h.Started() {
		t.Fatal("host still started after Shutdown")
	}

	if err := h.Start(context.Background()); err != nil {
		t.Fatalf("restart: %v", err)
	}
	second := h.Core()
	if second == nil || !h.Started() {
		t.Fatal("host did not start on restart")
	}
	if second == first {
		t.Fatal("restart reused the shut-down core")
	}
	h.Shutdown()
	if h.Started() {
		t.Fatal("host still started after the second Shutdown")
	}
}

// TestConcurrentStartKeepsOneCore serializes competing Start calls; exactly one
// core survives and every caller sees success.
func TestConcurrentStartKeepsOneCore(t *testing.T) {
	h := newLifecycleHost(t)

	const callers = 8
	var wg sync.WaitGroup
	errs := make(chan error, callers)
	for i := 0; i < callers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			errs <- h.Start(context.Background())
		}()
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatalf("concurrent Start: %v", err)
		}
	}

	core := h.Core()
	if core == nil {
		t.Fatal("Core() = nil after concurrent Start")
	}
	if err := h.Start(context.Background()); err != nil {
		t.Fatalf("Start after concurrent callers: %v", err)
	}
	if got := h.Core(); got != core {
		t.Fatalf("later Start replaced the core: %p != %p", got, core)
	}
}

// TestUnhandledEventsAreLogged locks in the P1-5 contract: contract drift must
// be visible in the log instead of silently dropping events.
func TestUnhandledEventsAreLogged(t *testing.T) {
	dir := t.TempDir()
	logPath := filepath.Join(dir, "host.log")
	if err := logging.Configure(logging.Options{
		NoStderr: true,
		FilePath: logPath,
		Level:    slog.LevelWarn,
	}); err != nil {
		t.Fatalf("configure logging: %v", err)
	}
	t.Cleanup(logging.Close)

	h := New(Options{})
	h.emitCoreEvent(appcore.Event{Kind: "future-core-event", Payload: 42})
	h.emitCoreEvent(appcore.Event{Kind: appcore.EventDownloadQueueUpdated, Payload: 42})
	h.emitHTTPServerEvent(httpserver.Event{Name: httpserver.ExtensionModsAcceptedEvent, Payload: 42})
	logging.Close()

	data, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatalf("read log: %v", err)
	}
	logText := string(data)
	for _, want := range []string{
		"unhandled core event",
		"future-core-event",
		"invalid core event payload",
		"downloadQueueUpdated",
		"invalid extension event payload",
	} {
		if !strings.Contains(logText, want) {
			t.Fatalf("log does not contain %q:\n%s", want, logText)
		}
	}
}
