package cliapp

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"text/tabwriter"

	"github.com/urfave/cli/v2"

	"github.com/link-fgfgui/mod-downloader/core/appcore"
	"github.com/link-fgfgui/mod-downloader/core/logging"
	"github.com/link-fgfgui/mod-downloader/core/minecraft"
	"github.com/link-fgfgui/mod-downloader/core/models"
	"github.com/link-fgfgui/mod-downloader/core/storage"
	appstructs "github.com/link-fgfgui/mod-downloader/core/structs"
	structs "github.com/link-fgfgui/mod-downloader/core/structs/minecraft"

	"github.com/link-fgfgui/mod-downloader-cli/cliapp/prompt"
)

type runner struct {
	stdout io.Writer
	stderr io.Writer
}

type runtimeInput struct {
	WorkDir          string
	MinecraftVersion string
	ModLoader        string
	CacheDir         string
	TargetModsDir    string
	// DirExplicit is true when the target directory was selected explicitly via
	// --dir/-C (or the minecraft-dir config key), so the "directory must be
	// named mods" safety check is bypassed.
	DirExplicit bool
}

type removedMod struct {
	ID       string `json:"id"`
	Name     string `json:"name"`
	FileName string `json:"fileName"`
	Path     string `json:"path"`
	DryRun   bool   `json:"dryRun"`
}

type showView struct {
	Project  models.ModProject   `json:"project"`
	Versions []models.ModVersion `json:"versions"`
}

func New(stdout, stderr io.Writer) *cli.App {
	r := runner{stdout: stdout, stderr: stderr}
	app := &cli.App{
		Name:  "mod-downloader-cli",
		Usage: "Search and install Minecraft mods from inside a mods directory",
		Before: func(c *cli.Context) error {
			return logging.Configure(logging.Options{Level: cliLogLevel(c)})
		},
		Flags: []cli.Flag{
			verboseFlag(),
			jsonFlag(),
			&cli.StringFlag{Name: "mc-version", Aliases: []string{"minecraft-version", "version"}, Usage: "Minecraft version; auto-detected from the current mods path when omitted", EnvVars: []string{"MINECRAFT_VERSION"}},
			&cli.StringFlag{Name: "loader", Usage: "mod loader: fabric, forge, or neoforge; auto-detected from the current mods path when omitted", EnvVars: []string{"MOD_LOADER"}},
			&cli.StringFlag{Name: "cache-dir", Usage: "metadata cache directory", EnvVars: []string{"MOD_DOWNLOADER_CACHE_DIR"}},
			&cli.StringFlag{Name: "curseforge-api-key", Usage: "CurseForge API key", EnvVars: []string{"CF_API_KEY"}},
			&cli.StringFlag{Name: "modrinth-api-key", Usage: "Modrinth API key", EnvVars: []string{"MODRINTH_API_KEY"}},
			&cli.StringFlag{Name: "dir", Aliases: []string{"C"}, Usage: "target mods directory, or its parent Minecraft instance / version directory", EnvVars: []string{"MOD_DOWNLOADER_DIR"}},
		},
		Commands: []*cli.Command{
			r.showCommand(),
			r.installCommand(),
			r.listCommand(),
			r.freezeCommand(),
			r.removeCommand(),
			r.enableCommand(),
			r.disableCommand(),
			r.checkUpdateCommand(),
			r.upgradeCommand(),
			r.verifyCommand(),
			r.infoCommand(),
			r.configCommand(),
			r.cacheCommand(),
		},
	}
	app.Writer = stdout
	app.ErrWriter = stderr
	for _, cmd := range app.Commands {
		// Accept -v after positional arguments too.
		cmd.Flags = append(cmd.Flags, verboseFlag())
		enableTrailingFlags(cmd)
	}
	return app
}

// cliLogLevel maps -v/--verbose to the core log level. The CLI stays quiet by
// default (warnings and errors only); -v enables debug logging.
func cliLogLevel(c *cli.Context) slog.Level {
	if c != nil && c.Bool("verbose") {
		return slog.LevelDebug
	}
	return slog.LevelWarn
}

func (r runner) showCommand() *cli.Command {
	return &cli.Command{
		Name:      "show",
		Usage:     "Show matching project versions",
		ArgsUsage: "<project>",
		Flags: []cli.Flag{
			jsonFlag(),
			&cli.StringFlag{Name: "project", Usage: "project ID, slug, or platform-prefixed reference"},
			&cli.StringFlag{Name: "platform", Usage: "project platform: modrinth or curseforge"},
		},
		Action: func(c *cli.Context) error {
			runtime, err := r.runtimeInput(c, false)
			if err != nil {
				return err
			}
			if err := requireVersionLoader(runtime); err != nil {
				return err
			}
			return r.withService(c, runtime, func(ctx context.Context, svc *appcore.Service) error {
				project, err := resolveProject(c, svc, runtime)
				if err != nil {
					return err
				}
				view := showView{
					Project:  project,
					Versions: svc.ListMatchingProjectVersions(project, runtime.MinecraftVersion, runtime.ModLoader),
				}
				return r.write(c, view, writeShow)
			})
		},
	}
}

func (r runner) installCommand() *cli.Command {
	return &cli.Command{
		Name:      "install",
		Aliases:   []string{"i"},
		Usage:     "Install projects into the current mods directory; falls back to keyword search when no exact slug or ID matches",
		ArgsUsage: "<project...>",
		Flags: []cli.Flag{
			jsonFlag(),
			&cli.StringFlag{Name: "project", Usage: "project ID, slug, or platform-prefixed reference"},
			&cli.StringFlag{Name: "platform", Usage: "project platform: modrinth or curseforge"},
			&cli.StringFlag{Name: "version-id", Usage: "advanced: explicit platform version ID for a single project (prefer automatic derivation)"},
			&cli.StringFlag{Name: "requirement", Aliases: []string{"r"}, Usage: "install mods listed in a requirements file, like pip install -r"},
			&cli.BoolFlag{Name: "force", Usage: "allow running outside a directory named mods"},
			&cli.BoolFlag{Name: "yes", Aliases: []string{"y"}, Usage: "non-interactive: accept automatic choices without prompting"},
			&cli.BoolFlag{Name: "no-input", Usage: "non-interactive: never open the interactive selector"},
			&cli.BoolFlag{Name: "dry-run", Usage: "print the resolved install plan without downloading or writing files"},
			&cli.DurationFlag{Name: "timeout", Usage: "optional install timeout, for example 2m"},
			&cli.BoolFlag{Name: "server", Usage: "start the local HTTP receiver on 127.0.0.1:18801 and queue received mods for install"},
		},
		Action: func(c *cli.Context) error {
			runtime, err := r.runtimeInput(c, true)
			if err != nil {
				return err
			}
			if err := requireVersionLoader(runtime); err != nil {
				return err
			}
			if err := validateModsWorkDir(runtime.TargetModsDir, c.Bool("force"), runtime.DirExplicit); err != nil {
				return err
			}
			refs := projectRefs(c)
			platformFilter := normalizePlatformName(c.String("platform"))
			if requirement := strings.TrimSpace(c.String("requirement")); requirement != "" {
				if err := requirementInstallConflict(c, refs, platformFilter); err != nil {
					return err
				}
				return r.withService(c, runtime, func(ctx context.Context, svc *appcore.Service) error {
					if timeout := c.Duration("timeout"); timeout > 0 {
						var cancel context.CancelFunc
						ctx, cancel = context.WithTimeout(ctx, timeout)
						defer cancel()
					}
					return r.installRequirements(ctx, c, svc, runtime, requirement)
				})
			}
			if c.Bool("server") {
				if len(refs) > 0 {
					return errors.New("--server cannot be combined with project arguments")
				}
				if c.Bool("dry-run") {
					return errors.New("--server cannot be combined with --dry-run")
				}
				if strings.TrimSpace(c.String("version-id")) != "" {
					return errors.New("--server cannot be combined with --version-id; use the payload's file field instead")
				}
				return r.withService(c, runtime, func(ctx context.Context, svc *appcore.Service) error {
					return r.serveInstall(ctx, svc, runtime)
				})
			}
			wizardMode := len(refs) == 0 && interactiveAllowed(c) && !c.Bool("json") && !c.Bool("dry-run")
			multiTarget := false
			targets := []InstallTarget{}
			if !wizardMode {
				installRefs := refs
				if len(refs) > 1 {
					installRefs = splitInstallRefs(refs)
				}
				multiTarget = len(installRefs) > 1
				targets, err = ParseInstallTargets(installRefs, "")
				if err != nil {
					return err
				}
			}
			if err := versionIDExclusivity(len(targets), c.String("version-id")); err != nil {
				return err
			}
			return r.withService(c, runtime, func(ctx context.Context, svc *appcore.Service) error {
				if timeout := c.Duration("timeout"); timeout > 0 {
					var cancel context.CancelFunc
					ctx, cancel = context.WithTimeout(ctx, timeout)
					defer cancel()
				}
				interactive := interactiveAllowed(c)
				if wizardMode {
					wizardTargets, wizardErr := r.runInstallWizard(ctx, svc, runtime, platformFilter)
					if wizardErr != nil {
						if errors.Is(wizardErr, prompt.ErrCanceled) {
							return cli.Exit("canceled", 130)
						}
						return wizardErr
					}
					targets = wizardTargets
				}

				// Phase 2: resolve every target to exactly one project. Multi-arg
				// installs are all-or-nothing: if any token misses, the whole
				// input is retried as one keyword search.
				resolved, err := resolveInstallTargets(svc, targets, runtime, platformFilter, interactive, multiTarget, r.stderr)
				if err != nil {
					return err
				}
				plan := make([]InstallPlanEntry, 0, len(resolved))
				for _, item := range resolved {
					plan = append(plan, buildInstallPlanEntry(svc, item.Target, item.Project, runtime))
				}

				// Phase 5.1: preview the resolved plan (skipped in JSON mode where
				// the structured report is the machine-readable output).
				if !c.Bool("json") {
					_ = r.writeInstallPlan(plan, runtime.MinecraftVersion+" | "+runtime.ModLoader)
				}

				if c.Bool("dry-run") {
					if c.Bool("json") {
						return writeInstallPlanJSON(r.stdout, plan)
					}
					_, _ = fmt.Fprintln(r.stdout, "dry-run: no files were downloaded or written.")
					return nil
				}

				// Phase 5.2: execute the installs with a live progress line, and
				// Phase 5.3: collect a unified report (skipped + errors) instead of
				// aborting on the first failure so every target is accounted for.
				jsonMode := c.Bool("json")
				results := make([]appcore.InstallWaitResult, 0, len(plan))
				report := InstallReport{Targets: targetRaws(plan), Resolved: plan}
				failureReasons := []string{}
				totalBytes := int64(0)
				for _, entry := range plan {
					if entry.FileSize > 0 {
						totalBytes += entry.FileSize
					}
				}
				doneBytes := int64(0)
				for i, entry := range plan {
					result := r.installWithProgress(ctx, svc, appcore.InstallToDirRequest{
						ProjectID:        entry.Project.ID,
						Project:          entry.Project,
						TargetDir:        runtime.TargetModsDir,
						MinecraftVersion: runtime.MinecraftVersion,
						ModLoader:        runtime.ModLoader,
						VersionID:        c.String("version-id"),
					}, jsonMode, i+1, len(plan), firstNonEmptyStr(entry.Project.Title, entry.Project.Slug, entry.Project.ID), doneBytes, totalBytes)
					doneBytes += entry.FileSize
					results = append(results, result)
					if len(result.Errors) > 0 {
						report.Errors = append(report.Errors, result.Errors[0].Reason)
						failureReasons = append(failureReasons, result.Errors[0].Reason)
						continue
					}
					if result.Result.Skipped {
						report.Skipped = append(report.Skipped, InstallSkipped{Target: entry.Target, Reason: result.Result.Reason})
						failureReasons = append(failureReasons, result.Result.Reason)
					}
				}
				if jsonMode {
					if err := writeInstallReportJSON(r.stdout, report); err != nil {
						return err
					}
				} else if len(results) == 1 {
					if err := r.write(c, results[0], writeInstallResult); err != nil {
						return err
					}
				} else if err := r.write(c, results, writeInstallResults); err != nil {
					return err
				}
				if len(failureReasons) > 0 {
					return cli.Exit(strings.Join(failureReasons, "; "), 1)
				}
				return nil
			})
		},
	}
}

func (r runner) listCommand() *cli.Command {
	return &cli.Command{
		Name:    "list",
		Aliases: []string{"mods"},
		Usage:   "List installed mods in the current mods directory",
		Flags: []cli.Flag{
			jsonFlag(),
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
			return r.withService(c, runtime, func(ctx context.Context, svc *appcore.Service) error {
				mods, err := svc.LocalModsInDir(runtime.TargetModsDir, runtime.MinecraftVersion, runtime.ModLoader)
				if err != nil {
					return err
				}
				return r.write(c, mods, writeMods)
			})
		},
	}
}

func (r runner) removeCommand() *cli.Command {
	return &cli.Command{
		Name:      "remove",
		Usage:     "Remove installed mods from the current mods directory",
		ArgsUsage: "<mod...>",
		Flags: []cli.Flag{
			jsonFlag(),
			&cli.BoolFlag{Name: "dry-run", Usage: "print matching files without deleting them"},
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
			targets := normalizedArgs(c.Args().Slice())
			if len(targets) == 0 {
				return errors.New("mod is required")
			}
			return r.withService(c, runtime, func(ctx context.Context, svc *appcore.Service) error {
				mods, err := svc.LocalModsInDir(runtime.TargetModsDir, runtime.MinecraftVersion, runtime.ModLoader)
				if err != nil {
					return err
				}
				removed := planRemovedMods(runtime.TargetModsDir, mods, targets)
				if len(removed) == 0 {
					return cli.Exit("no matching installed mods", 1)
				}
				dryRun := c.Bool("dry-run")
				if !dryRun {
					paths := make([]string, 0, len(removed))
					for _, mod := range removed {
						paths = append(paths, mod.Path)
					}
					if _, err := svc.ApplyLocalModBatchOperation(appstructs.LocalModBatchOperationRequest{
						Paths:            paths,
						Action:           appcore.LocalModBatchActionDelete,
						TargetDir:        runtime.TargetModsDir,
						MinecraftVersion: runtime.MinecraftVersion,
						ModLoader:        runtime.ModLoader,
					}); err != nil {
						return err
					}
				}
				for i := range removed {
					removed[i].DryRun = dryRun
				}
				return r.write(c, removed, writeRemovedMods)
			})
		},
	}
}

func (r runner) cacheCommand() *cli.Command {
	return &cli.Command{
		Name:  "cache",
		Usage: "Manage metadata cache",
		Subcommands: []*cli.Command{
			{
				Name:  "clean",
				Usage: "Delete the metadata cache file",
				Flags: []cli.Flag{jsonFlag()},
				Action: func(c *cli.Context) error {
					cachePath, err := cacheFilePath(c)
					if err != nil {
						return err
					}
					err = os.Remove(cachePath)
					if err != nil && !os.IsNotExist(err) {
						return err
					}
					result := map[string]any{
						"path":    cachePath,
						"removed": err == nil,
					}
					return r.write(c, result, writeCacheClean)
				},
			},
		},
	}
}

func (r runner) withService(c *cli.Context, runtime runtimeInput, fn func(context.Context, *appcore.Service) error) error {
	cfg, _ := loadUserConfig()
	overrides := appcore.ConfigOverrides{}
	if key := firstNonEmptyString(c.String("curseforge-api-key"), cfg.CurseForgeAPIKey); key != "" {
		overrides.CurseForgeAPIKey = key
		overrides.HasCurseForgeAPIKey = true
	}
	if key := firstNonEmptyString(c.String("modrinth-api-key"), cfg.ModrinthAPIKey); key != "" {
		overrides.ModrinthAPIKey = key
		overrides.HasModrinthAPIKey = true
	}
	svc := appcore.New(appcore.Options{
		ConfigOverrides: overrides,
		Runtime: appcore.RuntimeOptions{
			WorkDir:          runtime.WorkDir,
			TargetModsDir:    runtime.TargetModsDir,
			MinecraftVersion: runtime.MinecraftVersion,
			ModLoader:        runtime.ModLoader,
			CacheDir:         runtime.CacheDir,
			NoConfigFile:     true,
		},
		LogLevel: cliLogLevel(c),
	})
	ctx := c.Context
	if ctx == nil {
		ctx = context.Background()
	}
	// Register cleanup before Startup: Startup itself releases partially
	// initialized resources on failure, and Close is idempotent.
	defer svc.Close()
	if err := svc.Startup(ctx); err != nil {
		return err
	}
	return fn(ctx, svc)
}

func (r runner) runtimeInput(c *cli.Context, targetCurrentDir bool) (runtimeInput, error) {
	wd, err := os.Getwd()
	if err != nil {
		return runtimeInput{}, err
	}
	cfg, _ := loadUserConfig()
	runtime := runtimeInput{
		WorkDir:          wd,
		MinecraftVersion: strings.TrimSpace(c.String("mc-version")),
		ModLoader:        normalizeLoader(c.String("loader")),
		CacheDir:         strings.TrimSpace(c.String("cache-dir")),
	}
	// Config file values are the lowest-priority fallback above auto-detection,
	// but below explicit flags and environment variables.
	if runtime.MinecraftVersion == "" {
		runtime.MinecraftVersion = strings.TrimSpace(cfg.MinecraftVersion)
	}
	if runtime.ModLoader == "" {
		runtime.ModLoader = normalizeLoader(cfg.Loader)
	}
	if runtime.CacheDir == "" {
		runtime.CacheDir = strings.TrimSpace(cfg.CacheDir)
	}

	dir := firstNonEmptyString(c.String("dir"), cfg.MinecraftDir)
	if dir != "" {
		modsDir, info, ok, err := locateRuntimeDir(dir)
		if err != nil {
			return runtimeInput{}, err
		}
		if ok {
			runtime.TargetModsDir = modsDir
			runtime.WorkDir = modsDir
			runtime.DirExplicit = true
			if runtime.MinecraftVersion == "" {
				runtime.MinecraftVersion = strings.TrimSpace(info.MinecraftVersion)
			}
			if runtime.ModLoader == "" {
				runtime.ModLoader = normalizeLoader(info.ModLoader)
			}
		}
	}

	if targetCurrentDir && runtime.TargetModsDir == "" {
		runtime.TargetModsDir = wd
	}
	if runtime.MinecraftVersion == "" || runtime.ModLoader == "" {
		inferDir := runtime.TargetModsDir
		if inferDir == "" {
			inferDir = wd
		}
		inferred := inferRuntimeFromModsParent(inferDir)
		if runtime.MinecraftVersion == "" {
			runtime.MinecraftVersion = inferred.MinecraftVersion
		}
		if runtime.ModLoader == "" {
			runtime.ModLoader = inferred.ModLoader
		}
	}
	if runtime.ModLoader != "" && !validLoader(runtime.ModLoader) {
		return runtimeInput{}, fmt.Errorf("invalid loader %q: expected fabric, forge, or neoforge", runtime.ModLoader)
	}
	return runtime, nil
}

// locateRuntimeDir resolves a user-supplied --dir value into a concrete mods
// directory, accepting either the mods folder itself, a version directory that
// contains a mods/ folder, or a Minecraft game / Prism instance directory. It
// also returns the detected version/loader when available.
func locateRuntimeDir(dir string) (string, structs.VersionInfo, bool, error) {
	dir = cleanAbsPath(dir)
	if dir == "" {
		return "", structs.VersionInfo{}, false, nil
	}
	info, err := os.Stat(dir)
	if err != nil {
		return "", structs.VersionInfo{}, false, fmt.Errorf("--dir path is not accessible: %s", dir)
	}
	if !info.IsDir() {
		return "", structs.VersionInfo{}, false, fmt.Errorf("--dir path is not a directory: %s", dir)
	}

	if filepath.Base(dir) == "mods" {
		return dir, inferVersionInfoForResolvedModsDir(dir), true, nil
	}
	if modsDir := filepath.Join(dir, "mods"); isDirectory(modsDir) {
		return modsDir, inferVersionInfoForResolvedModsDir(modsDir), true, nil
	}

	for _, gameDir := range candidateGameDirs(dir) {
		versions := minecraft.LoadLauncherVersions(gameDir, minecraft.LoadGameDirVersions)
		for _, version := range versions {
			if !appcore.ValidMinecraftInstance(version) {
				continue
			}
			versionDir := minecraft.VersionDirPath(gameDir, version)
			if versionDir == "" {
				continue
			}
			return filepath.Join(versionDir, "mods"), version, true, nil
		}
	}

	return "", structs.VersionInfo{}, false, fmt.Errorf(
		"no mods directory found under %s: expected a mods folder, a version directory containing mods/, or a Minecraft game directory",
		dir,
	)
}

func candidateGameDirs(dir string) []string {
	candidates := make([]string, 0, 2)
	if nested := filepath.Join(dir, minecraft.PrismInstanceDotMinecraftDirName); isDirectory(nested) {
		candidates = append(candidates, nested)
	}
	return append(candidates, dir)
}

func isDirectory(path string) bool {
	info, err := os.Stat(path)
	return err == nil && info.IsDir()
}

// inferVersionInfoForResolvedModsDir best-effort infers the Minecraft version and
// loader for a mods directory, falling back to reading a sibling manifest.
func inferVersionInfoForResolvedModsDir(modsDir string) structs.VersionInfo {
	if info, ok := inferVersionInfoForModsDir(modsDir); ok {
		return info
	}
	if info, ok := inferVersionInfoFromDir(filepath.Dir(modsDir)); ok {
		return info
	}
	return structs.VersionInfo{}
}

func firstNonEmptyString(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}

func inferRuntimeFromModsParent(workDir string) runtimeInput {
	workDir = strings.TrimSpace(workDir)
	if workDir == "" {
		return runtimeInput{}
	}
	workDir = filepath.Clean(workDir)
	if filepath.Base(workDir) != "mods" {
		return runtimeInput{}
	}
	versionDir := filepath.Dir(workDir)
	info, ok := inferVersionInfoForModsDir(workDir)
	if !ok {
		info, ok = inferVersionInfoFromDir(versionDir)
	}
	if !ok {
		return runtimeInput{}
	}
	runtime := runtimeInput{MinecraftVersion: strings.TrimSpace(info.MinecraftVersion)}
	if loader := normalizeLoader(info.ModLoader); validLoader(loader) {
		runtime.ModLoader = loader
	}
	return runtime
}

func inferVersionInfoForModsDir(modsDir string) (structs.VersionInfo, bool) {
	modsDir = strings.TrimSpace(modsDir)
	if modsDir == "" {
		return structs.VersionInfo{}, false
	}
	modsDir = cleanAbsPath(modsDir)
	for _, root := range candidateMinecraftRootsForModsDir(modsDir) {
		versions := minecraft.LoadLauncherVersions(root, minecraft.LoadGameDirVersions)
		for _, version := range versions {
			versionModsDir := filepath.Join(minecraft.VersionDirPath(root, version), "mods")
			if samePath(versionModsDir, modsDir) {
				return version, true
			}
		}
	}
	return structs.VersionInfo{}, false
}

func candidateMinecraftRootsForModsDir(modsDir string) []string {
	versionDir := filepath.Dir(modsDir)
	versionsDir := filepath.Dir(versionDir)
	gameDir := filepath.Dir(versionsDir)

	candidates := make([]string, 0, 4)
	if gameDir != "." && gameDir != string(filepath.Separator) {
		prismRoots := []string{filepath.Dir(gameDir)}
		if filepath.Base(gameDir) == minecraft.PrismInstanceDotMinecraftDirName {
			prismRoots = append([]string{filepath.Dir(filepath.Dir(gameDir))}, prismRoots...)
		}
		for _, root := range prismRoots {
			if minecraft.IsPrismInstancesDir(root) {
				candidates = appendUniquePath(candidates, root)
			}
		}
		candidates = appendUniquePath(candidates, gameDir)
	}
	return candidates
}

func inferVersionInfoFromDir(versionDir string) (structs.VersionInfo, bool) {
	versionDir = strings.TrimSpace(versionDir)
	if versionDir == "" {
		return structs.VersionInfo{}, false
	}
	versionDir = filepath.Clean(versionDir)

	candidates := make([]string, 0)
	base := filepath.Base(versionDir)
	baseManifest := base + ".json"
	candidates = append(candidates, baseManifest)

	entries, err := os.ReadDir(versionDir)
	if err == nil {
		for _, entry := range entries {
			if entry.IsDir() || strings.ToLower(filepath.Ext(entry.Name())) != ".json" || entry.Name() == baseManifest {
				continue
			}
			candidates = append(candidates, entry.Name())
		}
	}

	var fallback structs.VersionInfo
	for _, candidate := range candidates {
		info, ok := minecraft.CheckManifest(filepath.Join(versionDir, candidate))
		if !ok {
			continue
		}
		if fallback.ID == "" && fallback.Name == "" {
			fallback = info
		}
		if validLoader(info.ModLoader) {
			return info, true
		}
	}
	if fallback.ID != "" || fallback.Name != "" {
		return fallback, true
	}
	return structs.VersionInfo{}, false
}

func appendUniquePath(paths []string, path string) []string {
	path = cleanAbsPath(path)
	if path == "" {
		return paths
	}
	for _, existing := range paths {
		if samePath(existing, path) {
			return paths
		}
	}
	return append(paths, path)
}

func samePath(a, b string) bool {
	return cleanAbsPath(a) == cleanAbsPath(b)
}

func cleanAbsPath(path string) string {
	path = strings.TrimSpace(path)
	if path == "" {
		return ""
	}
	if abs, err := filepath.Abs(path); err == nil {
		path = abs
	}
	return filepath.Clean(path)
}

func jsonFlag() cli.Flag {
	return &cli.BoolFlag{Name: "json", Usage: "write structured JSON output"}
}

func verboseFlag() cli.Flag {
	return &cli.BoolFlag{Name: "verbose", Aliases: []string{"v"}, Usage: "enable debug logging"}
}

func (r runner) write(c *cli.Context, value any, human func(io.Writer, any) error) error {
	if c.Bool("json") {
		encoder := json.NewEncoder(r.stdout)
		encoder.SetIndent("", "  ")
		return encoder.Encode(value)
	}
	return human(r.stdout, value)
}

func resolveProject(c *cli.Context, svc *appcore.Service, runtime runtimeInput) (models.ModProject, error) {
	ref := strings.TrimSpace(c.String("project"))
	if ref == "" {
		ref = strings.TrimSpace(c.Args().First())
	}
	return resolveProjectRef(ref, c.String("platform"), svc, runtime)
}

func resolveProjectRef(ref, platform string, svc *appcore.Service, runtime runtimeInput) (models.ModProject, error) {
	ref = strings.TrimSpace(ref)
	if ref == "" {
		return models.ModProject{}, errors.New("project is required")
	}
	platform = strings.ToLower(strings.TrimSpace(platform))
	parsedPlatform, projectID := models.ParseProjectKey(ref)
	if platform == "" {
		platform = parsedPlatform
	}
	if parsedPlatform != "" {
		ref = projectID
	}
	if platform == "" {
		return models.ModProject{}, errors.New("platform is required when project is not prefixed")
	}
	project, ok := svc.LookupProject(platform, ref, runtime.MinecraftVersion, runtime.ModLoader)
	if ok {
		return project, nil
	}
	return models.ModProject{
		ID:        models.ProjectKey(platform, ref),
		Platform:  platform,
		ProjectID: ref,
		Slug:      ref,
		Title:     ref,
	}, nil
}

func projectRefs(c *cli.Context) []string {
	refs := normalizedArgs(c.Args().Slice())
	if project := strings.TrimSpace(c.String("project")); project != "" {
		refs = append([]string{project}, refs...)
	}
	return refs
}

// splitInstallRefs expands multi-argument installs into whitespace tokens so
// `install a b c` behaves like pip install, while a single quoted argument such
// as `install "mini map"` stays one search query.
func splitInstallRefs(refs []string) []string {
	out := make([]string, 0, len(refs))
	for _, ref := range refs {
		out = append(out, strings.Fields(ref)...)
	}
	return out
}

func normalizedArgs(args []string) []string {
	out := make([]string, 0, len(args))
	for _, arg := range args {
		arg = strings.TrimSpace(arg)
		if arg != "" {
			out = append(out, arg)
		}
	}
	return out
}

func filterProjectsByPlatform(projects []models.ModProject, platform string) []models.ModProject {
	platform = strings.ToLower(strings.TrimSpace(platform))
	if platform == "" {
		return projects
	}
	out := make([]models.ModProject, 0, len(projects))
	for _, project := range projects {
		if strings.ToLower(strings.TrimSpace(project.Platform)) == platform {
			out = append(out, project)
		}
	}
	return out
}

func requireVersionLoader(runtime runtimeInput) error {
	if strings.TrimSpace(runtime.MinecraftVersion) == "" {
		return errors.New("mc-version is required; pass --mc-version or set MINECRAFT_VERSION")
	}
	if strings.TrimSpace(runtime.ModLoader) == "" {
		return errors.New("loader is required; pass --loader or set MOD_LOADER")
	}
	return nil
}

func validateModsWorkDir(dir string, force bool, dirExplicit bool) error {
	if strings.TrimSpace(dir) == "" {
		return errors.New("empty mods directory")
	}
	info, err := os.Stat(dir)
	if err != nil {
		return err
	}
	if !info.IsDir() {
		return fmt.Errorf("mods path is not a directory: %s", dir)
	}
	if !force && !dirExplicit && filepath.Base(filepath.Clean(dir)) != "mods" {
		return errors.New("current directory must be a mods directory; cd into your instance's mods folder or pass --force")
	}
	return nil
}

func normalizeLoader(loader string) string {
	return strings.ToLower(strings.TrimSpace(loader))
}

func validLoader(loader string) bool {
	switch normalizeLoader(loader) {
	case "fabric", "forge", "neoforge":
		return true
	default:
		return false
	}
}

func cacheFilePath(c *cli.Context) (string, error) {
	if cacheDir := strings.TrimSpace(c.String("cache-dir")); cacheDir != "" {
		return filepath.Join(cacheDir, storage.CacheFileName), nil
	}
	return storage.DefaultCachePath()
}

// planRemovedMods resolves the target selectors to physical jar paths. The
// actual deletion is delegated to appcore.ApplyLocalModBatchOperation so the
// CLI gets the same path validation, index cleanup and usage stats as the GUI
// and TUI.
func planRemovedMods(modsDir string, mods []structs.ModInfo, targets []string) []removedMod {
	targetSet := make(map[string]bool, len(targets))
	for _, target := range targets {
		targetSet[normalizeModRef(target)] = true
	}

	var removed []removedMod
	seen := make(map[string]bool)
	for _, mod := range mods {
		if !matchesAnyModTarget(mod, targetSet) {
			continue
		}
		path := mod.Path
		if strings.TrimSpace(path) == "" {
			path = filepath.Join(modsDir, mod.FileName)
		}
		if !filepath.IsAbs(path) {
			path = filepath.Join(modsDir, path)
		}
		path = filepath.Clean(path)
		if seen[path] {
			continue
		}
		seen[path] = true
		removed = append(removed, removedMod{
			ID:       mod.ID,
			Name:     mod.Name,
			FileName: mod.FileName,
			Path:     path,
		})
	}
	return removed
}

func matchesAnyModTarget(mod structs.ModInfo, targets map[string]bool) bool {
	candidates := []string{
		mod.ID,
		mod.Name,
		mod.FileName,
		strings.TrimSuffix(mod.FileName, filepath.Ext(mod.FileName)),
	}
	for _, candidate := range candidates {
		if targets[normalizeModRef(candidate)] {
			return true
		}
	}
	return false
}

func normalizeModRef(value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	value = strings.ToLower(minecraft.EnableModPath(value))
	value = strings.TrimSuffix(value, ".jar")
	return value
}

func writeShow(w io.Writer, value any) error {
	view := value.(showView)
	tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
	fmt.Fprintf(tw, "ID\t%s\n", view.Project.ID)
	fmt.Fprintf(tw, "PLATFORM\t%s\n", view.Project.Platform)
	fmt.Fprintf(tw, "TITLE\t%s\n", view.Project.Title)
	fmt.Fprintf(tw, "SLUG\t%s\n", view.Project.Slug)
	fmt.Fprintln(tw)
	fmt.Fprintln(tw, "VERSION_ID\tNAME\tFILE\tDOWNLOADS")
	for _, version := range view.Versions {
		fmt.Fprintf(tw, "%s\t%s\t%s\t%d\n", version.ID, version.Name, version.FileName, version.Downloads)
	}
	return tw.Flush()
}

func writeInstallResult(w io.Writer, value any) error {
	result := value.(appcore.InstallWaitResult)
	if result.Result.Queued {
		fmt.Fprintf(w, "installed %s (%s)\n", result.Result.FileName, result.Result.VersionID)
		return nil
	}
	if result.Result.Skipped {
		fmt.Fprintf(w, "skipped: %s\n", result.Result.Reason)
		return nil
	}
	fmt.Fprintln(w, "no download queued")
	return nil
}

func writeInstallResults(w io.Writer, value any) error {
	results := value.([]appcore.InstallWaitResult)
	for _, result := range results {
		if err := writeInstallResult(w, result); err != nil {
			return err
		}
	}
	return nil
}

func writeMods(w io.Writer, value any) error {
	mods := value.([]structs.ModInfo)
	tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
	fmt.Fprintln(tw, "ID\tNAME\tVERSION\tFILE\tENABLED")
	for _, mod := range mods {
		fmt.Fprintf(tw, "%s\t%s\t%s\t%s\t%t\n", mod.ID, mod.Name, mod.Version, mod.FileName, mod.Enabled)
	}
	return tw.Flush()
}

func writeRemovedMods(w io.Writer, value any) error {
	removed := value.([]removedMod)
	tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
	fmt.Fprintln(tw, "ID\tNAME\tFILE\tREMOVED")
	for _, mod := range removed {
		fmt.Fprintf(tw, "%s\t%s\t%s\t%t\n", mod.ID, mod.Name, mod.FileName, !mod.DryRun)
	}
	return tw.Flush()
}

func writeCacheClean(w io.Writer, value any) error {
	result := value.(map[string]any)
	if result["removed"].(bool) {
		fmt.Fprintf(w, "removed %s\n", result["path"])
		return nil
	}
	fmt.Fprintf(w, "cache not found: %s\n", result["path"])
	return nil
}
