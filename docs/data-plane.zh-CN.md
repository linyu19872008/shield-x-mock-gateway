# 数据面

[English](data-plane.md) | **简体中文**

`gateway` 子命令是一个 HTTP 数据面模拟器。它接收真实请求，使用最新的本地策略快照求值，返回 mock 响应，并上报真实事件。它不会把请求代理到业务上游。

```bash
mock-gateway gateway \
  --config ./config.yaml \
  --listen 127.0.0.1:18081 \
  --gateway-id gw-dataplane-check
```

## 请求流程

```text
客户端
  │
  ▼
mock 数据面 :18081
  ├─ 构造求值输入（IP、Host、Path、Method、UA、Header、Query、Cookie）
  ├─ 对本地快照做 first-match 策略求值
  ├─ 选择 mock 响应
  ├─ 写响应
  └─ 事件入队 → SDK reporter → shield-x
```

策略快照由 SDK poller 维护。首次快照加载完成前的请求按“没有命中策略”处理。

## 动作响应

| 策略动作 | 状态码 | Body / Header |
|---|---|---|
| 未命中 | `ingress.mock_status`（默认 200） | `mock_body` |
| `observe` | `ingress.mock_status` | `mock_body`；mock 不拦截 |
| `block` | 策略状态码、模板状态码或 403 | 渲染拦截页；`Cache-Control: no-store` |
| `redirect` | `301` / `302` / `307` | `Location: <redirect_url>`；`Cache-Control: no-store`；空 body |
| `javascript_challenge` | 200 | 简单 mock 挑战页 |
| 求值异常 | 503 | 纯文本错误；事件 `decision_source=gateway_error` |

所有响应都带 `X-Shieldx-Request-Id`。数据面响应还带 `X-Shieldx-Mock: dataplane`。

## 重定向语义

| 状态码 | 含义 | 使用建议 |
|---|---|---|
| `302 Found` | 临时重定向 | 默认；目标后续可改 |
| `301 Moved Permanently` | 永久重定向 | 测试目标可能变化时不要用 |
| `307 Temporary Redirect` | 临时且保留方法和 body | 需要保持方法时使用 |

目标始终来自策略的 `redirect_url`。mock 不访问目标，也不会自动追加 query、Cookie 或凭据。

重定向地址必须是绝对 `http(s)` URL，并且应指向你控制的地址。跳回同一 host/path 的策略可能造成循环。

## 拦截响应

`block` 使用策略引用的模板。模板缺失或渲染失败时返回内置兜底页，事件标记 `action_result=failed`。

响应可以包含模板 Header。mock 会附加 `Cache-Control: no-store`。

## 客户端 IP 与 `X-Forwarded-For`

默认从 TCP 远端地址取客户端 IP，忽略 `X-Forwarded-For`。

只有 mock 位于可信代理之后、且代理会覆盖该头时，才开启 `ingress.trust_proxy: true`。否则客户端可以伪造 IP，影响基于 IP 的策略匹配。

## 事件上报

事件包含真实请求字段和 mock 结果：

| 字段 | 来源 |
|---|---|
| `client_ip` | 远端地址或可信 XFF |
| `host`、`path`、`method`、`query` | 请求 |
| `ua`、`referer` | 请求头 |
| `status` | 实际写给客户端的状态 |
| `duration_ms` | 本地处理耗时 |
| `action` / `action_result` | 命中的动作和结果 |
| `matched_policy_id` / priority | 策略匹配结果 |
| `config_revision` | 快照 revision |
| `bytes` | mock 响应体长度 |
| `upstream_status` | 不设置；没有访问上游 |

事件通过 `shield-x-client-go` 的批量、重试和 spool 逻辑上报。

## 用 curl 验证

```bash
curl -sS -D- -o /dev/null \
  -A 'AhrefsBot/7.0' \
  http://127.0.0.1:18081/crawler-test
```

命中重定向策略时预期：

```http
HTTP/1.1 302 Found
Location: https://example.com/trap
Cache-Control: no-store
X-Shieldx-Mock: dataplane
X-Shieldx-Request-Id: gw-dataplane-check-...
Content-Length: 0
```

普通浏览器 UA 应返回 mock 上游响应，而不是重定向，除非它同时命中其他策略。

## 限制

- 不代理到业务应用，因此不能证明真实上游被保护；
- 不提供真实 TLS/mTLS；
- 挑战页是模拟，不是真实浏览器挑战；
- 性能数字只代表 mock，不代表生产网关；
- 没有真实网关的连接池和中间件开销。

真实端到端验证请使用真实网关，并应用相同的策略和流量。
