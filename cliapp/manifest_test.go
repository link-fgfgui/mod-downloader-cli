package cliapp

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/link-fgfgui/mod-downloader-core/models"
)

func TestFormatManifest(t *testing.T) {
	entries := []manifestEntry{
		{Platform: "modrinth", Project: "sodium", Version: "0.5.8"},
		{Platform: "curseforge", Project: "iris"},
		{LocalFile: "custom-addon.jar"},
	}
	header := manifestHeader{
		ExportedAt:       time.Date(2026, 10, 14, 0, 0, 0, 0, time.UTC),
		MinecraftVersion: "1.21.1",
		Loader:           "fabric",
	}
	out := formatManifest(entries, header)
	for _, want := range []string{
		"modrinth:sodium==0.5.8",
		"curseforge:iris",
		"# local: custom-addon.jar",
		"# minecraft: 1.21.1",
		"# loader: fabric",
		"# exported: 2026-10-14T00:00:00Z",
	} {
		if !strings.Contains(out, want) {
			t.Fatalf("manifest missing %q:\n%s", want, out)
		}
	}
}

func TestParseManifest(t *testing.T) {
	text := "# comment\n\nmodrinth:sodium==0.5.8\ncurseforge:iris\nmodrinth:lithium=1.0\n"
	entries, err := parseManifest(text)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 3 {
		t.Fatalf("entries = %#v, want 3", entries)
	}
	if entries[0].Platform != "modrinth" || entries[0].Project != "sodium" || entries[0].Version != "0.5.8" {
		t.Fatalf("entry 0 = %#v", entries[0])
	}
	if entries[1].Version != "" {
		t.Fatalf("entry 1 version = %q, want empty", entries[1].Version)
	}
	if entries[2].Version != "1.0" {
		t.Fatalf("entry 2 version = %q, want 1.0", entries[2].Version)
	}

	for _, bad := range []string{"noplatform", "bogus:x", "modrinth:"} {
		if _, err := parseManifest(bad); err == nil {
			t.Fatalf("parseManifest(%q) succeeded, want error", bad)
		}
	}
}

func TestExportCommand(t *testing.T) {
	isolateGlobalState(t)
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	old := resolveProjectsByHashesFn
	resolveProjectsByHashesFn = func([]string) map[string]models.ModProject { return nil }
	t.Cleanup(func() { resolveProjectsByHashesFn = old })

	modsDir := filepath.Join(t.TempDir(), "mods")
	writeModJar(t, modsDir, "custom-addon.jar", fabricJarEntries("custom", "1.0", "{}"))

	out, _, err := runCLI(t, "--dir", modsDir, "export")
	if err != nil {
		t.Fatalf("export failed: %v", err)
	}
	if !strings.Contains(out, "# local: custom-addon.jar") {
		t.Fatalf("export output missing local entry:\n%s", out)
	}

	outputFile := filepath.Join(t.TempDir(), "mods.txt")
	if _, _, err := runCLI(t, "--dir", modsDir, "export", "-o", outputFile); err != nil {
		t.Fatalf("export -o failed: %v", err)
	}
	data, err := os.ReadFile(outputFile)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), "# local: custom-addon.jar") {
		t.Fatalf("exported file missing local entry:\n%s", data)
	}
}

func TestImportCommandEmptyManifest(t *testing.T) {
	isolateGlobalState(t)
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("MINECRAFT_VERSION", "1.21.1")
	t.Setenv("MOD_LOADER", "fabric")

	manifestFile := filepath.Join(t.TempDir(), "mods.txt")
	if err := os.WriteFile(manifestFile, []byte("# only comments\n\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	modsDir := filepath.Join(t.TempDir(), "mods")
	if err := os.MkdirAll(modsDir, 0o755); err != nil {
		t.Fatal(err)
	}

	out, _, err := runCLI(t, "--cache-dir", t.TempDir(), "--dir", modsDir, "import", manifestFile)
	if err != nil {
		t.Fatalf("import failed: %v", err)
	}
	if !strings.Contains(out, "installed 0, skipped 0, failed 0") {
		t.Fatalf("import summary = %q", out)
	}

	if _, _, err := runCLI(t, "--cache-dir", t.TempDir(), "--dir", modsDir, "import", filepath.Join(t.TempDir(), "missing.txt")); err == nil {
		t.Fatal("import succeeded with a missing manifest file")
	}
}
