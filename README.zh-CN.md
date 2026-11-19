# mock-gateway

[![CI](https://github.com/linyu19872008/shield-x-mock-gateway/actions/workflows/ci.yml/badge.svg)](https://github.com/linyu19872008/shield-x-mock-gateway/actions/workflows/ci.yml)
[![Release](https://img.shields.io/github/v/release/linyu19872008/shield-x-mock-gateway)](https://github.com/linyu19872008/shield-x-mock-gateway/releases)
[![Go](https://img.shields.io/github/go-mod/go-version/linyu19872008/shield-x-mock-gateway)](go.mod)
[![License: MIT](https://img.shields.io/badge/license-MIT-blue.svg)](LICENSE)

[English](README.md) | **简体中文**

用于 **shield-x agent-v1 契约** 的独立网关模拟器。它在本地求值 shield-x 策略，返回 mock HTTP 响应，生成合成事件，并通过 `shield-x-client-go` 上报真实事件。

它是开发和测试工具，**不是生产 WAF**，也不会把请求代理到业务上游。

## 它是什么 / 不是什么

| 它是 | 它不是 |
|---|---|
| 本地策略求值器和 HTTP 数据面模拟器 | 生产网关或 WAF |
| shield-x 开发用的合成事件源 | 到业务应用的逆向代理 |
| `shield-x-client-go` 的 SDK E2E 驱动 | 真实网关联调的替代品 |
| 模板一致性向量的运行器 | 真实网关行为完全一致的保证 |

## 特性

- 五个子命令：`conformance`、`serve`、`gateway`、`run`、`loadtest`。
- 本地求值 shield-x 策略，遵循 first-match 语义。
- mock `200`、`403`、`301`/`302`/`307` 和 JavaScript 挑战响应。
- 内嵌控制台：策略/模板查看、流量控制、故障注入、上报统计、日志。
- 经由 SDK 批量上报事件，支持磁盘 spool 和补发。
- 语言无关的模板一致性向量。

## UI 语言

内嵌控制台支持英文和简体中文。

- 没有已保存的语言偏好时，中文浏览器默认中文，其余默认英文；
- 使用侧栏的 `EN / 中文` 切换；
- 选择保存在 `mock_lang` Cookie 中；
- 截图或自动化可通过 `?lang=en` / `?lang=zh` 覆盖。

## 环境要求

- 从源码或 `go install` 安装时需要 Go **1.26+**。
- `serve`、`gateway`、`run`、`loadtest` 需要可访问的 shield-x 实例和站点 Token。
- `conformance` 不需要 shield-x。

## 安装

### Go install

```bash
go install github.com/linyu19872008/shield-x-mock-gateway/cmd/mock-gateway@v0.1.2
```

安装后的二进制名为 `mock-gateway`。

### Release 二进制

从 [Releases](https://github.com/linyu19872008/shield-x-mock-gateway/releases) 下载，并使用发布页的 `checksums.txt` 校验。

### 从源码构建

```bash
git clone https://github.com/linyu19872008/shield-x-mock-gateway.git
cd shield-x-mock-gateway
go build -o bin/mock-gateway ./cmd/mock-gateway
```

> `go install` 的二进制不包含测试向量。运行 `conformance` 时请克隆仓库使用内置向量，或通过 `--vectors` 指定外部文件。

## 快速开始

### 1. 离线运行 conformance

```bash
git clone https://github.com/linyu19872008/shield-x-mock-gateway.git
cd shield-x-mock-gateway
go run ./cmd/mock-gateway conformance \
  --vectors ./testdata/template-conformance/cases.json
```

预期：14/14 通过，退出码 `0`。

### 2. 连接 shield-x 并打开控制台

1. 在 shield-x 控制台创建站点 Token 和网关 ID。
2. 复制示例配置并修改连接字段：

```bash
cp config.example.yaml config.yaml
```

```yaml
shieldx:
  base_url: "http://127.0.0.1:8080/shieldx"
  token: "sx_REPLACE_ME"
  gateway_id: "gw-mock-01"
```

3. 启动控制台：

```bash
./bin/mock-gateway serve --config ./config.yaml
```

4. 打开 <http://127.0.0.1:18080/>。

控制台可以启停 `run` / `loadtest`、查看策略快照和模板、注入故障、查看上报与 spool 统计。

### 3. 通过数据面验证 302

先在 shield-x 创建一条重定向策略，然后启动数据面：

```bash
./bin/mock-gateway gateway \
  --config ./config.yaml \
  --listen 127.0.0.1:18081 \
  --gateway-id gw-dataplane-check
```

发送命中策略的请求。例如策略匹配 `AhrefsBot`：

```bash
curl -sS -D- -o /dev/null \
  -A 'AhrefsBot/7.0' \
  http://127.0.0.1:18081/crawler-test
```

预期：

```http
HTTP/1.1 302 Found
Location: https://example.com/trap
Cache-Control: no-store
```

`Location` 来自策略的 `redirect_url`。mock **不会访问该目标**，只返回响应。请使用你控制的地址，并只对测试策略使用。

## 子命令

| 子命令 | 用途 | 需要 shield-x |
|---|---|---|
| `conformance` | 本地跑模板一致性向量 | 否 |
| `serve` | 控制台 UI + 控制 API | 是 |
| `gateway` | 接收真实 HTTP 请求并返回 mock 响应 | 是 |
| `run` | 持续生成合成事件 | 是 |
| `loadtest` | 固定速率压测并输出 JSON 报告 | 是 |
| `version` | 打印版本 | 否 |

完整参数见 [CLI 参考](docs/cli.zh-CN.md)。

## 配置

最小配置：

```yaml
shieldx:
  base_url: "http://127.0.0.1:8080/shieldx"
  token: "sx_REPLACE_ME"
  gateway_id: "gw-mock-01"

poll:
  interval: 5s

ingress:
  listen: "127.0.0.1:18081"
  trust_proxy: false
```

- `base_url` 必须包含 `/shieldx` 路径前缀。
- `token` 是站点 Token，不要提交到仓库。
- `gateway_id` 每个模拟网关应唯一。
- `ingress.trust_proxy` 默认 `false`，只有位于可信代理后才开启。
- `serve` 还支持 `--config`、`--listen`、`--allow-remote`、`--ui-token`。

完整字段见 [配置参考](docs/configuration.zh-CN.md)。

## 兼容性

| 组件 | 版本 |
|---|---|
| mock-gateway | v0.1.2 |
| Go | 1.26+ |
| shield-x-client-go | v0.1.1 |
| 契约 | agent-v1 |

`v0.1.0` 使用了旧的模块布局，已 retract。请使用 `v0.1.2`。

## 安全边界

- 控制台和数据面默认只监听 loopback。
- 远程访问控制台必须使用 `--allow-remote --ui-token`。
- 默认忽略 `X-Forwarded-For`；只有 `ingress.trust_proxy: true` 时才使用。
- mock 不会把请求转发到业务上游。
- 不要把它当作生产安全控制。
- 不要把真实 Token 贴到 issue、日志或截图里。

详见 [数据面](docs/data-plane.zh-CN.md) 和 [故障排查](docs/troubleshooting.zh-CN.md)。

## 文档

- [文档索引](docs/README.zh-CN.md)
- [快速开始](docs/quickstart.zh-CN.md)
- [CLI 参考](docs/cli.zh-CN.md)
- [配置参考](docs/configuration.zh-CN.md)
- [数据面](docs/data-plane.zh-CN.md)
- [架构](docs/architecture.zh-CN.md)
- [设计说明](docs/design.zh-CN.md)
- [agent-v1 契约参考](docs/contract/agent-v1.zh-CN.md)
- [故障排查](docs/troubleshooting.zh-CN.md)

## 开发

```bash
go test ./...
go vet ./...
go build ./...
make conformance
make docs-check
```

PR 要求和翻译规则见 [CONTRIBUTING.md](CONTRIBUTING.md)。

## 许可证

MIT。见 [LICENSE](LICENSE)、[NOTICE](NOTICE) 和 [THIRD_PARTY_NOTICES.md](THIRD_PARTY_NOTICES.md)。

`testdata/template-conformance` 向量和 `docs/contract/agent-v1.zh-CN.md` 来自 shield-x，保持 Apache-2.0；详见 NOTICE。
