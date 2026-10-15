package cliapp

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	structs "github.com/link-fgfgui/mod-downloader-core/structs/minecraft"
)

func writeVersionManifest(t *testing.T, versionDir, id, mcVersion, loader string) {
	t.Helper()
	content := `{
		"name": "Test ` + mcVersion + `",
		"id": "` + id + `",
		"patches": [
			{"id": "game", "version": "` + mcVersion + `"},
			{"id": "` + loader + `", "version": "0.16.0"}
		]
	}`
	if err := os.WriteFile(filepath.Join(versionDir, id+".json"), []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestLocateRuntimeDirForms(t *testing.T) {
	isolateGlobalState(t)
	root := t.TempDir()
	versionDir := filepath.Join(root, "versions", "fabric-1.21.1")
	modsDir := filepath.Join(versionDir, "mods")
	if err := os.MkdirAll(modsDir, 0o755); err != nil {
		t.Fatal(err)
	}
	writeVersionManifest(t, versionDir, "fabric-1.21.1", "1.21.1", "fabric")

	// mods directory directly.
	got, _, ok, err := locateRuntimeDir(modsDir)
	if err != nil || !ok || got != modsDir {
		t.Fatalf("locate mods dir = %q ok=%v err=%v", got, ok, err)
	}

	// version directory containing mods/.
	got, info, ok, err := locateRuntimeDir(versionDir)
	if err != nil || !ok || got != modsDir {
		t.Fatalf("locate version dir = %q ok=%v err=%v", got, ok, err)
	}
	if info.MinecraftVersion != "1.21.1" || info.ModLoader != "fabric" {
		t.Fatalf("inferred info = %#v", info)
	}

	// game directory containing versions/.
	got, _, ok, err = locateRuntimeDir(root)
	if err != nil || !ok || got != modsDir {
		t.Fatalf("locate game dir = %q ok=%v err=%v", got, ok, err)
	}

	// missing directory.
	if _, _, _, err := locateRuntimeDir(filepath.Join(root, "does-not-exist")); err == nil {
		t.Fatal("locateRuntimeDir succeeded on a missing directory")
	}
}

func TestDirFlagListFromOtherDirectory(t *testing.T) {
	isolateGlobalState(t)
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("MINECRAFT_VERSION", "")
	t.Setenv("MOD_LOADER", "")

	root := t.TempDir()
	versionDir := filepath.Join(root, "versions", "fabric-1.21.1")
	modsDir := filepath.Join(versionDir, "mods")
	if err := os.MkdirAll(modsDir, 0o755); err != nil {
		t.Fatal(err)
	}
	writeVersionManifest(t, versionDir, "fabric-1.21.1", "1.21.1", "fabric")
	writeModJar(t, modsDir, "sodium.jar", fabricJarEntries("sodium", "0.5.8", "{}"))

	// Run from an unrelated working directory.
	chdir(t, t.TempDir())

	out, _, err := runCLI(t, "--cache-dir", t.TempDir(), "--dir", versionDir, "list", "--json")
	if err != nil {
		t.Fatalf("list --dir failed: %v", err)
	}
	var mods []structs.ModInfo
	if err := json.Unmarshal([]byte(out), &mods); err != nil {
		t.Fatalf("decode mods: %v\n%s", err, out)
	}
	if len(mods) != 1 || mods[0].ID != "sodium" {
		t.Fatalf("mods = %#v", mods)
	}
}
