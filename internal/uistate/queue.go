package uistate

import appstructs "github.com/link-fgfgui/mod-downloader-core/structs"

// Queue tab identifiers for the global download drawer.
const (
	QueueTabDownloads = "downloads"
	QueueTabOptional  = "optional"
)

// QueueState is the pure state of the global download queue drawer (mirrors
// stores/downloadQueue.ts plus the drawer presentation rules documented in
// download-queue-actions.md).
type QueueState struct {
	Queue                  appstructs.DownloadQueueState
	CompletedInActiveCycle bool
	ActiveTab              string
	Open                   bool
}

// NewQueueState returns a queue state defaulting to the downloads tab.
func NewQueueState() QueueState { return QueueState{ActiveTab: QueueTabDownloads} }

// ActiveCount equals pending + running.
func (q QueueState) ActiveCount() int { return q.Queue.Pending + q.Queue.Running }

// MessageCount prefers the explicit count and falls back to the reminder length.
func (q QueueState) MessageCount() int {
	if q.Queue.MessageCount != 0 {
		return q.Queue.MessageCount
	}
	return len(q.Queue.OptionalReminders)
}

// HasVisibleItems reports whether the drawer has any renderable content.
func (q QueueState) HasVisibleItems() bool {
	return len(q.Queue.Items) > 0 || len(q.Queue.OptionalReminders) > 0
}

// ReminderOnly is true when the queue is idle but optional-dependency reminders remain.
func (q QueueState) ReminderOnly() bool { return q.ActiveCount() == 0 && q.MessageCount() > 0 }

// NoteDownloadCompleted mirrors the `download-completed` listener marking the cycle.
func (q *QueueState) NoteDownloadCompleted() { q.CompletedInActiveCycle = true }

// ApplyQueue updates the snapshot and reports whether the terminal bell should
// ring. A bell rings only once per drain cycle, and only when the cycle observed
// at least one successful completion (failures/cancels do not ring).
func (q *QueueState) ApplyQueue(next appstructs.DownloadQueueState) (ring bool) {
	previousActive := q.Queue.Active
	q.Queue = next
	if previousActive && !next.Active {
		ring = q.CompletedInActiveCycle
		q.CompletedInActiveCycle = false
	}
	return ring
}

// SyncPresentation applies the drawer auto tab-switch / auto-collapse rules.
func (q *QueueState) SyncPresentation() {
	if !q.HasVisibleItems() {
		q.Open = false
		return
	}
	if len(q.Queue.Items) == 0 && len(q.Queue.OptionalReminders) > 0 {
		q.ActiveTab = QueueTabOptional
		return
	}
	if len(q.Queue.OptionalReminders) == 0 && q.ActiveTab == QueueTabOptional {
		q.ActiveTab = QueueTabDownloads
	}
}

// CanRemove reports whether a queue entry may be removed without confirmation.
// Running entries cannot be removed directly (they must be canceled first).
func CanRemove(status string) bool {
	switch status {
	case "pending", "failed", "canceled":
		return true
	default:
		return false
	}
}
