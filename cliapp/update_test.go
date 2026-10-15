package cliapp

import (
	"testing"

	"github.com/link-fgfgui/mod-downloader-core/models"
)

func sampleUpdates() []modUpdate {
	return []modUpdate{
		{ID: "sodium", Name: "Sodium", FileName: "sodium.jar", Outdated: true},
		{ID: "lithium", Name: "Lithium", FileName: "lithium.jar", Outdated: false},
		{ID: "iris", Name: "Iris", FileName: "iris.jar", Outdated: true},
	}
}

func TestFilterOutdated(t *testing.T) {
	outdated := filterOutdated(sampleUpdates())
	if len(outdated) != 2 {
		t.Fatalf("outdated = %#v, want 2", outdated)
	}
}

func TestSelectUpgradeTargets(t *testing.T) {
	all := selectUpgradeTargets(sampleUpdates(), nil, true)
	if len(all) != 2 {
		t.Fatalf("--all selected %d, want 2", len(all))
	}

	byName := selectUpgradeTargets(sampleUpdates(), []string{"iris"}, false)
	if len(byName) != 1 || byName[0].ID != "iris" {
		t.Fatalf("by name selected %#v", byName)
	}

	byFile := selectUpgradeTargets(sampleUpdates(), []string{"sodium.jar"}, false)
	if len(byFile) != 1 || byFile[0].ID != "sodium" {
		t.Fatalf("by file selected %#v", byFile)
	}

	// Non-outdated mods are never selected.
	if got := selectUpgradeTargets(sampleUpdates(), []string{"lithium"}, false); len(got) != 0 {
		t.Fatalf("non-outdated selected %#v", got)
	}
}

func TestBuildUpgradePlansKeepDisabled(t *testing.T) {
	updates := []modUpdate{
		{ID: "sodium", Name: "Sodium", Path: "/mods/sodium.jar.disabled", LatestFileName: "sodium-0.6.jar", LatestVersion: "0.6", CurrentVersion: "0.5"},
	}
	plans := buildUpgradePlans(updates)
	if len(plans) != 1 {
		t.Fatalf("plans = %#v", plans)
	}
	if !plans[0].KeepDisabled {
		t.Fatal("disabled state not preserved")
	}
	if plans[0].OldFile != "sodium.jar.disabled" || plans[0].NewFile != "sodium-0.6.jar" {
		t.Fatalf("plan files = %#v", plans[0])
	}
}

func TestOnlineProjectForMod(t *testing.T) {
	mod := modInfo("sodium", "0.5.8", "/mods/sodium.jar")
	mod.OnlinePlatform = "Modrinth"
	mod.OnlineSlug = "sodium"
	mod.OnlineProjectID = "AANobbMI"
	mod.OnlineName = "Sodium"
	project, platform, ok := onlineProjectForMod(mod, nil)
	if !ok || platform != "modrinth" || project.Slug != "sodium" {
		t.Fatalf("online metadata project = %#v platform=%q ok=%v", project, platform, ok)
	}

	byHash := modInfo("iris", "1.7", "/mods/iris.jar")
	byHash.SHA1 = "abc123"
	resolved := map[string]models.ModProject{
		"abc123": {ID: "curseforge:123", Platform: "CurseForge", ProjectID: "123", Slug: "iris"},
	}
	project, platform, ok = onlineProjectForMod(byHash, resolved)
	if !ok || platform != "curseforge" || project.ProjectID != "123" {
		t.Fatalf("hash-resolved project = %#v platform=%q ok=%v", project, platform, ok)
	}

	unknown := modInfo("custom", "1.0", "/mods/custom.jar")
	if _, _, ok := onlineProjectForMod(unknown, nil); ok {
		t.Fatal("unmapped mod resolved unexpectedly")
	}
}

func TestCollectModUpdatesWithoutProjects(t *testing.T) {
	old := resolveProjectsByHashesFn
	resolveProjectsByHashesFn = func([]string) map[string]models.ModProject { return nil }
	t.Cleanup(func() { resolveProjectsByHashesFn = old })

	updates := collectModUpdates(nil, nil, "1.21.1", "fabric", "")
	if len(updates) != 0 {
		t.Fatalf("updates = %#v, want empty", updates)
	}
}
