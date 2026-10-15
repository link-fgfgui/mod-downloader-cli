package cliapp

import (
	"errors"
	"fmt"
	"strings"

	"github.com/link-fgfgui/mod-downloader-core/models"
)

// InstallTarget is one parsed, project-level install request.
//
// The `install` / `i` command declares *projects* (slug or project ID) to
// introduce into the current instance; the concrete version is always derived
// from the active [MinecraftVersion | ModLoader] and the preferred release
// variant. Version/file identifiers are therefore rejected up-front (see
// ValidateProjectTarget).
type InstallTarget struct {
	// Raw is the original user-supplied reference.
	Raw string
	// Platform is the explicit or defaulted platform ("modrinth" | "curseforge" | "").
	Platform string
	// Query is the slug / project ID / search text.
	Query string
	// IsID reports whether Query looks like a bare numeric project ID (CurseForge).
	IsID bool
}

// ParseInstallTarget splits a raw reference into platform + query using the
// shared `models.ParseProjectKey` contract. A non-empty defaultPlatform acts as
// the fallback when the reference carries no explicit prefix.
func ParseInstallTarget(raw, defaultPlatform string) (InstallTarget, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return InstallTarget{}, fmt.Errorf("empty install target")
	}
	platform, query := models.ParseProjectKey(raw)
	if platform == "" {
		platform = strings.ToLower(strings.TrimSpace(defaultPlatform))
	}
	if strings.TrimSpace(query) == "" {
		return InstallTarget{}, fmt.Errorf("invalid install target: %q", raw)
	}
	return InstallTarget{
		Raw:      raw,
		Platform: platform,
		Query:    query,
		IsID:     isNumeric(query),
	}, nil
}

// ValidateProjectTarget rejects references that name a concrete version or file
// rather than a project, returning a friendly, actionable diagnostic.
//
// Definitive rules:
//   - explicit `version:`, `version-id:`, `file:`, `file-id:` prefixes;
//   - CurseForge file IDs: pure numeric identifiers of 7+ digits (CurseForge
//     project ("mod") IDs are shorter; file IDs are the large 7–9 digit values).
//
// Deliberate exclusion: a bare 8-character Base62 string is *not* rejected.
// Modrinth project IDs and version IDs share that exact shape (the spec's own
// example project ID `AANobbMI` is 8 characters), so a static rule would reject
// valid projects. The definitive prefixes above are the reliable signal.
func ValidateProjectTarget(target string) error {
	raw := strings.TrimSpace(target)
	if raw == "" {
		return fmt.Errorf("empty install target")
	}
	lower := strings.ToLower(raw)

	for _, prefix := range []string{"version:", "version-id:", "file:", "file-id:"} {
		if strings.HasPrefix(lower, prefix) {
			return versionFileSemanticsError(raw, "an explicit "+prefix+" prefix")
		}
	}

	// Inspect the identifier part (ignoring any platform prefix) for CF file IDs.
	_, query := models.ParseProjectKey(raw)
	query = strings.TrimSpace(query)
	if isNumeric(query) && len(query) >= 7 {
		return versionFileSemanticsError(raw, "a CurseForge fileID-like numeric value")
	}
	return nil
}

func versionFileSemanticsError(raw, reason string) error {
	return fmt.Errorf(`Error: %q appears to be %s.
The "install" (or "i") command only accepts a project slug or projectID (e.g. "sodium" or "curseforge:394468").
The CLI automatically selects the best compatible version for the active Minecraft version and mod loader.`, raw, reason)
}

// isNumeric reports whether s is composed solely of ASCII digits.
func isNumeric(s string) bool {
	if s == "" {
		return false
	}
	for _, r := range s {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}

// ValidateInstallTargets validates every target and returns the first error.
func ValidateInstallTargets(targets []string) error {
	for _, target := range targets {
		if err := ValidateProjectTarget(target); err != nil {
			return err
		}
	}
	return nil
}

// emptyTargetsMessage guides the user when no project was supplied.
const emptyTargetsMessage = `no install target given.

Provide one or more projects to install, for example:
  mod-downloader-cli i sodium
  mod-downloader-cli i iris fabric-api
  mod-downloader-cli i curseforge:394468 modrinth:AANobbMI

Use --project <ref> to pass a reference via a flag instead of a positional argument.`

// ParseInstallTargets validates and parses every reference, applying
// defaultPlatform to references without an explicit platform prefix.
func ParseInstallTargets(refs []string, defaultPlatform string) ([]InstallTarget, error) {
	if len(refs) == 0 {
		return nil, errors.New(emptyTargetsMessage)
	}
	if err := ValidateInstallTargets(refs); err != nil {
		return nil, err
	}
	targets := make([]InstallTarget, 0, len(refs))
	for _, ref := range refs {
		target, err := ParseInstallTarget(ref, defaultPlatform)
		if err != nil {
			return nil, err
		}
		targets = append(targets, target)
	}
	return targets, nil
}

// versionIDExclusivity guards the single-multi-target invariant: a single
// explicit version ID cannot be applied to multiple distinct projects. It is a
// no-op when no version ID was supplied.
func versionIDExclusivity(targetCount int, versionID string) error {
	if strings.TrimSpace(versionID) == "" || targetCount <= 1 {
		return nil
	}
	return fmt.Errorf("--version-id cannot be combined with multiple install targets (%d given); "+
		"specify a single project, or let the CLI derive the version for each project", targetCount)
}
