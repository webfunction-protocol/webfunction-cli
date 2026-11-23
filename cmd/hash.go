package cmd

import (
	"flag"
	"fmt"
)

func init() {
	Register(&HashCommand{})
}

// HashCommand implements "wfn hash ...".
type HashCommand struct{}

func (c *HashCommand) Name() string { return "hash" }

func (c *HashCommand) Summary() string {
	return "Write a hash of a webfunction package's content to a file"
}

func (c *HashCommand) Usage() string {
	return `wfn hash (--url <url> | --file <path>) --hashfile <file>

Fetches a webfunction package (or reads one from --file) and writes a hash of
its content to a plain text file. Nothing else is done.

The hash is the SHA-256 of the package document in canonical JSON (keys
sorted, no insignificant whitespace), written as 64 lowercase hex digits
followed by a newline. The layout of the source document (whitespace, key
order) therefore doesn't affect it, but any change to its content does.

Flags:
  --url        URL of the webfunction package to hash (--url or --file is required)
  --file       Path to a local webfunction package JSON file to hash instead of --url
  --hashfile   File to write the hash to (required)

The codegen, convert and validate commands also accept --hashfile, and write
the hash of the very package they worked on, so no second fetch is needed.

Example:
  wfn hash --url https://api.reservepay.com/merchants --hashfile merchants.hash
  wfn hash --file ./package.json --hashfile package.hash`
}

func (c *HashCommand) Run(args []string) error {
	fs := flag.NewFlagSet(c.Name(), flag.ContinueOnError)

	url := fs.String("url", "", "URL of the webfunction package")
	file := fs.String("file", "", "path to a webfunction package JSON file (instead of --url)")
	hashfile := fs.String("hashfile", "", "file to write the hash of the package content to")

	if err := fs.Parse(args); err != nil {
		return err
	}

	var missing []string
	sourceMissing, err := packageSourceFlags(*url, *file)
	if err != nil {
		return fmt.Errorf("%w\n\n%s", err, c.Usage())
	}
	if sourceMissing != "" {
		missing = append(missing, sourceMissing)
	}
	if *hashfile == "" {
		missing = append(missing, "--hashfile")
	}
	if len(missing) > 0 {
		return fmt.Errorf("missing required flag(s): %v\n\n%s", missing, c.Usage())
	}

	loaded, err := loadPackage(*url, *file)
	if err != nil {
		return err
	}
	pkg := loaded.pkg

	name := pkg.Name
	if name == "" {
		name = "(unnamed package)"
	}
	fmt.Printf("%s %s (%d endpoint(s)) from %s\n", loaded.verb, name, len(pkg.Endpoints), loaded.from)

	return loaded.writeHashFile(*hashfile)
}
