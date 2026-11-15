# 设计说明

[English](design.md) | **简体中文**

本文面向需要修改 mock-gateway 内部实现的贡献者。使用说明请先读[快速开始](quickstart.zh-CN.md)和[数据面](data-plane.zh-CN.md)。

## 设计目标

1. **策略求值和渲染独立实现。** mock 不 import shield-x 服务端 internal 包，保证它仍是契约的独立检查实现。
2. **agent I/O 走 SDK。** 拉策略、拉模板、上报和 spool 补发统一通过 `shield-x-client-go` 和 `agentlink`。
3. **不代理上游。** mock 是模拟器，返回本地响应，不引入对业务应用的依赖。
4. **响应可预测。** 相同快照和请求得到相同结果，便于测试和 CI。
5. **异常显式化。** 求值异常上报 `gateway_error`，不退化成“未命中”。

## 包结构

```text
cmd/mock-gateway/
  main.go             仅子命令分发
internal/
  contract/           策略、动作、模板、事件、快照模型
  config/             YAML schema、默认值、校验
  evaluator/          first-match 策略求值
  template/           渲染、缓存、兜底页
  generator/          合成流量
  agentlink/          唯一 client-go 适配层 + 模型转换
  state/              策略状态、持久化、统计
  logging/            共享 logger
  engine/             运行时装配、agent link、故障传输
  control/            controller、control API、内嵌 Web UI、web/
  dataplane/          HTTP 数据面
  conformance/        模板一致性运行器
  cli/                子命令参数与编排
  version/            ldflags 版本变量
```

## 策略快照

poller 返回全量快照。`State.Apply`：

- 原子替换策略列表和 revision；
- 写入 `poll.persist_path`；
- 不合并部分更新。

轮询失败保留旧快照。重启后先加载磁盘快照，再等待首次网络轮询。

求值器必须使用服务端给出的顺序，不能按 priority 重新排序，否则不同网关实现可能行为不一致。

## 求值器

支持字段：

```text
ip, host, path, method, header, ua, query, cookie
```

支持组合：

```text
all, any, not
```

支持操作符：

```text
eq, prefix, suffix, contains, regex, in, in_cidr, not_in_cidr
```

缺失字段按空值处理。未知字段或操作符是求值异常，数据面会上报 `gateway_error`。

## 渲染器

渲染器实现契约中的模板替换规则，并提供内置兜底页。模板缺失或非法时仍返回兜底页，但事件标记 `action_result=failed`。

## 数据面

数据面刻意不是反向代理：

- 从请求构造求值输入；
- 选择策略；
- 写 mock 响应；
- 事件入队。

`observe` 和未命中请求返回可配置的 mock 上游内容，便于在没有真实后端的情况下测试策略选择。

`trust_proxy` 默认关闭。开启后使用第一个合法的 `X-Forwarded-For` 地址作为客户端 IP。

## 上报器和 spool

事件通过 SDK reporter 入队。批量、压缩、重试和磁盘 spool 都由 SDK 处理，mock 不实现第二套上报协议。

## 生成器

生成器与数据面独立，负责路径、Host、UA、IP 池、状态码分布和异常事件。`loadtest --dry-run` 是生成器路径的自检。

## 测试策略

- evaluator、renderer、convert、数据面、控制 API 的单元测试；
- template-conformance 渲染向量；
- dry-run 压测生成器开销；
- 302/block 等 HTTP 数据面测试；
- 不依赖生产 shield-x 实例。

## 非目标

- 生产流量处理；
- TLS 终止或 mTLS；
- 真实浏览器挑战；
- 替代真实网关联调。
