# Architecture

**English** | [简体中文](architecture.zh-CN.md)

mock-gateway is a single Go binary with two independent HTTP listeners and several
background components.

```text
                    ┌──────────────────────────────┐
                    │        mock-gateway          │
                    │                              │
  control UI/API ──►│ serve :18080                 │
                    │   ├─ controller              │
                    │   └─ generator/loadtest      │
                    │                              │
  real requests  ──►│ gateway :18081               │
                    │   ├─ policy evaluator        │
                    │   ├─ template renderer       │
                    │   └─ mock response           │
                    │                              │
                    │        agentlink             │
                    │   ├─ poller ──► shield-x     │
                    │   └─ reporter + spool        │
                    └──────────────────────────────┘
```

## Process modes

| Mode | Entry point | Purpose |
|---|---|---|
| `serve` | control listener | UI/API, start/stop synthetic traffic |
| `gateway` | data-plane listener | evaluate real requests and return mock responses |
| `run` | headless | poll policies and generate events continuously |
| `loadtest` | headless | fixed-rate generation with a JSON report |
| `conformance` | no listener | run template vectors locally |

`serve` and `gateway` can run in separate processes or side by side on different
ports. They use the same configuration file format.

## Components

| Component | Responsibility |
|---|---|
| Config loader | YAML defaults, validation, CLI overrides |
| SDK poller | pull policies/templates, handle 304 and retries |
| Policy state | atomic snapshot replacement and disk persistence |
| Template cache | render block pages and handle missing templates |
| Evaluator | first-match matching for ip/host/path/method/header/ua/query/cookie |
| Generator | synthetic traffic for `run`/`loadtest` |
| Data plane | real HTTP requests, mock responses, real events |
| Reporter | batch, gzip, retry, spool and replay |
| Control API/UI | status, policies, templates, traffic and fault controls |

## Policy lifecycle

```text
shield-x
  │  GET /policies/v1
  ▼
SDK poller
  │  full snapshot
  ▼
policyState.apply
  ├─ replace snapshot atomically
  ├─ persist to disk
  └─ update template cache if needed
```

A failed poll leaves the previous snapshot in place. A restart loads the persisted
snapshot before the first poll completes.

## Request lifecycle (data plane)

1. Parse the request into an evaluation input.
2. Evaluate policies in server order; first match wins.
3. Build the response:
   - no match/observe: mock upstream response;
   - block: template response;
   - redirect: status + `Location`;
   - challenge: mock challenge page;
   - evaluation error: 503 + `gateway_error`.
4. Write the response.
5. Enqueue an event; the SDK reporter batches and uploads it.

The data plane never contacts the target of a redirect and never proxies to an
application upstream.

## SDK boundary

`agentlink` is the only adapter between mock-gateway and
`shield-x-client-go`. Policy evaluation, template rendering, traffic generation,
and HTTP response construction remain independent mock implementations.

This separation keeps the mock useful for SDK E2E testing while avoiding a second
copy of the SDK internals.

## Trust boundaries

- Control and data-plane listeners are loopback by default.
- Remote UI access needs an explicit flag and token.
- `X-Forwarded-For` is untrusted unless `trust_proxy` is enabled.
- Tokens are configuration secrets and are never included in docs examples as
  real values.
- The mock is a test tool, not a production security boundary.
