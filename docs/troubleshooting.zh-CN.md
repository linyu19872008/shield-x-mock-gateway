# 故障排查

[English](troubleshooting.md) | **简体中文**

先看控制台状态和进程日志。多数问题来自配置、Token 或策略匹配，而不是 mock 本身。

## `401 INVALID_TOKEN`

```text
agent error: shield-x: HTTP 401 INVALID_TOKEN: Token 无效
```

检查：

1. `shieldx.token` 是真实站点 Token，不是 `sx_REPLACE_ME`；
2. `shieldx.base_url` 包含 `/shieldx`；
3. Token 没有被撤销或轮换；
4. Token 属于你期望的站点。

401 后 mock 可能仍使用持久化快照提供服务，但不会更新策略，也不会成功上报事件。

## 没有策略 / revision 不更新

- 检查控制台的 `/api/status` 和 `/api/policies`；
- 确认站点有启用中的策略；
- 确认 `capabilities.actions` 和 `capabilities.match_fields` 覆盖策略使用的动作/字段；
- 检查 `poll.interval`、网络和 TLS；
- 在控制台日志里搜索 `agent error`。

## 重定向策略不命中

按顺序检查：

1. 策略是否启用？
2. 优先级是否足够？first-match，高优先级的 block/observe 可能先命中；
3. Method 是否匹配？
4. UA/path/host/query 条件是否完全匹配？
5. 请求是否打到 `gateway`，而不是 `serve`？
6. 数据面端口是否正确（默认 `18081`）？
7. 网关是否拉到了最新 revision？

先用最小请求验证：

```bash
curl -sS -D- -o /dev/null \
  -A 'AhrefsBot/7.0' \
  http://127.0.0.1:18081/crawler-test
```

如果返回 200 + `mock upstream`，没有策略命中；如果 403，说明 block 策略先命中；如果 302，检查 `Location`。

## `conformance` 找不到向量

`go install` 的二进制不包含 `testdata/`。克隆仓库：

```bash
git clone https://github.com/linyu19872008/shield-x-mock-gateway.git
cd shield-x-mock-gateway
go run ./cmd/mock-gateway conformance \
  --vectors ./testdata/template-conformance/cases.json
```

或显式传入 `--vectors`。

## 端口被占用

默认端口：

| 进程 | 端口 |
|---|---|
| `serve` | `127.0.0.1:18080` |
| `gateway` | `127.0.0.1:18081` |

用 `--listen` 改端口。两者端口不同，可以同时运行。

## UI 空白或要求 Token

- 打开 `http://127.0.0.1:18080/`，不是 API 路径；
- 如果设置了 `--ui-token`，在 UI 输入同一个 Token；
- 执行 `curl http://127.0.0.1:18080/healthz`；
- 检查进程是否启动失败。

## 远程 UI 返回 401/403

远程访问必须同时提供：

```bash
mock-gateway serve \
  --config ./config.yaml \
  --listen 0.0.0.0:18080 \
  --allow-remote \
  --ui-token 'long-random-token'
```

API 请求带 `X-Mock-Token` 或 `Authorization: Bearer`。

## 事件没有出现在 shield-x

- 检查 `report` 配置和控制台队列/spool 统计；
- 确认 `report.spool_dir` 存在且可写；
- 搜索 413/429/5xx 日志；
- 401 时数据面仍可返回响应，但事件无法上报；
- 至少等待一个 `report.interval` 加一次上传时间。

## 拦截页变成内置兜底页

策略引用的模板缺失，或模板渲染失败。事件会标记 `action_result=failed`。检查模板是否已拉取、版本是否匹配。

## 目标地址一直没有被访问

这是预期行为。mock 只返回 `Location`，不代理、不访问目标。需要验证目标链路时请使用真实网关。

## 远程 IP 策略不生效

默认忽略 `X-Forwarded-For`。只有位于可信代理之后、且代理会覆盖该头时，才设置 `ingress.trust_proxy: true`。不要在直接暴露的监听器上开启。

## 获取帮助

提交 issue 时附上：

- `mock-gateway version`；
- 操作系统和 Go 版本（源码构建时）；
- 完整命令；
- 脱敏后的配置；
- 相关日志；
- 期望和实际的状态码/Header。

不要包含真实 Token、Cookie 或生产地址。
