package main

import (
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/link-fgfgui/mod-downloader-cli/cliapp"
	"github.com/link-fgfgui/mod-downloader-cli/internal/tviewui"
)

// defaultAppVersion mirrors the GUI's version normalization.
const defaultAppVersion = "dev"

// appVersion is overridable at link time:
//
//	go build -ldflags "-X main.appVersion=..."
var appVersion = defaultAppVersion

func currentAppVersion() string {
	version := strings.TrimSpace(appVersion)
	if version == "" {
		return defaultAppVersion
	}
	return version
}

func main() {
	args := os.Args[1:]

	// No subcommand (or an explicit `tui`) launches the full-screen TUI. Any
	// other subcommand keeps the existing apt-style CLI workflow.
	if len(args) == 0 || args[0] == "tui" {
		if err := tviewui.Run(context.Background(), tviewui.Options{
			Version:               currentAppVersion(),
			LoadMinecraftReleases: true,
		}); err != nil {
			_, _ = fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		return
	}

	app := cliapp.New(os.Stdout, os.Stderr)
	if err := app.RunContext(context.Background(), os.Args); err != nil {
		_, _ = fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
