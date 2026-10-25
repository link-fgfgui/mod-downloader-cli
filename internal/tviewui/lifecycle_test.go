package tviewui

import (
	"strings"
	"testing"
	"time"

	"github.com/gdamore/tcell/v2"
	"go.uber.org/goleak"

	appstructs "github.com/link-fgfgui/mod-downloader-core/structs"

	"github.com/link-fgfgui/mod-downloader-cli/internal/tuihost"
)

// TestApplicationLifecycleNoLeaks starts and stops the real tview event loop on
// a simulation screen and verifies that no goroutines survive the shutdown.
func TestApplicationLifecycleNoLeaks(t *testing.T) {
	defer goleak.VerifyNone(t)

	host := tuihost.New(tuihost.Options{})
	a := New(host)
	sim := tcell.NewSimulationScreen("UTF-8")
	if err := sim.Init(); err != nil {
		t.Fatalf("init simulation screen: %v", err)
	}
	a.application.SetScreen(sim)
	sim.SetSize(90, 30)

	done := make(chan struct{})
	go func() {
		defer close(done)
		if err := a.application.Run(); err != nil {
			t.Errorf("run: %v", err)
		}
	}()

	ready := make(chan struct{})
	a.application.QueueUpdateDraw(func() { close(ready) })
	select {
	case <-ready:
	case <-time.After(2 * time.Second):
		t.Fatal("application did not process its first update")
	}

	a.application.Stop()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("application did not stop")
	}
}

// TestCompactEventsKeepsLatestQueueSnapshot verifies that a burst of queue
// snapshots collapses to the newest one while every other event survives in
// order.
func TestCompactEventsKeepsLatestQueueSnapshot(t *testing.T) {
	events := []tuihost.Event{
		{Kind: tuihost.EventDownloadCompleted},
		{Kind: tuihost.EventDownloadQueueUpdated, Payload: "first"},
		{Kind: tuihost.EventDownloadFailed},
		{Kind: tuihost.EventDownloadQueueUpdated, Payload: "last"},
	}
	got := compactEvents(events)
	if len(got) != 3 {
		t.Fatalf("expected 3 events, got %d: %+v", len(got), got)
	}
	if got[0].Kind != tuihost.EventDownloadCompleted || got[1].Kind != tuihost.EventDownloadFailed {
		t.Fatalf("non-queue events lost or reordered: %+v", got)
	}
	if got[2].Kind != tuihost.EventDownloadQueueUpdated || got[2].Payload != "last" {
		t.Fatalf("expected newest queue snapshot last, got %+v", got[2])
	}
}

// TestEventPumpAppliesBatchedEvents feeds a burst of host events through the
// pump and verifies they are applied on the UI goroutine in one batch.
func TestEventPumpAppliesBatchedEvents(t *testing.T) {
	defer goleak.VerifyNone(t)

	host := tuihost.New(tuihost.Options{})
	a := New(host)
	sim := tcell.NewSimulationScreen("UTF-8")
	if err := sim.Init(); err != nil {
		t.Fatalf("init simulation screen: %v", err)
	}
	a.application.SetScreen(sim)
	sim.SetSize(90, 30)

	done := make(chan struct{})
	go func() {
		defer close(done)
		if err := a.application.Run(); err != nil {
			t.Errorf("run: %v", err)
		}
	}()
	a.startEventPump()

	ready := make(chan struct{})
	a.application.QueueUpdateDraw(func() { close(ready) })
	select {
	case <-ready:
	case <-time.After(2 * time.Second):
		t.Fatal("application did not process its first update")
	}

	a.enqueueEvent(tuihost.Event{Kind: tuihost.EventDownloadCompleted})
	a.enqueueEvent(tuihost.Event{Kind: tuihost.EventDownloadQueueUpdated, Payload: appstructs.DownloadQueueState{Running: 1, Active: true}})
	a.enqueueEvent(tuihost.Event{Kind: tuihost.EventDownloadQueueUpdated, Payload: appstructs.DownloadQueueState{Running: 2, Active: true}})

	deadline := time.Now().Add(2 * time.Second)
	for {
		applied := make(chan bool, 1)
		a.application.QueueUpdate(func() {
			applied <- a.queueState.Queue.Running == 2 && a.queueState.CompletedInActiveCycle
		})
		if <-applied {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("batched events were not applied")
		}
		time.Sleep(5 * time.Millisecond)
	}

	a.stopEventPump()
	a.application.Stop()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("application did not stop")
	}
}

// TestBusySpinnerReachesTheScreen runs the real event pump and verifies the
// loading indicator shows up while background work is in flight.
func TestBusySpinnerReachesTheScreen(t *testing.T) {
	defer goleak.VerifyNone(t)

	host := tuihost.New(tuihost.Options{})
	a := New(host)
	sim := tcell.NewSimulationScreen("UTF-8")
	if err := sim.Init(); err != nil {
		t.Fatalf("init simulation screen: %v", err)
	}
	a.application.SetScreen(sim)
	sim.SetSize(100, 30)

	done := make(chan struct{})
	go func() {
		defer close(done)
		if err := a.application.Run(); err != nil {
			t.Errorf("run: %v", err)
		}
	}()
	a.startEventPump()

	ready := make(chan struct{})
	a.application.QueueUpdateDraw(func() { close(ready) })
	select {
	case <-ready:
	case <-time.After(2 * time.Second):
		t.Fatal("application did not process its first update")
	}

	a.beginBusy("Loading test")
	if !waitForText(a, sim, "Loading test", 2*time.Second) {
		t.Fatal("busy label never reached the status bar")
	}
	a.endBusy()
	if !waitForTextGone(a, sim, "Loading test", 2*time.Second) {
		t.Fatal("busy label stayed after endBusy")
	}

	a.stopEventPump()
	a.application.Stop()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("application did not stop")
	}
}

// waitForText polls the screen contents until needle appears.
func waitForText(a *App, sim tcell.SimulationScreen, needle string, timeout time.Duration) bool {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if screenContains(a, sim, needle) {
			return true
		}
		time.Sleep(5 * time.Millisecond)
	}
	return false
}

func waitForTextGone(a *App, sim tcell.SimulationScreen, needle string, timeout time.Duration) bool {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if !screenContains(a, sim, needle) {
			return true
		}
		time.Sleep(5 * time.Millisecond)
	}
	return false
}

func screenContains(a *App, sim tcell.SimulationScreen, needle string) bool {
	applied := make(chan bool, 1)
	a.application.QueueUpdate(func() {
		cells, width, height := sim.GetContents()
		var b strings.Builder
		for y := 0; y < height; y++ {
			for x := 0; x < width; x++ {
				cell := cells[y*width+x]
				if len(cell.Runes) > 0 {
					b.WriteRune(cell.Runes[0])
				} else {
					b.WriteByte(' ')
				}
			}
		}
		applied <- strings.Contains(b.String(), needle)
	})
	return <-applied
}
