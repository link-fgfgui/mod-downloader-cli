package cliapp

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/link-fgfgui/mod-downloader-core/global"
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
