package cliapp

import (
	"os/exec"
	"strings"
	"testing"
)

func TestCLIDoesNotDependOnWailsRuntime(t *testing.T) {
	packages := []string{
		"github.com/link-fgfgui/mod-downloader-cli/cliapp",
		"github.com/link-fgfgui/mod-downloader-cli/cmd/mod-downloader-cli",
	}
	args := append([]string{"list", "-deps"}, packages...)
	out, err := exec.Command("go", args...).Output()
	if err != nil {
		t.Fatalf("go list deps failed: %v", err)
	}
	for _, pkg := range strings.Fields(string(out)) {
		if pkg == "github.com/wailsapp/wails/v2/pkg/runtime" {
			t.Fatalf("CLI dependency tree includes Wails runtime")
		}
	}
}
