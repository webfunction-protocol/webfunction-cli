package cmd

import (
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"wfn/rubygen"
)

// rubyOutput runs codegen for ruby with the given --namespace and returns
// both generated files, or the command's error.
func rubyOutput(t *testing.T, namespace string, private bool) (rb, rbs string, outPath string, err error) {
	t.Helper()
	oldWarn := warnWriter
	warnWriter = io.Discard
	defer func() { warnWriter = oldWarn }()

	outPath = filepath.Join(t.TempDir(), "client.rb")
	args := []string{"--target", "ruby", "--url", goldenSourceURL, "--file", genericsFixture("generic.json"), "-o", outPath, "--namespace", namespace}
	if private {
		args = append(args, "--private")
	}
	if _, err = runCommand(t, codegenRun, args...); err != nil {
		return "", "", outPath, err
	}
	rbBytes, readErr := os.ReadFile(outPath)
	rbsBytes, rbsErr := os.ReadFile(strings.TrimSuffix(outPath, ".rb") + ".rbs")
	if readErr != nil || rbsErr != nil {
		t.Fatalf("reading the generated files: %v / %v", readErr, rbsErr)
	}
	return string(rbBytes), string(rbsBytes), outPath, nil
}

// indentNonEmpty indents every non-empty line by n spaces.
func indentNonEmpty(s string, n int) string {
	lines := strings.Split(s, "\n")
	for i, l := range lines {
		if l != "" {
			lines[i] = strings.Repeat(" ", n) + l
		}
	}
	return strings.Join(lines, "\n")
}

// nestRuby rebuilds what a file generated for the single module Billing
// must look like when generated for the nested modules segments: the same
// preamble, one module line per segment, the body shifted in by one level
// per extra module, and an end per module.
func nestRuby(t *testing.T, single string, segments []string) string {
	t.Helper()
	const open = "module Billing\n"
	at := strings.Index(single, open)
	if at < 0 || !strings.HasSuffix(single, "end\n") {
		t.Fatalf("the single-module output has an unexpected shape:\n%s", single)
	}
	preamble := single[:at]
	body := strings.TrimSuffix(single[at+len(open):], "end\n")

	var b strings.Builder
	b.WriteString(preamble)
	for i, seg := range segments {
		b.WriteString(strings.Repeat("  ", i) + "module " + seg + "\n")
	}
	b.WriteString(indentNonEmpty(body, 2*(len(segments)-1)))
	for i := len(segments) - 1; i >= 0; i-- {
		b.WriteString(strings.Repeat("  ", i) + "end\n")
	}
	return b.String()
}

// Nested namespaces become nested modules. The compact form (module
// Acme::Billing) raises NameError at load time unless Acme already exists.
func TestRubyNamespaceGeneratesNestedModules(t *testing.T) {
	for _, private := range []bool{false, true} {
		single, singleRbs, _, err := rubyOutput(t, "Billing", private)
		if err != nil {
			t.Fatal(err)
		}
		for _, namespace := range []string{"Acme::Billing", "Acme::Corp::Billing"} {
			segments := strings.Split(namespace, "::")
			// The single-module output is for "Billing", so only the last
			// segment is "Billing": rebuild each expectation from it.
			rb, rbs, _, err := rubyOutput(t, namespace, private)
			if err != nil {
				t.Fatalf("%s: %v", namespace, err)
			}
			if want := nestRuby(t, single, segments); rb != want {
				t.Errorf("%s (private=%v): the .rb file is not the nested form.\n%s", namespace, private, firstDifference(rb, want))
			}
			if want := nestRuby(t, singleRbs, segments); rbs != want {
				t.Errorf("%s (private=%v): the .rbs file is not the nested form.\n%s", namespace, private, firstDifference(rbs, want))
			}
			if strings.Contains(rb, "module Acme::") || strings.Contains(rbs, "module Acme::") {
				t.Errorf("%s: the compact module form must not be emitted", namespace)
			}
		}
	}
}

func TestRubyDefaultNamespaceIsUnchanged(t *testing.T) {
	rb, rbs, _, err := rubyOutput(t, "", false)
	if err != nil {
		t.Fatal(err)
	}
	for name, text := range map[string]string{".rb": rb, ".rbs": rbs} {
		if !strings.Contains(text, "\nmodule WebFunctionClient\n") || !strings.HasSuffix(text, "\nend\n") {
			t.Errorf("the default module should be WebFunctionClient in the %s file", name)
		}
	}
}

func TestRubyNamespaceAcceptsConstantNames(t *testing.T) {
	for _, namespace := range []string{"Acme", "Acme2", "A_b", "Acme::Billing_v2", "A::B::C::D"} {
		if _, _, _, err := rubyOutput(t, namespace, false); err != nil {
			t.Errorf("%s should be accepted: %v", namespace, err)
		}
	}
}

// Anything that is not a chain of Ruby constants would generate a file
// that does not load, so it is refused before anything is written.
func TestRubyNamespaceRejectsWhatIsNotAConstantPath(t *testing.T) {
	cases := []string{
		"acme",            // constants start with a capital
		"Acme::billing",   // every segment, not just the first
		"Acme::",          // empty trailing segment
		"::Acme",          // leading :: (top-level scope) is not a module name
		"Acme::::Billing", // empty segment
		"Acme::9Bad",
		"Acme.Billing",
		"Acme\\Billing", // the PHP separator
		"Acme Billing",
		"Acme:Billing",
		"Acme-Billing",
		"Billing_API.v2",
		"BEGIN", // keywords
		"Acme::END",
	}
	for _, namespace := range cases {
		_, _, outPath, err := rubyOutput(t, namespace, false)
		if err == nil {
			t.Errorf("%q should be rejected", namespace)
			continue
		}
		if !strings.Contains(err.Error(), "invalid namespace segment") || !strings.Contains(err.Error(), strconv.Quote(namespace)) {
			t.Errorf("%q: the error should name the problem and the value, got: %v", namespace, err)
		}
		for _, path := range []string{outPath, strings.TrimSuffix(outPath, ".rb") + ".rbs"} {
			if _, statErr := os.Stat(path); statErr == nil {
				t.Errorf("%q: %s must not be written when the namespace is invalid", namespace, filepath.Base(path))
			}
		}
	}
}

func TestRubygenGenerateRefusesAnInvalidModuleName(t *testing.T) {
	pkg := parsePackageJSON(t, readFile(t, genericsFixture("generic.json")))
	rb, rbs, err := rubygen.Generate(pkg, goldenSourceURL, "acme", false)
	if err == nil || rb != "" || rbs != "" {
		t.Errorf("an invalid module name should return an error and no output, got err=%v, %d + %d bytes", err, len(rb), len(rbs))
	}
}