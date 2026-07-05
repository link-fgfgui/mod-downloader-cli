package main

import (
	"context"
	"fmt"
	"os"

	"github.com/link-fgfgui/mod-downloader-cli/cliapp"
)

func main() {
	app := cliapp.New(os.Stdout, os.Stderr)
	if err := app.RunContext(context.Background(), os.Args); err != nil {
		_, _ = fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
