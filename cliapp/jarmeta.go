package cliapp

import (
	"archive/zip"
	"crypto/sha512"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"sort"
	"strings"

	"github.com/BurntSushi/toml"
)

// jarDependency is a dependency declared inside a mod jar's metadata.
type jarDependency struct {
	ID        string `json:"id"`
	Version   string `json:"version,omitempty"`
	Mandatory bool   `json:"mandatory"`
	Side      string `json:"side,omitempty"`
}

// jarMetadata is the normalized metadata parsed from a mod jar. It covers the
// common Fabric, Quilt, Forge, NeoForge, and legacy mcmod.info formats.
type jarMetadata struct {
	Loader           string          `json:"loader"`
	ID               string          `json:"id"`
	Name             string          `json:"name"`
	Version          string          `json:"version"`
	Description      string          `json:"description,omitempty"`
	Authors          []string        `json:"authors,omitempty"`
	License          string          `json:"license,omitempty"`
	Environment      string          `json:"environment,omitempty"`
	MinecraftVersion string          `json:"minecraftVersion,omitempty"`
	Depends          []jarDependency `json:"depends,omitempty"`
	Recommends       []jarDependency `json:"recommends,omitempty"`
	Breaks           []jarDependency `json:"breaks,omitempty"`
	Provides         []string        `json:"provides,omitempty"`
}

// builtinModIDs are dependency targets that are always present in a Minecraft
// instance and must not be reported as missing.
var builtinModIDs = map[string]bool{
	"minecraft":    true,
	"java":         true,
	"fabricloader": true,
	"fabric":       true,
	"fabric-api":   false,
	"forge":        true,
	"neoforge":     true,
	"javafml":      true,
	"lowcodefml":   true,
	"fml":          true,
	"architectury": false,
}

func isBuiltinModID(id string) bool {
	return builtinModIDs[strings.ToLower(strings.TrimSpace(id))]
}

func parseJarMetadata(jarPath string) (jarMetadata, error) {
	r, err := zip.OpenReader(jarPath)
	if err != nil {
		return jarMetadata{}, err
	}
	defer r.Close()
	return parseZipMetadata(&r.Reader)
}

func parseZipMetadata(r *zip.Reader) (jarMetadata, error) {
	if hasZipFile(r, "fabric.mod.json") {
		if meta, err := parseFabricMetadata(r); err == nil {
			return meta, nil
		}
	}
	if hasZipFile(r, "quilt.mod.json") {
		if meta, err := parseQuiltMetadata(r); err == nil {
			return meta, nil
		}
	}
	if hasZipFile(r, "META-INF/neoforge.mods.toml") {
		if meta, err := parseModsTomlMetadata(r, "META-INF/neoforge.mods.toml", "neoforge"); err == nil {
			return meta, nil
		}
	}
	if hasZipFile(r, "META-INF/mods.toml") {
		if meta, err := parseModsTomlMetadata(r, "META-INF/mods.toml", "forge"); err == nil {
			return meta, nil
		}
	}
	if hasZipFile(r, "mcmod.info") {
		if meta, err := parseMcmodInfoMetadata(r); err == nil {
			return meta, nil
		}
	}
	return jarMetadata{}, fmt.Errorf("no parseable mod metadata in jar")
}

func parseFabricMetadata(r *zip.Reader) (jarMetadata, error) {
	data, err := readZipEntry(r, "fabric.mod.json")
	if err != nil {
		return jarMetadata{}, err
	}
	var raw struct {
		ID          string                     `json:"id"`
		Name        string                     `json:"name"`
		Version     string                     `json:"version"`
		Description string                     `json:"description"`
		Authors     flexibleStrings            `json:"authors"`
		License     string                     `json:"license"`
		Environment string                     `json:"environment"`
		Depends     map[string]flexibleStrings `json:"depends"`
		Recommends  map[string]flexibleStrings `json:"recommends"`
		Breaks      map[string]flexibleStrings `json:"breaks"`
		Conflicts   map[string]flexibleStrings `json:"conflicts"`
		Provides    flexibleStrings            `json:"provides"`
	}
	if err := json.Unmarshal(data, &raw); err != nil {
		return jarMetadata{}, fmt.Errorf("parse fabric.mod.json: %w", err)
	}
	meta := jarMetadata{
		Loader:      "fabric",
		ID:          strings.TrimSpace(raw.ID),
		Name:        strings.TrimSpace(raw.Name),
		Version:     strings.TrimSpace(raw.Version),
		Description: strings.TrimSpace(raw.Description),
		License:     strings.TrimSpace(raw.License),
		Environment: strings.TrimSpace(raw.Environment),
		Provides:    normalizeIDs(raw.Provides.strings()),
	}
	meta.Authors = raw.Authors.strings()
	meta.Depends = dependenciesFromMap(raw.Depends, true)
	meta.Recommends = dependenciesFromMap(raw.Recommends, false)
	breaks := append(dependenciesFromMap(raw.Breaks, false), dependenciesFromMap(raw.Conflicts, false)...)
	meta.Breaks = breaks
	meta.MinecraftVersion = mapValue(raw.Depends, "minecraft")
	return meta, nil
}

func parseQuiltMetadata(r *zip.Reader) (jarMetadata, error) {
	data, err := readZipEntry(r, "quilt.mod.json")
	if err != nil {
		return jarMetadata{}, err
	}
	var raw struct {
		QuiltLoader struct {
			ID       string `json:"id"`
			Version  string `json:"version"`
			Provides []struct {
				ID string `json:"id"`
			} `json:"provides"`
			Depends []struct {
				ID       string `json:"id"`
				Versions string `json:"versions"`
				Optional bool   `json:"optional"`
			} `json:"depends"`
			Metadata struct {
				Name         string            `json:"name"`
				Description  string            `json:"description"`
				License      string            `json:"license"`
				Contributors map[string]string `json:"contributors"`
			} `json:"metadata"`
		} `json:"quilt_loader"`
	}
	if err := json.Unmarshal(data, &raw); err != nil {
		return jarMetadata{}, fmt.Errorf("parse quilt.mod.json: %w", err)
	}
	ql := raw.QuiltLoader
	meta := jarMetadata{
		Loader:      "quilt",
		ID:          strings.TrimSpace(ql.ID),
		Name:        strings.TrimSpace(ql.Metadata.Name),
		Version:     strings.TrimSpace(ql.Version),
		Description: strings.TrimSpace(ql.Metadata.Description),
		License:     strings.TrimSpace(ql.Metadata.License),
	}
	for id, role := range ql.Metadata.Contributors {
		meta.Authors = append(meta.Authors, fmt.Sprintf("%s (%s)", id, role))
	}
	sort.Strings(meta.Authors)
	for _, provide := range ql.Provides {
		if id := strings.TrimSpace(provide.ID); id != "" {
			meta.Provides = append(meta.Provides, id)
		}
	}
	for _, dep := range ql.Depends {
		id := strings.TrimSpace(dep.ID)
		if id == "" {
			continue
		}
		entry := jarDependency{ID: id, Version: strings.TrimSpace(dep.Versions), Mandatory: !dep.Optional}
		if strings.EqualFold(id, "minecraft") {
			meta.MinecraftVersion = entry.Version
		}
		if entry.Mandatory {
			meta.Depends = append(meta.Depends, entry)
		} else {
			meta.Recommends = append(meta.Recommends, entry)
		}
	}
	return meta, nil
}

type tomlModEntry struct {
	ModID       string `toml:"modId"`
	Version     string `toml:"version"`
	DisplayName string `toml:"displayName"`
	Description string `toml:"description"`
	Authors     string `toml:"authors"`
}

type tomlDependencyEntry struct {
	ModID        string `toml:"modId"`
	Mandatory    bool   `toml:"mandatory"`
	VersionRange string `toml:"versionRange"`
	Side         string `toml:"side"`
}

func parseModsTomlMetadata(r *zip.Reader, path, loader string) (jarMetadata, error) {
	data, err := readZipEntry(r, path)
	if err != nil {
		return jarMetadata{}, err
	}
	var raw struct {
		Licenses     string                           `toml:"license"`
		Mods         []tomlModEntry                   `toml:"mods"`
		Dependencies map[string][]tomlDependencyEntry `toml:"dependencies"`
	}
	if err := toml.Unmarshal(data, &raw); err != nil {
		return jarMetadata{}, fmt.Errorf("parse %s: %w", path, err)
	}
	if len(raw.Mods) == 0 {
		return jarMetadata{}, fmt.Errorf("%s declares no mods", path)
	}
	primary := raw.Mods[0]
	meta := jarMetadata{
		Loader:      loader,
		ID:          strings.TrimSpace(primary.ModID),
		Name:        strings.TrimSpace(primary.DisplayName),
		Version:     strings.TrimSpace(primary.Version),
		Description: strings.TrimSpace(primary.Description),
		License:     strings.TrimSpace(raw.Licenses),
	}
	if meta.Name == "" {
		meta.Name = meta.ID
	}
	if authors := strings.TrimSpace(primary.Authors); authors != "" {
		for _, a := range strings.Split(authors, ",") {
			if trimmed := strings.TrimSpace(a); trimmed != "" {
				meta.Authors = append(meta.Authors, trimmed)
			}
		}
	}
	for _, deps := range raw.Dependencies {
		for _, dep := range deps {
			id := strings.TrimSpace(dep.ModID)
			if id == "" {
				continue
			}
			entry := jarDependency{ID: id, Version: strings.TrimSpace(dep.VersionRange), Mandatory: dep.Mandatory, Side: strings.TrimSpace(dep.Side)}
			if strings.EqualFold(id, "minecraft") {
				meta.MinecraftVersion = entry.Version
			}
			if entry.Mandatory {
				meta.Depends = append(meta.Depends, entry)
			} else {
				meta.Recommends = append(meta.Recommends, entry)
			}
		}
	}
	return meta, nil
}

func parseMcmodInfoMetadata(r *zip.Reader) (jarMetadata, error) {
	data, err := readZipEntry(r, "mcmod.info")
	if err != nil {
		return jarMetadata{}, err
	}
	type mcmodEntry struct {
		ModID        string   `json:"modid"`
		Name         string   `json:"name"`
		Version      string   `json:"version"`
		Description  string   `json:"description"`
		AuthorList   []string `json:"authorList"`
		License      string   `json:"license"`
		MCVersion    string   `json:"mcversion"`
		Dependencies []string `json:"dependencies"`
		RequiredMods []string `json:"requiredMods"`
	}
	var entries []mcmodEntry
	if err := json.Unmarshal(data, &entries); err != nil {
		var wrapper struct {
			ModList []mcmodEntry `json:"modList"`
		}
		if err := json.Unmarshal(data, &wrapper); err != nil {
			return jarMetadata{}, fmt.Errorf("parse mcmod.info: %w", err)
		}
		entries = wrapper.ModList
	}
	if len(entries) == 0 {
		return jarMetadata{}, fmt.Errorf("mcmod.info declares no mods")
	}
	primary := entries[0]
	meta := jarMetadata{
		Loader:      "forge",
		ID:          strings.TrimSpace(primary.ModID),
		Name:        strings.TrimSpace(primary.Name),
		Version:     strings.TrimSpace(primary.Version),
		Description: strings.TrimSpace(primary.Description),
		License:     strings.TrimSpace(primary.License),
		Authors:     normalizeIDs(primary.AuthorList),
	}
	if meta.Name == "" {
		meta.Name = meta.ID
	}
	for _, dep := range append(primary.Dependencies, primary.RequiredMods...) {
		dep = strings.TrimSpace(dep)
		if dep == "" {
			continue
		}
		id, version := parseLegacyDependency(dep)
		meta.Depends = append(meta.Depends, jarDependency{ID: id, Version: version, Mandatory: true})
		if strings.EqualFold(id, "minecraft") {
			meta.MinecraftVersion = version
		}
	}
	if strings.TrimSpace(primary.MCVersion) != "" {
		meta.MinecraftVersion = strings.TrimSpace(primary.MCVersion)
	}
	return meta, nil
}

func parseLegacyDependency(value string) (string, string) {
	if idx := strings.IndexAny(value, "@[]()"); idx >= 0 {
		return strings.TrimSpace(value[:idx]), strings.TrimSpace(value[idx:])
	}
	return value, ""
}

// flexibleStrings accepts the several shapes Fabric and Quilt use for
// list-like fields: a bare string, an array of strings, or an array of objects
// carrying the value under "name", "id", or "versions".
type flexibleStrings []string

func (f *flexibleStrings) UnmarshalJSON(data []byte) error {
	trimmed := strings.TrimSpace(string(data))
	if trimmed == "" || trimmed == "null" {
		*f = nil
		return nil
	}
	if trimmed[0] == '"' {
		var single string
		if err := json.Unmarshal(data, &single); err != nil {
			return err
		}
		*f = []string{single}
		return nil
	}
	var list []json.RawMessage
	if err := json.Unmarshal(data, &list); err != nil {
		return err
	}
	out := make([]string, 0, len(list))
	for _, item := range list {
		var text string
		if err := json.Unmarshal(item, &text); err == nil {
			out = append(out, text)
			continue
		}
		var obj map[string]json.RawMessage
		if err := json.Unmarshal(item, &obj); err != nil {
			continue
		}
		for _, key := range []string{"name", "id", "versions", "version"} {
			raw, ok := obj[key]
			if !ok {
				continue
			}
			var value flexibleStrings
			if err := value.UnmarshalJSON(raw); err != nil {
				continue
			}
			out = append(out, value.strings()...)
		}
	}
	*f = out
	return nil
}

func (f flexibleStrings) strings() []string {
	out := make([]string, 0, len(f))
	for _, value := range f {
		if trimmed := strings.TrimSpace(value); trimmed != "" {
			out = append(out, trimmed)
		}
	}
	return out
}

func dependenciesFromMap(values map[string]flexibleStrings, mandatory bool) []jarDependency {
	deps := make([]jarDependency, 0, len(values))
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for _, key := range keys {
		id := strings.TrimSpace(key)
		if id == "" {
			continue
		}
		deps = append(deps, jarDependency{ID: id, Version: joinVersionConstraints(values[key]), Mandatory: mandatory})
	}
	return deps
}

func mapValue(values map[string]flexibleStrings, key string) string {
	for k, v := range values {
		if strings.EqualFold(strings.TrimSpace(k), key) {
			return joinVersionConstraints(v)
		}
	}
	return ""
}

// joinVersionConstraints flattens a dependency value declared as a list of
// alternatives into a single range expression, keeping the bracketed form when
// several constraints are present.
func joinVersionConstraints(values flexibleStrings) string {
	parts := values.strings()
	switch len(parts) {
	case 0:
		return ""
	case 1:
		return parts[0]
	default:
		return "[" + strings.Join(parts, ",") + "]"
	}
}

func normalizeIDs(values []string) []string {
	out := make([]string, 0, len(values))
	seen := make(map[string]bool, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" || seen[strings.ToLower(value)] {
			continue
		}
		seen[strings.ToLower(value)] = true
		out = append(out, value)
	}
	return out
}

func hasZipFile(r *zip.Reader, name string) bool {
	for _, f := range r.File {
		if f.Name == name {
			return true
		}
	}
	return false
}

func readZipEntry(r *zip.Reader, name string) ([]byte, error) {
	for _, f := range r.File {
		if f.Name != name {
			continue
		}
		rc, err := f.Open()
		if err != nil {
			return nil, err
		}
		defer rc.Close()
		return io.ReadAll(rc)
	}
	return nil, fmt.Errorf("file not found in archive: %s", name)
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

// versionRangeAllows reports whether a declared dependency version range accepts
// the given version. It supports the common Maven-style and prefix operators
// used by mod metadata, and treats an empty or wildcard range as "any". Compound
// ranges may be separated by commas or whitespace (for example Modrinth's
// ">=1.21- <1.21.2-"); every clause must accept the version.
func versionRangeAllows(declared, version string) (bool, bool) {
	declared = strings.TrimSpace(declared)
	if declared == "" || declared == "*" {
		return true, false
	}
	version = strings.TrimSpace(version)
	if version == "" {
		return true, false
	}
	if strings.HasPrefix(declared, "[") || strings.HasPrefix(declared, "(") {
		return mavenRangeAllows(declared, version), true
	}

	parts := splitRangeClauses(declared)
	if len(parts) == 0 {
		return false, false
	}
	allowed := true
	for _, part := range parts {
		ok, recognized := singleRangeAllows(part, version)
		if !recognized {
			return false, false
		}
		if !ok {
			allowed = false
		}
	}
	return allowed, true
}

// splitRangeClauses splits a compound range on commas and whitespace.
func splitRangeClauses(declared string) []string {
	fields := strings.FieldsFunc(declared, func(r rune) bool {
		return r == ',' || r == ' ' || r == '\t' || r == '\n' || r == '\r'
	})
	parts := make([]string, 0, len(fields))
	for _, field := range fields {
		if field = strings.TrimSpace(field); field != "" {
			parts = append(parts, field)
		}
	}
	return parts
}

func singleRangeAllows(part, version string) (bool, bool) {
	switch {
	case strings.HasPrefix(part, ">="):
		return compareVersions(version, strings.TrimSpace(part[2:])) >= 0, true
	case strings.HasPrefix(part, "<="):
		return compareVersions(version, strings.TrimSpace(part[2:])) <= 0, true
	case strings.HasPrefix(part, ">"):
		return compareVersions(version, strings.TrimSpace(part[1:])) > 0, true
	case strings.HasPrefix(part, "<"):
		return compareVersions(version, strings.TrimSpace(part[1:])) < 0, true
	case strings.HasPrefix(part, "=="):
		return compareVersions(version, strings.TrimSpace(part[2:])) == 0, true
	case strings.HasPrefix(part, "~"):
		return tildeAllows(strings.TrimSpace(part[1:]), version), true
	case strings.HasPrefix(part, "^"):
		return caretAllows(strings.TrimSpace(part[1:]), version), true
	case strings.HasPrefix(part, "["):
		return mavenRangeAllows(part, version), true
	case strings.HasPrefix(part, "("):
		return mavenRangeAllows(part, version), true
	default:
		if idx := strings.Index(part, ".x"); idx >= 0 {
			prefix := strings.TrimSuffix(part[:idx], ".")
			return strings.HasPrefix(version, prefix+".") || version == prefix, true
		}
		return compareVersions(version, part) == 0, true
	}
}

// tildeAllows implements "~1.21.1" => >=1.21.1 <1.22.0 (patch-level flexibility).
func tildeAllows(base, version string) bool {
	baseParts := splitVersion(base)
	versionParts := splitVersion(version)
	if len(baseParts) == 0 || len(versionParts) == 0 {
		return false
	}
	if len(baseParts) == 1 {
		return versionParts[0] == baseParts[0]
	}
	if compareVersions(version, base) < 0 {
		return false
	}
	upper := make([]int, len(baseParts))
	copy(upper, baseParts)
	upper[len(baseParts)-2]++
	for i := len(baseParts) - 1; i > len(baseParts)-2; i-- {
		upper[i] = 0
	}
	return compareVersionParts(versionParts, upper) < 0
}

// caretAllows implements "^1.21.1" => >=1.21.1 <2.0.0 (major-level flexibility).
func caretAllows(base, version string) bool {
	baseParts := splitVersion(base)
	versionParts := splitVersion(version)
	if len(baseParts) == 0 || len(versionParts) == 0 {
		return false
	}
	if compareVersions(version, base) < 0 {
		return false
	}
	upper := make([]int, len(baseParts))
	copy(upper, baseParts)
	upper[0]++
	for i := 1; i < len(upper); i++ {
		upper[i] = 0
	}
	return compareVersionParts(versionParts, upper) < 0
}

func mavenRangeAllows(part, version string) bool {
	part = strings.TrimSpace(part)
	trimmed := strings.Trim(part, "[]()")
	parts := strings.SplitN(trimmed, ",", 2)
	if len(parts) != 2 {
		return false
	}
	low := strings.TrimSpace(parts[0])
	high := strings.TrimSpace(parts[1])
	lowInclusive := strings.HasPrefix(part, "[")
	highInclusive := strings.HasSuffix(part, "]")
	cmpLow := compareVersions(version, low)
	cmpHigh := compareVersions(version, high)
	if low != "" {
		if cmpLow < 0 || (cmpLow == 0 && !lowInclusive) {
			return false
		}
	}
	if high != "" {
		if cmpHigh > 0 || (cmpHigh == 0 && !highInclusive) {
			return false
		}
	}
	return true
}

func compareVersions(left, right string) int {
	return compareVersionParts(splitVersion(left), splitVersion(right))
}

func compareVersionParts(left, right []int) int {
	size := len(left)
	if len(right) > size {
		size = len(right)
	}
	for i := 0; i < size; i++ {
		var l, r int
		if i < len(left) {
			l = left[i]
		}
		if i < len(right) {
			r = right[i]
		}
		if l != r {
			if l < r {
				return -1
			}
			return 1
		}
	}
	return 0
}

func splitVersion(version string) []int {
	version = strings.TrimSpace(version)
	version = strings.TrimPrefix(version, "v")
	version = strings.TrimPrefix(version, "^")
	version = strings.TrimPrefix(version, "~")
	parts := make([]int, 0, 4)
	for _, raw := range strings.Split(version, ".") {
		digits := strings.Builder{}
		for _, ch := range raw {
			if ch < '0' || ch > '9' {
				break
			}
			digits.WriteRune(ch)
		}
		if digits.Len() == 0 {
			parts = append(parts, 0)
			continue
		}
		value, err := parseIntSafe(digits.String())
		if err != nil {
			parts = append(parts, 0)
			continue
		}
		parts = append(parts, value)
	}
	return parts
}

func parseIntSafe(value string) (int, error) {
	result := 0
	for _, ch := range value {
		if ch < '0' || ch > '9' {
			return 0, fmt.Errorf("invalid digit")
		}
		result = result*10 + int(ch-'0')
	}
	return result, nil
}
