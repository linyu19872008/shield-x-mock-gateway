# Contributing

Thanks for helping improve mock-gateway.

## Development

```bash
go test ./...
go vet ./...
go run . conformance --vectors ./testdata/template-conformance/cases.json
```

Use English commit messages. Keep changes focused and include tests for behavior
changes. By submitting a contribution, you agree that it is licensed under the
MIT license used by this repository.

## Scope

mock-gateway is a testing and simulation tool. It must not proxy requests to a
business upstream or be presented as a production gateway.
