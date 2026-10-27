package tviewui

import (
	"strings"
	"testing"

	"github.com/link-fgfgui/mod-downloader-core/models"
	appstructs "github.com/link-fgfgui/mod-downloader-core/structs"
	structs "github.com/link-fgfgui/mod-downloader-core/structs/minecraft"

	"github.com/link-fgfgui/mod-downloader-cli/internal/i18n"
	"github.com/link-fgfgui/mod-downloader-cli/internal/tuihost"
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
	h.app.handleEvent(tuihost.Event{
		Kind:    tuihost.EventSearchModsUpdated,
		Payload: appstructs.SearchModsUpdate{RequestID: requestID, Results: results},
	})
	if len(h.app.search.SearchResults) != 2 || h.app.search.IsSearching {
		t.Fatalf("search update not applied: %+v", h.app.search)
	}

	// A stale request id must be ignored.
	h.app.handleEvent(tuihost.Event{
		Kind:    tuihost.EventSearchModsUpdated,
		Payload: appstructs.SearchModsUpdate{RequestID: "stale", Results: nil},
	})
	if len(h.app.search.SearchResults) != 2 {
		t.Fatal("stale search update replaced the results")
	}

	// Wrong payload type must be ignored.
	h.app.handleEvent(tuihost.Event{Kind: tuihost.EventSearchModsUpdated, Payload: "nope"})
}

func TestHandleEventQueueAndDownloadStatuses(t *testing.T) {
	h := newTestApp(t, 120, 40)
	h.app.handleEvent(tuihost.Event{
		Kind: tuihost.EventDownloadQueueUpdated,
		Payload: appstructs.DownloadQueueState{
			Active:  true,
			Running: 1,
			Items:   []appstructs.DownloadQueueItem{{ID: "a", Status: "running", Title: "Sodium"}},
		},
	})
	if !h.app.queue.Visible() || h.app.queueState.Queue.Running != 1 {
		t.Fatal("queue event was not applied")
	}

	h.app.handleEvent(tuihost.Event{Kind: tuihost.EventDownloadCompleted})
	if !h.app.queueState.CompletedInActiveCycle {
		t.Fatal("download completed was not recorded")
	}

	h.app.handleEvent(tuihost.Event{Kind: tuihost.EventDownloadFailed, Payload: appstructs.DownloadFailedEvent{
		FileName: "sodium.jar", Reason: "checksum",
	}})
	if !strings.Contains(h.app.toastText, "sodium.jar") || !strings.Contains(h.app.toastText, "checksum") {
		t.Fatalf("failure toast=%q", h.app.toastText)
	}
	if h.app.toastTimer != nil {
		h.app.toastTimer.Stop()
	}

	// Missing file name and reason fall back to localized defaults.
	h.app.handleEvent(tuihost.Event{Kind: tuihost.EventDownloadFailed, Payload: appstructs.DownloadFailedEvent{}})
	if h.app.toastText != i18n.T("download.errors.failedWithReason", map[string]string{
		"fileName": i18n.T("download.errors.unknownFile"),
		"reason":   i18n.T("download.errors.generic"),
	}) {
		t.Fatalf("default failure toast=%q", h.app.toastText)
	}
	if h.app.toastTimer != nil {
		h.app.toastTimer.Stop()
	}

	// Wrong payload types are dropped.
	h.app.handleEvent(tuihost.Event{Kind: tuihost.EventDownloadQueueUpdated, Payload: 42})
	h.app.handleEvent(tuihost.Event{Kind: tuihost.EventDownloadFailed, Payload: "nope"})
	h.app.handleEvent(tuihost.Event{Kind: tuihost.EventDownloadStatesUpdated})
}

func TestHandleEventSelectedVersionAndRelatives(t *testing.T) {
	h := newTestApp(t, 120, 40)
	version := structs.VersionInfo{ID: "inst-1", Name: "Instance", MinecraftVersion: "1.20.1", ModLoader: "fabric"}
	h.app.handleEvent(tuihost.Event{Kind: tuihost.EventSelectedVersionChanged, Payload: version})
	if h.app.mc.SelectedVersion == nil || h.app.mc.SelectedVersion.ID != "inst-1" {
		t.Fatalf("selected version not applied: %+v", h.app.mc.SelectedVersion)
	}
	if h.app.search.SelectedVersion != "1.20.1" || h.app.search.SelectedModLoader != "Fabric" {
		t.Fatalf("search tuple=%q|%q", h.app.search.SelectedVersion, h.app.search.SelectedModLoader)
	}
	h.app.handleEvent(tuihost.Event{Kind: tuihost.EventSelectedVersionChanged, Payload: "wrong"})
	h.app.handleEvent(tuihost.Event{Kind: tuihost.EventMinecraftDirChanged, Payload: "/tmp/.minecraft"})
	h.app.handleEvent(tuihost.Event{Kind: tuihost.EventUsageStatsUpdated})
	h.app.handleEvent(tuihost.Event{Kind: "unknown-kind"})
	if h.app.mc.SelectedVersion.ID != "inst-1" {
		t.Fatal("bad payload replaced the selected version")
	}
}

func TestHandleEventExtensionMods(t *testing.T) {
	h := newTestApp(t, 100, 30)
	h.switchPage(ViewHome)
	h.app.handleEvent(tuihost.Event{
		Kind:    tuihost.EventExtensionModsAccepted,
		Payload: appstructs.SearchModsUpdate{Results: []models.ModProject{projectAt(0)}},
	})
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
	h.app.handleEvent(tuihost.Event{
		Kind:    tuihost.EventExtensionModsAccepted,
		Payload: appstructs.SearchModsUpdate{Results: nil},
	})
	if h.app.active != ViewHome {
		t.Fatalf("empty extension results switched views: %v", h.app.active)
	}
	h.app.handleEvent(tuihost.Event{Kind: tuihost.EventExtensionModsAccepted, Payload: "nope"})
}

func TestHandleEventStatusIsRefreshed(t *testing.T) {
	h := newTestApp(t, 100, 30)
	h.switchPage(ViewDownload)
	h.app.handleEvent(tuihost.Event{Kind: tuihost.EventDownloadStatesUpdated})
	if h.app.statusBar == nil {
		t.Fatal("status bar missing")
	}
	h.app.handleEvent(tuihost.Event{
		Kind:    tuihost.EventDownloadQueueUpdated,
		Payload: appstructs.DownloadQueueState{Active: true, Running: 2, Pending: 3},
	})
	if got := h.app.statusBar.queue.GetLabel(); !strings.Contains(got, "2/3") {
		t.Fatalf("capsule label=%q", got)
	}
}

// TestCompactEventsKeepsNonQueueOrder is the companion of the lifecycle test:
// an all-queue batch collapses to its last snapshot, and an empty batch is
// returned unchanged.
func TestCompactEventsKeepsNonQueueOrder(t *testing.T) {
	queue := func(payload string) tuihost.Event {
		return tuihost.Event{Kind: tuihost.EventDownloadQueueUpdated, Payload: payload}
	}
	events := []tuihost.Event{queue("a"), queue("b"), {Kind: tuihost.EventUsageStatsUpdated}, queue("c")}
	got := compactEvents(events)
	if len(got) != 2 || got[0].Kind != tuihost.EventUsageStatsUpdated || got[1].Payload != "c" {
		t.Fatalf("compacted=%+v", got)
	}
	plain := []tuihost.Event{{Kind: tuihost.EventUsageStatsUpdated}, {Kind: tuihost.EventDownloadCompleted}}
	if got := compactEvents(plain); len(got) != 2 {
		t.Fatalf("non-queue batch=%+v", got)
	}
}

func TestQueueStateHelpersUsedByTheShell(t *testing.T) {
	state := uistate.NewQueueState()
	if state.ActiveTab != uistate.QueueTabDownloads {
		t.Fatalf("default tab=%q", state.ActiveTab)
	}
	state.ApplyQueue(appstructs.DownloadQueueState{
		OptionalReminders: []appstructs.OptionalDependencyReminder{{ID: "r1"}},
	})
	state.SyncPresentation()
	if state.ActiveTab != uistate.QueueTabOptional || !state.HasVisibleItems() {
		t.Fatalf("reminder-only presentation=%+v", state)
	}
	if state.MessageCount() != 1 || !state.ReminderOnly() {
		t.Fatalf("messageCount=%d reminderOnly=%v", state.MessageCount(), state.ReminderOnly())
	}
}
