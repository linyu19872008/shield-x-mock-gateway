# CLI 参考

[English](cli.md) | **简体中文**

```text
mock-gateway <command> [flags]
```

`mock-gateway` 包含五个运行子命令和 `version`。

## 退出码

| 码 | 含义 |
|---|---|
| `0` | 成功 |
| `1` | 运行或配置失败 |
| `2` | 参数错误或未知子命令 |

## `conformance`

运行语言无关的模板渲染向量，不需要 shield-x。

```bash
mock-gateway conformance \
  --vectors ./testdata/template-conformance/cases.json
```

| 参数 | 默认值 | 说明 |
|---|---|---|
| `--vectors` | `./testdata/template-conformance/cases.json` | 向量文件路径 |
| `-v` | `false` | 输出每条用例说明 |

退出码 `0` 表示全部通过；失败时打印失败用例并返回 `1`。

## `serve`

启动控制台 UI 和控制 API。默认监听 `127.0.0.1:18080`。

```bash
mock-gateway serve --config ./config.yaml
```

| 参数 | 默认值 | 说明 |
|---|---|---|
| `--config` | 空 | YAML 配置路径 |
| `--listen` | `127.0.0.1:18080` | 控制台监听地址 |
| `--allow-remote` | `false` | 允许非 loopback 监听 |
| `--ui-token` | 空 | 与 `--allow-remote` 同时使用；通过 `X-Mock-Token` 或 Bearer 发送 |

控制台可以启停 `run` / `loadtest`，但不代理业务流量。修改配置后需要重启。

远程示例：

```bash
mock-gateway serve \
  --config ./config.yaml \
  --listen 0.0.0.0:18080 \
  --allow-remote \
  --ui-token 'choose-a-long-random-token'
```

## `gateway`

启动 HTTP 数据面。接收真实请求、在本地求值策略、返回 mock 响应并上报事件。

```bash
mock-gateway gateway --config ./config.yaml
```

| 参数 | 默认值 | 说明 |
|---|---|---|
| `--config` | 空 | YAML 配置路径 |
| `--listen` | `ingress.listen` | 数据面监听；默认 `127.0.0.1:18081` |
| `--allow-remote` | `false` | 允许非 loopback 监听 |
| `--base-url` | 配置值 | 覆盖 `shieldx.base_url` |
| `--token` | 配置值 | 覆盖 `shieldx.token` |
| `--gateway-id` | 配置值 | 覆盖 `shieldx.gateway_id` |

数据面不会转发到业务上游。`observe` 和未命中请求返回配置的 mock 响应。详见 [数据面](data-plane.zh-CN.md)。

## `run`

拉取策略并持续生成合成事件。

```bash
mock-gateway run --config ./config.yaml
```

| 参数 | 默认值 | 说明 |
|---|---|---|
| `--config` | 空 | YAML 配置路径 |
| `--base-url` | 配置值 | 覆盖 `shieldx.base_url` |
| `--token` | 配置值 | 覆盖 `shieldx.token` |
| `--gateway-id` | 配置值 | 覆盖 `shieldx.gateway_id` |
| `--rate` | `0` | 覆盖 `generate.rate`；`0` 表示使用配置 |
| `--backfill` | `0` | 回填时间范围，例如 `1h` |

Ctrl-C 退出时会 flush 剩余事件。

## `loadtest`

固定速率压测并输出 JSON 报告。

```bash
mock-gateway loadtest \
  --config ./config.yaml \
  --rate 3000 \
  --duration 10m \
  --report ./loadtest.json
```

| 参数 | 默认值 | 说明 |
|---|---|---|
| `--config` | 空 | YAML 配置路径 |
| `--base-url` | 配置值 | 覆盖 `shieldx.base_url` |
| `--token` | 配置值 | 覆盖 `shieldx.token` |
| `--gateway-id` | 配置值 | 覆盖 `shieldx.gateway_id` |
| `--rate` | 配置值 | 目标速率，例如 `3000` 或 `3k` |
| `--duration` | `1m` | 压测时长 |
| `--dry-run` | `false` | 只生成不上报，用于确认 mock 自身吞吐 |
| `--report` | stdout | 报告输出路径 |

在把低达成率归因于服务端之前，先跑一次 dry-run。

## `version`

```bash
mock-gateway version
```

打印版本并返回 `0`。
