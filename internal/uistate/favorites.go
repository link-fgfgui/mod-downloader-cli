package uistate

import (
	"strings"

	"github.com/link-fgfgui/mod-downloader-core/storage"
)

// NormalizeKeyPart mirrors normalizeKeyPart (trim + lower-case).
func NormalizeKeyPart(value string) string { return strings.ToLower(strings.TrimSpace(value)) }

// NormalizeMinecraftVersion mirrors normalizeMinecraftVersion (trim only).
func NormalizeMinecraftVersion(value string) string { return strings.TrimSpace(value) }

// FavoriteKey mirrors favoriteKey.
func FavoriteKey(mod storage.FavoriteMod) string {
	return strings.Join([]string{
		NormalizeKeyPart(mod.Platform),
		NormalizeKeyPart(mod.ModID),
		NormalizeMinecraftVersion(mod.MinecraftVersion),
		NormalizeKeyPart(mod.ModLoader),
	}, "|")
}

// HasFavoriteScope reports whether both scope dimensions are set.
func HasFavoriteScope(minecraftVersion, modLoader string) bool {
	return minecraftVersion != "" && modLoader != ""
}

// FavoriteMatchesScope mirrors favoriteMatchesScope.
func FavoriteMatchesScope(mod storage.FavoriteMod, minecraftVersion, modLoader string) bool {
	return HasFavoriteScope(minecraftVersion, modLoader) &&
		NormalizeMinecraftVersion(mod.MinecraftVersion) == minecraftVersion &&
		NormalizeKeyPart(mod.ModLoader) == modLoader
}

// FavoriteContentsForScope filters a list's contents down to the active scope.
func FavoriteContentsForScope(contents storage.FavoriteListContents, minecraftVersion, modLoader string) storage.FavoriteListContents {
	filtered := make([]storage.FavoriteModEntry, 0, len(contents.Mods))
	for _, mod := range contents.Mods {
		if FavoriteMatchesScope(mod.FavoriteMod, minecraftVersion, modLoader) {
			filtered = append(filtered, mod)
		}
	}
	contents.Mods = filtered
	return contents
}

// ListMatchesScope mirrors listMatchesScope: a list with no scope matches every
// scope; otherwise it must match the active tuple exactly.
func ListMatchesScope(list storage.FavoriteList, minecraftVersion, modLoader string) bool {
	listVersion := NormalizeMinecraftVersion(list.MinecraftVersion)
	listLoader := NormalizeKeyPart(list.ModLoader)
	if listVersion == "" && listLoader == "" {
		return true
	}
	return HasFavoriteScope(minecraftVersion, modLoader) && listVersion == minecraftVersion && listLoader == modLoader
}

// ListOrder mirrors listOrder: pinned first, then sortOrder, then name.
func ListOrder(a, b storage.FavoriteList) bool {
	ap, bp := boolToInt(a.Pinned), boolToInt(b.Pinned)
	if ap != bp {
		return ap > bp
	}
	if a.SortOrder != b.SortOrder {
		return a.SortOrder < b.SortOrder
	}
	return strings.Compare(a.Name, b.Name) < 0
}

// SortLists applies ListOrder.
func SortLists(lists []storage.FavoriteList) []storage.FavoriteList {
	out := append([]storage.FavoriteList(nil), lists...)
	for i := 1; i < len(out); i++ {
		for j := i; j > 0 && ListOrder(out[j], out[j-1]); j-- {
			out[j], out[j-1] = out[j-1], out[j]
		}
	}
	return out
}

// PartitionLists mirrors pinnedLists/ungroupedLists getters.
func PartitionLists(lists []storage.FavoriteList, minecraftVersion, modLoader string) (pinned, ungrouped []storage.FavoriteList) {
	for _, list := range lists {
		if !ListMatchesScope(list, minecraftVersion, modLoader) {
			continue
		}
		if list.Pinned {
			pinned = append(pinned, list)
		} else {
			ungrouped = append(ungrouped, list)
		}
	}
	return SortLists(pinned), SortLists(ungrouped)
}

func boolToInt(v bool) int {
	if v {
		return 1
	}
	return 0
}
