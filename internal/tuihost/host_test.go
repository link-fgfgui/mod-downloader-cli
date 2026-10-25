package tuihost

import (
	"reflect"
	"testing"

	"github.com/link-fgfgui/mod-downloader-core/appcore"
	"github.com/link-fgfgui/mod-downloader-core/httpserver"
	appstructs "github.com/link-fgfgui/mod-downloader-core/structs"
	structs "github.com/link-fgfgui/mod-downloader-core/structs/minecraft"
)

func TestNormalizeVersion(t *testing.T) {
	if got := normalizeVersion(" 1.2.3 "); got != "1.2.3" {
		t.Fatalf("normalizeVersion=%q", got)
	}
	if got := normalizeVersion("   "); got != "dev" {
		t.Fatalf("empty version=%q", got)
	}
}

func TestEmitRequiresSink(t *testing.T) {
	h := New(Options{Version: "test"})
	h.emit(EventDownloadCompleted, nil) // no sink: must not panic

	var got Event
	h.SetEventSink(func(event Event) { got = event })
	h.emit(EventDownloadQueueUpdated, "payload")
	if got.Kind != EventDownloadQueueUpdated || got.Payload != "payload" {
		t.Fatalf("event=%+v", got)
	}

	h.SetEventSink(nil)
	got = Event{}
	h.emit(EventUsageStatsUpdated, nil)
	if got.Kind != "" {
		t.Fatalf("sink was not cleared: %+v", got)
	}
}

func TestServerNoticeStartsEmpty(t *testing.T) {
	h := New(Options{})
	if notice := h.ServerNotice(); notice != "" {
		t.Fatalf("notice=%q", notice)
	}
	if h.Version() != "dev" {
		t.Fatalf("version=%q", h.Version())
	}
}

// TestCoreEventMapping proves every appcore and HTTP event reaches the sink in
// the UI-neutral form the tview adapter consumes.
func TestCoreEventMapping(t *testing.T) {
	h := New(Options{})
	var got []Event
	h.SetEventSink(func(event Event) { got = append(got, event) })

	update := appstructs.SearchModsUpdate{RequestID: "req", Loading: true, Append: true}
	queue := appstructs.DownloadQueueState{Active: true, Running: 1}
	failed := appstructs.DownloadFailedEvent{FileName: "sodium.jar", Reason: "checksum"}
	version := structs.VersionInfo{ID: "inst-1", MinecraftVersion: "1.20.1", ModLoader: "Fabric"}

	h.emitCoreEvent(appcore.Event{Kind: appcore.EventSearchModsUpdated, Payload: update})
	h.emitCoreEvent(appcore.Event{Kind: appcore.EventDownloadStatesUpdated})
	h.emitCoreEvent(appcore.Event{Kind: appcore.EventDownloadQueueUpdated, Payload: queue})
	h.emitCoreEvent(appcore.Event{Kind: appcore.EventDownloadFailed, Payload: failed})
	h.emitCoreEvent(appcore.Event{Kind: appcore.EventDownloadCompleted})
	h.emitCoreEvent(appcore.Event{Kind: appcore.EventMinecraftDirChanged, Payload: "/tmp/.minecraft"})
	h.emitCoreEvent(appcore.Event{Kind: appcore.EventSelectedVersionChanged, Payload: version})
	h.emitCoreEvent(appcore.Event{Kind: appcore.EventUsageStatsUpdated})
	h.emitHTTPServerEvent(httpserver.Event{Name: httpserver.ExtensionModsAcceptedEvent, Payload: update})

	want := []Event{
		{Kind: EventSearchModsUpdated, Payload: update},
		{Kind: EventDownloadStatesUpdated},
		{Kind: EventDownloadQueueUpdated, Payload: queue},
		{Kind: EventDownloadFailed, Payload: failed},
		{Kind: EventDownloadCompleted},
		{Kind: EventMinecraftDirChanged, Payload: "/tmp/.minecraft"},
		{Kind: EventSelectedVersionChanged, Payload: version},
		{Kind: EventUsageStatsUpdated},
		{Kind: EventExtensionModsAccepted, Payload: update},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("events=%+v\nwant=%+v", got, want)
	}

	// Wrong payload types and unrelated server events must be dropped.
	before := len(got)
	h.emitCoreEvent(appcore.Event{Kind: appcore.EventDownloadQueueUpdated, Payload: "nope"})
	h.emitCoreEvent(appcore.Event{Kind: appcore.EventSelectedVersionChanged, Payload: update})
	h.emitHTTPServerEvent(httpserver.Event{Name: "other"})
	if len(got) != before {
		t.Fatalf("invalid events reached the sink: %+v", got[before:])
	}
}
