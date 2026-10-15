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

	"github.com/urfave/cli/v2"

	"github.com/link-fgfgui/mod-downloader-core/appcore"
	structs "github.com/link-fgfgui/mod-downloader-core/structs/minecraft"
)

const disabledSuffix = ".jar.disabled"

func localModPathDisabled(path string) bool {
	return strings.HasSuffix(strings.ToLower(path), disabledSuffix)
}

// modStateChange describes one enable/disable operation (or a friendly skip).
type modStateChange struct {
	Action   string `json:"action"`
	ID       string `json:"id"`
	Name     string `json:"name"`
	File     string `json:"file"`
	From     string `json:"from"`
	To       string `json:"to,omitempty"`
	Executed bool   `json:"executed"`
	Note     string `json:"note,omitempty"`
}

func (r runner) enableCommand() *cli.Command {
	return r.modStateCommand("enable", true)
}

func (r runner) disableCommand() *cli.Command {
	return r.modStateCommand("disable", false)
}

func (r runner) modStateCommand(name string, enable bool) *cli.Command {
	usage := "Enable disabled mods by renaming <file>.jar.disabled back to <file>.jar"
	if !enable {
		usage = "Disable mods by renaming <file>.jar to <file>.jar.disabled"
	}
	return &cli.Command{
		Name:      name,
		Usage:     usage,
		ArgsUsage: "<mod...>",
		Flags: []cli.Flag{
			jsonFlag(),
			&cli.BoolFlag{Name: "dry-run", Usage: "print planned changes without renaming files"},
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
				changes, err := planModStateChanges(runtime.TargetModsDir, mods, targets, enable)
				if err != nil {
					return err
				}
				if len(changes) == 0 {
					return cli.Exit("no matching installed mods", 1)
				}
				if !c.Bool("dry-run") {
					if err := applyModStateChanges(changes); err != nil {
						return err
					}
				}
				return r.write(c, changes, writeModStateChanges)
			})
		},
	}
}

func planModStateChanges(modsDir string, mods []structs.ModInfo, targets []string, enable bool) ([]modStateChange, error) {
	targetSet := make(map[string]bool, len(targets))
	for _, target := range targets {
		targetSet[normalizeModRef(target)] = true
	}

	action := "disable"
	if enable {
		action = "enable"
	}

	changes := make([]modStateChange, 0)
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

		currentDisabled := localModPathDisabled(path)
		change := modStateChange{
			Action: action,
			ID:     mod.ID,
			Name:   mod.Name,
			File:   filepath.Base(path),
			From:   path,
		}

		switch {
		case enable && !currentDisabled:
			change.Note = "already enabled"
		case !enable && currentDisabled:
			change.Note = "already disabled"
		case enable:
			change.To = strings.TrimSuffix(path, ".disabled")
		default:
			change.To = path + ".disabled"
		}

		if change.To != "" {
			if _, err := os.Stat(change.To); err == nil {
				return nil, fmt.Errorf("target file already exists: %s", change.To)
			} else if !os.IsNotExist(err) {
				return nil, err
			}
		}
		changes = append(changes, change)
	}
	return changes, nil
}

func applyModStateChanges(changes []modStateChange) error {
	for i := range changes {
		if changes[i].To == "" {
			continue
		}
		if err := os.Rename(changes[i].From, changes[i].To); err != nil {
			return err
		}
		changes[i].Executed = true
	}
	return nil
}

func writeModStateChanges(w io.Writer, value any) error {
	changes := value.([]modStateChange)
	tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
	fmt.Fprintln(tw, "ID\tNAME\tFILE\tACTION\tSTATUS")
	for _, change := range changes {
		status := change.Note
		if status == "" {
			if change.Executed {
				status = "done"
			} else {
				status = "preview"
			}
		}
		fmt.Fprintf(tw, "%s\t%s\t%s\t%s\t%s\n", change.ID, change.Name, change.File, change.Action, status)
	}
	return tw.Flush()
}
