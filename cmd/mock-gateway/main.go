// mock-gateway is a standalone gateway simulator for the shield-x agent-v1 contract.
package main

import (
	"fmt"
	"os"

	"github.com/linyu19872008/shield-x-mock-gateway/internal/cli"
	"github.com/linyu19872008/shield-x-mock-gateway/internal/version"
)

func main() {
	if len(os.Args) < 2 {
		usage()
		os.Exit(2)
	}

	switch os.Args[1] {
	case "conformance":
		os.Exit(cli.Conformance(os.Args[2:]))
	case "serve":
		os.Exit(cli.Serve(os.Args[2:]))
	case "run":
		os.Exit(cli.Run(os.Args[2:]))
	case "loadtest":
		os.Exit(cli.LoadTest(os.Args[2:]))
	case "gateway":
		os.Exit(cli.Gateway(os.Args[2:]))
	case "version":
		fmt.Println(version.Version)
	case "help", "-h", "--help":
		usage()
	default:
		usage()
		fmt.Fprintf(os.Stderr, "\n未知子命令：%s\n", os.Args[1])
		os.Exit(2)
	}
}

func usage() {
	fmt.Fprint(os.Stderr, `mock-gateway — shield-x 的开发数据源 / 契约测试 / 压测发生器

用法：
  mock-gateway <子命令> [参数]

子命令：
  conformance   跑模板渲染一致性测试向量（不依赖 shield-x）
  serve         控制台：HTTP API + 可启停的运行控制
  gateway       数据面：接收真实请求，本地求值后返回 mock 响应
  run           常驻：轮询策略 + 持续生成并上报事件
  loadtest      压测：固定速率 + 时长，输出 JSON 报告
  version       打印版本

示例：
  mock-gateway conformance --vectors ./testdata/template-conformance/cases.json
  mock-gateway serve --config ./config.example.yaml
  mock-gateway gateway --config ./config.example.yaml
  mock-gateway run --config ./config.example.yaml
  mock-gateway loadtest --config ./config.yaml --rate 3000 --duration 10m
`)
}
