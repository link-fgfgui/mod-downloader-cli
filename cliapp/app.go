package cliapp

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"text/tabwriter"

	"github.com/urfave/cli/v2"

	"github.com/link-fgfgui/mod-downloader-core/appcore"
	"github.com/link-fgfgui/mod-downloader-core/database"
	"github.com/link-fgfgui/mod-downloader-core/minecraft"
	"github.com/link-fgfgui/mod-downloader-core/models"
	appstructs "github.com/link-fgfgui/mod-downloader-core/structs"
	structs "github.com/link-fgfgui/mod-downloader-core/structs/minecraft"
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

type searchTarget struct {
	Query string
	Mode  string
}

const (
	searchModeText = "search"
	searchModeSlug = "slug"
	searchModeID   = "id"
)

func New(stdout, stderr io.Writer) *cli.App {
	r := runner{stdout: stdout, stderr: stderr}
	app := &cli.App{
		Name:  "mod-downloader-cli",
		Usage: "Search and install Minecraft mods from inside a mods directory",
		Flags: []cli.Flag{
			jsonFlag(),
			&cli.StringFlag{Name: "mc-version", Aliases: []string{"minecraft-version", "version"}, Usage: "Minecraft version; auto-detected from the current mods path when omitted", EnvVars: []string{"MINECRAFT_VERSION"}},
			&cli.StringFlag{Name: "loader", Usage: "mod loader: fabric, forge, or neoforge; auto-detected from the current mods path when omitted", EnvVars: []string{"MOD_LOADER"}},
			&cli.StringFlag{Name: "cache-dir", Usage: "metadata cache directory", EnvVars: []string{"MOD_DOWNLOADER_CACHE_DIR"}},
			&cli.StringFlag{Name: "curseforge-api-key", Usage: "CurseForge API key", EnvVars: []string{"CF_API_KEY"}},
			&cli.StringFlag{Name: "modrinth-api-key", Usage: "Modrinth API key", EnvVars: []string{"MODRINTH_API_KEY"}},
		},
		Commands: []*cli.Command{
			r.searchCommand(),
			r.showCommand(),
			r.installCommand(),
			r.listCommand(),
			r.removeCommand(),
			r.cacheCommand(),
		},
	}
	app.Writer = stdout
	app.ErrWriter = stderr
	return app
}

func (r runner) searchCommand() *cli.Command {
	return &cli.Command{
		Name:      "search",
		Usage:     "Search Modrinth and configured CurseForge metadata",
		ArgsUsage: "[<query>]",
		Flags: []cli.Flag{
			jsonFlag(),
			&cli.StringFlag{Name: "query", Usage: "search query; defaults to the first positional argument"},
			&cli.StringFlag{Name: "slug", Usage: "match an exact project slug"},
			&cli.StringFlag{Name: "id", Usage: "match an exact project ID"},
			&cli.StringFlag{Name: "platform", Usage: "provider filter: modrinth or curseforge"},
			&cli.IntFlag{Name: "limit", Value: 10, Usage: "maximum result count per provider"},
			&cli.IntFlag{Name: "offset", Usage: "result offset"},
		},
		Action: func(c *cli.Context) error {
			target, err := searchTargetFromContext(c)
			if err != nil {
				return err
			}
			runtime, err := r.runtimeInput(c, false)
			if err != nil {
				return err
			}
			if err := requireVersionLoader(runtime); err != nil {
				return err
			}
			return r.withService(c, runtime, func(ctx context.Context, svc *appcore.Service) error {
				projects := searchProjects(c, svc, runtime, target)
				return r.write(c, projects, writeProjects)
			})
		},
	}
}

func searchTargetFromContext(c *cli.Context) (searchTarget, error) {
	type candidate struct {
		name  string
		query string
		mode  string
	}
	candidates := make([]candidate, 0, 4)
	if query := strings.TrimSpace(c.String("query")); query != "" {
		candidates = append(candidates, candidate{name: "--query", query: query, mode: searchModeText})
	}
	if arg := strings.TrimSpace(c.Args().First()); arg != "" {
		candidates = append(candidates, candidate{name: "<query>", query: arg, mode: searchModeText})
	}
	if slug := strings.TrimSpace(c.String("slug")); slug != "" {
		candidates = append(candidates, candidate{name: "--slug", query: slug, mode: searchModeSlug})
	}
	if id := strings.TrimSpace(c.String("id")); id != "" {
		candidates = append(candidates, candidate{name: "--id", query: id, mode: searchModeID})
	}
	if len(candidates) == 0 {
		return searchTarget{}, errors.New("query, --slug, or --id is required")
	}
	if len(candidates) > 1 {
		names := make([]string, 0, len(candidates))
		for _, candidate := range candidates {
			names = append(names, candidate.name)
		}
		return searchTarget{}, fmt.Errorf("pass only one search target: %s", strings.Join(names, ", "))
	}
	return searchTarget{Query: candidates[0].query, Mode: candidates[0].mode}, nil
}

func searchProjects(c *cli.Context, svc *appcore.Service, runtime runtimeInput, target searchTarget) []models.ModProject {
	if target.Mode == searchModeText {
		update := svc.SearchModsTextCollect(appstructs.SearchModsRequest{
			Query:     target.Query,
			Version:   runtime.MinecraftVersion,
			ModLoader: runtime.ModLoader,
			Limit:     c.Int("limit"),
			Offset:    c.Int("offset"),
		})
		return filterProjectsByPlatform(update.Results, c.String("platform"))
	}

	platforms := []string{strings.ToLower(strings.TrimSpace(c.String("platform")))}
	if platforms[0] == "" {
		platforms = []string{"modrinth", "curseforge"}
	}

	projects := make([]models.ModProject, 0, len(platforms))
	for _, platform := range platforms {
		var project models.ModProject
		var ok bool
		switch target.Mode {
		case searchModeSlug:
			project, ok = svc.LookupProjectBySlug(platform, target.Query, runtime.MinecraftVersion, runtime.ModLoader)
		case searchModeID:
			project, ok = svc.LookupProjectByID(platform, target.Query, runtime.MinecraftVersion, runtime.ModLoader)
		}
		if ok {
			projects = append(projects, project)
		}
	}
	return projects
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
		Usage:     "Install projects into the current mods directory",
		ArgsUsage: "<project...>",
		Flags: []cli.Flag{
			jsonFlag(),
			&cli.StringFlag{Name: "project", Usage: "project ID, slug, or platform-prefixed reference"},
			&cli.StringFlag{Name: "platform", Usage: "project platform: modrinth or curseforge"},
			&cli.StringFlag{Name: "version-id", Usage: "explicit platform version ID to install"},
			&cli.BoolFlag{Name: "force", Usage: "allow running outside a directory named mods"},
			&cli.DurationFlag{Name: "timeout", Usage: "optional install timeout, for example 2m"},
		},
		Action: func(c *cli.Context) error {
			runtime, err := r.runtimeInput(c, true)
			if err != nil {
				return err
			}
			if err := requireVersionLoader(runtime); err != nil {
				return err
			}
			if err := validateModsWorkDir(runtime.TargetModsDir, c.Bool("force")); err != nil {
				return err
			}
			return r.withService(c, runtime, func(ctx context.Context, svc *appcore.Service) error {
				if timeout := c.Duration("timeout"); timeout > 0 {
					var cancel context.CancelFunc
					ctx, cancel = context.WithTimeout(ctx, timeout)
					defer cancel()
				}

				refs := projectRefs(c)
				if len(refs) == 0 {
					return errors.New("project is required")
				}
				results := make([]appcore.InstallWaitResult, 0, len(refs))
				for _, ref := range refs {
					project, err := resolveProjectRef(ref, c.String("platform"), svc, runtime)
					if err != nil {
						return err
					}
					result := svc.InstallModToDirAndWait(ctx, appcore.InstallToDirRequest{
						ProjectID:        project.ID,
						Project:          project,
						TargetDir:        runtime.TargetModsDir,
						MinecraftVersion: runtime.MinecraftVersion,
						ModLoader:        runtime.ModLoader,
						VersionID:        c.String("version-id"),
					})
					if len(result.Errors) > 0 {
						return cli.Exit(result.Errors[0].Reason, 1)
					}
					if result.Result.Skipped {
						return cli.Exit(result.Result.Reason, 1)
					}
					results = append(results, result)
				}
				if len(results) == 1 {
					return r.write(c, results[0], writeInstallResult)
				}
				return r.write(c, results, writeInstallResults)
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
			if err := validateModsWorkDir(runtime.TargetModsDir, c.Bool("force")); err != nil {
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
			if err := validateModsWorkDir(runtime.TargetModsDir, c.Bool("force")); err != nil {
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
				removed, err := removeMatchingMods(runtime.TargetModsDir, mods, targets, c.Bool("dry-run"))
				if err != nil {
					return err
				}
				if len(removed) == 0 {
					return cli.Exit("no matching installed mods", 1)
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
	overrides := appcore.ConfigOverrides{}
	if c.IsSet("curseforge-api-key") {
		overrides.CurseForgeAPIKey = c.String("curseforge-api-key")
		overrides.HasCurseForgeAPIKey = true
	}
	if c.IsSet("modrinth-api-key") {
		overrides.ModrinthAPIKey = c.String("modrinth-api-key")
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
	})
	ctx := c.Context
	if ctx == nil {
		ctx = context.Background()
	}
	if err := svc.Startup(ctx); err != nil {
		return err
	}
	defer svc.Close()
	return fn(ctx, svc)
}

func (r runner) runtimeInput(c *cli.Context, targetCurrentDir bool) (runtimeInput, error) {
	wd, err := os.Getwd()
	if err != nil {
		return runtimeInput{}, err
	}
	runtime := runtimeInput{
		WorkDir:          wd,
		MinecraftVersion: strings.TrimSpace(c.String("mc-version")),
		ModLoader:        normalizeLoader(c.String("loader")),
		CacheDir:         strings.TrimSpace(c.String("cache-dir")),
	}
	if targetCurrentDir {
		runtime.TargetModsDir = wd
	}
	if runtime.MinecraftVersion == "" || runtime.ModLoader == "" {
		inferred := inferRuntimeFromModsParent(wd)
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
		versions := appcore.New(appcore.Options{}).LoadVersionsFromDisk(root)
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
		if filepath.Base(gameDir) == minecraft.PrismInstanceGameDirName {
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

func validateModsWorkDir(dir string, force bool) error {
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
	if !force && filepath.Base(filepath.Clean(dir)) != "mods" {
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
		return filepath.Join(cacheDir, database.CacheFileName), nil
	}
	return database.DefaultCachePath()
}

func removeMatchingMods(modsDir string, mods []structs.ModInfo, targets []string, dryRun bool) ([]removedMod, error) {
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
		if !dryRun {
			if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
				return nil, err
			}
		}
		removed = append(removed, removedMod{
			ID:       mod.ID,
			Name:     mod.Name,
			FileName: mod.FileName,
			Path:     path,
			DryRun:   dryRun,
		})
	}
	return removed, nil
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
	value = strings.TrimSuffix(value, ".disabled")
	value = strings.TrimSuffix(value, ".jar")
	return value
}

func writeProjects(w io.Writer, value any) error {
	projects := value.([]models.ModProject)
	tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
	fmt.Fprintln(tw, "ID\tSLUG\tTITLE\tPLATFORM")
	for _, project := range projects {
		fmt.Fprintf(tw, "%s\t%s\t%s\t%s\n", project.ID, project.Slug, project.Title, project.Platform)
	}
	return tw.Flush()
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
