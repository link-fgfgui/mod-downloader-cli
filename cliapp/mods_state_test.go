package cliapp

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestDisableEnableRoundTrip(t *testing.T) {
	isolateGlobalState(t)
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("MINECRAFT_VERSION", "")
	t.Setenv("MOD_LOADER", "")

	modsDir := filepath.Join(t.TempDir(), "mods")
	writeModJar(t, modsDir, "sodium-fabric-0.5.8.jar", fabricJarEntries("sodium", "0.5.8", "{}"))
	chdir(t, modsDir)

	cacheDir := t.TempDir()
	out, _, err := runCLI(t, "--cache-dir", cacheDir, "disable", "--json", "sodium")
	if err != nil {
		t.Fatalf("disable failed: %v", err)
	}
	var changes []modStateChange
	if err := json.Unmarshal([]byte(out), &changes); err != nil {
		t.Fatal(err)
	}
	if len(changes) != 1 || !changes[0].Executed || changes[0].Action != "disable" {
		t.Fatalf("disable changes = %#v", changes)
	}
	disabledPath := filepath.Join(modsDir, "sodium-fabric-0.5.8.jar.disabled")
	if _, err := os.Stat(disabledPath); err != nil {
		t.Fatalf("disabled file missing: %v", err)
	}

	out, _, err = runCLI(t, "--cache-dir", cacheDir, "list", "--json")
	if err != nil {
		t.Fatalf("list failed: %v", err)
	}
	var mods []struct {
		ID      string `json:"id"`
		Enabled bool   `json:"enabled"`
	}
	if err := json.Unmarshal([]byte(out), &mods); err != nil {
		t.Fatal(err)
	}
	if len(mods) != 1 || mods[0].ID != "sodium" || mods[0].Enabled {
		t.Fatalf("list mods = %#v, want sodium disabled", mods)
	}

	// Repeating disable is a friendly no-op.
	out, _, err = runCLI(t, "--cache-dir", cacheDir, "disable", "--json", "sodium")
	if err != nil {
		t.Fatalf("repeat disable failed: %v", err)
	}
	changes = nil
	if err := json.Unmarshal([]byte(out), &changes); err != nil {
		t.Fatal(err)
	}
	if len(changes) != 1 || changes[0].Executed || changes[0].Note != "already disabled" {
		t.Fatalf("repeat disable changes = %#v", changes)
	}

	if _, _, err := runCLI(t, "--cache-dir", cacheDir, "enable", "--json", "sodium"); err != nil {
		t.Fatalf("enable failed: %v", err)
	}
	enabledPath := filepath.Join(modsDir, "sodium-fabric-0.5.8.jar")
	if _, err := os.Stat(enabledPath); err != nil {
		t.Fatalf("enabled file missing: %v", err)
	}

	out, _, err = runCLI(t, "--cache-dir", cacheDir, "enable", "--json", "sodium")
	if err != nil {
		t.Fatalf("repeat enable failed: %v", err)
	}
	changes = nil
	if err := json.Unmarshal([]byte(out), &changes); err != nil {
		t.Fatal(err)
	}
	if len(changes) != 1 || changes[0].Executed || changes[0].Note != "already enabled" {
		t.Fatalf("repeat enable changes = %#v", changes)
	}
}

func TestDisableDryRunKeepsFile(t *testing.T) {
	isolateGlobalState(t)
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("MINECRAFT_VERSION", "")
	t.Setenv("MOD_LOADER", "")

	modsDir := filepath.Join(t.TempDir(), "mods")
	writeModJar(t, modsDir, "lithium.jar", fabricJarEntries("lithium", "0.12", "{}"))
	chdir(t, modsDir)

	cacheDir := t.TempDir()
	out, _, err := runCLI(t, "--cache-dir", cacheDir, "disable", "--json", "--dry-run", "lithium")
	if err != nil {
		t.Fatalf("disable --dry-run failed: %v", err)
	}
	var changes []modStateChange
	if err := json.Unmarshal([]byte(out), &changes); err != nil {
		t.Fatal(err)
	}
	if len(changes) != 1 || changes[0].Executed || changes[0].To == "" {
		t.Fatalf("dry-run changes = %#v", changes)
	}
	if _, err := os.Stat(filepath.Join(modsDir, "lithium.jar")); err != nil {
		t.Fatalf("dry-run modified files: %v", err)
	}

	if _, _, err := runCLI(t, "--cache-dir", cacheDir, "disable", "--json", "nonexistent"); err == nil {
		t.Fatal("disable succeeded with no matching mod")
	}
}
