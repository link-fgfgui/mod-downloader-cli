package cliapp

import (
	"archive/zip"
	"bytes"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/urfave/cli/v2"

	"github.com/link-fgfgui/mod-downloader-core/global"
	structs "github.com/link-fgfgui/mod-downloader-core/structs/minecraft"
)

// TestMain neutralizes cli.OsExiter so that commands returning cli.Exit errors
// do not terminate the test process; RunContext still returns the error.
func TestMain(m *testing.M) {
	cli.OsExiter = func(int) {}
	os.Exit(m.Run())
}

// modInfo builds a local ModInfo for unit tests.
func modInfo(id, version, path string) structs.ModInfo {
	return structs.ModInfo{
		ID:       id,
		Name:     id,
		Version:  version,
		Path:     path,
		FileName: strings.TrimSuffix(filepath.Base(path), ".jar"),
		Enabled:  true,
	}
}

// writeModJar creates a zip file with the given entries and returns its path.
func writeModJar(t *testing.T, dir, name string, entries map[string]string) string {
	t.Helper()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, name)
	f, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()

	zw := zip.NewWriter(f)
	names := make([]string, 0, len(entries))
	for n := range entries {
		names = append(names, n)
	}
	sort.Strings(names)
	for _, n := range names {
		w, err := zw.Create(n)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := w.Write([]byte(entries[n])); err != nil {
			t.Fatal(err)
		}
	}
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
	return path
}

// fabricJarEntries builds fabric.mod.json content for a test jar.
func fabricJarEntries(id, version, dependsJSON string) map[string]string {
	if dependsJSON == "" {
		dependsJSON = "{}"
	}
	content := fmt.Sprintf(
		`{"schemaVersion":1,"id":%q,"name":%q,"version":%q,"description":"desc","license":"MIT","depends":%s}`,
		id, id, version, dependsJSON,
	)
	return map[string]string{"fabric.mod.json": content}
}

// runCLI runs the CLI with the given arguments (without the program name).
func runCLI(t *testing.T, args ...string) (string, string, error) {
	t.Helper()
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	app := New(&stdout, &stderr)
	err := app.RunContext(context.Background(), append([]string{"mod-downloader-cli"}, args...))
	return stdout.String(), stderr.String(), err
}

// chdir moves into dir for the duration of the test.
func chdir(t *testing.T, dir string) {
	t.Helper()
	oldwd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(dir); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := os.Chdir(oldwd); err != nil {
			t.Fatal(err)
		}
	})
}

// isolateGlobalState resets shared core caches after the test.
func isolateGlobalState(t *testing.T) {
	t.Helper()
	t.Cleanup(func() {
		global.InvalidateVersions()
		global.ClearLocalMods()
		global.ClearJarMetadata()
		global.HardlinkIndexClear()
	})
}
