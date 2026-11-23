package cmd

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strings"

	"github.com/webfunction-protocol/webfunction-go"
)

// loadedPackage is a package together with where it came from.
type loadedPackage struct {
	pkg  *webfunction.Package
	hash string // see packageHash; of the very document pkg was built from
	from string // where it came from, as a URL: the endpoint, or a file:// URL
	verb string // "Fetched" or "Loaded", for the status line
}

// loadPackage returns the package a command works on: read from file when
// it is set, otherwise fetched from endpointURL.
func loadPackage(endpointURL, file string) (*loadedPackage, error) {
	if file != "" {
		pkg, hash, from, err := readPackageFile(file)
		if err != nil {
			return nil, err
		}
		return &loadedPackage{pkg: pkg, hash: hash, from: from, verb: "Loaded"}, nil
	}

	// Invoke the endpoint the way webfunction.FromPackageEndpoint does, but
	// keep the decoded document, so the package and its hash both come from
	// the one fetch.
	doc, err := webfunction.Execute(endpointURL, webfunction.ExecuteOptions{})
	if err != nil {
		return nil, fmt.Errorf("fetching package: %w", err)
	}
	b, err := json.Marshal(doc)
	if err != nil {
		return nil, fmt.Errorf("fetching package: parsing package from %s: %w", endpointURL, err)
	}
	var pkg webfunction.Package
	if err := json.Unmarshal(b, &pkg); err != nil {
		return nil, fmt.Errorf("fetching package: parsing package from %s: %w", endpointURL, err)
	}
	hash, err := packageHash(doc)
	if err != nil {
		return nil, fmt.Errorf("fetching package: hashing package from %s: %w", endpointURL, err)
	}
	return &loadedPackage{pkg: &pkg, hash: hash, from: endpointURL, verb: "Fetched"}, nil
}

// packageHash returns the hash of a package document, as decoded from JSON
// (a map[string]any): the SHA-256, in lowercase hex, of its canonical JSON.
// Canonical means keys sorted, no insignificant whitespace and no HTML
// escaping, so the layout of the source document doesn't matter but any
// change to its content does. It hashes the whole document rather than the
// parsed webfunction.Package, so the hash doesn't depend on which fields
// this version of webfunction-go knows about.
func packageHash(doc any) (string, error) {
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	if err := enc.Encode(doc); err != nil {
		return "", err
	}
	sum := sha256.Sum256(bytes.TrimSuffix(buf.Bytes(), []byte("\n")))
	return hex.EncodeToString(sum[:]), nil
}

// writeHashFile writes the package's hash, followed by a newline, to path.
// It does nothing when path is empty (no --hashfile was given).
func (l *loadedPackage) writeHashFile(path string) error {
	if path == "" {
		return nil
	}
	if err := os.WriteFile(path, []byte(l.hash+"\n"), 0o644); err != nil {
		return fmt.Errorf("writing %s: %w", path, err)
	}
	fmt.Printf("Wrote %s\n", path)
	return nil
}

// loadPackageFile reads a package from a JSON file, returning it with the
// file's absolute file:// URL. The file holds the same document a package
// endpoint returns.
func loadPackageFile(path string) (*webfunction.Package, string, error) {
	pkg, _, from, err := readPackageFile(path)
	return pkg, from, err
}

// readPackageFile is loadPackageFile, plus the package's hash (see
// packageHash), taken from the same read of the file.
func readPackageFile(path string) (pkg *webfunction.Package, hash, from string, err error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, "", "", fmt.Errorf("reading package file: %w", err)
	}
	var p webfunction.Package
	if err := json.Unmarshal(raw, &p); err != nil {
		return nil, "", "", fmt.Errorf("parsing package file %s: %w", path, err)
	}
	var doc any
	if err := json.Unmarshal(raw, &doc); err != nil {
		return nil, "", "", fmt.Errorf("parsing package file %s: %w", path, err)
	}
	hash, err = packageHash(doc)
	if err != nil {
		return nil, "", "", fmt.Errorf("hashing package file %s: %w", path, err)
	}
	abs, err := filepath.Abs(path)
	if err != nil {
		abs = path
	}
	return &p, hash, fileURL(abs), nil
}

// fileURL returns the file:// URL for an absolute path. Windows drive
// paths (C:\dir\pkg.json) become file:///C:/dir/pkg.json.
func fileURL(path string) string {
	p := filepath.ToSlash(path)
	if !strings.HasPrefix(p, "/") {
		p = "/" + p
	}
	return (&url.URL{Scheme: "file", Path: p}).String()
}

// packageSourceFlags checks --url and --file for commands that accept
// exactly one of them (convert, validate). It returns the flag name to list
// as missing when neither was given, or an error when both were.
func packageSourceFlags(endpointURL, file string) (missing string, err error) {
	switch {
	case endpointURL != "" && file != "":
		return "", errors.New("--url and --file cannot be used together")
	case endpointURL == "" && file == "":
		return "--url or --file", nil
	}
	return "", nil
}
