package cmd

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// Package docs are markdown and may contain backslashes (Windows paths,
// regexes, escapes). pythongen puts them inside non-raw triple-quoted
// docstrings, so each backslash must be escaped: otherwise "\d" and "\s"
// warn (SyntaxError under -W error), "\n", "\t" and "\x41" silently become
// a newline, a tab and "A", and a line ending in a backslash joins the next
// line to itself.

func pythonDocsFixture() string {
	return filepath.Join("..", "testdata", "codegen", "pythondocs", "backslashes.json")
}

func TestPythonDocsEscapeBackslashes(t *testing.T) {
	src := generateCode(t, "python", pythonDocsFixture(), false)[".py"]

	for _, want := range []string{
		`Saves to C:\\new\\table and matches \\d+ or \\s.`,
		`Backslash then triple quotes: \\\"\"\" end.`,
		`Id like \\d{4} and a tab \\t marker.`,
		`code "BAD_PATH" - Bad path C:\\temp and \\x41 more.`,
	} {
		if !strings.Contains(src, want) {
			t.Errorf("generated python does not contain %q", want)
		}
	}

	found := false
	for _, l := range strings.Split(src, "\n") {
		if strings.Contains(l, "A line ending in a backslash") {
			found = true
			if got := strings.TrimRight(l, "\r"); !strings.HasSuffix(got, `backslash\\`) {
				t.Errorf("a docs line ending in a backslash must end in an escaped backslash, got %q", got)
			}
		}
	}
	if !found {
		t.Error("the line ending in a backslash was not found in the output")
	}
}

// The generated module must compile without any warning, and its docstrings
// must read back as the original text, which a text comparison can't prove.
func TestPythonDocsRoundTripThroughPython(t *testing.T) {
	py, err := exec.LookPath("python3")
	if err != nil {
		t.Skip("python3 not available")
	}
	src := generateCode(t, "python", pythonDocsFixture(), false)[".py"]
	file := filepath.Join(t.TempDir(), "client.py")
	if err := os.WriteFile(file, []byte(src), 0o644); err != nil {
		t.Fatal(err)
	}
	const script = `import ast, json, sys
with open(sys.argv[1], encoding="utf-8") as f:
    src = f.read()
tree = compile(src, sys.argv[1], "exec", flags=ast.PyCF_ONLY_AST)
docs = []
for n in ast.walk(tree):
    if isinstance(n, ast.Expr) and isinstance(n.value, ast.Constant) and isinstance(n.value.value, str):
        docs.append(n.value.value)
print(json.dumps(docs))
`
	out, err := exec.Command(py, "-W", "error", "-c", script, file).CombinedOutput()
	if err != nil {
		t.Fatalf("generated python does not compile cleanly: %v\n%s", err, out)
	}
	var docs []string
	if err := json.Unmarshal(out, &docs); err != nil {
		t.Fatalf("cannot read docstrings back: %v\n%s", err, out)
	}
	// Docstring continuation lines are indented; compare them without that.
	var norm []string
	for _, l := range strings.Split(strings.Join(docs, "\n"), "\n") {
		norm = append(norm, strings.TrimSpace(l))
	}
	all := strings.Join(norm, "\n")
	for _, want := range []string{
		`Saves to C:\new\table and matches \d+ or \s.`,
		"A line ending in a backslash\\\nNext line.",
		`Backslash then triple quotes: \""" end.`,
		`Id like \d{4} and a tab \t marker.`,
		`code "BAD_PATH" - Bad path C:\temp and \x41 more.`,
	} {
		if !strings.Contains(all, want) {
			t.Errorf("docstrings do not read back as the original text, missing %q", want)
		}
	}
}