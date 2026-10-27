package tviewui

import (
	"context"
	"io"
	"os"
	"testing"
	"time"

	"github.com/gdamore/tcell/v2"
	"go.uber.org/goleak"

	"github.com/link-fgfgui/mod-downloader-core/global"
	"github.com/link-fgfgui/mod-downloader-core/logging"
)

// runSimulated runs the real tviewui.Run path (host startup, HTTP server,
// event pump, shutdown) against a simulation screen. Ctrl+C is injected before
// the loop starts, so the run exits as soon as the first event is read.
func runSimulated(t *testing.T, onReady func()) error {
	t.Helper()
	// Keep the host's config, sqlite, metadata and log files out of the repo and
	// the real user cache.
	t.Chdir(t.TempDir())
	cacheDir := t.TempDir()
	t.Setenv("XDG_CACHE_HOME", cacheDir)
	t.Setenv("LOCALAPPDATA", cacheDir)
	t.Setenv("HOME", cacheDir)

	// Keep setup logging out of the test output; Run reconfigures the logger.
	_ = logging.Configure(logging.Options{Disabled: true})

	// Seed the release-version cache so the startup refresh stays local and the
	// test does not depend on network access.
	previous := global.GetMinecraftReleaseVersions()
	global.SetMinecraftReleaseVersions([]string{"1.21.1"})
	t.Cleanup(func() { global.SetMinecraftReleaseVersions(previous) })

	sim := tcell.NewSimulationScreen("UTF-8")
	// tview calls Fini() from Application.Stop, so the screen must not be
	// finalized a second time here.
	t.Cleanup(logging.Close)

	started := make(chan *App, 1)
	errCh := make(chan error, 1)
	go func() {
		errCh <- runWith(context.Background(), Options{Version: "test"}, sim, func(a *App) {
			started <- a
		})
	}()

	var app *App
	select {
	case app = <-started:
	case err := <-errCh:
		return err
	case <-time.After(15 * time.Second):
		t.Fatal("tviewui.Run did not start")
	}

	// Wait for the initial refreshes so quitting cannot race their goroutines.
	select {
	case <-app.startupDone:
	case <-time.After(15 * time.Second):
		t.Fatal("startup refreshes did not finish")
	}
	if onReady != nil {
		onReady()
	}
	sim.SetSize(80, 24)
	sim.InjectKey(tcell.KeyCtrlC, 0, tcell.ModNone)

	select {
	case err := <-errCh:
		return err
	case <-time.After(15 * time.Second):
		t.Fatal("tviewui.Run did not stop after Ctrl+C")
		return nil
	}
}

func TestRunLifecycleNoLeaks(t *testing.T) {
	defer goleak.VerifyNone(t)
	if err := runSimulated(t, nil); err != nil {
		t.Fatalf("run: %v", err)
	}
}

// TestRunKeepsStderrClean starts and stops the full TUI while capturing
// os.Stderr. Startup emits an Info record from the HTTP server, so a leak here
// would show up as non-empty output and corrupt a real terminal.
func TestRunKeepsStderrClean(t *testing.T) {
	reader, writer, err := os.Pipe()
	if err != nil {
		t.Fatalf("pipe: %v", err)
	}
	original := os.Stderr
	os.Stderr = writer
	t.Cleanup(func() {
		os.Stderr = original
		_ = reader.Close()
	})

	if err := runSimulated(t, func() { logging.Info("tui stderr probe") }); err != nil {
		t.Fatalf("run: %v", err)
	}

	os.Stderr = original
	if err := writer.Close(); err != nil {
		t.Fatalf("close stderr pipe: %v", err)
	}
	data, err := io.ReadAll(reader)
	if err != nil {
		t.Fatalf("read stderr: %v", err)
	}
	if len(data) > 0 {
		t.Fatalf("TUI wrote %d bytes to stderr: %q", len(data), data)
	}
}
