package cliapp

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"text/tabwriter"
	"time"

	"github.com/urfave/cli/v2"

	"github.com/link-fgfgui/mod-downloader-core/appcore"
	"github.com/link-fgfgui/mod-downloader-core/models"
	"github.com/link-fgfgui/mod-downloader-core/providers"
	"github.com/link-fgfgui/mod-downloader-core/storage"
	structs "github.com/link-fgfgui/mod-downloader-core/structs/minecraft"
)

// resolveProjectsByHashesFn resolves SHA1 hashes to online projects. It is a
// package variable so tests can substitute the network-dependent implementation.
var resolveProjectsByHashesFn = providers.ResolveProjectsByHashes

// modUpdate describes a locally installed mod and its online update status.
type modUpdate struct {
	ID              string `json:"id"`
	Name            string `json:"name"`
	FileName        string `json:"fileName"`
	Path            string `json:"path"`
	Platform        string `json:"platform,omitempty"`
	CurrentVersion  string `json:"currentVersion,omitempty"`
	LatestVersion   string `json:"latestVersion,omitempty"`
	LatestVersionID string `json:"latestVersionId,omitempty"`
	LatestFileName  string `json:"latestFileName,omitempty"`
	LatestSHA1      string `json:"latestSha1,omitempty"`
	InstalledSHA1   string `json:"installedSha1,omitempty"`
	Enabled         bool   `json:"enabled"`
	Outdated        bool   `json:"outdated"`
	Compatible      bool   `json:"compatible"`
	project         models.ModProject
}

func (r runner) checkUpdateCommand() *cli.Command {
	return &cli.Command{
		Name:    "check-update",
		Aliases: []string{"outdated"},
		Usage:   "Check installed mods against the latest compatible versions",
		Flags: []cli.Flag{
			jsonFlag(),
			&cli.StringFlag{Name: "platform", Usage: "provider filter: modrinth or curseforge"},
			&cli.BoolFlag{Name: "force", Usage: "allow running outside a directory named mods"},
		},
		Action: func(c *cli.Context) error {
			runtime, err := r.runtimeInput(c, true)
			if err != nil {
				return err
			}
			if err := validateModsWorkDir(runtime.TargetModsDir, c.Bool("force"), runtime.DirExplicit); err != nil {
				return err
			}
			if err := requireVersionLoader(runtime); err != nil {
				return err
			}
			return r.withService(c, runtime, func(ctx context.Context, svc *appcore.Service) error {
				mods, err := svc.LocalModsInDir(runtime.TargetModsDir, runtime.MinecraftVersion, runtime.ModLoader)
				if err != nil {
					return err
				}
				updates := collectModUpdates(svc, mods, runtime.MinecraftVersion, runtime.ModLoader, c.String("platform"))
				outdated := filterOutdated(updates)
				return r.write(c, outdated, writeOutdated)
			})
		},
	}
}

func (r runner) upgradeCommand() *cli.Command {
	return &cli.Command{
		Name:      "upgrade",
		Usage:     "Download the latest compatible versions and remove superseded jars",
		ArgsUsage: "[<mod...>]",
		Flags: []cli.Flag{
			jsonFlag(),
			&cli.BoolFlag{Name: "all", Usage: "upgrade every mod with a newer compatible version"},
			&cli.BoolFlag{Name: "dry-run", Usage: "print planned downloads and deletions without changing files"},
			&cli.DurationFlag{Name: "timeout", Usage: "optional per-mod upgrade timeout, for example 2m"},
			&cli.BoolFlag{Name: "force", Usage: "allow running outside a directory named mods"},
		},
		Action: func(c *cli.Context) error {
			runtime, err := r.runtimeInput(c, true)
			if err != nil {
				return err
			}
			if err := validateModsWorkDir(runtime.TargetModsDir, c.Bool("force"), runtime.DirExplicit); err != nil {
				return err
			}
			if err := requireVersionLoader(runtime); err != nil {
				return err
			}
			targets := normalizedArgs(c.Args().Slice())
			if !c.Bool("all") && len(targets) == 0 {
				return errors.New("mod is required unless --all is set")
			}
			return r.withService(c, runtime, func(ctx context.Context, svc *appcore.Service) error {
				mods, err := svc.LocalModsInDir(runtime.TargetModsDir, runtime.MinecraftVersion, runtime.ModLoader)
				if err != nil {
					return err
				}
				updates := collectModUpdates(svc, mods, runtime.MinecraftVersion, runtime.ModLoader, "")
				selected := selectUpgradeTargets(updates, targets, c.Bool("all"))
				if len(selected) == 0 {
					return cli.Exit("no outdated mods matched", 1)
				}
				plans := buildUpgradePlans(selected)
				if c.Bool("dry-run") {
					return r.write(c, plans, writeUpgradePlans)
				}
				r.applyUpgradePlans(ctx, svc, runtime, plans, c.Duration("timeout"))
				return r.write(c, plans, writeUpgradePlans)
			})
		},
	}
}

// collectModUpdates resolves each installed jar to an online project and compares
// it with the latest compatible platform version. Jars that cannot be mapped to
// a platform project are skipped.
func collectModUpdates(svc *appcore.Service, mods []structs.ModInfo, mcVersion, modLoader, platformFilter string) []modUpdate {
	platformFilter = strings.ToLower(strings.TrimSpace(platformFilter))
	hashes := make([]string, 0)
	seenHash := make(map[string]bool)
	for _, mod := range mods {
		sha1 := strings.ToLower(strings.TrimSpace(mod.SHA1))
		if sha1 == "" || seenHash[sha1] {
			continue
		}
		seenHash[sha1] = true
		if strings.TrimSpace(mod.OnlinePlatform) == "" {
			hashes = append(hashes, sha1)
		}
	}
	var resolved map[string]models.ModProject
	if len(hashes) > 0 {
		resolved = resolveProjectsByHashesFn(hashes)
	}

	updates := make([]modUpdate, 0)
	seenPath := make(map[string]bool)
	for _, mod := range mods {
		path := strings.TrimSpace(mod.Path)
		if path == "" {
			continue
		}
		if seenPath[path] {
			continue
		}
		seenPath[path] = true

		sha1 := strings.ToLower(strings.TrimSpace(mod.SHA1))
		project, platform, ok := onlineProjectForMod(mod, resolved)
		if !ok {
			continue
		}
		if platformFilter != "" && platform != platformFilter {
			continue
		}

		update := modUpdate{
			ID:            mod.ID,
			Name:          mod.Name,
			FileName:      filepath.Base(path),
			Path:          path,
			Platform:      platform,
			InstalledSHA1: sha1,
			Enabled:       mod.Enabled,
			project:       project,
		}
		update.CurrentVersion = currentInstalledVersion(mod, platform, sha1)

		versions := svc.ListMatchingProjectVersions(project, mcVersion, modLoader)
		if len(versions) == 0 {
			updates = append(updates, update)
			continue
		}
		latest := versions[0]
		update.Compatible = true
		update.LatestVersion = strings.TrimSpace(latest.Version)
		update.LatestVersionID = strings.TrimSpace(latest.ID)
		update.LatestFileName = strings.TrimSpace(latest.FileName)
		update.LatestSHA1 = strings.ToLower(strings.TrimSpace(latest.SHA1))
		if update.LatestVersion == "" {
			update.LatestVersion = strings.TrimSpace(latest.Name)
		}
		update.Outdated = update.LatestSHA1 != "" && update.LatestSHA1 != sha1
		updates = append(updates, update)
	}
	return updates
}

func onlineProjectForMod(mod structs.ModInfo, resolved map[string]models.ModProject) (models.ModProject, string, bool) {
	platform := strings.ToLower(strings.TrimSpace(mod.OnlinePlatform))
	if platform != "" {
		ref := firstNonEmptyString(mod.OnlineSlug, mod.OnlineProjectID)
		if ref != "" {
			project := models.ModProject{
				ID:        models.ProjectKey(platform, ref),
				Platform:  platform,
				ProjectID: firstNonEmptyString(mod.OnlineProjectID, ref),
				Slug:      mod.OnlineSlug,
				Title:     mod.OnlineName,
			}
			return project, platform, true
		}
	}
	sha1 := strings.ToLower(strings.TrimSpace(mod.SHA1))
	if sha1 != "" && resolved != nil {
		if project, ok := resolved[sha1]; ok {
			return project, strings.ToLower(strings.TrimSpace(project.Platform)), true
		}
	}
	return models.ModProject{}, "", false
}

func currentInstalledVersion(mod structs.ModInfo, platform, sha1 string) string {
	if sha1 != "" {
		if version, ok := storage.GetVersionBySHA1(platform, sha1); ok {
			if v := strings.TrimSpace(version.Version); v != "" {
				return v
			}
		}
	}
	return strings.TrimSpace(mod.Version)
}

func filterOutdated(updates []modUpdate) []modUpdate {
	out := make([]modUpdate, 0, len(updates))
	for _, update := range updates {
		if update.Outdated {
			out = append(out, update)
		}
	}
	return out
}

func selectUpgradeTargets(updates []modUpdate, targets []string, all bool) []modUpdate {
	outdated := filterOutdated(updates)
	if all {
		return outdated
	}
	targetSet := make(map[string]bool, len(targets))
	for _, target := range targets {
		targetSet[normalizeModRef(target)] = true
	}
	selected := make([]modUpdate, 0, len(outdated))
	for _, update := range outdated {
		candidates := []string{update.ID, update.Name, update.FileName, strings.TrimSuffix(update.FileName, filepath.Ext(update.FileName))}
		for _, candidate := range candidates {
			if targetSet[normalizeModRef(candidate)] {
				selected = append(selected, update)
				break
			}
		}
	}
	return selected
}

// upgradePlan records the download and cleanup operations for one mod upgrade.
type upgradePlan struct {
	ID             string `json:"id"`
	Name           string `json:"name"`
	Platform       string `json:"platform"`
	CurrentVersion string `json:"currentVersion"`
	LatestVersion  string `json:"latestVersion"`
	OldFile        string `json:"oldFile"`
	NewFile        string `json:"newFile"`
	KeepDisabled   bool   `json:"keepDisabled"`
	Executed       bool   `json:"executed"`
	Note           string `json:"note,omitempty"`

	project         models.ModProject
	latestVersionID string
}

func buildUpgradePlans(updates []modUpdate) []upgradePlan {
	plans := make([]upgradePlan, 0, len(updates))
	for _, update := range updates {
		oldFile := filepath.Base(update.Path)
		newFile := update.LatestFileName
		if strings.TrimSpace(newFile) == "" {
			newFile = oldFile
		}
		plans = append(plans, upgradePlan{
			ID:              update.ID,
			Name:            update.Name,
			Platform:        update.Platform,
			CurrentVersion:  update.CurrentVersion,
			LatestVersion:   update.LatestVersion,
			OldFile:         oldFile,
			NewFile:         newFile,
			KeepDisabled:    localModPathDisabled(update.Path),
			project:         update.project,
			latestVersionID: update.LatestVersionID,
		})
	}
	return plans
}

func (r runner) applyUpgradePlans(ctx context.Context, svc *appcore.Service, runtime runtimeInput, plans []upgradePlan, timeout time.Duration) {
	for i := range plans {
		plan := &plans[i]
		installCtx := ctx
		var cancel context.CancelFunc
		if timeout > 0 {
			installCtx, cancel = context.WithTimeout(ctx, timeout)
		}

		result := svc.InstallModToDirAndWait(installCtx, appcore.InstallToDirRequest{
			ProjectID:        plan.ID,
			Project:          plan.project,
			TargetDir:        runtime.TargetModsDir,
			MinecraftVersion: runtime.MinecraftVersion,
			ModLoader:        runtime.ModLoader,
			VersionID:        plan.latestVersionID,
		})
		if cancel != nil {
			cancel()
		}

		if len(result.Errors) > 0 {
			plan.Note = result.Errors[0].Reason
			continue
		}
		if result.Result.Skipped {
			plan.Note = result.Result.Reason
			continue
		}

		newFile := firstNonEmptyString(result.Result.FileName, plan.NewFile)
		newPath := filepath.Join(runtime.TargetModsDir, filepath.Base(newFile))
		oldPath := filepath.Join(runtime.TargetModsDir, plan.OldFile)

		if plan.KeepDisabled && !localModPathDisabled(newPath) {
			if err := renameToDisabled(newPath); err != nil {
				plan.Note = err.Error()
			}
		}
		removeSupersededFiles(oldPath)
		plan.Executed = true
	}
}

// renameToDisabled renames an enabled jar to its disabled form.
func renameToDisabled(path string) error {
	if localModPathDisabled(path) {
		return nil
	}
	return os.Rename(path, path+".disabled")
}

// removeSupersededFiles deletes an old jar and any .old backup the downloader
// may have created while archiving it.
func removeSupersededFiles(oldPath string) {
	if strings.TrimSpace(oldPath) == "" {
		return
	}
	if err := os.Remove(oldPath); err != nil && !os.IsNotExist(err) {
		// Best-effort cleanup; failures are non-fatal.
		_ = err
	}
	matches, _ := filepath.Glob(oldPath + ".old*")
	for _, match := range matches {
		_ = os.Remove(match)
	}
}

func writeOutdated(w io.Writer, value any) error {
	updates := value.([]modUpdate)
	if len(updates) == 0 {
		fmt.Fprintln(w, "all mods are up to date")
		return nil
	}
	tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
	fmt.Fprintln(tw, "ID\tNAME\tCURRENT_VERSION\tLATEST_VERSION\tPLATFORM")
	for _, update := range updates {
		fmt.Fprintf(tw, "%s\t%s\t%s\t%s\t%s\n", update.ID, update.Name, update.CurrentVersion, update.LatestVersion, update.Platform)
	}
	return tw.Flush()
}

func writeUpgradePlans(w io.Writer, value any) error {
	plans := value.([]upgradePlan)
	tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
	fmt.Fprintln(tw, "ID\tNAME\tCURRENT\tLATEST\tOLD_FILE\tNEW_FILE\tSTATUS")
	for _, plan := range plans {
		status := plan.Note
		if status == "" {
			if plan.Executed {
				status = "upgraded"
			} else {
				status = "preview"
			}
		}
		fmt.Fprintf(tw, "%s\t%s\t%s\t%s\t%s\t%s\t%s\n", plan.ID, plan.Name, plan.CurrentVersion, plan.LatestVersion, plan.OldFile, plan.NewFile, status)
	}
	return tw.Flush()
}
