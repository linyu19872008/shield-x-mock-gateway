# Data plane

**English** | [简体中文](data-plane.zh-CN.md)

The `gateway` command is an HTTP data-plane simulator. It receives real requests,
evaluates the latest local policy snapshot, returns a mock response, and reports a
real event. It never proxies the request to an upstream application.

```bash
mock-gateway gateway \
  --config ./config.yaml \
  --listen 127.0.0.1:18081 \
  --gateway-id gw-dataplane-check
```

## Request flow

```text
client
  │
  ▼
mock data plane :18081
  ├─ build evaluation input (IP, host, path, method, UA, headers, query, cookies)
  ├─ first-match policy evaluation against the local snapshot
  ├─ choose mock response
  ├─ write response
  └─ enqueue event → SDK reporter → shield-x
```

The policy snapshot is maintained by the SDK poller. A request that arrives before
the first snapshot is loaded is treated as having no matching policy.

## Action responses

| Policy action | Status | Body / headers |
|---|---|---|
| no match | `ingress.mock_status` (default 200) | `mock_body` |
| `observe` | `ingress.mock_status` | `mock_body`; the mock does not block |
| `block` | policy status, template status, or 403 | rendered block page; `Cache-Control: no-store` |
| `redirect` | `301`, `302`, or `307` | `Location: <redirect_url>`; `Cache-Control: no-store`; empty body |
| `javascript_challenge` | 200 | simple mock challenge page |
| evaluation error | 503 | plain error body; event `decision_source=gateway_error` |

All responses include `X-Shieldx-Request-Id`. Data-plane responses also include
`X-Shieldx-Mock: dataplane`.

## Redirect semantics

| Status | Meaning | Use |
|---|---|---|
| `302 Found` | temporary redirect | default; target can change later |
| `301 Moved Permanently` | permanent redirect | avoid for test targets that may change |
| `307 Temporary Redirect` | temporary, preserves method and body | use when the method must not change |

The target is always the policy's `redirect_url`. The mock does not fetch it and
does not append query strings, cookies, or credentials.

A redirect URL must be an absolute `http(s)` URL and should point to a target you
control. A policy that redirects back to the same host/path can create a loop.

## Block responses

`block` uses the template referenced by the policy. If the template is missing or
cannot be rendered, the built-in fallback page is returned and the event is marked
`action_result=failed`.

The response may include template headers. `Cache-Control: no-store` is applied by
the mock.

## Client IP and `X-Forwarded-For`

By default the client IP is taken from the TCP remote address and
`X-Forwarded-For` is ignored.

Enable `ingress.trust_proxy: true` only when the mock is behind a trusted proxy
that overwrites the header. Otherwise an arbitrary client can spoof its IP and
change IP-based policy matching.

## Event reporting

The event contains the real request fields and the mock result:

| Field | Source |
|---|---|
| `client_ip` | remote address, or trusted XFF |
| `host`, `path`, `method`, `query` | request |
| `ua`, `referer` | request headers |
| `status` | response written to the client |
| `duration_ms` | local processing time |
| `action` / `action_result` | matched action and result |
| `matched_policy_id` / priority | policy match |
| `config_revision` | snapshot revision |
| `bytes` | mock response body length |
| `upstream_status` | not set; no upstream is contacted |

Events go through `shield-x-client-go` batching, retry, and spool behavior.

## Verify with curl

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
X-Shieldx-Mock: dataplane
X-Shieldx-Request-Id: gw-dataplane-check-...
Content-Length: 0
```

A normal browser user agent should return the mock upstream response, not a
redirect, unless it also matches a policy.

## Limits

- It does not proxy to a business application, so it cannot prove that a real
  upstream is protected.
- It does not provide real TLS or mTLS.
- The challenge page is a simulation, not a browser challenge.
- Performance numbers reflect the mock, not a production gateway.
- There is no connection pooling or middleware overhead of a real gateway.

For real end-to-end verification, use a real gateway implementation and apply the
same policy and traffic.
