# webfunction-cli

`wfn` is a command line tool for working with [webfunction](https://webfunction.org) packages.

It's designed to grow a small set of subcommands over time, each focused on
one task.

## Installation

Clone and build locally:

```sh
git clone https://github.com/webfunction-protocol/webfunction-cli.git
cd webfunction-cli
go build -o wfn .
```

## Usage

```sh
wfn                 # show general help
wfn help            # same as above
wfn help <command>  # show detailed help for a specific command
wfn --version       # print the version and exit
wfn <command> [arguments]
```

`--version` can appear anywhere on the command line; when it does, wfn prints its version and does nothing else. The version is set in `cmd/version.go`, and a release build can override it with `go build -ldflags "-X wfn/cmd.Version=1.2.3" -o wfn .`

## Commands

| Command   | Description                                              |
|-----------|------------------------------------------------------------|
| `help`    | Show general help, or help for a specific command           |
| `codegen` | Generate a typed client for a webfunction package (targets: java, go, php, js, csharp, ruby, python; `--private` to include private endpoints, arguments, and attributes; `--file` to read the package from a local JSON file, though `--url` is still required; `--namespace` to set the namespace, package or module of the generated client); generics are expanded into concrete types |
| `convert` | Convert a webfunction package to another document format (targets: openapi, postman; `--url` or `--file` for the package; `--private` to include private endpoints, arguments, and attributes; generics are expanded into concrete types) |
| `validate` | Check a webfunction package's structure, flags, and generics usage, writing a report (formats: json, markdown, html; `--url` or `--file` for the package) |
| `hash` | Write a hash of a webfunction package's content to a file, and do nothing else (`--url` or `--file` for the package; `--hashfile` for the file to write, required) |

More commands will be added over time.

## Where the package comes from

`convert`, `validate` and `hash` take exactly one of:

- `--url <url>`: fetch the package from its package endpoint.
- `--file <path>`: read a local JSON file holding the same document the
  endpoint returns. Handy for testing and for packages that aren't deployed yet.

Giving both, or neither, is an error.

`codegen` always needs `--url`, because the generated client fetches the
package from that address at runtime. There, `--file` is optional and only
replaces the fetch at generation time, so you can generate a client for a live
URL from a local (possibly edited) package.

With `--file`, the status line reads `Loaded ... from file:///...` instead of
`Fetched ... from <url>`, and `validate` records that `file://` URL as the
report's `package_url`.

```sh
wfn convert --target openapi --file ./package.json -o openapi.json
wfn validate --file ./package.json -o report.html --format html
wfn codegen --target go --url https://example.com/pkg --file ./package.json -o client.go
```

### Namespace

`codegen --namespace <name>` sets the namespace, package or module the
generated client lives in. It defaults to `WebFunctionClient`. Each target maps
it onto its own convention, and `js` and `python` don't use it:

| Target   | `--namespace` becomes                                              | Example                                       |
|----------|--------------------------------------------------------------------|-----------------------------------------------|
| `php`    | a PHP namespace: backslash-separated, each segment a valid identifier (otherwise `codegen` fails) | `Acme\Billing` gives `namespace Acme\Billing;` |
| `go`     | a Go package name: lowercased, punctuation removed                  | `Acme.Billing_API` gives `package acmebillingapi` |
| `java`   | a Java package: dot-separated, lowercased, punctuation removed      | `Acme.Billing_API` gives `package acme.billingapi;` |
| `csharp` | a C# namespace: dot-separated, each segment PascalCased             | `Acme.Billing_API` gives `namespace Acme.BillingApi;` |
| `ruby`   | a Ruby constant path: `::`-separated, each name starting with a capital letter (otherwise `codegen` fails); each name becomes a nested `module` | `Acme::Billing` gives `module Acme` containing `module Billing` |

```sh
wfn codegen --target csharp --url https://example.com/pkg --namespace Acme.Billing -o Client.cs
```

## Package hash

`codegen`, `convert` and `validate` all accept `--hashfile <file>`, which writes
a hash of the package content to that plain text file once the command has
written its usual output. `wfn hash (--url <url> | --file <path>) --hashfile <file>`
does only that: it writes the hash and nothing else (`--hashfile` is required).

```sh
wfn codegen --target go --url https://api.reservepay.com/merchants -o merchant.go --hashfile merchants.hash
wfn hash --url https://api.reservepay.com/merchants --hashfile merchants.hash
```

The hash is taken from the very document the command worked on, so there is no
second fetch that could return something different. It is the SHA-256 of the
package document in canonical JSON (keys sorted, no insignificant whitespace,
no HTML escaping), written as 64 lowercase hex digits followed by a newline.
Whitespace and key order in the source therefore don't affect it, but any
change to the content does. If the command fails before producing its output,
no hash file is written; `validate` writes it whenever it writes its report,
even when the report contains errors.

## Generics

A package can define single-parameter generic objects: an object definition
that uses the bare type `T`, applied with a single-key object such as
`{ "object.list": "object.user" }`. `{ "array": "object.user" }` is an array of
a type, the same as `[["object.user"]]`. An application can appear anywhere a
type is expected, including inside object definitions. `T` is only in scope
inside the object definition that uses it, and a generic object must not apply
itself, directly or through other generic objects (see the
[generics spec](https://webfunction.org/generics)). Support in this tool:

- `validate` checks how generics are used: unapplied generics, malformed
  applications, `T` used outside an object definition, generic objects that
  apply themselves, name collisions, and so on.
- `convert` expands each application into a concrete type before converting.
  For OpenAPI, `{ "object.list": "object.user" }` becomes a `ListOfUser`
  component. Output for packages that don't use generics is unchanged.
- `codegen` expands generics the same way, for every target, so each
  application is generated as an ordinary type and the generic definition
  itself is not emitted. `{ "object.list": "object.user" }` becomes
  `ListOfUser` (in JavaScript, a `ListOfUserAttributes` typedef). Generated
  names join the pieces with `Of` and `Or`:
  - `{ "object.list": ["object.user", "null"] }` is `ListOfUserOrNull`
  - `{ "object.page": { "array": "object.user" } }` is `PageOfArrayOfUser`

  If a generated name clashes with an object you defined, the generated one
  gets a number (`ListOfUser2`) and `wfn validate` reports a
  `generic-name-collision`. Output for packages that don't use generics is
  unchanged.
- `convert` and `codegen` also print a `warning: generics: ...` line on stderr
  for each generics problem the validator finds, then carry on, resolving the
  problem to `any`. They never fail because of them; run `wfn validate` for
  the full report.

## Project layout

```
.
├── main.go             # entry point: parses args, dispatches to a command
├── cmd/
│   ├── command.go      # Command interface + registry
│   ├── help.go         # the "help" command
│   ├── codegen.go      # the "codegen" command
│   ├── convert.go      # the "convert" command
│   ├── generics_warning.go # generics warnings printed by convert and codegen
│   ├── hash.go         # the "hash" command
│   ├── package_source.go   # loads a package from --url or --file, and hashes it, for all commands
│   └── validate.go     # the "validate" command
├── csharpgen/          # codegen --target csharp
├── gogen/              # codegen --target go
├── javagen/            # codegen --target java
├── jsgen/              # codegen --target js
├── phpgen/             # codegen --target php
├── pythongen/          # codegen --target python
├── rubygen/            # codegen --target ruby
├── generics/           # expands generic objects into concrete objects (used by validate, convert and codegen)
├── openapiconvert/     # convert --target openapi's package-to-OpenAPI-3.1 conversion
├── postmanconvert/     # convert --target postman's package-to-Postman-collection conversion
├── privatefilter/      # strips private endpoints/arguments/attributes before convert/codegen
├── validate/           # the checks and report rendering behind the "validate" command
└── testdata/           # fixture packages for the validate, convert and codegen tests, plus golden converter and codegen output
```

Each command implements a small `Command` interface (`Name`, `Summary`,
`Usage`, `Run`) and registers itself in an `init()` function. Adding a new
command means adding a new file under `cmd/` — no other files need to change.

## Testing

```sh
go test ./...
```

Each converter's output for every fixture package is pinned byte for byte by
golden files in `testdata/convert/golden/`. After an intended change to
converter output, regenerate them and review the diff:

```sh
go test ./openapiconvert ./postmanconvert -run TestGoldenOutput -update
```

See `testdata/convert/README.md` for details.

## License

MIT — see [LICENSE](LICENSE).
