package cliapp

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/urfave/cli/v2"
)

const (
	configDirName  = "mod-downloader-cli"
	configFileName = "config.json"
)

// userConfig is the persistent CLI configuration. It is always stored under the
// operating system's standard user configuration directory and never in the
// current working directory.
type userConfig struct {
	MinecraftVersion string `json:"mc-version,omitempty"`
	Loader           string `json:"loader,omitempty"`
	CurseForgeAPIKey string `json:"curseforge-api-key,omitempty"`
	ModrinthAPIKey   string `json:"modrinth-api-key,omitempty"`
	CacheDir         string `json:"cache-dir,omitempty"`
	MinecraftDir     string `json:"minecraft-dir,omitempty"`
}

type configKeySpec struct {
	name     string
	secret   bool
	validate func(string) error
}

var configKeySpecs = []configKeySpec{
	{name: "mc-version"},
	{name: "loader", validate: validateLoaderValue},
	{name: "curseforge-api-key", secret: true},
	{name: "modrinth-api-key", secret: true},
	{name: "cache-dir"},
	{name: "minecraft-dir"},
}

func configKeySpecFor(key string) (configKeySpec, bool) {
	key = strings.ToLower(strings.TrimSpace(key))
	for _, spec := range configKeySpecs {
		if spec.name == key {
			return spec, true
		}
	}
	return configKeySpec{}, false
}

func validateLoaderValue(value string) error {
	if !validLoader(value) {
		return fmt.Errorf("invalid loader %q: expected fabric, forge, or neoforge", strings.TrimSpace(value))
	}
	return nil
}

// userConfigPath returns the OS-standard configuration file path.
// Linux: ~/.config/mod-downloader-cli/config.json
// macOS: ~/Library/Application Support/mod-downloader-cli/config.json
// Windows: %APPDATA%\mod-downloader-cli\config.json
func userConfigPath() (string, error) {
	base, err := os.UserConfigDir()
	if err != nil {
		return "", fmt.Errorf("resolve user config dir: %w", err)
	}
	return filepath.Join(base, configDirName, configFileName), nil
}

func loadUserConfig() (userConfig, error) {
	path, err := userConfigPath()
	if err != nil {
		return userConfig{}, err
	}
	return loadUserConfigAt(path)
}

func loadUserConfigAt(path string) (userConfig, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return userConfig{}, nil
		}
		return userConfig{}, err
	}
	var cfg userConfig
	if err := json.Unmarshal(data, &cfg); err != nil {
		return userConfig{}, fmt.Errorf("parse config %s: %w", path, err)
	}
	return cfg, nil
}

func saveUserConfig(cfg userConfig) error {
	path, err := userConfigPath()
	if err != nil {
		return err
	}
	return saveUserConfigAt(path, cfg)
}

func saveUserConfigAt(path string, cfg userConfig) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')
	return os.WriteFile(path, data, 0o600)
}

func (cfg userConfig) get(key string) (string, bool) {
	switch strings.ToLower(strings.TrimSpace(key)) {
	case "mc-version":
		return cfg.MinecraftVersion, cfg.MinecraftVersion != ""
	case "loader":
		return cfg.Loader, cfg.Loader != ""
	case "curseforge-api-key":
		return cfg.CurseForgeAPIKey, cfg.CurseForgeAPIKey != ""
	case "modrinth-api-key":
		return cfg.ModrinthAPIKey, cfg.ModrinthAPIKey != ""
	case "cache-dir":
		return cfg.CacheDir, cfg.CacheDir != ""
	case "minecraft-dir":
		return cfg.MinecraftDir, cfg.MinecraftDir != ""
	default:
		return "", false
	}
}

func (cfg *userConfig) set(key, value string) error {
	key = strings.ToLower(strings.TrimSpace(key))
	value = strings.TrimSpace(value)
	spec, ok := configKeySpecFor(key)
	if !ok {
		return fmt.Errorf("unknown config key: %s", key)
	}
	if spec.validate != nil {
		if err := spec.validate(value); err != nil {
			return err
		}
	}
	switch key {
	case "mc-version":
		cfg.MinecraftVersion = value
	case "loader":
		cfg.Loader = normalizeLoader(value)
	case "curseforge-api-key":
		cfg.CurseForgeAPIKey = value
	case "modrinth-api-key":
		cfg.ModrinthAPIKey = value
	case "cache-dir":
		cfg.CacheDir = value
	case "minecraft-dir":
		cfg.MinecraftDir = value
	}
	return nil
}

func (cfg *userConfig) unset(key string) bool {
	key = strings.ToLower(strings.TrimSpace(key))
	_, present := cfg.get(key)
	switch key {
	case "mc-version":
		cfg.MinecraftVersion = ""
	case "loader":
		cfg.Loader = ""
	case "curseforge-api-key":
		cfg.CurseForgeAPIKey = ""
	case "modrinth-api-key":
		cfg.ModrinthAPIKey = ""
	case "cache-dir":
		cfg.CacheDir = ""
	case "minecraft-dir":
		cfg.MinecraftDir = ""
	default:
		return false
	}
	return present
}

type configEntry struct {
	Key   string `json:"key"`
	Value string `json:"value"`
}

func (cfg userConfig) entries(showSecrets bool) []configEntry {
	entries := make([]configEntry, 0, len(configKeySpecs))
	for _, spec := range configKeySpecs {
		value, _ := cfg.get(spec.name)
		if value == "" {
			continue
		}
		if spec.secret && !showSecrets {
			value = maskSecret(value)
		}
		entries = append(entries, configEntry{Key: spec.name, Value: value})
	}
	sort.Slice(entries, func(i, j int) bool { return entries[i].Key < entries[j].Key })
	return entries
}

func maskSecret(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return ""
	}
	if len(value) <= 4 {
		return "****"
	}
	return "****" + value[len(value)-4:]
}

func (r runner) configCommand() *cli.Command {
	secretFlag := func() cli.Flag {
		return &cli.BoolFlag{Name: "show-secrets", Usage: "print secret values (API keys) in plain text"}
	}
	return &cli.Command{
		Name:  "config",
		Usage: "Manage persistent user configuration",
		Subcommands: []*cli.Command{
			{
				Name:      "get",
				Usage:     "Print the value of a config key",
				ArgsUsage: "<key>",
				Flags:     []cli.Flag{jsonFlag(), secretFlag()},
				Action:    r.configGetAction,
			},
			{
				Name:      "set",
				Usage:     "Set a config key to a value",
				ArgsUsage: "<key> <value>",
				Flags:     []cli.Flag{jsonFlag()},
				Action:    r.configSetAction,
			},
			{
				Name:      "unset",
				Usage:     "Remove a config key",
				ArgsUsage: "<key>",
				Flags:     []cli.Flag{jsonFlag()},
				Action:    r.configUnsetAction,
			},
			{
				Name:   "list",
				Usage:  "List all configured keys and values",
				Flags:  []cli.Flag{jsonFlag(), secretFlag()},
				Action: r.configListAction,
			},
			{
				Name:   "path",
				Usage:  "Print the configuration file path",
				Flags:  []cli.Flag{jsonFlag()},
				Action: r.configPathAction,
			},
		},
	}
}

func (r runner) configGetAction(c *cli.Context) error {
	key := strings.TrimSpace(c.Args().First())
	if key == "" {
		return cli.Exit("config key is required", 1)
	}
	spec, ok := configKeySpecFor(key)
	if !ok {
		return cli.Exit(fmt.Sprintf("unknown config key: %s", key), 1)
	}
	cfg, err := loadUserConfig()
	if err != nil {
		return err
	}
	value, present := cfg.get(spec.name)
	if !present {
		return cli.Exit(fmt.Sprintf("config key is not set: %s", spec.name), 1)
	}
	if spec.secret && !c.Bool("show-secrets") {
		value = maskSecret(value)
	}
	return r.write(c, configEntry{Key: spec.name, Value: value}, writeConfigEntry)
}

func (r runner) configSetAction(c *cli.Context) error {
	args := c.Args().Slice()
	if len(args) < 2 {
		return cli.Exit("config set requires <key> <value>", 1)
	}
	key := strings.TrimSpace(args[0])
	value := strings.Join(args[1:], " ")
	cfg, err := loadUserConfig()
	if err != nil {
		return err
	}
	if err := cfg.set(key, value); err != nil {
		return cli.Exit(err.Error(), 1)
	}
	if err := saveUserConfig(cfg); err != nil {
		return err
	}
	stored, _ := cfg.get(key)
	return r.write(c, configEntry{Key: normalizeConfigKeyName(key), Value: stored}, writeConfigEntry)
}

func (r runner) configUnsetAction(c *cli.Context) error {
	key := strings.TrimSpace(c.Args().First())
	if key == "" {
		return cli.Exit("config key is required", 1)
	}
	if _, ok := configKeySpecFor(key); !ok {
		return cli.Exit(fmt.Sprintf("unknown config key: %s", key), 1)
	}
	cfg, err := loadUserConfig()
	if err != nil {
		return err
	}
	removed := cfg.unset(key)
	if !removed {
		return cli.Exit(fmt.Sprintf("config key is not set: %s", normalizeConfigKeyName(key)), 1)
	}
	if err := saveUserConfig(cfg); err != nil {
		return err
	}
	result := map[string]any{"key": normalizeConfigKeyName(key), "removed": true}
	return r.write(c, result, writeConfigUnset)
}

func (r runner) configListAction(c *cli.Context) error {
	cfg, err := loadUserConfig()
	if err != nil {
		return err
	}
	entries := cfg.entries(c.Bool("show-secrets"))
	return r.write(c, entries, writeConfigList)
}

func (r runner) configPathAction(c *cli.Context) error {
	path, err := userConfigPath()
	if err != nil {
		return err
	}
	result := map[string]any{"path": path}
	return r.write(c, result, writeConfigPath)
}

func normalizeConfigKeyName(key string) string {
	return strings.ToLower(strings.TrimSpace(key))
}

func writeConfigEntry(w io.Writer, value any) error {
	entry := value.(configEntry)
	fmt.Fprintf(w, "%s=%s\n", entry.Key, entry.Value)
	return nil
}

func writeConfigList(w io.Writer, value any) error {
	entries := value.([]configEntry)
	for _, entry := range entries {
		fmt.Fprintf(w, "%s=%s\n", entry.Key, entry.Value)
	}
	return nil
}

func writeConfigUnset(w io.Writer, value any) error {
	result := value.(map[string]any)
	fmt.Fprintf(w, "unset %s\n", result["key"].(string))
	return nil
}

func writeConfigPath(w io.Writer, value any) error {
	result := value.(map[string]any)
	fmt.Fprintln(w, result["path"].(string))
	return nil
}
