package cmd

import (
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// The generated Ruby client must be usable without repeating the --url it
// was generated from, as the Go and Python clients already are: Client
// carries the URL as SOURCE_URL, and Client.new with no real client builds
// one from it. from_package_endpoint(url, ...) stays for callers who want a
// different address, and Client.new(real_client) stays for injection.

func generateRubyFor(t *testing.T, url string) (rb, rbs string) {
	t.Helper()
	oldWarn := warnWriter
	warnWriter = io.Discard
	defer func() { warnWriter = oldWarn }()
	out := filepath.Join(t.TempDir(), "client.rb")
	args := []string{"--target", "ruby", "--url", url, "--file", fixturePath("clean.json"), "-o", out}
	if _, err := runCommand(t, codegenRun, args...); err != nil {
		t.Fatalf("codegen --target ruby failed: %v", err)
	}
	return readFile(t, out), readFile(t, strings.TrimSuffix(out, ".rb")+".rbs")
}

func TestRubyClientBakesSourceURL(t *testing.T) {
	rb, rbs := generateRubyFor(t, goldenSourceURL)
	for _, want := range []string{
		`SOURCE_URL = "` + goldenSourceURL + `"`,
		"def initialize(real_client = nil, bearer_auth: nil, version: nil, pipelined: false)",
		"::WebFunction::Client.from_package_endpoint(SOURCE_URL, bearer_auth: bearer_auth, version: version, pipelined: pipelined)",
		"def self.from_package_endpoint(url, bearer_auth: nil, version: nil, pipelined: false)",
	} {
		if !strings.Contains(rb, want) {
			t.Errorf("generated .rb is missing %q", want)
		}
	}
	for _, want := range []string{
		"SOURCE_URL: String",
		"def initialize: (?untyped real_client, ?bearer_auth: String?, ?version: String?, ?pipelined: bool) -> void",
		"def self.from_package_endpoint: (String url, ?bearer_auth: String?, ?version: String?, ?pipelined: bool) -> Client",
	} {
		if !strings.Contains(rbs, want) {
			t.Errorf("generated .rbs is missing %q", want)
		}
	}
	if strings.Contains(rb, "new_client") || strings.Contains(rbs, "new_client") {
		t.Error("new_client must not be generated: Client.new is the constructor")
	}
}

// A URL is data, not code: Ruby interpolates "#{...}" inside double quotes,
// so the baked-in literal must escape it.
func TestRubySourceURLIsNotInterpolated(t *testing.T) {
	rb, _ := generateRubyFor(t, `https://example.test/p?q=#{1+1}&r="x"`)
	want := `SOURCE_URL = "https://example.test/p?q=\#{1+1}&r=\"x\""`
	if !strings.Contains(rb, want) {
		t.Errorf("generated .rb is missing the escaped literal %q", want)
	}
}

// Loads the generated file with real Ruby, stubs the gem's
// from_package_endpoint, and checks what Client.new does with and without a
// real client. Runs only when WFN_RUBY_LIB points at the webfunction-ruby
// gem's lib directory.
func TestRubyGeneratedClientConstructor(t *testing.T) {
	lib := os.Getenv("WFN_RUBY_LIB")
	if lib == "" {
		t.Skip("set WFN_RUBY_LIB to the webfunction-ruby lib directory to run this test")
	}
	if _, err := exec.LookPath("ruby"); err != nil {
		t.Skip("ruby not installed")
	}
	abs, err := filepath.Abs(lib)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(abs, "web_function.rb")); err != nil {
		t.Fatalf("WFN_RUBY_LIB=%q: no web_function.rb in %s (use an absolute path; a leading ~ is not expanded here): %v", lib, abs, err)
	}
	url := `https://example.test/p?q=#{1+1}&r="x"`
	rb, _ := generateRubyFor(t, url)
	path := filepath.Join(t.TempDir(), "client.rb")
	if err := os.WriteFile(path, []byte(rb), 0o644); err != nil {
		t.Fatal(err)
	}
	script := `
require ARGV[0]
c = WebFunctionClient::Client
puts c::SOURCE_URL
seen = nil
::WebFunction::Client.define_singleton_method(:from_package_endpoint) { |u, **o| seen = [u, o]; :real }
c.new(bearer_auth: "tok", version: "2")
puts seen[0], seen[1][:bearer_auth], seen[1][:version], seen[1][:pipelined]
fake = Object.new
def fake.call(name, args = {})
  "called #{name}"
end
seen = nil
injected = c.new(fake)
puts seen.inspect
puts injected.call("ping")
begin
  c.new(fake, bearer_auth: "x")
rescue ArgumentError => e
  puts e.class
end
`
	got, err := exec.Command("ruby", "-I", abs, "-e", script, path).CombinedOutput()
	if err != nil {
		t.Fatalf("ruby failed: %v\n%s", err, got)
	}
	if want := url + "\n" + url + "\ntok\n2\nfalse\nnil\ncalled ping\nArgumentError\n"; string(got) != want {
		t.Errorf("ruby printed %q, want %q", got, want)
	}
}