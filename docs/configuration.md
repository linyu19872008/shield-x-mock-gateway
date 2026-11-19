# Configuration reference

**English** | [简体中文](configuration.zh-CN.md)

mock-gateway reads a YAML file. Unspecified fields keep their defaults. Copy
[`config.example.yaml`](../config.example.yaml) and edit only what you need.

```bash
cp config.example.yaml config.yaml
```

Durations use Go syntax: `500ms`, `5s`, `2m`, `1h`.

## `shieldx`

| Field | Type | Default | Required | Description |
|---|---|---|---|---|
| `base_url` | string | `http://127.0.0.1:8080/shieldx` | yes | shield-x base URL, including `/shieldx` |
| `token` | string | empty | yes for connected modes | site token; do not commit it |
| `gateway_id` | string | `gw-mock-01` | yes | unique ID for this simulated gateway |
| `gateway_group` | string | empty | no | optional site-local group sent as `X-Shieldx-Gateway-Group` |
| `agent_version` | string | `mock-gateway/<version>` | no | reported agent version |
| `capabilities.actions` | list | all four actions | no | actions this mock claims to support |
| `capabilities.match_fields` | list | all supported fields | no | policy fields this mock can evaluate |
| `capabilities.block_templates` | bool | `true` | no | whether block templates are supported |
| `capabilities.template_spec_version` | int | `1` | no | template spec version |
| `capabilities.max_rules` | int | `5000` | no | maximum accepted rules |

The server filters policies using the capability declaration. If a policy uses an
action or field not listed here, it may be filtered out.

## `poll`

| Field | Type | Default | Description |
|---|---|---|---|
| `interval` | duration | `5s` | policy/template polling interval; minimum `1s` |
| `timeout` | duration | `3s` | per-request timeout |
| `persist_path` | string | `./data/policy-snapshot.json` | local snapshot path |
| `use_etag` | bool | `true` | send/use ETag and handle `304 Not Modified` |

The persisted snapshot lets a restart use the last known revision while the first
poll is in progress.

## `template`

| Field | Type | Default | Description |
|---|---|---|---|
| `fetch_on_missing` | bool | `true` | fetch templates referenced by policies but missing locally |
| `timeout` | duration | `3s` | template request timeout |

## `report`

| Field | Type | Default | Description |
|---|---|---|---|
| `interval` | duration | `2s` | batch flush interval |
| `batch_size` | int | `300` | events per batch; clamped to 1–1000 |
| `format` | string | `json` | `json` or `ndjson` |
| `gzip` | bool | `true` | compress event batches |
| `timeout` | duration | `10s` | upload timeout |
| `spool_dir` | string | `./data/spool` | disk spool for failed uploads |
| `spool_max_mb` | int | `200` | maximum spool size |
| `max_backoff` | duration | `60s` | maximum retry backoff |

## `generate`

Synthetic generation is used by `run` and `loadtest`. The `gateway` data plane
does not use this section.

| Field | Type | Default | Description |
|---|---|---|---|
| `rate` | int | `200` | generated events per second |
| `workers` | int | `4` | generator workers |
| `duration` | duration | `0` | `0` means run until stopped |
| `backfill` | duration | `0` | backfill window used by `run --backfill` |
| `ramp` | list | empty | rate steps: `{at, rate}` |
| `paths` | list | example paths | simulated path/method weights |
| `host_pool` | list | example hosts | simulated host weights |
| `ua_pool` | list | example UAs | simulated user-agent weights |
| `ip_pools.normal` | list | TEST-NET ranges | normal client CIDRs |
| `ip_pools.malicious` | list | `203.0.113.66` | malicious client CIDRs |
| `ip_pools.internal` | list | `10.0.0.0/8` | internal client CIDRs |
| `status_distribution` | map | 200/302/404/500 | synthetic upstream status weights |
| `bad_event_ratio` | float | `0.001` | invalid-event injection ratio |
| `gateway_error_ratio` | float | `0.0005` | `gateway_error` injection ratio |
| `timeout_ratio` | float | `0.001` | timeout event injection ratio |
| `sampled_ratio` | float | `0` | simulated sampling ratio |

`ramp` example:

```yaml
generate:
  rate: 100
  ramp:
    - { at: "0s",   rate: 100 }
    - { at: "60s",  rate: 1000 }
    - { at: "180s", rate: 3000 }
```

## `fault`

| Field | Type | Default | Description |
|---|---|---|---|
| `shieldx_down` | bool | `false` | make all shield-x requests fail |
| `http_status_override` | int | `0` | force a status such as `429` or `503` |
| `latency` | duration | `0s` | artificial latency |
| `drop_percent` | int | `0` | drop percentage of requests |

Fault injection is for testing retry, spool, and alert behavior. It is not a
production feature.

## `ingress`

Used by `gateway`.

| Field | Type | Default | Description |
|---|---|---|---|
| `listen` | string | `127.0.0.1:18081` | data-plane listener |
| `allow_remote` | bool | `false` | permit non-loopback listening |
| `mock_status` | int | `200` | mock upstream status for unmatched/observe |
| `mock_content_type` | string | `text/plain; charset=utf-8` | mock response content type |
| `mock_body` | string | `mock upstream\n` | mock response body |
| `read_timeout` | duration | `5s` | HTTP read timeout |
| `write_timeout` | duration | `10s` | HTTP write timeout |
| `trust_proxy` | bool | `false` | trust `X-Forwarded-For` from a proxy |

Only enable `trust_proxy` when a trusted reverse proxy sets the header. Otherwise
clients can spoof their source IP.

## Validation

The process fails at startup on:

- missing `shieldx.base_url`;
- negative `generate.rate`;
- invalid `report.format`;
- `ingress.mock_status` outside 100–599;
- an invalid listener when `allow_remote` is false.

## Backfill example

`config.backfill.yaml` is a separate example for generating historical events.
It is not required for normal use.
