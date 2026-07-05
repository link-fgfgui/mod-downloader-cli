package cliapp

import (
	"bytes"
	"context"
	"encoding/json"
	"flag"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/urfave/cli/v2"

	"github.com/link-fgfgui/mod-downloader-core/global"
	"github.com/link-fgfgui/mod-downloader-core/models"
	structs "github.com/link-fgfgui/mod-downloader-core/structs/minecraft"
)

func TestAppDoesNotExposePersistentConfigOrVersionsCommands(t *testing.T) {
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	app := New(&stdout, &stderr)

	if app.Command("config") != nil {
		t.Fatal("config command is exposed")
	}
	if app.Command("versions") != nil {
		t.Fatal("versions command is exposed")
	}
}

func TestSearchTargetFromContext(t *testing.T) {
	tests := []struct {
		name      string
		args      []string
		wantQuery string
		wantMode  string
		wantErr   string
	}{
		{
			name:      "positional query uses search mode",
			args:      []string{"sodium"},
			wantQuery: "sodium",
			wantMode:  searchModeText,
		},
		{
			name:      "query flag uses search mode",
			args:      []string{"--query", "lithium"},
			wantQuery: "lithium",
			wantMode:  searchModeText,
		},
		{
			name:      "slug uses slug mode",
			args:      []string{"--slug", "sodium"},
			wantQuery: "sodium",
			wantMode:  searchModeSlug,
		},
		{
			name:      "id uses id mode",
			args:      []string{"--id", "AANobbMI"},
			wantQuery: "AANobbMI",
			wantMode:  searchModeID,
		},
		{
			name:    "requires one target",
			wantErr: "query, --slug, or --id is required",
		},
		{
			name:    "rejects mixed targets",
			args:    []string{"--slug", "sodium", "--id", "AANobbMI"},
			wantErr: "pass only one search target",
		},
		{
			name:    "rejects query flag plus positional query",
			args:    []string{"--query", "sodium", "lithium"},
			wantErr: "pass only one search target",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := newSearchTestContext(t, tt.args)
			got, err := searchTargetFromContext(ctx)
			if tt.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
					t.Fatalf("searchTargetFromContext() error = %v, want containing %q", err, tt.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("searchTargetFromContext() error = %v", err)
			}
			if got.Query != tt.wantQuery || got.Mode != tt.wantMode {
				t.Fatalf("searchTargetFromContext() = %#v, want query %q mode %q", got, tt.wantQuery, tt.wantMode)
			}
		})
	}
}

func TestWriteProjectsColumnOrder(t *testing.T) {
	var out bytes.Buffer
	err := writeProjects(&out, []models.ModProject{
		{
			ID:       "modrinth:AANobbMI",
			Slug:     "sodium",
			Title:    "Sodium",
			Platform: "Modrinth",
		},
	})
	if err != nil {
		t.Fatal(err)
	}

	lines := strings.Split(strings.TrimSpace(out.String()), "\n")
	if len(lines) != 2 {
		t.Fatalf("output line count = %d, want 2\n%s", len(lines), out.String())
	}
	if got := strings.Join(strings.Fields(lines[0]), "|"); got != "ID|SLUG|TITLE|PLATFORM" {
		t.Fatalf("header = %q, want ID|SLUG|TITLE|PLATFORM", got)
	}
	if got := strings.Join(strings.Fields(lines[1]), "|"); got != "modrinth:AANobbMI|sodium|Sodium|Modrinth" {
		t.Fatalf("row = %q, want reordered project fields", got)
	}
}

func newSearchTestContext(t *testing.T, args []string) *cli.Context {
	t.Helper()
	set := flag.NewFlagSet("search", flag.ContinueOnError)
	set.String("query", "", "")
	set.String("slug", "", "")
	set.String("id", "", "")
	if err := set.Parse(args); err != nil {
		t.Fatal(err)
	}
	return cli.NewContext(cli.NewApp(), set, nil)
}

func TestListCommandJSONScansCurrentModsDir(t *testing.T) {
	oldwd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	modsDir := filepath.Join(t.TempDir(), "mods")
	if err := os.MkdirAll(modsDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(modsDir); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := os.Chdir(oldwd); err != nil {
			t.Fatal(err)
		}
	})

	cacheDir := t.TempDir()
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	app := New(&stdout, &stderr)
	err = app.RunContext(context.Background(), []string{
		"mod-downloader-cli",
		"--cache-dir", cacheDir,
		"list",
		"--json",
	})
	if err != nil {
		t.Fatalf("list command failed: %v\nstderr: %s", err, stderr.String())
	}

	var got []structs.ModInfo
	if err := json.Unmarshal(stdout.Bytes(), &got); err != nil {
		t.Fatalf("decode json: %v\n%s", err, stdout.String())
	}
	if len(got) != 0 {
		t.Fatalf("mods len = %d, want 0", len(got))
	}
	if _, err := os.Stat("mod-downloader.toml"); !os.IsNotExist(err) {
		t.Fatalf("list created mod-downloader.toml or stat failed: %v", err)
	}
}

func TestInstallRequiresModsWorkdir(t *testing.T) {
	oldwd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	if err := os.Chdir(dir); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := os.Chdir(oldwd); err != nil {
			t.Fatal(err)
		}
	})

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	app := New(&stdout, &stderr)
	err = app.RunContext(context.Background(), []string{
		"mod-downloader-cli",
		"--cache-dir", t.TempDir(),
		"--mc-version", "1.21.1",
		"--loader", "fabric",
		"install",
		"modrinth:sodium",
	})
	if err == nil {
		t.Fatal("install succeeded outside a mods directory")
	}
	if !strings.Contains(err.Error(), "current directory must be a mods directory") {
		t.Fatalf("install error = %q", err)
	}
}

func TestInstallRequiresVersionAndLoader(t *testing.T) {
	oldwd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	modsDir := filepath.Join(t.TempDir(), "mods")
	if err := os.MkdirAll(modsDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(modsDir); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := os.Chdir(oldwd); err != nil {
			t.Fatal(err)
		}
	})

	t.Setenv("MINECRAFT_VERSION", "")
	t.Setenv("MOD_LOADER", "")

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	app := New(&stdout, &stderr)
	err = app.RunContext(context.Background(), []string{
		"mod-downloader-cli",
		"--cache-dir", t.TempDir(),
		"install",
		"modrinth:sodium",
	})
	if err == nil {
		t.Fatal("install succeeded without Minecraft version and loader")
	}
	if !strings.Contains(err.Error(), "mc-version is required") {
		t.Fatalf("install error = %q", err)
	}
}

func TestInferRuntimeFromModsParentManifest(t *testing.T) {
	t.Cleanup(func() {
		global.InvalidateVersions()
		global.ClearLocalMods()
	})
	versionDir := filepath.Join(t.TempDir(), "versions", "fabric-1.21.1")
	modsDir := filepath.Join(versionDir, "mods")
	if err := os.MkdirAll(modsDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(versionDir, "fabric-1.21.1.json"), []byte(`{
		"name": "Fabric 1.21.1",
		"id": "fabric-1.21.1",
		"patches": [
			{"id": "game", "version": "1.21.1"},
			{"id": "fabric", "version": "0.16.0"}
		]
	}`), 0o644); err != nil {
		t.Fatal(err)
	}

	got := inferRuntimeFromModsParent(modsDir)
	if got.MinecraftVersion != "1.21.1" || got.ModLoader != "fabric" {
		t.Fatalf("inferRuntimeFromModsParent() = %#v, want 1.21.1/fabric", got)
	}
}

func TestInferRuntimeFromPrismModsDirUsesLauncherScan(t *testing.T) {
	t.Cleanup(func() {
		global.InvalidateVersions()
		global.ClearLocalMods()
	})
	instancesDir := t.TempDir()
	instanceDir := filepath.Join(instancesDir, "FabricPack")
	gameDir := filepath.Join(instanceDir, ".minecraft")
	versionDir := filepath.Join(gameDir, "versions", "fabric-loader-1.21.1")
	modsDir := filepath.Join(versionDir, "mods")
	if err := os.MkdirAll(modsDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(instanceDir, "instance.cfg"), []byte("{}"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(versionDir, "fabric-loader-1.21.1.json"), []byte(`{
		"name": "Fabric 1.21.1",
		"id": "fabric-loader-1.21.1",
		"patches": [
			{"id": "game", "version": "1.21.1"},
			{"id": "fabric", "version": "0.16.0"}
		]
	}`), 0o644); err != nil {
		t.Fatal(err)
	}

	info, ok := inferVersionInfoForModsDir(modsDir)
	if !ok {
		t.Fatal("inferVersionInfoForModsDir() ok = false, want true")
	}
	if info.ID != "FabricPack/fabric-loader-1.21.1" || info.Name != "FabricPack" {
		t.Fatalf("inferred version = %#v, want Prism composite ID", info)
	}

	got := inferRuntimeFromModsParent(modsDir)
	if got.MinecraftVersion != "1.21.1" || got.ModLoader != "fabric" {
		t.Fatalf("inferRuntimeFromModsParent() = %#v, want 1.21.1/fabric", got)
	}
}

func TestInferRuntimeFromModsParentIgnoresUnsupportedLoader(t *testing.T) {
	t.Cleanup(func() {
		global.InvalidateVersions()
		global.ClearLocalMods()
	})
	versionDir := filepath.Join(t.TempDir(), "versions", "vanilla-1.21.1")
	modsDir := filepath.Join(versionDir, "mods")
	if err := os.MkdirAll(modsDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(versionDir, "vanilla-1.21.1.json"), []byte(`{
		"name": "Vanilla 1.21.1",
		"id": "vanilla-1.21.1",
		"patches": [
			{"id": "game", "version": "1.21.1"}
		]
	}`), 0o644); err != nil {
		t.Fatal(err)
	}

	got := inferRuntimeFromModsParent(modsDir)
	if got.MinecraftVersion != "1.21.1" || got.ModLoader != "" {
		t.Fatalf("inferRuntimeFromModsParent() = %#v, want version only", got)
	}
}

func TestCacheCleanUsesCacheDir(t *testing.T) {
	cacheDir := t.TempDir()
	cachePath := filepath.Join(cacheDir, "mods.gob.zst")
	if err := os.WriteFile(cachePath, []byte("cache"), 0o644); err != nil {
		t.Fatal(err)
	}

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	app := New(&stdout, &stderr)
	err := app.RunContext(context.Background(), []string{
		"mod-downloader-cli",
		"--cache-dir", cacheDir,
		"cache",
		"clean",
		"--json",
	})
	if err != nil {
		t.Fatalf("cache clean failed: %v\nstderr: %s", err, stderr.String())
	}

	var got struct {
		Path    string `json:"path"`
		Removed bool   `json:"removed"`
	}
	if err := json.Unmarshal(stdout.Bytes(), &got); err != nil {
		t.Fatalf("decode json: %v\n%s", err, stdout.String())
	}
	if got.Path != cachePath || !got.Removed {
		t.Fatalf("cache clean result = %#v, want path %q removed", got, cachePath)
	}
	if _, err := os.Stat(cachePath); !os.IsNotExist(err) {
		t.Fatalf("cache file still exists or stat failed: %v", err)
	}
}
