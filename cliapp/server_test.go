package cliapp

import (
	"bytes"
	"strings"
	"testing"

	"github.com/link-fgfgui/mod-downloader/core/appcore"
	"github.com/link-fgfgui/mod-downloader/core/models"
	appstructs "github.com/link-fgfgui/mod-downloader/core/structs"
	appview "github.com/link-fgfgui/mod-downloader/core/structs/view"
)

func stubQueueModDownload(t *testing.T) *appstructs.ModDownloadRequest {
	t.Helper()
	orig := queueModDownloadFn
	t.Cleanup(func() { queueModDownloadFn = orig })

	var queued appstructs.ModDownloadRequest
	queueModDownloadFn = func(_ *appcore.Service, req appstructs.ModDownloadRequest) appstructs.ModDownloadResult {
		queued = req
		return appstructs.ModDownloadResult{Queued: true, QueueID: "q1", FileName: "sodium.jar", VersionID: "ver-1"}
	}
	return &queued
}

func TestServerAcceptQueuesSlugPayload(t *testing.T) {
	installSeams(t)
	lookupProjectBySlugFn = func(_ *appcore.Service, platform, slug, _, _ string) (models.ModProject, bool) {
		if platform == platformModrinth && slug == "sodium" {
			return projectRef(platform, "AANobbMI", slug, "Sodium", 1), true
		}
		return models.ModProject{}, false
	}
	lookupProjectByIDFn = func(*appcore.Service, string, string, string, string) (models.ModProject, bool) {
		return models.ModProject{}, false
	}
	queued := stubQueueModDownload(t)

	var stdout, stderr bytes.Buffer
	r := runner{stdout: &stdout, stderr: &stderr}
	svc := appcore.New(appcore.Options{})
	runtime := runtimeInput{TargetModsDir: "/mods", MinecraftVersion: "1.21.1", ModLoader: "fabric"}

	results := r.acceptServerPayloads(svc, runtime, []appstructs.ExtensionProjectPayload{{
		PlatformCode: "mr",
		Slug:         "sodium",
		VersionID:    "ver-1",
	}})

	if len(results) != 1 || !results[0].Queued || results[0].QueueID != "q1" {
		t.Fatalf("results = %#v, want one queued result", results)
	}
	if queued.TargetDir != "/mods" || queued.MinecraftVersion != "1.21.1" || queued.ModLoader != "fabric" {
		t.Fatalf("queued runtime = %#v", queued)
	}
	if queued.VersionID != "ver-1" {
		t.Fatalf("queued version = %q, want payload file", queued.VersionID)
	}
	if queued.ProjectID != "modrinth:AANobbMI" || queued.Result.Slug != "sodium" {
		t.Fatalf("queued project = %#v", queued)
	}
	if !strings.Contains(stdout.String(), "queued Sodium") {
		t.Fatalf("stdout = %q", stdout.String())
	}
}

func TestServerAcceptPrefersExplicitID(t *testing.T) {
	installSeams(t)
	idLookups := 0
	lookupProjectByIDFn = func(_ *appcore.Service, platform, projectID, _, _ string) (models.ModProject, bool) {
		idLookups++
		if platform == platformCurseforge && projectID == "394468" {
			return projectRef(platform, projectID, "jei", "JEI", 1), true
		}
		return models.ModProject{}, false
	}
	lookupProjectBySlugFn = func(*appcore.Service, string, string, string, string) (models.ModProject, bool) {
		t.Fatal("slug lookup must not run when an explicit ID is present")
		return models.ModProject{}, false
	}
	queued := stubQueueModDownload(t)

	var stdout, stderr bytes.Buffer
	r := runner{stdout: &stdout, stderr: &stderr}
	svc := appcore.New(appcore.Options{})
	id := "394468"
	results := r.acceptServerPayloads(svc, runtimeInput{TargetModsDir: "/mods", MinecraftVersion: "1.21.1", ModLoader: "forge"}, []appstructs.ExtensionProjectPayload{{
		PlatformCode: "cf",
		ID:           &id,
		Slug:         "ignored",
	}})

	if len(results) != 1 || !results[0].Queued {
		t.Fatalf("results = %#v", results)
	}
	if idLookups != 1 {
		t.Fatalf("ID lookups = %d, want 1", idLookups)
	}
	if queued.ProjectID != "curseforge:394468" {
		t.Fatalf("queued project = %#v", queued)
	}
}

func TestServerAcceptFallsBackToSearch(t *testing.T) {
	installSeams(t)
	lookupProjectBySlugFn = func(*appcore.Service, string, string, string, string) (models.ModProject, bool) {
		return models.ModProject{}, false
	}
	lookupProjectByIDFn = func(*appcore.Service, string, string, string, string) (models.ModProject, bool) {
		return models.ModProject{}, false
	}
	searchModsTextFn = func(_ *appcore.Service, req appstructs.SearchModsRequest) appview.SearchModsUpdate {
		if req.Query != "mini map" {
			t.Fatalf("search query = %q", req.Query)
		}
		return appview.SearchModsUpdate{Results: []models.ModProject{
			projectRef(platformModrinth, "minimap", "minimap", "MiniMap", 100),
		}}
	}
	stubQueueModDownload(t)

	var stdout, stderr bytes.Buffer
	r := runner{stdout: &stdout, stderr: &stderr}
	svc := appcore.New(appcore.Options{})
	results := r.acceptServerPayloads(svc, runtimeInput{TargetModsDir: "/mods", MinecraftVersion: "1.21.1", ModLoader: "fabric"}, []appstructs.ExtensionProjectPayload{{
		PlatformCode: "mr",
		Slug:         "mini map",
	}})

	if len(results) != 1 || !results[0].Queued {
		t.Fatalf("results = %#v", results)
	}
}

func TestServerAcceptRejectsInvalidPayloads(t *testing.T) {
	installSeams(t)
	lookupProjectBySlugFn = func(*appcore.Service, string, string, string, string) (models.ModProject, bool) {
		return models.ModProject{}, false
	}
	lookupProjectByIDFn = func(*appcore.Service, string, string, string, string) (models.ModProject, bool) {
		return models.ModProject{}, false
	}
	searchModsTextFn = func(*appcore.Service, appstructs.SearchModsRequest) appview.SearchModsUpdate {
		return appview.SearchModsUpdate{}
	}
	origQueue := queueModDownloadFn
	t.Cleanup(func() { queueModDownloadFn = origQueue })
	queueModDownloadFn = func(*appcore.Service, appstructs.ModDownloadRequest) appstructs.ModDownloadResult {
		t.Fatal("queue must not be called for invalid payloads")
		return appstructs.ModDownloadResult{}
	}

	var stdout, stderr bytes.Buffer
	r := runner{stdout: &stdout, stderr: &stderr}
	svc := appcore.New(appcore.Options{})
	results := r.acceptServerPayloads(svc, runtimeInput{TargetModsDir: "/mods", MinecraftVersion: "1.21.1", ModLoader: "fabric"}, []appstructs.ExtensionProjectPayload{
		{PlatformCode: "xx", Slug: "sodium"},
		{PlatformCode: "mr"},
		{PlatformCode: "mr", Slug: "missing"},
	})

	if len(results) != 3 {
		t.Fatalf("results = %#v", results)
	}
	wants := []string{"invalid platform", "empty project id and slug", "no matching project found"}
	for i, want := range wants {
		if !results[i].Skipped || !strings.Contains(results[i].Reason, want) {
			t.Fatalf("result[%d] = %#v, want skipped containing %q", i, results[i], want)
		}
	}
}
