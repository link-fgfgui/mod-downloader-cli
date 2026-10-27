package tviewui

import (
	"strings"

	"github.com/link-fgfgui/mod-downloader-core/appcore"
	"github.com/link-fgfgui/mod-downloader-core/storage"

	"github.com/link-fgfgui/mod-downloader-cli/internal/i18n"
)

// --- favorites ---------------------------------------------------------------

// packwizDefaultFilename is the suggested export name for a favorite list.
func (a *App) packwizDefaultFilename(listID string) string {
	svc := a.service()
	if svc == nil {
		return "packwiz.zip"
	}
	return svc.FavoriteListPackwizDefaultFilename(listID)
}

func (a *App) loadFavoriteLists(listID string) {
	svc := a.service()
	if svc == nil {
		return
	}
	lists := svc.ListFavoriteLists()
	var contents storage.FavoriteListContents
	if listID != "" {
		contents = svc.ListFavoriteContents(listID)
	}
	a.uiUpdate(func() {
		a.favoritesPage().setLists(lists, contents, listID)
		if a.addFavoriteDialog != nil {
			a.addFavoriteDialog.refresh()
		}
	})
}

func (a *App) loadFavoriteContents(listID string) {
	svc := a.service()
	if svc == nil {
		return
	}
	contents := svc.ListFavoriteContents(listID)
	a.uiUpdate(func() {
		a.favoritesPage().setContents(listID, contents)
	})
}

func (a *App) createFavoriteList(name, mcVersion, modLoader string) {
	svc := a.service()
	if svc == nil {
		return
	}
	list := svc.CreateFavoriteListForScope(name, mcVersion, modLoader)
	lists := svc.ListFavoriteLists()
	a.uiUpdate(func() {
		a.favoritesPage().setLists(lists, storage.FavoriteListContents{}, list.ID)
		a.favoritesPage().selectedListID = list.ID
		a.favoritesPage().Refresh()
		if a.addFavoriteDialog != nil {
			a.addFavoriteDialog.refresh()
		}
	})
}

func (a *App) renameFavoriteList(id, name string) {
	svc := a.service()
	if svc == nil {
		return
	}
	svc.RenameFavoriteList(id, name)
	a.loadFavoriteLists(id)
}

func (a *App) deleteFavoriteList(id string) {
	svc := a.service()
	if svc == nil {
		return
	}
	svc.DeleteFavoriteList(id)
	a.loadFavoriteLists("")
}

func (a *App) updateFavoriteList(list storage.FavoriteList) {
	svc := a.service()
	if svc == nil {
		return
	}
	svc.UpdateFavoriteListMetadata(list)
	a.loadFavoriteLists(list.ID)
}

func (a *App) reorderFavoriteLists(ids []string) {
	svc := a.service()
	if svc == nil {
		return
	}
	svc.ReorderFavoriteLists(ids)
	a.loadFavoriteLists(a.favoritesPage().selectedListID)
}

func (a *App) addFavoriteMods(req appcore.FavoriteBulkAddRequest) {
	svc := a.service()
	if svc == nil {
		return
	}
	svc.AddFavoriteModsToLists(req)
	a.uiUpdate(func() {
		a.toastT("favorites.added")
		a.favoritesPage().Refresh()
		if a.addFavoriteDialog != nil {
			a.addFavoriteDialog.refresh()
		}
	})
	go a.loadFavoriteLists(a.favoritesPage().selectedListID)
}

func (a *App) removeFavoriteMods(selectedListID string, entries []storage.FavoriteModEntry) {
	svc := a.service()
	if svc == nil {
		return
	}
	for _, entry := range entries {
		listID := entry.SourceListID
		if listID == "" {
			listID = selectedListID
		}
		svc.RemoveFavoriteMod(listID, entry.Platform, entry.ModID, entry.MinecraftVersion, entry.ModLoader)
	}
	a.loadFavoriteLists(selectedListID)
}

func (a *App) exportPackwiz(listID, path, mcVersion, modLoader string) {
	svc := a.service()
	if svc == nil {
		return
	}
	_, err := svc.ExportFavoriteListPackwizZipForScope(listID, path, mcVersion, modLoader)
	a.uiUpdate(func() {
		if err != nil {
			a.toastT("favorites.export.failed", map[string]string{"reason": err.Error()})
			return
		}
		a.toastT("favorites.export.success")
		go a.refreshUsageStats()
	})
}

func (a *App) previewCrossVersionCopy(req appcore.FavoriteCrossVersionCopyRequest) {
	svc := a.service()
	if svc == nil {
		return
	}
	preview := svc.PreviewFavoriteListCrossVersionCopy(req)
	a.uiUpdate(func() {
		body := i18n.T("favorites.crossVersionCopy.target", map[string]string{
			"name":             preview.TargetListName,
			"minecraftVersion": preview.MinecraftVersion,
			"modLoader":        preview.ModLoader,
		})
		body += "\n" + i18n.T("favorites.crossVersionCopy.matched", map[string]string{"n": itoa(len(preview.Matched))})
		if len(preview.Conflicts) > 0 {
			body += "\n" + i18n.T("favorites.crossVersionCopy.conflicts", map[string]string{"n": itoa(len(preview.Conflicts))})
		}
		if preview.NameConflict {
			body += "\n" + i18n.T("favorites.crossVersionCopy.nameConflict")
		}
		a.openConfirm(
			i18n.T("favorites.actions.copyAcrossVersions"),
			body,
			[]confirmOption{
				{Label: i18n.T("favorites.actions.apply"), Value: "confirm", Primary: true},
				{Label: i18n.T("favorites.actions.cancel"), Value: "cancel"},
			},
			func(value string) {
				if value == "confirm" {
					go a.applyCrossVersionCopy(req)
				}
			},
		)
	})
}

func (a *App) applyCrossVersionCopy(req appcore.FavoriteCrossVersionCopyRequest) {
	svc := a.service()
	if svc == nil {
		return
	}
	result := svc.ApplyFavoriteListCrossVersionCopy(req)
	a.uiUpdate(func() {
		if result.Applied {
			a.toastT("favorites.crossVersionCopy.success", map[string]string{
				"name":    result.TargetList.Name,
				"added":   itoa(result.Result.Added),
				"skipped": itoa(result.Result.Skipped),
			})
		} else {
			reason := i18n.T("favorites.crossVersionCopy.unknownError")
			if len(result.Result.Errors) > 0 {
				reason = strings.Join(result.Result.Errors, "; ")
			} else if len(result.Preview.Errors) > 0 {
				reason = strings.Join(result.Preview.Errors, "; ")
			}
			a.toastT("favorites.crossVersionCopy.notApplied", map[string]string{"reason": reason})
		}
		go a.loadFavoriteLists(a.favoritesPage().selectedListID)
	})
}

func (a *App) copyFavoriteList(sourceID, targetID string) {
	svc := a.service()
	if svc == nil {
		return
	}
	result := svc.CopyFavoriteListToList(appcore.FavoriteListCopyRequest{SourceListID: sourceID, TargetListID: targetID})
	a.uiUpdate(func() {
		a.toastT("favorites.resultSummary", map[string]string{
			"added":   itoa(result.Added),
			"updated": itoa(result.Updated),
			"skipped": itoa(result.Skipped),
		})
		go a.loadFavoriteLists(a.favoritesPage().selectedListID)
	})
}

func (a *App) addFavoriteReference(parentID, childID string) {
	svc := a.service()
	if svc == nil {
		return
	}
	svc.AddFavoriteListReference(parentID, childID)
	a.uiUpdate(func() {
		a.toastT("favorites.references.added")
		go a.loadFavoriteLists(parentID)
	})
}

func (a *App) removeFavoriteReference(parentID, childID string) {
	svc := a.service()
	if svc == nil {
		return
	}
	svc.RemoveFavoriteListReference(parentID, childID)
	a.uiUpdate(func() {
		go a.loadFavoriteLists(parentID)
		go a.loadFavoriteRefs(parentID)
	})
}

func (a *App) loadFavoriteRefs(parentID string) {
	svc := a.service()
	if svc == nil {
		return
	}
	refs := svc.ListFavoriteListRefs(parentID)
	lists := svc.ListFavoriteLists()
	a.uiUpdate(func() {
		if a.refsDialog != nil {
			a.refsDialog.setRefs(refs, lists)
		}
	})
}

func (a *App) setFavoriteListIcon(list storage.FavoriteList, mode, value string) {
	svc := a.service()
	if svc == nil {
		return
	}
	iconURL := ""
	if mode == "project" && strings.TrimSpace(value) != "" {
		if project, ok := svc.LookupProjectBySlug("modrinth", value, "", ""); ok {
			iconURL = project.IconURL
		}
	}
	if mode == "" {
		value = ""
	}
	list.IconKind = mode
	list.IconValue = value
	list.IconURL = iconURL
	svc.UpdateFavoriteListMetadata(list)
	a.loadFavoriteLists(list.ID)
}
