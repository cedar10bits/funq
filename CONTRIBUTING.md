# Contributing

Issues and pull requests are welcome. funq is maintained by one person, so
response times vary.

## Pull requests

- Open an issue first for anything beyond a bug fix or doc change, so the API
  direction can be agreed before you write code.
- CI must pass: `gofumpt` (formatting), `go vet`, `golangci-lint`, and
  `go test -race ./...`. Run these locally before pushing.
- Add or update examples (`example_test.go`) and tests for any behaviour
  change. Public API changes need a `CHANGELOG.md` entry.
- Keep the public surface small. A new operation has to earn its place against
  "just write the loop".

## Building

Requires Go 1.27 or later. There are no dependencies beyond the standard
library.
