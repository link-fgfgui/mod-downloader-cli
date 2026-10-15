package cliapp

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestParseInstallTargetForms(t *testing.T) {
	cases := []struct {
		name            string
		raw             string
		defaultPlatform string
		wantPlatform    string
		wantQuery       string
		wantIsID        bool
	}{
		{"plain slug", "sodium", "", "", "sodium", false},
		{"slug with default platform", "sodium", "modrinth", "modrinth", "sodium", false},
		{"modrinth prefix slug", "modrinth:sodium", "", "modrinth", "sodium", false},
		{"curseforge numeric id", "curseforge:394468", "", "curseforge", "394468", true},
		{"modrinth id", "modrinth:AANobbMI", "", "modrinth", "AANobbMI", false},
		{"uppercase prefix", "CurseForge:jei", "", "curseforge", "jei", false},
		{"bare numeric id", "394468", "", "", "394468", true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			target, err := ParseInstallTarget(tc.raw, tc.defaultPlatform)
			if err != nil {
				t.Fatalf("ParseInstallTarget(%q) error: %v", tc.raw, err)
			}
			if target.Platform != tc.wantPlatform {
				t.Errorf("platform = %q, want %q", target.Platform, tc.wantPlatform)
			}
			if target.Query != tc.wantQuery {
				t.Errorf("query = %q, want %q", target.Query, tc.wantQuery)
			}
			if target.IsID != tc.wantIsID {
				t.Errorf("IsID = %v, want %v", target.IsID, tc.wantIsID)
			}
		})
	}
}

func TestValidateProjectTargetRejectsVersionAndFileSemantics(t *testing.T) {
	rejected := []string{
		"version:abcd1234",
		"version-id:abcd1234",
		"file:1234567",
		"file-id:1234567",
		"curseforge:1234567", // 7-digit numeric -> CF fileID shape
		"7654321",
	}
	for _, target := range rejected {
		if err := ValidateProjectTarget(target); err == nil {
			t.Errorf("ValidateProjectTarget(%q) = nil, want rejection", target)
		}
	}

	// The rejection message must guide the user.
	err := ValidateProjectTarget("file:1234567")
	if err == nil || !strings.Contains(err.Error(), "only accepts a project slug or projectID") {
		t.Fatalf("rejection guidance missing: %v", err)
	}
}

func TestValidateProjectTargetAcceptsProjects(t *testing.T) {
	accepted := []string{
		"sodium",
		"iris",
		"appleskin",
		"curseforge:394468", // 6-digit CF project id
		"modrinth:AANobbMI", // 8-char base62 project id (ambiguous by design)
		"AANobbMI",          // bare 8-char base62 -> treated as a project id
		"curseforge:jei",
	}
	for _, target := range accepted {
		if err := ValidateProjectTarget(target); err != nil {
			t.Errorf("ValidateProjectTarget(%q) = %v, want acceptance", target, err)
		}
	}
}

func TestVersionIDExclusivity(t *testing.T) {
	if err := versionIDExclusivity(3, "abc123"); err == nil {
		t.Fatal("expected mutual-exclusion error for multiple targets with --version-id")
	}
	if err := versionIDExclusivity(1, "abc123"); err != nil {
		t.Fatalf("single target with --version-id must be allowed: %v", err)
	}
	if err := versionIDExclusivity(3, ""); err != nil {
		t.Fatalf("multiple targets without --version-id must be allowed: %v", err)
	}
}

func TestParseInstallTargetsRequiresAtLeastOne(t *testing.T) {
	_, err := ParseInstallTargets(nil, "")
	if err == nil {
		t.Fatal("expected guidance error for zero targets")
	}
	if !strings.Contains(err.Error(), "mod-downloader-cli i sodium") {
		t.Fatalf("guidance message missing examples: %v", err)
	}

	targets, err := ParseInstallTargets([]string{"sodium", "curseforge:394468", "iris"}, "")
	if err != nil {
		t.Fatalf("ParseInstallTargets error: %v", err)
	}
	if len(targets) != 3 {
		t.Fatalf("expected 3 targets, got %d", len(targets))
	}
}

// fakeContext satisfies the Bool(string) bool contract used by interactiveAllowed.
type fakeContext map[string]bool

func (f fakeContext) Bool(name string) bool { return f[name] }

func TestInteractiveAllowedRespectsNonInteractiveFlags(t *testing.T) {
	for _, flag := range []string{"json", "yes", "no-input"} {
		if interactiveAllowed(fakeContext{flag: true}) {
			t.Errorf("interactiveAllowed must be false when --%s is set", flag)
		}
	}
}

func TestInstallAliasMatchesInstallCommand(t *testing.T) {
	modsDir := filepath.Join(t.TempDir(), "mods")
	if err := os.MkdirAll(modsDir, 0o755); err != nil {
		t.Fatal(err)
	}
	chdir(t, modsDir)
	isolateGlobalState(t)

	// `i` and `install` must both surface the same validation error, proving the
	// alias shares flag parsing and business logic.
	_, _, errInstall := runCLI(t, "--cache-dir", t.TempDir(), "install", "sodium")
	_, _, errAlias := runCLI(t, "--cache-dir", t.TempDir(), "i", "sodium")
	if errInstall == nil || errAlias == nil {
		t.Fatalf("expected both install and i to fail without mc-version: install=%v alias=%v", errInstall, errAlias)
	}
	if errInstall.Error() != errAlias.Error() {
		t.Fatalf("alias error mismatch:\n install: %q\n i:       %q", errInstall, errAlias)
	}
}

func TestInstallAliasHonoursTrailingFlags(t *testing.T) {
	modsDir := filepath.Join(t.TempDir(), "mods")
	if err := os.MkdirAll(modsDir, 0o755); err != nil {
		t.Fatal(err)
	}
	chdir(t, modsDir)
	isolateGlobalState(t)

	// Trailing --dry-run must survive re-parsing for the `i` alias: a bare
	// argument list is then validated, so the failure must be about the missing
	// version/loader rather than about an unknown flag.
	_, stderr, err := runCLI(t, "--cache-dir", t.TempDir(), "i", "sodium", "--dry-run", "--no-input")
	if err == nil {
		t.Fatal("expected failure without mc-version/loader")
	}
	if strings.Contains(stderr, "unknown") || strings.Contains(err.Error(), "unknown flag") {
		t.Fatalf("trailing flags were not recognised for the alias: %v / %s", err, stderr)
	}
	if !strings.Contains(err.Error(), "mc-version is required") {
		t.Fatalf("expected mc-version requirement, got %v", err)
	}
}

func TestInstallRejectsVersionIDAcrossTargets(t *testing.T) {
	modsDir := filepath.Join(t.TempDir(), "mods")
	if err := os.MkdirAll(modsDir, 0o755); err != nil {
		t.Fatal(err)
	}
	chdir(t, modsDir)

	_, _, err := runCLI(t, "--cache-dir", t.TempDir(),
		"--mc-version", "1.21.1", "--loader", "fabric",
		"i", "sodium", "iris", "--version-id", "abcd1234")
	if err == nil {
		t.Fatal("expected --version-id + multiple targets to be rejected")
	}
	if !strings.Contains(err.Error(), "--version-id cannot be combined with multiple install targets") {
		t.Fatalf("unexpected error: %v", err)
	}
}
