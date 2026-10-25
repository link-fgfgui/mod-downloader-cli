package tviewui

import (
	"strings"
	"testing"

	"github.com/gdamore/tcell/v2"
	"github.com/mattn/go-runewidth"
	"github.com/rivo/tview"

	"github.com/link-fgfgui/mod-downloader-cli/internal/i18n"
	"github.com/link-fgfgui/mod-downloader-cli/internal/tuihost"
)

// testApp renders the shell on a deterministic simulation screen without
// starting the core (offline mode). All controller methods are no-ops.
type testApp struct {
	t   *testing.T
	sim tcell.SimulationScreen
	app *App
}

func newTestApp(t *testing.T, w, h int) *testApp {
	t.Helper()
	host := tuihost.New(tuihost.Options{})
	a := New(host)
	// Tests are deterministic regardless of the host locale.
	i18n.Apply(i18n.LanguageEnglish)
	a.rebuildAllText()
	sim := tcell.NewSimulationScreen("UTF-8")
	if err := sim.Init(); err != nil {
		t.Fatalf("init simulation screen: %v", err)
	}
	a.application.SetScreen(sim)
	sim.SetSize(w, h)
	t.Cleanup(sim.Fini)
	a.application.ForceDraw()
	return &testApp{t: t, sim: sim, app: a}
}

// text serialises the simulated screen into plain lines.
func (h *testApp) text() string {
	cells, w, hgt := h.sim.GetContents()
	var b strings.Builder
	for y := 0; y < hgt; y++ {
		for x := 0; x < w; {
			cell := cells[y*w+x]
			r := ' '
			if len(cell.Runes) > 0 {
				r = cell.Runes[0]
				b.WriteRune(r)
			} else {
				b.WriteByte(' ')
			}
			// Wide characters occupy two cells; the continuation cell still holds
			// stale bytes, so skip it instead of printing another rune.
			step := runewidth.RuneWidth(r)
			if step < 1 {
				step = 1
			}
			x += step
		}
		b.WriteByte('\n')
	}
	return b.String()
}

func (h *testApp) lineCount() int {
	_, _, height := h.sim.GetContents()
	return height
}

func (h *testApp) sendKey(key tcell.Key, r rune, mod tcell.ModMask) {
	event := tcell.NewEventKey(key, r, mod)
	if h.app.handleGlobalKey(event) != nil {
		if focus := h.app.application.GetFocus(); focus != nil {
			if handler := focus.InputHandler(); handler != nil {
				handler(event, func(p tview.Primitive) { h.app.application.SetFocus(p) })
			}
		}
	}
	h.app.application.ForceDraw()
}

func (h *testApp) rune(r rune)     { h.sendKey(tcell.KeyRune, r, tcell.ModNone) }
func (h *testApp) key(k tcell.Key) { h.sendKey(k, 0, tcell.ModNone) }

func (h *testApp) mustContain(marker string) string {
	h.t.Helper()
	text := h.text()
	if !strings.Contains(text, marker) {
		h.t.Fatalf("frame does not contain %q:\n%s", marker, text)
	}
	return text
}

func (h *testApp) mustNotContain(marker string) {
	h.t.Helper()
	if text := h.text(); strings.Contains(text, marker) {
		h.t.Fatalf("frame unexpectedly contains %q:\n%s", marker, text)
	}
}

func (h *testApp) switchPage(id ViewID) {
	h.app.switchView(id)
	h.app.application.ForceDraw()
}

func (h *testApp) setChinese() {
	i18n.Apply(i18n.LanguageChinese)
	h.app.rebuildAllText()
	h.app.application.ForceDraw()
}

func (h *testApp) setEnglish() {
	i18n.Apply(i18n.LanguageEnglish)
	h.app.rebuildAllText()
	h.app.application.ForceDraw()
}

func (h *testApp) sidebarWidth() int {
	_, _, w, _ := h.app.sidebarCol.GetRect()
	return w
}

// draw is a shorthand for ForceDraw after a state mutation.
func (h *testApp) draw() { h.app.application.ForceDraw() }

// focusType exposes the focused primitive type for assertions.
func (h *testApp) focus() tview.Primitive { return h.app.application.GetFocus() }
