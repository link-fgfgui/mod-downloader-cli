// Package uistate holds adapter-neutral, pure view state controllers shared by
// the TUI (and testable in isolation). It mirrors the Pinia stores of the Wails
// GUI frontend so the terminal client can reproduce the same behaviour without
// pulling in any UI dependency.
package uistate

import (
	"sort"
	"strconv"
	"strings"

	"github.com/link-fgfgui/mod-downloader-core/models"
	appstructs "github.com/link-fgfgui/mod-downloader-core/structs"
)

// SearchPageSize mirrors `searchPageSize` in stores/downloadSearch.ts.
const SearchPageSize = 10

// search statuses mirrored from the download search store getters.
var (
	// ConfirmStatuses require a confirmation dialog before queueing.
	ConfirmStatuses = map[string]bool{"update": true, "conflict": true, "incompatible": true}
	// MetadataRetryStatuses trigger a metadata refresh instead of queueing.
	MetadataRetryStatuses = map[string]bool{"metadataLoading": true, "metadataFailed": true}
)

// downloadErrorKeys maps core skip reasons to i18n keys, mirroring the frontend map.
var downloadErrorKeys = map[string]string{
	"invalid request":      "invalidRequest",
	"no matching version":  "noMatchingVersion",
	"missing download url": "missingDownloadUrl",
	"no selected version":  "noSelectedVersion",
}

// DownloadErrorI18nKey returns the i18n key for a skip reason, or false when the
// reason is unknown (callers fall back to the generic error key).
func DownloadErrorI18nKey(reason string) (string, bool) {
	key, ok := downloadErrorKeys[strings.TrimSpace(reason)]
	return key, ok
}

// SearchState is the pure state of the Download/search view.
type SearchState struct {
	SearchText                 string
	SelectedVersion            string
	SelectedModLoader          string
	HasSelectedInstance        bool
	ShowDirOverlay             bool
	IsSearching                bool
	IsLoadingMore              bool
	HasMoreResults             bool
	SearchResults              []models.ModProject
	ActiveSearchRequestID      string
	NextSearchOffset           int
	ActiveSearchAppend         bool
	AppendBaseResults          []models.ModProject
	DownloadStates             []appstructs.ModDownloadButtonState
	ActiveDownloadStateRequest string
	DownloadingKeys            map[string]bool
	downloadQueueMembership    string
}

// SetTargetTuple mirrors setTargetTuple: updates the active [MC version | loader]
// tuple and clears download states when anything changed.
func (s *SearchState) SetTargetTuple(mcVersion, modLoader string, hasSelectedInstance bool) bool {
	changed := s.SelectedVersion != mcVersion || s.SelectedModLoader != modLoader || s.HasSelectedInstance != hasSelectedInstance
	s.SelectedVersion = mcVersion
	s.SelectedModLoader = modLoader
	s.HasSelectedInstance = hasSelectedInstance
	if changed {
		s.DownloadStates = nil
	}
	return changed
}

// BeginSearch mirrors runSearch's state mutations and returns (requestID, offset).
// When append is requested while a search is in flight (or no pages remain) it
// returns an empty requestID so callers skip the request, matching the store guard.
func (s *SearchState) BeginSearch(appendMode bool) (requestID string, offset int, proceed bool) {
	if s.SelectedVersion == "" || s.SelectedModLoader == "" {
		s.SearchResults = nil
		s.DownloadStates = nil
		s.HasMoreResults = false
		return "", 0, false
	}
	if appendMode && (s.IsSearching || s.IsLoadingMore || !s.HasMoreResults) {
		return "", 0, false
	}

	requestID = NewRequestID()
	s.ActiveSearchRequestID = requestID
	s.ActiveSearchAppend = appendMode
	if appendMode {
		offset = s.NextSearchOffset
		s.AppendBaseResults = append([]models.ModProject(nil), s.SearchResults...)
		s.IsLoadingMore = true
	} else {
		s.SearchResults = nil
		s.DownloadStates = nil
		s.AppendBaseResults = nil
		s.NextSearchOffset = 0
		s.HasMoreResults = true
		s.IsSearching = true
	}
	return requestID, offset, true
}

// AdvanceOffset mirrors the post-request offset bookkeeping.
func (s *SearchState) AdvanceOffset(requestID string, offset int) {
	if s.ActiveSearchRequestID == requestID {
		s.NextSearchOffset = offset + SearchPageSize
	}
}

// FinishSearch clears the in-flight loading flags for the matching request.
func (s *SearchState) FinishSearch(requestID string) {
	if s.ActiveSearchRequestID != requestID {
		return
	}
	if s.ActiveSearchAppend {
		s.IsLoadingMore = false
	} else {
		s.IsSearching = false
	}
}

// ApplySearchUpdate mirrors the `search-mods-updated` handler. Updates carrying
// a stale requestID are dropped (race protection). It returns true when states
// should be refreshed (i.e. the update finished loading).
func (s *SearchState) ApplySearchUpdate(update appstructs.SearchModsUpdate) (accepted, shouldRefreshStates bool) {
	if update.RequestID != s.ActiveSearchRequestID {
		return false, false
	}
	appendResults := update.Append || (update.RequestID == s.ActiveSearchRequestID && s.ActiveSearchAppend)
	if appendResults {
		merged := make([]models.ModProject, 0, len(s.AppendBaseResults)+len(update.Results))
		merged = append(merged, s.AppendBaseResults...)
		merged = append(merged, update.Results...)
		s.SearchResults = merged
	} else {
		s.SearchResults = update.Results
	}
	s.HasMoreResults = len(update.Results) >= SearchPageSize
	if appendResults {
		s.IsLoadingMore = update.Loading
	} else {
		s.IsSearching = update.Loading
	}
	if !update.Loading {
		return true, true
	}
	return true, false
}

// ShowResults mirrors `showResults`: inject a local snapshot as the result list.
func (s *SearchState) ShowResults(results []models.ModProject) {
	s.SearchText = ""
	s.SearchResults = results
	s.DownloadStates = nil
	s.ActiveSearchRequestID = ""
	s.ActiveSearchAppend = false
	s.AppendBaseResults = nil
	s.NextSearchOffset = 0
	s.HasMoreResults = false
	s.IsSearching = false
	s.IsLoadingMore = false
}

// ApplyExtensionResults mirrors the `extension-mods-accepted` handler.
func (s *SearchState) ApplyExtensionResults(results []models.ModProject) bool {
	if len(results) == 0 {
		return false
	}
	s.SearchResults = results
	s.AppendBaseResults = nil
	s.HasMoreResults = false
	s.IsSearching = false
	s.IsLoadingMore = false
	s.ActiveSearchRequestID = ""
	return true
}

// ApplyQueueMembership mirrors `downloadQueueMembershipKey` diffing: it returns
// true only when the membership key changed, so callers can skip redundant
// download-state refreshes.
func (s *SearchState) ApplyQueueMembership(items []appstructs.DownloadQueueItem) bool {
	membership := QueueMembershipKey(items)
	if membership == s.downloadQueueMembership {
		return false
	}
	s.downloadQueueMembership = membership
	return true
}

// ClearQueueMembership resets the cached membership key (used on stop).
func (s *SearchState) ClearQueueMembership() { s.downloadQueueMembership = "" }

// QueueMembershipKey builds the `id:status` sorted join used to detect queue changes.
func QueueMembershipKey(items []appstructs.DownloadQueueItem) string {
	parts := make([]string, 0, len(items))
	for _, item := range items {
		parts = append(parts, item.ID+":"+item.Status)
	}
	sort.Strings(parts)
	return strings.Join(parts, "|")
}

// StateForResultKey mirrors downloadKeyForResult: the state key at the result's
// index wins, otherwise the result's own composite id.
func (s *SearchState) StateForResultKey(result models.ModProject) string {
	for i := range s.SearchResults {
		if sameProject(s.SearchResults[i], result) {
			if i < len(s.DownloadStates) && s.DownloadStates[i].Key != "" {
				return s.DownloadStates[i].Key
			}
			break
		}
	}
	return result.ID
}

// sameProject compares the identity fields of two projects (ModProject carries
// slices so it is not directly comparable).
func sameProject(a, b models.ModProject) bool {
	return a.ID == b.ID && a.Platform == b.Platform && a.ProjectID == b.ProjectID && a.Slug == b.Slug
}

// BeginDownloading marks a key as in-flight; returns false when already in flight.
func (s *SearchState) BeginDownloading(key string) bool {
	if key == "" || s.DownloadingKeys[key] {
		return false
	}
	if s.DownloadingKeys == nil {
		s.DownloadingKeys = map[string]bool{}
	}
	s.DownloadingKeys[key] = true
	return true
}

// EndDownloading clears the in-flight marker for a key.
func (s *SearchState) EndDownloading(key string) {
	delete(s.DownloadingKeys, key)
}

// DisabledDownloadStates mirrors disabledDownloadStates(reason).
func (s *SearchState) DisabledDownloadStates(reason string) []appstructs.ModDownloadButtonState {
	if reason == "" {
		reason = "noSelectedVersion"
	}
	icon := "mdi-cube-off-outline"
	if reason != "noSelectedVersion" {
		icon = "mdi-alert-circle-outline"
	}
	states := make([]appstructs.ModDownloadButtonState, 0, len(s.SearchResults))
	for i := range s.SearchResults {
		r := s.SearchResults[i]
		states = append(states, appstructs.ModDownloadButtonState{
			Key:      ModIDFromResult(r),
			Status:   reason,
			Disabled: true,
			Loading:  false,
			Icon:     icon,
			Color:    "surface-variant",
		})
	}
	return states
}

// ModIDFromResult strips the platform prefix from a composite project id.
func ModIDFromResult(result models.ModProject) string {
	id := result.ID
	if strings.Contains(id, ":") {
		return strings.Join(strings.Split(id, ":")[1:], ":")
	}
	return id
}

var requestCounter uint64

// NewRequestID returns a monotonic request identifier. Unlike the frontend's
// Date.now()+random scheme a counter is deterministic and collision-free within
// a process, which keeps the race-discard logic testable.
func NewRequestID() string {
	requestCounter++
	return "req-" + strconv.FormatUint(requestCounter, 10)
}
