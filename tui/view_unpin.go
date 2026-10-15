package tui

import (
	"strings"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/link-fgfgui/mod-downloader-core/storage"

	"github.com/link-fgfgui/mod-downloader-cli/tui/i18n"
)

// unpinModel renders the pinned-version table (Phase 6).
type unpinModel struct {
	app    *AppModel
	pins   []storage.PinnedMod
	cursor int
	offset int
}

func (v *unpinModel) Update(app *AppModel, msg tea.Msg) tea.Cmd {
	switch typed := msg.(type) {
	case viewActivatedMsg:
		v.cursor = 0
		v.offset = 0
		return cmdListPinnedMods(app.host)
	case pinnedModsMsg:
		v.pins = typed.Pins
		if v.cursor >= len(v.pins) {
			v.cursor = maxInt(0, len(v.pins)-1)
		}
		v.clamp()
		return nil
	case tea.KeyMsg:
		return v.key(app, typed)
	}
	return nil
}

func (v *unpinModel) key(app *AppModel, key tea.KeyMsg) tea.Cmd {
	switch key.String() {
	case "up", "k":
		v.move(-1)
	case "down", "j":
		v.move(1)
	case "r":
		return cmdListPinnedMods(app.host)
	case "u", "enter":
		if len(v.pins) == 0 {
			return nil
		}
		pin := v.pins[v.cursor]
		return cmdUnpinMod(app.host, pin.Platform, pin.ModID, pin.MinecraftVersion, pin.ModLoader)
	case "U":
		if len(v.pins) == 0 {
			return nil
		}
		app.pushModal(newConfirmModal(
			"unpin-all",
			i18n.T("unpin.confirmAll.title"),
			i18n.T("unpin.confirmAll.body"),
			[]choiceOption{
				{Label: i18n.T("unpin.confirmAll.confirm"), Value: "confirm", Destructive: true},
				{Label: i18n.T("unpin.confirmAll.cancel"), Value: "cancel"},
			},
			func(app *AppModel, value string) tea.Cmd {
				if value != "confirm" {
					return nil
				}
				cmds := make([]tea.Cmd, 0, len(v.pins))
				for _, pin := range v.pins {
					cmds = append(cmds, cmdUnpinMod(app.host, pin.Platform, pin.ModID, pin.MinecraftVersion, pin.ModLoader))
				}
				return tea.Batch(cmds...)
			},
		))
	}
	return nil
}

func (v *unpinModel) move(delta int) {
	if len(v.pins) == 0 {
		return
	}
	v.cursor += delta
	if v.cursor < 0 {
		v.cursor = 0
	}
	if v.cursor >= len(v.pins) {
		v.cursor = len(v.pins) - 1
	}
	v.clamp()
}

func (v *unpinModel) clamp() {
	height := 10
	if v.cursor < v.offset {
		v.offset = v.cursor
	}
	if v.cursor >= v.offset+height {
		v.offset = v.cursor - height + 1
	}
	if v.offset < 0 {
		v.offset = 0
	}
}

func (v *unpinModel) View(app *AppModel) string {
	t := app.theme
	var b strings.Builder
	b.WriteString(t.Title().Render(i18n.T("unpin.title")))
	b.WriteString("\n")
	b.WriteString(t.Muted().Render("r " + i18n.T("unpin.refresh") + " · u " + i18n.T("unpin.unpin") + " · U " + i18n.T("unpin.unpinAll")))
	b.WriteString("\n\n")

	if len(v.pins) == 0 {
		b.WriteString(t.Muted().Render(i18n.T("unpin.empty")))
		return b.String()
	}

	header := padRight(i18n.T("unpin.columns.platform"), 14) +
		padRight(i18n.T("unpin.columns.modId"), 24) +
		padRight(i18n.T("unpin.columns.versionId"), 20) +
		padRight(i18n.T("unpin.columns.minecraftVersion"), 12) +
		padRight(i18n.T("unpin.columns.modLoader"), 10)
	b.WriteString(t.Subtitle().Render(header))
	b.WriteString("\n")

	const rows = 10
	end := v.offset + rows
	if end > len(v.pins) {
		end = len(v.pins)
	}
	for i := v.offset; i < end; i++ {
		pin := v.pins[i]
		line := padRight(truncate(pin.Platform, 13), 14) +
			padRight(truncate(pin.ModID, 23), 24) +
			padRight(truncate(pin.VersionID, 19), 20) +
			padRight(truncate(pin.MinecraftVersion, 11), 12) +
			padRight(truncate(pin.ModLoader, 9), 10)
		if i == v.cursor {
			line = t.Selected().Render(stripStyles(line))
		} else {
			line = t.OnSurface().Render(line)
		}
		b.WriteString(line)
		b.WriteString("\n")
	}
	return strings.TrimRight(b.String(), "\n")
}

func (v *unpinModel) FooterHints(app *AppModel) string {
	return "↑↓ " + uiText("navigate") + " · u " + i18n.T("unpin.unpin") + " · U " + i18n.T("unpin.unpinAll") + " · r " + i18n.T("unpin.refresh")
}

func (v *unpinModel) ClearFocus(app *AppModel) (bool, tea.Cmd) { return false, nil }

func (v *unpinModel) CapturesKeys(app *AppModel) bool { return false }

func (v *unpinModel) RowCount() int { return len(v.pins) }

func (v *unpinModel) MouseWheel(app *AppModel, delta int) tea.Cmd {
	v.move(delta)
	return nil
}

func (v *unpinModel) ClickRow(app *AppModel, index int, ev tea.MouseMsg) tea.Cmd {
	if index >= 0 && index < len(v.pins) {
		v.cursor = index
	}
	return nil
}

func (v *unpinModel) ListGeometry(app *AppModel) (header, offset, rows int) {
	return 3, v.offset, len(v.pins)
}
