package cliapp

import (
	"bytes"
	"crypto/sha512"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/link-fgfgui/mod-downloader-core/appcore"
	"github.com/link-fgfgui/mod-downloader-core/models"
	"github.com/link-fgfgui/mod-downloader-core/storage"
	appstructs "github.com/link-fgfgui/mod-downloader-core/structs"
)

// TestConfigCommandTextOutput covers the human-readable config writers that
// the JSON-focused round-trip test does not reach.
func TestConfigCommandTextOutput(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())

	if _, _, err := runCLI(t, "config", "set", "loader", "fabric"); err != nil {
		t.Fatalf("config set: %v", err)
	}
	out, _, err := runCLI(t, "config", "get", "loader")
	if err != nil {
		t.Fatalf("config get: %v", err)
	}
	if strings.TrimSpace(out) != "loader=fabric" {
		t.Fatalf("config get = %q", out)
	}

	out, _, err = runCLI(t, "config", "list")
	if err != nil {
		t.Fatalf("config list: %v", err)
	}
	if !strings.Contains(out, "loader=fabric") {
		t.Fatalf("config list = %q", out)
	}

	out, _, err = runCLI(t, "config", "path")
	if err != nil {
		t.Fatalf("config path: %v", err)
	}
	path, err := userConfigPath()
	if err != nil {
		t.Fatal(err)
	}
	if strings.TrimSpace(out) != path {
		t.Fatalf("config path = %q, want %q", strings.TrimSpace(out), path)
	}

	out, _, err = runCLI(t, "config", "unset", "loader")
	if err != nil {
		t.Fatalf("config unset: %v", err)
	}
	if strings.TrimSpace(out) != "unset loader" {
		t.Fatalf("config unset = %q", out)
	}

	for _, args := range [][]string{
		{"config", "unset"},
		{"config", "unset", "nope"},
		{"config", "unset", "loader"},
	} {
		if _, _, err := runCLI(t, args...); err == nil {
			t.Fatalf("runCLI(%v) succeeded, want error", args)
		}
	}
}

// TestModsTextOutput covers the list/disable/enable tabular writers.
func TestModsTextOutput(t *testing.T) {
	isolateGlobalState(t)
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("MINECRAFT_VERSION", "")
	t.Setenv("MOD_LOADER", "")

	modsDir := filepath.Join(t.TempDir(), "mods")
	writeModJar(t, modsDir, "sodium.jar", fabricJarEntries("sodium", "0.5.8", "{}"))
	chdir(t, modsDir)
	cacheDir := t.TempDir()

	out, _, err := runCLI(t, "--cache-dir", cacheDir, "list")
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	for _, want := range []string{"ID", "NAME", "ENABLED", "sodium", "true"} {
		if !strings.Contains(out, want) {
			t.Fatalf("list output missing %q:\n%s", want, out)
		}
	}

	out, _, err = runCLI(t, "--cache-dir", cacheDir, "disable", "sodium")
	if err != nil {
		t.Fatalf("disable: %v", err)
	}
	for _, want := range []string{"ACTION", "STATUS", "disable", "done"} {
		if !strings.Contains(out, want) {
			t.Fatalf("disable output missing %q:\n%s", want, out)
		}
	}

	out, _, err = runCLI(t, "--cache-dir", cacheDir, "enable", "sodium")
	if err != nil {
		t.Fatalf("enable: %v", err)
	}
	if !strings.Contains(out, "enable") || !strings.Contains(out, "done") {
		t.Fatalf("enable output = %q", out)
	}
}

// TestCacheCleanTextOutput covers both cache clean outcomes.
func TestCacheCleanTextOutput(t *testing.T) {
	cacheDir := t.TempDir()

	out, _, err := runCLI(t, "--cache-dir", cacheDir, "cache", "clean")
	if err != nil {
		t.Fatalf("cache clean: %v", err)
	}
	if !strings.Contains(out, "cache not found") {
		t.Fatalf("cache clean = %q", out)
	}

	cachePath := filepath.Join(cacheDir, storage.CacheFileName)
	if err := os.WriteFile(cachePath, []byte("cache"), 0o644); err != nil {
		t.Fatal(err)
	}
	out, _, err = runCLI(t, "--cache-dir", cacheDir, "cache", "clean")
	if err != nil {
		t.Fatalf("cache clean existing: %v", err)
	}
	if !strings.Contains(out, "removed") || !strings.Contains(out, cachePath) {
		t.Fatalf("cache clean existing = %q", out)
	}
}

// TestShowAndSearchUnknownPlatform exercises the project resolution fallback and
// the search/show writers without leaving the process: an unknown platform has
// no provider, so every lookup misses.
func TestShowAndSearchUnknownPlatform(t *testing.T) {
	isolateGlobalState(t)
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("MINECRAFT_VERSION", "1.20.1")
	t.Setenv("MOD_LOADER", "fabric")
	cacheDir := t.TempDir()

	out, _, err := runCLI(t, "--cache-dir", cacheDir, "show", "--platform", "unknown", "mystery")
	if err != nil {
		t.Fatalf("show positional: %v", err)
	}
	for _, want := range []string{"unknown:mystery", "PLATFORM", "unknown", "VERSION_ID"} {
		if !strings.Contains(out, want) {
			t.Fatalf("show output missing %q:\n%s", want, out)
		}
	}

	out, _, err = runCLI(t, "--cache-dir", cacheDir, "show", "--platform", "unknown", "--project", "other")
	if err != nil {
		t.Fatalf("show --project: %v", err)
	}
	if !strings.Contains(out, "unknown:other") {
		t.Fatalf("show --project output = %q", out)
	}

	out, _, err = runCLI(t, "--cache-dir", cacheDir, "search", "--slug", "mystery", "--platform", "unknown")
	if err != nil {
		t.Fatalf("search slug: %v", err)
	}
	for _, want := range []string{"ID", "SLUG", "TITLE", "PLATFORM"} {
		if !strings.Contains(out, want) {
			t.Fatalf("search slug output missing %q:\n%s", want, out)
		}
	}

	if _, _, err := runCLI(t, "--cache-dir", cacheDir, "search", "--id", "mystery", "--platform", "unknown"); err != nil {
		t.Fatalf("search id: %v", err)
	}
}

// TestExportJSONManifestWriter covers the structured export writer.
func TestExportJSONManifestWriter(t *testing.T) {
	isolateGlobalState(t)
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	old := resolveProjectsByHashesFn
	resolveProjectsByHashesFn = func([]string) map[string]models.ModProject { return nil }
	t.Cleanup(func() { resolveProjectsByHashesFn = old })

	modsDir := filepath.Join(t.TempDir(), "mods")
	writeModJar(t, modsDir, "custom-addon.jar", fabricJarEntries("custom", "1.0", "{}"))

	out, _, err := runCLI(t, "--dir", modsDir, "export", "--json")
	if err != nil {
		t.Fatalf("export --json: %v", err)
	}
	var entries []manifestEntry
	if err := json.Unmarshal([]byte(out), &entries); err != nil {
		t.Fatalf("export --json output: %v\n%s", err, out)
	}
	if len(entries) != 1 || entries[0].LocalFile != "custom-addon.jar" {
		t.Fatalf("export entries = %#v", entries)
	}
}

// TestInstallWriters covers the install result/plan writers directly; the
// executed install paths need real downloads.
func TestInstallWriters(t *testing.T) {
	var buf bytes.Buffer
	queued := appcore.InstallWaitResult{Result: appstructs.ModDownloadResult{
		Queued: true, FileName: "sodium.jar", VersionID: "v1",
	}}
	if err := writeInstallResult(&buf, queued); err != nil {
		t.Fatal(err)
	}
	if got := strings.TrimSpace(buf.String()); got != "installed sodium.jar (v1)" {
		t.Fatalf("install result = %q", got)
	}

	buf.Reset()
	skipped := appcore.InstallWaitResult{Result: appstructs.ModDownloadResult{Skipped: true, Reason: "already installed"}}
	if err := writeInstallResult(&buf, skipped); err != nil {
		t.Fatal(err)
	}
	if got := strings.TrimSpace(buf.String()); got != "skipped: already installed" {
		t.Fatalf("skipped result = %q", got)
	}

	buf.Reset()
	if err := writeInstallResult(&buf, appcore.InstallWaitResult{}); err != nil {
		t.Fatal(err)
	}
	if got := strings.TrimSpace(buf.String()); got != "no download queued" {
		t.Fatalf("empty result = %q", got)
	}

	buf.Reset()
	if err := writeInstallResults(&buf, []appcore.InstallWaitResult{queued, skipped}); err != nil {
		t.Fatal(err)
	}
	if lines := strings.Split(strings.TrimSpace(buf.String()), "\n"); len(lines) != 2 {
		t.Fatalf("install results = %q", buf.String())
	}

	plan := []InstallPlanEntry{{
		Target: "unknown:mystery", Platform: "unknown", ProjectID: "mystery",
		Title: "Mystery", Version: "1.0", FileName: "mystery.jar", FileSize: 2048,
	}}
	buf.Reset()
	if err := writeInstallPlanJSON(&buf, plan); err != nil {
		t.Fatal(err)
	}
	var report InstallReport
	if err := json.Unmarshal(buf.Bytes(), &report); err != nil {
		t.Fatalf("install plan json: %v\n%s", err, buf.String())
	}
	if len(report.Targets) != 1 || report.Targets[0] != "unknown:mystery" || len(report.Resolved) != 1 {
		t.Fatalf("install plan report = %#v", report)
	}
	if err := writeInstallPlanJSON(&buf, "not a plan"); err == nil {
		t.Fatal("writeInstallPlanJSON accepted a non-plan payload")
	}

	buf.Reset()
	r := runner{stdout: &buf}
	if err := r.writeInstallPlan(plan, "1.20.1 | fabric"); err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"TARGET", "unknown:mystery", "resolved 1 project(s)"} {
		if !strings.Contains(buf.String(), want) {
			t.Fatalf("install plan text missing %q:\n%s", want, buf.String())
		}
	}
	buf.Reset()
	if err := r.writeInstallPlan(nil, "1.20.1 | fabric"); err != nil || buf.Len() != 0 {
		t.Fatalf("empty plan wrote %q (err=%v)", buf.String(), err)
	}
}

// TestCLIPureHelpers covers small helpers that the command tests only reach
// through narrower paths.
func TestCLIPureHelpers(t *testing.T) {
	projects := []models.ModProject{{Platform: "Modrinth"}, {Platform: "CurseForge"}}
	if got := filterProjectsByPlatform(projects, ""); len(got) != 2 {
		t.Fatalf("filter empty platform = %d projects", len(got))
	}
	if got := filterProjectsByPlatform(projects, " MODRINTH "); len(got) != 1 || got[0].Platform != "Modrinth" {
		t.Fatalf("filter modrinth = %#v", got)
	}
	if got := filterProjectsByPlatform(projects, "unknown"); len(got) != 0 {
		t.Fatalf("filter unknown = %#v", got)
	}

	sizes := map[int64]string{
		0:                  "—",
		-1:                 "—",
		512:                "512 B",
		1024:               "1.0 KiB",
		1536:               "1.5 KiB",
		1024 * 1024:        "1.0 MiB",
		1024 * 1024 * 1024: "1.0 GiB",
	}
	for bytes, want := range sizes {
		if got := humanSize(bytes); got != want {
			t.Fatalf("humanSize(%d) = %q, want %q", bytes, got, want)
		}
	}

	path := filepath.Join(t.TempDir(), "payload.bin")
	if err := os.WriteFile(path, []byte("hello"), 0o644); err != nil {
		t.Fatal(err)
	}
	if got := fileSize(path); got != 5 {
		t.Fatalf("fileSize = %d, want 5", got)
	}
	wantHash := fmt.Sprintf("%x", sha512.Sum512([]byte("hello")))
	if got := fileSHA512(path); got != wantHash {
		t.Fatalf("fileSHA512 = %q, want %q", got, wantHash)
	}
	if got := fileSize(filepath.Join(t.TempDir(), "missing.bin")); got != 0 {
		t.Fatalf("fileSize(missing) = %d", got)
	}
	if got := fileSHA512(filepath.Join(t.TempDir(), "missing.bin")); got != "" {
		t.Fatalf("fileSHA512(missing) = %q", got)
	}

	if writerIsTerminal(&bytes.Buffer{}) {
		t.Fatal("bytes.Buffer reported as a terminal")
	}
	// stdinIsTerminal just needs to be callable without a TTY; its result
	// depends on the environment running the tests.
	_ = stdinIsTerminal()
}
