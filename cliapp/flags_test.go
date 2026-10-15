package cliapp

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestTrailingFlagsAfterPositionalArgs(t *testing.T) {
	isolateGlobalState(t)
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("MINECRAFT_VERSION", "")
	t.Setenv("MOD_LOADER", "")

	modsDir := filepath.Join(t.TempDir(), "mods")
	writeModJar(t, modsDir, "lithium.jar", fabricJarEntries("lithium", "0.12", "{}"))
	chdir(t, modsDir)

	cacheDir := t.TempDir()
	jarPath := filepath.Join(modsDir, "lithium.jar")

	if _, _, err := runCLI(t, "--cache-dir", cacheDir, "remove", "--dry-run", "lithium"); err != nil {
		t.Fatalf("remove --dry-run failed: %v", err)
	}
	if _, err := os.Stat(jarPath); err != nil {
		t.Fatalf("remove --dry-run deleted the jar: %v", err)
	}

	if _, _, err := runCLI(t, "--cache-dir", cacheDir, "remove", "lithium", "--dry-run"); err != nil {
		t.Fatalf("remove with trailing --dry-run failed: %v", err)
	}
	if _, err := os.Stat(jarPath); err != nil {
		t.Fatalf("trailing --dry-run was ignored and the jar was deleted: %v", err)
	}

	if _, _, err := runCLI(t, "--cache-dir", cacheDir, "remove", "lithium", "--dry-run=false"); err != nil {
		t.Fatalf("remove with trailing --dry-run=false failed: %v", err)
	}
	if _, err := os.Stat(jarPath); !os.IsNotExist(err) {
		t.Fatalf("--dry-run=false was not honoured, jar still present: %v", err)
	}
}

func TestTrailingBoolFalseAndJSON(t *testing.T) {
	isolateGlobalState(t)
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("MINECRAFT_VERSION", "")
	t.Setenv("MOD_LOADER", "")

	modsDir := filepath.Join(t.TempDir(), "mods")
	writeModJar(t, modsDir, "iris.jar", fabricJarEntries("iris", "1.7.0", ""))
	chdir(t, modsDir)

	cacheDir := t.TempDir()
	out, _, err := runCLI(t, "--cache-dir", cacheDir, "disable", "iris", "--dry-run=false", "--json")
	if err != nil {
		t.Fatalf("disable failed: %v", err)
	}
	var changes []modStateChange
	if err := json.Unmarshal([]byte(out), &changes); err != nil {
		t.Fatalf("decode: %v\n%s", err, out)
	}
	if len(changes) != 1 || !changes[0].Executed {
		t.Fatalf("changes = %#v, want executed disable", changes)
	}
	if _, err := os.Stat(filepath.Join(modsDir, "iris.jar.disabled")); err != nil {
		t.Fatalf("jar was not disabled: %v", err)
	}
}

func TestTrailingStringFlagValue(t *testing.T) {
	isolateGlobalState(t)
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("MINECRAFT_VERSION", "")
	t.Setenv("MOD_LOADER", "")

	modsDir := filepath.Join(t.TempDir(), "mods")
	writeModJar(t, modsDir, "iris.jar", fabricJarEntries("iris", "1.7.0", ""))
	chdir(t, modsDir)

	output := filepath.Join(t.TempDir(), "mods.txt")
	if _, _, err := runCLI(t, "--cache-dir", t.TempDir(), "export", "-o", output); err != nil {
		t.Fatalf("export failed: %v", err)
	}
	if _, err := os.Stat(output); err != nil {
		t.Fatalf("export -o ignored: %v", err)
	}
}

func TestTrailingUnknownFlagFailsLoudly(t *testing.T) {
	isolateGlobalState(t)
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("MINECRAFT_VERSION", "")
	t.Setenv("MOD_LOADER", "")

	modsDir := filepath.Join(t.TempDir(), "mods")
	writeModJar(t, modsDir, "iris.jar", fabricJarEntries("iris", "1.7.0", ""))
	chdir(t, modsDir)

	if _, _, err := runCLI(t, "--cache-dir", t.TempDir(), "remove", "iris", "--not-a-flag"); err == nil {
		t.Fatal("unknown trailing flag was silently accepted")
	}
	if _, err := os.Stat(filepath.Join(modsDir, "iris.jar")); err != nil {
		t.Fatalf("jar was deleted despite the bad flag: %v", err)
	}
}
