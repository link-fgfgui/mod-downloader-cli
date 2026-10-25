package tviewui

import (
	"strings"

	"github.com/gdamore/tcell/v2"
	"github.com/mattn/go-runewidth"
	"github.com/rivo/tview"
)

// statusBar is the single bottom row: key hints on the left and the clickable
// queue capsule on the right. Every hint that maps to a shortcut is itself
// clickable.
type statusBar struct {
	app        *App
	flex       *tview.Flex
	hints      *tview.TextView
	queue      *tview.Button
	queueWidth int
	actions    map[string]func()
}

func newStatusBar(a *App) *statusBar {
	s := &statusBar{app: a}
	s.hints = tview.NewTextView()
	s.hints.SetDynamicColors(true)
	s.hints.SetRegions(true)
	s.hints.SetBackgroundColor(a.theme.SurfaceAlt)
	s.hints.SetTextColor(a.theme.OnSurface)
	s.hints.SetTextAlign(tview.AlignLeft)
	s.hints.SetHighlightedFunc(func(added, _, _ []string) {
		if len(added) == 0 {
			return
		}
		if run := s.actions[added[0]]; run != nil {
			run()
			// Clear the highlight so the bar keeps reading as plain hints.
			s.hints.Highlight()
		}
	})

	s.queue = tview.NewButton("[ ↓ 0/0 ]")
	s.queue.SetStyle(tcell.StyleDefault.Background(a.theme.SurfaceAlt).Foreground(a.theme.Primary))
	s.queue.SetActivatedStyle(a.theme.selectionStyle())
	s.queue.SetSelectedFunc(func() { a.toggleQueue() })

	s.flex = tview.NewFlex().SetDirection(tview.FlexColumn).
		AddItem(s.hints, 0, 1, false).
		AddItem(s.queue, 13, 0, false)
	s.flex.SetBackgroundColor(a.theme.SurfaceAlt)
	return s
}

func (s *statusBar) Primitive() tview.Primitive { return s.flex }

// SetHints renders the hint segments, tagging the clickable ones as regions.
func (s *statusBar) SetHints(hints []hint) {
	if len(hints) == 0 {
		hints = []hint{hintText(uiText("keys"))}
	}
	hints = s.fit(hints)
	s.actions = make(map[string]func(), len(hints))
	var b strings.Builder
	for i, h := range hints {
		if i > 0 {
			b.WriteString(" · ")
		}
		if h.Run == nil {
			b.WriteString(tview.Escape(h.Text))
			continue
		}
		id := "hint" + itoa(i)
		s.actions[id] = h.Run
		// Clickable segments are tinted so they read as controls, not text.
		b.WriteString(`["` + id + `"]` + s.app.theme.tagWith(s.app.theme.Primary, "b") +
			tview.Escape(h.Text) + resetTag + `[""]`)
	}
	s.hints.SetText(b.String())
}

// SetQueueLabel updates the capsule text and its responsive width.
func (s *statusBar) SetQueueLabel(label string) {
	text := "[ " + label + " ]"
	s.queue.SetLabel(text)
	// Bracketed labels look like style tags to tview's width helper, which
	// reports zero and clipped the capsule; measure the plain text.
	width := runewidth.StringWidth(text) + 1
	if width < 9 {
		width = 9
	}
	s.queueWidth = width
	s.flex.ResizeItem(s.queue, width, 0)
}

// fit drops trailing hints that do not fit so the bar never clips a label in
// the middle of a word.
func (s *statusBar) fit(hints []hint) []hint {
	// The status bar spans the whole screen, so the app width is the reliable
	// measure; the flex rect is still its 15-cell default before the first draw.
	width := s.app.width
	available := width - s.queueWidth
	if available <= 0 {
		return hints
	}
	used := 0
	kept := hints[:0]
	for i, h := range hints {
		w := runewidth.StringWidth(h.Text)
		if i > 0 {
			w += 3 // " · "
		}
		// Keep the first segment even when it is too long: clipping one label
		// beats rendering an empty bar.
		if i > 0 && used+w > available {
			break
		}
		kept = append(kept, h)
		used += w
	}
	return kept
}

// QueueButton exposes the capsule for mouse hit testing.
func (s *statusBar) QueueButton() *tview.Button { return s.queue }

// restyle reapplies the theme after a change.
func (s *statusBar) restyle() {
	s.hints.SetBackgroundColor(s.app.theme.SurfaceAlt)
	s.hints.SetTextColor(s.app.theme.OnSurface)
	s.queue.SetStyle(tcell.StyleDefault.Background(s.app.theme.SurfaceAlt).Foreground(s.app.theme.Primary))
	s.queue.SetActivatedStyle(s.app.theme.selectionStyle())
	s.flex.SetBackgroundColor(s.app.theme.SurfaceAlt)
}
