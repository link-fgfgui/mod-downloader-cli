package uistate

import (
	"strings"

	structs "github.com/link-fgfgui/mod-downloader-core/structs/minecraft"
)

// DefaultModLoader / ModLoaderOptions mirror the frontend tuple defaults.
const DefaultModLoader = "Fabric"

// ModLoaderOptions is the fixed loader set offered by the target tuple editor.
var ModLoaderOptions = []string{"Fabric", "Forge", "NeoForge"}

// MatchingModLoader mirrors matchingModLoader: return the canonical option when
// the input matches case-insensitively, otherwise the trimmed input.
func MatchingModLoader(value string) string {
	normalized := strings.ToLower(strings.TrimSpace(value))
	for _, option := range ModLoaderOptions {
		if strings.ToLower(option) == normalized {
			return option
		}
	}
	return strings.TrimSpace(value)
}

// MinecraftState mirrors stores/minecraft.ts state.
type MinecraftState struct {
	SelectedVersion            *structs.VersionInfo
	SelectedMinecraftVersion   string
	SelectedModLoader          string
	Versions                   []structs.VersionInfo
	ReleaseVersions            []string
	MinecraftDir               string
	InitializedSelectedModsKey string
}

// NewMinecraftState returns state with the default loader.
func NewMinecraftState() MinecraftState {
	return MinecraftState{SelectedModLoader: DefaultModLoader}
}

// SelectedModsKey mirrors selectedModsKey: "<dir>\x00<versionID|name>".
func (s MinecraftState) SelectedModsKey() string {
	versionKey := ""
	if s.SelectedVersion != nil {
		versionKey = s.SelectedVersion.ID
		if versionKey == "" {
			versionKey = s.SelectedVersion.Name
		}
	}
	if s.MinecraftDir == "" || versionKey == "" {
		return ""
	}
	return s.MinecraftDir + "\x00" + versionKey
}

// EnsureSelectedModsLoadedNeeded mirrors ensureSelectedModsLoaded's guard.
func (s MinecraftState) EnsureSelectedModsLoadedNeeded() bool {
	key := s.SelectedModsKey()
	return key != "" && key != s.InitializedSelectedModsKey
}

// MarkSelectedModsLoaded records that the current instance's mods are loaded,
// breaking the refresh/event feedback loop.
func (s *MinecraftState) MarkSelectedModsLoaded() {
	s.InitializedSelectedModsKey = s.SelectedModsKey()
}

// HasSelectedInstance mirrors hasSelectedInstance.
func (s MinecraftState) HasSelectedInstance() bool {
	return s.SelectedVersion != nil && (s.SelectedVersion.Name != "" || s.SelectedVersion.ID != "")
}

// SelectedInstanceLabel returns the instance's own display name. The active MC
// version and loader are rendered separately by the shell; appending them here
// duplicated the target whenever the instance name already contained it.
func (s MinecraftState) SelectedInstanceLabel() string {
	if s.SelectedVersion == nil {
		return ""
	}
	name := strings.TrimSpace(s.SelectedVersion.Name)
	if name != "" {
		return name
	}
	return strings.TrimSpace(s.SelectedVersion.ID)
}

// Mods mirrors the `mods` getter.
func (s MinecraftState) Mods() []structs.ModInfo {
	if s.SelectedVersion == nil {
		return nil
	}
	return s.SelectedVersion.Mods
}

// ApplySelectedVersion mirrors applySelectedVersion: adopt the version, insert
// unseen MC versions into release list, and normalize the loader.
func (s *MinecraftState) ApplySelectedVersion(version *structs.VersionInfo) {
	s.SelectedVersion = version
	if version == nil {
		return
	}
	if version.MinecraftVersion != "" {
		found := false
		for _, v := range s.ReleaseVersions {
			if v == version.MinecraftVersion {
				found = true
				break
			}
		}
		if !found {
			s.ReleaseVersions = append([]string{version.MinecraftVersion}, s.ReleaseVersions...)
		}
		s.SelectedMinecraftVersion = version.MinecraftVersion
	}
	if version.ModLoader != "" {
		s.SelectedModLoader = MatchingModLoader(version.ModLoader)
	}
}

// SetSelectedMinecraftVersion mirrors the manual tuple edit: clears the instance.
func (s *MinecraftState) SetSelectedMinecraftVersion(version string) {
	s.SelectedMinecraftVersion = strings.TrimSpace(version)
	s.SelectedVersion = nil
}

// SetSelectedModLoader mirrors the manual loader edit: clears the instance.
func (s *MinecraftState) SetSelectedModLoader(modLoader string) {
	s.SelectedModLoader = MatchingModLoader(modLoader)
	s.SelectedVersion = nil
}
