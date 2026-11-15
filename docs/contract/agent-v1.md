# agent-v1 contract reference

**English** | [简体中文](agent-v1.zh-CN.md)

This is a compact English reference for the shield-x agent-v1 contract as used by
mock-gateway. The full Chinese canonical document is
[agent-v1.zh-CN.md](agent-v1.zh-CN.md); when the two differ, the canonical document
is authoritative.

> Derived from shield-x and licensed under Apache-2.0. See
> [`LICENSES/Apache-2.0.txt`](../../LICENSES/Apache-2.0.txt) and
> [`THIRD_PARTY_NOTICES.md`](../../THIRD_PARTY_NOTICES.md).

## 1. Roles

- **shield-x control plane**: stores policies/templates, authenticates gateways,
  serves snapshots, accepts events.
- **gateway**: polls policies/templates, evaluates requests locally, reports
  events, and keeps working with the last known snapshot when shield-x is down.

mock-gateway implements the gateway side and a testing data plane.

## 2. Authentication

All agent requests use a tenant token:

```http
Authorization: Bearer <tenant-token>
```

Do not put tokens in query strings, logs, or issue reports.

## 3. Common headers

| Header | Direction | Purpose |
|---|---|---|
| `Authorization` | gateway → shield-x | bearer token |
| `X-Shieldx-Gateway-Id` | gateway → shield-x | gateway identity |
| `X-Shieldx-Agent-Version` | gateway → shield-x | implementation version |
| `X-Shieldx-Capabilities` | gateway → shield-x | compact JSON capability declaration |
| `If-None-Match` | gateway → shield-x | ETag for policy/template polling |
| `X-Shieldx-Request-Id` | mock → client | request/event correlation |

## 4. Pull policies

```http
GET /shieldx/policies/v1
```

A full snapshot is returned. Example shape:

```json
{
  "revision": 12,
  "policies": [
    {
      "id": "pol_01...",
      "version": 1,
      "name": "Block env probes",
      "priority": 1000,
      "match": {
        "all": [
          {"field": "path", "op": "prefix", "value": "/.env"}
        ]
      },
      "action": {
        "type": "block",
        "params": {"status_code": 403, "template_id": "tpl_01..."}
      }
    }
  ]
}
```

Rules:

- policies are ordered by the server; the client must not re-sort;
- first match wins;
- an empty list means no policies and replaces the local snapshot;
- `304 Not Modified` keeps the current snapshot;
- on failure, keep the previous snapshot.

## 5. Pull templates

```http
GET /shieldx/templates/v1
```

Example:

```json
{
  "revision": 12,
  "templates": [
    {
      "id": "tpl_01...",
      "name": "Block 403",
      "version": 1,
      "checksum": "sha256:...",
      "content_type": "text/html; charset=utf-8",
      "status_code": 403,
      "headers": {"Cache-Control": "no-store"},
      "content": "<html>...{{request_id}}...</html>",
      "vars": {"site_name": "Example"}
    }
  ]
}
```

Template rendering is byte-for-byte defined by the template-conformance vectors.
Supported placeholders and rules are documented in
[`testdata/template-conformance/README.md`](../../testdata/template-conformance/README.md).

## 6. Policy match model

```json
{"all": [...]}
{"any": [...]}
{"not": {...}}
```

Condition:

```json
{"field": "path", "op": "prefix", "value": "/admin"}
```

Fields: `ip`, `host`, `path`, `method`, `header`, `ua`, `query`, `cookie`.

Operators: `eq`, `prefix`, `suffix`, `contains`, `regex`, `in`, `in_cidr`,
`not_in_cidr`.

## 7. Actions

| Type | Parameters | Gateway behavior |
|---|---|---|
| `observe` | none | do not change the response; report the event |
| `block` | `status_code`, `template_id` | render a block response |
| `redirect` | `redirect_url`, `status_code` | return 301/302/307 + `Location`; default 302 |
| `javascript_challenge` | `challenge_ttl_s` | return a challenge response |

`redirect_url` must be an absolute `http(s)` URL. `status_code` is limited to
301/302/307.

## 8. Report events

```http
POST /shieldx/events/v1
```

Events are sent in batches. A `202 Accepted` means the batch was accepted. The
reporter handles 413/429/5xx, retry/backoff, gzip, and disk spool.

Core event fields:

| Field | Type | Notes |
|---|---|---|
| `event_id` | string | unique per event |
| `ts` | RFC3339 timestamp | event time |
| `gateway_id` | string | gateway identity |
| `client_ip` | string | client address |
| `real_ip_chain` | list | optional trusted proxy chain |
| `host`, `path`, `query`, `method` | string | request |
| `ua`, `referer` | string | optional |
| `status` | int | response status |
| `upstream_status` | int/null | upstream status when applicable |
| `duration_ms` | int | processing time |
| `bytes` | int | response bytes |
| `matched_policy_id` | string/null | null when no policy matched |
| `matched_policy_priority` | int | optional |
| `config_revision` | int | policy revision |
| `action` | string | `none`/`observe`/`block`/`redirect`/`javascript_challenge` |
| `action_result` | string | `executed`/`failed`/`challenged` |
| `decision_source` | string | `policy`/`none`/`gateway_error` |
| `decision_latency_us` | int | optional |
| `block_template_id`, `block_template_version` | optional | block |
| `trace_id` | string | optional |
| `clock_skew_ms` | int | optional |

## 9. Capabilities

```json
{
  "actions": ["observe", "block", "redirect", "javascript_challenge"],
  "match_fields": ["ip", "host", "path", "method", "header", "ua", "query", "cookie"],
  "block_templates": true,
  "template_spec_version": 1,
  "max_rules": 5000
}
```

The server may filter policies that use unsupported capabilities.

## 10. Failure behavior

| Condition | Gateway behavior |
|---|---|
| shield-x unreachable | keep last snapshot; retry with backoff |
| invalid token | do not update snapshot; report failure |
| malformed policy | report `gateway_error` for the request |
| missing template | use fallback page; mark `action_result=failed` |
| empty policy list | apply the empty snapshot |
| reporter failure | retry and spool if configured |

## 11. Data-plane response summary

| Action | Status | Body |
|---|---|---|
| no match/observe | configured mock status | mock body |
| block | template or 403 | block page |
| redirect | 301/302/307 | empty; `Location` |
| challenge | 200 | mock challenge page |
| evaluation error | 503 | error text |

The mock does not forward requests to an upstream application.
