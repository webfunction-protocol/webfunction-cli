package cmd

import (
	"fmt"
	"sort"
)

func init() {
	Register(&HelpCommand{})
}

// HelpCommand implements "wfn help" and "wfn help <command>".
type HelpCommand struct{}

func (h *HelpCommand) Name() string { return "help" }

func (h *HelpCommand) Summary() string {
	return "Show general help, or help for a specific command"
}

func (h *HelpCommand) Usage() string {
	return `wfn help [command]

With no argument, prints general usage and lists all available commands.
With a command name, prints detailed usage for that command.

Examples:
  wfn help
  wfn help codegen`
}

func (h *HelpCommand) Run(args []string) error {
	if len(args) == 0 {
		printGeneralHelp()
		return nil
	}

	name := args[0]
	c, ok := Lookup(name)
	if !ok {
		fmt.Printf("wfn: unknown command %q\n\n", name)
		printGeneralHelp()
		return nil
	}

	fmt.Println(c.Usage())
	return nil
}

// printGeneralHelp prints the top-level usage banner plus a sorted list of
// every registered command and its one-line summary.
func printGeneralHelp() {
	fmt.Println(`wfn - a command line tool for working with webfunction packages

Usage:
  wfn <command> [arguments]
  wfn help              Show this help
  wfn help <command>     Show detailed help for a command
  wfn --version         Show the version

Available commands:`)

	names := make([]string, 0, len(All()))
	for name := range All() {
		names = append(names, name)
	}
	sort.Strings(names)

	for _, name := range names {
		c := registry[name]
		fmt.Printf("  %-10s %s\n", c.Name(), c.Summary())
	}

	fmt.Println(`
Run "wfn help <command>" for more information about a command.`)
}
