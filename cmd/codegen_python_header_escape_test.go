package cmd

import (
	"encoding/json"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// The package name comes from the package and the source URL from the
// command line; pythongen writes both into "#" comment lines and into
// non-raw triple-quoted docstrings. A newline would end a comment and turn
// the rest of the name into code, and a backslash or `"""` would corrupt or
// close a docstring. Both must be rendered on one line, escaped.

const (
	// the name in hostile-name.json decodes to: Pay """ \d<newline>import os #
	hostileSourceURL = "https://example.com/p?x=\\d\"\"\"\ny"
)

func generateWithURL(t *testing.T, url string) string {
	t.Helper()
	oldWarn := warnWriter
	warnWriter = io.Discard
	defer func() { warnWriter = oldWarn }()
	out := filepath.Join(t.TempDir(), "client.py")
	fixture := filepath.Join("..", "testdata", "codegen", "pythondocs", "hostile-name.json")
	if _, err := runCommand(t, codegenRun, "--target", "python", "--url", url, "--file", fixture, "-o", out); err != nil {
		t.Fatalf("codegen failed: %v", err)
	}
	return readFile(t, out)
}

func TestPythonHeaderKeepsNameAndURLOnOneLine(t *testing.T) {
	src := generateWithURL(t, hostileSourceURL)

	for _, want := range []string{
		`# Package: Pay """ \d import os #`,
		`# Source:  https://example.com/p?x=\d""" y`,
	} {
		found := false
		for _, l := range strings.Split(src, "\n") {
			if strings.TrimRight(l, "\r") == want {
				found = true
			}
		}
		if !found {
			t.Errorf("no header comment line equal to %q", want)
		}
	}
	for _, l := range strings.Split(src, "\n") {
		if strings.HasPrefix(strings.TrimSpace(l), "import os") {
			t.Errorf("package name leaked a line of code into the module: %q", l)
		}
	}
}

func TestPythonHeaderRoundTripThroughPython(t *testing.T) {
	py, err := exec.LookPath("python3")
	if err != nil {
		t.Skip("python3 not available")
	}
	src := generateWithURL(t, hostileSourceURL)
	file := filepath.Join(t.TempDir(), "client.py")
	if err := os.WriteFile(file, []byte(src), 0o644); err != nil {
		t.Fatal(err)
	}
	const script = `import ast, json, sys
with open(sys.argv[1], encoding="utf-8") as f:
    src = f.read()
tree = compile(src, sys.argv[1], "exec", flags=ast.PyCF_ONLY_AST)
docs, urls = [], []
for n in ast.walk(tree):
    if isinstance(n, ast.Expr) and isinstance(n.value, ast.Constant) and isinstance(n.value.value, str):
        docs.append(n.value.value)
    if isinstance(n, ast.Assign) and len(n.targets) == 1 and isinstance(n.targets[0], ast.Name) and n.targets[0].id == "_SOURCE_URL" and isinstance(n.value, ast.Constant):
        urls.append(n.value.value)
print(json.dumps({"docs": docs, "urls": urls}))
`
	out, err := exec.Command(py, "-W", "error", "-c", script, file).CombinedOutput()
	if err != nil {
		t.Fatalf("generated python does not compile cleanly: %v\n%s", err, out)
	}
	var got struct {
		Docs []string `json:"docs"`
		URLs []string `json:"urls"`
	}
	if err := json.Unmarshal(out, &got); err != nil {
		t.Fatalf("cannot read the module back: %v\n%s", err, out)
	}
	all := strings.Join(got.Docs, "\n")
	for _, want := range []string{
		`Generated, typed client for the Pay """ \d import os # webfunction package.`,
		`Generated, typed async client for the Pay """ \d import os # webfunction package.`,
		`Source: https://example.com/p?x=\d""" y`,
	} {
		if !strings.Contains(all, want) {
			t.Errorf("docstrings do not read back as the original text, missing %q", want)
		}
	}
	// _SOURCE_URL is what the client really connects to: it must be exact.
	if len(got.URLs) == 0 {
		t.Fatal("no _SOURCE_URL assignment found")
	}
	for _, u := range got.URLs {
		if u != hostileSourceURL {
			t.Errorf("_SOURCE_URL = %q, want %q", u, hostileSourceURL)
		}
	}
}