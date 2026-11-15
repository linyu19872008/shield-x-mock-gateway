# Contributing

Thanks for helping improve mock-gateway.

## Development

```bash
go test ./...
go vet ./...
go build ./...
make conformance
make docs-check
```

Use English commit messages. Keep changes focused and include tests for behavior
changes. By submitting a contribution, you agree that it is licensed under the
MIT license used by this repository.

## Scope

mock-gateway is a testing and simulation tool. It must not proxy requests to a
business upstream or be presented as a production gateway.

## Documentation

English is the default language. User-facing documents have a Simplified Chinese
mirror named `<file>.zh-CN.md`.

- If you change an English document, update its Chinese mirror in the same PR.
- Keep commands, flags, defaults, and version numbers identical in both versions.
- Legal and third-party notices are English-only.
- Use `make docs-check` before opening a PR.

## Pull requests

A PR should include:

- what changed and why;
- test evidence (`go test`, `go vet`, `make conformance`);
- documentation updates when behavior or CLI output changes;
- no real tokens, hostnames, cookies, or production data.

Small, focused PRs are easier to review. Branch protection requires the `test` and `links`
checks and linear history.
