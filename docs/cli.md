# CLI reference

**English** | [简体中文](cli.zh-CN.md)

```text
mock-gateway <command> [flags]
```

`mock-gateway` has five operational commands plus `version`.

## Exit codes

| Code | Meaning |
|---|---|
| `0` | success |
| `1` | runtime/configuration failure |
| `2` | invalid command-line flags or unknown command |

## `conformance`

Run the language-neutral template-rendering vectors. No shield-x instance is required.

```bash
mock-gateway conformance \
  --vectors ./testdata/template-conformance/cases.json
```

| Flag | Default | Description |
|---|---|---|
| `--vectors` | `./testdata/template-conformance/cases.json` | path to the vector file |
| `-v` | `false` | print the explanation for each case |

Exit code `0` means every vector passed. A failing run prints the failing case and
returns `1`.

## `serve`

Start the control UI and control API. default listener: `127.0.0.1:18080`.

```bash
mock-gateway serve --config ./config.yaml
```

| Flag | Default | Description |
|---|---|---|
| `--config` | empty | YAML config path |
| `--listen` | `127.0.0.1:18080` | control listener |
| `--allow-remote` | `false` | allow a non-loopback listener |
| `--ui-token` | empty | required with `--allow-remote`; sent as `X-Mock-Token` or `Authorization: Bearer` |

The console can start and stop `run` and `loadtest` traffic. It does not proxy
application traffic. Configuration changes require a restart.

Remote example:

```bash
mock-gateway serve \
  --config ./config.yaml \
  --listen 0.0.0.0:18080 \
  --allow-remote \
  --ui-token 'choose-a-long-random-token'
```

## `gateway`

Start the HTTP data plane. This command receives real requests, evaluates the
local policy snapshot, returns a mock response, and reports the event.

```bash
mock-gateway gateway --config ./config.yaml
```

| Flag | Default | Description |
|---|---|---|
| `--config` | empty | YAML config path |
| `--listen` | `ingress.listen` | data-plane listener; default `127.0.0.1:18081` |
| `--allow-remote` | `false` | allow a non-loopback listener |
| `--base-url` | config value | override `shieldx.base_url` |
| `--token` | config value | override `shieldx.token` |
| `--gateway-id` | config value | override `shieldx.gateway_id` |

The data plane never forwards requests upstream. `observe` and unmatched requests
return the configured mock response. See [Data plane](data-plane.md).

## `run`

Poll policies and continuously generate synthetic events.

```bash
mock-gateway run --config ./config.yaml
```

| Flag | Default | Description |
|---|---|---|
| `--config` | empty | YAML config path |
| `--base-url` | config value | override `shieldx.base_url` |
| `--token` | config value | override `shieldx.token` |
| `--gateway-id` | config value | override `shieldx.gateway_id` |
| `--rate` | `0` | override `generate.rate`; `0` means use the config |
| `--backfill` | `0` | generate a backfilled time range, for example `1h` |

Press Ctrl-C to flush and exit.

## `loadtest`

Run a fixed-rate test and emit a JSON report.

```bash
mock-gateway loadtest \
  --config ./config.yaml \
  --rate 3000 \
  --duration 10m \
  --report ./loadtest.json
```

| Flag | Default | Description |
|---|---|---|
| `--config` | empty | YAML config path |
| `--base-url` | config value | override `shieldx.base_url` |
| `--token` | config value | override `shieldx.token` |
| `--gateway-id` | config value | override `shieldx.gateway_id` |
| `--rate` | config value | target rate, for example `3000` or `3k` |
| `--duration` | `1m` | test duration |
| `--dry-run` | `false` | generate without reporting; use to test the mock itself |
| `--report` | stdout | report output path |

Always run a dry run before interpreting a low achieved rate as a server problem.

## `version`

```bash
mock-gateway version
```

Prints the binary version and exits `0`.
