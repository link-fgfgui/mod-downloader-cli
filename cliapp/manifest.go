package cliapp

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/urfave/cli/v2"

	"github.com/link-fgfgui/mod-downloader-core/appcore"
	"github.com/link-fgfgui/mod-downloader-core/models"
	structs "github.com/link-fgfgui/mod-downloader-core/structs/minecraft"
)

type manifestHeader struct {
	ExportedAt       time.Time
	MinecraftVersion string
	Loader           string
}

// manifestEntry is one line of a mod manifest. Platform/project/version describe
// an online mod; LocalFile describes a mod that could not be matched to a
// platform (serialized as a comment).
type manifestEntry struct {
	Platform  string
	Project   string
	Version   string
	LocalFile string
}

func (r runner) exportCommand() *cli.Command {
	return &cli.Command{
		Name:  "export",
		Usage: "Export installed mods to a portable requirements-style manifest",
		Flags: []cli.Flag{
			jsonFlag(),
			&cli.StringFlag{Name: "output", Aliases: []string{"o"}, Usage: "write the manifest to a file instead of stdout"},
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
				entries := buildManifestEntries(mods)
				header := manifestHeader{
					ExportedAt:       time.Now(),
					MinecraftVersion: runtime.MinecraftVersion,
					Loader:           runtime.ModLoader,
				}
				content := formatManifest(entries, header)

				output := strings.TrimSpace(c.String("output"))
				if output == "" {
					if c.Bool("json") {
						return r.write(c, entries, writeManifestEntries)
					}
					_, err := io.WriteString(r.stdout, content)
					return err
				}
				if err := os.WriteFile(output, []byte(content), 0o644); err != nil {
					return err
				}
				result := map[string]any{"output": output, "count": len(entries)}
				return r.write(c, result, writeExportResult)
			})
		},
	}
}

func buildManifestEntries(mods []structs.ModInfo) []manifestEntry {
	hashes := make([]string, 0)
	seenHash := make(map[string]bool)
	for _, mod := range mods {
		sha1 := strings.ToLower(strings.TrimSpace(mod.SHA1))
		if sha1 == "" || seenHash[sha1] || strings.TrimSpace(mod.OnlinePlatform) != "" {
			continue
		}
		seenHash[sha1] = true
		hashes = append(hashes, sha1)
	}
	var resolved map[string]models.ModProject
	if len(hashes) > 0 {
		resolved = resolveProjectsByHashesFn(hashes)
	}

	entries := make([]manifestEntry, 0)
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
			entries = append(entries, manifestEntry{LocalFile: filepath.Base(path)})
			continue
		}
		projectRef := firstNonEmptyString(project.Slug, project.ProjectID)
		entries = append(entries, manifestEntry{
			Platform: platform,
			Project:  projectRef,
			Version:  currentInstalledVersion(mod, platform, sha1),
		})
	}
	return entries
}

func formatManifest(entries []manifestEntry, header manifestHeader) string {
	var b strings.Builder
	b.WriteString("# mod-downloader-cli mod manifest\n")
	if !header.ExportedAt.IsZero() {
		b.WriteString("# exported: " + header.ExportedAt.UTC().Format(time.RFC3339) + "\n")
	}
	if strings.TrimSpace(header.MinecraftVersion) != "" {
		b.WriteString("# minecraft: " + strings.TrimSpace(header.MinecraftVersion) + "\n")
	}
	if strings.TrimSpace(header.Loader) != "" {
		b.WriteString("# loader: " + strings.TrimSpace(header.Loader) + "\n")
	}
	for _, entry := range entries {
		if strings.TrimSpace(entry.LocalFile) != "" {
			b.WriteString("# local: " + strings.TrimSpace(entry.LocalFile) + "\n")
			continue
		}
		if strings.TrimSpace(entry.Platform) == "" || strings.TrimSpace(entry.Project) == "" {
			continue
		}
		line := strings.ToLower(strings.TrimSpace(entry.Platform)) + ":" + strings.TrimSpace(entry.Project)
		if version := strings.TrimSpace(entry.Version); version != "" {
			line += "==" + version
		}
		b.WriteString(line + "\n")
	}
	return b.String()
}

func parseManifest(text string) ([]manifestEntry, error) {
	entries := make([]manifestEntry, 0)
	for _, raw := range strings.Split(text, "\n") {
		line := strings.TrimSpace(raw)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		platform, project, found := strings.Cut(line, ":")
		if !found {
			return nil, fmt.Errorf("invalid manifest line %q: expected <platform>:<id>", line)
		}
		platform = strings.ToLower(strings.TrimSpace(platform))
		if platform != "modrinth" && platform != "curseforge" {
			return nil, fmt.Errorf("invalid manifest line %q: unknown platform %q", line, platform)
		}
		version := ""
		if idx := strings.Index(project, "=="); idx >= 0 {
			version = strings.TrimSpace(project[idx+2:])
			project = project[:idx]
		} else if idx := strings.Index(project, "="); idx >= 0 {
			version = strings.TrimSpace(project[idx+1:])
			project = project[:idx]
		}
		project = strings.TrimSpace(project)
		if project == "" {
			return nil, fmt.Errorf("invalid manifest line %q: empty project", line)
		}
		entries = append(entries, manifestEntry{Platform: platform, Project: project, Version: version})
	}
	return entries, nil
}

type importResult struct {
	Line   string `json:"line"`
	ID     string `json:"id,omitempty"`
	Status string `json:"status"`
	Note   string `json:"note,omitempty"`
}

func (r runner) importCommand() *cli.Command {
	return &cli.Command{
		Name:      "import",
		Usage:     "Install mods listed in a manifest file",
		ArgsUsage: "<file>",
		Flags: []cli.Flag{
			jsonFlag(),
			&cli.BoolFlag{Name: "skip-existing", Usage: "skip mods already present in the target directory"},
			&cli.DurationFlag{Name: "timeout", Usage: "optional per-mod install timeout, for example 2m"},
			&cli.BoolFlag{Name: "force", Usage: "allow running outside a directory named mods"},
		},
		Action: func(c *cli.Context) error {
			file := strings.TrimSpace(c.Args().First())
			if file == "" {
				return errors.New("manifest file is required")
			}
			data, err := os.ReadFile(file)
			if err != nil {
				return err
			}
			entries, err := parseManifest(string(data))
			if err != nil {
				return err
			}
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
				installed, err := svc.LocalModsInDir(runtime.TargetModsDir, runtime.MinecraftVersion, runtime.ModLoader)
				if err != nil {
					return err
				}
				installedRefs := installedProjectRefs(installed)
				results := make([]importResult, 0, len(entries))
				for _, entry := range entries {
					line := entry.Platform + ":" + entry.Project
					if entry.Version != "" {
						line += "==" + entry.Version
					}
					result := r.importEntry(ctx, svc, runtime, entry, installedRefs, c.Bool("skip-existing"), c.Duration("timeout"))
					result.Line = line
					results = append(results, result)
				}
				return r.write(c, results, writeImportResults)
			})
		},
	}
}

func (r runner) importEntry(ctx context.Context, svc *appcore.Service, runtime runtimeInput, entry manifestEntry, installedRefs map[string]bool, skipExisting bool, timeout time.Duration) importResult {
	project, ok := svc.LookupProject(entry.Platform, entry.Project, runtime.MinecraftVersion, runtime.ModLoader)
	if !ok {
		project = models.ModProject{
			ID:        models.ProjectKey(entry.Platform, entry.Project),
			Platform:  entry.Platform,
			ProjectID: entry.Project,
			Slug:      entry.Project,
			Title:     entry.Project,
		}
	}

	projectRef := strings.ToLower(firstNonEmptyString(project.Slug, project.ProjectID, entry.Project))
	if skipExisting && installedRefs[entry.Platform+":"+projectRef] {
		return importResult{ID: project.ID, Status: "skipped", Note: "already installed"}
	}

	versionID := ""
	if entry.Version != "" {
		versions := svc.ListMatchingProjectVersions(project, runtime.MinecraftVersion, runtime.ModLoader)
		matched := false
		for _, version := range versions {
			if strings.EqualFold(strings.TrimSpace(version.Version), entry.Version) || strings.EqualFold(strings.TrimSpace(version.Name), entry.Version) {
				versionID = version.ID
				matched = true
				break
			}
		}
		if !matched {
			return importResult{ID: project.ID, Status: "failed", Note: fmt.Sprintf("version %q not available for this instance", entry.Version)}
		}
	}

	installCtx := ctx
	var cancel context.CancelFunc
	if timeout > 0 {
		installCtx, cancel = context.WithTimeout(ctx, timeout)
	}
	result := svc.InstallModToDirAndWait(installCtx, appcore.InstallToDirRequest{
		ProjectID:        project.ID,
		Project:          project,
		TargetDir:        runtime.TargetModsDir,
		MinecraftVersion: runtime.MinecraftVersion,
		ModLoader:        runtime.ModLoader,
		VersionID:        versionID,
	})
	if cancel != nil {
		cancel()
	}
	if len(result.Errors) > 0 {
		return importResult{ID: project.ID, Status: "failed", Note: result.Errors[0].Reason}
	}
	if result.Result.Skipped {
		return importResult{ID: project.ID, Status: "skipped", Note: result.Result.Reason}
	}
	return importResult{ID: project.ID, Status: "installed", Note: result.Result.FileName}
}

func installedProjectRefs(mods []structs.ModInfo) map[string]bool {
	refs := make(map[string]bool)
	for _, mod := range mods {
		platform := strings.ToLower(strings.TrimSpace(mod.OnlinePlatform))
		ref := strings.ToLower(firstNonEmptyString(mod.OnlineSlug, mod.OnlineProjectID))
		if platform == "" || ref == "" {
			continue
		}
		refs[platform+":"+ref] = true
	}
	return refs
}

func writeManifestEntries(w io.Writer, value any) error {
	entries := value.([]manifestEntry)
	for _, entry := range entries {
		if entry.LocalFile != "" {
			fmt.Fprintf(w, "# local: %s\n", entry.LocalFile)
			continue
		}
		line := entry.Platform + ":" + entry.Project
		if entry.Version != "" {
			line += "==" + entry.Version
		}
		fmt.Fprintln(w, line)
	}
	return nil
}

func writeExportResult(w io.Writer, value any) error {
	result := value.(map[string]any)
	fmt.Fprintf(w, "exported %d mods to %s\n", result["count"], result["output"])
	return nil
}

func writeImportResults(w io.Writer, value any) error {
	results := value.([]importResult)
	installed, skipped, failed := 0, 0, 0
	for _, result := range results {
		switch result.Status {
		case "installed":
			installed++
		case "skipped":
			skipped++
		case "failed":
			failed++
		}
		note := result.Note
		if note != "" {
			note = " (" + note + ")"
		}
		fmt.Fprintf(w, "%s: %s%s\n", result.Status, result.Line, note)
	}
	fmt.Fprintf(w, "installed %d, skipped %d, failed %d\n", installed, skipped, failed)
	return nil
}
