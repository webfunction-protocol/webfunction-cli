package cmd

import (
	"crypto/sha256"
	"encoding/hex"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync/atomic"
	"testing"
)

// SHA-256 (hex) of the canonical JSON of each fixture - keys sorted,
// compact, no HTML escaping - computed independently of wfn.
const (
	cleanFixtureHash  = "75479eb278dc3512d06a8d4ee49b15bcd999889d10986b650daf7f6750b08a32"
	flawedFixtureHash = "23de96b832a2e87d9759f5e2707b00c31ba53e6eea4c001abcd9cb427e132489"
)

func hashRun(args []string) error { return (&HashCommand{}).Run(args) }

func dirEntries(t *testing.T, dir string) []string {
	t.Helper()
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	var names []string
	for _, e := range entries {
		names = append(names, e.Name())
	}
	sort.Strings(names)
	return names
}

func writeTemp(t *testing.T, name, content string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), name)
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestHashCommandIsRegistered(t *testing.T) {
	if _, ok := Lookup("hash"); !ok {
		t.Error(`command "hash" is not registered`)
	}
}

// The hash is the SHA-256 of the package document in canonical JSON: keys
// sorted, no whitespace, and no HTML escaping.
func TestPackageHashIsSHA256OfCanonicalJSON(t *testing.T) {
	doc := map[string]any{"b": []any{1.0, true, nil}, "a": "<&>"}
	got, err := packageHash(doc)
	if err != nil {
		t.Fatal(err)
	}
	sum := sha256.Sum256([]byte(`{"a":"<&>","b":[1,true,null]}`))
	if want := hex.EncodeToString(sum[:]); got != want {
		t.Errorf("packageHash = %s, want %s", got, want)
	}
}

func TestHashCommandWritesHashFile(t *testing.T) {
	dir := t.TempDir()
	hashfile := filepath.Join(dir, "package.hash")
	if _, err := runCommand(t, hashRun, "--file", fixturePath("clean.json"), "--hashfile", hashfile); err != nil {
		t.Fatal(err)
	}
	if got, want := readFile(t, hashfile), cleanFixtureHash+"\n"; got != want {
		t.Errorf("hash file = %q, want %q", got, want)
	}
	if got := dirEntries(t, dir); len(got) != 1 {
		t.Errorf("directory holds %v, want only the hash file", got)
	}
}

// Whitespace and key order in the source document don't change the hash;
// the content does.
func TestHashIgnoresLayoutButNotContent(t *testing.T) {
	hashOf := func(doc string) string {
		t.Helper()
		src := writeTemp(t, "package.json", doc)
		hashfile := filepath.Join(t.TempDir(), "h")
		if _, err := runCommand(t, hashRun, "--file", src, "--hashfile", hashfile); err != nil {
			t.Fatal(err)
		}
		return readFile(t, hashfile)
	}
	compact := hashOf(`{"name":"T","base_url":"https://x.test/"}`)
	spread := hashOf("{\n  \"base_url\": \"https://x.test/\",\n  \"name\": \"T\"\n}\n")
	if compact != spread {
		t.Errorf("layout changed the hash: %q vs %q", compact, spread)
	}
	if changed := hashOf(`{"name":"U","base_url":"https://x.test/"}`); changed == compact {
		t.Error("changing the content did not change the hash")
	}
}

func TestHashFromURLMatchesFile(t *testing.T) {
	hashfile := filepath.Join(t.TempDir(), "h")
	url := serveFile(t, fixturePath("clean.json"))
	if _, err := runCommand(t, hashRun, "--url", url, "--hashfile", hashfile); err != nil {
		t.Fatal(err)
	}
	if got, want := readFile(t, hashfile), cleanFixtureHash+"\n"; got != want {
		t.Errorf("hash from --url = %q, want %q", got, want)
	}
}

func TestHashCommandFlagErrors(t *testing.T) {
	clean := fixturePath("clean.json")
	cases := []struct {
		name    string
		args    func(hashfile string) []string
		wantErr string
	}{
		{"no flags", func(h string) []string { return nil }, "--hashfile"},
		{"no --hashfile", func(h string) []string { return []string{"--file", clean} }, "--hashfile"},
		{"no source", func(h string) []string { return []string{"--hashfile", h} }, "--url or --file"},
		{"both sources", func(h string) []string {
			return []string{"--url", "http://127.0.0.1:1/x", "--file", clean, "--hashfile", h}
		}, "cannot be used together"},
		{"missing file", func(h string) []string {
			return []string{"--file", "/nonexistent/package.json", "--hashfile", h}
		}, "reading package file"},
	}
	for _, c := range cases {
		dir := t.TempDir()
		hashfile := filepath.Join(dir, "h")
		_, err := runCommand(t, hashRun, c.args(hashfile)...)
		if err == nil || !strings.Contains(err.Error(), c.wantErr) {
			t.Errorf("%s: error = %v, want one containing %q", c.name, err, c.wantErr)
		}
		if got := dirEntries(t, dir); len(got) != 0 {
			t.Errorf("%s: wrote %v despite the error", c.name, got)
		}
	}
}

// --hashfile on codegen, convert and validate writes the same hash, next
// to the command's usual output.
func TestHashfileOnOtherCommands(t *testing.T) {
	clean := fixturePath("clean.json")
	cases := []struct {
		name    string
		run     func([]string) error
		args    func(out string) []string
		outputs []string // names expected in the directory besides the hash file
	}{
		{"codegen csharp", codegenRun, func(o string) []string {
			return []string{"--target", "csharp", "--url", goldenSourceURL, "--file", clean, "-o", o}
		}, []string{"out"}},
		{"codegen ruby", codegenRun, func(o string) []string {
			return []string{"--target", "ruby", "--url", goldenSourceURL, "--file", clean, "-o", o + ".rb"}
		}, []string{"out.rb", "out.rbs"}},
		{"convert openapi", convertRun, func(o string) []string {
			return []string{"--target", "openapi", "--file", clean, "-o", o}
		}, []string{"out"}},
		{"convert postman", convertRun, func(o string) []string {
			return []string{"--target", "postman", "--file", clean, "-o", o}
		}, []string{"out"}},
		{"validate", validateRun, func(o string) []string {
			return []string{"--file", clean, "-o", o}
		}, []string{"out"}},
	}
	for _, c := range cases {
		dir := t.TempDir()
		hashfile := filepath.Join(dir, "package.hash")
		args := append(c.args(filepath.Join(dir, "out")), "--hashfile", hashfile)
		if _, err := runCommand(t, c.run, args...); err != nil {
			t.Errorf("%s: %v", c.name, err)
			continue
		}
		if got, want := readFile(t, hashfile), cleanFixtureHash+"\n"; got != want {
			t.Errorf("%s: hash file = %q, want %q", c.name, got, want)
		}
		want := append([]string{"package.hash"}, c.outputs...)
		sort.Strings(want)
		if got := dirEntries(t, dir); strings.Join(got, ",") != strings.Join(want, ",") {
			t.Errorf("%s: directory holds %v, want %v", c.name, got, want)
		}

		// Without --hashfile, no hash file appears.
		dir2 := t.TempDir()
		if _, err := runCommand(t, c.run, c.args(filepath.Join(dir2, "out"))...); err != nil {
			t.Errorf("%s without --hashfile: %v", c.name, err)
		}
		for _, name := range dirEntries(t, dir2) {
			if !contains(c.outputs, name) {
				t.Errorf("%s without --hashfile: unexpected file %s", c.name, name)
			}
		}
	}
}

// validate still writes its report when it finds errors, and the hash file
// goes with it, though the command exits with an error.
func TestHashfileWrittenWhenValidateFindsErrors(t *testing.T) {
	dir := t.TempDir()
	hashfile := filepath.Join(dir, "package.hash")
	_, err := runCommand(t, validateRun, "--file", fixturePath("flawed.json"), "-o", filepath.Join(dir, "report.json"), "--hashfile", hashfile)
	if err == nil {
		t.Fatal("expected validate to report errors for flawed.json")
	}
	if got, want := readFile(t, hashfile), flawedFixtureHash+"\n"; got != want {
		t.Errorf("hash file = %q, want %q", got, want)
	}
}

// No hash file is left behind when the command fails to produce its output.
func TestHashfileNotWrittenWhenOutputFails(t *testing.T) {
	clean := fixturePath("clean.json")
	badOut := filepath.Join(t.TempDir(), "no-such-dir", "out")
	cases := []struct {
		name string
		run  func([]string) error
		args []string
	}{
		{"codegen", codegenRun, []string{"--target", "csharp", "--url", goldenSourceURL, "--file", clean, "-o", badOut}},
		{"convert", convertRun, []string{"--target", "openapi", "--file", clean, "-o", badOut}},
		{"validate", validateRun, []string{"--file", clean, "-o", badOut}},
	}
	for _, c := range cases {
		dir := t.TempDir()
		hashfile := filepath.Join(dir, "package.hash")
		if _, err := runCommand(t, c.run, append(c.args, "--hashfile", hashfile)...); err == nil {
			t.Errorf("%s: expected an error writing the output", c.name)
		}
		if got := dirEntries(t, dir); len(got) != 0 {
			t.Errorf("%s: left %v behind", c.name, got)
		}
	}
}

// The hash comes from the same single fetch the command generates from.
func TestHashfileUsesTheSingleFetch(t *testing.T) {
	body := readFile(t, fixturePath("clean.json"))
	var requests int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&requests, 1)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(body))
	}))
	defer srv.Close()

	dir := t.TempDir()
	hashfile := filepath.Join(dir, "package.hash")
	if _, err := runCommand(t, codegenRun, "--target", "csharp", "--url", srv.URL, "-o", filepath.Join(dir, "out"), "--hashfile", hashfile); err != nil {
		t.Fatal(err)
	}
	if n := atomic.LoadInt32(&requests); n != 1 {
		t.Errorf("server saw %d requests, want 1", n)
	}
	if got, want := readFile(t, hashfile), cleanFixtureHash+"\n"; got != want {
		t.Errorf("hash file = %q, want %q", got, want)
	}
}
