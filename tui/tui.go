package tui

import (
	"context"
	"os"

	tea "github.com/charmbracelet/bubbletea"
)

// prepareLogging forces the core logger to write to a log file instead of
// stdout: Bubble Tea owns stdout for the full-screen UI, so interleaved log
// lines would corrupt the display.
func prepareLogging() {
	if os.Getenv("LOGGING_FORCE_FILE") == "" {
		_ = os.Setenv("LOGGING_FORCE_FILE", "true")
	}
}

// Options configures a TUI run.
type Options struct {
	// Version is the normalized app version (ldflags-injected, "dev" fallback).
	Version string
	// LoadMinecraftReleases mirrors the GUI (true).
	LoadMinecraftReleases bool
}

// Run starts the full-screen TUI and blocks until the user quits, then performs
// the ordered shutdown (HTTP server stop -> core shutdown) so the selected
// .minecraft directory, background tasks and SQLite/log files are flushed.
func Run(ctx context.Context, opts Options) error {
	prepareLogging()
	host := NewHost(HostOptions{Version: opts.Version})
	hostOpts := HostOptions{Version: opts.Version, LoadMinecraftReleases: opts.LoadMinecraftReleases}
	if err := host.Start(ctx, hostOpts); err != nil {
		host.Shutdown()
		return err
	}

	model := NewAppModel(host)

	program := tea.NewProgram(
		model,
		tea.WithAltScreen(),
		tea.WithMouseCellMotion(),
	)
	host.ConfigureProgram(program)

	if _, err := program.Run(); err != nil {
		host.Shutdown()
		return err
	}
	host.Shutdown()
	return nil
}
