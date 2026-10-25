package tviewui

import (
	"strings"

	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"

	appstructs "github.com/link-fgfgui/mod-downloader-core/structs"

	"github.com/link-fgfgui/mod-downloader-cli/internal/i18n"
	"github.com/link-fgfgui/mod-downloader-cli/internal/uistate"
)

// disableImpactDialog lists the prerequisites that would go missing when the
// selected mods are disabled, and lets the user restore cached dependencies.
type disableImpactDialog struct {
	app       *App
	groups    []uistate.ModGroup
	action    string
	result    appstructs.LocalModDisableImpactResult
	list      *wheelList
	restored  map[string]bool
	restoring string
	d         *dialog
}

func (a *App) openDisableImpactDialog(groups []uistate.ModGroup, action string, result appstructs.LocalModDisableImpactResult) {
	v := &disableImpactDialog{
		app:      a,
		groups:   groups,
		action:   action,
		result:   result,
		restored: map[string]bool{},
	}
	v.list = newWheelList()
	v.list.ShowSecondaryText(true)
	v.list.SetUseStyleTags(true, true)
	a.theme.styleList(v.list.List)

	body := tview.NewTextView()
	body.SetTextColor(a.theme.Muted)
	body.SetBackgroundColor(a.theme.Surface)
	body.SetText(i18n.T("manage.disableImpact.body"))

	hint := tview.NewTextView()
	hint.SetTextColor(a.theme.Muted)
	hint.SetBackgroundColor(a.theme.Surface)
	hint.SetText("r " + i18n.T("manage.disableImpact.restore") + " · enter " + i18n.T("manage.disableImpact.continue") + " · esc " + i18n.T("manage.disableImpact.cancel"))

	box := dialogBox(a, i18n.T("manage.disableImpact.title"))
	box.AddItem(body, 2, 0, false)
	box.AddItem(v.list, 0, 1, true)
	box.AddItem(hint, 1, 0, false)
	d := &dialog{name: a.dialogName("disable-impact"), focus: v.list}
	continueButton := a.chipButton(i18n.T("manage.disableImpact.continue"), func() {
		groups, action := v.groups, v.action
		a.popDialogName(d.name)
		a.managePage().executeBatch(groups, action)
	})
	cancelButton := a.chipButton(uiText("cancel"), func() { a.popDialogName(d.name) })
	box.AddItem(tview.NewFlex().SetDirection(tview.FlexColumn).
		AddItem(continueButton, chipWidth(i18n.T("manage.disableImpact.continue")), 0, false).
		AddItem(cancelButton, chipWidth(uiText("cancel")), 0, false), 1, 0, false)
	page := newCentered(box, 78, 21)
	d.page = page
	d.handleKey = v.handleKey
	d.onClose = func() {
		a.managePage().pending = nil
		a.managePage().pendingAction = ""
		if a.impactDialog == v {
			a.impactDialog = nil
		}
	}
	v.d = d
	a.impactDialog = v
	a.pushDialog(d)
	v.render()
}

func (v *disableImpactDialog) render() {
	cursor := v.list.GetCurrentItem()
	v.list.Clear()
	t := v.app.theme
	for _, impact := range v.result.Impacts {
		names := []string{}
		for _, mod := range impact.AffectedMods {
			names = append(names, defaultString(mod.Name, defaultString(mod.FileName, mod.Path)))
		}
		main := t.tag(t.Warning) + "⚠ " + tview.Escape(i18n.T("manage.disableImpact.missing", map[string]string{"id": impact.ModID})) + resetTag
		detail := i18n.T("manage.disableImpact.affected", map[string]string{"mods": strings.Join(names, ", ")})
		if len(impact.Candidates) == 0 {
			detail += "  [" + i18n.T("manage.disableImpact.cacheMiss") + "]"
		}
		if v.restored[impact.ModID] {
			main += "  " + t.tag(t.Success) + "✓" + resetTag
		}
		if impact.ModID == v.restoring {
			main += "  " + t.tag(t.Warning) + "…" + resetTag
		}
		v.list.AddItem(main, t.tag(t.Muted)+tview.Escape(detail)+resetTag, 0, nil)
	}
	if cursor < 0 || cursor >= len(v.result.Impacts) {
		cursor = 0
	}
	v.list.SetCurrentItem(cursor)
}

func (v *disableImpactDialog) handleKey(event *tcell.EventKey) bool {
	if event.Key() == tcell.KeyEnter {
		groups := v.groups
		action := v.action
		v.app.popDialogName(v.d.name)
		v.app.managePage().executeBatch(groups, action)
		return true
	}
	if event.Key() == tcell.KeyRune && event.Rune() == 'r' {
		v.restoreCurrent()
		return true
	}
	return false
}

func (v *disableImpactDialog) restoreCurrent() {
	if v.restoring != "" {
		return
	}
	idx := v.list.GetCurrentItem()
	if idx < 0 || idx >= len(v.result.Impacts) {
		return
	}
	impact := v.result.Impacts[idx]
	if v.restored[impact.ModID] {
		return
	}
	if len(impact.Candidates) == 0 {
		v.app.toastT("manage.disableImpact.cacheMiss")
		return
	}
	if len(impact.Candidates) == 1 {
		v.restore(impact.ModID, impact.Candidates[0])
		return
	}
	items := make([]selectItem, 0, len(impact.Candidates))
	for _, candidate := range impact.Candidates {
		items = append(items, selectItem{
			Label:  defaultString(candidate.Title, candidate.ProjectID),
			Value:  candidate.Platform + "\x00" + candidate.ProjectID + "\x00" + candidate.VersionID,
			Detail: candidate.Platform,
		})
	}
	modID := impact.ModID
	v.app.openSelect(i18n.T("manage.disableImpact.chooseSource"), items, func(item selectItem) {
		parts := strings.Split(item.Value, "\x00")
		candidate := appstructs.CachedDependencyCandidate{ModID: modID}
		if len(parts) >= 3 {
			candidate = appstructs.CachedDependencyCandidate{ModID: modID, Platform: parts[0], ProjectID: parts[1], VersionID: parts[2]}
		}
		v.restore(modID, candidate)
	})
}

func (v *disableImpactDialog) restore(modID string, candidate appstructs.CachedDependencyCandidate) {
	if v.restoring != "" {
		return
	}
	v.restoring = modID
	v.render()
	go v.app.restoreCachedDependency(appstructs.RestoreCachedDependencyRequest{
		ModID:     modID,
		Platform:  candidate.Platform,
		ProjectID: candidate.ProjectID,
		VersionID: candidate.VersionID,
	})
}

// restoreDone applies the controller result.
func (v *disableImpactDialog) restoreDone(modID string, queued bool) {
	if v == nil {
		return
	}
	modID = v.restoring
	v.restoring = ""
	if queued {
		v.restored[modID] = true
		v.app.toastT("manage.disableImpact.queued")
	} else {
		v.app.toastT("manage.disableImpact.restoreFailed")
	}
	v.render()
}
