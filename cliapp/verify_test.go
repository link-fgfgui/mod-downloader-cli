package cliapp

import (
	"encoding/json"
	"path/filepath"
	"testing"

	"github.com/link-fgfgui/mod-downloader-core/minecraft"
)

func TestAnalyzeVerifyMissingDependency(t *testing.T) {
	isolateGlobalState(t)
	modsDir := t.TempDir()
	writeModJar(t, modsDir, "a.jar", fabricJarEntries("a", "1.0", `{"fabricloader":">=0.14","minecraft":"~1.21","fabric-api":"*"}`))
	writeModJar(t, modsDir, "b.jar", fabricJarEntries("b", "1.0", `{"a":"*"}`))

	mods := minecraft.ScanModsDir(modsDir, "verify-inst", "1.21.1", "", "")
	report := analyzeVerify(modsDir, mods, "1.21.1", "fabric", false)
	if report.Passed {
		t.Fatalf("expected failure, got %#v", report)
	}
	missing := map[string]bool{}
	for _, issue := range report.Issues {
		if issue.Kind == "missing-dependency" {
			missing[issue.ModID] = true
		}
	}
	if !missing["fabric-api"] {
		t.Fatalf("fabric-api not reported missing: %#v", report.Issues)
	}
	if missing["fabricloader"] || missing["minecraft"] || missing["a"] {
		t.Fatalf("builtin/present dependency reported missing: %#v", missing)
	}
	if report.Issues[0].Suggest == "" {
		t.Fatal("missing dependency has no install suggestion")
	}

	writeModJar(t, modsDir, "fabric-api.jar", fabricJarEntries("fabric-api", "0.9", `{"fabricloader":">=0.14"}`))
	mods = minecraft.ScanModsDir(modsDir, "verify-inst", "1.21.1", "", "")
	report = analyzeVerify(modsDir, mods, "1.21.1", "fabric", false)
	if !report.Passed {
		t.Fatalf("expected pass after installing fabric-api: %#v", report.Issues)
	}
}

func TestAnalyzeVerifyDuplicateModID(t *testing.T) {
	isolateGlobalState(t)
	modsDir := t.TempDir()
	writeModJar(t, modsDir, "dup1.jar", fabricJarEntries("dup", "1.0", "{}"))
	writeModJar(t, modsDir, "dup2.jar", fabricJarEntries("dup", "2.0", "{}"))

	mods := minecraft.ScanModsDir(modsDir, "verify-dup", "1.21.1", "", "")
	report := analyzeVerify(modsDir, mods, "1.21.1", "fabric", false)
	if report.Passed {
		t.Fatalf("expected duplicate failure, got %#v", report)
	}
	found := false
	for _, issue := range report.Issues {
		if issue.Kind == "duplicate-mod" && issue.ModID == "dup" {
			found = true
		}
	}
	if !found {
		t.Fatalf("duplicate-mod issue missing: %#v", report.Issues)
	}
}

func TestAnalyzeVerifyLoaderMismatchStrict(t *testing.T) {
	isolateGlobalState(t)
	toml := `
modLoader="javafml"
loaderVersion="[47,)"
license="MIT"
[[mods]]
modId="forgemod"
version="1.0"
displayName="Forge Mod"
description="d"
[[dependencies.forgemod]]
    modId="minecraft"
    mandatory=true
    versionRange="[1.20,1.22)"
    side="BOTH"
`
	modsDir := t.TempDir()
	writeModJar(t, modsDir, "forgemod.jar", map[string]string{"META-INF/mods.toml": toml})

	mods := minecraft.ScanModsDir(modsDir, "verify-loader", "1.21.1", "", "")
	report := analyzeVerify(modsDir, mods, "1.21.1", "fabric", false)
	if !report.Passed {
		t.Fatalf("advisory issue should not fail default verify: %#v", report.Issues)
	}
	foundAdvisory := false
	for _, issue := range report.Issues {
		if issue.Kind == "incompatible" && issue.Advisory {
			foundAdvisory = true
		}
	}
	if !foundAdvisory {
		t.Fatalf("loader mismatch not reported: %#v", report.Issues)
	}

	strict := analyzeVerify(modsDir, mods, "1.21.1", "fabric", true)
	if strict.Passed {
		t.Fatalf("strict verify should fail on advisory issue: %#v", strict.Issues)
	}
}

func TestVerifyCommandExitCode(t *testing.T) {
	isolateGlobalState(t)
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	modsDir := filepath.Join(t.TempDir(), "mods")
	writeModJar(t, modsDir, "a.jar", fabricJarEntries("a", "1.0", `{"fabric-api":"*"}`))
	chdir(t, modsDir)

	out, _, err := runCLI(t, "verify", "--json")
	if err == nil {
		t.Fatal("verify should exit non-zero on missing dependency")
	}
	var report verifyReport
	if err := json.Unmarshal([]byte(out), &report); err != nil {
		t.Fatalf("decode report: %v\n%s", err, out)
	}
	if report.Passed {
		t.Fatalf("report passed unexpectedly: %#v", report)
	}
	if len(report.Issues) == 0 {
		t.Fatal("report has no issues")
	}
}
