package tui

import (
	"github.com/link-fgfgui/mod-downloader-core/appcore"
	"github.com/link-fgfgui/mod-downloader-core/models"
	"github.com/link-fgfgui/mod-downloader-core/storage"
	appstructs "github.com/link-fgfgui/mod-downloader-core/structs"
	structs "github.com/link-fgfgui/mod-downloader-core/structs/minecraft"
)

// The message types below are the strongly-typed bridge between the adapter
// layer (appcore/httpserver callbacks) and the Bubble Tea main loop. Every
// core event is translated into exactly one of these so state is always mutated
// on the main goroutine.

type SearchModsUpdatedMsg struct{ Update appstructs.SearchModsUpdate }

type DownloadStatesUpdatedMsg struct{}

type DownloadQueueUpdatedMsg struct{ State appstructs.DownloadQueueState }

type DownloadFailedMsg struct {
	Event appstructs.DownloadFailedEvent
}

type DownloadCompletedMsg struct{}

type MinecraftDirChangedMsg struct{ Dir string }

type SelectedVersionChangedMsg struct{ Version structs.VersionInfo }

type UsageStatsUpdatedMsg struct{ Stats storage.UsageStats }

type ExtensionModsAcceptedMsg struct{ Update appstructs.SearchModsUpdate }

// windowSizeMsg is re-exported by Bubble Tea but kept here for symmetry with
// the other adapter messages.
type windowSizeMsg struct{ Width, Height int }

// --- Async command result messages -----------------------------------------

// errMsg carries a non-fatal error surfaced as a toast.
type errMsg struct {
	Context string
	Err     error
}

// toastMsg requests a transient toast (key already localized by the caller).
type toastMsg struct {
	Text  string
	Color toastColor
}

type toastColor int

const (
	toastSuccess toastColor = iota
	toastWarning
	toastError
	toastInfo
)

// minecraftSnapshotMsg carries a freshly loaded instance/version snapshot.
type minecraftSnapshotMsg struct {
	Versions       []structs.VersionInfo
	Selected       structs.VersionInfo
	ReleaseList    []string
	MinecraftDir   string
	RefreshDirOnly bool
}

// searchResultMsg carries a completed synchronous core search collection.
type searchResultMsg struct {
	RequestID string
	Update    appstructs.SearchModsUpdate
}

// downloadStatesMsg carries a completed GetDownloadStates result.
type downloadStatesMsg struct {
	RequestID string
	States    []appstructs.ModDownloadButtonState
}

// queueStateMsg carries a fresh queue snapshot (from a manual refresh).
type queueStateMsg struct{ State appstructs.DownloadQueueState }

// usageStatsMsg carries a usage-stats refresh.
type usageStatsMsg struct{ Stats storage.UsageStats }

// localModsMsg carries a refreshed local mod list for Manage.
type localModsMsg struct {
	Version structs.VersionInfo
	Err     error
}

// pendingExitMsg signals the program to quit after clean shutdown.
type pendingExitMsg struct{}

// favoritesLoadedMsg carries a favorites list/mod snapshot.
type favoritesLoadedMsg struct {
	Lists    []storage.FavoriteList
	Contents storage.FavoriteListContents
	ListID   string
}

// favoriteRefsMsg carries a parent list's reference relations plus the list
// directory needed to render the child names.
type favoriteRefsMsg struct {
	ParentID string
	Refs     []storage.FavoriteListRef
	Lists    []storage.FavoriteList
}

// pinnedModsMsg carries the pinned mod table.
type pinnedModsMsg struct{ Pins []storage.PinnedMod }

// settingsLoadedMsg carries the settings view.
type settingsLoadedMsg struct{ View appcore.SettingsView }

// searchSnapshotMsg injects a local snapshot (favorites "open as search results").
type searchSnapshotMsg struct{ Results []models.ModProject }
