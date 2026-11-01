package cliapp

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"text/tabwriter"
	"time"

	"github.com/mattn/go-isatty"

	"github.com/link-fgfgui/mod-downloader/core/appcore"

	"github.com/link-fgfgui/mod-downloader-cli/cliapp/progress"
)

// InstallSkipped records a target that resolved but was not installed.
type InstallSkipped struct {
	Target string `json:"target"`
	Reason string `json:"reason"`
}

// InstallReport is the unified --json install report (Phase 5.3): the resolved
// plan plus skipped targets and errors.
type InstallReport struct {
	Targets  []string           `json:"targets"`
	Resolved []InstallPlanEntry `json:"resolved"`
	Skipped  []InstallSkipped   `json:"skipped,omitempty"`
	Errors   []string           `json:"errors,omitempty"`
}

// disjointTargets returns the ordered, de-duplicated raw targets of a plan.
func targetRaws(plan []InstallPlanEntry) []string {
	seen := map[string]bool{}
	out := []string{}
	for _, entry := range plan {
		if entry.Target == "" || seen[entry.Target] {
			continue
		}
		seen[entry.Target] = true
		out = append(out, entry.Target)
	}
	return out
}

// writeInstallPlanJSON is the --json writer for the dry-run plan: it emits the
// unified report object (targets + resolved rows).
func writeInstallPlanJSON(w io.Writer, value any) error {
	plan, ok := value.([]InstallPlanEntry)
	if !ok {
		return fmt.Errorf("unexpected install plan payload %T", value)
	}
	return encodeJSON(w, InstallReport{Targets: targetRaws(plan), Resolved: plan})
}

// writeInstallReportJSON is the --json writer for the executed install report.
func writeInstallReportJSON(w io.Writer, value any) error {
	report, ok := value.(InstallReport)
	if !ok {
		return fmt.Errorf("unexpected install report payload %T", value)
	}
	return encodeJSON(w, report)
}

func encodeJSON(w io.Writer, value any) error {
	encoder := json.NewEncoder(w)
	encoder.SetIndent("", "  ")
	return encoder.Encode(value)
}

// writeInstallReportText renders the same report for text mode.
func writeInstallReportText(w io.Writer, value any) error {
	report, ok := value.(InstallReport)
	if !ok {
		return fmt.Errorf("unexpected install report payload %T", value)
	}
	tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
	fmt.Fprintln(tw, "TARGET\tPLATFORM\tMOD_ID\tTITLE\tVERSION\tSIZE")
	for _, entry := range report.Resolved {
		fmt.Fprintf(tw, "%s\t%s\t%s\t%s\t%s\t%s\n", entry.Target, entry.Platform, modIDColumn(entry), entry.Title, orDash(entry.Version), sizeColumn(entry))
	}
	if err := tw.Flush(); err != nil {
		return err
	}
	for _, skipped := range report.Skipped {
		fmt.Fprintf(w, "skipped %s: %s\n", skipped.Target, skipped.Reason)
	}
	for _, errMsg := range report.Errors {
		fmt.Fprintf(w, "failed: %s\n", errMsg)
	}
	return nil
}

// writeInstallPlan renders the resolved install preview as an aligned table
// (Phase 5.1): TARGET, PLATFORM, MOD_ID, TITLE, VERSION, SIZE.
func (r runner) writeInstallPlan(plan []InstallPlanEntry, target string) error {
	if len(plan) == 0 {
		return nil
	}
	tw := tabwriter.NewWriter(r.stdout, 0, 0, 2, ' ', 0)
	if _, err := fmt.Fprintln(tw, "TARGET\tPLATFORM\tMOD_ID\tTITLE\tVERSION\tSIZE"); err != nil {
		return err
	}
	for _, entry := range plan {
		if _, err := fmt.Fprintf(tw, "%s\t%s\t%s\t%s\t%s\t%s\n",
			entry.Target,
			entry.Platform,
			modIDColumn(entry),
			entry.Title,
			orDash(entry.Version),
			sizeColumn(entry),
		); err != nil {
			return err
		}
	}
	if err := tw.Flush(); err != nil {
		return err
	}
	_, err := fmt.Fprintf(r.stdout, "\nresolved %d project(s) for %s\n\n", len(plan), target)
	return err
}

// installWithProgress runs one install while rendering a live download progress
// line (Phase 5.2). In JSON mode no progress is drawn.
func (r runner) installWithProgress(ctx context.Context, svc *appcore.Service, req appcore.InstallToDirRequest, jsonMode bool, index, count int, label string, doneBefore, totalAll int64) appcore.InstallWaitResult {
	if jsonMode {
		return svc.InstallModToDirAndWait(ctx, req)
	}
	reporter := progress.NewAuto(r.stdout, writerIsTerminal(r.stdout))
	defer reporter.Finish()

	done := make(chan appcore.InstallWaitResult, 1)
	go func() { done <- svc.InstallModToDirAndWait(ctx, req) }()

	ticker := time.NewTicker(120 * time.Millisecond)
	defer ticker.Stop()
	for {
		select {
		case result := <-done:
			return result
		case <-ticker.C:
			state := svc.GetDownloadQueueState()
			name := label
			for _, item := range state.Items {
				if item.Status == "running" {
					name = firstNonEmptyStr(item.Title, item.FileName, label)
					break
				}
			}
			overall := doneBefore + state.BytesComplete
			if totalAll > 0 && overall > totalAll {
				overall = totalAll
			}
			reporter.Update(progress.State{
				Index:        index,
				Count:        count,
				Name:         name,
				Done:         state.BytesComplete,
				Total:        state.TotalBytes,
				OverallDone:  overall,
				OverallTotal: totalAll,
				Speed:        state.BytesPerSecond,
			})
		}
	}
}

// writerIsTerminal reports whether w is an interactive terminal.
func writerIsTerminal(w io.Writer) bool {
	f, ok := w.(*os.File)
	if !ok {
		return false
	}
	fd := f.Fd()
	return isatty.IsTerminal(fd) || isatty.IsCygwinTerminal(fd)
}

func modIDColumn(entry InstallPlanEntry) string {
	if len(entry.ModIDs) == 0 {
		return "—"
	}
	out := entry.ModIDs[0]
	for _, id := range entry.ModIDs[1:] {
		out += "," + id
	}
	return out
}

func sizeColumn(entry InstallPlanEntry) string {
	if entry.FileSize <= 0 {
		return "—"
	}
	return humanSize(entry.FileSize)
}

func orDash(value string) string {
	if value == "" {
		return "—"
	}
	return value
}
