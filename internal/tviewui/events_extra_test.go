package tviewui

import (
	"strings"
	"testing"

	"github.com/link-fgfgui/mod-downloader/core/models"
	appstructs "github.com/link-fgfgui/mod-downloader/core/structs"
	structs "github.com/link-fgfgui/mod-downloader/core/structs/minecraft"
	appview "github.com/link-fgfgui/mod-downloader/core/structs/view"

	"github.com/link-fgfgui/mod-downloader-cli/internal/i18n"
	"github.com/link-fgfgui/mod-downloader-cli/internal/uistate"
)

func TestHandleEventSearchModsUpdated(t *testing.T) {
	h := newTestApp(t, 100, 30)
	h.app.search.SelectedVersion = "1.20.1"
	h.app.search.SelectedModLoader = "Fabric"
	requestID, _, ok := h.app.search.BeginSearch(false)
	if !ok {
		t.Fatal("search did not start")
	}
	results := []models.ModProject{projectAt(0), projectAt(1)}
	h.app.handleSearchUpdate(appview.SearchModsUpdate{RequestID: requestID, Results: results})
	if len(h.app.search.SearchResults) != 2 || h.app.search.IsSearching {
		t.Fatalf("search update not applied: %+v", h.app.search)
	}

	// A stale request id must be ignored.
	h.app.handleSearchUpdate(appview.SearchModsUpdate{RequestID: "stale", Results: nil})
	if len(h.app.search.SearchResults) != 2 {
		t.Fatal("stale search update replaced the results")
	}
}

func TestHandleEventQueueAndDownloadStatuses(t *testing.T) {
	h := newTestApp(t, 120, 40)
	h.app.handleQueueUpdate(appview.DownloadQueueState{
		Active:  true,
		Running: 1,
		Items:   []appview.DownloadQueueItem{{ID: "a", Status: "running", Title: "Sodium"}},
	})
	if !h.app.queue.Visible() || h.app.queueState.Queue.Running != 1 {
		t.Fatal("queue event was not applied")
	}

	h.app.handleDownloadCompleted()
	if !h.app.queueState.CompletedInActiveCycle {
		t.Fatal("download completed was not recorded")
	}

	h.app.handleDownloadFailed(appstructs.DownloadFailedEvent{FileName: "sodium.jar", Reason: "checksum"})
	if !strings.Contains(h.app.toastText, "sodium.jar") || !strings.Contains(h.app.toastText, "checksum") {
		t.Fatalf("failure toast=%q", h.app.toastText)
	}
	if h.app.toastTimer != nil {
		h.app.toastTimer.Stop()
	}

	// Missing file name and reason fall back to localized defaults.
	h.app.handleDownloadFailed(appstructs.DownloadFailedEvent{})
	if h.app.toastText != i18n.T("download.errors.failedWithReason", map[string]string{
		"fileName": i18n.T("download.errors.unknownFile"),
		"reason":   i18n.T("download.errors.generic"),
	}) {
		t.Fatalf("default failure toast=%q", h.app.toastText)
	}
	if h.app.toastTimer != nil {
		h.app.toastTimer.Stop()
	}

	h.app.handleDownloadStatesUpdated()
}

func TestHandleEventSelectedVersionAndRelatives(t *testing.T) {
	h := newTestApp(t, 120, 40)
	version := structs.VersionInfo{ID: "inst-1", Name: "Instance", MinecraftVersion: "1.20.1", ModLoader: "fabric"}
	h.app.handleSelectedVersionChanged(version)
	if h.app.mc.SelectedVersion == nil || h.app.mc.SelectedVersion.ID != "inst-1" {
		t.Fatalf("selected version not applied: %+v", h.app.mc.SelectedVersion)
	}
	if h.app.search.SelectedVersion != "1.20.1" || h.app.search.SelectedModLoader != "Fabric" {
		t.Fatalf("search tuple=%q|%q", h.app.search.SelectedVersion, h.app.search.SelectedModLoader)
	}
	h.app.handleMinecraftDirChanged()
	h.app.handleUsageStatsUpdated()
	if h.app.mc.SelectedVersion.ID != "inst-1" {
		t.Fatal("unrelated events replaced the selected version")
	}
}

func TestHandleEventExtensionMods(t *testing.T) {
	h := newTestApp(t, 100, 30)
	h.switchPage(ViewHome)
	h.app.handleExtensionModsAccepted(appview.SearchModsUpdate{Results: []models.ModProject{projectAt(0)}})
	if h.app.active != ViewDownload {
		t.Fatalf("extension results did not switch to download: %v", h.app.active)
	}
	if len(h.app.search.SearchResults) != 1 {
		t.Fatalf("extension results=%d", len(h.app.search.SearchResults))
	}
	if h.app.toastText != i18n.T("download.queued") {
		t.Fatalf("extension toast=%q", h.app.toastText)
	}
	if h.app.toastTimer != nil {
		h.app.toastTimer.Stop()
	}

	// Empty results leave the current view alone.
	h.switchPage(ViewHome)
	h.app.handleExtensionModsAccepted(appview.SearchModsUpdate{Results: nil})
	if h.app.active != ViewHome {
		t.Fatalf("empty extension results switched views: %v", h.app.active)
	}
}

func TestHandleEventStatusIsRefreshed(t *testing.T) {
	h := newTestApp(t, 100, 30)
	h.switchPage(ViewDownload)
	h.app.handleDownloadStatesUpdated()
	if h.app.statusBar == nil {
		t.Fatal("status bar missing")
	}
	h.app.handleQueueUpdate(appview.DownloadQueueState{Active: true, Running: 2, Pending: 3})
	if got := h.app.statusBar.queue.GetLabel(); !strings.Contains(got, "2/3") {
		t.Fatalf("capsule label=%q", got)
	}
}

// TestCompactEventsKeepsNonQueueOrder is the companion of the lifecycle test:
// an all-queue batch collapses to its last snapshot, and an empty batch is
// returned unchanged.
func TestCompactEventsKeepsNonQueueOrder(t *testing.T) {
	queue := func(payload string) pendingEvent {
		return pendingEvent{queue: true, apply: func() { _ = payload }}
	}
	events := []pendingEvent{queue("a"), queue("b"), {apply: func() {}}, queue("c")}
	got := compactEvents(events)
	if len(got) != 2 || got[0].queue {
		t.Fatalf("compacted=%+v", got)
	}
	if !got[1].queue {
		t.Fatalf("newest queue snapshot missing: %+v", got[1])
	}
	plain := []pendingEvent{{apply: func() {}}, {apply: func() {}}}
	if got := compactEvents(plain); len(got) != 2 {
		t.Fatalf("non-queue batch=%+v", got)
	}
}

func TestQueueStateHelpersUsedByTheShell(t *testing.T) {
	state := uistate.NewQueueState()
	if state.ActiveTab != uistate.QueueTabDownloads {
		t.Fatalf("default tab=%q", state.ActiveTab)
	}
	state.ApplyQueue(appview.DownloadQueueState{
		OptionalReminders: []appview.OptionalDependencyReminder{{ID: "r1"}},
	})
	state.SyncPresentation()
	if state.ActiveTab != uistate.QueueTabOptional || !state.HasVisibleItems() {
		t.Fatalf("reminder-only presentation=%+v", state)
	}
	if state.MessageCount() != 1 || !state.ReminderOnly() {
		t.Fatalf("messageCount=%d reminderOnly=%v", state.MessageCount(), state.ReminderOnly())
	}
}
