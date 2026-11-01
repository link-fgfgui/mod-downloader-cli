package cliapp

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/urfave/cli/v2"

	"github.com/link-fgfgui/mod-downloader/core/appcore"
	"github.com/link-fgfgui/mod-downloader/core/models"
	structs "github.com/link-fgfgui/mod-downloader/core/structs/minecraft"
)

// requirement is one line of a pip-style requirements list. Platform/project/
// version describe an online mod; LocalFile describes a mod that could not be
// matched to a platform (serialized as a comment).
type requirement struct {
	Platform  string
	Project   string
	Version   string
	LocalFile string
}

func (r requirement) String() string {
	if local := strings.TrimSpace(r.LocalFile); local != "" {
		return "# local: " + local
	}
	line := strings.ToLower(strings.TrimSpace(r.Platform)) + ":" + strings.TrimSpace(r.Project)
	if version := strings.TrimSpace(r.Version); version != "" {
		line += "==" + version
	}
	return line
}

func (r runner) freezeCommand() *cli.Command {
	return &cli.Command{
		Name:  "freeze",
		Usage: "Print installed mods as a pip-style requirements list",
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
				return r.write(c, buildRequirements(mods), writeRequirements)
			})
		},
	}
}

func buildRequirements(mods []structs.ModInfo) []requirement {
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

	entries := make([]requirement, 0)
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
			entries = append(entries, requirement{LocalFile: filepath.Base(path)})
			continue
		}
		projectRef := firstNonEmptyString(project.Slug, project.ProjectID)
		entries = append(entries, requirement{
			Platform: platform,
			Project:  projectRef,
			Version:  currentInstalledVersion(mod, platform, sha1),
		})
	}
	return entries
}

func parseRequirements(text string) ([]requirement, error) {
	entries := make([]requirement, 0)
	for _, raw := range strings.Split(text, "\n") {
		line := strings.TrimSpace(raw)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		platform, project, found := strings.Cut(line, ":")
		if !found {
			return nil, fmt.Errorf("invalid requirement line %q: expected <platform>:<id>", line)
		}
		platform = strings.ToLower(strings.TrimSpace(platform))
		if platform != "modrinth" && platform != "curseforge" {
			return nil, fmt.Errorf("invalid requirement line %q: unknown platform %q", line, platform)
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
			return nil, fmt.Errorf("invalid requirement line %q: empty project", line)
		}
		entries = append(entries, requirement{Platform: platform, Project: project, Version: version})
	}
	return entries, nil
}

type requirementResult struct {
	Line   string `json:"line"`
	ID     string `json:"id,omitempty"`
	Status string `json:"status"`
	Note   string `json:"note,omitempty"`
}

func (r runner) installRequirements(ctx context.Context, c *cli.Context, svc *appcore.Service, runtime runtimeInput, file string) error {
	data, err := os.ReadFile(file)
	if err != nil {
		return err
	}
	entries, err := parseRequirements(string(data))
	if err != nil {
		return err
	}
	installed, err := svc.LocalModsInDir(runtime.TargetModsDir, runtime.MinecraftVersion, runtime.ModLoader)
	if err != nil {
		return err
	}
	installedVersions := installedRequirementVersions(installed)
	results := make([]requirementResult, 0, len(entries))
	failed := 0
	for _, entry := range entries {
		result := r.installRequirement(ctx, svc, runtime, entry, installedVersions)
		result.Line = entry.String()
		if result.Status == "failed" {
			failed++
		}
		results = append(results, result)
	}
	if err := r.write(c, results, writeRequirementResults); err != nil {
		return err
	}
	if failed > 0 {
		return cli.Exit(fmt.Sprintf("%d requirement(s) failed", failed), 1)
	}
	return nil
}

func (r runner) installRequirement(ctx context.Context, svc *appcore.Service, runtime runtimeInput, entry requirement, installedVersions map[string]string) requirementResult {
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
	if installedVersion, ok := installedVersions[entry.Platform+":"+projectRef]; ok {
		if entry.Version == "" || strings.EqualFold(strings.TrimSpace(installedVersion), entry.Version) {
			return requirementResult{ID: project.ID, Status: "skipped", Note: "already installed"}
		}
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
			return requirementResult{ID: project.ID, Status: "failed", Note: fmt.Sprintf("version %q not available for this instance", entry.Version)}
		}
	}

	result := svc.InstallModToDirAndWait(ctx, appcore.InstallToDirRequest{
		ProjectID:        project.ID,
		Project:          project,
		TargetDir:        runtime.TargetModsDir,
		MinecraftVersion: runtime.MinecraftVersion,
		ModLoader:        runtime.ModLoader,
		VersionID:        versionID,
	})
	if len(result.Errors) > 0 {
		return requirementResult{ID: project.ID, Status: "failed", Note: result.Errors[0].Reason}
	}
	if result.Result.Skipped {
		return requirementResult{ID: project.ID, Status: "skipped", Note: result.Result.Reason}
	}
	return requirementResult{ID: project.ID, Status: "installed", Note: result.Result.FileName}
}

// installedRequirementVersions maps "platform:project" to the installed version
// so unpinned requirements and matching pins can be skipped, like pip's
// "Requirement already satisfied".
func installedRequirementVersions(mods []structs.ModInfo) map[string]string {
	versions := make(map[string]string)
	for _, mod := range mods {
		platform := strings.ToLower(strings.TrimSpace(mod.OnlinePlatform))
		ref := strings.ToLower(firstNonEmptyString(mod.OnlineSlug, mod.OnlineProjectID))
		if platform == "" || ref == "" {
			continue
		}
		versions[platform+":"+ref] = currentInstalledVersion(mod, platform, strings.ToLower(strings.TrimSpace(mod.SHA1)))
	}
	return versions
}

func writeRequirements(w io.Writer, value any) error {
	entries, ok := value.([]requirement)
	if !ok {
		return fmt.Errorf("unexpected requirements payload %T", value)
	}
	for _, entry := range entries {
		if _, err := fmt.Fprintln(w, entry.String()); err != nil {
			return err
		}
	}
	return nil
}

func writeRequirementResults(w io.Writer, value any) error {
	results, ok := value.([]requirementResult)
	if !ok {
		return fmt.Errorf("unexpected requirement results payload %T", value)
	}
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

// requirementInstallConflict rejects install flags that do not apply to a
// requirements file. Silently ignoring them would make `install -r` surprising.
func requirementInstallConflict(c *cli.Context, refs []string, platform string) error {
	switch {
	case len(refs) > 0:
		return errors.New("--requirement cannot be combined with project arguments")
	case platform != "":
		return errors.New("--requirement cannot be combined with --platform")
	case c.Bool("server"):
		return errors.New("--requirement cannot be combined with --server")
	case c.Bool("dry-run"):
		return errors.New("--requirement cannot be combined with --dry-run")
	case strings.TrimSpace(c.String("version-id")) != "":
		return errors.New("--requirement cannot be combined with --version-id")
	}
	return nil
}
