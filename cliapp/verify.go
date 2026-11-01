package cliapp

import (
	"context"
	"crypto/sha512"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"text/tabwriter"

	"github.com/urfave/cli/v2"

	"github.com/link-fgfgui/mod-downloader/core/appcore"
	"github.com/link-fgfgui/mod-downloader/core/minecraft"
	"github.com/link-fgfgui/mod-downloader/core/modbridge"
	"github.com/link-fgfgui/mod-downloader/core/storage"
	structs "github.com/link-fgfgui/mod-downloader/core/structs/minecraft"
)

func osStatFile(path string) (string, bool) {
	info, err := os.Stat(path)
	if err != nil || info.IsDir() {
		return "", false
	}
	return path, true
}

// The report types are owned by appcore so the GUI and TUI can reuse the same
// verification rules; the CLI only formats them.

func (r runner) verifyCommand() *cli.Command {
	return &cli.Command{
		Name:    "verify",
		Aliases: []string{"check"},
		Usage:   "Check installed mods for missing dependencies, duplicate ids, and incompatibilities",
		Flags: []cli.Flag{
			jsonFlag(),
			&cli.BoolFlag{Name: "strict", Usage: "treat compatibility warnings as failures"},
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
			// Scan with an empty loader so every jar's metadata is parsed,
			// regardless of the jar's declared loader.
			instanceID := modbridge.InstanceIDForTargetDir(runtime.TargetModsDir)
			mods := minecraft.ScanModsDir(runtime.TargetModsDir, instanceID, runtime.MinecraftVersion, "", "")
			report := analyzeVerify(runtime.TargetModsDir, mods, runtime.MinecraftVersion, runtime.ModLoader, c.Bool("strict"))
			if err := r.write(c, report, writeVerifyReport); err != nil {
				return err
			}
			if !report.Passed {
				return cli.Exit("verification failed", 1)
			}
			return nil
		},
	}
}

// analyzeVerify delegates to the shared core rules.
func analyzeVerify(modsDir string, mods []structs.ModInfo, mcVersion, modLoader string, strict bool) appcore.VerifyReport {
	return appcore.VerifyLocalMods(modsDir, mods, mcVersion, modLoader, strict)
}

func writeVerifyReport(w io.Writer, value any) error {
	report := value.(appcore.VerifyReport)
	tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
	if len(report.Issues) == 0 {
		fmt.Fprintf(tw, "checked %d jars, all checks passed\n", report.Checked)
		return tw.Flush()
	}
	for _, issue := range report.Issues {
		label := strings.ToUpper(issue.Kind)
		if issue.Advisory {
			label += " (warning)"
		}
		fmt.Fprintf(tw, "%s\t%s\t%s\n", label, issue.ModID, issue.Message)
		if issue.Suggest != "" {
			fmt.Fprintf(tw, "\t\t%s\n", issue.Suggest)
		}
	}
	fmt.Fprintf(tw, "\nchecked %d jars, %d issue(s)\n", report.Checked, len(report.Issues))
	if err := tw.Flush(); err != nil {
		return err
	}
	return nil
}

// --- info ---

type projectURLInfo struct {
	Platform  string `json:"platform,omitempty"`
	ProjectID string `json:"projectId,omitempty"`
	Slug      string `json:"slug,omitempty"`
	Name      string `json:"name,omitempty"`
	URL       string `json:"url,omitempty"`
}

type jarFileInfo struct {
	Path   string `json:"path"`
	Name   string `json:"name"`
	Size   int64  `json:"size"`
	SHA1   string `json:"sha1"`
	SHA512 string `json:"sha512"`
}

type infoView struct {
	Metadata  structs.ModInfo  `json:"metadata"`
	File      jarFileInfo      `json:"file"`
	Installed *structs.ModInfo `json:"installed,omitempty"`
	Online    *projectURLInfo  `json:"online,omitempty"`
}

func (r runner) infoCommand() *cli.Command {
	return &cli.Command{
		Name:      "info",
		Aliases:   []string{"inspect"},
		Usage:     "Show detailed metadata for an installed mod or a local jar file",
		ArgsUsage: "<mod-or-jar-file>",
		Flags: []cli.Flag{
			jsonFlag(),
			&cli.BoolFlag{Name: "force", Usage: "allow running outside a directory named mods"},
		},
		Action: func(c *cli.Context) error {
			arg := strings.TrimSpace(c.Args().First())
			if arg == "" {
				return errors.New("mod or jar file is required")
			}
			if info, ok := osStatFile(arg); ok {
				view, err := buildInfoView(info, nil)
				if err != nil {
					return err
				}
				return r.write(c, view, writeInfoView)
			}
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
				targetSet := map[string]bool{normalizeModRef(arg): true}
				for i := range mods {
					if !matchesAnyModTarget(mods[i], targetSet) {
						continue
					}
					path := appcore.ResolveLocalModPath(runtime.TargetModsDir, mods[i])
					view, err := buildInfoView(path, &mods[i])
					if err != nil {
						return err
					}
					return r.write(c, view, writeInfoView)
				}
				return cli.Exit(fmt.Sprintf("no matching installed mod: %s", arg), 1)
			})
		},
	}
}

// buildInfoView parses a jar through the shared core parser; the first declared
// mod is the primary metadata entry.
func buildInfoView(path string, installed *structs.ModInfo) (infoView, error) {
	abs, err := filepath.Abs(path)
	if err != nil {
		abs = path
	}
	sha1 := minecraft.FileSHA1(abs)
	mods := minecraft.ParseLocalModJarWithSHA1(abs, sha1, "")
	if len(mods) == 0 {
		return infoView{}, fmt.Errorf("parse %s: no parseable mod metadata", filepath.Base(abs))
	}
	view := infoView{
		Metadata: mods[0],
		File: jarFileInfo{
			Path:   abs,
			Name:   filepath.Base(abs),
			Size:   fileSize(abs),
			SHA1:   sha1,
			SHA512: fileSHA512(abs),
		},
		Installed: installed,
	}
	view.Online = onlineProjectForInfo(installed, sha1)
	return view, nil
}

func onlineProjectForInfo(installed *structs.ModInfo, sha1 string) *projectURLInfo {
	if installed != nil {
		platform := strings.ToLower(strings.TrimSpace(installed.OnlinePlatform))
		if platform != "" {
			slug := strings.TrimSpace(installed.OnlineSlug)
			return &projectURLInfo{
				Platform:  platform,
				ProjectID: strings.TrimSpace(installed.OnlineProjectID),
				Slug:      slug,
				Name:      strings.TrimSpace(installed.OnlineName),
				URL:       projectURL(platform, slug),
			}
		}
	}
	if strings.TrimSpace(sha1) == "" {
		return nil
	}
	for _, platform := range []string{"curseforge", "modrinth"} {
		version, ok := storage.GetVersionBySHA1(platform, sha1)
		if !ok {
			continue
		}
		project, found := storage.GetModProject(platform, version.ProjectID)
		if !found {
			continue
		}
		return &projectURLInfo{
			Platform:  platform,
			ProjectID: project.ProjectID,
			Slug:      project.Slug,
			Name:      project.Title,
			URL:       projectURL(platform, project.Slug),
		}
	}
	return nil
}

func projectURL(platform, slug string) string {
	slug = strings.TrimSpace(slug)
	if slug == "" {
		return ""
	}
	switch strings.ToLower(strings.TrimSpace(platform)) {
	case "modrinth":
		return "https://modrinth.com/mod/" + slug
	case "curseforge":
		return "https://www.curseforge.com/minecraft/mc-mods/" + slug
	default:
		return ""
	}
}

func writeInfoView(w io.Writer, value any) error {
	view := value.(infoView)
	meta := view.Metadata
	tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)

	fmt.Fprintln(tw, "MOD ID\t"+meta.ID)
	fmt.Fprintln(tw, "NAME\t"+meta.Name)
	fmt.Fprintln(tw, "VERSION\t"+meta.Version)
	fmt.Fprintln(tw, "LOADER\t"+strings.Join(meta.Loaders, ", "))
	if meta.Environment != "" {
		fmt.Fprintln(tw, "ENVIRONMENT\t"+meta.Environment)
	}
	if meta.License != "" {
		fmt.Fprintln(tw, "LICENSE\t"+meta.License)
	}
	if len(meta.Authors) > 0 {
		fmt.Fprintln(tw, "AUTHORS\t"+strings.Join(meta.Authors, ", "))
	}
	if meta.MinecraftVersion != "" {
		fmt.Fprintln(tw, "MINECRAFT\t"+meta.MinecraftVersion)
	}
	if meta.Description != "" {
		fmt.Fprintln(tw, "DESCRIPTION\t"+meta.Description)
	}

	fmt.Fprintln(tw, "\nFILE\t"+view.File.Name)
	fmt.Fprintln(tw, "PATH\t"+view.File.Path)
	fmt.Fprintf(tw, "SIZE\t%d\n", view.File.Size)
	fmt.Fprintln(tw, "SHA1\t"+view.File.SHA1)
	fmt.Fprintln(tw, "SHA512\t"+view.File.SHA512)

	writeDependencySection(tw, "DEPENDS", meta.Dependencies)
	writeDependencySection(tw, "RECOMMENDS", meta.Recommends)
	writeDependencySection(tw, "BREAKS", meta.Breaks)

	if view.Online != nil {
		fmt.Fprintln(tw, "\nONLINE\t\t")
		fmt.Fprintln(tw, "PLATFORM\t"+view.Online.Platform)
		fmt.Fprintln(tw, "SLUG\t"+view.Online.Slug)
		fmt.Fprintln(tw, "URL\t"+view.Online.URL)
	}
	return tw.Flush()
}

func writeDependencySection(w io.Writer, title string, deps []structs.LocalModDependency) {
	if len(deps) == 0 {
		return
	}
	fmt.Fprintf(w, "\n%s\t\t\n", title)
	for _, dep := range deps {
		fmt.Fprintf(w, "%s\t%s\t\n", dep.ModID, dep.Version)
	}
}

func fileSHA512(path string) string {
	f, err := os.Open(path)
	if err != nil {
		return ""
	}
	defer f.Close()
	h := sha512.New()
	if _, err := io.Copy(h, f); err != nil {
		return ""
	}
	return hex.EncodeToString(h.Sum(nil))
}

func fileSize(path string) int64 {
	info, err := os.Stat(path)
	if err != nil {
		return 0
	}
	return info.Size()
}
