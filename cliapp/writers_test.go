package cliapp

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/link-fgfgui/mod-downloader-core/appcore"
	"github.com/link-fgfgui/mod-downloader-core/models"
	structs "github.com/link-fgfgui/mod-downloader-core/structs/minecraft"
)

// TestManifestAndUpdateWriters covers the remaining human-readable writers that
// the command tests only reach through JSON or error paths.
func TestManifestAndUpdateWriters(t *testing.T) {
	var buf bytes.Buffer

	if err := writeManifestEntries(&buf, []manifestEntry{
		{Platform: "modrinth", Project: "sodium", Version: "0.5.8"},
		{LocalFile: "custom-addon.jar"},
	}); err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"modrinth:sodium==0.5.8", "# local: custom-addon.jar"} {
		if !strings.Contains(buf.String(), want) {
			t.Fatalf("manifest writer missing %q:\n%s", want, buf.String())
		}
	}

	buf.Reset()
	if err := writeImportResults(&buf, []importResult{
		{Line: "modrinth:sodium", Status: "installed", Note: "sodium.jar"},
		{Line: "modrinth:lithium", Status: "skipped", Note: "already installed"},
		{Line: "modrinth:iris", Status: "failed", Note: "no route"},
	}); err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		"installed: modrinth:sodium (sodium.jar)",
		"skipped: modrinth:lithium (already installed)",
		"failed: modrinth:iris (no route)",
		"installed 1, skipped 1, failed 1",
	} {
		if !strings.Contains(buf.String(), want) {
			t.Fatalf("import writer missing %q:\n%s", want, buf.String())
		}
	}

	buf.Reset()
	if err := writeOutdated(&buf, []modUpdate(nil)); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(buf.String(), "all mods are up to date") {
		t.Fatalf("empty outdated output = %q", buf.String())
	}
	buf.Reset()
	if err := writeOutdated(&buf, []modUpdate{{
		ID: "sodium", Name: "Sodium", CurrentVersion: "0.5.8",
		LatestVersion: "0.6.0", Platform: "modrinth",
	}}); err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"CURRENT_VERSION", "LATEST_VERSION", "sodium", "0.6.0"} {
		if !strings.Contains(buf.String(), want) {
			t.Fatalf("outdated output missing %q:\n%s", want, buf.String())
		}
	}

	buf.Reset()
	if err := writeUpgradePlans(&buf, []upgradePlan{
		{ID: "a", Name: "A", CurrentVersion: "1", LatestVersion: "2", OldFile: "a.jar", NewFile: "a-new.jar", Executed: true},
		{ID: "b", Name: "B", CurrentVersion: "1", LatestVersion: "2", OldFile: "b.jar", NewFile: "b-new.jar"},
		{ID: "c", Name: "C", Note: "download failed"},
	}); err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"upgraded", "preview", "download failed", "STATUS"} {
		if !strings.Contains(buf.String(), want) {
			t.Fatalf("upgrade output missing %q:\n%s", want, buf.String())
		}
	}

	buf.Reset()
	if err := writeExportResult(&buf, map[string]any{"count": 2, "output": "mods.txt"}); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(buf.String(), "exported 2 mods to mods.txt") {
		t.Fatalf("export result = %q", buf.String())
	}
}

// TestVerifyReportWriter covers the passing and failing text reports.
func TestVerifyReportWriter(t *testing.T) {
	var buf bytes.Buffer
	if err := writeVerifyReport(&buf, verifyReport{Checked: 3, Passed: true}); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(buf.String(), "checked 3 jars, all checks passed") {
		t.Fatalf("clean report = %q", buf.String())
	}

	buf.Reset()
	if err := writeVerifyReport(&buf, verifyReport{
		Checked: 2,
		Issues: []verifyIssue{
			{Kind: "missing-dependency", ModID: "fabric-api", Message: "missing required dependency", Suggest: "install modrinth:fabric-api"},
			{Kind: "incompatible", ModID: "sodium", Message: "wrong loader", Advisory: true},
		},
	}); err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		"MISSING-DEPENDENCY", "fabric-api", "install modrinth:fabric-api",
		"INCOMPATIBLE (warning)", "checked 2 jars, 2 issue(s)",
	} {
		if !strings.Contains(buf.String(), want) {
			t.Fatalf("issue report missing %q:\n%s", want, buf.String())
		}
	}
}

// TestResolveHelpers covers the pure selection and filesystem helpers.
func TestResolveHelpers(t *testing.T) {
	candidates := []models.ModProject{
		{Platform: "CurseForge", ProjectID: "cf"},
		{Platform: "Modrinth", ProjectID: "mr"},
	}
	if got := pickByPlatformPreference(candidates, ""); got.ProjectID != "mr" {
		t.Fatalf("default preference picked %q, want the Modrinth candidate", got.ProjectID)
	}
	if got := pickByPlatformPreference(candidates, "curseforge"); got.ProjectID != "cf" {
		t.Fatalf("explicit preference picked %q, want the CurseForge candidate", got.ProjectID)
	}
	if got := pickByPlatformPreference(candidates, "unknown"); got.ProjectID != "mr" {
		t.Fatalf("unknown preference picked %q, want the ranked fallback", got.ProjectID)
	}
	if got := pickByPlatformPreference([]models.ModProject{{Platform: "Other", ProjectID: "x"}}, ""); got.ProjectID != "x" {
		t.Fatalf("single candidate picked %q", got.ProjectID)
	}

	for _, value := range []string{"1", "T", "true", "YES", "y", "0", "f", "false", "No", "n"} {
		if err := validateBool("flag", value); err != nil {
			t.Fatalf("validateBool(%q) = %v", value, err)
		}
	}
	if err := validateBool("flag", "maybe"); err == nil || !strings.Contains(err.Error(), "-flag") {
		t.Fatalf("validateBool(maybe) = %v", err)
	}

	installed := installedProjectRefs([]structs.ModInfo{
		{OnlinePlatform: "modrinth", OnlineSlug: "Sodium"},
		{OnlinePlatform: "curseforge", OnlineProjectID: "Iris"},
		{OnlinePlatform: "", OnlineSlug: "skip"},
		{OnlinePlatform: "modrinth", OnlineSlug: "", OnlineProjectID: ""},
		{OnlinePlatform: "modrinth", OnlineSlug: "sodium"},
	})
	if len(installed) != 2 || !installed["modrinth:sodium"] || !installed["curseforge:iris"] {
		t.Fatalf("installedProjectRefs = %#v", installed)
	}

	dir := t.TempDir()
	filePath := filepath.Join(dir, "mod.jar")
	if err := os.WriteFile(filePath, []byte("jar"), 0o644); err != nil {
		t.Fatal(err)
	}
	if got, ok := osStatFile(filePath); !ok || got != filePath {
		t.Fatalf("osStatFile(file) = %q, %v", got, ok)
	}
	if _, ok := osStatFile(dir); ok {
		t.Fatal("osStatFile(directory) reported a file")
	}
	if _, ok := osStatFile(filepath.Join(dir, "missing.jar")); ok {
		t.Fatal("osStatFile(missing) reported a file")
	}

	if err := renameToDisabled(filePath); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filePath + ".disabled"); err != nil {
		t.Fatalf("disabled file missing: %v", err)
	}
	// Already-disabled paths are a no-op.
	if err := renameToDisabled(filePath + ".disabled"); err != nil {
		t.Fatal(err)
	}

	oldPath := filepath.Join(dir, "old.jar")
	backupPath := oldPath + ".old1"
	if err := os.WriteFile(oldPath, []byte("old"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(backupPath, []byte("old"), 0o644); err != nil {
		t.Fatal(err)
	}
	removeSupersededFiles(oldPath)
	if _, err := os.Stat(oldPath); !os.IsNotExist(err) {
		t.Fatalf("old jar still exists: %v", err)
	}
	if _, err := os.Stat(backupPath); !os.IsNotExist(err) {
		t.Fatalf("old backup still exists: %v", err)
	}
	removeSupersededFiles("")
}

// TestInfoCommandLocalJar runs the info command on a local file: no online
// lookup is involved because the installed metadata is absent and the cache is
// empty.
func TestInfoCommandLocalJar(t *testing.T) {
	isolateGlobalState(t)
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())

	jar := writeModJar(t, t.TempDir(), "sodium.jar", fabricJarEntries("sodium", "0.5.8", `{"fabric-api":"*"}`))

	out, _, err := runCLI(t, "info", jar)
	if err != nil {
		t.Fatalf("info: %v", err)
	}
	for _, want := range []string{"MOD ID", "sodium", "VERSION", "0.5.8", "FILE", "SHA1", "DEPENDS", "fabric-api"} {
		if !strings.Contains(out, want) {
			t.Fatalf("info output missing %q:\n%s", want, out)
		}
	}

	out, _, err = runCLI(t, "info", "--json", jar)
	if err != nil {
		t.Fatalf("info --json: %v", err)
	}
	var view infoView
	if err := json.Unmarshal([]byte(out), &view); err != nil {
		t.Fatalf("info --json output: %v\n%s", err, out)
	}
	if view.Metadata.ID != "sodium" || view.File.SHA1 == "" || view.Online != nil {
		t.Fatalf("info view = %#v", view)
	}

	if _, _, err := runCLI(t, "info"); err == nil {
		t.Fatal("info without an argument succeeded")
	}
}

// TestInfoWriterAndURLHelpers covers the optional metadata sections and every
// online-info branch.
func TestInfoWriterAndURLHelpers(t *testing.T) {
	view := infoView{
		Metadata: jarMetadata{
			Loader: "fabric", ID: "sodium", Name: "Sodium", Version: "0.5.8",
			Description: "fast renderer", Authors: []string{"jellysquid", "other"},
			License: "LGPL-3.0", Environment: "client", MinecraftVersion: "1.20.1",
			Depends:    []jarDependency{{ID: "fabric-api", Version: "*"}},
			Recommends: []jarDependency{{ID: "indium", Version: "1.0"}},
			Breaks:     []jarDependency{{ID: "optifine"}},
		},
		File:   jarFileInfo{Path: "/mods/sodium.jar", Name: "sodium.jar", Size: 5, SHA1: "aa", SHA512: "bb"},
		Online: &projectURLInfo{Platform: "modrinth", Slug: "sodium", URL: "https://modrinth.com/mod/sodium"},
	}
	var buf bytes.Buffer
	if err := writeInfoView(&buf, view); err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		"ENVIRONMENT", "LICENSE", "AUTHORS", "jellysquid, other", "MINECRAFT",
		"DESCRIPTION", "DEPENDS", "fabric-api", "RECOMMENDS", "indium",
		"BREAKS", "optifine", "ONLINE", "https://modrinth.com/mod/sodium",
	} {
		if !strings.Contains(buf.String(), want) {
			t.Fatalf("info writer missing %q:\n%s", want, buf.String())
		}
	}

	if got := projectURL("modrinth", "sodium"); got != "https://modrinth.com/mod/sodium" {
		t.Fatalf("modrinth URL = %q", got)
	}
	if got := projectURL("CurseForge", "iris"); got != "https://www.curseforge.com/minecraft/mc-mods/iris" {
		t.Fatalf("curseforge URL = %q", got)
	}
	if got := projectURL("unknown", "x"); got != "" {
		t.Fatalf("unknown URL = %q", got)
	}
	if got := projectURL("modrinth", "  "); got != "" {
		t.Fatalf("empty slug URL = %q", got)
	}

	installed := structs.ModInfo{
		OnlinePlatform: "Modrinth", OnlineSlug: "sodium",
		OnlineProjectID: "AANobbMI", OnlineName: "Sodium",
	}
	online := onlineProjectForInfo(&installed, "sha")
	if online == nil || online.ProjectID != "AANobbMI" || online.URL != "https://modrinth.com/mod/sodium" {
		t.Fatalf("online info = %#v", online)
	}
	unknown := structs.ModInfo{OnlinePlatform: "unknown"}
	if online := onlineProjectForInfo(&unknown, "sha"); online == nil || online.URL != "" {
		t.Fatalf("unknown platform online info = %#v", online)
	}
	if got := onlineProjectForInfo(nil, ""); got != nil {
		t.Fatalf("empty hash online info = %#v", got)
	}
	if got := onlineProjectForInfo(nil, "uncached-sha1"); got != nil {
		t.Fatalf("uncached hash online info = %#v", got)
	}
}

// TestCollectModUpdatesOffline feeds unknown platforms to the update collector:
// providers return no versions, so every lookup stays local.
func TestCollectModUpdatesOffline(t *testing.T) {
	isolateGlobalState(t)
	old := resolveProjectsByHashesFn
	resolveProjectsByHashesFn = func(hashes []string) map[string]models.ModProject {
		return map[string]models.ModProject{
			"hash-2": {Platform: "unknown", ProjectID: "mystery", Slug: "mystery"},
		}
	}
	t.Cleanup(func() { resolveProjectsByHashesFn = old })

	svc := &appcore.Service{}
	mods := []structs.ModInfo{
		{ID: "a", Name: "A", Version: "1.0", Path: "/mods/a.jar", SHA1: "HASH-2"},
		{ID: "b", Name: "B", Path: "/mods/a.jar"},
		{ID: "c", Name: "C", OnlinePlatform: "unknown", OnlineSlug: "c"},
		{ID: "d", Name: "D", Version: "2.0", Path: "/mods/d.jar", OnlinePlatform: "unknown", OnlineSlug: "d", SHA1: "sha-d"},
	}

	updates := collectModUpdates(svc, mods, "1.20.1", "fabric", "")
	if len(updates) != 2 {
		t.Fatalf("collectModUpdates returned %d updates: %#v", len(updates), updates)
	}
	byID := map[string]modUpdate{}
	for _, update := range updates {
		byID[update.ID] = update
	}
	if got := byID["a"]; got.Platform != "unknown" || got.CurrentVersion != "1.0" || got.Outdated {
		t.Fatalf("update a = %#v", got)
	}
	if got := byID["d"]; got.Platform != "unknown" || got.CurrentVersion != "2.0" {
		t.Fatalf("update d = %#v", got)
	}

	if filtered := collectModUpdates(svc, mods, "1.20.1", "fabric", "modrinth"); len(filtered) != 0 {
		t.Fatalf("platform filter kept %#v", filtered)
	}
}
