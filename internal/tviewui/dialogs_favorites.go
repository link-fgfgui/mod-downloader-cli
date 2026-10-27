package tviewui

import (
	"strings"

	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"

	"github.com/link-fgfgui/mod-downloader-core/appcore"
	"github.com/link-fgfgui/mod-downloader-core/storage"

	"github.com/link-fgfgui/mod-downloader-cli/internal/i18n"
	"github.com/link-fgfgui/mod-downloader-cli/internal/uistate"
)

// addFavoriteDialog lets the user pick (or create) favorite lists for a set of
// drafts.
type addFavoriteDialog struct {
	app    *App
	drafts []favoriteDraft
	lists  []storage.FavoriteList
	chosen map[string]bool
	head   *tview.TextView
	list   *wheelList
	d      *dialog
}

func (a *App) openAddFavoriteDialog(drafts []favoriteDraft) {
	if len(drafts) == 0 {
		return
	}
	if !uistate.HasFavoriteScope(a.mc.SelectedMinecraftVersion, a.mc.SelectedModLoader) {
		a.toastT("favorites.empty.selectOrCreate")
		return
	}
	v := &addFavoriteDialog{app: a, drafts: drafts, chosen: map[string]bool{}}
	v.lists = a.favoritesPage().visibleLists()

	v.head = tview.NewTextView().SetDynamicColors(true)
	v.head.SetBackgroundColor(a.theme.Surface)
	v.head.SetText(a.theme.tag(a.theme.Muted) + i18n.T("favorites.addDialog.count", map[string]string{"n": itoa(len(drafts))}) + resetTag)

	v.list = newWheelList()
	v.list.ShowSecondaryText(false)
	a.theme.styleList(v.list.List)
	v.list.SetSelectedFunc(func(index int, _, _ string, _ rune) {
		v.toggle(index)
	})

	box := dialogBox(a, i18n.T("favorites.addDialog.title"))
	box.AddItem(v.head, 1, 0, false)
	box.AddItem(v.list, 0, 1, true)
	box.AddItem(v.buttonRow(), 1, 0, false)
	box.AddItem(v.hintRow(), 1, 0, false)
	page := newCentered(box, 64, 19)
	d := &dialog{name: a.dialogName("add-favorite"), page: page, focus: v.list}
	d.handleKey = v.handleKey
	d.onClose = func() {
		if a.addFavoriteDialog == v {
			a.addFavoriteDialog = nil
		}
	}
	v.d = d
	a.addFavoriteDialog = v
	a.pushDialog(d)
	v.render()
	if !a.favoritesPage().loaded {
		go a.loadFavoriteLists("")
	}
}

func (v *addFavoriteDialog) hintRow() *tview.TextView {
	hint := tview.NewTextView()
	hint.SetTextColor(v.app.theme.Muted)
	hint.SetBackgroundColor(v.app.theme.Surface)
	hint.SetText("space " + uiText("space") + " · enter " + uiText("confirm") + " · n " + uiText("newList") + " · esc " + uiText("close"))
	return hint
}

// buttonRow provides the same actions for mouse users.
func (v *addFavoriteDialog) buttonRow() *tview.Flex {
	add := tview.NewButton(i18n.T("favorites.actions.add"))
	v.app.theme.styleButton(add)
	add.SetSelectedFunc(v.confirm)
	newList := tview.NewButton(i18n.T("favorites.actions.create"))
	v.app.theme.styleButton(newList)
	newList.SetSelectedFunc(v.promptNewList)
	cancel := tview.NewButton(i18n.T("favorites.actions.cancel"))
	cancel.SetStyle(tcell.StyleDefault.Background(v.app.theme.Surface).Foreground(v.app.theme.Error))
	cancel.SetActivatedStyle(tcell.StyleDefault.Background(v.app.theme.Error).Foreground(v.app.theme.SelectedFg))
	cancel.SetSelectedFunc(func() { v.app.popDialogName(v.d.name) })
	return tview.NewFlex().SetDirection(tview.FlexColumn).
		AddItem(add, 0, 1, false).
		AddItem(newList, 0, 1, false).
		AddItem(cancel, 0, 1, false)
}

func (v *addFavoriteDialog) render() {
	cursor := v.list.GetCurrentItem()
	v.list.Clear()
	t := v.app.theme
	if len(v.lists) == 0 {
		v.list.AddItem(t.tag(t.Muted)+i18n.T("favorites.empty.noLists")+resetTag, "", 0, nil)
		return
	}
	for _, list := range v.lists {
		check := checkBoxTagged(v.chosen[list.ID])
		v.list.AddItem(check+" "+tview.Escape(list.Name), "", 0, nil)
	}
	if cursor < 0 || cursor >= len(v.lists) {
		cursor = 0
	}
	v.list.SetCurrentItem(cursor)
}

// refresh receives the reloaded list directory.
func (v *addFavoriteDialog) refresh() {
	if v == nil {
		return
	}
	v.lists = v.app.favoritesPage().visibleLists()
	v.render()
}

func (v *addFavoriteDialog) toggle(index int) {
	if index < 0 || index >= len(v.lists) {
		return
	}
	id := v.lists[index].ID
	v.chosen[id] = !v.chosen[id]
	v.render()
	v.list.SetCurrentItem(index)
}

func (v *addFavoriteDialog) handleKey(event *tcell.EventKey) bool {
	switch event.Key() {
	case tcell.KeyEnter:
		v.confirm()
		return true
	case tcell.KeyRune:
		switch event.Rune() {
		case ' ':
			v.toggle(v.list.GetCurrentItem())
			return true
		case 'n':
			v.promptNewList()
			return true
		}
	}
	return false
}

func (v *addFavoriteDialog) confirm() {
	ids := []string{}
	for _, list := range v.lists {
		if v.chosen[list.ID] {
			ids = append(ids, list.ID)
		}
	}
	if len(ids) == 0 {
		idx := v.list.GetCurrentItem()
		if idx >= 0 && idx < len(v.lists) {
			ids = append(ids, v.lists[idx].ID)
		}
	}
	if len(ids) == 0 {
		return
	}
	mods := make([]storage.FavoriteMod, 0, len(v.drafts))
	for _, d := range v.drafts {
		mods = append(mods, storage.FavoriteMod{
			Platform:         d.Platform,
			ModID:            d.ModID,
			VersionID:        d.VersionID,
			MinecraftVersion: d.MinecraftVersion,
			ModLoader:        d.ModLoader,
			Title:            d.Title,
			Slug:             d.Slug,
			IconURL:          d.IconURL,
			Description:      d.Description,
			Categories:       d.Categories,
		})
	}
	v.app.popDialogName(v.d.name)
	go v.app.addFavoriteMods(appcore.FavoriteBulkAddRequest{TargetListIDs: ids, Mods: mods})
}

func (v *addFavoriteDialog) promptNewList() {
	v.app.openPrompt(i18n.T("favorites.dialog.createTitle"), "", "", i18n.T("favorites.dialog.name"), nil,
		func(name string) {
			if strings.TrimSpace(name) == "" {
				return
			}
			go v.app.createFavoriteList(name, v.app.mc.SelectedMinecraftVersion, v.app.mc.SelectedModLoader)
		})
}

// favoriteRefsDialog manages the parent -> child references of one list.
type favoriteRefsDialog struct {
	app      *App
	parentID string
	refs     []storage.FavoriteListRef
	names    map[string]string
	list     *wheelList
	d        *dialog
}

func (a *App) openFavoriteRefsDialog(parentID string) {
	v := &favoriteRefsDialog{app: a, parentID: parentID, names: map[string]string{}}
	v.list = newWheelList()
	v.list.ShowSecondaryText(false)
	a.theme.styleList(v.list.List)

	box := dialogBox(a, i18n.T("favorites.dialog.referenceTitle"))
	box.AddItem(v.list, 0, 1, true)
	d := &dialog{name: a.dialogName("refs"), focus: v.list}
	closeButton := a.chipButton(uiText("close"), func() { a.popDialogName(d.name) })
	box.AddItem(tview.NewFlex().SetDirection(tview.FlexColumn).
		AddItem(closeButton, chipWidth(uiText("close")), 0, false), 1, 0, false)
	page := newCentered(box, 68, 19)
	d.page = page
	d.handleKey = v.handleKey
	d.onClose = func() {
		if a.refsDialog == v {
			a.refsDialog = nil
		}
	}
	v.d = d
	a.refsDialog = v
	a.pushDialog(d)
	v.render()
	go a.loadFavoriteRefs(parentID)
}

func (v *favoriteRefsDialog) setRefs(refs []storage.FavoriteListRef, lists []storage.FavoriteList) {
	if v == nil {
		return
	}
	v.refs = refs
	v.names = map[string]string{}
	for _, list := range lists {
		v.names[list.ID] = list.Name
	}
	v.render()
}

func (v *favoriteRefsDialog) render() {
	v.list.Clear()
	t := v.app.theme
	if len(v.refs) == 0 {
		v.list.AddItem(t.tag(t.Muted)+i18n.T("favorites.empty.noItems")+resetTag, "", 0, nil)
		return
	}
	for _, ref := range v.refs {
		name := v.names[ref.ChildListID]
		if name == "" {
			name = ref.ChildListID
		}
		v.list.AddItem("▸ "+tview.Escape(name), "", 0, nil)
	}
}

func (v *favoriteRefsDialog) handleKey(event *tcell.EventKey) bool {
	if event.Key() != tcell.KeyRune {
		return false
	}
	switch event.Rune() {
	case 'x', 'd':
		idx := v.list.GetCurrentItem()
		if idx < 0 || idx >= len(v.refs) {
			return true
		}
		ref := v.refs[idx]
		go v.app.removeFavoriteReference(ref.ParentListID, ref.ChildListID)
		return true
	}
	return false
}
