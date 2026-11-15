# mock-gateway

Standalone gateway simulator for the **shield-x agent-v1 contract**.

It is a development data source, SDK E2E driver, load generator, and HTTP data-plane simulator for testing shield-x policies. It evaluates policies locally and returns mock responses; **it is not a production gateway and never proxies to a business upstream**.

详细设计见 [`DESIGN.md`](DESIGN.md)。

---

## 安装

```bash
go install github.com/linyu19872008/shield-x-mock-gateway/cmd/mock-gateway@v0.1.0
```

或从源码构建：

```bash
git clone https://github.com/linyu19872008/shield-x-mock-gateway.git
cd shield-x-mock-gateway
go build -o bin/mock-gateway ./cmd/mock-gateway
```

要求 Go 1.26+。

---

## 为什么存在

shield-x 的开发不该等 gateway 改造。mock-gateway 提供四件事：

| 用途 | 说明 |
|---|---|
| 开发数据源 | 稳定产出策略与事件流，前后端不用等真实网关 |
| **SDK E2E 驱动** | agent 侧统一经由 `shield-x-client-go`，验证 SDK 的轮询/上报/补发 |
| 压测发生器 | 验证 3000 events/s 下 shield-x 的容量 |
| 数据面模拟器 | 接收真实 HTTP 请求，按策略返回 mock 响应并上报真实事件；不代理业务系统 |

> **它刻意不 import shield-x 的 `internal/*`。** 求值、渲染、流量生成独立实现；
> 与 shield-x 的 agent 交互统一经由 `shield-x-client-go`。协议独立性由
> `template-conformance`、client-go 契约夹具与 `probe` 共同承担。

---

## 五个子命令

```bash
# 1) 模板渲染一致性（不依赖 shield-x，纯本地）
./bin/mock-gateway conformance --vectors ./testdata/template-conformance/cases.json

# 2) 控制台：HTTP API + 可启停运行（默认 127.0.0.1:18080）
./bin/mock-gateway serve --config ./config.example.yaml

# 3) 数据面：接收真实请求，本地求值后返回 mock 响应
./bin/mock-gateway gateway --config ./config.example.yaml --listen 127.0.0.1:18081

# 4) 常驻：轮询策略 + 生成并上报事件
./bin/mock-gateway run --config ./config.example.yaml

# 5) 压测：固定速率 + 时长，输出 JSON 报告
./bin/mock-gateway loadtest --config ./config.yaml --rate 3000 --duration 10m
```

### serve（控制台 API）

默认只监听 `127.0.0.1`。远程访问必须同时提供 `--allow-remote --ui-token`，
请求带 `X-Mock-Token` 或 `Authorization: Bearer`。

| 方法 | 路径 | 作用 |
|---|---|---|
| GET | `/healthz` | 健康检查 |
| GET | `/api/status` | 运行状态、队列/上报/轮询统计 |
| POST | `/api/start` | `{"mode":"run\|loadtest","rate":50,"duration":"5s"}` |
| POST | `/api/stop` | 停止并等待 flush |
| GET | `/api/logs?limit=200` | 环形日志 |
| GET | `/api/config` | 脱敏后的配置视图 |
| POST | `/api/fault` | `{"down":true}` 切换 shield-x 不可达 |
| GET | `/api/policies` | 当前策略快照 |
| GET | `/api/templates` | 当前模板缓存 |

启动后打开 `http://127.0.0.1:18080/` 使用内嵌 Web UI：

- 页面：概览 / 连接与能力 / 策略与模板 / 流量生成 / 故障注入 / 上报与 Spool / 日志与配置
- 明暗主题跟随系统，侧栏可手动切换；`?theme=dark` 强制深色
- 配置了 `--ui-token` 时，首次 API 请求会弹出 Token 输入并保存在浏览器本地
- UI 是原生 HTML/CSS/JS，`go:embed` 进二进制，不需要 npm 或构建步骤

### gateway（数据面模拟）

`gateway` 接收真实 HTTP 请求，用本地策略快照求值，然后直接返回 mock 响应；**不会代理、也不会访问业务系统**。

```bash
./bin/mock-gateway gateway \
  --config ./config.example.yaml \
  --listen 127.0.0.1:18081 \
  --gateway-id gw-dataplane-check
```

验证 302：

```bash
curl -sS -D- -o /dev/null \
  -A 'AhrefsBot/7.0' \
  http://127.0.0.1:18081/crawler-test

# HTTP/1.1 302 Found
# Location: https://www.baidu.com
# Cache-Control: no-store
```

响应规则：

| 策略 | mock 响应 |
|---|---|
| 未命中 / observe | 配置的 `ingress.mock_status` + `mock_body`（默认 200） |
| block | 渲染拦截模板；默认 403；带 `X-Shieldx-Request-Id` |
| redirect | `301/302/307` + `Location`；默认 302；无 body |
| javascript_challenge | 简单 mock 挑战页（不是真实 JS 校验） |
| 策略求值异常 | 503 + `decision_source=gateway_error` |

真实事件仍通过 `shield-x-client-go` 批量上报；`gateway_id` 建议用独立值，避免和控制台的 `gw-mock-01` 混数据。

> 默认只监听 loopback；远程监听必须显式 `--allow-remote`。默认不信任 `X-Forwarded-For`，只有 `ingress.trust_proxy: true` 才使用它。

### conformance

逐条跑 `testdata/template-conformance/cases.json` 的向量，退出码 `0` 表示全通过。

```
  PASS  01-simple-replace
  PASS  02-multi-placeholders
  ...
结果：14/14 通过
全部通过 → 可声明 block_templates: true
```

**任何语言的 gateway 实现都可以用同一份向量自测**，见 [向量 README](testdata/template-conformance/README.md)。

### run

```
[00:48:54] 启动 mock-gateway：base=http://127.0.0.1:8080/shieldx gateway_id=gw-mock-01 rate=20/s workers=4 format=json
[00:48:54] 策略已更新：revision=1 policies=3
[00:48:54] 模板已更新：revision=1 templates=3
[run] gen=200 queue=3 sent=1 accepted=200 rejected=0 dropped=0 spool=0 | policy_rev=1 policies=3 templates=3 | poll ok=2 304=1 fail=0
```

每 10 秒打一行状态。`Ctrl-C` 退出时会 flush 剩余事件。

### loadtest

```json
{
  "target_rate": 3000,
  "achieved_rate": 2993.65,
  "generated": 14970,
  "accepted": 14970,
  "dropped_client": 0,
  "http_errors": 0,
  "upload_latency_ms": { "p50": 12, "p95": 38, "p99": 61, "max": 320 },
  "poll": { "count": 3, "p99_ms": 8, "ratio_304": 0.67, "failed": 0 },
  "notes": []
}
```

> `achieved_rate` 明显低于 `target_rate` 时，报告会提示：
> **先用 `--dry-run` 复测 mock 自身**，否则分不清是 mock 瓶颈还是 shield-x 瓶颈。

---

## 压测前必做：自检 mock 自身

```bash
./bin/mock-gateway loadtest --dry-run --rate 3000 --duration 10s
```

`--dry-run` 只生成不上报。达成率 ≥ 99% 才说明 mock 不是瓶颈。

**这一步不做，后面所有压测结论都不可信。**

---

## 配置

复制 [`config.example.yaml`](config.example.yaml)。核心字段：

| 字段 | 说明 |
|---|---|
| `shieldx.base_url` | **要包含 `/shieldx` 前缀**，接口拼在其后 |
| `shieldx.token` | 租户 Token |
| `shieldx.capabilities` | 能力声明，服务端据此过滤策略 |
| `poll.interval` | 拉策略周期（默认 5s） |
| `report.format` | `json` 或 `ndjson` |
| `report.spool_dir` | 上报失败时的落盘目录 |
| `generate.rate` | 事件速率 |
| `generate.ramp` | 速率爬坡台阶 |
| `generate.*_ratio` | 异常事件注入比例 |
| `fault.*` | 故障注入（断网 / 限流 / 延迟 / 丢包） |

### 异常注入（默认就开）

| 配置 | 产生的事件 | 验证什么 |
|---|---|---|
| `bad_event_ratio` | `client_ip` 非法 | 服务端 `rejected` 统计 |
| `gateway_error_ratio` | `decision_source=gateway_error` | **防护失效告警** |
| `timeout_ratio` | `status=504` | 异常路径上报 |

这三种是真实网关一定会有、但最容易忘记造的事件。

### 故障演练

```bash
# shield-x 不可达 5 分钟，观察 spool 增长与恢复后补发
./bin/mock-gateway run --config config.yaml   # 配 fault.shieldx_down: true
```

---

## 行为要点（对应接入契约）

| 契约要求 | 实现 |
|---|---|
| 全量快照整体替换 | `policyState.apply` 原子替换，**不增量合并** |
| 空列表清空本地策略 | 空列表同样 apply + 落盘 |
| 失败保留旧快照 | SDK poller 不覆盖；mock 状态只在 200 时更新 |
| 快照落盘 | mock 自己原子写（tmp → rename），重启先读磁盘，再把 revision 交给 SDK `InitialRevision` |
| 事件幂等 | `event_id = gateway_id-seq`，补发可重复 |
| 上报 202 即成功 | 其余按 413/429/400/5xx 由 SDK reporter 处理 |
| 失败落盘补发 | SDK reporter + `report.spool_dir`，启动与恢复后自动补发 |

---

## 与真实 gateway 的差距

| 差距 | 后果 |
|---|---|
| 不转发真实请求 | 无法验证"拦截是否真的阻止了攻击" |
| 不解析真实 XFF | 真实 IP 逻辑未覆盖 |
| 无真实 TLS | 证书 / mTLS 未覆盖 |
| 性能数字仅代表 mock | 不代表 gateway 自身开销 |
| JS 挑战只模拟 | 不产生真实浏览器交互 |
| 不模拟中间件开销 | 测不到"加策略引擎对业务 RT 的影响" |

**这些只能由真实 gateway 联调补齐。mock 的定位是"让 shield-x 不必等"，不是替代联调。**

---

## 开发

```bash
# 从仓库根目录
make mock-build      # 编译到 bin/mock-gateway
go test ./...
go build ./...
```


## License

MIT. See [LICENSE](LICENSE), [NOTICE](NOTICE), and [THIRD_PARTY_NOTICES.md](THIRD_PARTY_NOTICES.md).

The `testdata/template-conformance` vectors and `docs/contract/gateway-contract-v1.md` are derived from shield-x and remain under Apache-2.0; see the notices.
