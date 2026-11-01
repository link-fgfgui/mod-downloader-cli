package uistate

import (
	"github.com/link-fgfgui/mod-downloader/core/appcore"
	appstructs "github.com/link-fgfgui/mod-downloader/core/structs"
)

// APIKeySentinel mirrors the frontend `apiKeyKeepSentinel` and core's constant:
// passing it keeps the stored key untouched while the other side is being saved.
const APIKeySentinel = appcore.APIKeyKeepSentinel

// KeyDraft tracks a single API-key editor's transient state.
type KeyDraft struct {
	Value string
	Clear bool
}

// Reset clears the transient editor state (mirrors the post-save reset).
func (d *KeyDraft) Reset() {
	d.Value = ""
	d.Clear = false
}

// Resolve returns the value to send to the core: an explicit clear wins, an
// empty untouched field falls back to the keep-sentinel (never accidentally
// clearing the other side's key).
func (d KeyDraft) Resolve() string {
	if d.Clear {
		return ""
	}
	if d.Value != "" {
		return d.Value
	}
	return APIKeySentinel
}

// BuildSaveAPIKeysRequest mirrors saveAPIKeys: each field resolves independently
// so an untouched key is preserved while the edited one is written.
func BuildSaveAPIKeysRequest(curseforge, modrinth KeyDraft) appstructs.SaveAPIKeysRequest {
	return appstructs.SaveAPIKeysRequest{
		CurseForgeAPIKey: curseforge.Resolve(),
		ModrinthAPIKey:   modrinth.Resolve(),
	}
}

// MaskKey exposes the core masking helper for the settings view.
func MaskKey(value string) string { return appcore.MaskKey(value) }
