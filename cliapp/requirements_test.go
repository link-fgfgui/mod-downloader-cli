package cliapp

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/link-fgfgui/mod-downloader/core/models"
)

func TestWriteRequirements(t *testing.T) {
	entries := []requirement{
		{Platform: "modrinth", Project: "sodium", Version: "0.5.8"},
		{Platform: "curseforge", Project: "iris"},
		{LocalFile: "custom-addon.jar"},
	}
	var buf bytes.Buffer
	if err := writeRequirements(&buf, entries); err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		"modrinth:sodium==0.5.8",
		"curseforge:iris",
		"# local: custom-addon.jar",
	} {
		if !strings.Contains(buf.String(), want) {
			t.Fatalf("requirements missing %q:\n%s", want, buf.String())
		}
	}
}

func TestParseRequirements(t *testing.T) {
	text := "# comment\n\nmodrinth:sodium==0.5.8\ncurseforge:iris\nmodrinth:lithium=1.0\n"
	entries, err := parseRequirements(text)
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
		if _, err := parseRequirements(bad); err == nil {
			t.Fatalf("parseRequirements(%q) succeeded, want error", bad)
		}
	}
}

func TestFreezeCommand(t *testing.T) {
	isolateGlobalState(t)
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	old := resolveProjectsByHashesFn
	resolveProjectsByHashesFn = func([]string) map[string]models.ModProject { return nil }
	t.Cleanup(func() { resolveProjectsByHashesFn = old })

	modsDir := filepath.Join(t.TempDir(), "mods")
	writeModJar(t, modsDir, "custom-addon.jar", fabricJarEntries("custom", "1.0", "{}"))

	out, _, err := runCLI(t, "--dir", modsDir, "freeze")
	if err != nil {
		t.Fatalf("freeze failed: %v", err)
	}
	if !strings.Contains(out, "# local: custom-addon.jar") {
		t.Fatalf("freeze output missing local entry:\n%s", out)
	}
}

func TestInstallRequirementsEmptyFile(t *testing.T) {
	isolateGlobalState(t)
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("MINECRAFT_VERSION", "1.21.1")
	t.Setenv("MOD_LOADER", "fabric")

	requirementsFile := filepath.Join(t.TempDir(), "requirements.txt")
	if err := os.WriteFile(requirementsFile, []byte("# only comments\n\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	modsDir := filepath.Join(t.TempDir(), "mods")
	if err := os.MkdirAll(modsDir, 0o755); err != nil {
		t.Fatal(err)
	}

	out, _, err := runCLI(t, "--cache-dir", t.TempDir(), "--dir", modsDir, "install", "-r", requirementsFile)
	if err != nil {
		t.Fatalf("install -r failed: %v", err)
	}
	if !strings.Contains(out, "installed 0, skipped 0, failed 0") {
		t.Fatalf("install -r summary = %q", out)
	}

	if _, _, err := runCLI(t, "--cache-dir", t.TempDir(), "--dir", modsDir, "install", "-r", filepath.Join(t.TempDir(), "missing.txt")); err == nil {
		t.Fatal("install -r succeeded with a missing requirements file")
	}
}

func TestInstallRequirementsRejectsIncompatibleFlags(t *testing.T) {
	isolateGlobalState(t)
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("MINECRAFT_VERSION", "1.21.1")
	t.Setenv("MOD_LOADER", "fabric")

	requirementsFile := filepath.Join(t.TempDir(), "requirements.txt")
	if err := os.WriteFile(requirementsFile, []byte("modrinth:sodium\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	modsDir := filepath.Join(t.TempDir(), "mods")
	if err := os.MkdirAll(modsDir, 0o755); err != nil {
		t.Fatal(err)
	}

	cases := [][]string{
		{"install", "-r", requirementsFile, "sodium"},
		{"install", "-r", requirementsFile, "--platform", "modrinth"},
		{"install", "-r", requirementsFile, "--server"},
		{"install", "-r", requirementsFile, "--dry-run"},
		{"install", "-r", requirementsFile, "--version-id", "abc"},
	}
	for _, args := range cases {
		full := append([]string{"--cache-dir", t.TempDir(), "--dir", modsDir}, args...)
		if _, _, err := runCLI(t, full...); err == nil || !strings.Contains(err.Error(), "cannot be combined") {
			t.Fatalf("%v error = %v, want a cannot-be-combined error", args, err)
		}
	}
}
