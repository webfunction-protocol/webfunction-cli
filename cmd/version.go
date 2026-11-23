package cmd

// Version is the version of wfn. Release builds can override it at build
// time with: go build -ldflags "-X wfn/cmd.Version=1.2.3"
var Version = "0.1.0"

// WantsVersion reports whether args include the --version option, in any
// position. When it does, wfn prints Version and does nothing else.
func WantsVersion(args []string) bool {
	for _, a := range args {
		if a == "--version" {
			return true
		}
	}
	return false
}
