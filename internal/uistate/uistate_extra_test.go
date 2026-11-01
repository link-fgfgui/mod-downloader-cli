package uistate

import (
	"testing"

	"github.com/link-fgfgui/mod-downloader/core/appcore"
	"github.com/link-fgfgui/mod-downloader/core/configs"
	"github.com/link-fgfgui/mod-downloader/core/models"
	"github.com/link-fgfgui/mod-downloader/core/storage"
	structs "github.com/link-fgfgui/mod-downloader/core/structs/minecraft"
	appview "github.com/link-fgfgui/mod-downloader/core/structs/view"
)

func TestDownloadErrorI18nKey(t *testing.T) {
	tests := []struct {
		name   string
		reason string
		want   string
		ok     bool
	}{
		{"invalid request", "invalid request", "invalidRequest", true},
		{"no matching version trimmed", "  no matching version  ", "noMatchingVersion", true},
		{"missing download url", "missing download url", "missingDownloadUrl", true},
		{"no selected version", " no selected version ", "noSelectedVersion", true},
		{"unknown reason", "boom", "", false},
		{"empty reason", "", "", false},
		{"whitespace only", "   ", "", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, ok := DownloadErrorI18nKey(tt.reason)
			if got != tt.want || ok != tt.ok {
				t.Fatalf("DownloadErrorI18nKey(%q) = (%q, %v), want (%q, %v)", tt.reason, got, ok, tt.want, tt.ok)
			}
		})
	}
}

func TestSetTargetTuple(t *testing.T) {
	state := SearchState{}
	if !state.SetTargetTuple("1.20.1", "Fabric", true) {
		t.Fatal("first tuple assignment must report a change")
	}
	state.DownloadStates = []appview.ModDownloadButtonState{{Key: "sodium"}}
	if state.SetTargetTuple("1.20.1", "Fabric", true) {
		t.Fatal("identical tuple must not report a change")
	}
	if len(state.DownloadStates) != 1 {
		t.Fatal("unchanged tuple must preserve download states")
	}
	if !state.SetTargetTuple("1.20.1", "Forge", true) {
		t.Fatal("loader change must report a change")
	}
	if state.DownloadStates != nil {
		t.Fatalf("changed tuple must clear download states, got %#v", state.DownloadStates)
	}
	if !state.SetTargetTuple("1.21", "Forge", false) {
		t.Fatal("version and instance change must report a change")
	}
}

func TestFinishSearch(t *testing.T) {
	state := SearchState{SelectedVersion: "1.20.1", SelectedModLoader: "Fabric", HasMoreResults: true}
	reqID, _, ok := state.BeginSearch(false)
	if !ok || !state.IsSearching {
		t.Fatalf("begin search should start a non-append load: %#v", state)
	}
	state.FinishSearch("other-request")
	if !state.IsSearching {
		t.Fatal("mismatched request must not clear the loading flag")
	}
	state.FinishSearch(reqID)
	if state.IsSearching {
		t.Fatal("matching request must clear the non-append loading flag")
	}

	appendState := SearchState{SelectedVersion: "1.20.1", SelectedModLoader: "Fabric", HasMoreResults: true}
	appendID, offset, ok := appendState.BeginSearch(true)
	if !ok || offset != 0 || !appendState.IsLoadingMore {
		t.Fatalf("begin append should start loading more: ok=%v offset=%d %#v", ok, offset, appendState)
	}
	appendState.FinishSearch(appendID)
	if appendState.IsLoadingMore || appendState.IsSearching {
		t.Fatalf("matching append request must clear only the append flag: %#v", appendState)
	}
}

func TestDownloadingKeyLifecycle(t *testing.T) {
	state := SearchState{}
	if state.BeginDownloading("") {
		t.Fatal("empty key must not begin downloading")
	}
	if !state.BeginDownloading("sodium") {
		t.Fatal("first begin for a key must succeed")
	}
	if state.BeginDownloading("sodium") {
		t.Fatal("duplicate begin for an in-flight key must fail")
	}
	state.EndDownloading("sodium")
	if !state.BeginDownloading("sodium") {
		t.Fatal("key must be begin-able again after end")
	}
	state.EndDownloading("never-started")
	if !state.DownloadingKeys["sodium"] {
		t.Fatal("ending an unknown key must not drop unrelated in-flight keys")
	}
}

func TestDisabledDownloadStates(t *testing.T) {
	state := SearchState{SearchResults: []models.ModProject{
		{ID: "modrinth:sodium"},
		{ID: "curseforge:12345"},
	}}

	states := state.DisabledDownloadStates("")
	if len(states) != 2 {
		t.Fatalf("expected one state per result, got %d", len(states))
	}
	first := states[0]
	if first.Key != "sodium" {
		t.Fatalf("key must strip the platform prefix, got %q", first.Key)
	}
	if first.Status != "noSelectedVersion" {
		t.Fatalf("empty reason must default to noSelectedVersion, got %q", first.Status)
	}
	if first.Icon != "mdi-cube-off-outline" {
		t.Fatalf("noSelectedVersion must use the cube-off icon, got %q", first.Icon)
	}
	if !first.Disabled || first.Loading {
		t.Fatalf("disabled states must be disabled and not loading: %#v", first)
	}
	if first.Color != "surface-variant" {
		t.Fatalf("unexpected color %q", first.Color)
	}

	alert := state.DisabledDownloadStates("noMatchingVersion")
	if alert[1].Status != "noMatchingVersion" || alert[1].Icon != "mdi-alert-circle-outline" {
		t.Fatalf("non-default reason must use the alert icon: %#v", alert[1])
	}

	explicit := state.DisabledDownloadStates("noSelectedVersion")
	if explicit[0].Icon != "mdi-cube-off-outline" {
		t.Fatalf("explicit noSelectedVersion must use the cube-off icon, got %q", explicit[0].Icon)
	}

	empty := (&SearchState{}).DisabledDownloadStates("")
	if len(empty) != 0 {
		t.Fatalf("no results must produce no states, got %d", len(empty))
	}
}

func TestModIDFromResult(t *testing.T) {
	tests := []struct {
		name string
		id   string
		want string
	}{
		{"platform prefix", "modrinth:sodium", "sodium"},
		{"no prefix", "sodium", "sodium"},
		{"multiple colons", "curseforge:123:456", "123:456"},
		{"empty", "", ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := ModIDFromResult(models.ModProject{ID: tt.id}); got != tt.want {
				t.Fatalf("ModIDFromResult(%q) = %q, want %q", tt.id, got, tt.want)
			}
		})
	}
}

func TestStateForResultKey(t *testing.T) {
	sodium := project("sodium")
	lithium := project("lithium")
	state := SearchState{
		SearchResults:  []models.ModProject{sodium, lithium},
		DownloadStates: []appview.ModDownloadButtonState{{Key: "sodium-key"}},
	}
	if got := state.StateForResultKey(sodium); got != "sodium-key" {
		t.Fatalf("state key at the result index must win, got %q", got)
	}
	if got := state.StateForResultKey(lithium); got != "lithium" {
		t.Fatalf("missing state must fall back to the result id, got %q", got)
	}
	unknown := models.ModProject{ID: "unknown", Platform: "modrinth", ProjectID: "unknown"}
	if got := state.StateForResultKey(unknown); got != "unknown" {
		t.Fatalf("result not in the list must fall back to its id, got %q", got)
	}

	emptyKey := SearchState{
		SearchResults:  []models.ModProject{sodium},
		DownloadStates: []appview.ModDownloadButtonState{{Key: ""}},
	}
	if got := emptyKey.StateForResultKey(sodium); got != "sodium" {
		t.Fatalf("empty state key must fall back to the result id, got %q", got)
	}
}

func TestQueueMembershipKeyAndClear(t *testing.T) {
	itemsA := []appview.DownloadQueueItem{{ID: "1", Status: "running"}, {ID: "2", Status: "pending"}}
	itemsB := []appview.DownloadQueueItem{{ID: "2", Status: "pending"}, {ID: "1", Status: "running"}}
	if QueueMembershipKey(itemsA) != QueueMembershipKey(itemsB) {
		t.Fatalf("membership key must be order independent: %q vs %q", QueueMembershipKey(itemsA), QueueMembershipKey(itemsB))
	}
	if got, want := QueueMembershipKey(itemsA), "1:running|2:pending"; got != want {
		t.Fatalf("membership key = %q, want %q", got, want)
	}
	if got := QueueMembershipKey(nil); got != "" {
		t.Fatalf("empty queue membership must be empty, got %q", got)
	}
	if QueueMembershipKey(itemsA) == QueueMembershipKey([]appview.DownloadQueueItem{{ID: "1", Status: "failed"}}) {
		t.Fatal("status changes must change the membership key")
	}

	state := SearchState{}
	if !state.ApplyQueueMembership(itemsA) {
		t.Fatal("first membership must report a change")
	}
	if state.ApplyQueueMembership(itemsB) {
		t.Fatal("same membership in different order must not report a change")
	}
	state.ClearQueueMembership()
	if !state.ApplyQueueMembership(itemsA) {
		t.Fatal("cleared membership must report a change on the next apply")
	}
}

func TestApplySearchUpdateLoading(t *testing.T) {
	state := SearchState{SelectedVersion: "1.20.1", SelectedModLoader: "Fabric"}
	reqID, _, ok := state.BeginSearch(false)
	if !ok {
		t.Fatal("search should start with a target tuple")
	}
	accepted, refresh := state.ApplySearchUpdate(appview.SearchModsUpdate{
		RequestID: reqID,
		Results:   []models.ModProject{project("sodium")},
		Loading:   true,
	})
	if !accepted {
		t.Fatal("matching request must be accepted")
	}
	if refresh {
		t.Fatal("loading update must not request a states refresh")
	}
	if !state.IsSearching || len(state.SearchResults) != 1 {
		t.Fatalf("loading update must keep searching with results applied: %#v", state)
	}
}

func TestLoadSettingsDraftsFallbacks(t *testing.T) {
	tests := []struct {
		name          string
		view          appview.SettingsView
		wantTheme     string
		wantLanguage  string
		wantRelease   string
		wantMultipler float64
	}{
		{
			name:          "empty view uses defaults",
			view:          appview.SettingsView{},
			wantTheme:     "dark",
			wantLanguage:  "system",
			wantRelease:   "latest",
			wantMultipler: configs.DefaultAnimationDurationMultiplier,
		},
		{
			name: "explicit values pass through",
			view: appview.SettingsView{
				Theme:                       "light",
				Language:                    "zh-CN",
				PreferredReleaseType:        "beta",
				AnimationDurationMultiplier: 2.5,
			},
			wantTheme:     "light",
			wantLanguage:  "zh",
			wantRelease:   "beta",
			wantMultipler: 2.5,
		},
		{
			name:          "zero multiplier falls back",
			view:          appview.SettingsView{AnimationDurationMultiplier: 0},
			wantTheme:     "dark",
			wantLanguage:  "system",
			wantRelease:   "latest",
			wantMultipler: configs.DefaultAnimationDurationMultiplier,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			drafts := LoadSettingsDrafts(tt.view)
			if drafts.Theme != tt.wantTheme {
				t.Fatalf("theme = %q, want %q", drafts.Theme, tt.wantTheme)
			}
			if drafts.Language != tt.wantLanguage {
				t.Fatalf("language = %q, want %q", drafts.Language, tt.wantLanguage)
			}
			if drafts.PreferredReleaseType != tt.wantRelease {
				t.Fatalf("release type = %q, want %q", drafts.PreferredReleaseType, tt.wantRelease)
			}
			if drafts.AnimationDurationMultiplier != tt.wantMultipler {
				t.Fatalf("multiplier = %v, want %v", drafts.AnimationDurationMultiplier, tt.wantMultipler)
			}
		})
	}
}

func TestValidateNetworkSettingsClamps(t *testing.T) {
	tests := []struct {
		name           string
		drafts         SettingsDrafts
		wantFile       int
		wantConcurrent int
		wantRPS        int
	}{
		{"zero falls back", SettingsDrafts{}, configs.DefaultFileConcurrency, configs.DefaultConcurrentDownloads, 0},
		{"file below minimum", SettingsDrafts{FileConcurrency: -2}, configs.DefaultFileConcurrency, configs.DefaultConcurrentDownloads, 0},
		{"file above maximum", SettingsDrafts{FileConcurrency: 999}, configs.MaxFileConcurrency, configs.DefaultConcurrentDownloads, 0},
		{"concurrent below minimum", SettingsDrafts{ConcurrentDownloads: -3}, configs.DefaultFileConcurrency, configs.DefaultConcurrentDownloads, 0},
		{"concurrent above maximum", SettingsDrafts{ConcurrentDownloads: 500}, configs.DefaultFileConcurrency, configs.MaxConcurrentDownloads, 0},
		{"concurrent valid", SettingsDrafts{ConcurrentDownloads: 32}, configs.DefaultFileConcurrency, 32, 0},
		{"rps below minimum", SettingsDrafts{RequestsPerSecond: -5}, configs.DefaultFileConcurrency, configs.DefaultConcurrentDownloads, 0},
		{"rps above maximum", SettingsDrafts{RequestsPerSecond: 1000}, configs.DefaultFileConcurrency, configs.DefaultConcurrentDownloads, configs.MaxRequestsPerSecond},
		{"rps valid", SettingsDrafts{RequestsPerSecond: 50}, configs.DefaultFileConcurrency, configs.DefaultConcurrentDownloads, 50},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ValidateNetworkSettings(tt.drafts)
			if got.FileConcurrency != tt.wantFile {
				t.Fatalf("file concurrency = %d, want %d", got.FileConcurrency, tt.wantFile)
			}
			if got.ConcurrentDownloads != tt.wantConcurrent {
				t.Fatalf("concurrent downloads = %d, want %d", got.ConcurrentDownloads, tt.wantConcurrent)
			}
			if got.RequestsPerSecond != tt.wantRPS {
				t.Fatalf("requests per second = %d, want %d", got.RequestsPerSecond, tt.wantRPS)
			}
		})
	}
}

func TestMaskKey(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want string
	}{
		{"empty", "", ""},
		{"short", "short", "****"},
		{"exactly eight", "12345678", "****"},
		{"long", "123456789", "1234****6789"},
		{"long trimmed", " 123456789 ", "1234****6789"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := MaskKey(tt.in); got != tt.want {
				t.Fatalf("MaskKey(%q) = %q, want %q", tt.in, got, tt.want)
			}
		})
	}
}

func TestKeyDraftResetAndResolve(t *testing.T) {
	tests := []struct {
		name  string
		draft KeyDraft
		want  string
	}{
		{"clear wins over value", KeyDraft{Value: "new-key", Clear: true}, ""},
		{"clear only", KeyDraft{Clear: true}, ""},
		{"value sent", KeyDraft{Value: "new-key"}, "new-key"},
		{"untouched keeps sentinel", KeyDraft{}, APIKeySentinel},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.draft.Resolve(); got != tt.want {
				t.Fatalf("Resolve() = %q, want %q", got, tt.want)
			}
		})
	}

	draft := KeyDraft{Value: "new-key", Clear: true}
	draft.Reset()
	if draft.Value != "" || draft.Clear {
		t.Fatalf("Reset must clear value and clear flag: %#v", draft)
	}
}

func TestFavoriteKeyNormalization(t *testing.T) {
	tests := []struct {
		name string
		mod  storage.FavoriteMod
		want string
	}{
		{
			name: "trim and lowercase",
			mod: storage.FavoriteMod{
				Platform:         " Modrinth ",
				ModID:            " Sodium ",
				MinecraftVersion: " 1.20.1 ",
				ModLoader:        " Fabric ",
			},
			want: "modrinth|sodium|1.20.1|fabric",
		},
		{
			name: "empty parts",
			mod:  storage.FavoriteMod{},
			want: "|||",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := FavoriteKey(tt.mod); got != tt.want {
				t.Fatalf("FavoriteKey() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestHasFavoriteScope(t *testing.T) {
	tests := []struct {
		version string
		loader  string
		want    bool
	}{
		{"", "", false},
		{"", "Fabric", false},
		{"1.20.1", "", false},
		{"1.20.1", "Fabric", true},
	}
	for _, tt := range tests {
		if got := HasFavoriteScope(tt.version, tt.loader); got != tt.want {
			t.Fatalf("HasFavoriteScope(%q, %q) = %v, want %v", tt.version, tt.loader, got, tt.want)
		}
	}
}

func TestManageRowKey(t *testing.T) {
	toggle := ManageRow{Kind: RowKindIncompatibleToggle}
	if got := toggle.RowKey(); got != "incompatible-toggle" {
		t.Fatalf("toggle row key = %q", got)
	}

	group := appview.LocalModGroup{RowKey: "sodium|abc|/mods/sodium.jar|sodium.jar"}
	row := ManageRow{Kind: RowKindMod, Group: group}
	if got, want := row.RowKey(), "sodium|abc|/mods/sodium.jar|sodium.jar"; got != want {
		t.Fatalf("row key = %q, want %q", got, want)
	}

	sparse := ManageRow{Kind: RowKindMod, Group: appview.LocalModGroup{RowKey: "only.jar"}}
	if got := sparse.RowKey(); got != "only.jar" {
		t.Fatalf("empty fields must be skipped, got %q", got)
	}

	if got := (ManageRow{Kind: RowKindMod}).RowKey(); got != "" {
		t.Fatalf("empty group row key = %q", got)
	}
}

func TestStrongIDsAndNames(t *testing.T) {
	group := appview.LocalModGroup{Strong: []structs.ModInfo{
		{ID: "a", Name: "Alpha"},
		{ID: "", Name: ""},
		{ID: "b", Name: ""},
	}}
	ids := StrongModIDs(group)
	if len(ids) != 2 || ids[0] != "a" || ids[1] != "b" {
		t.Fatalf("StrongModIDs() = %#v, want [a b]", ids)
	}
	names := StrongModNames(group)
	if len(names) != 2 || names[0] != "Alpha" || names[1] != "b" {
		t.Fatalf("StrongModNames() = %#v, want [Alpha b]", names)
	}
}

func TestHasGroupedDetails(t *testing.T) {
	tests := []struct {
		name  string
		group appview.LocalModGroup
		want  bool
	}{
		{"single strong without jij", appview.LocalModGroup{Strong: []structs.ModInfo{{ID: "a"}}}, false},
		{"multiple strong", appview.LocalModGroup{Strong: []structs.ModInfo{{ID: "a"}, {ID: "b"}}}, true},
		{"single strong with jij", appview.LocalModGroup{Strong: []structs.ModInfo{{ID: "a"}}, JIJ: []structs.JIJModInfo{{ID: "j"}}}, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := HasGroupedDetails(tt.group); got != tt.want {
				t.Fatalf("HasGroupedDetails() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestCanReplaceVersion(t *testing.T) {
	tests := []struct {
		name        string
		group       appview.LocalModGroup
		hasInstance bool
		want        bool
	}{
		{"no online metadata", appview.LocalModGroup{}, true, false},
		{"platform only", appview.LocalModGroup{CanReplaceVersion: false}, true, false},
		{"project only", appview.LocalModGroup{CanReplaceVersion: false}, true, false},
		{"no instance", appview.LocalModGroup{CanReplaceVersion: true}, false, false},
		{"all conditions", appview.LocalModGroup{CanReplaceVersion: true}, true, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := CanReplaceVersion(tt.group, tt.hasInstance); got != tt.want {
				t.Fatalf("CanReplaceVersion() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestCanFavoriteGroupAndSelection(t *testing.T) {
	valid := appview.LocalModGroup{Primary: structs.ModInfo{OnlinePlatform: "Modrinth", OnlineProjectID: "sodium"}}
	valid2 := appview.LocalModGroup{Primary: structs.ModInfo{OnlinePlatform: "CurseForge", OnlineProjectID: "123"}}
	missingOnline := appview.LocalModGroup{Primary: structs.ModInfo{}}

	groupTests := []struct {
		name    string
		group   appview.LocalModGroup
		version string
		loader  string
		want    bool
	}{
		{"all conditions", valid, "1.20.1", "Fabric", true},
		{"blank version", valid, "   ", "Fabric", false},
		{"blank loader", valid, "1.20.1", "  ", false},
		{"missing online metadata", missingOnline, "1.20.1", "Fabric", false},
	}
	for _, tt := range groupTests {
		t.Run("group/"+tt.name, func(t *testing.T) {
			if got := CanFavoriteGroup(tt.group, tt.version, tt.loader); got != tt.want {
				t.Fatalf("CanFavoriteGroup() = %v, want %v", got, tt.want)
			}
		})
	}

	selectionTests := []struct {
		name   string
		groups []appview.LocalModGroup
		want   bool
	}{
		{"empty selection", nil, false},
		{"all valid", []appview.LocalModGroup{valid, valid2}, true},
		{"one invalid", []appview.LocalModGroup{valid, missingOnline}, false},
	}
	for _, tt := range selectionTests {
		t.Run("selection/"+tt.name, func(t *testing.T) {
			if got := CanFavoriteSelection(tt.groups, "1.20.1", "Fabric"); got != tt.want {
				t.Fatalf("CanFavoriteSelection() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestOnlineProjectFromGroup(t *testing.T) {
	groups := appcore.BuildLocalModGroups([]structs.ModInfo{{
		ID:              "local-id",
		Name:            "Sodium Local",
		OnlineName:      "Sodium",
		OnlinePlatform:  "Modrinth",
		OnlineProjectID: "sodium",
		OnlineSlug:      "sodium-mod",
		IconURL:         "https://example.invalid/icon.png",
		Description:     "fast rendering",
		Categories:      []string{" Optimization ", "optimization", "Utility"},
	}}, "")
	group := groups[0]
	got := OnlineProjectFromGroup(group)
	if got.ID != "modrinth:sodium" {
		t.Fatalf("project id = %q, want modrinth:sodium", got.ID)
	}
	if got.Platform != "Modrinth" || got.ProjectID != "sodium" || got.Slug != "sodium-mod" {
		t.Fatalf("identity fields wrong: %#v", got)
	}
	if got.Title != "Sodium" {
		t.Fatalf("title = %q, want Sodium", got.Title)
	}
	if got.Icon != "mdi-package-variant" || got.IconURL != "https://example.invalid/icon.png" || got.Description != "fast rendering" {
		t.Fatalf("presentation fields wrong: %#v", got)
	}
	if len(got.Categories) != 2 || got.Categories[0] != "optimization" || got.Categories[1] != "utility" {
		t.Fatalf("categories = %#v, want [optimization utility]", got.Categories)
	}
}

func TestMinecraftSelectedModsKey(t *testing.T) {
	tests := []struct {
		name  string
		state MinecraftState
		want  string
	}{
		{"nil version", MinecraftState{MinecraftDir: "/game"}, ""},
		{"no dir", MinecraftState{SelectedVersion: &structs.VersionInfo{ID: "v1"}}, ""},
		{"id preferred", MinecraftState{MinecraftDir: "/game", SelectedVersion: &structs.VersionInfo{ID: "v1", Name: "Friendly"}}, "/game\x00v1"},
		{"name fallback", MinecraftState{MinecraftDir: "/game", SelectedVersion: &structs.VersionInfo{Name: "Friendly"}}, "/game\x00Friendly"},
		{"both empty", MinecraftState{MinecraftDir: "/game", SelectedVersion: &structs.VersionInfo{}}, ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.state.SelectedModsKey(); got != tt.want {
				t.Fatalf("SelectedModsKey() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestMinecraftSelectedModsLoadedGuard(t *testing.T) {
	state := MinecraftState{MinecraftDir: "/game", SelectedVersion: &structs.VersionInfo{ID: "v1"}}
	if !state.EnsureSelectedModsLoadedNeeded() {
		t.Fatal("uninitialized instance must need loading")
	}
	state.MarkSelectedModsLoaded()
	if state.EnsureSelectedModsLoadedNeeded() {
		t.Fatal("marked instance must not need loading again")
	}
	state.SelectedVersion = &structs.VersionInfo{ID: "v2"}
	if !state.EnsureSelectedModsLoadedNeeded() {
		t.Fatal("switching instance must need loading")
	}
	if (MinecraftState{}).EnsureSelectedModsLoadedNeeded() {
		t.Fatal("state without an instance must not need loading")
	}
}

func TestMinecraftHasSelectedInstance(t *testing.T) {
	tests := []struct {
		name  string
		state MinecraftState
		want  bool
	}{
		{"nil version", MinecraftState{}, false},
		{"empty version", MinecraftState{SelectedVersion: &structs.VersionInfo{}}, false},
		{"id only", MinecraftState{SelectedVersion: &structs.VersionInfo{ID: "v1"}}, true},
		{"name only", MinecraftState{SelectedVersion: &structs.VersionInfo{Name: "Pack"}}, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.state.HasSelectedInstance(); got != tt.want {
				t.Fatalf("HasSelectedInstance() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestMinecraftSelectedInstanceLabel(t *testing.T) {
	tests := []struct {
		name  string
		state MinecraftState
		want  string
	}{
		{"no version", MinecraftState{}, ""},
		{"no name or id", MinecraftState{SelectedVersion: &structs.VersionInfo{}}, ""},
		{
			name: "instance name only",
			state: MinecraftState{
				SelectedVersion:          &structs.VersionInfo{Name: "My Pack", ID: "v1"},
				SelectedMinecraftVersion: "1.20.1",
				SelectedModLoader:        "Fabric",
			},
			want: "My Pack",
		},
		{
			name: "trimmed name",
			state: MinecraftState{
				SelectedVersion: &structs.VersionInfo{Name: "  My Pack  ", ID: "v1"},
			},
			want: "My Pack",
		},
		{
			name:  "id fallback without scope",
			state: MinecraftState{SelectedVersion: &structs.VersionInfo{ID: "1.20.1-fabric"}},
			want:  "1.20.1-fabric",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.state.SelectedInstanceLabel(); got != tt.want {
				t.Fatalf("SelectedInstanceLabel() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestMinecraftMods(t *testing.T) {
	if got := (MinecraftState{}).Mods(); got != nil {
		t.Fatalf("no selected version must return nil, got %#v", got)
	}
	mods := []structs.ModInfo{{ID: "sodium", FileName: "sodium.jar"}}
	state := MinecraftState{SelectedVersion: &structs.VersionInfo{Mods: mods}}
	got := state.Mods()
	if len(got) != 1 || got[0].ID != "sodium" {
		t.Fatalf("Mods() = %#v, want the selected version's mods", got)
	}
}

func TestMinecraftSetSelectedModLoader(t *testing.T) {
	state := NewMinecraftState()
	state.SelectedVersion = &structs.VersionInfo{ID: "v1"}
	state.SetSelectedModLoader(" neoforge ")
	if state.SelectedModLoader != "NeoForge" {
		t.Fatalf("loader must normalize to NeoForge, got %q", state.SelectedModLoader)
	}
	if state.SelectedVersion != nil {
		t.Fatal("manual loader edit must clear the instance")
	}

	state.SelectedVersion = &structs.VersionInfo{ID: "v2"}
	state.SetSelectedModLoader(" quilt ")
	if state.SelectedModLoader != "quilt" {
		t.Fatalf("unknown loader must be trimmed but preserved, got %q", state.SelectedModLoader)
	}
	if state.SelectedVersion != nil {
		t.Fatal("manual loader edit must clear the instance")
	}
}

func TestQueueCountsAndVisibility(t *testing.T) {
	q := QueueState{Queue: appview.DownloadQueueState{Pending: 2, Running: 3}}
	if got := q.ActiveCount(); got != 5 {
		t.Fatalf("ActiveCount() = %d, want 5", got)
	}

	explicit := QueueState{Queue: appview.DownloadQueueState{
		MessageCount:      4,
		OptionalReminders: []appview.OptionalDependencyReminder{{ID: "r1"}, {ID: "r2"}},
	}}
	if got := explicit.MessageCount(); got != 4 {
		t.Fatalf("explicit message count must win, got %d", got)
	}
	fallback := QueueState{Queue: appview.DownloadQueueState{
		OptionalReminders: []appview.OptionalDependencyReminder{{ID: "r1"}, {ID: "r2"}},
	}}
	if got := fallback.MessageCount(); got != 2 {
		t.Fatalf("reminder length must be the fallback, got %d", got)
	}
	if got := (QueueState{}).MessageCount(); got != 0 {
		t.Fatalf("empty queue message count = %d, want 0", got)
	}

	visibleTests := []struct {
		name  string
		queue appview.DownloadQueueState
		want  bool
	}{
		{"items", appview.DownloadQueueState{Items: []appview.DownloadQueueItem{{ID: "d1"}}}, true},
		{"reminders", appview.DownloadQueueState{OptionalReminders: []appview.OptionalDependencyReminder{{ID: "r1"}}}, true},
		{"counts only", appview.DownloadQueueState{Pending: 3, MessageCount: 2}, false},
		{"empty", appview.DownloadQueueState{}, false},
	}
	for _, tt := range visibleTests {
		t.Run(tt.name, func(t *testing.T) {
			if got := (QueueState{Queue: tt.queue}).HasVisibleItems(); got != tt.want {
				t.Fatalf("HasVisibleItems() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestQueueNoteDownloadCompleted(t *testing.T) {
	state := NewQueueState()
	active := appview.DownloadQueueState{Active: true, Running: 1}
	idle := appview.DownloadQueueState{Active: false}

	if state.ApplyQueue(active) {
		t.Fatal("starting an active cycle must not ring")
	}
	state.NoteDownloadCompleted()
	if !state.CompletedInActiveCycle {
		t.Fatal("completion must be recorded for the active cycle")
	}
	if !state.ApplyQueue(idle) {
		t.Fatal("draining after a completion must ring once")
	}
	if state.CompletedInActiveCycle {
		t.Fatal("draining must reset the completion marker")
	}
	if state.ApplyQueue(idle) {
		t.Fatal("the bell must not ring twice for one drain")
	}

	state.ApplyQueue(active)
	if state.ApplyQueue(idle) {
		t.Fatal("draining without a completion must stay silent")
	}
}

func TestFilterGroupsDefaultsAndExpandedRows(t *testing.T) {
	groups := appcore.BuildLocalModGroups([]structs.ModInfo{
		{ID: "a", FileName: "a.jar", Enabled: true, Loaders: []string{"fabric"}},
		{ID: "b", FileName: "b.jar", Enabled: false, Loaders: []string{"forge"}},
	}, "fabric")
	if got := FilterGroups(groups, "", ""); len(got) != 2 {
		t.Fatalf("empty filter kept %d groups", len(got))
	}
	if got := FilterGroups(groups, "", FilterEnabled); len(got) != 1 || got[0].Primary.ID != "a" {
		t.Fatalf("enabled filter=%+v", got)
	}
	if got := FilterGroups(groups, "B.JAR", FilterAll); len(got) != 1 || got[0].Primary.ID != "b" {
		t.Fatalf("keyword filter=%+v", got)
	}
	if groups[0].Incompatible || !groups[1].Incompatible {
		t.Fatalf("loader compatibility = %v/%v, want false/true", groups[0].Incompatible, groups[1].Incompatible)
	}

	rows, count := PartitionAndBuildRows(groups, true)
	if count != 1 || len(rows) != 3 {
		t.Fatalf("expanded rows=%d count=%d", len(rows), count)
	}
	if rows[1].Kind != RowKindIncompatibleToggle || rows[2].Group.Primary.ID != "b" {
		t.Fatalf("expanded rows=%+v", rows)
	}
	rows, count = PartitionAndBuildRows(groups, false)
	if count != 1 || len(rows) != 2 {
		t.Fatalf("collapsed rows=%d count=%d", len(rows), count)
	}
}

func TestMinecraftApplySelectedVersionBranches(t *testing.T) {
	state := NewMinecraftState()
	state.ApplySelectedVersion(nil)
	if state.SelectedVersion != nil {
		t.Fatal("nil version was stored")
	}
	state.ReleaseVersions = []string{"1.20.1"}
	state.ApplySelectedVersion(&structs.VersionInfo{ID: "inst", MinecraftVersion: "1.20.1", ModLoader: "forge"})
	if len(state.ReleaseVersions) != 1 {
		t.Fatalf("known release duplicated: %v", state.ReleaseVersions)
	}
	if state.SelectedMinecraftVersion != "1.20.1" || state.SelectedModLoader != "Forge" {
		t.Fatalf("state=%+v", state)
	}
	state.ApplySelectedVersion(&structs.VersionInfo{ID: "new", MinecraftVersion: "1.19.2"})
	if len(state.ReleaseVersions) != 2 || state.ReleaseVersions[0] != "1.19.2" {
		t.Fatalf("new release not prepended: %v", state.ReleaseVersions)
	}
	if state.SelectedModLoader != "Forge" {
		t.Fatalf("empty loader overwrote the selection: %q", state.SelectedModLoader)
	}
}
