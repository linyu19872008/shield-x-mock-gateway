# mock-gateway

[![CI](https://github.com/linyu19872008/shield-x-mock-gateway/actions/workflows/ci.yml/badge.svg)](https://github.com/linyu19872008/shield-x-mock-gateway/actions/workflows/ci.yml)
[![Release](https://img.shields.io/github/v/release/linyu19872008/shield-x-mock-gateway)](https://github.com/linyu19872008/shield-x-mock-gateway/releases)
[![Go](https://img.shields.io/github/go-mod/go-version/linyu19872008/shield-x-mock-gateway)](go.mod)
[![License: MIT](https://img.shields.io/badge/license-MIT-blue.svg)](LICENSE)

**English** | [简体中文](README.zh-CN.md)

A standalone gateway simulator for the **shield-x agent-v1 contract**. It evaluates
shield-x policies locally, returns mock HTTP responses, generates synthetic events,
and reports real events through `shield-x-client-go`.

It is a development and testing tool. It is **not** a production WAF and it never
proxies requests to an upstream application.

## What it is / what it is not

| It is | It is not |
|---|---|
| A local policy evaluator and HTTP data-plane simulator | A production gateway or WAF |
| A synthetic event source for shield-x development | A reverse proxy to your application |
| An SDK E2E driver for `shield-x-client-go` | A replacement for real gateway integration |
| A runner for the template-conformance vectors | A guarantee that a real gateway behaves identically |

## Features

- Five commands: `conformance`, `serve`, `gateway`, `run`, and `loadtest`.
- Local evaluation of shield-x policies with first-match semantics.
- Mock `200`, `403`, `301`/`302`/`307`, and JavaScript challenge responses.
- An embedded control UI with policy/template inspection, traffic controls, fault
  injection, reporting stats, and logs.
- Batched event reporting with disk spool and replay through the SDK.
- Language-neutral template-conformance vectors.

## UI language

The embedded control UI supports English and Simplified Chinese.

- No saved preference: Chinese browsers start in Chinese; all others start in English.
- Use the `EN / 中文` switch in the sidebar.
- The choice is stored in the `mock_lang` cookie.
- For screenshots or automation, append `?lang=en` or `?lang=zh` to the URL.

## Requirements

- Go **1.26+** when installing from source or `go install`.
- A reachable shield-x instance and a tenant token for `serve`, `gateway`, `run`,
  and `loadtest`.
- No shield-x instance is needed for `conformance`.

## Install

### Go install

```bash
go install github.com/linyu19872008/shield-x-mock-gateway/cmd/mock-gateway@v0.1.2
```

The installed binary is named `mock-gateway`.

### Release binary

Download a binary from the [releases page](https://github.com/linyu19872008/shield-x-mock-gateway/releases)
and verify it with the published `checksums.txt`.

### From source

```bash
git clone https://github.com/linyu19872008/shield-x-mock-gateway.git
cd shield-x-mock-gateway
go build -o bin/mock-gateway ./cmd/mock-gateway
```

> The `go install` binary does not include the test vectors. Clone this repository
> when you want to run `conformance` with the bundled vectors, or pass
> `--vectors /path/to/cases.json`.

## Quick start

### 1. Run conformance offline

```bash
git clone https://github.com/linyu19872008/shield-x-mock-gateway.git
cd shield-x-mock-gateway
go run ./cmd/mock-gateway conformance \
  --vectors ./testdata/template-conformance/cases.json
```

Expected: `14/14` passes and exit code `0`.

### 2. Connect to shield-x and open the control UI

1. Create a tenant token and a gateway ID in your shield-x console.
2. Copy the example configuration and edit the connection fields:

```bash
cp config.example.yaml config.yaml
```

```yaml
shieldx:
  base_url: "http://127.0.0.1:8080/shieldx"
  token: "sx_REPLACE_ME"
  gateway_id: "gw-mock-01"
```

3. Start the control console:

```bash
./bin/mock-gateway serve --config ./config.yaml
```

4. Open <http://127.0.0.1:18080/>.

The console can start `run` or `loadtest` traffic, inspect the current policy
snapshot and templates, inject faults, and show reporting/spool statistics.

### 3. Test a redirect policy through the data plane

Create a redirect policy in shield-x first. Then start the data plane:

```bash
./bin/mock-gateway gateway \
  --config ./config.yaml \
  --listen 127.0.0.1:18081 \
  --gateway-id gw-dataplane-check
```

Send a request that matches the policy. For example, if the policy matches the
`AhrefsBot` user agent:

```bash
curl -sS -D- -o /dev/null \
  -A 'AhrefsBot/7.0' \
  http://127.0.0.1:18081/crawler-test
```

Expected:

```http
HTTP/1.1 302 Found
Location: https://example.com/trap
Cache-Control: no-store
```

The `Location` comes from the policy `redirect_url`. The mock does **not** contact
that target; it only returns the response. Use a target you control and test with
a policy that you created for this purpose.

## Commands

| Command | Purpose | Needs shield-x |
|---|---|---|
| `conformance` | Run template-conformance vectors locally | No |
| `serve` | Control UI and control API | Yes |
| `gateway` | Real HTTP data plane with mock responses | Yes |
| `run` | Continuous synthetic event generation | Yes |
| `loadtest` | Fixed-rate load test and JSON report | Yes |
| `version` | Print the binary version | No |

See [CLI reference](docs/cli.md) for all flags, defaults, and exit codes.

## Configuration

A minimal configuration:

```yaml
shieldx:
  base_url: "http://127.0.0.1:8080/shieldx"
  token: "sx_REPLACE_ME"
  gateway_id: "gw-mock-01"

poll:
  interval: 5s

ingress:
  listen: "127.0.0.1:18081"
  trust_proxy: false
```

- `base_url` must include the `/shieldx` path prefix.
- `token` is a tenant token. Do not commit it.
- `gateway_id` should be unique per simulated gateway.
- `ingress.trust_proxy` defaults to `false`; only enable it behind a trusted proxy.
- `serve` also accepts `--config`, `--listen`, `--allow-remote`, and `--ui-token`.

See [configuration reference](docs/configuration.md) for every field and default.

## Compatibility

| Component | Version |
|---|---|
| mock-gateway | v0.1.2 |
| Go | 1.26+ |
| shield-x-client-go | v0.1.1 |
| Contract | agent-v1 |

The `v0.1.0` tag used an older module layout and is retracted. Use `v0.1.2`.

## Security boundaries

- The control console and data plane listen on loopback by default.
- Remote UI access requires `--allow-remote --ui-token`.
- `X-Forwarded-For` is ignored unless `ingress.trust_proxy: true`.
- The mock never forwards requests to an upstream application.
- Do not use this tool as a production security control.
- Do not paste real tokens into issues, logs, or screenshots.

See [data plane](docs/data-plane.md) and [troubleshooting](docs/troubleshooting.md)
for behavior and diagnostics.

## Documentation

- [Documentation index](docs/README.md)
- [Quick start](docs/quickstart.md)
- [CLI reference](docs/cli.md)
- [Configuration reference](docs/configuration.md)
- [Data plane](docs/data-plane.md)
- [Architecture](docs/architecture.md)
- [Design notes](docs/design.md)
- [agent-v1 contract reference](docs/contract/agent-v1.md)
- [Troubleshooting](docs/troubleshooting.md)

## Development

```bash
go test ./...
go vet ./...
go build ./...
make conformance
make docs-check
```

See [CONTRIBUTING.md](CONTRIBUTING.md) for PR expectations and translation rules.

## License

MIT. See [LICENSE](LICENSE), [NOTICE](NOTICE), and [THIRD_PARTY_NOTICES.md](THIRD_PARTY_NOTICES.md).

The `testdata/template-conformance` vectors and
`docs/contract/agent-v1.zh-CN.md` are derived from shield-x and remain under
Apache-2.0; see the notices.
