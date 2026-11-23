package cmd

import (
	"io"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// The generated Java header already says the file must be saved as exactly
// "Client.java". It must also say where: in a directory matching the
// package, because Maven, Gradle and IDEs expect that layout. The directory
// shown is derived from the final package name (after the namespace has been
// lowercased and cleaned), so it always matches the "package ...;" line.

func generateJavaHeader(t *testing.T, namespace string) (header, source string) {
	t.Helper()
	oldWarn := warnWriter
	warnWriter = io.Discard
	defer func() { warnWriter = oldWarn }()
	out := filepath.Join(t.TempDir(), "Client.java")
	args := []string{"--target", "java", "--url", goldenSourceURL, "--file", fixturePath("clean.json"), "-o", out}
	if namespace != "" {
		args = append(args, "--namespace", namespace)
	}
	if _, err := runCommand(t, codegenRun, args...); err != nil {
		t.Fatalf("codegen --target java failed: %v", err)
	}
	source = readFile(t, out)
	i := strings.Index(source, "\npackage ")
	if i < 0 {
		t.Fatal("generated Java has no package line")
	}
	return source[:i], source
}

func TestJavaHeaderSaysWhereToSaveTheFile(t *testing.T) {
	cases := []struct{ namespace, dir string }{
		{"", "webfunctionclient"},
		{"jon.things.merchant", "jon/things/merchant"},
		{"Acme.Billing_API", "acme/billingapi"},
	}
	for _, c := range cases {
		header, source := generateJavaHeader(t, c.namespace)
		want := "<source root>/" + c.dir + "/Client.java"
		if !strings.Contains(header, want) {
			t.Errorf("namespace %q: header should show %q, got:\n%s", c.namespace, want, header)
		}
		for _, also := range []string{"directory matching its package", "src/main/java"} {
			if !strings.Contains(header, also) {
				t.Errorf("namespace %q: header should mention %q", c.namespace, also)
			}
		}
		// The existing file-name rule must stay.
		if !strings.Contains(header, `exactly "Client.java"`) {
			t.Errorf("namespace %q: header lost the exact file name rule", c.namespace)
		}
		// The directory shown must match the real package line.
		m := regexp.MustCompile(`(?m)^package ([A-Za-z0-9_.]+);$`).FindStringSubmatch(source)
		if m == nil {
			t.Fatalf("namespace %q: no package line found", c.namespace)
		}
		if got := strings.ReplaceAll(m[1], ".", "/"); got != c.dir {
			t.Errorf("namespace %q: package is %q, so the directory should be %q, test expects %q", c.namespace, m[1], got, c.dir)
		}
	}
}