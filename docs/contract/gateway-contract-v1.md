<!--
Derived from the shield-x repository (docs/gateway-contract-v1.md).
Copyright 2026 linyu19872008. Licensed under Apache-2.0.
See ../../LICENSES/Apache-2.0.txt and ../../THIRD_PARTY_NOTICES.md.
-->

# shield-x × gateway 接入契约 v1

> Status: current · Audience: gateway team · Source of truth: yes · Last verified: 2026-11-15

> 版本：**v1.0** · 日期：2026-11-14 · 状态：待 gateway 团队评审
> 读者：gateway 开发团队
> 目的：定义 gateway 与 shield-x 之间的**全部交互**。本文档自包含，不需要先读 shield-x 架构文档。

---

## 0. 一页速览

**shield-x 是一个策略中心 + 观测平台，不碰你的流量。** 你的 gateway 只做三件事：

| 序号 | 动作 | 接口 | 频率 |
|---|---|---|---|
| 1 | 定时拉策略 | `GET /shieldx/policies/v1` | 1~30 秒 |
| 2 | 拉拦截模板（按需） | `GET /shieldx/templates/v1` | 策略引用到新模板时 |
| 3 | 批量上报事件 | `POST /shieldx/events/v1` | 数秒一次 |

**五条不可违背的硬性契约：**

1. **拉策略返回的是全量快照，必须整体替换本地策略，禁止增量合并。**
2. **`policies: []` + revision 变化 → 必须清空本地策略。** 空列表是合法状态，不是异常。
3. **所有网络交互必须在独立协程/线程，绝不阻塞请求主路径。**
4. **拉策略失败 → 继续用旧快照，永远不裸奔。**
5. **脱敏在 gateway 侧完成**：`Authorization` / `Cookie` 明文不得外发。

**术语约定**：本文中的 **gateway** 指任何实现本契约的网关（自研、OpenResty、Nginx 插件等），不特指某一项目或版本；`mock-gateway` 是 shield-x 内置的参考实现，不是 gateway 的部署形态。

---

## 1. 角色与边界

### 1.1 谁负责什么

| 职责 | shield-x | gateway |
|---|---|---|
| 策略定义与存储 | ✅ | — |
| 策略下发（快照） | ✅ 提供接口 | ✅ 拉取、校验、落盘、应用 |
| 策略求值 | — | ✅ **全部在本地** |
| 动作执行 | — | ✅ 全部在本地 |
| 拦截页渲染 | ✅ 提供模板 | ✅ **本地渲染** |
| 事件采集 | — | ✅ |
| 事件存储与统计 | ✅ | — |
| 可用性 | 挂了只影响"看"和"改" | 挂了影响业务 —— 所以 **gateway 不能依赖 shield-x 在线** |

**核心原则：shield-x 不在流量路径上。** 任何情况下，shield-x 不可达都不得影响 gateway 的正常转发。

### 1.2 时钟与时区

- 所有时间戳使用 **RFC3339 带时区偏移**，例：`2026-11-14T00:20:59.123+08:00`
- 分桶统计以 **shield-x 接收时间** 为准（避免网关时钟漂移污染图表），但 gateway 应上报 `clock_skew_ms` 供排查
- 建议 gateway 启用 NTP

---

## 2. 认证与传输

### 2.1 认证

```http
Authorization: Bearer sx_EXAMPLE_TOKEN
```

- Token 是**租户级**的，一个租户下所有 gateway 共用（可配置多个 Token 用于轮替）。
- Token 由 shield-x 管理后台生成，**只在创建时展示一次**。
- Token 是**明文 Bearer 凭据**，泄露相当于整个租户失守——请按你方的部署安全要求保管。
- 认证失败返回 `401`，Token 被吊销 / 租户被禁用返回 `403`。

### 2.2 传输

| 项 | 要求 |
|---|---|
| 压缩 | 请求体支持 `Content-Encoding: gzip`（事件上报强烈建议开启） |
| 超时 | 拉策略 **≤ 3s**；上报事件 **≤ 10s** |
| 重试 | 指数退避，起始 1s，上限 60s，加 ±20% 抖动 |
| 连接复用 | 必须复用 HTTP 长连接（Keep-Alive），不要每次新建 |

> **传输层不属于本契约范围**（监听协议、端口、证书、反向代理均由部署方决定）。契约只约束应用层：路径、Header、报文格式与语义。

### 2.3 请求头一览

| Header | 必填 | 适用接口 | 说明 |
|---|---|---|---|
| `Authorization` | ✅ | 全部 | `Bearer <token>` |
| `X-Shieldx-Gateway-Id` | ✅ | policies / templates | 网关标识，见 §3.1 |
| `X-Shieldx-Agent-Version` | 建议 | 全部 | 如 `gateway/1.4.2`，用于兼容判断 |
| `X-Shieldx-Capabilities` | ✅ | policies | 能力声明，见 §8 |
| `If-None-Match` | 建议 | policies / templates | 传上次的 `revision` 以拿 304 |
| `Content-Encoding` | 建议 | events | `gzip` |
| `Content-Type` | ✅ | events | `application/json` 或 `application/x-ndjson` |

---

## 3. 网关标识

### 3.1 `gateway_id`

**格式：** 1~64 字符，`[A-Za-z0-9._-]`，租户内唯一。

**来源：** 由 gateway 自行决定，建议从配置文件或主机名派生，例如 `gw-prod-sh-01`。

**为什么是自报：** shield-x 用租户级 Token 认证，无法区分具体是哪台网关，因此 `gateway_id` 由上报方声明。shield-x 会**自动发现**首次出现的 `gateway_id` 并登记，管理后台可改名、禁用。

**注意：** 同一个 `gateway_id` 必须稳定。频繁变化会导致数据分裂、策略作用域失效。

---

## 4. 接口一：拉策略

### 4.1 请求

```http
GET /shieldx/policies/v1 HTTP/1.1
Host: shield-x.internal:8080
Authorization: Bearer sx_xxxxx
X-Shieldx-Gateway-Id: gw-prod-sh-01
X-Shieldx-Agent-Version: gateway/1.4.2
X-Shieldx-Capabilities: {"actions":["observe","block","redirect"],"match_fields":["ip","host","path","method","header"],"block_templates":true,"template_spec_version":1}
If-None-Match: "rev-38"
```

无 query 参数。租户由 Token 决定，**不得**在 URL 或 body 中传租户。

### 4.2 响应：有变更

```json
HTTP/1.1 200 OK
Content-Type: application/json
ETag: "rev-39"

{
  "revision": 39,
  "generated_at": "2026-11-14T00:20:59+08:00",
  "policy_count": 2,
  "filtered_out": 0,
  "policies": [
    {
      "id": "pol_01JF7X2K8QW3N5R9T4V6Y0B1D",
      "version": 3,
      "name": "封禁恶意爬虫 IP 段",
      "priority": 1000,
      "match": {
        "all": [
          { "field": "ip", "op": "in_cidr", "value": ["203.0.113.0/24"] }
        ]
      },
      "action": { "type": "block", "params": { "template_id": "tpl_01JF7X2K8QW3N5R9T4V6Y0B1E" } }
    },
    {
      "id": "pol_01JF7X2K8QW3N5R9T4V6Y0B1F",
      "version": 1,
      "name": "sqlmap UA 观察",
      "priority": 500,
      "match": {
        "all": [
          { "field": "ua", "op": "contains", "value": "sqlmap" }
        ]
      },
      "action": { "type": "observe" }
    }
  ]
}
```

### 4.3 响应：无变更

```http
HTTP/1.1 304 Not Modified
ETag: "rev-39"
```

**无响应体。** 这是常态——秒级轮询下 99% 的请求应该是 304。

### 4.4 响应：零策略（合法状态）

```json
HTTP/1.1 200 OK
ETag: "rev-40"

{
  "revision": 40,
  "generated_at": "2026-11-14T00:21:03+08:00",
  "policy_count": 0,
  "filtered_out": 0,
  "policies": []
}
```

> ⚠️ **这是合法响应，不是错误。** 表示租户当前没有任何生效策略，gateway 必须**清空本地策略集**。

### 4.5 六条强制语义

| # | 语义 | 说明 |
|---|---|---|
| 1 | **全量快照** | 返回的是"当前所有生效策略"的完整列表，不是增量 diff |
| 2 | **整体替换** | 拿到新 revision 后**整体替换**本地快照。**禁止增量合并、禁止按 id 逐条更新** |
| 3 | **空列表生效** | `revision` 变化 + `policies: []` → **必须清空**本地策略 |
| 4 | **失败保留旧快照** | 网络失败/超时/5xx → 继续用旧快照工作，不重试到成功为止 |
| 5 | **已排序** | 服务端已按 `priority` 降序 + `id` 升序排好，gateway 按数组顺序求值即可，**不要自己再排序** |
| 6 | **ETag 必用** | 下次请求带 `If-None-Match: "<上次的 revision>"`，只需比较 `revision` 数字 |

### 4.6 ⚠️ 禁止的四种实现（会直接导致线上事故）

```go
// ❌ 错误 1：空列表时保留旧策略 → 关闭策略永远不生效
if len(resp.Policies) == 0 { keepOldPolicies() }

// ❌ 错误 2：增量合并 → 删除/关闭的策略永远下不掉
for _, p := range resp.Policies { upsert(p) }

// ❌ 错误 3：自己排序 → 与 shield-x 的 tie-break 规则不一致，行为不一致
sort.Slice(policies, func(i, j int) bool { return policies[i].Priority > policies[j].Priority })

// ❌ 错误 4：拉取阻塞请求路径 → shield-x 慢，业务就慢
func handleRequest(w, r) { policies := fetchPoliciesSync(); ... }
```

**正确写法：**

```go
// ✅ 正确：revision 变了就整体原子替换，无论是否为空
if resp.Revision != local.Revision {
    local.Store(newSnapshot(resp.Revision, resp.Policies)) // atomic.Pointer
}
```

### 4.7 错误码

| 状态码 | 含义 | gateway 应做什么 |
|---|---|---|
| `400` | 请求参数非法 | **不要重试**，记录日志告警（多半是客户端 bug） |
| `401` | Token 缺失/无效 | 告警，按退避重试（可能是配置错误） |
| `403` | Token 已吊销 / 租户已禁用 | 告警，按退避重试，继续用旧快照 |
| `429` | 请求过于频繁 | 读 `Retry-After` 退避 |
| `5xx` | 服务端异常 | 指数退避重试，继续用旧快照 |
| 超时/网络错误 | — | 指数退避重试，继续用旧快照 |

---

## 5. 接口二：拉拦截模板

### 5.1 什么时候需要拉

策略包里的 `action.params.template_id` 指向一个模板。gateway 本地缓存按 `(id, version)` 维护。

**只有当策略引用了本地不存在的 `template_id` 时才需要拉取。** 不要每次轮询都拉模板。

### 5.2 请求

```http
GET /shieldx/templates/v1 HTTP/1.1
Authorization: Bearer sx_xxxxx
X-Shieldx-Gateway-Id: gw-prod-sh-01
If-None-Match: "rev-38"
```

### 5.3 响应

```json
HTTP/1.1 200 OK
ETag: "rev-39"

{
  "revision": 39,
  "templates": [
    {
      "id": "tpl_01JF7X2K8QW3N5R9T4V6Y0B1E",
      "name": "默认 403 拦截页",
      "version": 2,
      "checksum": "sha256:9f86d081884c7d659a2feaa0c55ad015a3bf4f1b2b0b822cd15d6c15b0f00a08",
      "content_type": "text/html; charset=utf-8",
      "status_code": 403,
      "headers": {
        "X-Frame-Options": "DENY",
        "X-Content-Type-Options": "nosniff"
      },
      "content": "<!doctype html><html lang=\"zh-CN\"><head><meta charset=\"utf-8\"><title>访问被拦截</title><style>body{font-family:-apple-system,\"PingFang SC\",sans-serif;display:grid;place-items:center;height:100vh;margin:0;background:#f6f7f9;color:#1f2329}</style></head><body><main style=\"text-align:center\"><h1 style=\"font-size:20px\">访问被拦截</h1><p style=\"color:#646a73\">请求编号：{{request_id}}</p><p style=\"color:#646a73\">如有疑问请联系 {{contact_email}}</p></main></body></html>",
      "vars": {
        "site_name": "示例站点",
        "contact_email": "security@example.com"
      }
    }
  ]
}
```

**注意：模板与策略共享同一个 `revision`。** 所以策略接口拿到 `revision: 39`，模板接口也应该是 `39`。这样可以避免两套版本对齐问题。

### 5.4 模板缓存策略

| 情况 | 行为 |
|---|---|
| 拉取失败 | 使用本地缓存的旧版本 |
| 本地也没有 | **使用 gateway 二进制内置的默认页**（见 §7.4） |
| checksum 校验失败 | 丢弃，使用本地缓存或内置默认页 |

---

## 6. 接口三：上报事件

### 6.1 请求

```http
POST /shieldx/events/v1 HTTP/1.1
Host: shield-x.internal:8080
Authorization: Bearer sx_xxxxx
Content-Type: application/json
Content-Encoding: gzip

{ "agent_version": "...", "gateway_id": "...", "config_revision": 39,
  "policy_count": 2, "events": [ ... ] }
```

### 6.2 两种合法请求体格式

**格式 A：JSON 数组（默认）**

```json
{
  "agent_version": "gateway/1.4.2",
  "gateway_id": "gw-prod-sh-01",
  "config_revision": 39,
  "policy_count": 2,
  "client_stats": { "queue_depth": 0, "dropped_total": 0, "spool_size": 0 },
  "events": [
    { "event_id": "gw01-0000123456", "...": "见 §6.3" }
  ]
}
```

**格式 B：NDJSON（推荐，`Content-Type: application/x-ndjson`）**

```text
{"agent_version":"gateway/1.4.2","gateway_id":"gw-prod-sh-01","config_revision":39,"policy_count":2}
{"event_id":"gw01-0000123456", "...":"事件字段"}
{"event_id":"gw01-0000123457", "...":"事件字段"}
```

- 第一行是批次元信息，后续每行一个事件
- 服务端逐行解析，**单条坏数据不会导致整批失败**

> 两种格式服务端都支持，gateway 任选其一。建议：日事件量大时用 NDJSON。

### 6.3 批次元信息字段

| 字段 | 类型 | 必填 | 说明 |
|---|---|---|---|
| `agent_version` | string | ✅ | gateway 版本，如 `gateway/1.4.2` |
| `gateway_id` | string | ✅ | 见 §3.1 |
| `config_revision` | int | ✅ | 产生这批事件时**本地策略集**的版本号 |
| `policy_count` | int | ✅ | 本地当前加载的策略条数（0 = 零策略状态） |
| `client_stats` | object | 建议 | 见下表，用于数据完整性监控 |
| `events` | array | ✅（格式 A） | 最多 1000 条或 1MB |

`client_stats`：

| 字段 | 说明 |
|---|---|
| `queue_depth` | 本地待上报队列深度 |
| `dropped_total` | 累计丢弃事件数（队列溢出/spool 满） |
| `spool_size` | 磁盘待补发数量 |

### 6.4 单个事件字段

| 字段 | 类型 | 必填 | 说明 |
|---|---|---|---|
| `event_id` | string(64) | ✅ | 网关内唯一，建议 `<gateway_id>-<单调递增序列>` |
| `ts` | string | ✅ | 请求开始时间，RFC3339 |
| `gateway_id` | string(64) | — | 顶层已有时可省略；混批时逐条指定 |
| `client_ip` | string | ✅ | 客户端 IP，v4 或 v6 字面量 |
| `real_ip_chain` | string[] | — | 代理链 IP，按顺序 |
| `host` | string(253) | ✅ | 请求 Host |
| `path` | string(2048) | ✅ | 路径，**不含 query** |
| `query` | string | — | 原始 query（建议脱敏后上报） |
| `method` | string(16) | ✅ | `GET` / `POST` … |
| `scheme` | string | — | `http` / `https` |
| `ua` | string(256) | — | User-Agent，**超长截断** |
| `referer` | string(1024) | — | |
| `status` | int | ✅ | 最终返回给客户端的状态码 |
| `upstream_status` | int\|null | — | 上游状态码；未转发上游时为 null |
| `duration_ms` | int | ✅ | 请求总耗时（毫秒） |
| `bytes` | int | — | 响应体字节数 |
| `matched_policy_id` | string\|null | ✅ | **命中的策略 ID；未命中必须为 `null`** |
| `matched_policy_priority` | int\|null | — | 命中策略的优先级 |
| `config_revision` | int | ✅ | 本条事件产生时的策略集版本（通常等于批次值） |
| `action` | enum | ✅ | `observe` / `block` / `redirect` / `javascript_challenge` / `none` |
| `action_result` | enum | ✅ | `executed` / `challenged` / `passed` / `failed` / `skipped` |
| `decision_source` | enum | ✅ | `policy` / `none` / `gateway_error` |
| `decision_latency_us` | int | — | 策略匹配耗时（微秒） |
| `block_template_id` | string\|null | — | 仅 `block` 动作 |
| `block_template_version` | int\|null | — | 仅 `block` 动作 |
| `headers_digest` | object | — | 头部摘要，**只允许放脱敏后的值或 hash** |
| `trace_id` | string(64) | — | 链路追踪 ID |
| `clock_skew_ms` | int | — | 网关与服务端时钟偏差估计 |

### 6.5 三态语义（最重要的一条）

`matched_policy_id` 和 `decision_source` 必须配合使用，**三种状态不能混**：

| `matched_policy_id` | `decision_source` | 含义 | 统计后果 |
|---|---|---|---|
| `"pol_xxx"` | `policy` | 命中策略并执行了动作 | 计入"命中" |
| `null` | `none` | **策略集正常加载（含空集），但未命中任何策略** → 正常放行 | 计入"通过" |
| `null` | `gateway_error` | **策略未加载成功 / 引擎异常** → 防护失效 | **计入"防护失效"，必须告警** |

> 如果 gateway 把后两种都填成 `none`，shield-x 的图表会显示"一切正常"，而实际上防护已经失效。这是本契约里最严重的实现错误。

### 6.6 `action_result` 取值语义

| 值 | 何时使用 |
|---|---|
| `executed` | 动作已执行（观察已记录 / 拦截页已返回 / 重定向已发出） |
| `challenged` | JS 挑战页已下发，等待客户端通过 |
| `passed` | JS 挑战通过（携带有效 cookie 的后续请求） |
| `failed` | **动作执行失败**（如模板缺失且内置页也失败、挑战生成异常） |
| `skipped` | 命中策略但动作被跳过（如灰度比例未命中） |

`failed` 必须单独告警——它意味着"该拦没拦住"。

### 6.7 响应

```json
HTTP/1.1 202 Accepted

{
  "accepted": 1000,
  "rejected": 0,
  "server_time": "2026-11-14T00:21:05+08:00"
}
```

- `202` = 已接收，**不代表已落库**。gateway 收到 202 即视为成功，可清理本地缓冲。
- `accepted` + `rejected` 应等于发送条数；不等时以返回值为准。
- `rejected` 通常是字段校验失败，**不要重发**（重发还是会被拒）；应记录日志排查。

### 6.8 限流与背压

| 状态码 | 含义 | gateway 应做什么 |
|---|---|---|
| `202` | 成功 | 正常清空本地缓冲 |
| `413` | 单批过大 | 拆小批次重发（不要原样重试） |
| `429` | 触发限流 | **读 `Retry-After` 严格退避**，本地持续缓冲；**严禁立刻重试** |
| `400` | 请求体非法 | 记录日志，丢弃该批（不要重试，避免无限循环） |
| `401`/`403` | 认证问题 | 告警 + 退避重试；Token 吊销需人工介入 |
| `5xx` | 服务端异常 | 指数退避重试，本地持续缓冲 |

**批量建议：**

| 参数 | 建议值 |
|---|---|
| 单批条数 | 200~500 条（上限 1000） |
| 单批大小 | ≤ 1 MB（压缩前） |
| 上报间隔 | 2~5 秒（或攒够批次立即发） |
| 本地缓冲上限 | 10 万条或 200 MB，超限丢弃最旧并计数 |

### 6.9 幂等

- `event_id` 在 `(租户, gateway_id)` 范围内必须唯一。
- 重发（网络超时后的补发）是**正常且被允许**的，服务端会去重。
- **不要为了"避免重复"而放弃补发**——丢数据比重复数据严重得多。

### 6.10 脱敏（硬要求）

以下内容**不得**上报明文：

| 内容 | 处理方式 |
|---|---|
| `Authorization` 头 | 完全丢弃 |
| `Cookie` / `Set-Cookie` | 完全丢弃，或只上报字段名的 hash |
| 请求 body | 默认不上报；如需上报只传长度和 hash |
| URL 中的敏感 query | 按配置脱敏（如 `token=***`） |
| `ua` | 可保留，但截断 256 字符 |

**脱敏必须在 gateway 侧完成。** 不要把明文发到 shield-x 再让它脱敏——那样 shield-x 就成了敏感数据汇聚点。

---

## 7. 动作执行规范

### 7.1 `observe`（观察）

- **完全放行**，不改变响应、不增加延迟。
- 必须上报事件，且 `matched_policy_id` 照常填写、`action = observe`、`action_result = executed`。
- 用途：策略上线前验证影响面。

### 7.2 `block`（拦截）

**必须返回自定义响应体**（这是能力基线里的硬要求，也是拦截模板的前提）。

响应规范：

```http
HTTP/1.1 403 Forbidden
Content-Type: text/html; charset=utf-8
Cache-Control: no-store
X-Shieldx-Request-Id: <本次请求唯一 ID>
Content-Security-Policy: default-src 'none'; style-src 'unsafe-inline'; img-src data:
X-Frame-Options: DENY
X-Content-Type-Options: nosniff
```

- 状态码默认 403；若策略 `params.status_code` 指定则以其为准。
- 模板里 `{{request_id}}` 的值**必须**等于响应头 `X-Shieldx-Request-Id`。
- 模板中声明的自定义响应头（`headers` 字段）一并附加。

### 7.3 `redirect`（重定向）

```http
HTTP/1.1 302 Found
Location: <params.redirect_url>
Cache-Control: no-store
```

状态码支持 301 / 302 / 307，默认 302。

### 7.4 `javascript_challenge`（JS 挑战）

**流程：**

1. 返回挑战页（含 JS），状态码 200 或 403（可配置）
2. 客户端 JS 计算 proof，POST/GET 到 gateway 的验证端点
3. 验证通过 → gateway 种 cookie（带签名，含过期时间）
4. 后续请求携带有效 cookie → 直接放行

**Gateway 侧必须实现：**

- 挑战页 HTML（可作为内置资源，或由 shield-x 下发）
- cookie 签名密钥（本地生成或配置，**不需要 shield-x 参与**）
- cookie 校验逻辑
- 挑战有效期（由策略 `params.challenge_ttl_s` 控制）

**必须处理的情况：**

| 情况 | 行为 |
|---|---|
| 客户端无 JS 能力（curl/爬虫） | 自然无法通过 → 等效拦截 |
| 搜索引擎爬虫 | 按 UA 白名单直接放行（默认放行主流搜索引擎） |
| 挑战通过后 | 上报 `action_result = passed` |
| 挑战不下发 | 上报 `action_result = failed` |

**统计口径：** 一次完整流程产生两条事件——`challenged` 和 `passed`。挑战通过率 = `passed / challenged`。

### 7.5 兜底：内置默认拦截页（必须实现）

**gateway 二进制必须内置一个最小可用的拦截页**，用于：

- 模板拉取失败且本地无缓存
- 模板 checksum 校验失败
- 模板渲染异常

内置页要求：纯 HTML、无外部依赖、含 request_id、响应头同上。

> 不能出现"因为模板没拉到，拦截动作返回白屏或 500"的情况——**那比不拦截更糟**。

---

## 8. 能力协商

### 8.1 声明方式

拉策略时通过 `X-Shieldx-Capabilities` 上报：

```json
{
  "actions": ["observe", "block", "redirect", "javascript_challenge"],
  "match_fields": ["ip", "host", "path", "method", "header", "ua", "query", "cookie"],
  "block_templates": true,
  "template_spec_version": 1,
  "max_rules": 5000,
  "supports_rate_limit": false
}
```

| 字段 | 说明 |
|---|---|
| `actions` | 支持的动作类型 |
| `match_fields` | 支持的匹配字段 |
| `block_templates` | 是否支持自定义拦截模板 |
| `template_spec_version` | 支持的模板渲染规范版本（见 §9） |
| `max_rules` | 可承载的最大策略条数 |
| `supports_rate_limit` | 是否支持限速动作 |

### 8.2 服务端行为

- **不兼容的策略不会下发给该网关**（例如 gateway 不支持 `javascript_challenge`，则这类策略不下发）。
- 未下发的数量通过 `filtered_out` 返回。
- 管理后台会显示"该策略在 3 台网关上不可用（不支持 javascript_challenge）"。

### 8.3 未声明 capabilities 时

按最小能力集处理：`actions = ["observe"]`、`block_templates = false`。

> 这是为了防止"以为支持其实不支持"导致的假拦截。**请务必声明。**

---

## 9. 拦截模板渲染规范 v1

模板渲染必须**与语言无关**，任何语言的 gateway 实现都必须产出**完全一致**的输出。

### 9.1 占位符语法

```text
{{ name }}
```

- 允许花括号内外有任意空白
- `name` 必须完全匹配白名单（区分大小写）
- **不解析表达式、条件、循环、注释**

### 9.2 白名单

| 占位符 | 值来源 |
|---|---|
| `{{request_id}}` | gateway 生成的本次请求唯一 ID |
| `{{timestamp}}` | 拦截时间，RFC3339 |
| `{{client_ip}}` | 客户端 IP |
| `{{host}}` | 请求 Host |
| `{{path}}` | 请求路径 |
| `{{site_name}}` | 模板 `vars.site_name` |
| `{{contact_email}}` | 模板 `vars.contact_email` |

### 9.3 替换规则（严格）

1. 扫描模板，匹配 `\{\{\s*([a-z_]+)\s*\}\}`（**只匹配小写字母和下划线**）。
2. 如果捕获组是白名单内的 key → 替换为变量值（**先做 HTML 转义**）。
3. 如果不在白名单 → **原样保留整段 `{{ xxx }}`，不做任何替换**。
4. 变量值为 `null` 或缺失 → 替换为空字符串。

### 9.4 HTML 转义表（顺序很重要）

**必须先替换 `&`，否则会二次转义：**

| 原字符 | 转义为 |
|---|---|
| `&` | `&amp;` |
| `<` | `&lt;` |
| `>` | `&gt;` |
| `"` | `&quot;` |
| `'` | `&#39;` |

### 9.5 边界条件

| 条件 | 行为 |
|---|---|
| 模板 ≤ 64 KB（UTF-8） | 正常处理 |
| 模板 > 64 KB | 拒绝渲染，使用兜底页 |
| 带 UTF-8 BOM | **去除 BOM** 后处理 |
| CRLF 换行 | 原样保留，不做转换 |
| CSS 中的 `{` `}` | **不得误判**——只有形如 `{{ key }}` 且 key 在白名单内才替换 |
| 模板含 `{{` 但无闭合 | 原样保留 |

### 9.6 一致性验收

本仓库提供 `testdata/template-conformance/cases.json`（共 14 条），内含 JSON 测试向量：

```json
{
  "name": "特殊字符转义",
  "template": "<p>{{host}}</p>",
  "vars": { "host": "a<b>&\"c\"", "request_id": "r1" },
  "expected": "<p>a&lt;b&gt;&amp;&quot;c&quot;</p>"
}
```

**`test` 命令：** 读取所有向量，逐条渲染并比对 `expected`。**全部通过**才可声明支持 `block_templates: true`。

---

## 10. 匹配语义

### 10.1 求值顺序

1. 按 `priority` 降序（已由服务端排好序）
2. 同 `priority` 按 `id` 升序
3. **第一条命中的策略立即执行并停止**（first-match 短路）

### 10.2 条件求值

```
"match": { "all": [ cond1, cond2 ] }     // 全部满足（AND）
"match": { "any": [ cond1, cond2 ] }     // 任一满足（OR）
"match": { "not": { ... } }              // 取反
```

支持嵌套，但服务端保证：**最大深度 4 层、最多 20 个条件**。

### 10.3 字段与操作符

| 字段 | 支持的操作符 | 值类型 | 说明 |
|---|---|---|---|
| `ip` | `in_cidr` / `not_in_cidr` / `eq` | string / string[] | IPv4/IPv6；支持 CIDR |
| `host` | `eq` / `suffix` / `in` | string / string[] | 大小写不敏感 |
| `path` | `eq` / `prefix` / `suffix` / `contains` / `regex` | string | 大小写敏感；不含 query；`eq`/`prefix` 的值须以 `/` 开头 |
| `method` | `eq` / `in` | string / string[] | 大小写不敏感 |
| `header` | `eq` / `contains` / `prefix` / `regex` | string | **需额外 `name` 字段指定头名** |
| `ua` | `contains` / `prefix` / `regex` | string | 大小写不敏感 |
| `query` | `eq` / `contains` / `regex` | string | 需 `name` 指定参数名 |
| `cookie` | `eq` / `contains` / `regex` | string | 需 `name` 指定 cookie 名 |

`header` / `query` / `cookie` 的条件格式：

```json
{ "field": "header", "name": "X-Forwarded-For", "op": "contains", "value": "1.2.3" }
```

### 10.4 正则约束

- **必须使用 RE2 语义**（线性时间，无回溯）。**禁止使用 PCRE / backtracking 引擎**——一条灾难性回溯的正则可以把 gateway CPU 打满。
- 服务端保证正则长度 ≤ 256 字符，且已通过编译校验。
- `regex` 操作为**部分匹配**（等价于 `.*expr.*`），除非显式使用 `^` `$`。

### 10.5 特殊 IP

- `ip` 字段取**真实客户端 IP**（按 XFF 链解析后的最左非可信地址），不是 TCP 对端 IP。
- 真实 IP 解析规则由 gateway 配置决定，shield-x 不干预，但**必须在事件里上报 `real_ip_chain`** 以便排查。

---

## 11. 客户端职责清单

### 11.1 必须实现

| # | 要求 | 验收方式 |
|---|---|---|
| 1 | 拉策略与上报在独立协程/线程，不阻塞请求 | 压测 gateway，shield-x 停机时业务 RT 无变化 |
| 2 | 拉策略超时 ≤ 3s，失败指数退避 | 断网测试 |
| 3 | 策略快照**落盘持久化**（含 revision + checksum） | 重启后无需网络即可用旧策略 |
| 4 | 无磁盘快照时按配置 `fail_open`（默认）或 `fail_closed` | — |
| 5 | 上报失败落本地 spool，恢复后补发 | 断网 10 分钟后恢复，数据完整 |
| 6 | spool 溢出丢弃并计数，随 `client_stats` 上报 | — |
| 7 | **异常路径也上报**（超时、连接重置、panic） | 构造超时请求，检查是否有事件 |
| 8 | 脱敏：Authorization / Cookie 不出网 | 抓包检查 |
| 9 | 正确处理空策略集（清空本地） | 后台关闭全部策略，观察 gateway 行为 |
| 10 | 内置默认拦截页 | 断网后触发拦截，页面仍可用 |

### 11.2 禁止实现

| # | 禁止项 | 后果 |
|---|---|---|
| 1 | 空列表时保留旧策略 | 关闭策略永不生效 |
| 2 | 增量合并策略 | 删除/关闭策略永不生效 |
| 3 | 自己给策略排序 | 与 shield-x tie-break 不一致，行为不一致 |
| 4 | 同步拉策略（阻塞请求） | shield-x 慢拖垮业务 |
| 5 | 把 `null` + 未命中 和 `null` + 引擎异常 混为一谈 | 统计失真，防护失效被掩盖 |
| 6 | 上报明文 Cookie / Authorization | 安全事故 |
| 7 | 上报无限重试 | 打崩 shield-x |

---

## 12. 验收清单（联调用）

联调时逐条打勾，**全绿才算接入完成**：

- [ ] **A1** 能拿到策略，`revision` 正确记录
- [ ] **A2** 无变更时返回 `304`，gateway 不重复应用
- [ ] **A3** 后台新增策略 → 一轮轮询内生效
- [ ] **A4** 后台**关闭全部策略** → gateway 清空本地策略（观察模式下仍正常转发）
- [ ] **A5** 后台关闭**单条**策略 → 该策略不再命中
- [ ] **A6** 断网 → gateway 继续用旧策略，业务不受影响
- [ ] **A7** 重启 gateway → 先用磁盘快照恢复，再拉最新
- [ ] **B1** `observe` 策略命中 → 事件带 `matched_policy_id`，响应不变
- [ ] **B2** `block` 策略命中 → 返回模板页，状态码 403，带 `X-Shieldx-Request-Id`
- [ ] **B3** 模板删除/拉取失败 → 回退内置默认页，事件 `action_result = failed`
- [ ] **B4** `redirect` 策略命中 → 302 到指定地址
- [ ] **B5** `javascript_challenge` → 浏览器能通过，curl 不能通过
- [ ] **C1** 事件能批量上报，字段完整
- [ ] **C2** 重复发送同一批 → shield-x 去重，统计不重复
- [ ] **C3** 断网积压 10 分钟 → 恢复后数据完整补发
- [ ] **C4** 未命中策略的事件 → `matched_policy_id = null`、`decision_source = none`
- [ ] **C5** 策略引擎异常时 → `decision_source = gateway_error`（**必须能构造出这个场景**）
- [ ] **C6** 超大批次 → `413`，拆批重发成功
- [ ] **D1** 能力不兼容的策略（如不声明 `javascript_challenge`）→ `filtered_out > 0`
- [ ] **D2** 模板渲染结果与 `template-conformance` 测试向量**全量一致**
- [ ] **E1** 跨租户隔离：租户 A 的 Token 拿不到租户 B 的策略

---

## 13. 版本与兼容

| 项 | 规则 |
|---|---|
| 接口版本 | URL 中固定 `/v1`；不兼容变更走 `/v2` |
| 客户端版本 | 上报 `X-Shieldx-Agent-Version`；服务端可据此拒绝过低版本 |
| 模板规范版本 | `template_spec_version`，新增占位符时升版本 |
| 向后兼容 | 服务端只**新增**可选字段，不修改已有字段语义 |

---

## 14. 联调支持

| 事项 | 说明 |
|---|---|
| shield-x 测试环境 | 提供固定 Token + 独立租户 |
| mock gateway | shield-x 仓库提供，可参考其实现（它是最小合法客户端） |
| 测试向量 | `testdata/template-conformance/cases.json`（14 条）+ `README.md`（schema 与运行说明） |
| 问题反馈 | 附上 `X-Shieldx-Request-Id` 或 `event_id`，便于定位 |

---

## 附录 A：完整请求示例

> 示例按 shield-x 默认监听 `:8080` 编写。**实际地址与端口以你方部署为准**，把 `shield-x.internal:8080` 换成真实值即可。

**拉策略**

```bash
curl -sS -D- http://shield-x.internal:8080/shieldx/policies/v1 \
  -H "Authorization: Bearer $SHIELDX_TOKEN" \
  -H "X-Shieldx-Gateway-Id: gw-prod-sh-01" \
  -H "X-Shieldx-Agent-Version: gateway/1.4.2" \
  -H 'X-Shieldx-Capabilities: {"actions":["observe","block"],"match_fields":["ip","path","method"],"block_templates":true,"template_spec_version":1}' \
  -H 'If-None-Match: "rev-38"'
```

**上报事件（JSON）**

```bash
curl -sS http://shield-x.internal:8080/shieldx/events/v1 \
  -H "Authorization: Bearer $SHIELDX_TOKEN" \
  -H "Content-Type: application/json" \
  --data-binary @events.json
```

**上报事件（NDJSON + gzip）**

```bash
curl -sS http://shield-x.internal:8080/shieldx/events/v1 \
  -H "Authorization: Bearer $SHIELDX_TOKEN" \
  -H "Content-Type: application/x-ndjson" \
  -H "Content-Encoding: gzip" \
  --data-binary @events.ndjson.gz
```

---

## 附录 B：常见问题

**Q：gateway 需要多久拉一次？**
A：由 gateway 配置，单位秒。建议 5~30 秒。最小值 1 秒（服务端会拒绝更短的周期）。

**Q：shield-x 挂了会怎样？**
A：gateway 继续用磁盘快照上的旧策略正常工作，事件在本地缓冲。业务完全不受影响。恢复后自动补发。

**Q：策略条数上限是多少？**
A：取决于 gateway 声明的 `max_rules`。默认建议 1000 条以内，服务端会按能力过滤。

**Q：能不能只在需要的时候拉策略（比如每秒拉一次）？**
A：可以，但请确保 99% 的请求走 `304`，否则会给 shield-x 造成不必要的压力。

**Q：模板能不能由 gateway 自己写死，不用 shield-x 下发？**
A：可以（那就不声明 `block_templates`），但会失去后台可视化编辑能力。内置默认兜底页无论如何都要有。
