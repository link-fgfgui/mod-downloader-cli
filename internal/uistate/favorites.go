package uistate

import (
	"github.com/link-fgfgui/mod-downloader/core/storage"
)

// NormalizeKeyPart delegates to the core favorite scope normalization.
func NormalizeKeyPart(value string) string { return storage.NormalizeFavoriteKeyPart(value) }

// NormalizeMinecraftVersion delegates to the core favorite scope normalization.
func NormalizeMinecraftVersion(value string) string {
	return storage.NormalizeFavoriteMinecraftVersion(value)
}

// FavoriteKey is the UI selection key for one favorite entry.
func FavoriteKey(mod storage.FavoriteMod) string { return storage.FavoriteModKey(mod) }

// HasFavoriteScope reports whether both scope dimensions are set.
func HasFavoriteScope(minecraftVersion, modLoader string) bool {
	return storage.HasFavoriteScope(minecraftVersion, modLoader)
}
