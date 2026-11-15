# Design notes

**English** | [简体中文](design.zh-CN.md)

This document is for contributors who need to change mock-gateway internals. For
user-facing behavior, start with [Quick start](quickstart.md) and
[Data plane](data-plane.md).

## Design goals

1. **Independent policy evaluation and rendering.** The mock must not import
   shield-x server internals. This keeps the implementation useful as an
   independent check of the contract.
2. **SDK-based agent I/O.** Policy polling, template pulling, event reporting, and
   spool replay go through `shield-x-client-go` via `agentlink`.
3. **No upstream proxy.** The mock is a simulator. It returns local responses and
   never introduces a dependency on a business application.
4. **Deterministic responses.** Given the same snapshot and request, the response
   is predictable, which makes tests and CI reliable.
5. **Explicit failure.** Evaluation errors become `gateway_error`, not a silent
   "no match".

## Package layout

```text
cmd/mock-gateway/
  main.go           command dispatch
  agentlink/        only adapter to shield-x-client-go
  config.go         YAML schema, defaults, validation
  state.go          policy snapshot and disk persistence
  evaluator.go      first-match policy evaluation
  renderer.go       template rendering
  templates.go      template cache and fallback page
  generator.go      synthetic traffic
  gateway.go        HTTP data plane
  serve.go          control UI/API
  run.go            headless generator
  loadtest.go       load test and report
  web/              embedded control UI
```

## Policy snapshot

The poller returns a full snapshot. `policyState.apply`:

- atomically swaps the policy list and revision;
- persists the snapshot to `poll.persist_path`;
- never merges partial updates.

A failed poll leaves the old snapshot. On restart, the persisted snapshot is loaded
before the first network poll.

The evaluator must use the server-provided order. It must not re-sort by priority
on the client; doing so can diverge from another gateway implementation.

## Evaluator

Supported fields:

```text
ip, host, path, method, header, ua, query, cookie
```

Supported composition:

```text
all, any, not
```

Supported operators:

```text
eq, prefix, suffix, contains, regex, in, in_cidr, not_in_cidr
```

Missing fields evaluate to an empty value. An unsupported field/operator is an
evaluation error, which the data plane reports as `gateway_error`.

## Renderer

The renderer implements the template substitution rules from the contract. It has
a built-in fallback page for missing or invalid templates. A fallback response is
still sent, but the event is marked `action_result=failed`.

## Data plane

The data plane is intentionally not a reverse proxy:

- it builds an evaluation input from the request;
- it selects a policy;
- it writes a mock response;
- it enqueues an event.

`observe` and unmatched requests return configurable mock upstream content. This
makes it possible to test policy selection without a real backend.

`trust_proxy` is off by default. When enabled, the first valid `X-Forwarded-For`
address is used as the client IP.

## Reporter and spool

Events are enqueued through the SDK reporter. The reporter handles batching,
compression, retry, and disk spool. The mock does not implement a second upload
protocol.

## Generator

The generator is independent of the data plane. It produces weighted paths, hosts,
UAs, IP pools, status distributions, and fault events. `loadtest --dry-run` is the
self-check for the generator path.

## Testing strategy

- unit tests for evaluator, renderer, conversion, data plane, and control API;
- template-conformance vectors for rendering;
- dry-run load test for generator overhead;
- HTTP data-plane tests for redirect and block responses;
- no test requires a production shield-x instance.

## Non-goals

- production traffic handling;
- TLS termination or mTLS;
- real browser challenge execution;
- replacing a real gateway integration test.
