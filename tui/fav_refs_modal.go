package tui

import (
	"strings"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/link-fgfgui/mod-downloader-core/storage"

	"github.com/link-fgfgui/mod-downloader-cli/tui/i18n"
)

// favoriteRefsModal lists a parent list's child references and allows removing
// them (ListFavoriteListRefs / RemoveFavoriteListReference parity).
type favoriteRefsModal struct {
	id       string
	parentID string
	refs     []storage.FavoriteListRef
	names    map[string]string
	cursor   int
}

func newFavoriteRefsModal(parentID string) *favoriteRefsModal {
	return &favoriteRefsModal{id: "favorite-refs", parentID: parentID, names: map[string]string{}}
}

func (m *favoriteRefsModal) ID() string          { return m.id }
func (m *favoriteRefsModal) CapturesInput() bool { return true }
func (m *favoriteRefsModal) PreferredWidth() int { return 68 }

func (m *favoriteRefsModal) Update(app *AppModel, msg tea.Msg) tea.Cmd {
	switch typed := msg.(type) {
	case favoriteRefsMsg:
		if typed.ParentID != m.parentID {
			return nil
		}
		m.refs = typed.Refs
		m.names = map[string]string{}
		for _, list := range typed.Lists {
			m.names[list.ID] = list.Name
		}
		if m.cursor >= len(m.refs) {
			m.cursor = maxInt(0, len(m.refs)-1)
		}
		return nil
	case favoritesLoadedMsg:
		// A removal refreshed the parent; reload the refs so the modal stays live.
		return cmdListFavoriteRefs(app.host, m.parentID)
	}

	key, ok := msg.(tea.KeyMsg)
	if !ok {
		return nil
	}
	switch key.String() {
	case "esc":
		app.popModal(m.id)
	case "up", "k":
		if m.cursor > 0 {
			m.cursor--
		}
	case "down", "j":
		if m.cursor < len(m.refs)-1 {
			m.cursor++
		}
	case "x", "d", "delete":
		if m.cursor < len(m.refs) {
			ref := m.refs[m.cursor]
			return cmdRemoveFavoriteListRef(app.host, ref.ParentListID, ref.ChildListID)
		}
	}
	return nil
}

func (m *favoriteRefsModal) View(app *AppModel) string {
	t := app.theme
	var b strings.Builder
	b.WriteString(t.Title().Render(i18n.T("favorites.dialog.referenceTitle")))
	b.WriteString("\n")
	b.WriteString(t.Muted().Render(m.listName(m.parentID)))
	b.WriteString("\n\n")

	if len(m.refs) == 0 {
		b.WriteString(t.Muted().Render(i18n.T("favorites.empty.noItems")))
		return b.String()
	}
	for i, ref := range m.refs {
		line := "▸ " + truncate(m.listName(ref.ChildListID), 40)
		if i == m.cursor {
			line = t.Selected().Render(stripStyles(line))
		} else {
			line = t.OnSurface().Render(line)
		}
		b.WriteString(line)
		b.WriteString("\n")
	}
	b.WriteString("\n")
	b.WriteString(t.Muted().Render("x " + i18n.T("favorites.actions.removeSelected") + " · esc " + uiText("close")))
	return b.String()
}

func (m *favoriteRefsModal) listName(id string) string {
	if name, ok := m.names[id]; ok && name != "" {
		return name
	}
	return id
}
