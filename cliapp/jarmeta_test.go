package cliapp

import (
	"testing"
)

func TestParseFabricMetadata(t *testing.T) {
	dir := t.TempDir()
	path := writeModJar(t, dir, "sodium.jar", fabricJarEntries("sodium", "0.5.8",
		`{"fabricloader":">=0.14","minecraft":"~1.21","fabric-api":"*"}`))

	meta, err := parseJarMetadata(path)
	if err != nil {
		t.Fatal(err)
	}
	if meta.Loader != "fabric" || meta.ID != "sodium" || meta.Version != "0.5.8" {
		t.Fatalf("metadata = %#v", meta)
	}
	if meta.MinecraftVersion != "~1.21" {
		t.Fatalf("minecraft version = %q, want ~1.21", meta.MinecraftVersion)
	}
	ids := map[string]bool{}
	for _, dep := range meta.Depends {
		ids[dep.ID] = true
	}
	if !ids["fabricloader"] || !ids["minecraft"] || !ids["fabric-api"] {
		t.Fatalf("depends = %#v", meta.Depends)
	}
}

func TestParseModsTomlMetadata(t *testing.T) {
	dir := t.TempDir()
	toml := `
modLoader="javafml"
loaderVersion="[47,)"
license="LGPL-3.0"

[[mods]]
modId="examplemod"
version="1.2.3"
displayName="Example Mod"
description="An example"

[[dependencies.examplemod]]
    modId="forge"
    mandatory=true
    versionRange="[47,)"
    side="BOTH"
[[dependencies.examplemod]]
    modId="minecraft"
    mandatory=true
    versionRange="[1.20,1.22)"
    side="BOTH"
`
	path := writeModJar(t, dir, "example.jar", map[string]string{"META-INF/mods.toml": toml})

	meta, err := parseJarMetadata(path)
	if err != nil {
		t.Fatal(err)
	}
	if meta.Loader != "forge" || meta.ID != "examplemod" || meta.Version != "1.2.3" {
		t.Fatalf("metadata = %#v", meta)
	}
	if meta.MinecraftVersion != "[1.20,1.22)" {
		t.Fatalf("minecraft version = %q", meta.MinecraftVersion)
	}
	if len(meta.Depends) != 2 {
		t.Fatalf("depends = %#v, want 2", meta.Depends)
	}
}

func TestParseQuiltMetadata(t *testing.T) {
	dir := t.TempDir()
	quilt := `{
		"schema_version": 1,
		"quilt_loader": {
			"id": "qmod",
			"version": "1.0.0",
			"metadata": {"name": "Q Mod", "description": "d", "license": "MIT"},
			"depends": [
				{"id": "quilt_loader", "versions": ">=0.17.0"},
				{"id": "minecraft", "versions": ">=1.20"},
				{"id": "optional-lib", "versions": "*", "optional": true}
			]
		}
	}`
	path := writeModJar(t, dir, "qmod.jar", map[string]string{"quilt.mod.json": quilt})

	meta, err := parseJarMetadata(path)
	if err != nil {
		t.Fatal(err)
	}
	if meta.Loader != "quilt" || meta.ID != "qmod" || meta.Name != "Q Mod" {
		t.Fatalf("metadata = %#v", meta)
	}
	if len(meta.Depends) != 2 {
		t.Fatalf("depends = %#v, want 2 (optional excluded)", meta.Depends)
	}
	if len(meta.Recommends) != 1 || meta.Recommends[0].ID != "optional-lib" {
		t.Fatalf("recommends = %#v", meta.Recommends)
	}
}

func TestParseMcmodInfoMetadata(t *testing.T) {
	dir := t.TempDir()
	info := `[{"modid":"legacy","name":"Legacy","version":"1.0","description":"d","authorList":["Alice"],"license":"MIT","mcversion":"1.12.2","dependencies":["forge"],"requiredMods":["other@[1,)"]}]`
	path := writeModJar(t, dir, "legacy.jar", map[string]string{"mcmod.info": info})

	meta, err := parseJarMetadata(path)
	if err != nil {
		t.Fatal(err)
	}
	if meta.ID != "legacy" || meta.Version != "1.0" {
		t.Fatalf("metadata = %#v", meta)
	}
	if meta.MinecraftVersion != "1.12.2" {
		t.Fatalf("minecraft version = %q", meta.MinecraftVersion)
	}
	if len(meta.Depends) != 2 {
		t.Fatalf("depends = %#v, want 2", meta.Depends)
	}
}

func TestVersionRangeAllows(t *testing.T) {
	tests := []struct {
		declared string
		version  string
		want     bool
		known    bool
	}{
		{"", "1.21.1", true, false},
		{"*", "1.21.1", true, false},
		{">=1.21", "1.21.1", true, true},
		{">=1.22", "1.21.1", false, true},
		{"~1.21", "1.21.1", true, true},
		{"~1.21.1", "1.21.4", true, true},
		{"~1.21.1", "1.22.0", false, true},
		{"^1.21", "1.21.9", true, true},
		{"^1.21", "2.0", false, true},
		{"[1.20.1,1.21)", "1.21.1", false, true},
		{"[1.20.1,1.21.1]", "1.21.1", true, true},
		{"1.21.x", "1.21.1", true, true},
		{"1.21.1", "1.21.1", true, true},
		{"1.21.2", "1.21.1", false, true},
		{">=1.21- <1.21.2-", "1.21.1", true, true},
		{">=1.21- <1.21.2-", "1.21.2", false, true},
		{">=1.21- <1.21.2-", "1.20.6", false, true},
		{">=1.20.1, <1.21.2-", "1.21.1", true, true},
		{">=1.20.1, <1.21.2-", "1.22", false, true},
		{">=1.21-", "1.21.4", true, true},
		{"<1.21.2-", "1.21.1", true, true},
	}
	for _, tt := range tests {
		got, known := versionRangeAllows(tt.declared, tt.version)
		if got != tt.want || known != tt.known {
			t.Fatalf("versionRangeAllows(%q, %q) = (%v, %v), want (%v, %v)", tt.declared, tt.version, got, known, tt.want, tt.known)
		}
	}
}

func TestParseFabricMetadataRealWorldShapes(t *testing.T) {
	dir := t.TempDir()
	entries := map[string]string{
		"fabric.mod.json": `{"schemaVersion":1,"id":"iris","name":"Iris","version":"1.8.14",
		"description":"shaders","authors":[{"name":"coderbot","contact":{"email":"a@b.c"}},{"name":"IMS212"}],
		"environment":"client","license":"LGPL-3.0-only",
		"depends":{"fabricloader":">=0.12.3","minecraft":["1.21.1"],"sodium":["0.8.x"]},
		"breaks":{"embeddium":"*","originsenhanced":"<=1.3.2"},
		"provides":["iris"]}`,
		"quilt.mod.json": `{"quilt_loader":{"id":"iris"}}`,
	}
	meta, err := parseJarMetadata(writeModJar(t, dir, "iris.jar", entries))
	if err != nil {
		t.Fatalf("parse failed: %v", err)
	}
	if meta.ID != "iris" || meta.Name != "Iris" || meta.Environment != "client" {
		t.Fatalf("metadata = %#v", meta)
	}
	if len(meta.Authors) != 2 || meta.Authors[0] != "coderbot" || meta.Authors[1] != "IMS212" {
		t.Fatalf("authors = %#v", meta.Authors)
	}
	if meta.MinecraftVersion != "1.21.1" {
		t.Fatalf("minecraft version = %q, want 1.21.1", meta.MinecraftVersion)
	}
	deps := map[string]string{}
	for _, dep := range meta.Depends {
		deps[dep.ID] = dep.Version
	}
	if deps["sodium"] != "0.8.x" || deps["minecraft"] != "1.21.1" || deps["fabricloader"] != ">=0.12.3" {
		t.Fatalf("depends = %#v", meta.Depends)
	}
	if len(meta.Breaks) != 2 || len(meta.Provides) != 1 {
		t.Fatalf("breaks/provides = %#v / %#v", meta.Breaks, meta.Provides)
	}

	// A multi-element constraint list must stay usable as a range.
	saltEntries := map[string]string{
		"fabric.mod.json": `{"schemaVersion":1,"id":"sodium","version":"0.8.13",
		"authors":["JellySquid"],"depends":{"minecraft":["1.21","1.21.1"]}}`,
	}
	salt, err := parseJarMetadata(writeModJar(t, dir, "sodium.jar", saltEntries))
	if err != nil {
		t.Fatalf("parse sodium failed: %v", err)
	}
	if allowed, known := versionRangeAllows(salt.MinecraftVersion, "1.21.1"); !known || !allowed {
		t.Fatalf("range %q rejected 1.21.1 (allowed=%v known=%v)", salt.MinecraftVersion, allowed, known)
	}
}
