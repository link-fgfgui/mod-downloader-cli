package cliapp

import (
	"strings"
	"testing"

	"github.com/link-fgfgui/mod-downloader-core/appcore"
	"github.com/link-fgfgui/mod-downloader-core/global"
	"github.com/link-fgfgui/mod-downloader-core/models"
	appstructs "github.com/link-fgfgui/mod-downloader-core/structs"
)

// installSeams restores the real (network-bound) resolver seams after a test.
func installSeams(t *testing.T) {
	t.Helper()
	origID, origSlug, origSearch, origVersions, origModIDs, origPaths, origInstance :=
		lookupProjectByIDFn, lookupProjectBySlugFn, searchModsTextFn,
		listMatchingVersionsFn, versionModIDsFn, localModPathsFn, instanceIDForTargetDirFn
	t.Cleanup(func() {
		lookupProjectByIDFn = origID
		lookupProjectBySlugFn = origSlug
		searchModsTextFn = origSearch
		listMatchingVersionsFn = origVersions
		versionModIDsFn = origModIDs
		localModPathsFn = origPaths
		instanceIDForTargetDirFn = origInstance
	})
}

func projectRef(platform, projectID, slug, title string, downloads int64) models.ModProject {
	return models.ModProject{
		ID:        models.ProjectKey(platform, projectID),
		Platform:  platform,
		ProjectID: projectID,
		Slug:      slug,
		Title:     title,
		Downloads: downloads,
	}
}

func TestResolverDisambiguatesMultipleExactMatchesNonInteractively(t *testing.T) {
	installSeams(t)
	mr := projectRef("modrinth", "AANobbMI", "sodium", "Sodium", 1200000)
	cf := projectRef("curseforge", "394468", "sodium", "Sodium", 850000)

	lookupProjectBySlugFn = func(_ *appcore.Service, platform, slug, _, _ string) (models.ModProject, bool) {
		if slug != "sodium" {
			return models.ModProject{}, false
		}
		switch platform {
		case "modrinth":
			return mr, true
		case "curseforge":
			return cf, true
		}
		return models.ModProject{}, false
	}
	lookupProjectByIDFn = func(*appcore.Service, string, string, string, string) (models.ModProject, bool) {
		return models.ModProject{}, false
	}
	// The generic fallback must never be hit: both platform lookups succeed above,
	// but if the slug lookup misses we return a fake to keep the test offline.

	svc := appcore.New(appcore.Options{})
	target, err := ParseInstallTarget("sodium", "")
	if err != nil {
		t.Fatal(err)
	}
	chosen, err := resolveInstallTarget(svc, target, runtimeInput{MinecraftVersion: "1.21.1", ModLoader: "fabric"}, "", false)
	if err != nil {
		t.Fatalf("resolve error: %v", err)
	}
	if len(chosen) != 1 {
		t.Fatalf("expected exactly one non-interactive pick, got %d", len(chosen))
	}
	if chosen[0].Platform != "modrinth" {
		t.Fatalf("non-interactive pick should prefer Modrinth, got %q", chosen[0].Platform)
	}
}

func TestResolverHonoursPlatformFilterInDisambiguation(t *testing.T) {
	installSeams(t)
	cf := projectRef("curseforge", "394468", "sodium", "Sodium", 850000)
	lookupProjectBySlugFn = func(_ *appcore.Service, platform, slug, _, _ string) (models.ModProject, bool) {
		if platform == "curseforge" && slug == "sodium" {
			return cf, true
		}
		return models.ModProject{}, false
	}
	svc := appcore.New(appcore.Options{})
	target, _ := ParseInstallTarget("sodium", "")
	chosen, err := resolveInstallTarget(svc, target, runtimeInput{MinecraftVersion: "1.21.1", ModLoader: "fabric"}, "curseforge", false)
	if err != nil {
		t.Fatalf("resolve error: %v", err)
	}
	if len(chosen) != 1 || chosen[0].Platform != "curseforge" {
		t.Fatalf("platform filter should force curseforge, got %#v", chosen)
	}
}

func TestResolverSinglePlatformPrefixSkipsCrossPlatform(t *testing.T) {
	installSeams(t)
	calledPlatforms := []string{}
	lookupProjectBySlugFn = func(_ *appcore.Service, platform, slug, _, _ string) (models.ModProject, bool) {
		calledPlatforms = append(calledPlatforms, platform)
		return projectRef(platform, "id-"+slug, slug, slug, 1), true
	}
	svc := appcore.New(appcore.Options{})
	target, _ := ParseInstallTarget("modrinth:sodium", "")
	if _, err := resolveInstallTarget(svc, target, runtimeInput{MinecraftVersion: "1.21.1", ModLoader: "fabric"}, "", false); err != nil {
		t.Fatalf("resolve error: %v", err)
	}
	if len(calledPlatforms) != 1 || calledPlatforms[0] != "modrinth" {
		t.Fatalf("platform prefix must restrict lookups to modrinth, got %v", calledPlatforms)
	}
}

func TestResolverFallsBackToSearchAndErrorsNonInteractive(t *testing.T) {
	installSeams(t)
	lookupProjectBySlugFn = func(*appcore.Service, string, string, string, string) (models.ModProject, bool) {
		return models.ModProject{}, false
	}
	searched := false
	searchModsTextFn = func(_ *appcore.Service, req appstructs.SearchModsRequest) appstructs.SearchModsUpdate {
		searched = true
		if req.Query != "mini map" {
			t.Fatalf("search query = %q, want %q", req.Query, "mini map")
		}
		return appstructs.SearchModsUpdate{Results: []models.ModProject{
			projectRef("modrinth", "1", "minimap", "MiniMap", 100),
			projectRef("curseforge", "2", "mini-map", "Mini Map", 90),
		}}
	}
	svc := appcore.New(appcore.Options{})
	target, _ := ParseInstallTarget("mini map", "")
	_, err := resolveInstallTarget(svc, target, runtimeInput{MinecraftVersion: "1.21.1", ModLoader: "fabric"}, "", false)
	if !searched {
		t.Fatal("search fallback was not triggered")
	}
	if err == nil {
		t.Fatal("non-interactive search fallback with multiple results must error")
	}
	if !strings.Contains(err.Error(), "non-interactive") || !strings.Contains(err.Error(), "curseforge:394468") {
		t.Fatalf("guidance message missing: %v", err)
	}
}

func TestResolverSearchFallbackSingleResultAutoSelects(t *testing.T) {
	installSeams(t)
	lookupProjectBySlugFn = func(*appcore.Service, string, string, string, string) (models.ModProject, bool) {
		return models.ModProject{}, false
	}
	searchModsTextFn = func(*appcore.Service, appstructs.SearchModsRequest) appstructs.SearchModsUpdate {
		return appstructs.SearchModsUpdate{Results: []models.ModProject{
			projectRef("modrinth", "1", "appleskin", "AppleSkin", 10),
		}}
	}
	svc := appcore.New(appcore.Options{})
	target, _ := ParseInstallTarget("appleskin", "")
	chosen, err := resolveInstallTarget(svc, target, runtimeInput{MinecraftVersion: "1.21.1", ModLoader: "fabric"}, "", false)
	if err != nil {
		t.Fatalf("single search result should auto-select: %v", err)
	}
	if len(chosen) != 1 || chosen[0].Slug != "appleskin" {
		t.Fatalf("unexpected selection: %#v", chosen)
	}
}

func TestResolverNoResultsErrors(t *testing.T) {
	installSeams(t)
	lookupProjectBySlugFn = func(*appcore.Service, string, string, string, string) (models.ModProject, bool) {
		return models.ModProject{}, false
	}
	searchModsTextFn = func(*appcore.Service, appstructs.SearchModsRequest) appstructs.SearchModsUpdate {
		return appstructs.SearchModsUpdate{}
	}
	svc := appcore.New(appcore.Options{})
	target, _ := ParseInstallTarget("nope-nothing", "")
	if _, err := resolveInstallTarget(svc, target, runtimeInput{MinecraftVersion: "1.21.1", ModLoader: "fabric"}, "", false); err == nil {
		t.Fatal("expected an error when neither exact nor search matches")
	}
}

func TestBuildInstallPlanEntryEnrichesMetadata(t *testing.T) {
	installSeams(t)
	listMatchingVersionsFn = func(*appcore.Service, models.ModProject, string, string) []models.ModVersion {
		return []models.ModVersion{{
			ID:       "ver-1",
			Version:  "0.6.0",
			FileName: "sodium-fabric-0.6.0.jar",
			FileSize: 1887436,
			ModIDs:   []string{"sodium"},
		}}
	}
	versionModIDsFn = func(models.ModVersion, string) []string { return nil } // cache miss -> fall back to ModIDs
	localModPathsFn = func(modIDs []string, instanceID string) []global.LocalModFilePath {
		return []global.LocalModFilePath{{Path: "/mods/sodium.jar", InstanceID: instanceID}}
	}
	instanceIDForTargetDirFn = func(string) string { return "cli:/mods" }

	svc := appcore.New(appcore.Options{})
	entry := buildInstallPlanEntry(svc, "sodium", projectRef("modrinth", "AANobbMI", "sodium", "Sodium", 10),
		runtimeInput{MinecraftVersion: "1.21.1", ModLoader: "fabric", TargetModsDir: "/mods"})

	if entry.Version != "0.6.0" {
		t.Errorf("version = %q", entry.Version)
	}
	if len(entry.ModIDs) != 1 || entry.ModIDs[0] != "sodium" {
		t.Errorf("modIds = %v", entry.ModIDs)
	}
	if entry.FileName != "sodium-fabric-0.6.0.jar" || entry.FileSize != 1887436 {
		t.Errorf("file metadata wrong: %q %d", entry.FileName, entry.FileSize)
	}
	if entry.LocalState != "installed" {
		t.Errorf("localState = %q, want installed", entry.LocalState)
	}
}

func TestBuildInstallPlanEntryFallsBackToSlugWhenModIDsMissing(t *testing.T) {
	installSeams(t)
	listMatchingVersionsFn = func(*appcore.Service, models.ModProject, string, string) []models.ModVersion {
		return []models.ModVersion{{ID: "v1", Version: "1.0", ModIDs: nil}}
	}
	versionModIDsFn = func(models.ModVersion, string) []string { return nil }
	localModPathsFn = func([]string, string) []global.LocalModFilePath { return nil }
	instanceIDForTargetDirFn = func(string) string { return "i" }

	svc := appcore.New(appcore.Options{})
	entry := buildInstallPlanEntry(svc, "roughly-enough-items", projectRef("modrinth", "x", "roughly-enough-items", "REI", 1), runtimeInput{TargetModsDir: "/mods"})
	if len(entry.ModIDs) != 1 || entry.ModIDs[0] != "roughly-enough-items" {
		t.Fatalf("expected slug fallback, got %v", entry.ModIDs)
	}
	if entry.LocalState != "not_installed" {
		t.Fatalf("localState = %q", entry.LocalState)
	}
}
