package cli

import (
	"errors"
	"fmt"
	"strings"

	"wt/internal/config"
	"wt/internal/git"
)

const aliasUsage = "usage: wt alias [add <name> <command...> | rm <name> | list]"

// alias manages user-defined command aliases, stored git-style as
// `git config wt.alias.<name>` so they also work when set by hand. The add
// and rm forms write the global config — aliases are a habit, not a project
// property; a per-repo override is still possible with plain git config.
func alias(dir string, args []string) error {
	switch {
	case len(args) == 0 || args[0] == "list" && len(args) == 1:
		return aliasList(dir)
	case args[0] == "add":
		return aliasAdd(dir, args[1:])
	case args[0] == "rm" && len(args) == 2:
		return aliasRemove(dir, args[1])
	default:
		return errors.New(aliasUsage)
	}
}

func aliasList(dir string) error {
	pairs := config.Aliases(dir)
	if len(pairs) == 0 {
		logf("no aliases — try: wt alias add cr create -c")
		return nil
	}
	width := 0
	for _, p := range pairs {
		if len(p[0]) > width {
			width = len(p[0])
		}
	}
	for _, p := range pairs {
		fmt.Fprintf(stdout, "%s = %s\n", pad(p[0], width), p[1])
	}
	return nil
}

func aliasAdd(dir string, args []string) error {
	if len(args) < 2 {
		return errors.New(aliasUsage)
	}
	name := args[0]
	if !validAliasName(name) {
		return fmt.Errorf("invalid alias name '%s' — letters, digits and dashes only", name)
	}
	if reservedName(name) {
		return fmt.Errorf("'%s' is a wt command", name)
	}
	words := strings.Fields(strings.Join(args[1:], " "))
	if len(words) == 0 || !builtinNames[words[0]] {
		return hintf("wt help lists the commands",
			"alias must expand to a wt command, not %q", strings.Join(args[1:], " "))
	}
	expansion := strings.Join(words, " ")
	if _, err := git.Run(dir, "config", "--global", "wt.alias."+name, expansion); err != nil {
		return err
	}
	logf("alias '%s' = '%s' (global git config, wt.alias.%s)", name, expansion, name)
	return nil
}

func aliasRemove(dir, name string) error {
	if _, err := git.Run(dir, "config", "--global", "--unset", "wt.alias."+name); err != nil {
		if len(config.Alias(dir, name)) > 0 {
			return hintf("remove it with: git config --unset wt.alias."+name,
				"alias '%s' is set in this repo's local git config", name)
		}
		return hintf("wt alias lists them", "no alias named '%s'", name)
	}
	logf("alias '%s' removed", name)
	return nil
}

// validAliasName mirrors git's config key rules: the name becomes the last
// segment of wt.alias.<name>.
func validAliasName(name string) bool {
	if name == "" || !isLetter(rune(name[0])) {
		return false
	}
	for _, r := range name {
		if !isLetter(r) && (r < '0' || r > '9') && r != '-' {
			return false
		}
	}
	return true
}

func isLetter(r rune) bool {
	return r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z'
}

func reservedName(name string) bool {
	return builtinNames[name] || name == "help" || name == "complete"
}

// jumpAlias is wrapper plumbing: for a first word missing from the wrapper's
// static case lists, the shell function asks whether stdout must be captured
// and eval'd. It answers for user aliases and for jump builtins added after
// the wrapper was eval'd, so new commands work without a re-source. Prints
// "1" for a jump; silent otherwise.
func jumpAlias(dir string, args []string) error {
	if len(args) == 0 {
		return nil
	}
	var full []string
	if reservedName(args[0]) {
		// a builtin outranks any same-named alias
		full = append([]string{args[0]}, args[1:]...)
	} else if full = config.Alias(dir, args[0]); len(full) == 0 {
		return nil
	} else {
		full = append(full, args[1:]...)
	}
	jump := jumpCommands[full[0]]
	switch {
	case full[0] == "root" && len(full) > 1:
		switch full[1] {
		case "checkout", "ch", "create", "fork":
			jump = true
		}
	case full[0] == "global" && len(full) > 1:
		jump = full[1] == "checkout" || full[1] == "ch"
	}
	if jump {
		fmt.Fprintln(stdout, "1")
	}
	return nil
}
