package cliapp

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/link-fgfgui/mod-downloader/core/appcore"
	"github.com/link-fgfgui/mod-downloader/core/global"
	"github.com/link-fgfgui/mod-downloader/core/models"
	appstructs "github.com/link-fgfgui/mod-downloader/core/structs"
	appview "github.com/link-fgfgui/mod-downloader/core/structs/view"
)

func TestInstallMultiTargetFallsBackToWholeSearch(t *testing.T) {
	installSeams(t)
	isolateGlobalState(t)

	modsDir := filepath.Join(t.TempDir(), "mods")
	if err := os.MkdirAll(modsDir, 0o755); err != nil {
		t.Fatal(err)
	}
	chdir(t, modsDir)

	lookupProjectBySlugFn = func(_ *appcore.Service, platform, slug, _, _ string) (models.ModProject, bool) {
		if platform == platformModrinth && slug == "sodium" {
			return projectRef(platform, "AANobbMI", slug, "Sodium", 1), true
		}
		return models.ModProject{}, false
	}
	lookupProjectByIDFn = func(*appcore.Service, string, string, string, string) (models.ModProject, bool) {
		return models.ModProject{}, false
	}
	searchModsTextFn = func(_ *appcore.Service, req appstructs.SearchModsRequest) appview.SearchModsUpdate {
		if req.Query != "sodium nonexistent" {
			t.Fatalf("search query = %q, want whole input", req.Query)
		}
		return appview.SearchModsUpdate{Results: []models.ModProject{
			projectRef(platformModrinth, "minimap", "minimap", "MiniMap", 100),
		}}
	}
	listMatchingVersionsFn = func(*appcore.Service, models.ModProject, string, string) []models.ModVersion {
		return []models.ModVersion{{ID: "ver-1", Version: "1.0", FileName: "mod.jar", FileSize: 1, ModIDs: []string{"minimap"}}}
	}
	versionModIDsFn = func(models.ModVersion, string) []string { return nil }
	localModPathsFn = func([]string, string) []global.LocalModFilePath { return nil }
	instanceIDForTargetDirFn = func(string) string { return "test" }

	stdout, stderr, err := runCLI(t,
		"--cache-dir", t.TempDir(),
		"--mc-version", "1.21.1",
		"--loader", "fabric",
		"install", "sodium", "nonexistent",
		"--dry-run", "--no-input",
	)
	if err != nil {
		t.Fatalf("install failed: %v\nstderr: %s", err, stderr)
	}
	for _, want := range []string{"not found: nonexistent", `falling back to a single search for "sodium nonexistent"`} {
		if !strings.Contains(stderr, want) {
			t.Fatalf("stderr missing %q:\n%s", want, stderr)
		}
	}
	if !strings.Contains(stdout, "sodium nonexistent") || !strings.Contains(stdout, "MiniMap") {
		t.Fatalf("stdout missing whole-search plan:\n%s", stdout)
	}
	if strings.Contains(stdout, "Sodium") {
		t.Fatalf("successful token result should be discarded:\n%s", stdout)
	}
}
