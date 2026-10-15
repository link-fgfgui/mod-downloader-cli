package cliapp

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"strings"
	"text/tabwriter"
	"time"

	"github.com/link-fgfgui/mod-downloader-core/appcore"
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
func (r runner) installWithProgress(ctx context.Context, svc *appcore.Service, req appcore.InstallToDirRequest, jsonMode bool) appcore.InstallWaitResult {
	if jsonMode {
		return svc.InstallModToDirAndWait(ctx, req)
	}
	done := make(chan appcore.InstallWaitResult, 1)
	go func() { done <- svc.InstallModToDirAndWait(ctx, req) }()

	ticker := time.NewTicker(200 * time.Millisecond)
	defer ticker.Stop()
	for {
		select {
		case result := <-done:
			r.clearInstallProgress()
			return result
		case <-ticker.C:
			r.renderInstallProgress(svc)
		}
	}
}

// renderInstallProgress draws a single-line progress bar (overwritten in place).
func (r runner) renderInstallProgress(svc *appcore.Service) {
	state := svc.GetDownloadQueueState()
	bar := progressBarText(state.BytesComplete, state.TotalBytes, 24)
	percent := "  ?%"
	if state.TotalBytes > 0 {
		pct := float64(state.BytesComplete) / float64(state.TotalBytes) * 100
		if pct > 100 {
			pct = 100
		}
		percent = fmt.Sprintf("%3.0f%%", pct)
	}
	speed := humanSpeed(state.BytesPerSecond)
	pending := state.Running + state.Pending
	_, _ = fmt.Fprintf(r.stdout, "\r\x1b[K[%s] %s  %s  ·  %d 任务进行中", bar, percent, speed, pending)
}

func (r runner) clearInstallProgress() {
	_, _ = fmt.Fprint(r.stdout, "\r\x1b[K")
}

// progressBarText renders a fixed-width textual bar ("unknown size" -> dotted).
func progressBarText(complete, total int64, width int) string {
	if width < 4 {
		width = 4
	}
	if total <= 0 {
		filled := width / 2
		return strings.Repeat("·", filled) + strings.Repeat(" ", width-filled)
	}
	ratio := float64(complete) / float64(total)
	if ratio < 0 {
		ratio = 0
	}
	if ratio > 1 {
		ratio = 1
	}
	filled := int(math.Round(ratio * float64(width)))
	return strings.Repeat("█", filled) + strings.Repeat("░", width-filled)
}

// humanSpeed renders bytes/second as MiB/s.
func humanSpeed(bytesPerSecond int64) string {
	if bytesPerSecond <= 0 {
		return "0.00 MiB/s"
	}
	return fmt.Sprintf("%.2f MiB/s", float64(bytesPerSecond)/(1024*1024))
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
