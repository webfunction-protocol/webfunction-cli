package cmd

import (
	"bytes"
	"os/exec"
	"path/filepath"
	"regexp"
	"testing"
)

func TestWantsVersion(t *testing.T) {
	cases := []struct {
		args []string
		want bool
	}{
		{nil, false},
		{[]string{"--version"}, true},
		{[]string{"codegen", "--version"}, true},
		{[]string{"codegen", "--target", "go", "--version"}, true},
		{[]string{"--version", "codegen", "--target", "go"}, true},
		{[]string{"--versions"}, false},
		{[]string{"-version"}, false},
		{[]string{"version"}, false},
		{[]string{"codegen", "--target", "version"}, false},
	}
	for _, c := range cases {
		if got := WantsVersion(c.args); got != c.want {
			t.Errorf("WantsVersion(%q) = %v, want %v", c.args, got, c.want)
		}
	}
}

func TestVersionLooksLikeSemver(t *testing.T) {
	if !regexp.MustCompile(`^\d+\.\d+\.\d+$`).MatchString(Version) {
		t.Errorf("Version = %q, want MAJOR.MINOR.PATCH", Version)
	}
}

// buildWfn builds the real wfn binary, optionally overriding Version with
// -ldflags, and returns its path.
func buildWfn(t *testing.T, ldflags string) string {
	t.Helper()
	bin := filepath.Join(t.TempDir(), "wfn")
	args := []string{"build", "-o", bin}
	if ldflags != "" {
		args = append(args, "-ldflags", ldflags)
	}
	args = append(args, ".")
	build := exec.Command("go", args...)
	build.Dir = ".."
	if out, err := build.CombinedOutput(); err != nil {
		t.Fatalf("go build: %v\n%s", err, out)
	}
	return bin
}

// With --version present, the only thing that happens is the version being
// printed: nothing else on stdout or stderr, exit status 0, and the rest of
// the command line is never looked at (no command lookup, no file read, no
// network request).
func TestBinaryVersionFlagOnlyPrintsVersion(t *testing.T) {
	cases := [][]string{
		{"--version"},
		{"help", "--version"},
		{"nosuchcommand", "--version"},
		{"codegen", "--version"},
		{"codegen", "--target", "go", "--url", "http://127.0.0.1:1/never", "--version"},
		{"codegen", "--file", "/nonexistent/package.json", "--version"},
		{"--version", "validate", "--file", "/nonexistent/package.json"},
	}
	for _, build := range []struct{ name, ldflags, want string }{
		{"default", "", Version},
		{"ldflags override", "-X wfn/cmd.Version=9.8.7", "9.8.7"},
	} {
		bin := buildWfn(t, build.ldflags)
		for _, args := range cases {
			var stdout, stderr bytes.Buffer
			c := exec.Command(bin, args...)
			c.Stdout, c.Stderr = &stdout, &stderr
			if err := c.Run(); err != nil {
				t.Errorf("%s: wfn %q: %v", build.name, args, err)
			}
			if got, want := stdout.String(), build.want+"\n"; got != want {
				t.Errorf("%s: wfn %q: stdout = %q, want %q", build.name, args, got, want)
			}
			if stderr.Len() != 0 {
				t.Errorf("%s: wfn %q: stderr = %q, want empty", build.name, args, stderr.String())
			}
		}
	}
}
