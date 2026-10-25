package cliapp

import (
	"fmt"
	"os"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/mattn/go-isatty"

	"github.com/link-fgfgui/mod-downloader-core/appcore"
	"github.com/link-fgfgui/mod-downloader-core/global"
	"github.com/link-fgfgui/mod-downloader-core/modbridge"
	"github.com/link-fgfgui/mod-downloader-core/models"
	appstructs "github.com/link-fgfgui/mod-downloader-core/structs"
)

// platformModrinth / platformCurseforge are the canonical lowercase platform
// names used throughout the resolver.
const (
	platformModrinth   = "modrinth"
	platformCurseforge = "curseforge"
)

// lookupPlatforms is the cross-platform search set used when a target carries no
// explicit platform prefix.
var lookupPlatforms = []string{platformModrinth, platformCurseforge}

// Network-facing core calls are behind package variables so tests can inject
// deterministic fakes (mirrors the `resolveProjectsByHashesFn` seam in update.go).
var (
	lookupProjectByIDFn = func(svc *appcore.Service, platform, projectID, mcVersion, modLoader string) (models.ModProject, bool) {
		return svc.LookupProjectByID(platform, projectID, mcVersion, modLoader)
	}
	lookupProjectBySlugFn = func(svc *appcore.Service, platform, slug, mcVersion, modLoader string) (models.ModProject, bool) {
		return svc.LookupProjectBySlug(platform, slug, mcVersion, modLoader)
	}
	searchModsTextFn = func(svc *appcore.Service, req appstructs.SearchModsRequest) appstructs.SearchModsUpdate {
		return svc.SearchModsTextCollect(req)
	}
	listMatchingVersionsFn = func(svc *appcore.Service, project models.ModProject, mcVersion, modLoader string) []models.ModVersion {
		return svc.ListMatchingProjectVersions(project, mcVersion, modLoader)
	}
	versionModIDsFn = func(version models.ModVersion, modLoader string) []string {
		return modbridge.VersionModIDs(version, modLoader)
	}
	localModPathsFn = func(modIDs []string, instanceID string) []global.LocalModFilePath {
		return modbridge.LocalModPathsForModIDs(modIDs, instanceID)
	}
	instanceIDForTargetDirFn = func(targetDir string) string {
		return modbridge.InstanceIDForTargetDir(targetDir)
	}
)

// normalizePlatformName lowercases and trims a platform filter value.
func normalizePlatformName(value string) string {
	return strings.ToLower(strings.TrimSpace(value))
}

// interactiveAllowed reports whether the interactive selector may be used:
// never under --json / --yes / --no-input, and only on a real TTY stdin.
func interactiveAllowed(c interface {
	Bool(string) bool
}) bool {
	if c.Bool("json") || c.Bool("yes") || c.Bool("no-input") {
		return false
	}
	return stdinIsTerminal()
}

func stdinIsTerminal() bool {
	return isatty.IsTerminal(os.Stdin.Fd()) || isatty.IsCygwinTerminal(os.Stdin.Fd())
}

// resolveInstallTarget implements the Phase-2 decision state machine for a
// single target:
//
//	exact match on one platform  -> return it
//	multiple exact matches       -> interactive disambiguation (or auto-pick)
//	no exact match               -> search fallback -> selector (or hard error)
func resolveInstallTarget(svc *appcore.Service, target InstallTarget, runtime runtimeInput, platformFilter string, interactive bool) ([]models.ModProject, error) {
	platforms := lookupPlatforms
	if target.Platform != "" {
		platforms = []string{target.Platform}
	} else if platformFilter != "" {
		platforms = []string{platformFilter}
	}

	candidates := lookupExactCandidates(svc, platforms, target, runtime)
	sort.SliceStable(candidates, func(i, j int) bool {
		return platformRank(candidates[i].Platform) < platformRank(candidates[j].Platform)
	})

	if len(candidates) == 1 {
		return []models.ModProject{candidates[0]}, nil
	}
	if len(candidates) > 1 {
		if !interactive {
			chosen := pickByPlatformPreference(candidates, platformFilter)
			_, _ = fmt.Fprintf(os.Stderr, "note: %q matched multiple platforms; selected %s non-interactively (use --platform to choose, or run in a TTY to pick)\n",
				target.Raw, strings.ToLower(chosen.Platform))
			return []models.ModProject{chosen}, nil
		}
		chosen, ok := runCandidateSelector(fmt.Sprintf("? Multiple exact matches for %q. Select one to install:", target.Raw), candidates, false, svc, runtime)
		if !ok {
			return nil, fmt.Errorf("selection canceled for %q", target.Raw)
		}
		return chosen, nil
	}

	// No exact match: fall back to a keyword search.
	results := searchTargets(svc, target, runtime)
	if len(results) == 0 {
		return nil, fmt.Errorf("no matching project found for %q", target.Raw)
	}
	if len(results) == 1 {
		return []models.ModProject{results[0]}, nil
	}
	if !interactive {
		return nil, fmt.Errorf(
			"no exact match for %q; found %d search results.\nIn non-interactive mode, provide an exact slug or a platform-prefixed ID (e.g. curseforge:394468)",
			target.Raw, len(results))
	}
	chosen, ok := runCandidateSelector(fmt.Sprintf("? No exact match for %q. Found %d search results:", target.Raw, len(results)), results, true, svc, runtime)
	if !ok {
		return nil, fmt.Errorf("selection canceled for %q", target.Raw)
	}
	return chosen, nil
}

// lookupExactCandidates queries the given platforms concurrently and returns
// every exact hit. A platform that errors or misses simply contributes nothing.
func lookupExactCandidates(svc *appcore.Service, platforms []string, target InstallTarget, runtime runtimeInput) []models.ModProject {
	type result struct {
		project models.ModProject
		ok      bool
	}
	results := make([]result, len(platforms))
	var wg sync.WaitGroup
	for i, platform := range platforms {
		wg.Add(1)
		go func(i int, platform string) {
			defer wg.Done()
			results[i] = result{project: lookupOnPlatform(svc, platform, target, runtime), ok: true}
		}(i, platform)
	}
	wg.Wait()

	out := make([]models.ModProject, 0, len(platforms))
	seen := map[string]bool{}
	for _, r := range results {
		if r.project.ProjectID == "" {
			continue
		}
		key := strings.ToLower(r.project.Platform) + ":" + r.project.ProjectID
		if seen[key] {
			continue
		}
		seen[key] = true
		out = append(out, r.project)
	}
	return out
}

func lookupOnPlatform(svc *appcore.Service, platform string, target InstallTarget, runtime runtimeInput) models.ModProject {
	if target.IsID {
		if project, ok := lookupProjectByIDFn(svc, platform, target.Query, runtime.MinecraftVersion, runtime.ModLoader); ok {
			return project
		}
	}
	if project, ok := lookupProjectBySlugFn(svc, platform, target.Query, runtime.MinecraftVersion, runtime.ModLoader); ok {
		return project
	}
	if project, ok := svc.LookupProject(platform, target.Query, runtime.MinecraftVersion, runtime.ModLoader); ok {
		return project
	}
	return models.ModProject{}
}

// searchTargets performs the keyword-search fallback.
func searchTargets(svc *appcore.Service, target InstallTarget, runtime runtimeInput) []models.ModProject {
	update := searchModsTextFn(svc, appstructs.SearchModsRequest{
		RequestID: "cli-search",
		Query:     target.Query,
		Version:   runtime.MinecraftVersion,
		ModLoader: runtime.ModLoader,
		Limit:     10,
	})
	results := update.Results
	if len(results) == 0 {
		return nil
	}
	sort.SliceStable(results, func(i, j int) bool { return results[i].Downloads > results[j].Downloads })
	if len(results) > 10 {
		results = results[:10]
	}
	return results
}

// platformRank orders platforms deterministically (Modrinth first).
func platformRank(platform string) int {
	if strings.EqualFold(platform, platformModrinth) {
		return 0
	}
	if strings.EqualFold(platform, platformCurseforge) {
		return 1
	}
	return 2
}

// pickByPlatformPreference selects a candidate non-interactively: the explicit
// filter wins, otherwise Modrinth is preferred (with a stderr notice).
func pickByPlatformPreference(candidates []models.ModProject, platformFilter string) models.ModProject {
	if platformFilter != "" {
		for _, candidate := range candidates {
			if strings.EqualFold(candidate.Platform, platformFilter) {
				return candidate
			}
		}
	}
	best := candidates[0]
	for _, candidate := range candidates {
		if platformRank(candidate.Platform) < platformRank(best.Platform) {
			best = candidate
		}
	}
	return best
}

// InstallPlanEntry is one row of the resolved install preview / JSON report.
type InstallPlanEntry struct {
	Target     string   `json:"target"`
	Platform   string   `json:"platform"`
	ProjectID  string   `json:"projectId"`
	Title      string   `json:"title"`
	Version    string   `json:"version,omitempty"`
	ModIDs     []string `json:"modIds,omitempty"`
	FileName   string   `json:"fileName,omitempty"`
	FileSize   int64    `json:"fileSize,omitempty"`
	LocalState string   `json:"localState,omitempty"`

	Project models.ModProject `json:"-"`
}

// metadataTimeout bounds each candidate's metadata resolution (Phase 3.3). A
// slow or failing probe must never block the pipeline or the selector.
// metadataTimeout is a variable so tests can shorten it.
var metadataTimeout = 3 * time.Second

// versionLookupStatus distinguishes the outcomes of a bounded version probe.
type versionLookupStatus int

const (
	versionFound versionLookupStatus = iota
	versionMissing
	versionTimeout
)

// preferredVersionWithin runs the preferred-version lookup under a deadline.
func preferredVersionWithin(svc *appcore.Service, project models.ModProject, runtime runtimeInput, timeout time.Duration) (models.ModVersion, versionLookupStatus) {
	results := make(chan []models.ModVersion, 1)
	// Capture the seam before starting the goroutine: on timeout the caller
	// returns while the lookup is still running, and tests may restore the seam
	// immediately afterwards.
	lookup := listMatchingVersionsFn
	go func() {
		results <- lookup(svc, project, runtime.MinecraftVersion, runtime.ModLoader)
	}()
	timer := time.NewTimer(timeout)
	defer timer.Stop()
	select {
	case versions := <-results:
		if len(versions) == 0 {
			return models.ModVersion{}, versionMissing
		}
		return versions[0], versionFound
	case <-timer.C:
		return models.ModVersion{}, versionTimeout
	}
}

// slugFallbackModIDs returns the project slug as the display modid used when the
// remote probe times out or yields nothing.
func slugFallbackModIDs(project models.ModProject) []string {
	if strings.TrimSpace(project.Slug) != "" {
		return []string{project.Slug}
	}
	return nil
}

// buildInstallPlanEntry resolves the preferred version, real mod IDs and local
// state for one project (Phase 3 steps A–C).
func buildInstallPlanEntry(svc *appcore.Service, targetRaw string, project models.ModProject, runtime runtimeInput) InstallPlanEntry {
	entry := InstallPlanEntry{
		Target:    firstNonEmptyStr(targetRaw, project.ID),
		Platform:  project.Platform,
		ProjectID: project.ProjectID,
		Title:     firstNonEmptyStr(project.Title, project.Slug, project.ProjectID),
		Project:   project,
	}

	version, status := preferredVersionWithin(svc, project, runtime, metadataTimeout)
	switch status {
	case versionFound:
		entry.Version = firstNonEmptyStr(version.Version, version.Name, version.ID)
		entry.FileName = version.FileName
		entry.FileSize = version.FileSize
		entry.ModIDs = resolveModIDs(version, runtime.ModLoader)
		if len(entry.ModIDs) == 0 {
			entry.ModIDs = slugFallbackModIDs(project)
		}
		entry.LocalState = localStateFor(entry.ModIDs, runtime)
	case versionTimeout:
		// Graceful degradation: fall back to the slug and flag the timeout so the
		// preview/report never blocks on a slow provider.
		entry.ModIDs = slugFallbackModIDs(project)
		entry.LocalState = "metadata_timeout"
	case versionMissing:
		entry.ModIDs = slugFallbackModIDs(project)
		entry.LocalState = localStateFor(entry.ModIDs, runtime)
	}
	return entry
}

// resolveModIDs implements Phase 3 Step B: cached version mod IDs first, then
// the version's own parsed IDs. The core's modbridge performs the HTTP range
// probe and persists the result when the cache misses.
func resolveModIDs(version models.ModVersion, modLoader string) []string {
	ids := versionModIDsFn(version, modLoader)
	if len(ids) > 0 {
		return ids
	}
	if len(version.ModIDs) > 0 {
		return version.ModIDs
	}
	return nil
}

// localStateFor implements Phase 3 Step C: compare against installed local mods.
func localStateFor(modIDs []string, runtime runtimeInput) string {
	if len(modIDs) == 0 || runtime.TargetModsDir == "" {
		return "not_installed"
	}
	instanceID := instanceIDForTargetDirFn(runtime.TargetModsDir)
	paths := localModPathsFn(modIDs, instanceID)
	if len(paths) == 0 {
		return "not_installed"
	}
	return "installed"
}

func firstNonEmptyStr(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}
