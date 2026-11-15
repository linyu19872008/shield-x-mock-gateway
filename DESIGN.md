# mock-gateway 详细设计

> Status: current tool design · Audience: contributors · Last verified: 2026-11-15

> 版本：**v1.0** · 日期：2026-11-14
> 配套文档：[gateway 接入契约 v1](docs/contract/gateway-contract-v1.md) · [template-conformance](testdata/template-conformance/README.md)
> 定位：shield-x 的**开发数据源 + 契约一致性测试客户端 + 压测发生器**
>
> **实现状态（2026-11-14）**：`conformance` / `run` / `loadtest` 已落地于 `./`；
> `conformance` 14/14 通过；`loadtest --dry-run --rate 3000` 达成率 ≈99%（10s 窗口 98.8%，3s 窗口 99.3%）。
> `verify`（契约自动化验收）按计划留到 M6。
>
> **2026-11-14 架构调整**：agent 侧（拉策略/模板、上报、spool 补发）已全部改走 `shield-x-client-go`（`agentlink/`）。
> 策略求值、模板渲染、流量生成仍由 mock 独立实现。协议独立性验证职责转移：由 client-go 契约夹具 + `probe` + `template-conformance` 承担；
> mock 不再宣称自己是"契约的独立协议实现"。

---

## 0. 为什么需要它

shield-x 的开发不该等 gateway 改造。mock-gateway 解决三件事：

| 用途 | 说明 |
|---|---|
| **开发数据源** | 给后端/前端/图表提供稳定的策略与事件流，不用等真实网关 |
| **SDK E2E 驱动** | 通过 `shield-x-client-go` 调用真实接口，验证 SDK 在完整链路（轮询/上报/补发）上的行为 |
| **压测发生器** | 验证 3000 events/s 下 shield-x 的真实表现（这是 P0 最大的未知数） |

### 0.1 ⭐ 核心设计原则：求值/渲染独立，agent I/O 统一走 SDK

- mock **不得 import shield-x 的 `internal/*` 包**。
- **策略求值、模板渲染、流量生成必须独立实现**：这是 `template-conformance` 能作为独立判据的前提。
- **与 shield-x 的所有 agent 交互必须经由 `shield-x-client-go`**（唯一入口 `agentlink/`），用于 E2E 验证 SDK 的轮询、上报、spool 补发。
- **协议独立性不再由 mock 单独承担**：改由 client-go 的契约夹具、`probe` 与 `template-conformance` 共同承担。

> 这条原则让 mock 的身份从"独立协议实现"变成"独立求值/渲染 + SDK 的真实消费者"。协议是否可被独立实现，仍由 conformance 向量与 Java/其他语言实现来证明。

### 0.2 它不是什么

| 不是 | 原因 |
|---|---|
| 不是真实网关 | 不转发请求、不影响任何真实流量 |
| 不代表 gateway 性能 | 它的 RT 数字没有参考价值 |
| 不能替代真实联调 | 真实 IP 解析、中间件开销、反向代理行为都测不到 |
| 不是生产工具 | 无认证加固、无高可用设计 |

---

## 1. 技术选型

| 项 | 选择 | 理由 |
|---|---|---|
| 语言 | **Go** | 与主项目同工具链、单二进制、高吞吐生成能力、易交叉编译 |
| 依赖 | **仅标准库** | `net/http`、`encoding/json`、`compress/gzip`、`flag`/`cobra`（选 `flag` 即可） |
| 配置 | **YAML**（唯一外部依赖 `gopkg.in/yaml.v3`）或纯 JSON | 压测 profile 用 YAML 更易读；若想零依赖则用 JSON |
| 输出 | stdout 结构化文本 + 可选 JSON 报告 | 不依赖日志框架 |

> 项目本身"不引入额外组件"的原则针对**产品运行时**；mock 是开发工具，允许 YAML 依赖。

---

## 2. 子命令

```
mock-gateway serve        # 控制台：HTTP API + 可启停的运行控制
mock-gateway gateway      # 数据面：接收真实请求，返回 mock 响应
mock-gateway run          # 常驻：轮询策略 + 持续上报（默认模式）
mock-gateway loadtest     # 压测：固定速率 + 时长，输出报告
mock-gateway conformance  # 跑模板渲染测试向量，exit code 表示结果
mock-gateway verify       # 契约自动化验收（依赖 console API，M6 交付）
```

| 子命令 | 交付里程碑 | 说明 |
|---|---|---|
| `conformance` | **M0** | 最高优先级：它验证渲染规范 |
| `run` | **M0** | 开发期主力 |
| `loadtest` | **M0** | 容量结论来源 |
| `serve` | **M6 前置** | 控制台 API + 内嵌 Web UI（阶段 5–6） |
| `gateway` | P0 增量 | 真实 HTTP 数据面：求值 + mock 响应 + 真实事件上报 |
| `verify` | M6 | 需要后台 API 就绪 |

---

## 3. 配置文件

`config.example.yaml`：

```yaml
shieldx:
  base_url: "http://127.0.0.1:8080"
  token: "sx_REPLACE_ME"
  gateway_id: "gw-mock-01"
  agent_version: "mock-gateway/0.1.0"
  capabilities:
    actions: ["observe", "block", "redirect", "javascript_challenge"]
    match_fields: ["ip", "host", "path", "method", "header", "ua", "query", "cookie"]
    block_templates: true
    template_spec_version: 1
    max_rules: 5000

poll:
  interval: 5s
  timeout: 3s
  persist_path: "./data/policy-snapshot.json"
  use_etag: true

template:
  fetch_on_missing: true
  timeout: 3s

report:
  interval: 2s
  batch_size: 300
  format: json              # json | ndjson
  gzip: true
  timeout: 10s
  spool_dir: "./data/spool"
  spool_max_mb: 200
  max_backoff: 60s
  jitter: 0.2

generate:
  rate: 200                 # events/sec
  workers: 4
  duration: 0               # 0 = 无限
  ramp: []                  # 见 §7.3

  paths:
    - { path: "/api/login",        method: "POST", weight: 30 }
    - { path: "/api/user/profile", method: "GET",  weight: 50 }
    - { path: "/api/order",        method: "POST", weight: 10 }
    - { path: "/admin",            method: "GET",  weight: 5  }
    - { path: "/.env",             method: "GET",  weight: 5  }   # 扫描器特征

  host_pool:
    - { host: "api.example.com", weight: 90 }
    - { host: "admin.example.com", weight: 10 }

  ua_pool:
    - { ua: "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 Chrome/120.0 Safari/537.36", weight: 60 }
    - { ua: "curl/8.4.0",          weight: 20 }
    - { ua: "sqlmap/1.7.2#stable", weight: 5  }   # 用于命中 UA 策略
    - { ua: "Googlebot/2.1",       weight: 10 }
    - { ua: "python-requests/2.31", weight: 5 }

  ip_pools:
    normal:    ["198.51.100.0/24", "203.0.113.0/24"]
    malicious: ["203.0.113.66"]                    # 用于命中 IP 策略
    internal:  ["10.0.0.0/8"]

  status_distribution:      # 生成的事件里 status 的分布（用于图表真实感）
    "200": 85
    "302": 5
    "404": 8
    "500": 2

  bad_event_ratio: 0.001    # 故意构造非法事件，验证 rejected 统计
  gateway_error_ratio: 0.0005  # 构造 decision_source=gateway_error
  timeout_ratio: 0.001      # 构造异常路径事件（status=504）

fault:
  shieldx_down: false       # 完全不通
  http_status_override: 0   # 强制返回某状态码（401/429/503）
  latency_ms: 0             # 人为延迟
  drop_percent: 0           # 随机丢弃请求
```

**CLI 参数可覆盖配置**（`--rate`、`--duration`、`--base-url`、`--token` 等），配置用于复杂 profile，flag 用于临时调试。

---

## 4. 架构与进程模型

```
┌──────────────────── mock-gateway ─────────────────────┐
│ Control API (HTTP) ──► Controller (start/stop/status) │
│                              │                        │
│                              ├── agentlink ──► shield-x
│                              │     ├─ SDK poller（策略/模板）
│                              │     └─ SDK reporter + spool（事件）
│                              │
│                              └── Generator ──► Evaluator/Renderer
│                                    （独立实现，不上报 SDK 内部）
└───────────────────────────────────────────────────────┘
```

**并发模型：**

| 组件 | goroutine | 说明 |
|---|---|---|
| Controller | 1 | 串行化 start/stop，保证同一时刻只有一个运行 |
| Control API | HTTP server | 状态、启停、日志、故障注入 |
| SDK poller | 1 | 拉策略/模板，304、退避、原子快照 |
| Generator | N（默认 4） | 令牌桶限速，生产事件 |
| SDK reporter | 1 | 批量上报、413/429、spool 补发 |

**关键：Generator 与 SDK reporter 之间用 SDK 的有界队列（默认 10000 条）；满了丢弃最旧并计数。**

### 4.1 控制台模式（serve）

`serve` 在 control API 之上内嵌一套零构建 Web UI（原生 HTML/CSS/JS，`go:embed` 进单二进制）：

| 页面 | 内容 |
|---|---|
| 概览 | 运行状态、生成/入队/发送/接收、队列与 Spool、轮询 200/304 |
| 连接与能力 | Base URL、Gateway ID、Token 状态、能力声明、静态故障配置 |
| 策略与模板 | 当前策略快照（可展开 JSON）、模板卡片与预览 |
| 流量生成 | run / loadtest、速率、时长、快捷预设、停止 |
| 故障注入 | 运行时「不可达」开关、静态延迟/丢包/状态码覆盖 |
| 上报与 Spool | 待补发事件/文件数/占用、补发策略说明 |
| 日志与配置 | 环形日志过滤、脱敏配置 JSON |

- 默认监听 `127.0.0.1:18080`；远程访问需 `--allow-remote --ui-token`。
- 静态资源不要求 Token；`/api/*` 在设置 `--ui-token` 后要求 `X-Mock-Token` 或 Bearer。
- 明暗主题跟随系统，也可在侧栏手动切换；`?theme=dark` 可强制。
- UI 只消费 control API，不直接接触 SDK 内部。
- 截图：[overview-light](docs/screenshots/overview-light.png) · [overview-dark](docs/screenshots/overview-dark.png) · [logs](docs/screenshots/logs.png)。

### 4.2 数据面模式（gateway）

```
真实客户端（curl / 浏览器 / 爬虫）
        │
        ▼
mock-gateway gateway :18081
  ├─ 策略快照（复用 Poller/TemplateCache）
  ├─ 独立 Evaluator / Renderer
  ├─ mock 响应：200 / 403 / 301-307 / challenge
  └─ 真实 Event → SDK reporter → shield-x
```

硬约束：

- **不代理任何请求到业务系统**；
- `observe` / 未命中返回配置的 mock 上游响应；
- `block` 复用模板/兜底页；
- `redirect` 返回 `Location`，不访问目标；
- 默认 loopback；不默认信任 XFF；
- 与 `serve`/`run`/`loadtest` 互不改写，新增命令独立运行。

---

## 5. 拉策略客户端（Poller）

### 5.1 状态机

```
        ┌─────────┐  读磁盘快照成功   ┌──────────┐
 启动 ──►│  INIT   │─────────────────►│ DEGRADED │  可用旧快照
        └────┬────┘                   └────┬─────┘
             │ 无磁盘快照                    │ 首次拉取成功
             │                              ▼
             │                        ┌──────────┐
             └───────────────────────►│  READY   │
                                      └──────────┘
```

- `READY`：有可用策略快照
- `DEGRADED`：只有磁盘旧快照，网络拉取持续失败（打印警告 + 记指标）
- 永远不进入"无策略却不知道"的状态

### 5.2 单次轮询流程

```
1. 构造 GET /shieldx/policies/v1
   headers: Authorization / X-Shieldx-Gateway-Id / X-Shieldx-Agent-Version
            / X-Shieldx-Capabilities / If-None-Match
2. 超时 3s
3. 分支：
   - 304            → 记录 poll_latency，结束
   - 200            → 解析 → 校验 → 整体替换 → 落盘 → 结束
   - 4xx（除 429）  → 记录错误，**不重试**，按退避继续下一轮
   - 429            → 读 Retry-After，调整下次间隔
   - 5xx / 网络错误 → 保留旧快照，指数退避（1s→60s，±20% 抖动）
```

### 5.3 快照应用规则（与契约 §4.5 一一对应）

| 规则 | 实现要点 |
|---|---|
| 全量替换 | `atomic.Pointer[Snapshot]` 整体 swap，不 merge |
| 空列表清空 | `policies: []` → 同样执行 swap（新快照为空） |
| 排序信任服务端 | **不重新排序**；若检测到顺序错误（priority 逆序），打印警告但不修正 |
| revision 单调 | 若新 revision < 旧 revision，打印警告但**仍然接受**（避免 DB 重建后永久卡住） |
| 失败保留 | 任何失败路径都不修改当前快照 |
| 落盘 | 成功后原子写：`xxx.tmp` → `rename` |

### 5.4 磁盘快照格式

```json
{
  "revision": 39,
  "fetched_at": "2026-11-14T00:30:00+08:00",
  "server": "http://127.0.0.1:8080",
  "policies": [ ... ]
}
```

启动时若 `server` 与当前配置不一致，**忽略该快照**（避免串环境）。

---

## 6. 模板客户端与渲染器

### 6.1 模板拉取

- 策略应用后扫描所有 `action.params.template_id`
- 与本地缓存比对，缺失的收集起来，**去抖 100ms** 后合并成一次请求
- 拉取失败 → 标记缺失，渲染时用内置默认页

### 6.2 渲染器（独立实现契约 §9）

```go
func Render(content string, vars map[string]string) (string, error)
```

实现要点：

| 规则 | 实现 |
|---|---|
| 匹配 | 正则 `\{\{\s*([a-z_]+)\s*\}\}` |
| 白名单 | 仅 7 个 key；不在白名单 → **原样保留整段** |
| 转义 | 先 `&` 后 `<` `>` `"` `'`（顺序不能错） |
| 缺失变量 | 空串 |
| 大小上限 | 64 KB，超限返回错误 |
| BOM | 检测并剔除 |
| CSS `{}` | 因为只匹配 `{{...}}` 且 key 限小写，天然不会误伤 |

**这份实现必须能通过 `testdata/template-conformance/` 全部向量**——这也是它存在的意义。

### 6.3 内置默认页（mock 自身也要有）

```html
<!doctype html><html><head><meta charset="utf-8"><title>Blocked</title></head>
<body><h1>访问被拦截</h1><p>请求编号：{{request_id}}</p></body></html>
```

用途：验证 gateway 同样需要兜底逻辑；同时当模板缺失时 mock 仍能产出 `block` 事件。

---

## 7. 事件生成器

### 7.1 生成流程

```
令牌桶（全局 rate）
   │
   ▼
worker × N
   │
   ├─ 1. 按权重选 path / method / host / ua / ip
   ├─ 2. 构造「模拟请求」对象
   ├─ 3. Evaluator 求值 → matched_policy / action
   ├─ 4. 按 action 决定 status / action_result / decision_source
   ├─ 5. 按概率注入异常（bad / gateway_error / timeout）
   └─ 6. 生成事件 → 入队
```

**关键：`matched_policy_id` 必须由真实求值得出，不能伪造。** 否则 mock 就不能验证 shield-x 的统计逻辑。

### 7.2 求值器（独立实现契约 §10）

| 能力 | 说明 |
|---|---|
| 字段读取 | `ip` / `host` / `path` / `method` / `header` / `ua` / `query` / `cookie` |
| 操作符 | `eq` / `prefix` / `suffix` / `contains` / `regex` / `in` / `in_cidr` / `not_in_cidr` |
| 组合 | `all` / `any` / `not` |
| 短路 | first-match（按数组顺序，不排序） |
| 正则 | Go 的 `regexp`（RE2 语义，天然符合要求） |

### 7.3 速率控制

令牌桶，支持 ramp：

```yaml
ramp:
  - { at: "0s",   rate: 100 }
  - { at: "60s",  rate: 1000 }
  - { at: "180s", rate: 3000 }
  - { at: "300s", rate: 0 }
```

- 实现方式：`time.Ticker` 每 10ms 补一批令牌
- 实际速率误差应 < 5%
- `loadtest` 结束后校验达成率

### 7.4 分布与真实感

**目的：让图表看起来像真的，而不是纯随机噪点。**

| 维度 | 做法 |
|---|---|
| Path | 按权重分布（登录/查询占大头，扫描路径少量） |
| IP | 正常段随机 + 少量恶意 IP（会命中策略） |
| UA | 浏览器为主，掺入 sqlmap/curl |
| status | 按 `status_distribution` 分布 |
| 时间 | 可加日内波动曲线（可选）：`rate = base * (1 + 0.3*sin(t))` |

### 7.5 异常路径注入（重要）

| 类型 | 比例配置 | 产生的事件 |
|---|---|---|
| 非法事件 | `bad_event_ratio` | `client_ip` 格式错 / 缺必填字段 → 验证 `rejected` 统计 |
| 引擎异常 | `gateway_error_ratio` | `matched_policy_id=null` + `decision_source=gateway_error` |
| 超时 | `timeout_ratio` | `status=504`，`upstream_status=null` |

**这三种是真实 gateway 一定会有、但开发者最容易忘记造的事件。** 没有它们，shield-x 的"防护失效告警"和"数据完整性"两块永远测不到。

---

## 8. 上报器

### 8.1 批量策略

| 参数 | 默认 | 说明 |
|---|---|---|
| `batch_size` | 300 | 攒够就发 |
| `interval` | 2s | 到点就发（不足一批也发） |
| `max_batch` | 1000 | 硬上限 |
| 队列容量 | 50000 | 满则丢最旧并计数 |

### 8.2 请求构造

- 格式：JSON 数组（默认）或 NDJSON
- 压缩：gzip（默认开）
- 超时：10s

### 8.3 响应处理

| 响应 | 动作 |
|---|---|
| `202` | 成功，清理批次 |
| `413` | 批次减半重发（递归拆分，最小 10 条） |
| `429` | 读 `Retry-After`，**整个批次回队列**，暂停上报 |
| `400` | 记日志，**丢弃批次**（不重试，避免毒丸） |
| `401/403` | 告警，批次落 spool，指数退避 |
| `5xx` / 网络错误 | 批次落 spool，指数退避 |

### 8.4 Spool（磁盘补发）

```
data/spool/
  └── 20261114T003000Z-000123.ndjson.gz
```

- 每批一个文件，原子写（`.tmp` → `rename`）
- 后台每 30s 扫描重放，成功后删除
- 总大小超过 `spool_max_mb` → 删最旧并计数（打印告警）
- **这是验证"断网恢复后数据完整"的唯一手段**

### 8.5 上报统计（每 10s 打印一行）

```
[report] sent=1200 ok=1200 err=0 rejected=2 dropped=0 spool=0 rate=200.0/s p99_upload=45ms
```

---

## 9. 压测模式（loadtest）

### 9.1 与 run 的区别

| 项 | run | loadtest |
|---|---|---|
| 速率 | 固定/ramp | 固定 + ramp |
| 结束条件 | 无限 | `duration` 或 ramp 结束 |
| 输出 | 持续日志 | **结束输出 JSON 报告** |
| 采样策略 | — | 可选：关闭服务端采样，测全量写入上限 |

### 9.2 报告格式

```json
{
  "started_at": "2026-11-14T00:30:00+08:00",
  "duration_s": 600,
  "target_rate": 3000,
  "achieved_rate": 2987.4,
  "generated": 1792440,
  "accepted": 1792340,
  "rejected": 100,
  "dropped_client": 0,
  "spool_peak_mb": 0,
  "http_errors": 0,
  "retry_count": 3,
  "latency_ms": { "p50": 12, "p95": 38, "p99": 61, "max": 320 },
  "poll": { "count": 120, "p99_ms": 8, "304_ratio": 0.992 },
  "note": "achieved_rate 应 ≥ 95% target，否则 mock 自身是瓶颈"
}
```

> `achieved_rate` 低于目标 95% 时，报告里要显式标注：**可能是 mock 自身瓶颈，而不是 shield-x 的问题**。压测结论必须能区分这两者，否则会误导容量决策。

### 9.3 压测前的自检（必做）

```bash
mock-gateway loadtest --rate 3000 --duration 10s --dry-run
```

`--dry-run` 只生成不发送，用来确认 **mock 自身**能产出 3000/s。这一步不做，后面所有压测结论都不可信。

### 9.4 配套观察项（写进"压测手册"）

压测期间需要在 shield-x 侧同步观察：

| 观察项 | 命令/位置 |
|---|---|
| 实际入库速率 | `SELECT COUNT(*) FROM request_event WHERE received_at > ...` |
| 分区大小 | `SELECT PARTITION_NAME, TABLE_ROWS, DATA_LENGTH FROM information_schema.PARTITIONS WHERE TABLE_NAME='request_event'` |
| 队列深度 / 丢弃 | `/shieldx/metrics` |
| MySQL 压力 | `SHOW GLOBAL STATUS LIKE 'Innodb_rows_inserted'`、`SHOW PROCESSLIST` |
| 慢查询 | `performance_schema` 或 `slow_query_log` |

---

## 10. 契约一致性模式（conformance）

```bash
mock-gateway conformance --vectors ./testdata/template-conformance/cases.json
```

流程：

```
1. 读取所有 .json 向量文件
2. 对每条：Render(template, vars) → 与 expected 比对
3. 输出：
   PASS  escape-special-chars
   PASS  unknown-placeholder-kept
   FAIL  css-braces-not-replaced
         expected: ...
         actual:   ...
4. 全部通过 → exit 0；否则 exit 1
```

**这是 mock 最有价值的功能**：它让"模板渲染规范"从文字变成了可执行断言。

### 10.1 测试向量清单（M0 就要齐）

| 用例 | 验证点 |
|---|---|
| `simple-replace` | 基础替换 |
| `multi-placeholders` | 多个占位符混排 |
| `escape-special-chars` | `<>&"'` 转义 |
| `escape-amp-order` | `&` 先转义（防二次转义） |
| `unknown-kept` | 未知占位符原样保留 |
| `missing-var-empty` | 变量缺失 → 空串 |
| `css-braces` | CSS 的 `{}` 不被误替换 |
| `nested-braces` | `{{{{x}}}}` 的确定性行为 |
| `unicode` | 中文/emoji |
| `crlf-preserved` | 换行保留 |
| `bom-stripped` | BOM 剔除 |
| `too-large` | 超 64KB 拒绝 |
| `whitespace-in-placeholder` | `{{  request_id  }}` 合法 |
| `uppercase-not-matched` | 占位符名只允许小写，`{{Request_ID}}` 原样保留 |

> 每个向量都是 `(template, vars, expected)` 三元组，语言无关，任何 gateway 实现都能直接用。
>
> **向量已落地**：[`template-conformance/cases.json`](testdata/template-conformance/cases.json)（14 条），
> 规范与运行说明见 [`template-conformance/README.md`](testdata/template-conformance/README.md)。

---

## 11. 契约自动化验收（verify，M6）

把 [接入契约 §12](docs/contract/gateway-contract-v1.md) 里可自动化的项变成代码。

```bash
mock-gateway verify --config verify.yaml
```

需要一份管理员会话（通过 console API 驱动后台：建租户、发 Token、改策略），因此**依赖 M5 完成**。

| 自动化项 | 做法 |
|---|---|
| AC-01 空策略 | 后台关闭全部策略 → 断言 mock 本地策略数 = 0 |
| AC-02 304 | 连续两次拉取 → 第二次应是 304 |
| AC-03 变更生效 | 后台启停策略 → 一轮内 mock 快照变化 |
| AC-06 能力过滤 | 改 capabilities 后拉取 → `filtered_out` 正确 |
| AC-07 排序 | 断言返回顺序 = `priority DESC, id ASC` |
| AC-20 幂等 | 同批发两次 → 统计不重复（需查 stats API） |
| AC-30 渲染一致性 | 复用 conformance |
| AC-43 吊销时效 | 吊销 Token → ≤500ms 内请求被拒 |

> 不能自动化的项（如"断网恢复后业务无影响"）保留人工验收。

---

## 12. 目录结构

```
mock-gateway standalone repository
├── cmd/mock-gateway/
│   ├── main.go             # 子命令分发
│   └── agentlink/
│       └── agentlink.go    # 唯一 agent I/O 适配层（client-go 的 poller/reporter/spool）
├── convert.go              # mock 模型 <-> client-go 模型
├── state.go                # 本地快照 + 磁盘恢复
├── config.example.yaml
├── config.go               # 配置加载 + flag 覆盖 + 校验
├── engine.go               # 流水线编排（agentlink + generator）
├── controller.go           # 可启停 Controller + 环形日志
├── control_api.go          # /api/status /start /stop /logs /config /policies /templates /fault
├── serve.go                # serve 子命令（默认 loopback）
├── webui.go                # go:embed + 静态资源与 API 组合
├── web/                    # 原生 HTML/CSS/JS（零构建）
│   ├── index.html
│   ├── app.css
│   └── app.js
├── evaluator.go            # 策略求值（独立实现）
├── renderer.go             # 模板渲染（独立实现）
├── templates.go            # 模板缓存 / 缺失检测 / 兜底页
├── generator.go            # 令牌桶 + worker + 事件构造
├── model.go                # 契约实体（与 SDK 解耦）
├── stats.go                # 生成侧统计
├── run.go / loadtest.go / conformance.go
├── client.go               # 故障注入 Transport
└── README.md
```

约 **3500 行**（含测试）。`agentlink` 约 340 行；求值/渲染/生成仍占主体。

---

## 13. 自测清单

mock 自己也要验收：

- [ ] `conformance` 全量通过（14 个向量）
- [ ] `loadtest --dry-run --rate 3000` 达成率 ≥ 99%
- [ ] `loadtest --rate 3000 --duration 600` 达成率 ≥ 95%，且 mock CPU < 60%
- [ ] 断网 10 分钟 → spool 增长 → 恢复后全部补发且无重复
- [ ] 后台关闭全部策略 → mock 在 1 个轮询周期内清空
- [ ] 快照落盘 → 重启 mock 无网络也能加载
- [ ] 服务端返回 429 → mock 按 Retry-After 退避，不产生重试风暴
- [ ] 服务端返回 413 → mock 拆批成功
- [ ] 生成 100 万事件无内存泄漏（±5% 稳态）
- [ ] 三种异常事件（bad / gateway_error / timeout）都能产出，且能被 shield-x 正确统计

---

## 14. 与真实 gateway 的差距（诚实清单）

写进 README，避免被误用：

| 差距 | 后果 |
|---|---|
| 数据面不转发真实请求 | `gateway` 可验证真实 HTTP 响应，但不会验证业务上游是否被保护 |
| 不解析真实 XFF | 真实 IP 逻辑正确性无法验证 |
| 不经真实反向代理 | 代理改写、超时、连接复用等行为未覆盖 |
| 性能数字无意义 | mock 的 RT 不代表 gateway 的 RT |
| JS 挑战只模拟流程 | 不产生真实浏览器交互 |
| 不模拟网关中间件开销 | 无法验证"加一层策略引擎对业务 RT 的影响" |
| 不测试并发连接上限 | 连接池/t backlog 未覆盖 |
| agent I/O 由 SDK 完成 | mock 不再独立验证线格式；该职责由 client-go 契约夹具与 `probe` 承担 |

**这些必须由 gateway 侧的真实联调补齐。** mock 的定位是"让 shield-x 不必等"，不是"替代联调"。

---

## 15. 使用示例

### 15.1 开发期（M0 起）

```bash
# 1. 起 mock
mock-gateway run --config config.example.yaml

# 2. 观察输出
[06:30:00 UTC] started, gateway_id=gw-mock-01
[06:30:00 UTC] snapshot loaded from disk revision=0 (empty)
[06:30:02 UTC] poll ok revision=1 policies=0
[06:30:02 UTC] poll ok revision=2 policies=3
[06:30:04 UTC] report sent=400 ok=400 err=0
```

### 15.2 渲染规范验证

```bash
mock-gateway conformance --vectors ./testdata/template-conformance/cases.json
# PASS 14/14
```

### 15.3 容量验证（M0 收口）

```bash
# 先自检 mock 自身能力
mock-gateway loadtest --rate 3000 --duration 10s --dry-run

# 正式压测
mock-gateway loadtest --rate 3000 --duration 600 --report ./out/cap-3000.json
```

### 15.4 回填历史数据（演示 / 补数据）

`run --backfill` 不再按真实时间发事件，而是把 `rate × backfill` 条事件的
**时间戳铺满过去这段时间**并尽快发完，用于造图表数据、复现历史问题。

```bash
# 1) 生成过去 1 小时、平均 15 条/秒的 54000 条事件（时间戳按 ramp 曲线分布）
mock-gateway run --config config.backfill.yaml --backfill 1h --rate 15

# 2) 必须补聚合，否则总览/图表读 stat_* 仍是 0
shield-x rollup --since 90m
```

约束与注意：

| 项 | 说明 |
|---|---|
| 时间戳范围 | 服务端允许 ts 落后 24 小时内（`event.Validate`），1h 安全 |
| 采样 | 正常流量默认采样 10%，演示前把租户 `sample_normal_rate` 设为 1 |
| 聚合窗口 | 后台循环只聚合最近 15 分钟，回填数据必须用 `shield-x rollup` 补 |
| IP 池 | 支持 CIDR；`normal` / `malicious` / `internal` 三池影响策略命中率 |
| 退出 | 回填模式发完自动退出并打印汇总，不需要 Ctrl-C |

### 15.5 故障演练

```bash
# 模拟 shield-x 不可达 5 分钟
mock-gateway run --fault.shieldx_down=true --fault.down_duration=5m
```

---

## 16. 交付定义（M0 Done 的标准）

| 项 | 标准 |
|---|---|
| `conformance` | 14/14 通过 |
| `run` | 连续运行 1 小时稳定，无内存增长 |
| `loadtest` | 3000/s 达成率 ≥ 95%，产出 JSON 报告 |
| 压测结论 | 给出"shield-x 在 3000/s 下能否撑住 / 撑多久 / 磁盘增长曲线" |
| 文档 | README 含使用说明 + §14 差距清单 |

**这个交付物完成之前，不要开始写业务功能。** 因为容量结论会反过来改 DDL 默认值和采样策略。
