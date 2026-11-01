package uistate

import (
	"strings"

	"github.com/link-fgfgui/mod-downloader/core/models"
	"github.com/link-fgfgui/mod-downloader/core/searchexpr"
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
	Group appview.LocalModGroup
}

// appview.LocalModGroup (core-owned) is the aggregation of every physical JAR
// record sharing one fileName/path/id. Grouping, JIJ de-duplication, search
// text and the loader compatibility verdict all come from
// appcore.BuildLocalModGroups.

// Selectable reports whether the row participates in multi-select / Ctrl+A.
func (r ManageRow) Selectable() bool { return r.Kind != RowKindIncompatibleToggle }

// RowKey mirrors modRowKey.
func (r ManageRow) RowKey() string {
	if r.Kind == RowKindIncompatibleToggle {
		return string(RowKindIncompatibleToggle)
	}
	return r.Group.RowKey
}

// EnabledFilter enumerates the Manage status-filter options.
const (
	FilterAll      = "all"
	FilterEnabled  = "enabled"
	FilterDisabled = "disabled"
)

// FilterGroups mirrors groupedMods: keyword + enabled-state filtering.
func FilterGroups(groups []appview.LocalModGroup, search string, enabledFilter string) []appview.LocalModGroup {
	filter := enabledFilter
	if filter == "" {
		filter = FilterAll
	}
	out := make([]appview.LocalModGroup, 0, len(groups))
	for _, group := range groups {
		if !searchexpr.Matches(group.SearchText, search) {
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
func PartitionAndBuildRows(groups []appview.LocalModGroup, expanded bool) (rows []ManageRow, incompatibleCount int) {
	compatible := make([]appview.LocalModGroup, 0, len(groups))
	incompatible := make([]appview.LocalModGroup, 0)
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
func StrongModIDs(group appview.LocalModGroup) []string {
	out := []string{}
	for _, m := range group.Strong {
		if m.ID != "" {
			out = append(out, m.ID)
		}
	}
	return out
}

// StrongModNames mirrors strongModNames.
func StrongModNames(group appview.LocalModGroup) []string {
	out := []string{}
	for _, m := range group.Strong {
		if v := firstNonEmpty(m.Name, m.ID); v != "" {
			out = append(out, v)
		}
	}
	return out
}

// HasGroupedDetails mirrors hasGroupedDetails.
func HasGroupedDetails(group appview.LocalModGroup) bool {
	return len(group.Strong) > 1 || len(group.JIJ) > 0
}

// CanReplaceVersion mirrors canReplaceVersion.
func CanReplaceVersion(group appview.LocalModGroup, hasSelectedInstance bool) bool {
	return group.CanReplaceVersion && hasSelectedInstance
}

// CanFavoriteGroup mirrors canFavoriteGroup.
func CanFavoriteGroup(group appview.LocalModGroup, minecraftVersion, modLoader string) bool {
	return strings.TrimSpace(minecraftVersion) != "" &&
		strings.TrimSpace(modLoader) != "" &&
		group.Primary.OnlinePlatform != "" &&
		group.Primary.OnlineProjectID != ""
}

// CanFavoriteSelection mirrors canFavoriteSelection.
func CanFavoriteSelection(groups []appview.LocalModGroup, minecraftVersion, modLoader string) bool {
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

// OnlineProjectFromGroup returns the core-built online project projection.
func OnlineProjectFromGroup(group appview.LocalModGroup) models.ModProject {
	return group.OnlineProject
}

// SelectedGroupPaths mirrors selectedGroupPaths: trimmed primary paths, de-duplicated.
func SelectedGroupPaths(groups []appview.LocalModGroup) []string {
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
func PrimaryBatchAction(groups []appview.LocalModGroup) string {
	for _, group := range groups {
		if group.Primary.Enabled {
			return "disable"
		}
	}
	return "enable"
}

func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if v != "" {
			return v
		}
	}
	return ""
}
