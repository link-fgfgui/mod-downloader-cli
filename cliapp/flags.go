package cliapp

import (
	"flag"
	"fmt"
	"io"
	"strings"

	"github.com/urfave/cli/v2"
)

type trailingFlag struct {
	name     string
	value    string
	isBool   bool
	explicit bool
}

// enableTrailingFlags makes every command tolerate flags that follow positional
// arguments. Go's flag parser stops at the first non-flag token, so without this
// `remove iris --dry-run` would drop --dry-run and delete the file for real.
func enableTrailingFlags(cmd *cli.Command) {
	if cmd.Action != nil {
		action, name := cmd.Action, cmd.Name
		cmd.Action = func(c *cli.Context) error {
			return runActionAllowingTrailingFlags(c, name, action)
		}
	}
	for _, sub := range cmd.Subcommands {
		enableTrailingFlags(sub)
	}
}

func runActionAllowingTrailingFlags(c *cli.Context, name string, action cli.ActionFunc) error {
	bools := boolFlagNames(c)
	flags, positional, wantHelp, err := splitTrailingArgs(c.Args().Slice(), bools)
	if err != nil {
		return err
	}
	if wantHelp {
		return cli.ShowCommandHelp(c, name)
	}
	if len(flags) == 0 {
		return action(c)
	}

	rebuilt, err := reparseTrailingFlags(c, name, flags, positional)
	if err != nil {
		return err
	}
	return action(rebuilt)
}

// splitTrailingArgs separates positional arguments from flag tokens, reporting
// each flag with the metadata needed to re-emit it for Go's flag parser.
func splitTrailingArgs(tokens []string, bools map[string]bool) ([]trailingFlag, []string, bool, error) {
	flags := make([]trailingFlag, 0)
	positional := make([]string, 0, len(tokens))
	for i := 0; i < len(tokens); i++ {
		token := tokens[i]
		if token == "--" {
			positional = append(positional, tokens[i+1:]...)
			break
		}
		if !isFlagToken(token) {
			positional = append(positional, token)
			continue
		}

		name := strings.TrimLeft(token, "-")
		value, hasValue := "", false
		if idx := strings.Index(name, "="); idx >= 0 {
			name, value, hasValue = name[:idx], name[idx+1:], true
		}
		if name == "" {
			return nil, nil, false, fmt.Errorf("bad flag syntax: %s", token)
		}
		if name == "help" || name == "h" {
			return nil, nil, true, nil
		}

		isBool := bools[name]
		switch {
		case hasValue:
			if isBool {
				if err := validateBool(name, value); err != nil {
					return nil, nil, false, err
				}
			}

		case isBool:
			value = "true"
		default:
			if i+1 >= len(tokens) {
				return nil, nil, false, fmt.Errorf("flag needs an argument: -%s", name)
			}
			i++
			value = tokens[i]
		}
		flags = append(flags, trailingFlag{name: name, value: value, isBool: isBool, explicit: hasValue})
	}
	return flags, positional, false, nil
}

func validateBool(name, value string) error {
	switch strings.ToLower(value) {
	case "1", "t", "true", "yes", "y":
		return nil
	case "0", "f", "false", "no", "n":
		return nil
	default:
		return fmt.Errorf("invalid boolean value %q for -%s", value, name)
	}
}

func isFlagToken(token string) bool {
	return len(token) > 1 && strings.HasPrefix(token, "-") && token != "-"
}

// commandFlags lists every flag reachable from c, command-local first.
func commandFlags(c *cli.Context) []cli.Flag {
	flags := make([]cli.Flag, 0)
	if c != nil && c.Command != nil {
		flags = append(flags, c.Command.Flags...)
	}
	if c != nil && c.App != nil {
		flags = append(flags, c.App.Flags...)
	}
	return flags
}

// boolFlagNames resolves which reachable flag names are boolean switches.
func boolFlagNames(c *cli.Context) map[string]bool {
	names := make(map[string]bool)
	for _, f := range commandFlags(c) {
		probe := flag.NewFlagSet("probe", flag.ContinueOnError)
		if err := f.Apply(probe); err != nil {
			continue
		}
		probe.VisitAll(func(fl *flag.Flag) {
			if v, ok := fl.Value.(interface{ IsBoolFlag() bool }); ok && v.IsBoolFlag() {
				names[fl.Name] = true
			}
		})
	}
	return names
}

// reparseTrailingFlags rebuilds the command context so flags written after
// positional arguments take effect. Values parsed from earlier flags are carried
// over, and the remaining positional arguments are exposed again.
func reparseTrailingFlags(c *cli.Context, name string, flags []trailingFlag, positional []string) (*cli.Context, error) {
	set := flag.NewFlagSet(name, flag.ContinueOnError)
	set.SetOutput(io.Discard)
	set.Usage = func() {}
	if err := applyUniqueFlags(set, commandFlags(c)); err != nil {
		return nil, err
	}

	for _, flagName := range c.FlagNames() {
		if set.Lookup(flagName) == nil {
			continue
		}
		if err := set.Set(flagName, fmt.Sprint(c.Value(flagName))); err != nil {
			return nil, fmt.Errorf("invalid value for flag -%s", flagName)
		}
	}

	tokens := make([]string, 0, len(flags)*2+len(positional)+1)
	for _, f := range flags {
		switch {
		case f.isBool && !f.explicit:
			tokens = append(tokens, "-"+f.name)
		case f.isBool:
			tokens = append(tokens, "-"+f.name+"="+f.value)
		default:
			tokens = append(tokens, "-"+f.name, f.value)
		}
	}
	tokens = append(tokens, "--")
	tokens = append(tokens, positional...)
	if err := set.Parse(tokens); err != nil {
		return nil, err
	}
	return cli.NewContext(c.App, set, c), nil
}

func applyUniqueFlags(set *flag.FlagSet, flags []cli.Flag) error {
	seen := make(map[string]bool)
	set.VisitAll(func(f *flag.Flag) { seen[f.Name] = true })
	for _, f := range flags {
		probe := flag.NewFlagSet("probe", flag.ContinueOnError)
		if err := f.Apply(probe); err != nil {
			return err
		}
		conflict := false
		probe.VisitAll(func(fl *flag.Flag) {
			if seen[fl.Name] {
				conflict = true
			}
		})
		if conflict {
			continue
		}
		if err := f.Apply(set); err != nil {
			return err
		}
		probe.VisitAll(func(fl *flag.Flag) { seen[fl.Name] = true })
	}
	return nil
}
