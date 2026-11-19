# Quick start

**English** | [简体中文](quickstart.zh-CN.md)

This guide covers three independent paths:

1. run the template-conformance vectors offline;
2. connect the control console to shield-x;
3. exercise a redirect policy through the HTTP data plane.

## Prerequisites

| Need | Notes |
|---|---|
| Go 1.26+ | for `go install` or source builds |
| shield-x URL | something like `http://127.0.0.1:8080/shieldx` |
| site token | created in the shield-x console; never commit it |
| gateway ID | unique per simulated gateway, for example `gw-mock-01` |

Release binaries do not require Go. `conformance` does not require shield-x.

## Path 1: offline conformance

The vectors live in this repository, so clone it first:

```bash
git clone https://github.com/linyu19872008/shield-x-mock-gateway.git
cd shield-x-mock-gateway
go run ./cmd/mock-gateway conformance \
  --vectors ./testdata/template-conformance/cases.json
```

A successful run exits with code `0` and prints `14/14`.

If you installed with `go install`, the bundled vectors are not in the binary.
Pass an external path:

```bash
mock-gateway conformance --vectors /path/to/cases.json
```

## Path 2: control console

```bash
cp config.example.yaml config.yaml
```

Edit the connection block:

```yaml
shieldx:
  base_url: "http://127.0.0.1:8080/shieldx"
  token: "sx_REPLACE_ME"
  gateway_id: "gw-mock-01"
```

Start it:

```bash
./bin/mock-gateway serve --config ./config.yaml
```

Open <http://127.0.0.1:18080/>. The first poll usually appears within five
seconds. The UI follows the browser language; use the `EN / 中文` switch or `?lang=en` / `?lang=zh`. The console shows the policy revision, templates, reporting stats,
queue depth, and spool state.

Useful checks:

```bash
curl -sS http://127.0.0.1:18080/healthz
curl -sS http://127.0.0.1:18080/api/status
```

For remote access, both flags are required:

```bash
./bin/mock-gateway serve \
  --config ./config.yaml \
  --listen 0.0.0.0:18080 \
  --allow-remote \
  --ui-token 'choose-a-long-random-token'
```

## Path 3: HTTP data plane and redirects

The data plane is a separate listener. It evaluates the local policy snapshot and
returns mock responses; it does not proxy to a business upstream.

```bash
./bin/mock-gateway gateway \
  --config ./config.yaml \
  --listen 127.0.0.1:18081 \
  --gateway-id gw-dataplane-check
```

Create a redirect policy in shield-x first. The policy decides which requests
match and where they are sent. For example, a policy can match the `AhrefsBot`
user agent and redirect to a target you control.

```bash
curl -sS -D- -o /dev/null \
  -A 'AhrefsBot/7.0' \
  http://127.0.0.1:18081/crawler-test
```

Expected for a matching redirect policy:

```http
HTTP/1.1 302 Found
Location: https://example.com/trap
Cache-Control: no-store
Content-Length: 0
```

The mock never contacts `https://example.com/trap`. It only returns the response
defined by the policy. Use a target you control and do not point tests at
third-party sites.

A request that does not match a policy returns the configured mock upstream
response, usually `200` with `mock upstream`.

## Verify the result

1. The request gets the expected status and `Location`.
2. The shield-x event appears with `action=redirect` and `status=302`.
3. A normal browser user agent does not get redirected.
4. `gateway_id` is unique if you run several simulators.
5. The target is never fetched by the mock.

## Next steps

- [CLI reference](cli.md)
- [Configuration](configuration.md)
- [Data plane](data-plane.md)
- [Troubleshooting](troubleshooting.md)
