package cliapp

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/link-fgfgui/mod-downloader-core/appcore"
	"github.com/link-fgfgui/mod-downloader-core/models"
)

func TestMetadataTimeoutFallsBackToSlug(t *testing.T) {
	installSeams(t)
	origTimeout := metadataTimeout
	metadataTimeout = 20 * time.Millisecond
	t.Cleanup(func() { metadataTimeout = origTimeout })

	// Block well past the (shortened) deadline.
	listMatchingVersionsFn = func(*appcore.Service, models.ModProject, string, string) []models.ModVersion {
		time.Sleep(500 * time.Millisecond)
		return []models.ModVersion{{ID: "v1", Version: "1.0"}}
	}

	svc := appcore.New(appcore.Options{})
	entry := buildInstallPlanEntry(svc, "roughly-enough-items",
		projectRef("modrinth", "x", "roughly-enough-items", "REI", 1),
		runtimeInput{MinecraftVersion: "1.21.1", ModLoader: "fabric", TargetModsDir: "/mods"})

	if entry.LocalState != "metadata_timeout" {
		t.Fatalf("localState = %q, want metadata_timeout", entry.LocalState)
	}
	if len(entry.ModIDs) != 1 || entry.ModIDs[0] != "roughly-enough-items" {
		t.Fatalf("modIds must fall back to the slug, got %v", entry.ModIDs)
	}
	if entry.Version != "" {
		t.Fatalf("version should be unknown on timeout, got %q", entry.Version)
	}
}

func TestPreferredVersionWithinReportsMissing(t *testing.T) {
	installSeams(t)
	listMatchingVersionsFn = func(*appcore.Service, models.ModProject, string, string) []models.ModVersion {
		return nil
	}
	svc := appcore.New(appcore.Options{})
	_, status := preferredVersionWithin(svc, projectRef("modrinth", "x", "x", "X", 1), runtimeInput{}, 50*time.Millisecond)
	if status != versionMissing {
		t.Fatalf("status = %v, want versionMissing", status)
	}
}

func TestPreferredVersionWithinReportsFound(t *testing.T) {
	installSeams(t)
	listMatchingVersionsFn = func(*appcore.Service, models.ModProject, string, string) []models.ModVersion {
		return []models.ModVersion{{ID: "v9", Version: "9.9.9"}}
	}
	svc := appcore.New(appcore.Options{})
	version, status := preferredVersionWithin(svc, projectRef("modrinth", "x", "x", "X", 1), runtimeInput{}, 200*time.Millisecond)
	if status != versionFound || version.Version != "9.9.9" {
		t.Fatalf("unexpected result: %v %q", status, version.Version)
	}
}

func TestProgressBarText(t *testing.T) {
	if got := progressBarText(50, 100, 10); got != "█████░░░░░" {
		t.Fatalf("progressBarText = %q", got)
	}
	if got := progressBarText(1, 0, 10); !strings.Contains(got, "·") {
		t.Fatalf("unknown-size bar should be dotted, got %q", got)
	}
	if got := humanSpeed(1024 * 1024); got != "1.00 MiB/s" {
		t.Fatalf("humanSpeed = %q", got)
	}
}

func TestInstallReportJSONShape(t *testing.T) {
	plan := []InstallPlanEntry{
		{Target: "sodium", Platform: "Modrinth", ProjectID: "AANobbMI", Title: "Sodium", Version: "0.6.0", ModIDs: []string{"sodium"}, FileSize: 1048576},
		{Target: "iris", Platform: "Modrinth", ProjectID: "iris", Title: "Iris", LocalState: "metadata_timeout", ModIDs: []string{"iris"}},
	}
	report := InstallReport{
		Targets:  targetRaws(plan),
		Resolved: plan,
		Skipped:  []InstallSkipped{{Target: "jei", Reason: "no matching version"}},
		Errors:   []string{"boom"},
	}
	var buf bytes.Buffer
	if err := writeInstallReportJSON(&buf, report); err != nil {
		t.Fatal(err)
	}
	var decoded map[string]any
	if err := json.Unmarshal(buf.Bytes(), &decoded); err != nil {
		t.Fatalf("report is not valid JSON: %v\n%s", err, buf.String())
	}
	for _, key := range []string{"targets", "resolved", "skipped", "errors"} {
		if _, ok := decoded[key]; !ok {
			t.Fatalf("report JSON missing %q key: %s", key, buf.String())
		}
	}
	resolved := decoded["resolved"].([]any)
	if len(resolved) != 2 {
		t.Fatalf("resolved rows = %d, want 2", len(resolved))
	}
}

func TestTargetRawsDeduplicates(t *testing.T) {
	plan := []InstallPlanEntry{{Target: "sodium"}, {Target: "sodium"}, {Target: "iris"}, {Target: ""}}
	got := targetRaws(plan)
	if len(got) != 2 || got[0] != "sodium" || got[1] != "iris" {
		t.Fatalf("targetRaws = %v", got)
	}
}

func TestWriteInstallReportTextListsSkippedAndErrors(t *testing.T) {
	report := InstallReport{
		Targets:  []string{"sodium", "jei"},
		Resolved: []InstallPlanEntry{{Target: "sodium", Platform: "Modrinth", ProjectID: "x", Title: "Sodium", ModIDs: []string{"sodium"}}},
		Skipped:  []InstallSkipped{{Target: "jei", Reason: "no matching version"}},
		Errors:   []string{"network down"},
	}
	var buf bytes.Buffer
	if err := writeInstallReportText(&buf, report); err != nil {
		t.Fatal(err)
	}
	out := buf.String()
	if !strings.Contains(out, "TARGET") || !strings.Contains(out, "sodium") {
		t.Fatalf("plan table missing: %s", out)
	}
	if !strings.Contains(out, "skipped jei: no matching version") {
		t.Fatalf("skipped row missing: %s", out)
	}
	if !strings.Contains(out, "failed: network down") {
		t.Fatalf("error row missing: %s", out)
	}
}
