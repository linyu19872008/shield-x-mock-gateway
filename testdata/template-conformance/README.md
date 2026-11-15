# Template-conformance vectors

**English** | [简体中文](README.zh-CN.md)

Language-neutral vectors for the shield-x template-rendering contract. Any gateway
implementation can use the same vectors; passing all cases is required before
claiming `block_templates: true`.

- Vector file: [`cases.json`](cases.json) — 14 cases
- Source and license: [`SOURCE.md`](SOURCE.md)
- Contract reference: [agent-v1](../../docs/contract/agent-v1.md)

## File format

```json
{
  "cases": [
    {
      "id": "01-simple-replace",
      "description": "what this case proves",
      "template": "Hello {{name}}",
      "vars": {"name": "World"},
      "expected": "Hello World"
    }
  ]
}
```

| Field | Type | Meaning |
|---|---|---|
| `id` | string | unique, kebab-case |
| `description` | string | verification point |
| `template` | string | template source |
| `template_repeat` | object | alternative to `template`: `{text, times}` |
| `vars` | object | placeholder values |
| `expected` | string | expected output, byte-for-byte |
| `expect_error` | string | expected error code instead of `expected` |

## Rendering rules

The runner performs a single left-to-right pass with:

```text
\{\{\s*([a-z_]+)\s*\}\}
```

1. Strip a leading UTF-8 BOM.
2. If the UTF-8 size is greater than **65536 bytes**, fail with
   `TEMPLATE_TOO_LARGE`.
3. Match the regex left to right:
   - whitelisted name → replace with the **HTML-escaped** value;
   - unknown name → keep the whole match unchanged, including whitespace;
   - missing/null variable → replace with an empty string.
4. The replacement is not scanned again (single pass, no recursion).

Whitelist:

```text
request_id, timestamp, client_ip, host, path, site_name, contact_email
```

HTML escape order:

| Step | Replacement |
|---|---|
| 1 | `&` → `&amp;` |
| 2 | `<` → `&lt;` |
| 3 | `>` → `&gt;` |
| 4 | `"` → `&quot;` |
| 5 | `'` → `&#39;` |

Escaping is required to prevent template injection and reflected XSS. Do not add
extra escaping beyond these substitutions.

## Cases

| # | ID | What it checks |
|---|---|---|
| 01 | `01-simple-replace` | basic replacement |
| 02 | `02-multi-placeholders` | several placeholders in one template |
| 03 | `03-escape-special-chars` | all five special characters are escaped |
| 04 | `04-escape-amp-order` | `&` must be escaped before `<` |
| 05 | `05-unknown-kept` | unknown placeholder remains unchanged, including whitespace |
| 06 | `06-missing-var-empty` | a whitelisted placeholder with a missing value becomes empty |
| 07 | `07-css-braces` | inline CSS braces are not treated as placeholders |
| 08 | `08-nested-braces` | `{{{{request_id}}}}` has deterministic single-pass behavior |
| 09 | `09-unicode` | Chinese, emoji, and IPv6 values pass through |
| 10 | `10-crlf-preserved` | CRLF is preserved |
| 11 | `11-bom-stripped` | leading UTF-8 BOM is removed |
| 12 | `12-too-large` | templates over 64 KiB fail |
| 13 | `13-whitespace-in-placeholder` | whitespace inside braces is allowed |
| 14 | `14-uppercase-not-matched` | uppercase names are not placeholders |

### Two cases worth highlighting

**`04-escape-amp-order`**: for `a<b`, first escaping `&` and then `<` gives
`a&lt;b`; reversing the order gives an extra `&amp;lt;b`.

**`13-whitespace-in-placeholder`**: `{{   request_id   }}` must work. A naive
`strings.ReplaceAll(tpl, "{{request_id}}", value)` implementation fails this case.

## Run the vectors

With mock-gateway:

```bash
# from the repository root
go run ./cmd/mock-gateway conformance \
  --vectors ./testdata/template-conformance/cases.json

# verbose explanations
go run ./cmd/mock-gateway conformance -v \
  --vectors ./testdata/template-conformance/cases.json
```

Exit code `0` means all vectors passed.

## For other languages

Implement a renderer that follows the rules and run the same JSON cases in your
own test harness. Do not use a template engine that evaluates expressions or
allows arbitrary code execution.

## Common implementation errors

| Symptom | Likely cause |
|---|---|
| unknown placeholder replaced | regex is too broad |
| uppercase name replaced | placeholder matching is case-insensitive |
| HTML escaped incorrectly | missing or double escaping; follow the five-step order |
| empty value becomes error | missing vars treated as an error |
| 64 KiB not rejected | size limit measured on characters, not UTF-8 bytes |
| braces with spaces fail | parser is too strict about whitespace |
| CSS braces replaced | parser matches single braces, not the `{{...}}` regex |

## Change process

The shield-x repository is the canonical source for these vectors. When updating
them, copy from the canonical source and update `SOURCE.md` with the source
commit. The mock repository's CI must pass all vectors.
