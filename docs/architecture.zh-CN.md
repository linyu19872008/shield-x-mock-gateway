# 架构

[English](architecture.md) | **简体中文**

mock-gateway 是一个 Go 单二进制，包含两个独立 HTTP 监听器和多个后台组件。

```text
                    ┌──────────────────────────────┐
                    │        mock-gateway          │
                    │                              │
  控制 UI/API ────►│ serve :18080                 │
                    │   ├─ controller              │
                    │   └─ generator/loadtest      │
                    │                              │
  真实请求   ────►│ gateway :18081               │
                    │   ├─ policy evaluator        │
                    │   ├─ template renderer       │
                    │   └─ mock response           │
                    │                              │
                    │        agentlink             │
                    │   ├─ poller ──► shield-x     │
                    │   └─ reporter + spool        │
                    └──────────────────────────────┘
```

## 运行模式

| 模式 | 入口 | 用途 |
|---|---|---|
| `serve` | 控制台监听 | UI/API，启停合成流量 |
| `gateway` | 数据面监听 | 求值真实请求并返回 mock 响应 |
| `run` | 无监听 | 拉策略并持续生成事件 |
| `loadtest` | 无监听 | 固定速率生成并输出 JSON 报告 |
| `conformance` | 无监听 | 本地运行模板向量 |

`serve` 和 `gateway` 可以是两个独立进程，也可以在不同端口同时运行。两者使用相同的配置格式。

## 组件

| 组件 | 职责 |
|---|---|
| 配置加载 | YAML 默认值、校验、命令行覆盖 |
| SDK poller | 拉策略/模板，处理 304 和重试 |
| 策略状态 | 原子快照替换和磁盘持久化 |
| 模板缓存 | 渲染拦截页，处理缺失模板 |
| 求值器 | 对 ip/host/path/method/header/ua/query/cookie 做 first-match |
| 生成器 | 为 `run`/`loadtest` 生成合成流量 |
| 数据面 | 接收真实 HTTP 请求、返回 mock、上报真实事件 |
| 上报器 | 批量、压缩、重试、spool 和补发 |
| 控制 API/UI | 状态、策略、模板、流量和故障控制 |

## 策略生命周期

```text
shield-x
  │  GET /policies/v1
  ▼
SDK poller
  │  全量快照
  ▼
State.Apply
  ├─ 原子替换快照
  ├─ 落盘
  └─ 按需更新模板缓存
```

轮询失败时保留旧快照。进程重启后先加载磁盘快照，再等待首次轮询完成。

## 请求生命周期（数据面）

1. 把请求解析为求值输入；
2. 按服务端顺序求值策略，first-match；
3. 构造响应：
   - 未命中/observe：mock 上游响应；
   - block：模板响应；
   - redirect：状态码 + `Location`；
   - challenge：mock 挑战页；
   - 求值异常：503 + `gateway_error`；
4. 写响应；
5. 事件入队，由 SDK reporter 批量上报。

数据面不会访问重定向目标，也不会代理到业务上游。

## SDK 边界

`agentlink` 是 mock-gateway 与 `shield-x-client-go` 之间唯一的适配层。策略求值、模板渲染、流量生成和 HTTP 响应构造仍是 mock 的独立实现。

这样既保留 SDK E2E 能力，又避免复制 SDK 内部实现。

## 信任边界

- 控制台和数据面默认只监听 loopback；
- 远程 UI 需要显式参数和 Token；
- 除非开启 `trust_proxy`，否则不信任 `X-Forwarded-For`；
- Token 是配置密钥，文档中不出现真实值；
- mock 是测试工具，不是生产安全边界。
