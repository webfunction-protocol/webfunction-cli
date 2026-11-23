package cmd

import (
	"flag"
	"fmt"
	"os"

	"wfn/validate"
)

func init() {
	Register(&ValidateCommand{})
}

// ValidateCommand implements "wfn validate ...".
type ValidateCommand struct{}

func (c *ValidateCommand) Name() string { return "validate" }

func (c *ValidateCommand) Summary() string {
	return "Validate the structure of a webfunction package"
}

func (c *ValidateCommand) Usage() string {
	return `wfn validate (--url <url> | --file <path>) -o <file> [--format json|markdown|html]

Fetches a webfunction package (or reads one from --file) and checks its
structure for dangling references, inconsistent flags/versions, and other
known wire-format footguns, writing a report to -o.

Flags:
  --url        URL of the webfunction package to validate (--url or --file is required)
  --file       Path to a local webfunction package JSON file to validate instead of --url
  -o           Output file to write the report to (required)
  --format     Report format: json, markdown, or html (default: json)
  --hashfile   Also write a hash of the package content to this file (see "wfn help hash")

Exit status: non-zero if the report contains any error-severity finding
(useful for CI), even though the report file is still written either way.
Fetch/write failures also exit non-zero, same as codegen/convert.

Example:
  wfn validate --url https://api.reservepay.com/merchants -o report.json
  wfn validate --url https://api.reservepay.com/merchants -o report.html --format html
  wfn validate --file ./package.json -o report.json`
}

func (c *ValidateCommand) Run(args []string) error {
	fs := flag.NewFlagSet(c.Name(), flag.ContinueOnError)

	url := fs.String("url", "", "URL of the webfunction package")
	file := fs.String("file", "", "path to a webfunction package JSON file (instead of --url)")
	output := fs.String("o", "", "output file name")
	format := fs.String("format", "json", "report format ("+fmt.Sprint(validate.ValidFormats)+")")
	hashfile := fs.String("hashfile", "", "write a hash of the package content to this file")

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
	if *output == "" {
		missing = append(missing, "-o")
	}
	if len(missing) > 0 {
		return fmt.Errorf("missing required flag(s): %v\n\n%s", missing, c.Usage())
	}

	if !isValidFormat(*format) {
		return fmt.Errorf("invalid --format %q, must be one of: %v", *format, validate.ValidFormats)
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

	report := validate.Run(pkg, loaded.from)

	rendered, err := report.Render(*format)
	if err != nil {
		return fmt.Errorf("rendering report: %w", err)
	}

	if err := os.WriteFile(*output, []byte(rendered), 0o644); err != nil {
		return fmt.Errorf("writing %s: %w", *output, err)
	}
	fmt.Printf("Wrote %s\n", *output)
	if err := loaded.writeHashFile(*hashfile); err != nil {
		return err
	}
	fmt.Printf("%d error(s), %d warning(s), %d info note(s)\n", report.ErrorCount, report.WarningCount, report.InfoCount)

	if report.Failed() {
		return fmt.Errorf("validation found %d error(s) - see %s", report.ErrorCount, *output)
	}
	return nil
}

func isValidFormat(format string) bool {
	for _, f := range validate.ValidFormats {
		if f == format {
			return true
		}
	}
	return false
}
