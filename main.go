package main

import (
	"fmt"
	"os"

	"wfn/cmd"
)

func main() {
	args := os.Args[1:]

	// --version anywhere on the command line: print the version and nothing else.
	if cmd.WantsVersion(args) {
		fmt.Println(cmd.Version)
		return
	}

	// No command given: show general help.
	if len(args) == 0 {
		help, _ := cmd.Lookup("help")
		help.Run(nil)
		return
	}

	name := args[0]
	rest := args[1:]

	c, ok := cmd.Lookup(name)
	if !ok {
		fmt.Fprintf(os.Stderr, "wfn: unknown command %q\n\n", name)
		help, _ := cmd.Lookup("help")
		help.Run(nil)
		os.Exit(1)
	}

	if err := c.Run(rest); err != nil {
		fmt.Fprintf(os.Stderr, "wfn: %s: %s\n", name, err)
		os.Exit(1)
	}
}
