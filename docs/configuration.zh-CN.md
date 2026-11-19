# 配置参考

[English](configuration.md) | **简体中文**

mock-gateway 读取一个 YAML 文件。未写出的字段使用默认值。复制
[`config.example.yaml`](../config.example.yaml)，只修改需要的部分。

```bash
cp config.example.yaml config.yaml
```

时长使用 Go 语法：`500ms`、`5s`、`2m`、`1h`。

## `shieldx`

| 字段 | 类型 | 默认值 | 必填 | 说明 |
|---|---|---|---|---|
| `base_url` | string | `http://127.0.0.1:8080/shieldx` | 是 | shield-x 地址，包含 `/shieldx` |
| `token` | string | 空 | 连接模式必填 | 站点 Token；不要提交 |
| `gateway_id` | string | `gw-mock-01` | 是 | 当前模拟网关唯一 ID |
| `agent_version` | string | `mock-gateway/<version>` | 否 | 上报的 agent 版本 |
| `capabilities.actions` | list | 全部四个动作 | 否 | 声明的动作能力 |
| `capabilities.match_fields` | list | 全部支持字段 | 否 | 可求值的策略字段 |
| `capabilities.block_templates` | bool | `true` | 否 | 是否支持拦截模板 |
| `capabilities.template_spec_version` | int | `1` | 否 | 模板规范版本 |
| `capabilities.max_rules` | int | `5000` | 否 | 最大策略数 |

服务端会用能力声明过滤策略。如果策略使用了未声明的动作或字段，可能被过滤掉。

## `poll`

| 字段 | 类型 | 默认值 | 说明 |
|---|---|---|---|
| `interval` | duration | `5s` | 拉策略/模板周期；最小 `1s` |
| `timeout` | duration | `3s` | 单次请求超时 |
| `persist_path` | string | `./data/policy-snapshot.json` | 本地快照路径 |
| `use_etag` | bool | `true` | 使用 ETag 和处理 `304` |

快照用于重启后在首次轮询完成前继续使用上次 revision。

## `template`

| 字段 | 类型 | 默认值 | 说明 |
|---|---|---|---|
| `fetch_on_missing` | bool | `true` | 策略引用但本地缺失时拉模板 |
| `timeout` | duration | `3s` | 模板请求超时 |

## `report`

| 字段 | 类型 | 默认值 | 说明 |
|---|---|---|---|
| `interval` | duration | `2s` | 批次 flush 周期 |
| `batch_size` | int | `300` | 每批事件数；限制 1–1000 |
| `format` | string | `json` | `json` 或 `ndjson` |
| `gzip` | bool | `true` | 压缩事件批次 |
| `timeout` | duration | `10s` | 上报超时 |
| `spool_dir` | string | `./data/spool` | 上报失败落盘目录 |
| `spool_max_mb` | int | `200` | 最大 spool 大小 |
| `max_backoff` | duration | `60s` | 最大重试退避 |

## `generate`

合成流量用于 `run` 和 `loadtest`。`gateway` 数据面不使用这一段。

| 字段 | 类型 | 默认值 | 说明 |
|---|---|---|---|
| `rate` | int | `200` | 每秒生成事件数 |
| `workers` | int | `4` | 生成 worker 数 |
| `duration` | duration | `0` | `0` 表示一直运行 |
| `backfill` | duration | `0` | `run --backfill` 的回填窗口 |
| `ramp` | list | 空 | 速率台阶：`{at, rate}` |
| `paths` | list | 示例路径 | 模拟路径/方法权重 |
| `host_pool` | list | 示例域名 | 模拟 Host 权重 |
| `ua_pool` | list | 示例 UA | 模拟 User-Agent 权重 |
| `ip_pools.normal` | list | TEST-NET 段 | 正常客户端 CIDR |
| `ip_pools.malicious` | list | `203.0.113.66` | 恶意客户端 CIDR |
| `ip_pools.internal` | list | `10.0.0.0/8` | 内网客户端 CIDR |
| `status_distribution` | map | 200/302/404/500 | 上游状态码权重 |
| `bad_event_ratio` | float | `0.001` | 非法事件注入比例 |
| `gateway_error_ratio` | float | `0.0005` | `gateway_error` 注入比例 |
| `timeout_ratio` | float | `0.001` | 超时事件注入比例 |
| `sampled_ratio` | float | `0` | 模拟采样率 |

`ramp` 示例：

```yaml
generate:
  rate: 100
  ramp:
    - { at: "0s",   rate: 100 }
    - { at: "60s",  rate: 1000 }
    - { at: "180s", rate: 3000 }
```

## `fault`

| 字段 | 类型 | 默认值 | 说明 |
|---|---|---|---|
| `shieldx_down` | bool | `false` | 让所有 shield-x 请求失败 |
| `http_status_override` | int | `0` | 强制返回 `429`/`503` 等 |
| `latency` | duration | `0s` | 人为延迟 |
| `drop_percent` | int | `0` | 随机丢请求百分比 |

故障注入用于验证重试、spool 和告警行为，不是生产功能。

## `ingress`

供 `gateway` 使用。

| 字段 | 类型 | 默认值 | 说明 |
|---|---|---|---|
| `listen` | string | `127.0.0.1:18081` | 数据面监听地址 |
| `allow_remote` | bool | `false` | 允许非 loopback |
| `mock_status` | int | `200` | 未命中/observe 的 mock 状态 |
| `mock_content_type` | string | `text/plain; charset=utf-8` | mock 响应类型 |
| `mock_body` | string | `mock upstream\n` | mock 响应体 |
| `read_timeout` | duration | `5s` | HTTP 读超时 |
| `write_timeout` | duration | `10s` | HTTP 写超时 |
| `trust_proxy` | bool | `false` | 信任代理的 `X-Forwarded-For` |

只有可信反向代理设置该头时才开启 `trust_proxy`，否则客户端可以伪造源 IP。

## 校验

启动时会在以下情况失败：

- 缺少 `shieldx.base_url`；
- `generate.rate` 为负；
- `report.format` 非法；
- `ingress.mock_status` 不在 100–599；
- `allow_remote` 为 false 但监听地址非 loopback。

## 回填示例

`config.backfill.yaml` 是历史事件回填的独立示例，日常使用不需要。
