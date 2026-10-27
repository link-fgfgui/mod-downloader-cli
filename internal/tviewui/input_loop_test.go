package tviewui

import (
	"testing"
	"time"

	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"

	"github.com/link-fgfgui/mod-downloader-core/models"
	appstructs "github.com/link-fgfgui/mod-downloader-core/structs"
	structs "github.com/link-fgfgui/mod-downloader-core/structs/minecraft"

	"github.com/link-fgfgui/mod-downloader-cli/internal/i18n"
	"github.com/link-fgfgui/mod-downloader-cli/internal/tuihost"
)

// This file drives the real tview event loop through SimulationScreen input
// injection. Unlike the offline shell tests, it exercises Application input
// capture, focus delegation, widget key handling and drawing end to end.

const inputTimeout = 15 * time.Second

func (l *liveApp) press(key tcell.Key, r rune, mod tcell.ModMask) {
	l.t.Helper()
	l.sim.InjectKey(key, r, mod)
}

func (l *liveApp) pressRune(r rune) {
	l.t.Helper()
	l.press(tcell.KeyRune, r, tcell.ModNone)
}

func (l *liveApp) pressKey(key tcell.Key) {
	l.t.Helper()
	l.press(key, 0, tcell.ModNone)
}

func (l *liveApp) typeText(text string) {
	l.t.Helper()
	for _, r := range text {
		l.pressRune(r)
	}
}

func (l *liveApp) waitText(needle string) {
	l.t.Helper()
	deadline := time.Now().Add(inputTimeout)
	for time.Now().Before(deadline) {
		if screenContains(l.app, l.sim, needle) {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	l.t.Fatalf("screen never contained %q", needle)
}

func (l *liveApp) waitNoText(needle string) {
	l.t.Helper()
	deadline := time.Now().Add(inputTimeout)
	for time.Now().Before(deadline) {
		if !screenContains(l.app, l.sim, needle) {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	l.t.Fatalf("screen still contained %q", needle)
}

func (l *liveApp) click(x, y int) {
	l.t.Helper()
	// tview derives MouseLeftClick from a press followed by a release at the
	// same position; a lone press only produces MouseLeftDown.
	l.sim.InjectMouse(x, y, tcell.Button1, tcell.ModNone)
	l.sim.InjectMouse(x, y, tcell.ButtonNone, tcell.ModNone)
}

func (l *liveApp) resize(w, h int) {
	l.t.Helper()
	l.sim.SetSize(w, h)
	// SimulationScreen.SetSize does not post an event; force the layout pass.
	l.app.application.QueueUpdateDraw(func() {})
}

func (l *liveApp) viewRect() (x, y, w, h int) {
	l.t.Helper()
	l.waitUI(func(a *App) {
		x, y, w, h = a.sidebar.GetRect()
	})
	return x, y, w, h
}

func TestLiveKeyboardNavigationThroughCapture(t *testing.T) {
	l := newLiveApp(t)

	for _, tc := range []struct {
		key   rune
		view  ViewID
		title string
	}{
		{'3', ViewManage, "Local Mod Management"},
		{'5', ViewUnpin, "Pinned Mod Versions"},
		{'6', ViewSettings, "Configure language, theme, downloads and API keys"},
		{'4', ViewFavorites, "Select or create a favorite list"},
		{'2', ViewDownload, "Please select a valid .minecraft directory first"},
		{'1', ViewHome, "Application status and history"},
	} {
		l.pressRune(tc.key)
		l.waitFor("view switch", func(a *App) bool { return a.active == tc.view })
		l.waitText(tc.title)
	}
}

func TestLiveHelpAndTargetDialogs(t *testing.T) {
	l := newLiveApp(t)

	l.pressRune('?')
	l.waitFor("help dialog", func(a *App) bool { return a.topDialog() != nil })
	l.waitText("Keyboard shortcuts")
	l.pressKey(tcell.KeyEsc)
	l.waitFor("help dialog closed", func(a *App) bool { return a.topDialog() == nil })
	l.waitNoText("Keyboard shortcuts")

	l.pressRune('t')
	l.waitFor("target dialog", func(a *App) bool {
		return a.topDialog() != nil && a.targetDialog != nil
	})
	l.waitText("Instance / Target")
	l.pressKey(tcell.KeyEsc)
	l.waitFor("target dialog closed", func(a *App) bool {
		return a.topDialog() == nil && a.targetDialog == nil
	})
	// Esc on a closed dialog at Home must not navigate or crash.
	l.pressKey(tcell.KeyEsc)
	l.waitFor("still home", func(a *App) bool { return a.active == ViewHome })
}

func TestLiveSearchTypingAndEscapeLayers(t *testing.T) {
	l := newLiveApp(t)

	l.pressRune('2')
	l.waitFor("download view", func(a *App) bool { return a.active == ViewDownload })
	l.waitText("Search")

	l.pressRune('/')
	l.waitFor("search focused", func(a *App) bool {
		page := a.downloadPage()
		return page.focusedSearch || a.application.GetFocus() == page.search
	})
	l.typeText("sodium 1.20")
	l.waitFor("typed query", func(a *App) bool {
		return a.downloadPage().search.GetText() == "sodium 1.20"
	})
	// Digits and spaces go to the input rather than activating nav shortcuts.
	l.waitFor("still on download", func(a *App) bool { return a.active == ViewDownload })

	// First Esc exits the input, second Esc leaves the page for Home.
	l.pressKey(tcell.KeyEsc)
	l.waitFor("input blurred", func(a *App) bool {
		page := a.downloadPage()
		return !page.focusedSearch && a.application.GetFocus() == page.results
	})
	l.waitFor("still download", func(a *App) bool { return a.active == ViewDownload })
	l.pressKey(tcell.KeyEsc)
	l.waitFor("home after escape", func(a *App) bool { return a.active == ViewHome })
}

func TestLiveTabMovesBetweenSearchAndList(t *testing.T) {
	l := newLiveApp(t)

	l.pressRune('2')
	l.waitFor("download view", func(a *App) bool { return a.active == ViewDownload })
	l.pressRune('/')
	l.waitFor("search focused", func(a *App) bool { return a.downloadPage().focusedSearch })

	l.pressKey(tcell.KeyTab)
	l.waitFor("list focused after tab", func(a *App) bool {
		page := a.downloadPage()
		return !page.focusedSearch && a.application.GetFocus() == page.results
	})
	l.pressKey(tcell.KeyTab)
	l.waitFor("search focused after back tab", func(a *App) bool {
		page := a.downloadPage()
		return page.focusedSearch && a.application.GetFocus() == page.search
	})
}

func TestLiveSidebarClickSwitchesViews(t *testing.T) {
	l := newLiveApp(t)

	x, y, _, _ := l.viewRect()
	// The nav list has a border, so row N lives at y+1+N.
	l.click(x+2, y+1+2)
	l.waitFor("manage view after click", func(a *App) bool { return a.active == ViewManage })
	l.waitText("Local Mod Management")

	l.click(x+2, y+1+5)
	l.waitFor("settings view after click", func(a *App) bool { return a.active == ViewSettings })
}

func TestLiveResizeReflowsChromeAndDialogs(t *testing.T) {
	l := newLiveApp(t)

	l.pressRune('3')
	l.waitFor("manage view", func(a *App) bool { return a.active == ViewManage })

	l.resize(60, 20)
	l.waitFor("rail breakpoint", func(a *App) bool { return a.bp == BPRail })
	l.waitUI(func(a *App) {
		if _, _, w, _ := a.sidebar.GetRect(); w != 7 {
			t.Errorf("rail sidebar width=%d, want 7", w)
		}
	})

	l.resize(40, 14)
	l.waitFor("compact breakpoint", func(a *App) bool { return a.bp == BPCompact })
	l.waitUI(func(a *App) {
		if _, _, w, _ := a.sidebar.GetRect(); w != 0 {
			t.Errorf("compact sidebar width=%d, want 0", w)
		}
		if a.active != ViewManage {
			t.Errorf("resize changed the active view: %v", a.active)
		}
	})

	// A dialog opened at a wide size must survive a narrow resize.
	l.resize(120, 40)
	l.waitFor("full breakpoint", func(a *App) bool { return a.bp == BPFull })
	l.pressRune('?')
	l.waitFor("help dialog", func(a *App) bool { return a.topDialog() != nil })
	l.resize(40, 20)
	l.waitFor("compact with dialog", func(a *App) bool { return a.bp == BPCompact })
	l.waitFor("dialog still open", func(a *App) bool { return a.topDialog() != nil })
	l.waitText("Keyboard shortcuts")
	l.pressKey(tcell.KeyEsc)
	l.waitFor("dialog closed", func(a *App) bool { return a.topDialog() == nil })
}

func TestLiveThemeDropdownKeyboard(t *testing.T) {
	l := newLiveApp(t)

	l.pressRune('6')
	l.waitFor("settings view", func(a *App) bool { return a.active == ViewSettings })
	// The form starts on the language dropdown; Tab moves to the theme dropdown.
	l.waitFor("language focused", func(a *App) bool {
		page := a.settingsPage()
		return formFocusIndex(page) == page.languageIdx
	})
	l.pressKey(tcell.KeyTab)
	l.waitFor("theme focused", func(a *App) bool {
		page := a.settingsPage()
		return formFocusIndex(page) == page.languageIdx+1
	})
	l.pressKey(tcell.KeyEnter)
	l.waitFor("theme dropdown open", func(a *App) bool { return a.settingsPage().theme.IsOpen() })
	// Options are dark/light/system; move to light and confirm.
	l.pressKey(tcell.KeyDown)
	l.pressKey(tcell.KeyEnter)
	l.waitFor("light theme applied", func(a *App) bool { return a.theme.Name == ThemeLight })
	l.waitText("Theme applied")
}

// formFocusIndex returns the focused form row, or -1.
func formFocusIndex(page *settingsPage) int {
	index, _ := page.form.GetFocusedItemIndex()
	return index
}

func TestLiveLanguageDropdownRebuildsTheForm(t *testing.T) {
	l := newLiveApp(t)

	l.pressRune('6')
	l.waitFor("settings view", func(a *App) bool { return a.active == ViewSettings })
	l.waitFor("language focused", func(a *App) bool {
		page := a.settingsPage()
		return formFocusIndex(page) == page.languageIdx
	})
	l.pressKey(tcell.KeyEnter)
	l.waitFor("language dropdown open", func(a *App) bool { return a.settingsPage().language.IsOpen() })
	// system -> zh.
	l.pressKey(tcell.KeyDown)
	l.pressKey(tcell.KeyEnter)
	l.waitFor("chinese applied", func(a *App) bool { return i18n.Current() == i18n.LanguageChinese })
	l.waitText("设置")
	l.waitFor("form rebuilt in chinese", func(a *App) bool {
		page := a.settingsPage()
		return page.builtLang == i18n.LanguageChinese && formFocusIndex(page) == page.languageIdx
	})
}

func TestLiveOpenDropdownConsumesEscape(t *testing.T) {
	l := newLiveApp(t)

	l.pressRune('6')
	l.waitFor("settings view", func(a *App) bool { return a.active == ViewSettings })
	l.waitFor("language focused", func(a *App) bool {
		page := a.settingsPage()
		return formFocusIndex(page) == page.languageIdx
	})
	l.pressKey(tcell.KeyEnter)
	l.waitFor("language dropdown open", func(a *App) bool { return a.settingsPage().language.IsOpen() })

	// The first Esc closes the dropdown and must not leave the page.
	l.pressKey(tcell.KeyEsc)
	l.waitFor("dropdown closed", func(a *App) bool { return !a.settingsPage().language.IsOpen() })
	l.waitFor("still on settings", func(a *App) bool { return a.active == ViewSettings })

	// The second Esc navigates home.
	l.pressKey(tcell.KeyEsc)
	l.waitFor("home after second escape", func(a *App) bool { return a.active == ViewHome })
}

func TestLiveQueueDockToggle(t *testing.T) {
	l := newLiveApp(t)

	l.pressRune('2')
	l.waitFor("download view", func(a *App) bool { return a.active == ViewDownload })
	l.app.enqueueEvent(tuihost.Event{
		Kind: tuihost.EventDownloadQueueUpdated,
		Payload: appstructs.DownloadQueueState{
			Pending: 1,
			Items: []appstructs.DownloadQueueItem{
				{ID: "pending-1", Status: "pending", Title: "Sodium"},
			},
		},
	})
	l.waitFor("queue state applied", func(a *App) bool {
		return a.queueState.Queue.Pending == 1 && len(a.queueState.Queue.Items) == 1
	})
	l.waitFor("dock closed while idle", func(a *App) bool { return !a.queue.Visible() })

	l.pressRune('q')
	l.waitFor("queue dock opened", func(a *App) bool { return a.queue.Visible() })
	l.waitFor("queue dock focused", func(a *App) bool { return a.queue.Focused() })
	l.waitText("Queue")

	l.pressKey(tcell.KeyEsc)
	l.waitFor("queue dock closed", func(a *App) bool { return !a.queue.Visible() })
	l.waitFor("page still download", func(a *App) bool { return a.active == ViewDownload })
}

func TestLiveSettingsTabSkipsReadOnlyRows(t *testing.T) {
	l := newLiveApp(t)

	l.pressRune('6')
	l.waitFor("settings view", func(a *App) bool { return a.active == ViewSettings })
	l.waitFor("language focused", func(a *App) bool {
		page := a.settingsPage()
		return formFocusIndex(page) == page.languageIdx
	})

	prevItem, prevButton := -2, -2
	for i := 0; i < 30; i++ {
		l.waitUI(func(a *App) {
			prevItem, prevButton = a.settingsPage().form.GetFocusedItemIndex()
		})
		l.pressKey(tcell.KeyTab)
		l.waitFor("focus moved to an interactive row", func(a *App) bool {
			page := a.settingsPage()
			item, button := page.form.GetFocusedItemIndex()
			if item == prevItem && button == prevButton {
				return false
			}
			if item >= 0 {
				_, readOnly := page.form.GetFormItem(item).(*tview.TextView)
				return !readOnly
			}
			return button >= 0
		})
	}

	// Backtab must also land on an interactive row, not a section header.
	l.waitUI(func(a *App) {
		prevItem, prevButton = a.settingsPage().form.GetFocusedItemIndex()
	})
	l.pressKey(tcell.KeyBacktab)
	l.waitFor("backtab moved to an interactive row", func(a *App) bool {
		page := a.settingsPage()
		item, button := page.form.GetFocusedItemIndex()
		if item == prevItem && button == prevButton {
			return false
		}
		if item >= 0 {
			_, readOnly := page.form.GetFormItem(item).(*tview.TextView)
			return !readOnly
		}
		return button >= 0
	})
}

func TestLiveMouseTitleAndDescriptionClicks(t *testing.T) {
	l := newLiveApp(t)

	var zones [3]listRowZones
	var innerX, innerY, innerW int
	l.waitUI(func(a *App) {
		a.mc.SelectedVersion = &structs.VersionInfo{ID: "inst", MinecraftVersion: "1.20.1", ModLoader: "Fabric"}
		a.mc.SelectedMinecraftVersion = "1.20.1"
		a.mc.SelectedModLoader = "Fabric"
		a.search.SetTargetTuple("1.20.1", "Fabric", true)
		a.search.SearchResults = []models.ModProject{
			{ID: "modrinth:alpha", Platform: "modrinth", ProjectID: "alpha", Title: "Alpha"},
			{ID: "modrinth:beta", Platform: "modrinth", ProjectID: "beta", Title: "Beta"},
			{ID: "modrinth:gamma", Platform: "modrinth", ProjectID: "gamma", Title: "Gamma"},
		}
		a.switchView(ViewDownload)
		a.downloadPage().Refresh()
	})
	l.waitFor("download view", func(a *App) bool { return a.active == ViewDownload })
	l.waitUI(func(a *App) {
		page := a.downloadPage()
		for i := range zones {
			zones[i] = page.rowClickZones(i)
		}
		innerX, innerY, innerW, _ = page.results.GetInnerRect()
	})
	if innerW < zones[1].TitleEnd {
		t.Fatalf("seed results are not visible (inner width=%d)", innerW)
	}
	keys := make([]string, 3)
	l.waitUI(func(a *App) {
		page := a.downloadPage()
		for i, result := range a.search.SearchResults {
			keys[i] = page.itemKey(result)
		}
	})

	l.click(innerX+zones[1].TitleStart+1, innerY+2)
	l.waitFor("title click selects only row 1", func(a *App) bool {
		page := a.downloadPage()
		return len(page.selected) == 1 && page.selected[keys[1]] &&
			page.results.GetCurrentItem() == 1
	})

	// A description-line click is not a hit; selection and cursor stay put.
	l.click(innerX+1, innerY+2*2+1)
	l.waitFor("description click is ignored", func(a *App) bool {
		page := a.downloadPage()
		return len(page.selected) == 1 && page.selected[keys[1]] &&
			page.results.GetCurrentItem() == 1
	})

	// A checkbox click joins the multi-selection.
	l.click(innerX+zones[2].CheckboxEnd-1, innerY+2*2)
	l.waitFor("checkbox click adds row 2", func(a *App) bool {
		page := a.downloadPage()
		return len(page.selected) == 2 && page.selected[keys[1]] && page.selected[keys[2]]
	})
}

func TestLiveEventStormKeepsInputResponsive(t *testing.T) {
	l := newLiveApp(t)

	l.pressRune('2')
	l.waitFor("download view", func(a *App) bool { return a.active == ViewDownload })
	const snapshots = 500
	for i := 0; i < snapshots; i++ {
		l.app.enqueueEvent(tuihost.Event{
			Kind: tuihost.EventDownloadQueueUpdated,
			Payload: appstructs.DownloadQueueState{
				Active:  true,
				Running: i,
				Items: []appstructs.DownloadQueueItem{
					{ID: "running", Status: "running", Title: "Sodium"},
				},
			},
		})
	}
	l.waitFor("latest queue snapshot", func(a *App) bool {
		return a.queueState.Queue.Running == snapshots-1
	})
	// Input must still be processed after the burst.
	l.pressRune('3')
	l.waitFor("view switch after storm", func(a *App) bool { return a.active == ViewManage })
}
