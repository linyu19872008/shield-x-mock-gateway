# 快速开始

[English](quickstart.md) | **简体中文**

本指南包含三条独立路径：

1. 离线运行模板一致性向量；
2. 把控制台连接到 shield-x；
3. 通过 HTTP 数据面验证重定向策略。

## 前置条件

| 需要 | 说明 |
|---|---|
| Go 1.26+ | 使用 `go install` 或源码构建时 |
| shield-x 地址 | 例如 `http://127.0.0.1:8080/shieldx` |
| 租户 Token | 在 shield-x 控制台创建；不要提交到仓库 |
| 网关 ID | 每个模拟网关唯一，例如 `gw-mock-01` |

Release 二进制不需要 Go。`conformance` 不需要 shield-x。

## 路径 1：离线 conformance

测试向量在本仓库中，先克隆：

```bash
git clone https://github.com/linyu19872008/shield-x-mock-gateway.git
cd shield-x-mock-gateway
go run ./cmd/mock-gateway conformance \
  --vectors ./testdata/template-conformance/cases.json
```

成功时退出码 `0`，并输出 `14/14`。

如果使用 `go install` 安装，二进制不包含向量，需要传入外部路径：

```bash
mock-gateway conformance --vectors /path/to/cases.json
```

## 路径 2：控制台

```bash
cp config.example.yaml config.yaml
```

修改连接配置：

```yaml
shieldx:
  base_url: "http://127.0.0.1:8080/shieldx"
  token: "sx_REPLACE_ME"
  gateway_id: "gw-mock-01"
```

启动：

```bash
./bin/mock-gateway serve --config ./config.yaml
```

打开 <http://127.0.0.1:18080/>。首次拉策略通常在 5 秒内完成。控制台会显示策略 revision、模板、上报统计、队列深度和 spool 状态。

常用检查：

```bash
curl -sS http://127.0.0.1:18080/healthz
curl -sS http://127.0.0.1:18080/api/status
```

远程访问必须同时提供两个参数：

```bash
./bin/mock-gateway serve \
  --config ./config.yaml \
  --listen 0.0.0.0:18080 \
  --allow-remote \
  --ui-token 'choose-a-long-random-token'
```

## 路径 3：HTTP 数据面和重定向

数据面是独立监听器。它在本地求值策略快照并返回 mock 响应，不代理到业务上游。

```bash
./bin/mock-gateway gateway \
  --config ./config.yaml \
  --listen 127.0.0.1:18081 \
  --gateway-id gw-dataplane-check
```

先在 shield-x 创建重定向策略。策略决定哪些请求命中、跳转到哪里。例如策略可以匹配 `AhrefsBot`，跳转到你控制的地址。

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
Content-Length: 0
```

mock 不会访问 `https://example.com/trap`，只返回策略定义的响应。请使用你控制的地址，不要把测试指向第三方站点。

没有命中策略的请求返回配置的 mock 上游响应，通常是 `200` + `mock upstream`。

## 验证结果

1. 请求得到预期状态码和 `Location`；
2. shield-x 事件出现 `action=redirect`、`status=302`；
3. 普通浏览器 UA 不会被重定向；
4. 同时运行多个模拟器时 `gateway_id` 唯一；
5. mock 没有访问目标地址。

## 下一步

- [CLI 参考](cli.zh-CN.md)
- [配置](configuration.zh-CN.md)
- [数据面](data-plane.zh-CN.md)
- [故障排查](troubleshooting.zh-CN.md)
