package cmd

import (
	"path/filepath"
	"strings"
	"testing"
)

// A package's error descriptions are markdown and are often hard-wrapped
// across several lines. Every target must keep the whole description. The
// js, php, go and java targets continue it on the following comment lines;
// python, csharp and ruby render one line per error, so they must join the
// description into that one line rather than keep only its first source line.

func errorDocsFixture() string {
	return filepath.Join("..", "testdata", "codegen", "errordocs", "errors.json")
}

func linesContaining(src, marker string) []string {
	var out []string
	for _, l := range strings.Split(src, "\n") {
		if strings.Contains(l, marker) {
			out = append(out, l)
		}
	}
	return out
}

func TestErrorDocsKeepFullTextInEveryTarget(t *testing.T) {
	wants := []string{
		"belongs to a different merchant.",
		"Second paragraph, should it appear?",
		"interp.",
	}
	for _, target := range validTargets {
		t.Run(target, func(t *testing.T) {
			var all strings.Builder
			for _, src := range generateCode(t, target, errorDocsFixture(), false) {
				all.WriteString(src)
			}
			for _, want := range wants {
				if !strings.Contains(all.String(), want) {
					t.Errorf("%s output is missing %q from an error description", target, want)
				}
			}
		})
	}
}

func TestErrorDocsOnOneLineWhereOneLinePerError(t *testing.T) {
	const notFound = "- This error occurs when no payment with the provided ID exists in your account, or the payment belongs to a different merchant."
	const twoParas = "- First paragraph line one first paragraph line two. Second paragraph, should it appear?"
	const single = "- One line only."

	cases := []struct {
		target  string
		marker  func(code string) string
		special string
	}{
		{"python", func(c string) string { return `code "` + c + `"` },
			"- Contains a */ closer, a <tag> & amp, a `backtick`, \"quotes\", a \\\"\\\"\\\" triple, and a #{ruby} interp."},
		{"csharp", func(c string) string { return `code "` + c + `"` },
			"- Contains a */ closer, a &lt;tag&gt; &amp; amp, a `backtick`, \"quotes\", a \"\"\" triple, and a #{ruby} interp."},
		{"ruby", func(c string) string { return "code " + c + " -" },
			"- Contains a */ closer, a <tag> & amp, a `backtick`, \"quotes\", a \"\"\" triple, and a #{ruby} interp."},
	}
	for _, c := range cases {
		t.Run(c.target, func(t *testing.T) {
			var all strings.Builder
			for _, src := range generateCode(t, c.target, errorDocsFixture(), false) {
				all.WriteString(src)
			}
			for code, want := range map[string]string{
				"NOT_FOUND": notFound,
				"TWO_PARAS": twoParas,
				"SPECIAL":   c.special,
				"SINGLE":    single,
			} {
				lines := linesContaining(all.String(), c.marker(code))
				if len(lines) == 0 {
					t.Errorf("%s: no line found for error %s", c.target, code)
				}
				for _, l := range lines {
					if !strings.Contains(l, want) {
						t.Errorf("%s: error %s line does not hold its whole description\nwant it to contain: %s\ngot: %s", c.target, code, want, strings.TrimSpace(l))
					}
				}
			}
		})
	}
}