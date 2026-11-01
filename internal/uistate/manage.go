package uistate

import (
	"path/filepath"
	"strings"

	"github.com/link-fgfgui/mod-downloader/core/models"
	appview "github.com/link-fgfgui/mod-downloader/core/structs/view"
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

// ModGroup is the core-owned aggregation of every physical JAR record sharing
// one fileName/path/id. Grouping, JIJ de-duplication, search text and the
// loader compatibility verdict all come from appcore.BuildLocalModGroups.
type ModGroup = appview.LocalModGroup

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
		if needle != "" && !strings.Contains(group.SearchText, needle) {
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
func PartitionAndBuildRows(groups []ModGroup, expanded bool) (rows []ManageRow, incompatibleCount int) {
	compatible := make([]ModGroup, 0, len(groups))
	incompatible := make([]ModGroup, 0)
	for _, group := range groups {
		if group.Incompatible {
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
func HasGroupedDetails(group ModGroup) bool { return len(group.Strong) > 1 || len(group.JIJ) > 0 }

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
		Title:       group.DisplayName,
		Icon:        "mdi-package-variant",
		IconURL:     mod.IconURL,
		Description: mod.Description,
		Categories:  group.Categories,
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
