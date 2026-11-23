package cmd

import (
	"flag"
	"fmt"
	"os"

	"wfn/openapiconvert"
	"wfn/postmanconvert"
)

func init() {
	Register(&ConvertCommand{})
}

// validConvertTargets is the set of formats convert currently knows how
// to convert a webfunction package to. Keep in sync with the generator
// switch in Run.
var validConvertTargets = []string{"openapi", "postman"}

// ConvertCommand implements "wfn convert ...".
type ConvertCommand struct{}

func (c *ConvertCommand) Name() string { return "convert" }

func (c *ConvertCommand) Summary() string {
	return "Convert a webfunction package to another document format"
}

func (c *ConvertCommand) Usage() string {
	return `wfn convert --target <format> (--url <url> | --file <path>) -o <file>

Converts a webfunction package definition into another document format.

Flags:
  --target     Output format. One of: ` + fmt.Sprint(validConvertTargets) + ` (required)
  --url        URL of the webfunction package to convert (--url or --file is required)
  --file       Path to a local webfunction package JSON file to convert instead of --url
  -o           Output file to write the converted document to (required)
  --private    Include endpoints, arguments, and attributes flagged private (default: excluded)
  --hashfile   Also write a hash of the package content to this file (see "wfn help hash")

Example:
  wfn convert --target openapi --url https://api.reservepay.com/merchants -o converted.json
  wfn convert --target postman --url https://api.reservepay.com/merchants -o merchants.postman_collection.json --private
  wfn convert --target openapi --file ./package.json -o converted.json`
}

func (c *ConvertCommand) Run(args []string) error {
	fs := flag.NewFlagSet(c.Name(), flag.ContinueOnError)

	target := fs.String("target", "", "output format ("+fmt.Sprint(validConvertTargets)+")")
	url := fs.String("url", "", "URL of the webfunction package")
	file := fs.String("file", "", "path to a webfunction package JSON file (instead of --url)")
	output := fs.String("o", "", "output file name")
	private := fs.Bool("private", false, "include endpoints, arguments, and attributes flagged private in the output")
	hashfile := fs.String("hashfile", "", "write a hash of the package content to this file")

	if err := fs.Parse(args); err != nil {
		return err
	}

	var missing []string
	if *target == "" {
		missing = append(missing, "--target")
	}
	sourceMissing, err := packageSourceFlags(*url, *file)
	if err != nil {
		return fmt.Errorf("%w\n\n%s", err, c.Usage())
	}
	if sourceMissing != "" {
		missing = append(missing, sourceMissing)
	}
	if *output == "" {
		missing = append(missing, "-o")
	}
	if len(missing) > 0 {
		return fmt.Errorf("missing required flag(s): %v\n\n%s", missing, c.Usage())
	}

	if !isValidConvertTarget(*target) {
		return fmt.Errorf("invalid --target %q, must be one of: %v", *target, validConvertTargets)
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
	if *private {
		fmt.Println("Including private endpoints, arguments, and attributes (--private)")
	}

	warnGenerics(warnWriter, pkg, loaded.from)

	var out string
	switch *target {
	case "openapi":
		out, err = openapiconvert.Generate(pkg, *private)
		if err != nil {
			return fmt.Errorf("converting to openapi: %w", err)
		}
	case "postman":
		out, err = postmanconvert.Generate(pkg, *private)
		if err != nil {
			return fmt.Errorf("converting to postman: %w", err)
		}
	default:
		// isValidConvertTarget already restricts *target to
		// validConvertTargets, so this is unreachable in practice - kept
		// as a safety net.
		return fmt.Errorf("wfn convert: target %q not yet implemented", *target)
	}

	if err := os.WriteFile(*output, []byte(out), 0o644); err != nil {
		return fmt.Errorf("writing %s: %w", *output, err)
	}
	fmt.Printf("Wrote %s\n", *output)
	return loaded.writeHashFile(*hashfile)
}

func isValidConvertTarget(target string) bool {
	for _, t := range validConvertTargets {
		if t == target {
			return true
		}
	}
	return false
}
