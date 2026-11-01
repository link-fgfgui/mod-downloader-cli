package tviewui

import (
	"strings"
	"testing"
	"time"

	"github.com/gdamore/tcell/v2"
	"go.uber.org/goleak"

	"github.com/link-fgfgui/mod-downloader/core/host"
	appview "github.com/link-fgfgui/mod-downloader/core/structs/view"
)

// TestApplicationLifecycleNoLeaks starts and stops the real tview event loop on
// a simulation screen and verifies that no goroutines survive the shutdown.
func TestApplicationLifecycleNoLeaks(t *testing.T) {
	defer goleak.VerifyNone(t)

	a := New(host.New(host.Options{}))
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
	queue := func(payload string) pendingEvent {
		return pendingEvent{queue: true, apply: func() { _ = payload }}
	}
	events := []pendingEvent{
		{apply: func() {}},
		queue("first"),
		{apply: func() {}},
		queue("last"),
	}
	got := compactEvents(events)
	if len(got) != 3 {
		t.Fatalf("expected 3 events, got %d: %+v", len(got), got)
	}
	if got[0].queue || got[1].queue {
		t.Fatalf("non-queue events lost or reordered: %+v", got)
	}
	if !got[2].queue {
		t.Fatalf("expected newest queue snapshot last, got %+v", got[2])
	}
}

// TestEventPumpAppliesBatchedEvents feeds a burst of host events through the
// pump and verifies they are applied on the UI goroutine in one batch.
func TestEventPumpAppliesBatchedEvents(t *testing.T) {
	defer goleak.VerifyNone(t)

	a := New(host.New(host.Options{}))
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

	a.enqueueEvent(false, a.handleDownloadCompleted)
	a.enqueueEvent(true, func() {
		a.handleQueueUpdate(appview.DownloadQueueState{Running: 1, Active: true})
	})
	a.enqueueEvent(true, func() {
		a.handleQueueUpdate(appview.DownloadQueueState{Running: 2, Active: true})
	})

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

	a := New(host.New(host.Options{}))
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
		applied <- strings.Contains(screenText(sim), needle)
	})
	return <-applied
}
