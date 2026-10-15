package cliapp

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestUserConfigPathUsesUserConfigDir(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", dir)

	path, err := userConfigPath()
	if err != nil {
		t.Fatal(err)
	}
	want := filepath.Join(dir, configDirName, configFileName)
	if path != want {
		t.Fatalf("userConfigPath() = %q, want %q", path, want)
	}
}

func TestUserConfigSetValidatesAndNormalizes(t *testing.T) {
	var cfg userConfig
	if err := cfg.set("loader", "Fabric"); err != nil {
		t.Fatal(err)
	}
	if cfg.Loader != "fabric" {
		t.Fatalf("loader = %q, want fabric", cfg.Loader)
	}
	if err := cfg.set("loader", "vanilla"); err == nil {
		t.Fatal("expected invalid loader error")
	}
	if err := cfg.set("unknown-key", "x"); err == nil {
		t.Fatal("expected unknown key error")
	}
	if err := cfg.set("mc-version", " 1.21.1 "); err != nil {
		t.Fatal(err)
	}
	if cfg.MinecraftVersion != "1.21.1" {
		t.Fatalf("mc-version = %q, want trimmed", cfg.MinecraftVersion)
	}
}

func TestUserConfigSaveLoadUnset(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.json")
	cfg := userConfig{}
	if err := cfg.set("loader", "fabric"); err != nil {
		t.Fatal(err)
	}
	if err := cfg.set("curseforge-api-key", "abcd1234efgh"); err != nil {
		t.Fatal(err)
	}
	if err := saveUserConfigAt(path, cfg); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("config file not created: %v", err)
	}

	loaded, err := loadUserConfigAt(path)
	if err != nil {
		t.Fatal(err)
	}
	if loaded.Loader != "fabric" || loaded.CurseForgeAPIKey != "abcd1234efgh" {
		t.Fatalf("loaded config = %#v", loaded)
	}

	entries := loaded.entries(false)
	masked := false
	for _, entry := range entries {
		if entry.Key == "curseforge-api-key" {
			if entry.Value == "abcd1234efgh" {
				t.Fatal("secret not masked in list")
			}
			masked = entry.Value == "****efgh"
		}
	}
	if !masked {
		t.Fatalf("secret not masked as ****efgh: %#v", entries)
	}

	if !loaded.unset("loader") {
		t.Fatal("unset loader reported nothing removed")
	}
	if loaded.Loader != "" {
		t.Fatalf("loader not cleared: %q", loaded.Loader)
	}
	if loaded.unset("loader") {
		t.Fatal("unset on empty key reported removed")
	}
}

func TestConfigCommandRoundTrip(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", dir)

	out, _, err := runCLI(t, "config", "path", "--json")
	if err != nil {
		t.Fatalf("config path failed: %v", err)
	}
	var pathResult struct {
		Path string `json:"path"`
	}
	if err := json.Unmarshal([]byte(out), &pathResult); err != nil {
		t.Fatal(err)
	}
	if pathResult.Path != filepath.Join(dir, configDirName, configFileName) {
		t.Fatalf("config path = %q", pathResult.Path)
	}

	if _, _, err := runCLI(t, "config", "set", "--json", "loader", "fabric"); err != nil {
		t.Fatalf("config set failed: %v", err)
	}
	out, _, err = runCLI(t, "config", "get", "--json", "loader")
	if err != nil {
		t.Fatalf("config get failed: %v", err)
	}
	var got configEntry
	if err := json.Unmarshal([]byte(out), &got); err != nil {
		t.Fatal(err)
	}
	if got.Key != "loader" || got.Value != "fabric" {
		t.Fatalf("config get = %#v", got)
	}

	if _, _, err := runCLI(t, "config", "set", "curseforge-api-key", "supersecret1234"); err != nil {
		t.Fatalf("config set secret failed: %v", err)
	}
	out, _, err = runCLI(t, "config", "list", "--json")
	if err != nil {
		t.Fatalf("config list failed: %v", err)
	}
	var entries []configEntry
	if err := json.Unmarshal([]byte(out), &entries); err != nil {
		t.Fatal(err)
	}
	if len(entries) != 2 {
		t.Fatalf("config list entries = %#v, want 2", entries)
	}
	for _, entry := range entries {
		if entry.Key == "curseforge-api-key" && entry.Value != "****1234" {
			t.Fatalf("secret not masked: %q", entry.Value)
		}
	}

	out, _, err = runCLI(t, "config", "list", "--json", "--show-secrets")
	if err != nil {
		t.Fatalf("config list --show-secrets failed: %v", err)
	}
	if !strings.Contains(out, "supersecret1234") {
		t.Fatalf("show-secrets did not reveal key: %s", out)
	}

	if _, _, err := runCLI(t, "config", "set", "loader", "vanilla"); err == nil {
		t.Fatal("config set accepted an invalid loader")
	}
	if _, _, err := runCLI(t, "config", "get", "nope"); err == nil {
		t.Fatal("config get accepted an unknown key")
	}
	if _, _, err := runCLI(t, "config", "unset", "mc-version"); err == nil {
		t.Fatal("config unset succeeded for an unset key")
	}
}
