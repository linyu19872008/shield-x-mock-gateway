# Troubleshooting

**English** | [简体中文](troubleshooting.zh-CN.md)

Start with the control console status and the process logs. Most failures are
configuration, token, or policy-match issues rather than a mock defect.

## `401 INVALID_TOKEN`

```text
agent error: shield-x: HTTP 401 INVALID_TOKEN: Token 无效
```

Check:

1. `shieldx.token` is a real site token, not `sx_REPLACE_ME`.
2. `shieldx.base_url` includes `/shieldx`.
3. The token was not revoked or rotated.
4. The token belongs to the site whose policies you expect.

The mock may still serve from a persisted snapshot after a 401, but it will not
receive updates or report events.

## No policies / revision stays old

- Check `/api/status` and `/api/policies` on the control console.
- Confirm the site has enabled policies.
- Confirm `capabilities.actions` and `capabilities.match_fields` include what the
  policies use; unsupported policies may be filtered out by the server.
- Check `poll.interval`, network access, and TLS settings.
- Look for `agent error` in the console log.

## A redirect policy does not match

Check in this order:

1. Is the policy enabled?
2. Is its priority high enough? The first match wins; a higher-priority block or
   observe policy can win first.
3. Does the request method match?
4. Does the UA/path/host/query condition match exactly?
5. Is the request reaching `gateway`, not `serve`?
6. Is the data plane on the expected port (`18081` by default)?
7. Did the gateway poll the latest revision?

Try a minimal request:

```bash
curl -sS -D- -o /dev/null \
  -A 'AhrefsBot/7.0' \
  http://127.0.0.1:18081/crawler-test
```

If the status is 200 with `mock upstream`, no policy matched. If it is 403, a block
policy matched first. If it is 302, check the `Location` value.

## `conformance` cannot find vectors

The `go install` binary does not bundle `testdata/`. Clone the repository:

```bash
git clone https://github.com/linyu19872008/shield-x-mock-gateway.git
cd shield-x-mock-gateway
go run ./cmd/mock-gateway conformance \
  --vectors ./testdata/template-conformance/cases.json
```

Or pass an explicit `--vectors` path.

## Port already in use

Defaults:

| Process | Port |
|---|---|
| `serve` | `127.0.0.1:18080` |
| `gateway` | `127.0.0.1:18081` |

Use `--listen` to change a port. `serve` and `gateway` can run at the same time
because they use different ports.

## UI is blank or asks for a token

- Open `http://127.0.0.1:18080/`, not the API path.
- If `--ui-token` is set, enter the same token in the UI.
- Verify `curl http://127.0.0.1:18080/healthz`.
- Check that the process did not fail at startup.

## Remote UI returns 401/403

Remote access requires both flags:

```bash
mock-gateway serve \
  --config ./config.yaml \
  --listen 0.0.0.0:18080 \
  --allow-remote \
  --ui-token 'long-random-token'
```

Send `X-Mock-Token: <token>` or `Authorization: Bearer <token>` on API requests.

## Events are not appearing in shield-x

- Check `report` settings and `/api/status` queue/spool counters.
- Check `report.spool_dir` exists and is writable.
- Look for 413/429/5xx responses in the console log.
- A 401 prevents reporting even if the data plane still serves responses.
- Wait at least one `report.interval` plus upload time.

## Block page is the built-in fallback

The policy references a missing template, or template rendering failed. The event
has `action_result=failed`. Check the template was pulled and the template version
matches the policy.

## The target URL is never contacted

That is expected. The mock returns `Location` and does not proxy or fetch the
target. Use a real gateway if you need to verify the target path end to end.

## Remote IP policy does not work

`X-Forwarded-For` is ignored by default. Enable `ingress.trust_proxy: true` only
behind a trusted proxy that overwrites the header. Do not enable it on a directly
exposed listener.

## Getting help

Open an issue with:

- mock-gateway version (`mock-gateway version`);
- operating system and Go version if building from source;
- exact command;
- redacted config;
- relevant log lines;
- expected and actual status/headers.

Never include a real site token, cookie, or production URL.
