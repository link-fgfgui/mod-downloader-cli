package uistate

import (
	"path/filepath"
	"strconv"
	"strings"

	"github.com/link-fgfgui/mod-downloader-core/models"
	structs "github.com/link-fgfgui/mod-downloader-core/structs/minecraft"
)

// ManageRowKind distinguishes regular mod rows from the incompatible toggle header.
type ManageRowKind string

const (
	RowKindMod                ManageRowKind = "mod"
	RowKindIncompatibleToggle ManageRowKind = "incompatible-toggle"
)

// ManageRow is one renderable row of the Manage list.
type ManageRow struct {
	Kind  ManageRowKind
	Group ModGroup
}

// ModGroup aggregates every physical JAR record sharing one fileName/path/id.
type ModGroup struct {
	Primary structs.ModInfo
	Strong  []structs.ModInfo
	Jij     []structs.JijModInfo
}

// Selectable reports whether the row participates in multi-select / Ctrl+A.
func (r ManageRow) Selectable() bool { return r.Kind != RowKindIncompatibleToggle }

// RowKey mirrors modRowKey.
func (r ManageRow) RowKey() string {
	if r.Kind == RowKindIncompatibleToggle {
		return string(RowKindIncompatibleToggle)
	}
	m := r.Group.Primary
	parts := []string{}
	for _, v := range []string{m.ID, m.SHA1, m.Path, m.FileName} {
		if v != "" {
			parts = append(parts, v)
		}
	}
	return strings.Join(parts, "|")
}

// BuildGroups mirrors allGroupedMods: aggregate by fileName || path || id,
// preserving first-seen order and collecting top-level (strong) declarations
// plus deduplicated Jar-in-Jar entries.
func BuildGroups(mods []structs.ModInfo) []ModGroup {
	order := make([]string, 0, len(mods))
	byKey := make(map[string]*ModGroup, len(mods))
	hasPrimary := make(map[string]bool, len(mods))
	for i := range mods {
		mod := mods[i]
		key := mod.FileName
		if key == "" {
			key = mod.Path
		}
		if key == "" {
			key = mod.ID
		}
		group, ok := byKey[key]
		if !ok {
			group = &ModGroup{}
			byKey[key] = group
			order = append(order, key)
		}
		group.Strong = append(group.Strong, mod)
		addJij(group, mod.JijMods)
		if !hasPrimary[key] {
			group.Primary = mod
			hasPrimary[key] = true
		}
	}
	groups := make([]ModGroup, 0, len(order))
	for _, key := range order {
		groups = append(groups, *byKey[key])
	}
	return groups
}

func addJij(group *ModGroup, mods []structs.JijModInfo) {
	for _, m := range mods {
		id := strings.TrimSpace(m.ID)
		if id == "" {
			continue
		}
		duplicate := false
		for _, existing := range group.Jij {
			if strings.EqualFold(existing.ID, id) {
				duplicate = true
				break
			}
		}
		if !duplicate {
			group.Jij = append(group.Jij, m)
		}
	}
}

// GroupLoaders returns the lowercased loader set declared by strong records.
func GroupLoaders(group ModGroup) map[string]bool {
	loaders := map[string]bool{}
	for _, mod := range group.Strong {
		for _, loader := range mod.Loaders {
			if n := strings.ToLower(strings.TrimSpace(loader)); n != "" {
				loaders[n] = true
			}
		}
	}
	return loaders
}

// IsGroupIncompatible mirrors isGroupIncompatible. The selected loader is
// compared lowercased against the group's declared loaders.
func IsGroupIncompatible(group ModGroup, selectedModLoader string) bool {
	loaders := GroupLoaders(group)
	selected := strings.ToLower(strings.TrimSpace(selectedModLoader))
	return len(loaders) > 0 && selected != "" && !loaders[selected]
}

// SearchableGroupText mirrors searchableGroupText: recursively flatten every
// string/number field of the group (including JIJ and categories) and
// lower-case it for substring matching.
func SearchableGroupText(group ModGroup) string {
	values := []string{}
	for _, mod := range group.Strong {
		collectScalars(mod, &values)
	}
	collectScalars(group.Primary, &values)
	for _, jij := range group.Jij {
		values = append(values, jij.ID, jij.Name)
	}
	return strings.ToLower(strings.Join(values, "\n"))
}

func collectScalars(mod structs.ModInfo, out *[]string) {
	appendIf := func(values ...string) {
		for _, v := range values {
			if v != "" {
				*out = append(*out, v)
			}
		}
	}
	appendIf(mod.ID, mod.Name, mod.Version, mod.Description, mod.FileName, mod.Path, mod.SHA1,
		mod.OnlineName, mod.OnlinePlatform, mod.OnlineProjectID, mod.OnlineSlug, mod.OnlineVersion,
		mod.OnlineVersionID, mod.OnlineFileName, mod.IconURL)
	appendIf(mod.Categories...)
	appendIf(mod.Loaders...)
	if mod.CfFingerprint != 0 {
		appendIf(strconv.FormatUint(uint64(mod.CfFingerprint), 10))
	}
	for _, dep := range mod.Dependencies {
		appendIf(dep.ModID, dep.Type)
	}
	for _, jij := range mod.JijMods {
		appendIf(jij.ID, jij.Name)
	}
}

// EnabledFilter enumerates the Manage status-filter options.
const (
	FilterAll      = "all"
	FilterEnabled  = "enabled"
	FilterDisabled = "disabled"
)

// FilterGroups mirrors groupedMods: keyword + enabled-state filtering.
func FilterGroups(groups []ModGroup, search string, enabledFilter string) []ModGroup {
	needle := strings.ToLower(strings.TrimSpace(search))
	filter := enabledFilter
	if filter == "" {
		filter = FilterAll
	}
	out := make([]ModGroup, 0, len(groups))
	for _, group := range groups {
		if needle != "" && !strings.Contains(SearchableGroupText(group), needle) {
			continue
		}
		if filter != FilterAll {
			want := filter == FilterEnabled
			if group.Primary.Enabled != want {
				continue
			}
		}
		out = append(out, group)
	}
	return out
}

// PartitionAndBuildRows mirrors compatibleGroupedMods/incompatibleGroupedMods
// plus managedRows: compatible groups first, then the toggle header (when any
// incompatible group exists), then the incompatible groups when expanded.
func PartitionAndBuildRows(groups []ModGroup, selectedModLoader string, expanded bool) (rows []ManageRow, incompatibleCount int) {
	compatible := make([]ModGroup, 0, len(groups))
	incompatible := make([]ModGroup, 0)
	for _, group := range groups {
		if IsGroupIncompatible(group, selectedModLoader) {
			incompatible = append(incompatible, group)
		} else {
			compatible = append(compatible, group)
		}
	}
	for _, group := range compatible {
		rows = append(rows, ManageRow{Kind: RowKindMod, Group: group})
	}
	if len(incompatible) == 0 {
		return rows, 0
	}
	rows = append(rows, ManageRow{Kind: RowKindIncompatibleToggle})
	if expanded {
		for _, group := range incompatible {
			rows = append(rows, ManageRow{Kind: RowKindMod, Group: group})
		}
	}
	return rows, len(incompatible)
}

// DisplayModName mirrors displayModName.
func DisplayModName(group ModGroup) string {
	if group.Primary.OnlineName != "" {
		return group.Primary.OnlineName
	}
	if group.Primary.Name != "" {
		return group.Primary.Name
	}
	return group.Primary.ID
}

// ModCategories returns trimmed, de-duplicated, order-preserving categories.
func ModCategories(group ModGroup) []string {
	seen := map[string]bool{}
	out := []string{}
	for _, c := range group.Primary.Categories {
		key := strings.ToLower(strings.TrimSpace(c))
		if key == "" || seen[key] {
			continue
		}
		seen[key] = true
		out = append(out, key)
	}
	return out
}

// StrongModIDs mirrors strongModIds.
func StrongModIDs(group ModGroup) []string {
	out := []string{}
	for _, m := range group.Strong {
		if m.ID != "" {
			out = append(out, m.ID)
		}
	}
	return out
}

// StrongModNames mirrors strongModNames.
func StrongModNames(group ModGroup) []string {
	out := []string{}
	for _, m := range group.Strong {
		if v := firstNonEmpty(m.Name, m.ID); v != "" {
			out = append(out, v)
		}
	}
	return out
}

// HasGroupedDetails mirrors hasGroupedDetails.
func HasGroupedDetails(group ModGroup) bool { return len(group.Strong) > 1 || len(group.Jij) > 0 }

// CanReplaceVersion mirrors canReplaceVersion.
func CanReplaceVersion(group ModGroup, hasSelectedInstance bool) bool {
	return group.Primary.OnlinePlatform != "" && group.Primary.OnlineProjectID != "" && hasSelectedInstance
}

// CanFavoriteGroup mirrors canFavoriteGroup.
func CanFavoriteGroup(group ModGroup, minecraftVersion, modLoader string) bool {
	return strings.TrimSpace(minecraftVersion) != "" &&
		strings.TrimSpace(modLoader) != "" &&
		group.Primary.OnlinePlatform != "" &&
		group.Primary.OnlineProjectID != ""
}

// CanFavoriteSelection mirrors canFavoriteSelection.
func CanFavoriteSelection(groups []ModGroup, minecraftVersion, modLoader string) bool {
	if len(groups) == 0 {
		return false
	}
	for _, group := range groups {
		if !CanFavoriteGroup(group, minecraftVersion, modLoader) {
			return false
		}
	}
	return true
}

// OnlineProjectFromGroup mirrors onlineProjectFromGroup.
func OnlineProjectFromGroup(group ModGroup) models.ModProject {
	mod := group.Primary
	platform := mod.OnlinePlatform
	projectID := mod.OnlineProjectID
	return models.ModProject{
		ID:          strings.ToLower(platform) + ":" + projectID,
		Platform:    platform,
		ProjectID:   projectID,
		Slug:        mod.OnlineSlug,
		Title:       DisplayModName(group),
		Icon:        "mdi-package-variant",
		IconURL:     mod.IconURL,
		Description: mod.Description,
		Categories:  ModCategories(group),
	}
}

// SelectedGroupPaths mirrors selectedGroupPaths: trimmed primary paths, de-duplicated.
func SelectedGroupPaths(groups []ModGroup) []string {
	seen := map[string]bool{}
	paths := []string{}
	for _, group := range groups {
		p := strings.TrimSpace(group.Primary.Path)
		if p == "" || seen[p] {
			continue
		}
		seen[p] = true
		paths = append(paths, p)
	}
	return paths
}

// PrimaryBatchAction mirrors primaryBatchAction: disable when any selected group is enabled.
func PrimaryBatchAction(groups []ModGroup) string {
	for _, group := range groups {
		if group.Primary.Enabled {
			return "disable"
		}
	}
	return "enable"
}

// CleanupCandidatesFromDelete mirrors the post-delete cleanup path: it returns
// the scan candidates that are not among the just-deleted paths.
func CleanupCandidatesFromDelete(candidates []UnusedCandidate, deletedPaths []string) []UnusedCandidate {
	if len(candidates) == 0 {
		return nil
	}
	deleted := map[string]bool{}
	for _, p := range deletedPaths {
		deleted[filepath.Clean(p)] = true
	}
	out := make([]UnusedCandidate, 0, len(candidates))
	for _, c := range candidates {
		if deleted[filepath.Clean(c.Path)] {
			continue
		}
		out = append(out, c)
	}
	return out
}

// UnusedCandidate is a UI-neutral view of an unused-dependency candidate.
type UnusedCandidate struct {
	Path       string
	FileName   string
	ModIDs     []string
	Name       string
	OnlineName string
	Evidence   []string
}

// CleanupEvidenceKey maps candidate evidence to the i18n suffix used by the UI.
func CleanupEvidenceKey(evidence []string) string {
	hasOnline := containsString(evidence, "online-library")
	hasRequired := containsString(evidence, "required-by-excluded")
	switch {
	case hasOnline && hasRequired:
		return "onlineAndDeleted"
	case hasRequired:
		return "deleted"
	default:
		return "online"
	}
}

func containsString(values []string, target string) bool {
	for _, v := range values {
		if v == target {
			return true
		}
	}
	return false
}

func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if v != "" {
			return v
		}
	}
	return ""
}
