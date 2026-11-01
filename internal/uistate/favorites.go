package uistate

import (
	"strings"

	"github.com/link-fgfgui/mod-downloader/core/storage"
)

// NormalizeKeyPart mirrors the scope normalization used for favorite keys.
func NormalizeKeyPart(value string) string { return strings.ToLower(strings.TrimSpace(value)) }

// NormalizeMinecraftVersion trims a Minecraft version for scope comparison.
func NormalizeMinecraftVersion(value string) string { return strings.TrimSpace(value) }

// FavoriteKey is the UI selection key for one favorite entry.
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
