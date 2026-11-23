package cmd

import (
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// csharpNamespaceLine runs codegen for csharp with the given --namespace
// (none when namespace is empty) and returns the "namespace X;" line of
// the generated file.
func csharpNamespaceLine(t *testing.T, namespace string) string {
	t.Helper()
	oldWarn := warnWriter
	warnWriter = io.Discard
	defer func() { warnWriter = oldWarn }()

	out := filepath.Join(t.TempDir(), "Client.cs")
	args := []string{"--target", "csharp", "--url", goldenSourceURL, "--file", fixturePath("clean.json"), "-o", out}
	if namespace != "" {
		args = append(args, "--namespace", namespace)
	}
	if _, err := runCommand(t, codegenRun, args...); err != nil {
		t.Fatalf("namespace %q: %v", namespace, err)
	}
	src, err := os.ReadFile(out)
	if err != nil {
		t.Fatal(err)
	}
	for _, line := range strings.Split(string(src), "\n") {
		if strings.HasPrefix(line, "namespace ") {
			return strings.TrimSuffix(strings.TrimSpace(line), ";")
		}
	}
	t.Fatalf("namespace %q: no namespace line in the output", namespace)
	return ""
}

// A namespace segment written entirely in capitals is an acronym the
// caller chose on purpose (Reservepay.SDK), so it is kept as written
// instead of being re-cased to Sdk.
func TestCSharpNamespaceKeepsAllCapsSegments(t *testing.T) {
	cases := []struct{ namespace, want string }{
		{"SDK", "namespace SDK"},
		{"Reservepay.SDK", "namespace Reservepay.SDK"},
		{"SDK.Reservepay", "namespace SDK.Reservepay"},
		{"Acme.API2", "namespace Acme.API2"},
	}
	for _, c := range cases {
		if got := csharpNamespaceLine(t, c.namespace); got != c.want {
			t.Errorf("--namespace %q: got %q, want %q", c.namespace, got, c.want)
		}
	}
}

// Guards: everything that is not a single all-caps word keeps being
// PascalCased exactly as before.
func TestCSharpNamespaceOtherSegmentsUnchanged(t *testing.T) {
	cases := []struct{ namespace, want string }{
		{"", "namespace WebFunctionClient"},
		{"Reservepay.Merchant", "namespace Reservepay.Merchant"},
		{"sdk", "namespace Sdk"},
		{"reservepay.sdk", "namespace Reservepay.Sdk"},
		{"my-company_thing", "namespace MyCompanyThing"},
		{"SDK_CLIENT", "namespace SdkClient"},
		{"SDK-client", "namespace SdkClient"},
		{"MySDK", "namespace MySdk"},
		{"123", "namespace _123"},
	}
	for _, c := range cases {
		if got := csharpNamespaceLine(t, c.namespace); got != c.want {
			t.Errorf("--namespace %q: got %q, want %q", c.namespace, got, c.want)
		}
	}
}
