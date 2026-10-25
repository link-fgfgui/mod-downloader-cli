package uistate

import (
	"testing"

	"github.com/link-fgfgui/mod-downloader-core/models"
	"github.com/link-fgfgui/mod-downloader-core/storage"
	appstructs "github.com/link-fgfgui/mod-downloader-core/structs"
	structs "github.com/link-fgfgui/mod-downloader-core/structs/minecraft"
)

func project(id string) models.ModProject {
	return models.ModProject{ID: id, Platform: "modrinth", ProjectID: id}
}

func TestSearchPaginationAppend(t *testing.T) {
	state := SearchState{SelectedVersion: "1.20.1", SelectedModLoader: "Fabric", HasSelectedInstance: true}

	reqID, offset, ok := state.BeginSearch(false)
	if !ok || offset != 0 {
		t.Fatalf("first search should proceed at offset 0, got ok=%v offset=%d", ok, offset)
	}
	page1 := make([]models.ModProject, 0, SearchPageSize)
	for i := 0; i < SearchPageSize; i++ {
		page1 = append(page1, project("a"+string(rune('0'+i))))
	}
	state.ApplySearchUpdate(appstructs.SearchModsUpdate{RequestID: reqID, Results: page1, Loading: false})
	if len(state.SearchResults) != SearchPageSize || !state.HasMoreResults {
		t.Fatalf("full first page must set hasMoreResults: %#v", state)
	}
	state.AdvanceOffset(reqID, 0)

	// Append must merge onto the base snapshot.
	reqID2, offset2, ok2 := state.BeginSearch(true)
	if !ok2 || offset2 != SearchPageSize {
		t.Fatalf("append search should proceed at offset %d, got ok=%v offset=%d", SearchPageSize, ok2, offset2)
	}
	page2 := []models.ModProject{project("tail")}
	state.ApplySearchUpdate(appstructs.SearchModsUpdate{RequestID: reqID2, Results: page2, Append: true, Loading: false})
	if len(state.SearchResults) != SearchPageSize+1 {
		t.Fatalf("append should merge onto the base page, got %d", len(state.SearchResults))
	}
	if state.SearchResults[0].ID != page1[0].ID || state.SearchResults[SearchPageSize].ID != "tail" {
		t.Fatalf("append order wrong: %#v", state.SearchResults)
	}
	if state.HasMoreResults {
		t.Fatal("short append page must clear hasMoreResults")
	}
}

func TestSearchRaceDiscard(t *testing.T) {
	state := SearchState{SelectedVersion: "1.20.1", SelectedModLoader: "Fabric", HasSelectedInstance: true}
	staleID, _, _ := state.BeginSearch(false)

	// A newer search replaces the active request id.
	freshID, _, _ := state.BeginSearch(false)
	if staleID == freshID {
		t.Fatal("request ids must differ")
	}
	state.ApplySearchUpdate(appstructs.SearchModsUpdate{RequestID: freshID, Results: []models.ModProject{project("new")}})

	accepted, _ := state.ApplySearchUpdate(appstructs.SearchModsUpdate{RequestID: staleID, Results: []models.ModProject{project("old")}})
	if accepted {
		t.Fatal("stale request must be discarded")
	}
	if len(state.SearchResults) != 1 || state.SearchResults[0].ID != "new" {
		t.Fatalf("stale update must not overwrite results: %#v", state.SearchResults)
	}
}

func TestSearchAppendGuardWhenNoMoreResults(t *testing.T) {
	state := SearchState{SelectedVersion: "1.20.1", SelectedModLoader: "Fabric", HasSelectedInstance: true, HasMoreResults: false}
	if _, _, ok := state.BeginSearch(true); ok {
		t.Fatal("append must be blocked when no more results remain")
	}
}

func TestSearchMissingTupleBlocksSearch(t *testing.T) {
	state := SearchState{SearchResults: []models.ModProject{project("x")}, HasMoreResults: true}
	if _, _, ok := state.BeginSearch(false); ok {
		t.Fatal("search must not proceed without target tuple")
	}
	if state.HasMoreResults || len(state.SearchResults) != 0 {
		t.Fatalf("state should be reset on blocked search: %#v", state)
	}
}

func TestShowResultsResetsPaging(t *testing.T) {
	state := SearchState{HasMoreResults: true, NextSearchOffset: 20, IsSearching: true, ActiveSearchRequestID: "req-1"}
	state.ShowResults([]models.ModProject{project("snap")})
	if state.SearchText != "" || state.HasMoreResults || state.NextSearchOffset != 0 || state.IsSearching || state.ActiveSearchRequestID != "" {
		t.Fatalf("showResults must reset paging state: %#v", state)
	}
	if len(state.SearchResults) != 1 {
		t.Fatalf("snapshot not applied: %#v", state.SearchResults)
	}
}

func TestQueueMembershipDiff(t *testing.T) {
	state := SearchState{}
	items := []appstructs.DownloadQueueItem{{ID: "1", Status: "running"}, {ID: "2", Status: "pending"}}
	if !state.ApplyQueueMembership(items) {
		t.Fatal("first membership should report a change")
	}
	if state.ApplyQueueMembership(items) {
		t.Fatal("identical membership must not report a change")
	}
	items[0].Status = "failed"
	if !state.ApplyQueueMembership(items) {
		t.Fatal("status change must report a change")
	}
}

func TestExtensionResults(t *testing.T) {
	state := SearchState{HasMoreResults: true}
	if state.ApplyExtensionResults(nil) {
		t.Fatal("empty payload must be ignored")
	}
	if !state.ApplyExtensionResults([]models.ModProject{project("e")}) {
		t.Fatal("non-empty payload must apply")
	}
	if state.HasMoreResults {
		t.Fatal("extension results must set hasMoreResults=false")
	}
}

func TestQueueBellRingsOncePerSuccessfulCycle(t *testing.T) {
	state := NewQueueState()
	active := appstructs.DownloadQueueState{Active: true, Running: 1}
	idle := appstructs.DownloadQueueState{Active: false}

	if state.ApplyQueue(active) {
		t.Fatal("no bell while queue starts")
	}
	state.NoteDownloadCompleted()
	if !state.ApplyQueue(idle) {
		t.Fatal("bell should ring once the cycle drains after a completion")
	}
	if state.ApplyQueue(idle) {
		t.Fatal("bell must not ring twice")
	}
}

func TestQueueBellSilentWithoutCompletion(t *testing.T) {
	state := NewQueueState()
	state.ApplyQueue(appstructs.DownloadQueueState{Active: true, Running: 1})
	if state.ApplyQueue(appstructs.DownloadQueueState{Active: false}) {
		t.Fatal("failure/cancel-only cycles must stay silent")
	}
}

func TestQueuePresentationAutoSwitchAndCollapse(t *testing.T) {
	state := NewQueueState()
	state.Open = true

	// Reminder-only queue -> switch to optional tab.
	state.ApplyQueue(appstructs.DownloadQueueState{
		OptionalReminders: []appstructs.OptionalDependencyReminder{{ID: "r1"}},
	})
	state.SyncPresentation()
	if state.ActiveTab != QueueTabOptional || !state.Open {
		t.Fatalf("expected optional tab open, got tab=%q open=%v", state.ActiveTab, state.Open)
	}

	// Reminders cleared while on optional -> back to downloads.
	state.ApplyQueue(appstructs.DownloadQueueState{Active: true, Pending: 1, Items: []appstructs.DownloadQueueItem{{ID: "d1"}}})
	state.SyncPresentation()
	if state.ActiveTab != QueueTabDownloads {
		t.Fatalf("expected downloads tab, got %q", state.ActiveTab)
	}

	// Nothing visible -> collapse.
	state.ApplyQueue(appstructs.DownloadQueueState{})
	state.SyncPresentation()
	if state.Open {
		t.Fatal("empty queue must collapse the drawer")
	}
}

func TestQueueReminderOnlyAndCanRemove(t *testing.T) {
	q := QueueState{Queue: appstructs.DownloadQueueState{Pending: 0, MessageCount: 2}}
	if !q.ReminderOnly() {
		t.Fatal("reminderOnly must be true with no active downloads but pending messages")
	}
	if !CanRemove("pending") || !CanRemove("failed") || !CanRemove("canceled") {
		t.Fatal("pending/failed/canceled entries must be removable")
	}
	if CanRemove("running") {
		t.Fatal("running entries must not be removable without cancel")
	}
}

func TestManageGroupingByPhysicalPath(t *testing.T) {
	mods := []structs.ModInfo{
		{ID: "sodium", FileName: "sodium.jar", Path: "/mods/sodium.jar", Enabled: true,
			JijMods: []structs.JijModInfo{{ID: "jij-a", Name: "A"}, {ID: "jij-a", Name: "A dup"}}},
		{ID: "sodium-extra", FileName: "sodium.jar", Path: "/mods/sodium.jar", Enabled: true},
	}
	groups := BuildGroups(mods)
	if len(groups) != 1 {
		t.Fatalf("expected a single grouped JAR, got %d", len(groups))
	}
	g := groups[0]
	if g.Primary.ID != "sodium" {
		t.Fatalf("primary should be first record, got %q", g.Primary.ID)
	}
	if len(g.Strong) != 2 {
		t.Fatalf("strong declarations must include both mods, got %d", len(g.Strong))
	}
	if len(g.Jij) != 1 {
		t.Fatalf("duplicate JIJ entries must be de-duplicated, got %d", len(g.Jij))
	}
}

func TestManageIncompatibleFoldingAndSelectability(t *testing.T) {
	fabric := structs.ModInfo{ID: "a", FileName: "a.jar", Loaders: []string{"fabric"}}
	forgeOnly := structs.ModInfo{ID: "b", FileName: "b.jar", Loaders: []string{"forge"}}
	groups := BuildGroups([]structs.ModInfo{fabric, forgeOnly})

	rows, incompat := PartitionAndBuildRows(groups, "Fabric", false)
	if incompat != 1 {
		t.Fatalf("expected 1 incompatible group, got %d", incompat)
	}
	if len(rows) != 2 { // compatible + toggle header, incompatible hidden while collapsed
		t.Fatalf("collapsed rows wrong: %d (%#v)", len(rows), rows)
	}
	if rows[1].Kind != RowKindIncompatibleToggle {
		t.Fatalf("last collapsed row must be the toggle, got %q", rows[1].Kind)
	}
	if rows[1].Selectable() {
		t.Fatal("toggle header must not be selectable")
	}

	rows, _ = PartitionAndBuildRows(groups, "Fabric", true)
	if len(rows) != 3 {
		t.Fatalf("expanded rows wrong: %d", len(rows))
	}
	if !rows[2].Selectable() {
		t.Fatal("incompatible mod row must be selectable when expanded")
	}
}

func TestManageFilterAndSearch(t *testing.T) {
	a := structs.ModInfo{ID: "sodium", FileName: "sodium.jar", Name: "Sodium", Enabled: true, Categories: []string{"Optimization"}}
	b := structs.ModInfo{ID: "lithium", FileName: "lithium.jar", Name: "Lithium", Enabled: false}
	groups := BuildGroups([]structs.ModInfo{a, b})

	if got := FilterGroups(groups, "optim", FilterAll); len(got) != 1 || got[0].Primary.ID != "sodium" {
		t.Fatalf("category search failed: %#v", got)
	}
	if got := FilterGroups(groups, "", FilterDisabled); len(got) != 1 || got[0].Primary.ID != "lithium" {
		t.Fatalf("disabled filter failed: %#v", got)
	}
	if got := FilterGroups(groups, "", FilterAll); len(got) != 2 {
		t.Fatalf("all filter should keep both, got %d", len(got))
	}
}

func TestManagePrimaryBatchActionAndPaths(t *testing.T) {
	group := BuildGroups([]structs.ModInfo{{ID: "a", FileName: "a.jar", Path: "/m/a.jar", Enabled: true}})
	if PrimaryBatchAction(group) != "disable" {
		t.Fatal("enabled selection should produce a disable action")
	}
	group2 := BuildGroups([]structs.ModInfo{{ID: "a", FileName: "a.jar", Path: "/m/a.jar", Enabled: false}})
	if PrimaryBatchAction(group2) != "enable" {
		t.Fatal("disabled selection should produce an enable action")
	}
	paths := SelectedGroupPaths(append(group, group2...))
	if len(paths) != 1 || paths[0] != "/m/a.jar" {
		t.Fatalf("paths must be de-duplicated: %#v", paths)
	}
}

func TestFavoriteScopeMatching(t *testing.T) {
	mod := storage.FavoriteMod{Platform: "Modrinth", ModID: "Sodium", MinecraftVersion: "1.20.1", ModLoader: "Fabric"}
	if !FavoriteMatchesScope(mod, "1.20.1", "fabric") {
		t.Fatal("scope must match case-insensitively on loader and exactly on version")
	}
	if FavoriteMatchesScope(mod, "1.20.2", "fabric") {
		t.Fatal("different MC version must not match")
	}
	if FavoriteMatchesScope(mod, "", "") {
		t.Fatal("unscoped tuple must not match")
	}
}

func TestFavoriteListPartitioning(t *testing.T) {
	lists := []storage.FavoriteList{
		{ID: "1", Name: "B", MinecraftVersion: "1.20.1", ModLoader: "Fabric", Pinned: false, SortOrder: 2},
		{ID: "2", Name: "A", MinecraftVersion: "1.20.1", ModLoader: "Fabric", Pinned: false, SortOrder: 1},
		{ID: "3", Name: "Pinned", Pinned: true},
		{ID: "4", Name: "Other", MinecraftVersion: "1.19", ModLoader: "Fabric"},
	}
	pinned, ungrouped := PartitionLists(lists, "1.20.1", "fabric")
	if len(pinned) != 1 || pinned[0].ID != "3" {
		t.Fatalf("pinned partition wrong: %#v", pinned)
	}
	if len(ungrouped) != 2 || ungrouped[0].ID != "2" || ungrouped[1].ID != "1" {
		t.Fatalf("ungrouped ordering wrong: %#v", ungrouped)
	}
}

func TestAPIKeySentinelResolution(t *testing.T) {
	req := BuildSaveApiKeysRequest(KeyDraft{}, KeyDraft{Value: "new-mr"})
	if req.CurseforgeApiKey != APIKeySentinel {
		t.Fatalf("untouched curseforge key must keep sentinel, got %q", req.CurseforgeApiKey)
	}
	if req.ModrinthApiKey != "new-mr" {
		t.Fatalf("edited modrinth key must be sent, got %q", req.ModrinthApiKey)
	}
	cleared := BuildSaveApiKeysRequest(KeyDraft{Clear: true}, KeyDraft{Clear: true})
	if cleared.CurseforgeApiKey != "" || cleared.ModrinthApiKey != "" {
		t.Fatalf("clear must send empty strings: %#v", cleared)
	}
}

func TestSettingsDraftClamps(t *testing.T) {
	view := appstructs.SettingsView{
		FileConcurrency:       99,
		ConcurrentDownloads:   0,
		TargetDownloadRateMiB: 9.9,
		RequestsPerSecond:     -3,
		Language:              "",
	}
	drafts := LoadSettingsDrafts(view)
	if drafts.FileConcurrency != 32 {
		t.Fatalf("file concurrency must clamp to 32, got %d", drafts.FileConcurrency)
	}
	if drafts.ConcurrentDownloads == 0 {
		t.Fatal("zero concurrent downloads must fall back to the default")
	}
	if drafts.TargetDownloadRateMiB != 5.0 {
		t.Fatalf("target rate must clamp to 5.0, got %v", drafts.TargetDownloadRateMiB)
	}
	if drafts.RequestsPerSecond != 0 {
		t.Fatalf("negative rps must clamp to 0, got %d", drafts.RequestsPerSecond)
	}
	if drafts.Language != "system" {
		t.Fatalf("empty language must default to system, got %q", drafts.Language)
	}
	net := ValidateNetworkSettings(SettingsDrafts{TargetDownloadRateMiB: 1.234, FileConcurrency: 4})
	if net.TargetDownloadRateMiB != 1.2 {
		t.Fatalf("target rate must round to 1 decimal, got %v", net.TargetDownloadRateMiB)
	}
}

func TestMinecraftApplySelectedVersion(t *testing.T) {
	state := NewMinecraftState()
	state.ApplySelectedVersion(&structs.VersionInfo{ID: "1.20.1-fabric", MinecraftVersion: "1.20.1", ModLoader: "fabric"})
	if state.SelectedMinecraftVersion != "1.20.1" || state.SelectedModLoader != "Fabric" {
		t.Fatalf("apply should sync version+loader: %#v", state)
	}
	found := false
	for _, v := range state.ReleaseVersions {
		if v == "1.20.1" {
			found = true
		}
	}
	if !found {
		t.Fatal("unseen MC version must be prepended to release versions")
	}

	state.SetSelectedMinecraftVersion("1.21")
	if state.SelectedVersion != nil || state.SelectedMinecraftVersion != "1.21" {
		t.Fatalf("manual tuple edit must clear the instance: %#v", state)
	}
	if MatchingModLoader("neoforge") != "NeoForge" {
		t.Fatal("loader matching must be case-insensitive")
	}
}
